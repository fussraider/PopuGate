#!/bin/sh

# Template for Nginx with SSL support on port 8443 and HTTP on port 80
# Use variables: $DOMAIN_NAME, $BACKEND_URL

cat <<EOF > /etc/nginx/conf.d/default.conf
server {
    listen 80;
    server_name ${DOMAIN_NAME:-localhost};

    # Certbot challenge location
    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }

    # Redirect all HTTP to HTTPS if DOMAIN_NAME is set
    # and certificates exist. We replace this placeholder later.
    # HTTP_REDIRECT_PLACEHOLDER

    location / {
        root   /usr/share/nginx/html;
        index  index.html index.htm;
        try_files \$uri \$uri/ /index.html;
    }

    # Proxy API requests to the backend service
    location /api/ {
        proxy_pass ${BACKEND_URL:-http://popugate-backend:8090/api/};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_cache_bypass \$http_upgrade;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}

# SSL server block on port 8443
server {
    listen 8443 ssl;
    server_name ${DOMAIN_NAME:-localhost};

    # Paths to certificates provided by Certbot
    ssl_certificate /etc/letsencrypt/live/${DOMAIN_NAME}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/${DOMAIN_NAME}/privkey.pem;

    # Basic SSL optimization
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers on;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384;
    ssl_session_timeout 1d;
    ssl_session_cache shared:SSL:10m;
    ssl_session_tickets off;

    location / {
        root   /usr/share/nginx/html;
        index  index.html index.htm;
        try_files \$uri \$uri/ /index.html;
    }

    location /api/ {
        proxy_pass ${BACKEND_URL:-http://popugate-backend:8090/api/};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_cache_bypass \$http_upgrade;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF

# Use envsubst to replace variables in the generated config
temp_conf=$(mktemp)
# Export variables so envsubst can see them
export DOMAIN_NAME="${DOMAIN_NAME:-localhost}"
export BACKEND_URL="${BACKEND_URL:-http://popugate-backend:8090/api/}"
envsubst '${DOMAIN_NAME} ${BACKEND_URL}' < /etc/nginx/conf.d/default.conf > "$temp_conf"
mv "$temp_conf" /etc/nginx/conf.d/default.conf

# Check if certificates exist
# Use the exported DOMAIN_NAME or the original one
if [ "$DOMAIN_NAME" = "localhost" ] || [ ! -f "/etc/letsencrypt/live/$DOMAIN_NAME/fullchain.pem" ]; then
    echo "SSL certificates not found or DOMAIN_NAME not set. Disabling SSL block and redirect to prevent startup failure."
    # Remove the SSL server block (lines from '# SSL server block' to end)
    sed -i '/# SSL server block/,$d' /etc/nginx/conf.d/default.conf
    # Remove redirect placeholder
    sed -i '/HTTP_REDIRECT_PLACEHOLDER/d' /etc/nginx/conf.d/default.conf
else
    echo "SSL certificates found for $DOMAIN_NAME. Enabling HTTPS on port 8443 and redirect from 80."
    # Replace placeholder with redirect rule
    sed -i 's|# HTTP_REDIRECT_PLACEHOLDER|return 301 https://$host:8443$request_uri;|' /etc/nginx/conf.d/default.conf
fi

exec nginx -g "daemon off;"
