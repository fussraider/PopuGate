# Docker build for PopuGate
FROM alpine:latest

# Install runtime dependencies
# ca-certificates for HTTPS
# docker-cli to interact with mounted docker.sock
RUN apk add --no-cache ca-certificates docker-cli docker-cli-compose docker-cli-buildx tzdata

WORKDIR /app

# Argument to specify binary architecture based on platform
ARG TARGETARCH

# Copy binary from build context based on target architecture
COPY bin/popugate-linux-${TARGETARCH} /usr/local/bin/popugate
RUN chmod +x /usr/local/bin/popugate

# Create data directory
RUN mkdir -p /data
ENV POPUGATE_DATA_DIR=/data
ENV POPUGATE_DEPLOYMENT=docker

# Expose default API/Web port
EXPOSE 8090

# Entrypoint script to handle initial setup if needed
COPY scripts/docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["server", "--port", "8090", "--data", "/data"]
