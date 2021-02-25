.PHONY: clean check test build \
		publish publish-latest image image-dev lint

BIN_NAME := neo-agent
MAIN_DIRECTORY := ./cmd/agent

TAG_NAME := $(shell git tag -l --contains HEAD)
SHA := $(shell git rev-parse --short HEAD)
VERSION := $(if $(TAG_NAME),$(TAG_NAME),$(SHA))
BUILD_DATE := $(shell date -u '+%Y-%m-%d_%I:%M:%S%p')

default: clean check test build

lint:
	golangci-lint run

clean:
	rm -rf cover.out

test: clean
	go test -v -race -cover ./...

build: clean
	@echo Version: $(VERSION) $(BUILD_DATE)
	CGO_ENABLED=0 go build -trimpath -ldflags '-X "main.version=${VERSION}" -X "main.commit=${SHA}" -X "main.date=${BUILD_DATE}"' -o ${BIN_NAME} ${MAIN_DIRECTORY}

image: build
	docker build -t gcr.io/traefiklabs/$(BIN_NAME):$(VERSION) .

image-dev: build
	docker build -t neo-agent:dev .

publish:
	docker push gcr.io/traefiklabs/$(BIN_NAME):$(VERSION)

publish-latest:
	docker tag gcr.io/traefiklabs/$(BIN_NAME):$(VERSION) gcr.io/traefiklabs/$(BIN_NAME):latest
	docker push gcr.io/traefiklabs/$(BIN_NAME):latest

check:
	golangci-lint run
