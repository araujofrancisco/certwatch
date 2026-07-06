# CertWatch

A lightweight, self-hosted SSL/TLS certificate inventory and expiration monitoring platform.

CertWatch discovers certificates across multiple protocols, tracks expiry dates, and sends timely notifications before certificates expire.

## Features

- **REST API** — manage domains, certificates, and auth programmatically
- **Bulk import** — add many domains at once via API or web UI (objects or plain strings)
- **Certificate discovery** — HTTPS (SNI-aware) + Certificate Transparency (crt.sh) + 6 protocol stubs
- **Server-side filtering** — domains and certificates list endpoints support multi-criteria query params
- **Expiration monitoring** — track expiry across all discovered certificates
- **Email notifications** — immediate alerts at configurable thresholds, plus daily/weekly digest reports
- **Cron scheduling** — 5-field POSIX cron with timezone support
- **Web dashboard** — Bootstrap 5 UI with summary cards, filters, inventory reports, CSV/JSON export, bulk import
- **Auto-scan on add** — domains scanned in background immediately after creation
- **Certificate dedup** — fingerprint + serial/issuer matching to prevent duplicates
- **Backup & restore** — timestamped online snapshots with 30-day retention, Docker & standalone support
- **Docker deployment** — multi-stage scratch image
- **Input validation** — domain format validation, email format check, request body size limits (1 MB)
- **Rate limiting** — 10 requests/minute per IP on auth endpoints
- **Security** — startup warning for default JWT secret, cascade delete, TLS SMTP option, notification dedup

## Quick start

```bash
make test       # 84 tests, all pass
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
discovery:
  scan_interval: "6h"
  timeout: "30s"
```

[Full config reference →](docs/guide/usage.md)
[Audit report →](docs/audit-report.md)

## Project status

| Phase | Status | Deliverable |
|-------|--------|-------------|
| 1 — Foundation | ✅ Complete | Go scaffold, Docker, SQLite, config, logging, CI |
| 2 — Backend | ✅ Complete | REST API, JWT auth, CRUD, scanners, 84 tests |
| 3 — Web UI | ✅ Complete | Bootstrap 5 dashboard, 7 pages, embed served |
| 4 — Notification | ✅ Complete | SMTP alerts, daily/weekly digests, cron scheduler |
| 5 — Reports | ✅ Complete | Inventory API + UI with summary cards, filters, export |
| 6 — Backup | ✅ Complete | Backup/restore scripts, 30-day retention, Docker & standalone |
| 7 — Bulk import | ✅ Complete | Multi-domain import via API + web UI |
| 8 — API docs | ⬜ | OpenAPI/Swagger |
| 9 — Testing | ⬜ | Integration, Docker tests |

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
| `GET` | `/health` | No | Health check (includes DB ping) |
| `POST` | `/api/auth/register` | RL | Register user |
| `POST` | `/api/auth/login` | RL | Login, get JWT |
| `GET` | `/api/domains` | Yes | List domains (`?q=&enabled=`) |
| `POST` | `/api/domains` | Yes | Add domain (auto-scans in background) |
| `POST` | `/api/domains/import` | Yes | Bulk import domains (objects or plain strings) |
| `GET` | `/api/domains/{id}` | Yes | Get domain |
| `DELETE` | `/api/domains/{id}` | Yes | Delete domain + cascade certs |
| `POST` | `/api/domains/{id}/scan` | Yes | Scan domain |
| `GET` | `/api/certificates` | Yes | List certs (`?q=&status=&protocol=&domain_id=&expiring=&expired=`) |
| `GET` | `/api/domains/{id}/certificates` | Yes | List certs for domain |
| `DELETE` | `/api/certificates/errors` | Yes | Purge all error certs |
| `DELETE` | `/api/domains/{id}/certificates/errors` | Yes | Purge error certs for domain |
| `GET` | `/api/reports/inventory` | Yes | Inventory report with summary stats |

RL = rate-limited (10 req/min per IP)

## Web UI routes

| Path | Description |
|------|-------------|
| `/login` | Sign in with email/password |
| `/register` | Create account |
| `/dashboard` | Summary cards + expiring certs table |
| `/domains` | Domain CRUD table with search/enabled filter |
| `/domains/{id}` | Domain detail + certificate history + purge errors |
| `/import` | Bulk import domains (paste one per line) |
| `/certificates` | All certs sorted by expiry with filters |
| `/reports` | Inventory with summary stats, filters, JSON/CSV |

## Documentation

- [Usage guide](docs/guide/usage.md) — configuration, API, web UI, notifications
- [Deployment guide](docs/guide/deployment.md) — Docker, production setup
- [Troubleshooting](docs/guide/troubleshooting.md) — common issues and fixes
- [Architecture](docs/architecture.md) — layer diagram and conventions
- [Audit report](docs/audit-report.md) — security review, 24/25 issues fixed

## Stack

- **Language**: Go 1.25+
- **Database**: SQLite (pure Go via modernc.org/sqlite, no CGO)
- **UI**: Bootstrap 5 served via Go embed (no build step required), 10 HTML templates
- **Deployment**: Docker (multi-stage scratch), Docker Compose
