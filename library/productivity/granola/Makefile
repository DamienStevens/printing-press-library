.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/granola-pp-cli$(BIN_EXT) ./cmd/granola-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/granola-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/granola-pp-mcp$(BIN_EXT) ./cmd/granola-pp-mcp

install-mcp:
	go install ./cmd/granola-pp-mcp

build-all: build build-mcp
