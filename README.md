

- read values from lerna.json
- use args
 - packages folder
 - build all or do a diff
- use LAST_BUILD_COMMIT
- - template env variables
- - use package/kube/
- read deploy.json
- multi cluster support
- dry run


### Environment

### Arguments

#### Image Root
> --image-root
String **required**

This sets the root of a docker image.

Example:
```
--image-root stevelacy becomes stevelacy/package
--image-root gcr.io/project becomes gcr.io/project/package
```


#### Dry Run
> --dry-run
Bool _optional_

This will build all docker images and run a deployment with `--dry-run` set. It does not push the images.

#### Output
> --output
String _optional_
> yaml, json


go run main.go --image-root stevelacy --dry-run
