package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/pelletier/go-toml"

	"compress/gzip"
	"docker.io/go-docker"
	"docker.io/go-docker/api/types"
	"io"
	"log"
	"text/template"
)

const manifest_name = "satchel-manifest.toml"
const loadScriptName = "load-images.sh"
const loadScriptTemplate = `#!/bin/bash

repository=${1}

imageFile={{ .OutputFile }}

if [ "$repository" == "" ]; then
    echo "Repository not specified"
    exit 1
fi

echo Loading Images from ${imageFile}
docker load < ${imageFile}

{{ range $key, $value := .Descriptor.Images }}
echo "Tagging {{ .Repository }}:{{ .Tag }} -> ${repository}/{{ .Repository }}:{{ .Tag }}"
docker tag {{ .Repository }}:{{ .Tag }} ${repository}/{{ .Repository }}:{{ .Tag }}
{{ end }}

{{ range $key, $value := .Descriptor.Images  }}
echo "Pushing ${repository}/{{ .Repository }}:{{ .Tag }}"
docker push ${repository}/{{ .Repository }}:{{ .Tag }}
{{ end }} 
`

const usage = `
satchel pulls and packs docker images between private registries

Flags:
`

type Descriptor struct {
	Images []Image `toml:"image"`
}

type Image struct {
	Registry   string `toml:"registry"`
	Repository string `toml:"repository"`
	Tag        string `toml:"tag"`
	Public     bool   `toml:"public"`
}

func (i Image) ImageName() string {
	registry := i.Registry
	if registry != "" {
		registry = registry + "/"
	}

	return fmt.Sprintf("%s%s:%s", registry, i.Repository, i.Tag)
}

func (i Image) ImageNameNoRegistry() string {
	return fmt.Sprintf("%s:%s", i.Repository, i.Tag)
}

var inputFile string
var outputFile string
var includePublic bool

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage)
		flag.PrintDefaults()
		os.Exit(2)
	}

	flag.StringVar(&inputFile, "in", "satchel.toml", "Input TOML manifest to use")
	flag.StringVar(&outputFile, "out", "satchel-images.tgz", "Name of archive file to generate")
	flag.BoolVar(&includePublic, "public", false, "Include public images in the archive")

	flag.Parse()

	validate()
	descriptor := loadDescriptor()

	pullImages(descriptor.Images)
	tagImages(descriptor.Images)

	saveImages(descriptor.Images)
	generateScript(*descriptor)
}

func validate() {
	if _, err := os.Stat(inputFile); err != nil {
		log.Fatalf("Input file '%v' not found", inputFile)
	}
}

func loadDescriptor() *Descriptor {
	tree, err := toml.LoadFile(inputFile)
	if err != nil {
		log.Fatalf("Error Loading File %v: %v", inputFile, err)
	}

	descriptor := &Descriptor{}
	tree.Unmarshal(descriptor)

	for i := range descriptor.Images {
		if descriptor.Images[i].Tag == "" {
			descriptor.Images[i].Tag = "latest"
		}
	}

	return descriptor
}

func pullImages(images []Image) {
	cli := newDockerClient()

	for _, image := range images {
		if image.Public && !includePublic {
			continue
		}

		log.Printf("Pulling Image %s\n", image.ImageName())
		_, err := cli.ImagePull(context.Background(), image.ImageName(), types.ImagePullOptions{})
		if err != nil {
			log.Fatal(err)
		}
	}
}

func tagImages(images []Image) {
	cli := newDockerClient()

	for _, image := range images {
		if image.Public && !includePublic {
			continue
		}

		src := image.ImageName()
		dest := image.ImageNameNoRegistry()

		log.Printf("Tagging Image %s -> %s", src, dest)
		cli.ImageTag(context.Background(), src, dest)
	}
}

func saveImages(images []Image) {
	cli := newDockerClient()

	imageSummaries, err := cli.ImageList(context.Background(), types.ImageListOptions{})
	if err != nil {
		log.Fatalf("Error Getting Docker Image List: %v", err)
	}

	imageIds := findImageIds(images, imageSummaries)

	readerCloser, err := cli.ImageSave(context.Background(), imageIds)
	if err != nil {
		log.Fatalf("Error saving images: %v", err)
	}

	log.Printf("Writing images to %v", outputFile)
	outFile, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("Error Creating Image File %v: %v", outputFile, err)
	}

	gzipWriter, err := gzip.NewWriterLevel(outFile, gzip.BestCompression)
	if err != nil {
		log.Fatalf("Error Creating Compressed Archive %v: %v", outputFile, err)
	}

	defer gzipWriter.Close()
	defer readerCloser.Close()

	_, err = io.Copy(gzipWriter, readerCloser)
}

func generateScript(descriptor Descriptor) {
	t, err := template.New("script").Parse(loadScriptTemplate)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Writing load script '%s'", loadScriptName)
	f, err := os.OpenFile(loadScriptName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
	defer f.Close()

	data := map[string]interface{}{
		"OutputFile": outputFile,
		"Descriptor": descriptor,
	}
	err = t.Execute(f, data)
	if err != nil {
		log.Fatalf("Error generating load script: %v", err)
	}
}

func newDockerClient() *docker.Client {
	cli, err := docker.NewEnvClient()
	if err != nil {
		log.Fatal(err)
	}

	return cli
}

func findImageIds(images []Image, imageSummaries []types.ImageSummary) []string {
	var imageIds []string

	for _, imageSummary := range imageSummaries {
		if imageSummary.ParentID == "" {
			for _, image := range images {
				if image.Public && !includePublic {
					continue
				}

				// Record image ID if it's a root image and has a correct tag
				if containsTag(image.ImageNameNoRegistry(), imageSummary.RepoTags) {
					imageIds = append(imageIds, imageSummary.ID)
					break
				}
			}
		}
	}

	return imageIds
}

func containsTag(tag string, tags []string) bool {
	for _, searchTag := range tags {
		if searchTag == tag {
			return true
		}
	}

	return false
}
