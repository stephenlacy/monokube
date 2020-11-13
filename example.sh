#!/bin/bash -e

export ROOT_DOMAIN="dev.example.com"
export NAMESPACE="default"

KUBERNETES_CLUSTER=$(kubectl config current-context)

export CLUSTER_NAME=${KUBERNETES_CLUSTER#*/}

go run main.go \
	--image-root stevelacy \
	--docker-args="--build-arg PACKAGE={{ .Name }}" \
	--cluster-name $CLUSTER_NAME \
	# --command post-deploy
	# --skip-packages example-1 \
