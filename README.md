**[English version](README.md)** | **[Русская версия](README_RU.md)**

<div align="center">
  <img src="web/public/icon-512x512.png" alt="PopuGate" width="192" height="192">

# PopuGate

**A modern MTProto proxy manager for Telegram with a web interface, Telegram bot, and monitoring system.**

[![Build](https://github.com/fussraider/PopuGate/actions/workflows/build.yml/badge.svg)](https://github.com/fussraider/PopuGate/actions/workflows/build.yml)
[![Release](https://github.com/fussraider/PopuGate/actions/workflows/release.yml/badge.svg)](https://github.com/fussraider/PopuGate/actions/workflows/release.yml)
[![GitHub release](https://img.shields.io/github/v/release/fussraider/PopuGate?include_prereleases)](https://github.com/fussraider/PopuGate/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/fussraider/PopuGate?v=0.1.4)](https://goreportcard.com/report/github.com/fussraider/PopuGate?v=0.1.4)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Swagger](https://img.shields.io/badge/API-Swagger-green)](https://petstore.swagger.io/?url=https://raw.githubusercontent.com/fussraider/PopuGate/master/docs/swagger.json)
[![GHCR Backend](https://img.shields.io/badge/ghcr.io-fussraider%2Fpopugate-blue)](https://github.com/fussraider/PopuGate/pkgs/container/popugate)
[![GHCR Web](https://img.shields.io/badge/ghcr.io-fussraider%2Fpopugate--web-blue)](https://github.com/fussraider/PopuGate/pkgs/container/popugate-web)
</div>

> **Disclaimer:** PopuGate is inspired by [MTProxyMax](https://github.com/SamNet-dev/MTProxyMax) — thanks to the author for the idea. This project is developed with active use of AI-assisted tools and may contain rough edges — it is a work in progress. Bug reports and pull requests are welcome.

---

## 🐳 Running in Docker (recommended)

The recommended way to run PopuGate with the built-in web interface and Nginx reverse proxy:

1. Create a `docker-compose.yml` file:
```yaml
services:
  popugate-backend:
    image: ghcr.io/fussraider/popugate:latest
    container_name: popugate-backend
    restart: unless-stopped
    volumes:
      - ./data:/data
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - ADMIN_PASSWORD=mysecretpassword
      - POPUGATE_DATA_DIR=/data
      - TZ=Europe/Moscow
      - HOSTNAME=popugate-backend
    extra_hosts:
      - "host.docker.internal:host-gateway"
    ports:
      - "9090:9090"

  popugate-web:
    image: ghcr.io/fussraider/popugate-web:latest
    container_name: popugate-web
    restart: unless-stopped
    ports:
      - "80:80"
      - "8443:8443"
    environment:
      - DOMAIN_NAME=your-domain.com
    volumes:
      - ./certbot/conf:/etc/letsencrypt:ro
      - ./certbot/www:/var/www/certbot:ro
    depends_on:
      - popugate-backend

  certbot:
    image: certbot/certbot
    container_name: certbot
    volumes:
      - ./certbot/conf:/etc/letsencrypt
      - ./certbot/www:/var/www/certbot
    entrypoint: "/bin/sh -c 'trap exit TERM; while :; do certbot renew; sleep 12h & wait $${!}; done;'"
```

2. Start the stack:
```bash
docker compose up -d
```

<details>
<summary>Building images manually</summary>

```bash
# Backend
DOCKER_BUILDKIT=1 docker buildx build -t popugate . --load

# Frontend
cd web && DOCKER_BUILDKIT=1 docker buildx build -t popugate-web . --load
```
</details>

> **Note:** The proxy engine (`telemt`) runs in `network: host` mode and binds ports (443, etc.) directly on the host machine. Do not forward proxy ports in the `ports` section for `popugate-backend` and do not assign `popugate-web` to port 443.

---

## 🚀 Quick Start (Binary)

Make sure **Docker is installed and running** on the server before launching. Run as `root` (for `iptables` and Docker access).

```bash
# 1. Download the latest release
wget -O popugate https://github.com/fussraider/PopuGate/releases/latest/download/popugate-linux-amd64
chmod +x popugate

# 2. Set the admin password
sudo ./popugate setup

# 3. Start the server
sudo ./popugate server
```

The backend is available on port `8090`, the proxy engine on port `443`. To run in the background, install the systemd service (see below).

**Supported platforms:** Linux (Ubuntu, Debian, CentOS, RHEL, Fedora, Rocky, AlmaLinux, Alpine).

---

## 🌐 Network Settings

| Port | Purpose |
|------|---------|
| `8090` | REST API and web interface (binary mode) |
| `80` / `8443` | Web interface HTTP/HTTPS (Docker Compose) |
| `443` | Incoming MTProto connections (bound by the engine) |
| `9090` | Prometheus metrics |

Proxy and metrics ports are configured individually per instance.

---

## ⚙️ Configuration

### Environment Variables

| Variable | Description |
|----------|-------------|
| `ADMIN_PASSWORD` | Admin password (first run) |
| `POPUGATE_DATA_DIR` | Working directory (database, configs, caches). Also set via `--data` (`-d`) flag |
| `POPUGATE_DEPLOYMENT` | Deployment type (`docker` — set automatically in the image) |
| `DEBUG` / `GIN_MODE` | Debug mode (`true`/`debug` = debug, default is release) |
| `LOG_LEVEL` | Log level (`debug`, `info`, `warn`, `error`, `fatal`) |
| `BACKUP_ENCRYPTION_KEY` | Backup encryption key (64 hex characters, AES-256-GCM) |
| `TELEMT_VERSION` | Override the telemt engine version |
| `TELEMT_COMMIT` | Override the commit/ref for engine build |
| `TELEMT_REPO` | Override the engine repository URL |

### Command-line Flags

- `--port <number>` (`-p`) — HTTP server port (default: `8090`)
- `--data <path>` (`-d`) — working directory (overrides `POPUGATE_DATA_DIR`)
- `--db <path>` — path to the SQLite file (default: `<data-dir>/settings.db`)

```bash
# Example: run with a custom configuration
sudo -E ./popugate server --port 9090 --data /var/lib/popugate
```

---

## 🔒 HTTPS Setup (SSL)

When using Docker Compose, you can set up automatic Let's Encrypt SSL certificate issuance:

```bash
sudo ./scripts/init-ssl.sh your-domain.com your-email@example.com
```

Specify the domain in `docker-compose.yml`: `DOMAIN_NAME=your-domain.com`. Certificates are renewed automatically every 12 hours. Port 80 must be accessible from the internet for Let's Encrypt to work.

---

## 🖥️ Web Interface Features

The web interface supports **light/dark themes** and **bilingual UI** (Russian / English).

### 📊 Dashboard
Proxy status, active secrets, connections, traffic, quick actions (start/stop/restart), system health, real-time resource monitoring (CPU, memory, disk).

### 🔑 Secrets
Access key management: creation, deletion, rotation, limits (connections, IPs, traffic quota), expiration, QR codes, tags, archiving, bulk operations, search, JSON export/import.

### 📋 Templates
Pre-configured limit presets (connections, IPs, quota, expiration, tags) for quick application to secrets.

### 🖥️ Instances
Independent proxies with their own port, masking domains, FakeTLS, and access tags. Multi-domain support, hot reload, logs (SSE), bulk operations.

### 🔀 Upstreams
Proxy chains (SOCKS4/SOCKS5) with weight-based balancing and network interface binding.

### 🌍 Geoblock
Country-based access restrictions (blacklist/whitelist) via `iptables`.

### 📈 Traffic
Global statistics, real-time active connections, per-secret detailed stats.

### 🤖 Telegram Bot
Proxy management, statistics, secret creation, QR codes, and scheduler task monitoring — right from Telegram.

### 🔄 Replication
Master-Slave synchronization of settings and secrets between servers over SSH.

### 💾 Backups
Automatic daily backups (database, engine configs, SSH keys) with retention-based rotation. Optional AES-256-GCM encryption. Download and restore via the web interface.

### 🕐 Scheduler
Background task management: enable/disable, change schedules (cron), manual runs, execution history with error details.

**Default tasks:** traffic-flush, quota-check, expiry-check, health-check, upstream-health, telegram-report, replication-sync, update-check, telemt-check, token-cleanup, daily-backup, backup-cleanup, history-cleanup, quota-reset, auto-rotate.

### 🆙 Updates
Automatic update checks and manual application. Binary mode — downloads from GitHub + restarts systemd. Docker mode — pulls a new image + recreates the container.

### 🐳 Docker
Docker availability check, installation, building and updating the `telemt` engine image.

### 🖥️ System Menu
Install/remove the systemd service, restart, view status and system information.

### ⚙️ Settings
Global parameters: Docker CPU/memory limits, custom IP, FakeTLS, PROXY protocol, custom Telegram URLs, Ad Tag, secret auto-rotation, maintenance mode, backup rotation, debug mode.

---

## 🛠 System Service (Systemd)

PopuGate supports native installation as a `systemd` service — auto-start on boot and automatic restart on crashes. Installation is available via the web interface (System section).

```bash
sudo systemctl status popugate
sudo systemctl restart popugate
sudo systemctl stop popugate
```

---

## 🛠 Development

### Building the Backend

```bash
make tidy        # Install dependencies
make build       # Build for current OS → bin/popugate
make cross-build # linux/amd64 + linux/arm64
```

Requirements: **Go 1.26+**, **Make**.

### Building the Frontend

```bash
cd web
pnpm install
pnpm run build   # → web/dist/
```

Built files are served via Nginx (see Docker Compose) or another web server that proxies to the backend.

### Testing and Linting

```bash
make test   # All tests (in-memory SQLite, no Docker required)
make lint   # golangci-lint
make fmt    # gofmt + goimports
```

Tests are isolated and do not require Docker or a network environment.
