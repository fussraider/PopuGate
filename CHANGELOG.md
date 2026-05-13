# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.2] - 2026-05-13

### Fixed
- Replace hardcoded demo password in docker-compose and fix `.gitignore` typo
- Fix command 'update' text in Telegram bot

### Changed
- Translate README to English, keep Russian version as `README_RU.md` with cross-links

## [0.1.1] - 2026-05-13

### Changed
- Overhaul Telegram bot UX with categorized commands, Markdown safety, and improved output formatting

## [0.1.0] - 2026-05-13

### Added
- Per-instance proxy configuration with tag-based access control
- Multi-link generation for Telegram proxy links

## [0.0.13] - 2026-05-10

### Fixed
- DataTable empty state flicker on initial load
- WebSocket live metrics format mismatch
- Load average UX display issues

## [0.0.12] - 2026-05-09

### Fixed
- Bind telemt metrics to `0.0.0.0` when backend runs inside Docker

## [0.0.11] - 2026-05-08

### Added
- Human-readable scheduler task descriptions
- Default-disabled tasks for safer initial setup

### Changed
- Upstream health check UX improvements

## [0.0.10] - 2026-05-07

### Added
- Scheduler management with cron tasks, enable/disable, schedule changes, and execution history
- Backup rotation and automatic cleanup
- Dark theme for the web UI
- Bot `/tasks` command to view scheduler status
- Swagger API documentation
- Traffic history with charts and comprehensive test coverage
- Templates system, audit log, secrets management, and custom tooltips
- Real-time system monitoring, upstream health checks, and enhanced bulk actions
- Encrypted backups with AES-256-GCM, manifest versioning, and checksum verification

### Changed
- Deduplicate `FormatBytes`, clean up `procStat` hack, add service tests

### Fixed
- Clear stale upstream health status on disable, fix status badge logic
- Race conditions, memory leaks, and security issues across backend and frontend

## [0.0.9] - 2026-05-02

### Added
- Telegram notifications system
- Shared frontend components library

### Fixed
- Token refresh race condition
- Upstream test feedback issues

## [0.0.8] - 2026-05-01

### Fixed
- Clear stale telemt update flag on server startup

## [0.0.7] - 2026-05-01

### Added
- Telemt engine update system: check GitHub releases, cache latest version, apply updates
- Frontend UI for engine update controls, status display, and release picker
- `PUT /api/v1/upstreams/:name` endpoint for editing existing upstreams
- Scheduler task for periodic telemt update checks (every 6 hours)
- Settings fields: `telemt_version`, `telemt_commit`, `telemt_repo`

### Changed
- Refactor `DockerService` to use `TelemtConfigProvider` interface instead of model-level constants

## [0.0.6] - 2026-04-29

### Added
- Docker self-update: pull backend+web images from GHCR, recreate container with full config preservation
- Docker-compose project detection and compose-aware container recreation
- Bare-metal web dist update: download `popugate-web-dist.tar.gz` from GitHub Releases with SHA256 verification
- Frontend: Docker/Binary mode badge, mode-specific confirm dialogs, auto-refresh after container restart
- CI: package web dist into release assets with checksums
- `docker-cli-compose` in Dockerfile, `POPUGATE_DEPLOYMENT=docker` env variable

## [0.0.5] - 2026-04-28

### Fixed
- Telegram bot test message sending
- Critical bugs and security issues across backend and frontend

## [0.0.4] - 2026-04-25

### Changed
- Improve database migrations, auth logic, and add unit tests

## [0.0.3] - 2026-04-20

### Added
- Comprehensive test suite and improved error handling
- Internationalization (i18n) support
- PWA support

### Changed
- Refactor web UI, add structured logger

### Fixed
- Statistics calculation errors

## [0.0.2] - 2026-04-14

### Fixed
- Telegram bot settings configuration
- Proxy engine build process
- CI/CD workflows optimization

## [0.0.1] - 2026-04-13

### Added
- Initial release of PopuGate — Telegram MTProto proxy manager
- Go backend with Gin framework and REST API
- Vue + TypeScript + SCSS frontend
- telemt 3.x Rust engine integration (Docker)
- Per-user access control and traffic monitoring
- Master-slave replication via SSH/SFTP
- Proxy chaining and geo-blocking
- Telegram bot integration
- SQLite storage with WAL mode
- JWT authentication

[0.1.2]: https://github.com/fussraider/PopuGate/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/fussraider/PopuGate/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/fussraider/PopuGate/compare/v0.0.13...v0.1.0
[0.0.13]: https://github.com/fussraider/PopuGate/compare/v0.0.12...v0.0.13
[0.0.12]: https://github.com/fussraider/PopuGate/compare/v0.0.11...v0.0.12
[0.0.11]: https://github.com/fussraider/PopuGate/compare/v0.0.10...v0.0.11
[0.0.10]: https://github.com/fussraider/PopuGate/compare/v0.0.9...v0.0.10
[0.0.9]: https://github.com/fussraider/PopuGate/compare/v0.0.8...v0.0.9
[0.0.8]: https://github.com/fussraider/PopuGate/compare/v0.0.7...v0.0.8
[0.0.7]: https://github.com/fussraider/PopuGate/compare/v0.0.6...v0.0.7
[0.0.6]: https://github.com/fussraider/PopuGate/compare/v0.0.5...v0.0.6
[0.0.5]: https://github.com/fussraider/PopuGate/compare/v0.0.4...v0.0.5
[0.0.4]: https://github.com/fussraider/PopuGate/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/fussraider/PopuGate/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/fussraider/PopuGate/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/fussraider/PopuGate/releases/tag/v0.0.1
