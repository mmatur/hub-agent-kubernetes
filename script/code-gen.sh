#!/bin/bash -e

set -e -o pipefail

PROJECT_MODULE="github.com/traefik/neo-agent"
IMAGE_NAME="kubernetes-codegen:latest"

echo "Building codegen Docker image...$USER"
docker build --build-arg USER=$USER -f "./script/Dockerfile.codegen" \
             -t "${IMAGE_NAME}" \
             "."

cmd="/go/src/k8s.io/code-generator/generate-groups.sh all $PROJECT_MODULE/pkg/crd/generated/client/neo $PROJECT_MODULE/pkg/crd/api neo:v1alpha1"

echo "Generating Neo clientSet code ..."
echo $(pwd)
docker run --rm \
           -v "$(pwd):/go/src/${PROJECT_MODULE}" \
           -w "/go/src/${PROJECT_MODULE}" \
           "${IMAGE_NAME}" $cmd

cmd="/go/src/k8s.io/code-generator/generate-groups.sh all $PROJECT_MODULE/pkg/crd/generated/client/traefik $PROJECT_MODULE/pkg/crd/api traefik:v1alpha1"

echo "Generating Traefik clientSet code ..."
echo $(pwd)
docker run --rm \
           -v "$(pwd):/go/src/${PROJECT_MODULE}" \
           -w "/go/src/${PROJECT_MODULE}" \
           "${IMAGE_NAME}" $cmd
