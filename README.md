# WP Maintenance Automation Go

A modern, secure WordPress site maintenance toolkit using Go. This project provides automated backups, upgrades, and restoration capabilities with comprehensive health checks and rollback functionality.

## Overview

WP Maintenance Automation Go is a Go-based implementation of WordPress maintenance tasks, featuring:

- **Secure Backups**: Encrypted, deduplicated backups using restic integration
- **Automated Upgrades**: WordPress core, plugins, themes, and database upgrades with health checks
- **Automatic Rollback**: Automatic rollback on upgrade failure
- **Staging Rehearsal**: Safe testing environment for upgrades before production
- **RESTful API**: API endpoints for backup, upgrade, and restore operations
- **Web Interface**: Modern web UI for monitoring and managing WordPress sites
- **CLI Client**: Command-line interface for scripting and automation

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
- `GET/POST /api/v1/backups` - List / create backups
- `GET/DELETE /api/v1/backups/:id` - Backup details / delete
- `POST /api/v1/upgrade` - Perform WordPress upgrade
- `GET /api/v1/snapshots` - List available snapshots
- `POST /api/v1/healthcheck` - Run health checks
- `GET/POST /api/v1/sites` - List / create sites
- `GET/PUT/DELETE /api/v1/sites/:id` - Site details / update / delete
- `GET/POST /api/v1/users` - List / create users
- `GET/DELETE /api/v1/users/:id` - User details / delete
- `GET/POST /api/v1/tokens` - List / create API tokens
- `DELETE /api/v1/tokens/:id` - Revoke token

### Web Interface
- Dashboard for monitoring backup status
- Real-time status updates
- System status panel (`/system`) with status + health summaries and raw API details
- Activity logs and reports

## Getting Started

### Prerequisites
- Go 1.25 or higher
- Docker and Docker Compose (recommended for deployment)
- SSH access to WordPress servers
- restic (for backup storage)
- MySQL/MariaDB database access

### Quick Start with Docker Compose

```bash
docker compose up -d
```

This starts three services:
- **Caddy** (port 80/443) - Reverse proxy with automatic HTTPS
- **API** (port 8081) - REST API server
- **Web** (port 8080) - Web UI server

Access the web UI at `https://localhost` (or `http://localhost` which redirects to HTTPS).

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
make build-api     # builds bin/wp-maintenance-api
make build-web     # builds bin/wp-maintenance-web
make build-cli     # builds bin/wp-maintenance
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
| `WEB_PORT` | `8080` | Web UI server port |
| `API_URL` | `http://localhost:8081` | API URL for web proxy |
| `DATA_DIR` | `./data` | Data directory (DB, logs) |
| `LOG_LEVEL` | `info` | Log level |
| `DEBUG` | `false` | Enable debug mode |
| `SECRET_KEY` | `default-secret-key` | JWT signing / encryption key |
| `DATA_ENCRYPTION_KEY` | falls back to `SECRET_KEY` | AES-GCM encryption key |
| `RESTIC_REPOSITORY` | - | Global restic repository (optional) |
| `RESTIC_PASSWORD_FILE` | - | Global restic password file (optional) |
| `TLS_DISABLE` | `false` | Disable API TLS |
| `WEB_TLS_DISABLE` | `false` | Disable Web TLS |
| `WP_MAINTENANCE_TOKEN` | - | Pre-shared token for web->API auth |
| `WP_MAINTENANCE_DOMAIN` | `localhost` | Domain for Caddy / Let's Encrypt |

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

Full API documentation available at `/api/docs` (when running with Swagger integration).

### Example Usage

Login and get a token:
```bash
curl -X POST https://localhost/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"password":"your-password"}'
```

Use the token for authenticated requests:
```bash
TOKEN="<your-token>"
curl -H "Authorization: Bearer $TOKEN" https://localhost/api/v1/status
```

Create a backup for a site:
```bash
curl -X POST https://localhost/api/v1/backups \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"site_id":1}'
```

## Project Structure

```
wp-maintenance-automation-go/
├── cmd/
│   ├── api/main.go         # API server entry point
│   ├── cli/main.go         # CLI client entry point
│   ├── server/main.go      # Combined API + Web server
│   └── web/main.go         # Web UI server entry point
├── api/docs/               # API documentation (Swagger)
├── internal/
│   ├── auth/               # Authentication (users, tokens, bcrypt)
│   ├── backup/             # Backup creation & management
│   ├── config/             # Configuration management
│   ├── healthcheck/        # HTTP health checking
│   ├── restic/             # Restic client wrapper
│   ├── restore/            # Restore operations
│   ├── ssh/                # SSH/rsync operations
│   └── upgrade/            # WordPress upgrade orchestrator
├── pkg/                    # Public/reusable packages
│   ├── api/               # Generic API handler
│   ├── models/            # Shared data models
│   └── utils/             # Utility functions
├── web/
│   ├── static/             # Static assets (CSS)
│   └── templates/          # HTML templates
├── tests/                  # Integration tests
├── Makefile                # Build and development utilities
└── docker-compose.yml      # Docker Compose configuration
```

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

## Support

For issues and questions:
- Open an issue on GitHub
- Check existing documentation
- Contact maintainers
