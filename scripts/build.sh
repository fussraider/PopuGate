#!/bin/bash
set -e

VERSION="${1:-0.0.0}"
BINARY="popugate"

echo "Building PopuGate v${VERSION}..."

mkdir -p bin

echo "  linux/amd64..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o "bin/${BINARY}-linux-amd64" ./cmd/popugate/

echo "  linux/arm64..."
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o "bin/${BINARY}-linux-arm64" ./cmd/popugate/

echo "Done. Binaries in bin/"
ls -lh bin/
