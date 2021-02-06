#!/bin/bash -e

export ROOT_DOMAIN="dev.example.com"
export NAMESPACE="default"

KUBERNETES_CLUSTER=$(kubectl config current-context)

export CLUSTER_NAME=${KUBERNETES_CLUSTER#*/}

go run main.go \
	--image-root stevelacy \
	--docker-root '.' \
	--cluster-name k3s \
	--only-packages example-1 \
	--command post-deploy \
	--diff "0132547"
	# --docker-args="--build-arg VERSION={{ .Version }} --build-arg PACKAGE={{ .Name }}" \
	# --path ./services
	# --skip-packages example-1 \
