.PHONY: clean lint test build build-portal \
		publish publish-latest image image-dev multi-arch-image-%

BIN_NAME := hub-agent-kubernetes
MAIN_DIRECTORY := ./cmd/agent

TAG_NAME := $(shell git tag -l --contains HEAD)
SHA := $(shell git rev-parse --short HEAD)
VERSION := $(if $(TAG_NAME),$(TAG_NAME),v0.0.0-$(SHA))
BUILD_DATE := $(shell date -u '+%Y-%m-%d_%I:%M:%S%p')
export DOCKER_BUILDKIT=1

DOCKER_BUILD_PLATFORMS ?= linux/amd64,linux/arm64,linux/arm/v7,linux/arm/v6
DOCKER_IMAGE_TAG ?= $(if $(TAG_NAME),$(TAG_NAME),latest)
OUTPUT := $(if $(OUTPUT),$(OUTPUT),$(BIN_NAME))

default: clean build-portal lint test build

lint:
	golangci-lint run

clean:
	rm -rf cover.out

test: clean
	go test -v -race -cover ./...

build: clean
	@echo Version: $(VERSION) $(BUILD_DATE)
	CGO_ENABLED=0 go build -v -trimpath -ldflags '-X "github.com/traefik/hub-agent-kubernetes/pkg/version.date=${BUILD_DATE}" -X "github.com/traefik/hub-agent-kubernetes/pkg/version.version=${VERSION}" -X "github.com/traefik/hub-agent-kubernetes/pkg/version.commit=${SHA}"' -o ${OUTPUT} ${MAIN_DIRECTORY}

build-portal:
	@make -C $(CURDIR)/portal dist

image: export GOOS := linux
image: export GOARCH := amd64
image: build-portal build
	docker build --build-arg VERSION=$(VERSION) -t ghcr.io/traefik/$(BIN_NAME):$(VERSION) .

image-dev: export GOOS := linux
image-dev: export GOARCH := amd64
image-dev: build
	docker build -t $(BIN_NAME):dev . -f ./dev.Dockerfile

dev: image-dev
	k3d image import $(BIN_NAME):dev --cluster=k3s-default-hub
	kubectl patch deployment -n hub-agent hub-agent-controller -p '{"spec":{"template":{"spec":{"containers":[{"name":"hub-agent-controller","image":"$(BIN_NAME):dev","imagePullPolicy":"Never"}]}}}}'
	kubectl patch deployment -n hub-agent hub-agent-auth-server -p '{"spec":{"template":{"spec":{"containers":[{"name":"hub-agent-auth-server","image":"$(BIN_NAME):dev","imagePullPolicy":"Never"}]}}}}'
	kubectl patch deployment -n hub-agent hub-agent-tunnel -p '{"spec":{"template":{"spec":{"containers":[{"name":"hub-agent-tunnel","image":"$(BIN_NAME):dev","imagePullPolicy":"Never"}]}}}}'
	kubectl patch deployment -n hub-agent hub-agent-dev-portal -p '{"spec":{"template":{"spec":{"containers":[{"name":"hub-agent-dev-portal","image":"$(BIN_NAME):dev","imagePullPolicy":"Never"}]}}}}'
	kubectl rollout restart deployment -n hub-agent hub-agent-controller
	kubectl rollout restart deployment -n hub-agent hub-agent-auth-server
	kubectl rollout restart deployment -n hub-agent hub-agent-tunnel
	kubectl rollout restart deployment -n hub-agent hub-agent-dev-portal

## Build Multi archs Docker image
multi-arch-image-%: build-portal
	docker buildx build $(DOCKER_BUILDX_ARGS) --build-arg VERSION=$(VERSION) -t ghcr.io/traefik/$(BIN_NAME):$* --platform=$(DOCKER_BUILD_PLATFORMS) -f buildx.Dockerfile .

publish:
	docker push ghcr.io/traefik/$(BIN_NAME):$(VERSION)

generate-crd:
	@$(CURDIR)/scripts/code-gen.sh
