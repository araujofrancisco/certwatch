# CertWatch

A lightweight, self-hosted SSL/TLS certificate inventory and expiration monitoring platform.

CertWatch discovers certificates across multiple protocols, tracks expiry dates, and sends timely notifications before certificates expire.

## Features

- **REST API** — manage domains, certificates, and auth programmatically
- **Certificate discovery** — HTTPS scanner with 7 additional protocol stubs (SMTP STARTTLS, IMAPS, LDAPS, POP3, FTPS, generic TLS, Certificate Transparency)
- **Expiration monitoring** — track expiry across all discovered certificates
- **Email notifications** — immediate alerts at configurable thresholds (30/14/7/3/1 days), plus daily/weekly digest reports
- **Cron scheduling** — 5-field POSIX cron with timezone support (default America/New_York)
- **Web dashboard** — Bootstrap 5 UI with login, domain management, certificate viewer, reports page
- **Docker deployment** — multi-stage scratch image, runs as non-root user
- **Input validation** — domain format validation, email format check, request body size limits (1 MB)
- **Rate limiting** — 10 requests/minute per IP on auth endpoints
- **Security** — startup warning for default JWT secret, cascade delete, TLS SMTP option, notification dedup

## Quick start

```bash
make test       # 90+ tests, all pass
make build      # static binary → build/certwatch
make run        # start on :8080
```

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

## Configuration

Set via `config/default.yaml` or `CERTWATCH_*` environment variables.

```yaml
server:
  host: "0.0.0.0"
  port: 8080
database:
  driver: sqlite
  dsn: "certwatch.db"
logging:
  level: info
  format: text
auth:
  secret: "change-me-in-production"  # ⚠️ override in production
  token_ttl: "24h"
notifications:
  smtp:
    host: ""
    port: 587
    force_tls: false                 # enable for explicit TLS
```

[Full config reference →](docs/guide/usage.md)
[Audit report →](docs/audit-report.md)

## Project status

| Phase | Status | Deliverable |
|-------|--------|-------------|
| 1 — Foundation | ✅ Complete | Go scaffold, Docker, SQLite, config, logging, CI, 30 tests |
| 2 — Backend | ✅ Complete | REST API, JWT auth, CRUD, HTTPS scanner + 7 stubs, 67 tests |
| 3 — Web UI | ✅ Complete | Bootstrap 5 dashboard, 8 pages, embed served |
| 4 — Notification | ✅ Complete | SMTP alerts, daily/weekly digests, cron scheduler, email templates |
| 5 — Reports | ➡️ Partial | Inventory page (UI) — CSV/Prometheus pending |
| 6 — Production | ⬜ | Reverse proxy, backup/restore scripts |
| 7 — API docs | ⬜ | OpenAPI/Swagger |
| 8 — Testing | ⬜ | Integration, Docker tests |

## Architecture

Clean architecture with dependency injection. [Full diagram →](docs/architecture.md)

```
cmd/certwatch/ → internal/api/ → internal/services/ → internal/repository/ → internal/database/
               → internal/auth/ → internal/middleware/
               → internal/discovery/
               → internal/notifier/ → internal/scheduler/
               → internal/templates/
               → internal/config/
               → internal/logging/
```

## API endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/health` | No | Health check |
| `POST` | `/api/auth/register` | No | Register user |
| `POST` | `/api/auth/login` | No | Login, get JWT |
| `GET` | `/api/domains` | Yes | List domains |
| `POST` | `/api/domains` | Yes | Add domain |
| `GET` | `/api/domains/{id}` | Yes | Get domain |
| `DELETE` | `/api/domains/{id}` | Yes | Delete domain |
| `POST` | `/api/domains/{id}/scan` | Yes | Trigger HTTPS scan |
| `GET` | `/api/certificates` | Yes | List all certs |
| `GET` | `/api/domains/{id}/certificates` | Yes | List certs for domain |

## Web UI routes

| Path | Description |
|------|-------------|
| `/login` | Sign in with email/password |
| `/register` | Create account |
| `/dashboard` | Summary cards + expiring certs table |
| `/domains` | Domain CRUD table with scan/delete actions |
| `/domains/{id}` | Domain detail + certificate history |
| `/certificates` | All certs sorted by expiry |
| `/reports` | Inventory page with CSV/metrics links |

## Documentation

- [Usage guide](docs/guide/usage.md) — configuration, API, web UI, health check, notifications
- [Deployment guide](docs/guide/deployment.md) — Docker, production setup
- [Troubleshooting](docs/guide/troubleshooting.md) — common issues and fixes
- [Architecture](docs/architecture.md) — layer diagram and conventions
- [Audit report](docs/audit-report.md) — security review, 24/25 issues fixed

## Stack

- **Language**: Go 1.22+
- **Database**: SQLite
- **UI**: Bootstrap 5 served via Go embed (no build step required)
- **Deployment**: Docker (multi-stage scratch, non-root user), Docker Compose
