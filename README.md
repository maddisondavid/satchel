# Satchel
A command line tool that takes the pain out of shipping Docker images between private registries

Sharing images between private registries requires that those images be packaged up, shipped then tagged and pushed to
the new registry.  It can be time consuming but `satchel` provides a workflow that makes it easy and reproducible.

## Building

```bash
$> go get github.com/maddisondavid/satchel
$> $GOPATH/bin/satchel
```

## Using

### Packing Up Images

1. Define `satchel.toml` file
    ```toml
       [[image]]
         repository = "java"
         tag = "8"
         public = true
    
       [[image]]
         registry = "gcr.io"
         repository = "google-containers/pause"
         tag = "1.0"
         public = false
    ```

2. Execute Satchel
    ```
    $ satchel -in satchel.toml -out my-images.tgz
    2018/07/29 16:13:14 Pulling Image gcr.io/google-containers/pause:1.0
    2018/07/29 16:13:14 Tagging Image gcr.io/google-containers/pause:1.0 -> google-containers/pause:1.0
    2018/07/29 16:13:14 Writing images to my-images.tgz
    2018/07/29 16:13:14 Writing load script 'load-images.sh'
    ```

 3. Satchel creates the compressed TAR archive of the images as well as a script to load and push them:

    ```bash
    $ ls
    load-images.sh      my-images.tgz
    
    $ cat load-images.sh
    #!/bin/bash
    
    repository=${1}
    
    imageFile=my-images.tgz
    
    if [ "$repository" == "" ]; then
        echo "Repository not specified"
        exit 1
    fi
    
    echo Loading Images from ${imageFile}
    docker load < ${imageFile}
    
    echo "Tagging java:8 -> ${repository}/java:8"
    docker tag java:8 ${repository}/java:8
    
    echo "Tagging google-containers/pause:1.0 -> ${repository}/google-containers/pause:1.0"
    docker tag google-containers/pause:1.0 ${repository}/google-containers/pause:1.0
    
    echo "Pushing ${repository}/java:8"
    docker push ${repository}/java:8
    
    echo "Pushing ${repository}/google-containers/pause:1.0"
    docker push ${repository}/google-containers/pause:1.0
    
    ```

### Loading Images

1. Ship both the image archive and the load script to remote site
2. Run `load-images.sh` passing in the name of the registry the images should be loaded into.  The images in the
archive will first be loaded to the local Docker, then tagged and pushed to the specified registry

    ```
    $ ./load-images.sh myregistry.example.com
    
    Loading Images from my-images.tgz
    Loaded image ID: sha256:350b164e7ae1dcddeffadd65c76226c9b6dc5553f5179153fb0e36b78f2a5e06
    Tagging java:8 -> myregistry.example.com/java:8
    Tagging google-containers/pause:1.0 -> myregistry.example.com/google-containers/pause:1.0
    Pushing myregistry.example.com/java:8
    The push refers to repository [myregistry.example.com/java]
    Pushing myregistry.example.com/google-containers/pause:1.0
    The push refers to repository [myregistry.example.com/google-containers/pause]
    ```

#### Notes

- By default `satchel` will not archive any images marked as `public = true` as this indicates the image is publicly available.
Pass `-public` to satchel to archive ALL images.
- Tag defaults to `latest` if not provided


