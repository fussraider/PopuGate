.PHONY: build test lint vulncheck clean swag tidy fmt build-test-version docker-build-test

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

# Reachability-aware CVE scan (govulncheck); allowlist lives in osv-scanner.toml.
# Same script CI runs, so a green `make vulncheck` means a green CI scan.
vulncheck:
	./scripts/vulncheck.sh

clean:
	rm -rf bin/

cross-build: build-linux-amd64 build-linux-arm64

build-linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-amd64 ./cmd/popugate/

build-linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-arm64 ./cmd/popugate/

swag:
	swag init -g cmd/popugate/server.go -o docs/ --parseInternal

tidy:
	go mod tidy

fmt:
	gofmt -w .
	goimports -w .

# Build with a custom version for testing self-update
TEST_VERSION ?= 0.1.0
build-test-version:
	CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$(TEST_VERSION) -X main.commit=test" -o bin/$(BINARY) ./cmd/popugate/

# Build Docker image with a custom version for testing self-update
docker-build-test:
	$(MAKE) build-linux-amd64 VERSION=$(TEST_VERSION)
	docker build --build-arg TARGETARCH=amd64 -t popugate:test-$(TEST_VERSION) .
