# WP Maintenance Automation Go

A modern, secure WordPress site maintenance toolkit using Go. This project provides automated backups, upgrades, and restoration capabilities with comprehensive health checks and rollback functionality.

## Overview

WP Maintenance Automation Go is a Go-based implementation of WordPress maintenance tasks, featuring:

- **Secure Backups**: Encrypted, deduplicated backups using restic integration
- **Automated Upgrades**: WordPress core, plugins, themes, and database upgrades with health checks
- **Automatic Rollback**: Automatic rollback on upgrade failure
- **Staging Rehearsal**: Ephemeral Docker-based staging — spins up a local WordPress + MariaDB stack from a backup snapshot, runs the full upgrade, healthchecks it, then destroys the environment on success (keeps it on failure for debugging)
- **RESTful API**: API endpoints for backup, upgrade, restore, staging, jobs, and metrics
- **Web Interface**: Modern web UI for monitoring and managing WordPress sites
- **Docker Host Metrics**: Live host CPU/memory/disk and per-container stats on the System page, via Docker host info or an optional host agent
- **CLI Client**: Command-line interface for scripting and automation
- **Multi-language UI**: 9 languages (EN, FR, IT, ES, PT, ZH, JA, KO, RU)

## Features

### Core Functionality

- SSH-based remote operations
- Database dump and file synchronization
- Encrypted backup storage with retention policies
- Automated WordPress version upgrades
- Comprehensive health checks
- One-click rollback capabilities

### API Endpoints

- `POST /api/v1/auth/login` - Authenticate and get a token
- `GET /api/v1/auth/state` - Check authentication state
- `POST /api/v1/auth/change-password` - Change password (auth required)
- `GET /api/v1/health` - Health check
- `GET /api/v1/status` - System status overview
- `GET /api/v1/backups` - List backups (filter with `?site_id=`)
- `GET/DELETE /api/v1/backups/:id` - Backup details / delete
- `POST /api/v1/backup` - Create a backup for a site
- `POST /api/v1/restore` - Restore a snapshot to a site
- `POST /api/v1/upgrade` - Perform WordPress upgrade (with optional staging rehearsal)
- `GET /api/v1/snapshots` - List available restic snapshots
- `POST /api/v1/healthcheck` - Run health checks
- `GET/POST /api/v1/sites` - List / create sites
- `GET/PUT/DELETE /api/v1/sites/:id` - Site details / update / delete
- `POST /api/v1/sites/detect-config` - Auto-detect site config via SSH
- `GET/POST /api/v1/users` - List / create users
- `GET/DELETE /api/v1/users/:id` - User details / delete
- `GET/POST /api/v1/tokens` - List / create API tokens
- `DELETE /api/v1/tokens/:id` - Revoke token
- `POST /api/v1/staging/cleanup` - Destroy a kept staging environment
- `POST /api/v1/rehearsal` - Start a staging rehearsal for a site
- `GET /api/v1/rehearsal/active` - Check whether a rehearsal environment is actually running
- `GET /api/v1/rehearsal/:id` - Rehearsal job details
- `POST /api/v1/rehearsal/:id/stop` - Stop a rehearsal and destroy its staging environment
- `POST /api/v1/rehearsal/:id/wpcli` - Run a WP-CLI command in a rehearsal environment
- `GET/POST /api/v1/jobs` - List / get background jobs (filter by `site_id`, `type`, `all`)
- `GET/DELETE /api/v1/jobs/:id` - Job details / cancel
- `GET /api/v1/metrics` - Host + container metrics

### Web Interface

- Dashboard for monitoring backup status
- Real-time status updates
- System status panel (`/system`) with status + health summaries and raw API details
- Activity logs and reports

## Getting Started

### Prerequisites

- Go 1.25 or higher
- Docker and Docker Compose (required for deployment and staging rehearsal)
- SSH access to WordPress servers
- restic (for backup storage)
- MySQL/MariaDB database access
- Docker socket access (`/var/run/docker.sock`) for ephemeral staging environments

### Quick Start with Docker Compose

```bash
docker compose up -d
```

This starts four services:

- **Caddy** (port 80/443) - Reverse proxy with automatic HTTPS
- **API** (port 8081) - REST API server
- **Web** (port 8080) - Web UI server
- **DB** (127.0.0.1:5432) - PostgreSQL storage

Access the web UI at `https://localhost` (or `http://localhost` which redirects to HTTPS).

Optional services (MySQL/WordPress test stack, restic REST storage) are behind compose profiles:

```bash
docker compose --profile test up -d   # MySQL + WordPress for local testing
docker compose --profile backup up -d # restic REST server
```

### Local Development

1. Clone the repository:

```bash
git clone https://github.com/yourusername/wp-maintenance-automation-go.git
cd wp-maintenance-automation-go
```

2. Install dependencies:

```bash
go mod download
```

3. Build the application:

```bash
make build
```

Or build individually:

```bash
make build-api         # builds bin/wp-maintenance-api
make build-web         # builds bin/wp-maintenance-web
make build-cli         # builds bin/wp-maintenance
make build-server      # builds bin/wp-maintenance-server
make build-host-agent  # builds bin/host-agent
```

4. Configure environment:

```bash
cp .env.example .env
# Edit .env with your settings
```

5. Run the servers:

```bash
make dev
```

Or run individually:

API server (default port 8081):

```bash
./bin/wp-maintenance-api
```

Web UI (default port 8080, proxies to API):

```bash
./bin/wp-maintenance-web
```

CLI client:

```bash
./bin/wp-maintenance help
```

### Development Commands

```bash
make dev        # Start combined API + Web server
make test       # Run tests
make lint       # Run linter
make clean      # Clean build artifacts
```

## Configuration

### Environment Variables

Key environment variables (see `.env.example`):

| Variable | Default | Description |

|----------|---------|-------------|
| `API_PORT` | `8081` | API server port |
| `PORT` | `8081` | Alias for `API_PORT` |
| `WEB_PORT` | `8080` | Web UI server port |
| `API_URL` | `http://localhost:8081` | API URL for web proxy |
| `DB_HOST` | - | PostgreSQL host (compose sets it to `db`) |
| `DB_PORT` | - | PostgreSQL port (compose sets it to `5432`) |
| `DB_USER` | `wpmaint` | PostgreSQL user |
| `DB_PASSWORD` | `wpmaint` | PostgreSQL password |
| `DB_NAME` | `wpmaintenance` | PostgreSQL database name |
| `DATA_DIR` | `./data` | Data directory (DB, logs) |
| `STATIC_DIR` | `./web/static` | Web static assets directory |
| `LOG_LEVEL` | `info` | Log level |
| `DEBUG` | `false` | Enable debug mode |
| `SECRET_KEY` | `default-secret-key` | JWT signing / encryption key |
| `DATA_ENCRYPTION_KEY` | falls back to `SECRET_KEY` | AES-GCM encryption key |
| `RESTIC_REPOSITORY` | - | Global restic repository (optional). Format: `s3:https://…`, `b2:…`, `gs:…`, `azure:…`, `sftp:…`, `rest:…`, `/local/path` or `local:/path`. See [restic docs](https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html). |
| `RESTIC_PASSWORD_FILE` | - | Global restic password file (optional) |
| `HOST_AGENT_URL` | - | URL of the host agent for real-host metrics (e.g. `http://127.0.0.1:9100`). Empty falls back to Docker host metrics |
| `COMPOSE_PROJECT_NAME` | - | Docker Compose project name, used to identify the app's own containers in host metrics |
| `TLS_DISABLE` | `false` | Disable API TLS |
| `WEB_TLS_DISABLE` | `false` | Disable Web TLS |
| `WP_MAINTENANCE_TOKEN` | - | Pre-shared token for web->API auth |
| `WP_MAINTENANCE_DOMAIN` | `localhost` | Domain for Caddy / Let's Encrypt |

### Restic Repository Formats

The `RESTIC_REPOSITORY` value (global or per-site) uses a URI scheme to select the backend:

| Backend | Example |

|---|---|
| Local | `local:/data/backups` or `/data/backups` |
| S3 / S3-compatible | `s3:https://s3.amazonaws.com/my-bucket` |
| S3 (MinIO) | `s3:https://minio.example.com/my-bucket` |
| SFTP | `sftp:user@server:/backups` |
| BackBlaze B2 | `b2:my-bucket:/backups` |
| Azure Blob | `azure:container:/path` |
| Google Cloud Storage | `gs:bucket:/path` |
| OpenStack Swift | `swift:container:/path` |
| REST server | `rest:https://server:8000/` |
| rclone | `rclone:crypt:remote:path` |

See the [restic documentation](https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html) for details.

### Per-Site Configuration (Database)

Each WordPress site is configured through the Web UI and stored in the database (not env vars):

- SSH host, user, port, key
- WordPress installation path
- Database credentials
- Restic repository and password
- Backup retention policy

### Caddy / TLS Configuration

- **No domain set** (`WP_MAINTENANCE_DOMAIN` empty): Caddy uses self-signed certificates via `tls internal` — suitable for local development.
- **Domain set** (`WP_MAINTENANCE_DOMAIN=example.com`): Caddy automatically provisions Let's Encrypt certificates for your domain.
- HTTP on port 80 redirects to HTTPS on port 443 automatically.

## Staging Rehearsal

When `staging_enabled` is set to `Yes` on a site, the upgrade flow includes an ephemeral Docker-based staging rehearsal before the production upgrade:

1. **Restore** — the latest restic snapshot is extracted to a temp directory
2. **Detect versions** — WP version, DB version (MariaDB/MySQL), and credentials are read from the backup artifacts
3. **Spin up containers** — a WordPress + MariaDB/MySQL stack is created via Docker Compose with matching versions, self-signed SSL, and WP-CLI
4. **Seed the environment** — DB dump is imported, WordPress files are copied, `wp-config.php` is patched, and `siteurl`/`home` are updated to `https://localhost:<mapped_port>`
5. **Run upgrade** — the full WP-CLI upgrade sequence runs against the staging container
6. **Healthcheck** — HTTPS healthcheck with retries against the staging URL
7. **Cleanup** — on success, containers are destroyed; on failure, containers are kept for manual inspection

Docker socket access (`/var/run/docker.sock`) must be mounted into the API container for staging to work. This is configured in `docker-compose.yml` by default.

## Host Metrics

The System page (`/system`) shows real host metrics (CPU, memory, disk) and per-container stats. The API gathers host info from two sources:

1. **Docker host** (default) — host CPU/memory/disk are read from Docker Desktop/Engine info.
2. **Host agent** — a small daemon that reports the actual machine's metrics, useful when the Docker host is a VM (e.g. Docker Desktop on macOS) or when the API container is remote.

Build and run the agent on the host machine:

```bash
make build-host-agent        # builds bin/host-agent
./bin/host-agent             # serves /metrics on 127.0.0.1:9100 by default
```

Then point the API at it via `HOST_AGENT_URL`:

```bash
HOST_AGENT_URL=http://127.0.0.1:9100 docker compose up -d --build api
```

When `HOST_AGENT_URL` is unset, the API falls back to Docker host info automatically.

## Authentication and Login Modes

- Login supports two modes:
  - First setup / forced password mode: requires `password` and `passwordConfirm`
  - Normal login mode: requires only `password`
- Auth state endpoint:
  - `GET /api/v1/auth/state` (API)
  - `GET /api/auth/state` (web proxy)

### Password Reset (CLI Recovery)

Reset internal admin password directly in local DB (no API token required):

```bash
./bin/wp-maintenance reset-password
```

Or from source:

```bash
go run ./cmd/cli reset-password
```

### Database Location and Git Ignore

- Runtime DB file: `data/wp-maintenance.db` (host filesystem)
- In Docker Compose, host `./data` is mounted into `/app/data`
- `data/` is ignored by git in `.gitignore`

### Encryption at Rest

- User passwords are stored as bcrypt hashes in `users.password_hash`
- Sensitive site fields are encrypted at rest using AES-GCM with prefix `enc:v1:`:
  - `sites.db_password`
  - `sites.restic_password_file`
- Key source order:
  - `DATA_ENCRYPTION_KEY`
  - fallback `SECRET_KEY`
  - fallback `default-secret-key`
- Legacy plaintext site secrets are auto-migrated to encrypted form on DB open

## API Documentation

The web server serves an interactive OpenAPI/Swagger UI and the machine-readable spec at `/api/docs` (source: `api/docs/openapi.yaml`). The spec can drive client code generation for CI integrations.

All API responses use a uniform envelope: `{"success": true, "data": <payload>}` on success and `{"success": false, "error": "<message>"}` on failure.

## CI/CD Automation

The API is designed for unattended use in pipelines. All endpoints (except `/api/v1/health`, `/api/v1/auth/state`, and `/api/v1/auth/login`) authenticate with a bearer token, so no interactive login is required.

### 1. Create a long-lived API token

Instead of the 24-hour web-session token, create an API token once (via the Web UI *Tokens* page or the CLI) and store it as a CI secret:

```bash
# create token for the admin user (persisted until revoked)
./bin/wp-maintenance token create <user_id> ci-backups 8760
```

Or via the API (`duration` is in hours; the token is created for the admin user):

```bash
curl -sX POST https://localhost/api/v1/tokens \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"ci-backups","duration":8760}' | jq -r '.data.token'
```

Store the returned token in your CI secret store.

### 2. Authenticate

CI clients can authenticate with the token in the `Authorization` header (as in the examples above) or in the query string for clients that cannot set headers:

```bash
curl "https://localhost/api/v1/status?token=$CI_TOKEN"
```

### 3. Drive jobs

Backup, restore, upgrade, and rehearsal endpoints are asynchronous: they enqueue a background job and return a `job_id`. Poll `GET /api/v1/jobs/:job_id` and check the `status` field (`queued`, `running`, `completed`, `failed`, `cancelled`).

Example GitHub Actions workflow running a nightly backup:

```yaml
name: nightly-backup
on:
  schedule:
    - cron: '0 2 * * *'
jobs:
  backup:
    runs-on: ubuntu-latest
    steps:
      - name: Queue backup
        id: backup
        run: |
          RESPONSE="$(curl -fsS -X POST "https://${{ vars.APP_HOST }}/api/v1/backup" \
            -H "Authorization: Bearer ${{ secrets.WP_MAINTENANCE_TOKEN }}" \
            -H "Content-Type: application/json" \
            -d "{\"site_id\":\"${{ vars.WP_SITE_ID }}\"}")"
          JOB_ID="$(echo "$RESPONSE" | jq -r '.data.job_id')"
          echo "job_id=$JOB_ID" >> "$GITHUB_OUTPUT"
      - name: Poll job until completion
        env:
          JOB_ID: ${{ steps.backup.outputs.job_id }}
        run: |
          for i in $(seq 1 60); do
            STATUS="$(curl -fsS -H "Authorization: Bearer ${{ secrets.WP_MAINTENANCE_TOKEN }}" \
              "https://${{ vars.APP_HOST }}/api/v1/jobs/$JOB_ID" | jq -r '.data.status')"
            [ "$STATUS" = "completed" ] && echo "backup OK" && exit 0
            [ "$STATUS" = "failed" ] && echo "backup failed" && exit 1
            sleep 10
          done
          echo "timeout waiting for job" && exit 1
```

### 4. CLI for scripting

The CLI (`bin/wp-maintenance`) wraps the same API and is well suited for scripts and local automations. Point it at the API with `API_URL` and authenticate with `WP_MAINTENANCE_TOKEN`:

```bash
export API_URL=https://localhost:8081
export WP_MAINTENANCE_TOKEN="$CI_TOKEN"
./bin/wp-maintenance backup -s <site-id>
./bin/wp-maintenance snapshots
./bin/wp-maintenance jobs --site-id <site-id>
```

See `./bin/wp-maintenance help` for the full command list.

## Testing

```bash
make test        # Run all tests
make test-cover  # Run tests with coverage
```

Run specific test suite:

```bash
go test ./internal/backup/...
go test ./internal/upgrade/...
```

Run visual Playwright test with explicit login password:

```bash
WP_MAINTENANCE_TEST_PASSWORD='<your-password>' npx playwright test visual-test.spec.js --workers=1
```

## Docker

Build and run with Docker Compose:

```bash
docker compose up -d
```

This starts:

1. **Caddy** on ports 80/443 — reverse proxy with TLS
2. **API server** on port 8081
3. **Web UI** on port 8080

For local development without a real domain, Caddy uses self-signed certificates. Browsers will show a security warning — proceed anyway or use `curl -k`.

For production, set `WP_MAINTENANCE_DOMAIN=yourdomain.com` and Caddy will automatically provision Let's Encrypt certificates.

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

This project is licensed under the MIT License - see [LICENSE](LICENSE) file for details.
