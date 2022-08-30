#!/bin/bash -e

set -e -o pipefail

PROJECT_MODULE="github.com/traefik/hub-agent-kubernetes"
IMAGE_NAME="kubernetes-codegen:latest"

echo "Building codegen Docker image..."
docker build --build-arg KUBE_VERSION=v0.20.15 --build-arg USER=$USER -f "./scripts/codegen.Dockerfile" \
             -t "${IMAGE_NAME}" \
             "."

cmd="/go/src/k8s.io/code-generator/generate-groups.sh all $PROJECT_MODULE/pkg/crd/generated/client/hub $PROJECT_MODULE/pkg/crd/api hub:v1alpha1"

echo "Generating Hub clientSet code ..."
docker run --rm \
           -v "$(pwd):/go/src/${PROJECT_MODULE}" \
           -w "/go/src/${PROJECT_MODULE}" \
           "${IMAGE_NAME}" $cmd

cmd="/go/src/k8s.io/code-generator/generate-groups.sh all $PROJECT_MODULE/pkg/crd/generated/client/traefik $PROJECT_MODULE/pkg/crd/api traefik:v1alpha1"

echo "Generating Traefik clientSet code ..."
docker run --rm \
           -v "$(pwd):/go/src/${PROJECT_MODULE}" \
           -w "/go/src/${PROJECT_MODULE}" \
           "${IMAGE_NAME}" $cmd


cmd="controller-gen crd:crdVersions=v1 paths=./pkg/crd/api/hub/v1alpha1/... output:dir=."

echo "Generating the CRD definitions ..."
docker run --rm \
           -v "$(pwd):/go/src/${PROJECT_MODULE}" \
           -w "/go/src/${PROJECT_MODULE}" \
           "${IMAGE_NAME}" $cmd
