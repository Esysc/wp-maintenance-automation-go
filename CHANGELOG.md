# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Changed
- Restore page: DB/File restore checkboxes now render only after a snapshot is selected, styled to match the UI (indigo gradient check mark).
- Restore page: snapshot select shows a loading spinner while backups are fetched instead of flashing "No backups available".

### Fixed
- Upgrade page: healthcheck URL field is now pre-filled from the selected site's `healthcheck_url` configuration.

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
