# Build stage
FROM node:24-alpine AS builder

# Build-time version injection
ARG VERSION=dev
ARG COMMIT=unknown
ENV VITE_APP_VERSION=$VERSION
ENV VITE_APP_COMMIT=$COMMIT

WORKDIR /app

# If pre-built dist folder is provided, use it
# Otherwise build from source (for backward compatibility / local build)
COPY . .

RUN if [ ! -d "dist" ]; then \
    npm install && \
    npm run build; \
    fi

# Final stage
FROM nginx:alpine

# Copy built assets from builder stage
COPY --from=builder /app/dist /usr/share/nginx/html

# Copy nginx config template and entrypoint
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 80 8443

ENTRYPOINT ["/entrypoint.sh"]
