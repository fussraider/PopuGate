# Docker build for PopuGate
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache make git gcc musl-dev

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN make build

# Final image
FROM alpine:latest

# Install runtime dependencies
# ca-certificates for HTTPS
# docker-cli to interact with mounted docker.sock
RUN apk add --no-cache ca-certificates docker-cli tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/popugate /usr/local/bin/popugate

# Create data directory
RUN mkdir -p /data
ENV POPUGATE_DATA_DIR=/data

# Expose default API/Web port
EXPOSE 8080

# Entrypoint script to handle initial setup if needed
COPY scripts/docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["server", "--port", "8080", "--data", "/data"]
