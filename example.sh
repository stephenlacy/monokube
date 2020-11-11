#!/bin/bash -e

export ROOT_DOMAIN="dev.example.com"
export NAMESPACE="default"

go run main.go \
	--image-root stevelacy \
	--docker-args="--build-arg PACKAGE={{ .Name }}"
