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
- `POST /api/v1/backup` - Create a new backup
- `POST /api/v1/upgrade` - Perform WordPress upgrade
- `POST /api/v1/restore` - Restore from backup
- `GET /api/v1/snapshots` - List available snapshots
- `GET /api/v1/status` - Check system status
- `POST /api/v1/healthcheck` - Run health checks

### Web Interface
- Dashboard for monitoring backup status
- Real-time status updates
- System status panel (`/system`) with status + health summaries and raw API details
- Activity logs and reports

## Getting Started

### Prerequisites
- Go 1.25 or higher
- Docker and Docker Compose (for development/testing)
- SSH access to WordPress servers
- restic (for backup storage)
- MySQL/MariaDB database access

### Installation

1. Clone the repository:
```bash
git clone https://github.com/yourusername/wp-maintenance-automation-go.git
cd wp-maintenance-automation-go
```

2. Install dependencies:
```bash
go mod download
```

3. Build the application (three binaries):
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

5. Run the application(s):

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

### Development

Run the development servers:
```bash
make dev
```

Run tests:
```bash
make test
```

Run linter:
```bash
make lint
```

## Operational Notes

### Authentication and Login Modes
- Login supports two modes:
  - First setup / forced password mode: requires `password` and `passwordConfirm`.
  - Normal login mode: requires only `password`.
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
- Runtime DB file: `data/wp-maintenance.db` (host filesystem).
- In Docker Compose, host `./data` is mounted into `/app/data`.
- `data/` is ignored by git in `.gitignore`.

### Encryption at Rest
- User passwords are stored as bcrypt hashes in `users.password_hash`.
- Sensitive site fields are encrypted at rest using AES-GCM with prefix `enc:v1:`:
  - `sites.db_password`
  - `sites.restic_password_file`
- Key source order:
  - `DATA_ENCRYPTION_KEY`
  - fallback `SECRET_KEY`
  - fallback `default-secret-key`
- Legacy plaintext site secrets are auto-migrated to encrypted form on DB open.

### Snapshots Endpoint Through Web UI
- The web proxy now exposes snapshots routes used by the UI:
  - `GET /api/snapshots`
  - `GET /api/v1/snapshots`
- If unauthenticated, the web layer can redirect to `/login`; the snapshots page now handles this safely.

## Configuration

Key environment variables see `.env.example` for complete configuration options:
- `API_PORT` - API server port (default: 8081)
- `WEB_PORT` - Web UI server port (default: 8080)
- `API_URL` - API server URL for web UI proxy (default: http://localhost:8081)
- `WP_SSH_HOST` - WordPress server hostname
- `WP_SSH_USER` - SSH username
- `WP_ROOT` - WordPress installation path
- `RESTIC_REPOSITORY` - Backup repository URL
- `DB_HOST` - Database server
- `DB_USER` - Database username
- `DB_PASSWORD` - Database password
- `ADMIN_USERNAME` - Initial admin username
- `ADMIN_PASSWORD` - Initial admin password
- `SECRET_KEY` - JWT/API token signing key
- `DATA_DIR` - Data directory for config and state (default: ./data)

## API Documentation

Full API documentation available at `/api/docs` (when running with Swagger integration).

### Example Usage

Create a backup:
```bash
curl -X POST http://localhost:8081/api/v1/backup \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{"wp_host":"example.com","wp_user":"ubuntu","wp_root":"/var/www/html"}'
```

Check status:
```bash
curl http://localhost:8081/api/v1/status
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

Run all tests:
```bash
make test
```

Run tests with coverage:
```bash
make test-cover
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
docker-compose up -d
```

This starts both the API server (port 8081) and web UI (port 8080).

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

This project is licensed under the MIT License - see [LICENSE](LICENSE) file for details.

## Support

For issues and questions:
- Open an issue on GitHub
- Check existing documentation
- Contact maintainers