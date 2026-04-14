.PHONY: build test lint clean

BINARY=popugate
VERSION?=$(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo dev)
COMMIT?=$(shell git rev-parse HEAD 2>/dev/null || echo unknown)
LDFLAGS=-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/popugate/

test:
	go test ./... -v

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

cross-build: build-linux-amd64 build-linux-arm64

build-linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-amd64 ./cmd/popugate/

build-linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-arm64 ./cmd/popugate/

tidy:
	go mod tidy

fmt:
	gofmt -w .
	goimports -w .
