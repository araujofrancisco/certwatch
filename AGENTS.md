# CertWatch — Agent instructions

## Status
Phases 1–6 implemented (Go backend, REST API, JWT auth, SQLite, HTTPS scanner, Bootstrap 5 web UI, cron notifications, reports, backup/restore scripts). 84 tests pass. Security audit: 24/25 issues fixed (`docs/audit-report.md`).

Git repo: `github.com/araujofrancisco/certwatch` — all committed on `main`.

Module: `github.com/araujofrancisco/certwatch` — matches all import paths.

## Commands
```bash
make build       # static binary → build/certwatch (CGO_ENABLED=0)
make run         # go run ./cmd/certwatch/
make test        # go test ./... -v -count=1
make lint        # golangci-lint v1.59.1 (auto-installed if missing)
make tidy        # go mod tidy
make docker-build / docker-run / docker-stop / docker-logs
make backup       # scripts/backup.sh — timestamped archive of DB + config
make restore      # scripts/restore.sh — interactive restore from backup
```
Single-package test: `go test ./internal/services/ -v -count=1`

## Architecture
Clean architecture. DI wiring in `cmd/certwatch/main.go`:
`cmd/certwatch/` → `internal/api/` → `internal/services/` → `internal/repository/` → `internal/database/`

All packages are `internal/` — not importable from outside the module.

## Config
Loading order: defaults → `config/default.yaml` → `CERTWATCH_*` env vars. `CERTWATCH_CONFIG` env overrides config path.

Database: SQLite via `modernc.org/sqlite` (pure Go, no CGO). Auto-migrates 4 tables on startup. **`EnsureDir` before `Open`** (was audit bug H4).

## Key quirks
- **`-health` flag**: binary supports `-health` for Docker healthcheck (tries port from config, falls back to 8080)
- **Request body**: limited to 1 MB via `http.MaxBytesReader`
- **Rate limiting**: 10 req/min per IP, in-memory, auth endpoints only
- **Scanner registration** in `main.go`. Priority order: HTTPS → CT → SMTP → IMAP → POP3 → LDAP → FTP → TLS
- **Sequential scanner** in `ScanDomain`: tries each protocol in priority order with per-scanner timeout (HTTPS 5s, CT 10s, stubs 2s). First success wins. No more waiting for all scanners to finish.
- **Auto-scan on add**: domains are scanned in background goroutine when created via `POST /api/domains`
- **HTTPS scanner**: uses `ServerName` (SNI) + 5s dialer timeout
- **CT scanner**: queries crt.sh API with wildcard fallback (`%.registered-domain`) for subdomain cert discovery
- **Certificate dedup**: `saveCertificate` checks fingerprint first, then `serial+issuer`. Updates existing cert if match found.
- **Scheduler**: not cron-daemon — polls every 30s via `time.NewTicker`
- **Notification dedup**: in-memory map `${certID}:${threshold}` (lost on restart)
- **Web UI**: Go embed (`//go:embed`), no build step. 9 HTML templates at `internal/api/web/templates/`, static at `internal/api/web/static/`. Templates use `{{define "page"}}` to avoid name collisions.
- **Server-side filtering**: `GET /api/domains` and `GET /api/certificates` accept query params (`q`, `status`, `protocol`, `domain_id`, `expiring`, `expired`, `enabled`). Dynamic SQL with `LIKE` and parameterized queries.
- **Reports**: `GET /api/reports/inventory` returns combined domain+cert data with summary stats. UI has summary cards, client-side filters, CSV/JSON download buttons.
- **CI** (`.github/workflows/ci.yml`): lint → test → build → check tidy. Go version 1.22.5 (doesn't match go.mod 1.25.0 or Dockerfile 1.25-alpine)
- **Route patterns**: Go 1.22+ syntax `"METHOD /path"` with `http.NewServeMux`
- **Logging**: `slog`, not logrus/zap

## Style
- Raw SQL with parameterized queries (no ORM)
- `html/template` auto-escapes all UI templates
- JWT in `internal/auth/`, middleware in `internal/middleware/`
- Scanner registry pattern with `sync.RWMutex`

## API endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/health` | No | Health check (includes DB ping) |
| `POST` | `/api/auth/register` | RL | Register user |
| `POST` | `/api/auth/login` | RL | Login, get JWT |
| `GET` | `/api/domains` | Yes | List domains (`?q=&enabled=`) |
| `POST` | `/api/domains` | Yes | Add domain (auto-scans in background) |
| `GET` | `/api/domains/{id}` | Yes | Get domain |
| `DELETE` | `/api/domains/{id}` | Yes | Delete domain + cascade certs |
| `POST` | `/api/domains/{id}/scan` | Yes | Scan domain |
| `GET` | `/api/certificates` | Yes | List certs (`?q=&status=&protocol=&domain_id=&expiring=&expired=`) |
| `GET` | `/api/domains/{id}/certificates` | Yes | List certs for domain |
| `DELETE` | `/api/certificates/errors` | Yes | Purge all error certs |
| `DELETE` | `/api/domains/{id}/certificates/errors` | Yes | Purge error certs for domain |
| `GET` | `/api/reports/inventory` | Yes | Inventory report with summary stats |

RL = rate-limited (10 req/min per IP)

## Docs
Start at `docs/_index.md`.
