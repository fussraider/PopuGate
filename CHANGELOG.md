# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-05-23

### Added
- **Bot: auto-registration of commands** via Telegram `setMyCommands` API — commands appear as autocomplete suggestions in Telegram clients
- **Bot: `/backup create`** — trigger backup creation directly from Telegram
- **Bot: `/resetquota <label>`** — reset traffic counters for a specific user
- **Bot: `/status` expanded** — now shows engine version, uptime, per-instance status, and traffic summary
- **Bot: `/instances` command** — list all instances with config details (FakeTLS, TCPMSS, TLS fronting, masking)
- **Bot: `/info <label>` command** — detailed secret card with masked key, limits, traffic, tags, and notes
- **Bot: `/geoblock` command** — show geo-blocking mode, countries, and cache freshness
- **Bot: `/replication` command** — show replication role, sync config, and slave list
- **Bot: `/upstreams` command** — list configured upstreams with status and weight
- **Bot: `/tasks` command** — show scheduled tasks status
- **Bot: `/update` command** — show PopuGate and engine version info
- **Bot: message chunking** — long responses split at paragraph boundaries to respect Telegram's 4096-char limit
- **Bot: inline keyboard buttons** — Dashboard and page links in command responses
- **Store: transaction support** — bulk secret operations (`BulkExtendExpiry`, `BulkToggleEnabled`, `BulkSetLimits`) now run inside database transactions
- **Store: prevent disabling last secret** — `cmdDisable` refuses to disable the only active secret
- **Tests: 55 new bot command tests** — comprehensive coverage of all bot commands using in-memory SQLite stores

### Changed
- **Structured logging** — replace `fmt.Printf`/`log.Printf` with scoped logger (`pkg/logger`) across database, bot, services, and stores
- **Database migration system refactored** — split into `loadAppliedVersions`, `extractUpSQL`, `applyMigration`, `execMigrationStatements` for clarity
- **Secret store refactored** — modular methods, consistent error wrapping, proper resource cleanup with `defer`
- **Backup store refactored** — split backup creation into `writeManifest`, `addToArchive`, `extractEntry`; path traversal protection
- **Update service rewritten** — improved error handling, SHA256 and size verification for downloaded assets, separate functions for container recreation
- **Telemt config generation** — regex validation for TOML keys, switch from `fmt.Sprintf` to `fmt.Fprintf`
- **SSH sync** — improved error handling and reporting in `sync.go`
- **Prometheus parser** — refactored for clarity
- **Error handling** — consistent `defer { _ = res.Close() }` pattern, panic recovery in goroutines, suppressed close errors where appropriate
- **Frontend: AuthLayout component** — new shared layout for login/register views
- **Frontend: UI refinements** — DockerView, InstancesView, SecretsView, SystemView, GeoblockView updates

### Fixed
- **Rate limiter** — refactored with improved correctness, added comprehensive tests
- **Resource leaks** — HTTP response bodies, database rows, and file handles now consistently closed on all error paths

## [0.1.10] - 2026-05-15

### Fixed
- **iptables rule deletion**: Fix `-D` flag being dropped when removing geoblock and TCPMSS rules — commands were executed as `iptables POSTROUTING ...` instead of `iptables -D POSTROUTING ...`

## [0.1.9] - 2026-05-15

### Fixed
- **NET_ADMIN capability**: Add `cap_add: NET_ADMIN` to backend container — iptables nf_tables backend requires this capability even with `network_mode: host`
- **Host address resolution**: `DockerHostAddr` now resolves `host.docker.internal` at runtime and falls back to `127.0.0.1` in host network mode where the hostname is unresolvable

## [0.1.8] - 2026-05-15

### Changed
- **Host networking mode**: Backend container now uses `network_mode: host` — required for iptables to operate on the host's network stack. Removed port mapping and Docker DNS from backend
- **Web-backend connectivity**: Web container connects to backend via `host.docker.internal` instead of Docker service DNS. New `BACKEND_URL` env variable (default: `http://host.docker.internal:8090/api/`)
- **iptables/ipset bundled**: Added `iptables` and `ipset` packages to the backend Docker image so geo-blocking and TCPMSS work out of the box in Docker deployments
- **iptables error handling**: All iptables/ipset operations now properly propagate errors with structured logging instead of silently ignoring failures. Geoblock handler returns specific error messages
- **TCPMSS lifecycle**: `applyTCPMSSRules` and `ReconcileInstanceRules` now return errors. Rules cleanup on instance stop logs warnings on failure
- **Container self-detection**: Rewritten `detectHostPath` to try multiple container ID sources (hostname, `/proc/self/cgroup`, `/proc/self/mountinfo`) — fixes host path resolution when running in `network_mode: host`

### Added
- **X-Warning response headers**: Non-critical operation failures (e.g., iptables rule reconciliation on instance update) are surfaced via `X-Warning` HTTP header and shown as toast notifications in the web UI

## [0.1.7] - 2026-05-15

### Added
- **TCPMSS Fragmentation** (per-instance): Fragment ClientHello packets via iptables TCPMSS clamping to defeat DPI reassembly. Configurable MSS value (1-1460, default 88). Requires iptables on the host
- **TLS Fronting Content** (per-instance): Auto-download and serve the TLS domain's website content for active probing defense. Includes manual refresh endpoint (`POST /instances/:id/refresh-fronting`)
- **`passwd` CLI command**: Change admin password from the terminal (interactive prompt or direct argument)
- **Change password in Settings UI**: Change admin password directly from the web interface
- **Anti-blocking section in instance form**: Grouped TCPMSS and TLS fronting controls under a dedicated "Anti-Blocking" card
- **Form card layout**: Reorganized instance, secrets, templates, and upstreams modal forms into labeled sections with visual card grouping

### Changed
- **Memory usage accuracy**: Read `/proc/meminfo` MemAvailable instead of sysinfo Freeram for accurate used memory on Linux
- **Changelog link**: Fix branch reference from `main` to `master` in updates view

## [0.1.6] - 2026-05-14

### Fixed
- Fix updater sidecar failing with `unknown command "sh"` — sidecar now overrides entrypoint to `/bin/sh` instead of inheriting `docker-entrypoint.sh` which wraps commands through `popugate`

## [0.1.5] - 2026-05-14

### Fixed
- Fix panic on nil map assignment in traffic flush (`flushAccumulator.histUsers` was not initialized)

## [0.1.4] - 2026-05-14

### Changed
- Clear hardcoded Swagger host so API docs work behind reverse proxies
- Fix `/setlimit` help text in Telegram bot — `quota_mb` → `quota-MB`
- Stream updater sidecar logs into main container output for better visibility during self-update
- Add pre-pull delay in compose recreate script to ensure HTTP response is delivered before container restart

### Fixed
- Fix navigation badge showing false positives for secrets with `expires_at = '0'`
- Add tooltip and `cursor: help` to navigation badge dots
- Add i18n translations for badge tooltips (en/ru)

## [0.1.3] - 2026-05-14

### Changed
- Decompose monolithic `runServer` into `appStores`/`appServices` structs with dedicated wiring functions
- Centralize version handling via `SetVersion`/`VersionTag`/`VersionURL` with consistent "v" prefix normalization
- Extract traffic flush logic into `flushAccumulator`, `computeUserDeltas`, `nonNegativeDelta` helpers
- Extract upstream health check into `resolveTestResult`, `handleFailover`, `checkUpstream` helpers
- Refactor instance handler into `getInstanceForUpdate`, `applyInstanceUpdates`, `validateAndSaveInstance`
- Extract geoblock service into `collectPorts`, `applyCountryIPSet`, `applyDefaultDeny` helpers
- Cache bot command dispatch map instead of reallocating on every incoming message
- Migrate Docker SDK types from `types.ContainerJSON` to `container.InspectResponse`

### Added
- Telegram inline keyboard buttons in notifications (upstream failover, instance revalidation, update checks)
- `NotifyWithButtonsFunc` callback type and `dashboardButton` helper for notification buttons
- `web_url` setting field for Telegram bot inline keyboard links
- Update availability banner on dashboard with navigation badge dots
- Release notes and changelog links in the updates view

### Fixed
- Escape port bindings in Docker sidecar scripts to prevent potential shell injection
- Fix `/command@botname` being silently ignored due to incorrect stripping order
- Propagate caller context through `dashboardButton` instead of using `context.Background()`
- Log error in upstream failover when database read fails
- Fix `gofmt` alignment in `ContainerService` struct and `SendMessageWithKeyboard` payload

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

[0.2.0]: https://github.com/fussraider/PopuGate/compare/v0.1.10...v0.2.0
[0.1.10]: https://github.com/fussraider/PopuGate/compare/v0.1.9...v0.1.10
[0.1.9]: https://github.com/fussraider/PopuGate/compare/v0.1.8...v0.1.9
[0.1.8]: https://github.com/fussraider/PopuGate/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/fussraider/PopuGate/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/fussraider/PopuGate/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/fussraider/PopuGate/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/fussraider/PopuGate/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/fussraider/PopuGate/compare/v0.1.2...v0.1.3
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
