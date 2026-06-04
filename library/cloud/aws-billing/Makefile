.PHONY: build test lint install clean

build:
	go build -o bin/aws-billing-pp-cli ./cmd/aws-billing-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/aws-billing-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/aws-billing-pp-mcp ./cmd/aws-billing-pp-mcp

install-mcp:
	go install ./cmd/aws-billing-pp-mcp

build-all: build build-mcp
