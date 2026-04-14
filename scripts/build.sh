#!/bin/bash
set -e

VERSION="${1:-$(git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo dev)}"
COMMIT="$(git rev-parse HEAD 2>/dev/null || echo unknown)"
BINARY="popugate"

echo "Building PopuGate ${VERSION} (${COMMIT})..."

mkdir -p bin

echo "  linux/amd64..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" -o "bin/${BINARY}-linux-amd64" ./cmd/popugate/

echo "  linux/arm64..."
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" -o "bin/${BINARY}-linux-arm64" ./cmd/popugate/

echo "Done. Binaries in bin/"
ls -lh bin/
