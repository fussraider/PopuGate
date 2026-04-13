#!/bin/bash

# Configuration
DOMAIN_NAME=$1
EMAIL=$2

if [ -z "$DOMAIN_NAME" ] || [ -z "$EMAIL" ]; then
    echo "Usage: $0 <domain_name> <email>"
    echo "Example: $0 example.com admin@example.com"
    exit 1
fi

# Path to docker-compose
DOCKER_COMPOSE="docker compose"

echo "### Starting Certbot for $DOMAIN_NAME..."

# Create directory for challenge if it doesn't exist
mkdir -p ./certbot/www

# Request certificate
$DOCKER_COMPOSE run --rm --entrypoint \
  "certbot certonly --webroot -w /var/www/certbot \
    --email $EMAIL --agree-tos --no-eff-email \
    -d $DOMAIN_NAME" certbot

echo "### Reloading Nginx to apply changes..."
$DOCKER_COMPOSE exec popugate-web /entrypoint.sh
