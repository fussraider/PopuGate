# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.5.0] - 2026-06-13

### Added
- **Upstream Bulk Actions**: Added bulk operations (Enable, Disable, Health Check, Delete) to the upstreams list table in the Web UI, with real-time reactive spinner states.
- **Immediate Container Hot-Reloads on Configuration Changes**:
  - Upstream modifications (adding, updating, removing, and toggling) now trigger immediate hot-reload (`SIGHUP`) of active proxy containers.
  - Secret modifications (creation, rotation, limits updates, renaming, expiration extensions, archiving/unarchiving, bulk settings, and imports) now trigger immediate hot-reload of active proxy containers.
  - Global configuration updates (like changing masking hosts, ports, domains, concurrency, etc.) now trigger immediate hot-reload of active proxy containers.
  - Background health check failover and auto-recovery events for upstreams now trigger immediate hot-reload of active proxy containers.
- **Comprehensive Info-Level Logging**:
  - Added detailed, contextual Info-level logging for all key system actions (managing secrets, upstreams, geoblocking rules, and global configurations).
  - Proxy container reload operations now log the explicit trigger reason (e.g., `"secret <name> created"`, `"upstream <name> auto-disabled"`, etc.) alongside status messages when config is regenerated and SIGHUP signals are sent.

### Optimized
- **Database Read Caching during Reload**: Refactored the `Reload` routine in `ContainerService` to query the SQLite database for upstreams and secrets exactly once and reuse them across all active proxy instances, eliminating redundant SQLite reads.

### Fixed
- **Silent Docker Connection Errors**: Resolved silent error dropping when checking proxy container status inside `ContainerService.Reload`. Checked container states are now logged as warnings on failure.
- **Traffic Not Collected During Zero-Downtime Restart**: Fixed a bug where `flushInstance` always used the static `MetricsPort` from the database instead of resolving the currently active swing container's metrics port. During a zero-downtime swing restart the active container runs on an alternative port (`primaryPort + 10000 + N`) with its metrics endpoint on `swingPort + 100`; the flush routine was hitting a dead endpoint, collecting nothing, and writing zero traffic to the database — causing an empty graph for the entire duration of the swing. The fix mirrors the existing `fetchLiveMetrics` logic: `Flush` now queries Docker for running containers and passes them to `flushInstance`, which calls `resolveSwingMetricsPort` before building the metrics URL.
- **WebSocket Graceful Disconnect on Page Unload**: Added a `beforeunload` window event listener to `useWebSocket` to gracefully close active connections before page destruction, resolving console warnings about interrupted connections (`ws://... was interrupted during page load`) in Firefox/Chrome.
- **WebSocket Double-Connection Race Conditions**: Implemented `useSharedWebSocket` with subscriber ref-counting and integrated it across all Pinia stores (`proxy`, `system`, `traffic`, `docker`), resolving duplicate connection attempts, page navigation conflicts, and console error spam.
- **Missing Scheduler Task Localization**: Added English (`en.json`) and Russian (`ru.json`) translations for the `fronting-update` (domain fronting content update) task, resolving `[intlify] Not found` console warnings.
- **Silent De-duplication in Bulk Upstream Add**: Bulk-generated upstream names now incorporate an identity hash over the full upstream identity (type, address, credentials, interface), so proxies sharing the same `host:port` but differing in credentials no longer collide and get silently dropped by the store's `INSERT OR IGNORE`. The `POST /upstreams/bulk` response now reports a `skipped` count alongside `count`, initial health checks run only for the upstreams actually inserted, and the Web UI surfaces skipped duplicates in the success toast.
- **IPv6 Proxy Parsing**: `ParseProxyLine` now correctly parses bracketed IPv6 addresses (e.g. `[::1]:1080`, `[2001:db8::1]:1080:user:pass`, `socks5://user:pass@[::1]:1080`); the colon-separated suffix form remains IPv4/hostname-only by design.
- **Robust Bulk-Check SSE Parsing**: The Web UI now parses the `/upstreams/bulk-check` event stream by complete event blocks (split on blank lines) instead of tracking `event:`/`data:` state across raw network chunks, eliminating dropped results when an event is split across a chunk boundary, and handles the terminating `complete` event explicitly.

## [0.4.0] - 2026-06-05

### Added
- **Automatic Upstream Failover & Recovery**: Added automatic health-based failover (disables upstreams after 3 consecutive failures) and auto-recovery (re-enables when checks pass) with Telegram notifications and status indicators in the Upstreams view and Dashboard's System Health card.
- **Advanced Audit Log Filtering**: Added comprehensive filtering by period (today, yesterday, last week, last month, custom date range), action names, and usernames in the Web UI. Added the `/api/v1/audit/filters` endpoint to query unique users and actions from the logs.
- **Zero-Downtime Swing Routing (Swing Routing)**: Implemented telemt container updates/reloads on alternative swing ports with atomic NAT redirection via iptables to prevent dropping live proxy connections.
- **TCP Network Tuning (BBR & TFO)**: Added one-click activation of TCP BBR congestion control and TCP FastOpen kernel optimizations (sysctl) with automatic backup and rollback.
- **Scheduler Auto-Updates**: Implemented background auto-update tasks to automatically check, download, and apply PopuGate (backend/app) updates with failure notifications via Telegram.
- **Host Docker Engine Updates**: Added manual and scheduled updates for the host Docker Engine and status check notifications.

### Changed
- **Vite 8.0 & Rolldown Migration**: Upgraded frontend build system to use Vite 8.0 and Rolldown, migrating `manualChunks` in `vite.config.ts` to a functional format to comply with Rolldown.
- **UI Consolidation (System View)**: Merged the separate Docker and Updates views into a single unified `SystemView` to streamline server administration.
- **Dependency Upgrades**: Upgraded Go backend packages (`go 1.26.1`, `modernc.org/sqlite v1.51.0`, `golang.org/x/crypto v0.52.0`) and frontend libraries (`vue-router 5.1.0`, `pinia 3.0.4`, `typescript 6.0.3`, `@lucide/vue 1.17.0`, `axios 1.17.0`).
- **Modal Scrolling & Layout**: Redesigned Modal window header/footer to be sticky while the body scrolls using flexbox layouts.
- **Table Loading Behavior**: Optimized `DataTable` skeleton loader condition to avoid flickering when caching empty state lists.

### Fixed
- **WebSocket Reconnection Races**: Resolved race conditions and connection leaks caused by concurrent WebSocket stream creations in Pinia stores.
- **Geoblock Environment Resilience**: Aligned API payloads, handled missing iptables/ipset command dependencies gracefully by disabling UI switches, and supported database transaction rollback.
- **Go Linter Warnings**: Fixed backend linter warnings (De Morgan's law simplification and ignoring close errors).

## [0.3.2] - 2026-05-31

### Fixed
- **Background Active Connections Logging**: Resolved a critical bug in background traffic flushing where active connections history was recorded as `0` instead of the actual count. Since the `telemt` Rust engine does not export a global `telemt_connections_current` metric, added the fallback logic to automatically calculate it as the sum of all active user connections, mirroring the successful real-time dashboard behavior.

## [0.3.1] - 2026-05-30

### Added
- **Bot Handler Unit Tests**: Added comprehensive unit test coverage (`bot_handler_test.go`) for testing Telegram bot status checks and toggle operations under simulated settings.

### Changed
- **Globally Shared Active Bot**: Refactored `BotHandler` to accept and use globally managed pointers (`activeBot **bot.Bot` and `botMu *sync.Mutex`) passed via `RouterConfig` rather than localized handler state, ensuring thread-safe, unified bot lifecycle controls.

## [0.3.0] - 2026-05-30

### Added
- **Active Connections History & Sparklines**: Added active connections tracking in SQLite `traffic_history` store, along with an interactive connections sparkline chart on the Dashboard and a full-size historical chart on the Traffic page.
- **Interactive User Traffic Donut Chart**: Added circular traffic share visualization for users, including bi-directional hover highlighting that links the donut segments directly to the data table rows.
- **Dashboard Quick Status Monitors (Services Row)**: Integrated quick status cards for Telegram Bot (running/disabled status), Backups (time elapsed since latest backup), and Geo-block config (blacklist/whitelist mode and blocked country count) linking to their respective pages.
- **Recent Activity Feed**: Created a dashboard log feed presenting the last 5 audit entries with category-based action color-coding (success/warning/danger) and relative "time ago" stamps.
- **Robust Relative Timestamps**: Added a client-side formatting helper (`timeAgo`) for user-friendly relative age representations (seconds, minutes, hours, days).

### Fixed
- **DataTable Infinite Skeleton Loader**: Resolved a major UX bug where tables mounted with a cached empty list of items (e.g., fresh database config) permanently hung on the skeleton loader instead of transitioning to the empty state.
- **Dashboard TypeScript Build Error**: Resolved a Vue compilation failure caused by calling `geoblockStore.load()` synchronously with no arguments under `Promise.all` before config settings were loaded.
- **Zero-Traffic Connection Recording**: Updated backend `traffic_service` and `InsertHistoryBatch` database logic to persist historical snapshots whenever there are active connections, preventing graph gaps during low-activity intervals.
- **Flaky API Handler Tests**: Mocked host-level socket checks (`isPortFree`) during next-metrics-port calculation in handler tests to make tests deterministic and independent of local developer network states.
- **Vite Update & Nginx Cache Policies**: Redefined caching rules in `nginx.conf` (1 year for hashed Vite assets, `no-cache` for `index.html`) and forced cache-busting timestamp queries upon auto-reloads to prevent clients from executing stale frontend builds.

## [0.2.2] - 2026-05-24

### Added
- **Anti-phishing host validation**: nginx `default_server` blocks reject requests with unknown `Host` headers when `DOMAIN_NAME` is configured — foreign domains pointed at the server receive no response (HTTP: `return 444`, HTTPS: `ssl_reject_handshake on`)
- **Backend `HostMiddleware`**: defense-in-depth host header check at the application level, activated when `web_url` setting is configured; always allows `localhost`/`127.0.0.1`/`::1`

### Changed
- **Anti-phishing docs**: README.md and README_RU.md document host validation behavior in the HTTPS section

## [0.2.1] - 2026-05-23

### Added
- **Version override for update testing**: `POPUGATE_OVERRIDE_VERSION` env and `--override-version` CLI flag allow overriding the application version at runtime to test self-update flows without rebuilding
- **`POPUGATE_CONTAINER_NAME` env**: Explicitly set the Docker container name for self-update — solves incorrect hostname detection when using `network_mode: host` (e.g., OrbStack, Docker Desktop)
- **`POPUGATE_DEPLOYMENT=binary`**: Force binary update mode even inside a Docker container for testing binary self-update
- **`make build-test-version`**: Build binary with a custom version via `TEST_VERSION=x.y.z`
- **`make docker-build-test`**: Build Docker image with a custom version for update testing

### Fixed
- **Sidecar crash on self-update**: Entrypoint `/bin/sh` combined with Cmd `["sh","-c",...]` produced `/bin/sh sh -c "..."` which failed — changed Cmd to `["-c",...]`
- **Container not found during Docker restart**: `selfContainerName()` returned host hostname instead of container name in `network_mode: host` — added `POPUGATE_CONTAINER_NAME` env with highest priority
- **Web dist extraction failure**: `extractWebDist` rejected tar archives containing `./` directory entries due to incorrect path traversal check — added equality check for `cleanTarget == cleanDir`

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

[0.5.0]: https://github.com/fussraider/PopuGate/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/fussraider/PopuGate/compare/v0.3.2...v0.4.0
[0.3.2]: https://github.com/fussraider/PopuGate/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/fussraider/PopuGate/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/fussraider/PopuGate/compare/v0.2.2...v0.3.0
[0.2.2]: https://github.com/fussraider/PopuGate/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/fussraider/PopuGate/compare/v0.2.0...v0.2.1
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
