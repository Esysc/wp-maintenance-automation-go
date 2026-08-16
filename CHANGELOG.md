# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added
- Rehearsal stop now supports an explicit force-cleanup checkbox so the UI can hard-remove leftover staging containers when a normal compose shutdown is not enough.

### Changed
- Rehearsal stop now clears stored rehearsal metadata after a successful cleanup, so inactive rehearsals no longer keep showing stale environment details.
- Rehearsal stop handling now fails closed if cleanup does not actually tear down the containers, instead of reporting a false success.

### Fixed
- Rehearsal stop and active-state polling now stay in sync with the stored job state, preventing the UI from showing an already-stopped rehearsal as still running.
- WP-CLI readiness and inventory probes were hardened to use valid commands and quieter execution flags, reducing noisy warnings during rehearsal startup.

## [0.5.1] - 2026-08-16

### Changed
- Site detect-config now treats `wp_root` values `/` and `.` as auto-detect mode, and prioritizes discovery from the SSH login working directory before broader filesystem fallback.
- Detect-config input handling was tightened: when a `wp_root` is explicitly provided, parsing is performed only for that path (no implicit default-path fallback).
- SSH key input handling was hardened to normalize escaped newlines and improve key material/path interpretation.

### Fixed
- SSH detect-config now returns stage-specific diagnostics (preflight connect/auth, WordPress root detection, and DB config parse) instead of generic failures.
- Detect-config now supports more real-world WordPress layouts: deeper root scan depth, `wp-includes/version.php`-based root detection, and parent-directory fallback for `wp-config.php`.
- Key-auth diagnostics now clearly distinguish invalid key payloads, unreadable key paths, and accidentally pasted public keys in the private-key field.
- Sites UI detect-config flow now surfaces backend error details reliably and uses a multiline SSH private key field to avoid truncation/formatting issues.

## [0.5.0] - 2026-08-16

### Added
- The sandbox site is now viewable in the browser: the app serves the local WordPress site through the API/web proxy at `/sandbox-site`, exposes its public URL on the sandbox status payload, and the Sandbox page offers an "Open in popup" button.
- Live sandbox start logs: startup progress is streamed to the UI over Server-Sent Events (`GET /api/v1/sandbox/start/logs`) and appears as it happens instead of only after the sandbox has finished starting.
- Sandbox site deletion: `DELETE /api/v1/sandbox` stops the sandbox containers, removes the site record with all of its backups from the database, and wipes the sandbox data directory; the Sandbox page exposes this in a danger-zone card.
- Production-like sandbox drill support for validating backup and restore workflows against a real WordPress stack running locally via Docker.
- New sandbox API endpoints under `/api/v1/sandbox` for starting, checking, breaking, stopping, and deleting the local test site (backup/restore use `/api/v1/backup` and `/api/v1/restore`).
- Sandbox page in the web UI to exercise the full end-to-end backup/restore disaster drill without touching a live production server.
- E2E fake-production restore tests that seed a realistic WordPress installation, back it up through the real SSH + restic pipeline, intentionally break it, and verify the restore exactly matches the original state.
- Real restic snapshot validation in the sandbox flow, using actual repository creation, backup snapshots, and restore operations instead of mocked backup metadata.

### Changed
- The sandbox now runs fully containerized: the API server resolves the sandbox web container's network address (and rejoins the sandbox network on demand) instead of relying on host-loopback port publishing.
- Sandbox runtime data (SSH keys, restic repository, state) is persisted in a Docker volume (`sandbox-data/`), so it survives container/image rebuilds.
- The break drill now requires at least one backup before it can be triggered, so the site can always be recovered.
- Sandbox page polish: backup/restore/break/stop buttons are disabled while a job is running, completion notes are shown after backup/restore jobs finish, the backup dropdown auto-refreshes and selects the newest snapshot, and the stop/drill descriptions were clarified.
- Sandbox UI strings are kept in sync across all nine locales.
- README feature overview and core capability list now document the sandbox disaster-drill workflow alongside staging rehearsal and restore flows.
- Added focused tests for sandbox retry/state handling and restic helpers to improve repository coverage.

### Fixed
- WordPress `.htaccess` rewrite rules are now restored on start and re-applied after a stop/start or a restore, so pretty permalinks keep working on the sandbox site.
- Deleting a site now removes its related rows (jobs and backups) transactionally, and the API returns a clear 409 when related data prevents deletion instead of failing with an opaque error.
- Metrics fetch failures no longer break the UI (the metrics panel degrades gracefully) and no longer conflict with site deletion.
- Sandbox state persistence errors are surfaced instead of being silently ignored, and wp-cli seeding is retried to reduce flakiness.
- Database layer now supports both PostgreSQL production DSNs and SQLite test file paths, including the schema migration checks used in unit tests.
- Fake SSH test client and restore job logic were aligned with the real `ssh.Client` interface (`SyncDir` signature and source/destination direction handling), allowing the end-to-end fake-production restore drill to compile and pass reliably.
- Restic backup parsing is more resilient: successful snapshot output is parsed back into a snapshot ID so the backup/restore workflow can continue even when the CLI emits opportunistic output beyond the strict JSON payload.

## [0.4.0] - 2026-08-01

### Changed
- Caddy: HTTPS now also works when accessing the UI via a LAN IP. Added `WP_MAINTENANCE_HOSTS` to list an extra site address (e.g. `192.168.1.116`) and `WP_MAINTENANCE_DEFAULT_SNI` (defaults to that host) so Caddy serves the correct certificate to SNI-less/IP connections, which previously failed with `ERR_SSL_PROTOCOL_ERROR`.
- Restore page: DB/File restore checkboxes now render only after a snapshot is selected, styled to match the UI (indigo gradient check mark).
- Restore page: snapshot select shows a loading spinner while backups are fetched instead of flashing "No backups available".
- System page: removed the redundant status stat cards (already shown on the Dashboard); the page now focuses on host and container metrics.
- System page: shows a loading spinner while metrics are fetched instead of "unavailable" placeholders; host/container metrics are now polled in the background by a shared provider, so data is ready when the page is opened.
- API docs: the OpenAPI spec (`api/docs/openapi.yaml`) now documents all routes, including the previously missing ones (`/api/v1/backup`, `/api/v1/restore`, `/api/v1/upgrade`, `/api/v1/auth/state`, `/api/v1/sites/detect-config`, `/api/v1/staging/cleanup`, rehearsal and job endpoints, and `/api/v1/metrics`).
- API docs: Swagger UI sidebar updated to match the current navigation (Sites and Rehearsal added, removed standalone Health Check page).
- README: API Documentation section trimmed to point at the Swagger UI/spec URL instead of inline curl examples.
- README: cleaned up stale and inaccurate content — replaced the endpoint list with a Swagger pointer, corrected the users/jobs HTTP methods, switched the database section from the old SQLite file to PostgreSQL, fixed broken Markdown tables, and updated the Docker service list and Web Interface features.

### Fixed
- Upgrade page: healthcheck URL field is now pre-filled from the selected site's `healthcheck_url` configuration.
- Swagger UI "Try it out" failing to reach the API: the OpenAPI spec now uses a relative server URL (same origin as the docs page) so requests go through the web proxy instead of hitting the API's self-signed cert / CORS directly.
- Web API proxy now forwards an incoming `Authorization` header (used by Swagger UI) in addition to the session cookie.
- Users page: change password now consistently uses animated toast notifications for client-side validation failures, backend errors, and success states.
- Password change hardening: `/api/v1/auth/change-password` now requires `current_password`, verifies it server-side, and enforces password strength validation on the backend.

### Removed
- Standalone Health Check page (nav item, route, dashboard action card, and associated styles/locale strings). The `/api/v1/healthcheck` endpoint remains available for the CLI.

## [0.3.0] - 2026-07-31

### Added
- Docker-host metrics on the System page: live host CPU/memory/disk plus per-container stats.
- Optional host agent (`cmd/host-agent`) exposing real-host metrics at `/metrics`; API prefers `HOST_AGENT_URL` and falls back to Docker host metrics.
- Frontend migrated to Vite + React SPA.

### Fixed
- Login page reload loop caused by a stale/expired auth token (401 now clears the session and routes to login without a hard page reload).
- Rehearsal page showing a stopped staging environment as "Active" — the active state is now verified against live container state via `GET /api/v1/rehearsal/active`.
- Logout 404 and SPA fallback routing.
- `parseBytes` handling of `docker stats` no-separator sizes (e.g. `2.969MiB`).
- Container CPU usage hidden while idle (0.00% now displayed).

## [0.2.0] - 2026-07-26

### Added
- Ephemeral Docker-based staging rehearsal: spins up a local WordPress + MariaDB/MySQL stack from a restic backup snapshot, runs the full upgrade, healthchecks it, then destroys on success (keeps on failure for debugging).
- `internal/staging` package: Docker Compose lifecycle management (`Create`, `Destroy`, `WPCLI`), HTTPS healthcheck with self-signed cert bypass, version/credential detection from backup artifacts.
- `staging/` support files: dynamic `Dockerfile.wp` (detected WP version + PHP version), `docker-compose.template.yml`, `wp-entrypoint.sh`, `wp-config-gen.sh`, `apache-default-ssl.conf`.
- `POST /api/v1/staging/cleanup` endpoint for manually destroying kept staging environments.
- Docker socket mount (`/var/run/docker.sock`) in API container for managing sibling staging containers.
- Docker CLI + Docker Compose plugin installed in API Docker image.
- Staging rehearsal integrated into upgrade flow: `PhaseRehearse` runs before production upgrade when `staging_enabled` is set on the site.
- Database migration to drop legacy `staging_host`, `staging_port`, `staging_user`, `staging_root` columns.
- Multi-language UI: 9 languages (EN, FR, IT, ES, PT, ZH, JA, KO, RU).
- Auto-detect site configuration via SSH (DB credentials from `wp-config.php`).
- Restic repository format documentation in README.

### Changed
- Removed `StagingHost`, `StagingPort`, `StagingUser`, `StagingRoot` fields from `Site` model — staging is now fully automated via Docker, no manual server configuration needed.
- Removed staging host form fields from site creation/edit modal.
- Removed staging host translation keys from all locale files.
- Updated upgrade handler to accept `snapshot_id` for staging rehearsal.
- Updated OpenAPI spec to remove staging host fields from Site schema.

### Fixed
- Legacy site test SQL inserts now include `wp_ssh_key` column to prevent NULL scan errors.

## [0.1.0] - 2026-07-26

### Added
- Caddy reverse proxy with automatic HTTPS (self-signed for localhost, Let's Encrypt for production domains).
- Docker Compose configuration: API (8081), Web (8080), Caddy (80/443), optional MySQL/WordPress test stack, optional restic backup storage.
- Environment variable cleanup and documentation.
- Pre-commit hook for Go formatting and build checks.
- Restic repository format documentation with backend examples (S3, SFTP, B2, Azure, GCS, rclone).

### Changed
- Simplified `.env.example` to minimal required variables.
- Simplified `Caddyfile` to use environment variable for domain.
- Updated Docker Compose service definitions.

## [0.0.1] - 2026-07-26

### Added
- Initial Go implementation of WordPress maintenance automation.
- Secure backup workflow with SSH + rsync + restic.
- Guided restore workflow with database import and file synchronization.
- Upgrade orchestration with backup, WordPress core/plugin/theme/language upgrade, database update, healthcheck, and reporting.
- RESTful API with authentication (JWT tokens, bcrypt passwords), site management, backup/restore endpoints, healthcheck endpoints, and snapshot listing.
- Web interface with dashboard, site management, backup/restore UI, upgrade UI, and system status panel.
- CLI client for scripting and automation.
- SSH client with SFTP, rsync, command execution, WordPress root detection, and DB config parsing.
- Restic client wrapper supporting backup, restore, snapshots, forget, check, unlock, init, stats, find, and tag operations.
- Healthcheck system with configurable retries, delay, timeout, and expected status code.
- Encrypted storage for sensitive site fields (DB password, restic password file, SSH key) using AES-GCM.
- Auto-migration of legacy plaintext secrets to encrypted form.
- Multi-site support with per-site restic repository, SSH credentials, and backup configuration.
- Backup retention policies via restic forget flags.
- Backup manifest system with file count, checksum, and metadata.
