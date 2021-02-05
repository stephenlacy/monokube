# monokube
> Monorepo deployment management

Works with lerna, yarn, and stand-alone monorepos

![monokube.gif](assets/monokube.gif)


```
$ monokube --image-root stevelacy

building 1 package(s)
Step 1/8 : FROM golang:1.14.11-alpine3.11 as build_image
...
 ---> 18d162cff2a9
Successfully tagged stevelacy/example-1:1.2.3-d80f667
built image: stevelacy/example-1:1.2.3-d80f667
The push refers to repository [docker.io/stevelacy/example-1]
...
running deployments
deployment.apps/example-1 configured
service/example-1-staged unchanged
Waiting for deployment "example-1" rollout to finish: 1 out of 3 new replicas have been updated...
Waiting for deployment "example-1" rollout to finish: 2 out of 3 new replicas have been updated...
Waiting for deployment "example-1" rollout to finish: 1 old replicas are pending termination...
deployment "example-1" successfully rolled out
running post-deployment tasks
service/example-1 unchanged
ingress.extensions/example-1 configured
deployment "example-1" successfully rolled out
all done
```


All together:
```
monokube \
	--image-root stevelacy \
	--docker-args="--build-arg PACKAGE={{ .Name }}" \
	--cluster-name $CLUSTER_NAME \
	--only-packages example-1 \
	--diff "0132547" \
	--path ./packages \
	--skip-packages example-1 \
	--command post-deploy
```


### Installing

Download the [latest linux-amd64.tar.gz](https://github.com/stevelacy/monokube/releases/latest/download/monokube-linux-amd64.tar.gz) or view the releases list [here](https://github.com/stevelacy/monokube/releases)

```
tar -xf monokube-linux-amd64.tar.gz
chmod +x monokube
./monokube
```

To install it globally to your usr/bin:
```
sudo mv monokube /usr/local/bin
```

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

#### Command
> --command
String _optional_ pre-build | build | pre-deploy | deploy | post-deploy

If this flag is not provided, all tasks will be run in order: `pre-build` `build` `pre-deploy` `deploy` `post-deploy`

If the flag is provided only that task will be run

Note for all `.sh` scripts: The env is modified to include:
- PACKAGE="package-name"
- IMAGE_ROOT="--image-root || imageRoot"

##### pre-build
> applies all kubernetes pre-build manifests and runs all `pre-build.sh` scripts
Both of these files are either applied or run, yaml is applied before sh:
- pre-build.yaml
- pre-build.sh

##### build
> builds all docker images

##### pre-deploy
> applies all kubernetes pre-deploy manifests and runs all `pre-deploy.sh` scripts
Both of these files are either applied or run, yaml is applied before sh:
- pre-deploy.yaml
- pre-deploy.sh

##### deploy
> applies all kubernetes manifests

##### post-deploy
> applies all kubernetes post-deploy manifests and runs all `post-deploy.sh` scripts
Both of these files are either applied or run, yaml is applied before sh:
- post-deploy.yaml
- post-deploy.sh

#### Path
> --path
String _optional_
Default: `packages`

monokube looks for packages in the path provided or in the `packages` key of a `lerna.json` file.


#### Dry Run
> --dry-run
Bool _optional_

This will build all docker images and run a deployment with `--dry-run` set. It does not push the images.


#### Output
> --output (-o)
String _optional_  yaml | json


#### Docker Args
> --docker-args
String _optional_

This is passed into the docker build command directly.

Example:
```
--docker-args="--build-arg PACKAGE={{ .Name }}"
--docker-args="--compress --memory 512"
```

#### Cluster Name
> --cluster-name
String _optional_

If provided only the packages with the --cluster-name in their `kube/deploy.yaml` will be deployed

```
--cluster-name dev
```

`./packages/example-1/kube/deploy.yaml`
```
{
	"clusters": [ "dev" ]
}
```

#### Diff
> --diff
String _optional_

Deploys a package only when there are changes between the current repo HEAD and the provided git commit


#### Skip Packages
> --skip-packages
String _optional_

Skip building or deploying provided packages

```
--skip-packages example-1 example-2
```


#### Only Packages
> --only-packages
String _optional_

Only build or deploy provided packages

```
--only-packages example-1 example-2
```


#### Templating

`deployment.yaml` manifest in the `package/kube/` folder is templated with the provided arguments and the ENV

```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Name }}
  namespace: {{ .Env.NAMESPACE }}
  labels:
    app: {{ .Name }}
    version: {{ .Version }}

```

Default values provided:
```
Name:     package folder name
Image:    built docker image
Version:  `version` field from `package/package.json`
Env:      key/value map of the current ENV
```
