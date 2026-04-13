.PHONY: build test lint clean

BINARY=popugate
VERSION?=0.0.0
LDFLAGS=-s -w -X main.version=$(VERSION)

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
