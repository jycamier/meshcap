BINARY_NAME := collector
BUILD_DIR := bin
MODULE := github.com/jycamier/meshcap
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build build-wasm test test-integration lint clean helm-lint helm-template

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/collector/

build-wasm:
	cd wasm && GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -ldflags "-X main.version=$(VERSION)" -o plugin.wasm .

test:
	go test -race -cover ./...

test-integration:
	go test -race -tags integration -cover ./...

lint:
	go vet ./...

clean:
	rm -rf $(BUILD_DIR)
	rm -f wasm/plugin.wasm

helm-lint:
	helm lint helm/meshcap/ --set collector.s3Bucket=test-bucket

helm-template:
	helm template test helm/meshcap/ --set collector.s3Bucket=test-bucket
