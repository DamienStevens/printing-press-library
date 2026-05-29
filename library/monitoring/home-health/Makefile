.PHONY: build test lint install clean

build:
	go build -o bin/home-health-pp-cli ./cmd/home-health-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/home-health-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/home-health-pp-mcp ./cmd/home-health-pp-mcp

install-mcp:
	go install ./cmd/home-health-pp-mcp

build-all: build build-mcp
