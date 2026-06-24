GOBIN := $(shell go env GOPATH)/bin
export PATH := $(GOBIN):$(PATH)

.PHONY: tools generate lint test build

# Install the pinned codegen toolchain.
tools:
	go install github.com/bufbuild/buf/cmd/buf@v1.50.0
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.6
	go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.18.1

# Regenerate Go + Connect code from the proto schema.
generate:
	buf lint
	buf generate

lint:
	buf lint
	go vet ./...

test:
	go test ./...

build:
	go build ./...
