.PHONY: clean lint test build \
		publish publish-latest image image-dev

BIN_NAME := hub-agent
MAIN_DIRECTORY := ./cmd/agent

TAG_NAME := $(shell git tag -l --contains HEAD)
SHA := $(shell git rev-parse --short HEAD)
VERSION := $(if $(TAG_NAME),$(TAG_NAME),$(SHA))
BUILD_DATE := $(shell date -u '+%Y-%m-%d_%I:%M:%S%p')

default: clean lint test build

lint:
	golangci-lint run

clean:
	rm -rf cover.out

test: clean
	go test -v -race -cover ./...

build: clean
	@echo Version: $(VERSION) $(BUILD_DATE)
	CGO_ENABLED=0 go build -v -trimpath -ldflags '-X "main.version=${VERSION}" -X "main.commit=${SHA}" -X "main.date=${BUILD_DATE}"' -o ${BIN_NAME} ${MAIN_DIRECTORY}

image: export GOOS := linux
image: export GOARCH := amd64
image: build
	docker build -t gcr.io/traefiklabs/$(BIN_NAME):$(VERSION) .

image-dev: export GOOS := linux
image-dev: export GOARCH := amd64
image-dev: build
	docker build -t $(BIN_NAME):dev . -f ./dev.Dockerfile

dev: image-dev
	k3d image import $(BIN_NAME):dev --cluster=k3s-default-hub
	kubectl patch deployment -n hub-agent hub-agent-controller -p '{"spec":{"template":{"spec":{"containers":[{"name":"hub-agent-controller","image":"$(BIN_NAME):dev","imagePullPolicy":"Never"}]}}}}'
	kubectl patch deployment -n hub-agent hub-agent-auth-server -p '{"spec":{"template":{"spec":{"containers":[{"name":"hub-agent-auth-server","image":"$(BIN_NAME):dev","imagePullPolicy":"Never"}]}}}}'
	kubectl rollout restart deployment -n hub-agent hub-agent-controller
	kubectl rollout restart deployment -n hub-agent hub-agent-auth-server

publish:
	docker push gcr.io/traefiklabs/$(BIN_NAME):$(VERSION)

publish-latest:
	docker tag gcr.io/traefiklabs/$(BIN_NAME):$(VERSION) gcr.io/traefiklabs/$(BIN_NAME):latest
	docker push gcr.io/traefiklabs/$(BIN_NAME):latest

generate-crd:
	@$(CURDIR)/scripts/code-gen.sh
