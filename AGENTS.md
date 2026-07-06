# CertWatch — Agent instructions

## Status
Phases 1–4 implemented (Go backend, REST API, JWT auth, SQLite, HTTPS scanner, Bootstrap 5 web UI, cron notifications). 84 tests pass. Security audit: 24/25 issues fixed (`docs/audit-report.md`).

## First time
```bash
git init
```
No git repo yet. Disk dir is `certwatch/` (will need rename from `ssl-monitoring` on commit per `docs/directory-layout.md`).

Module: `github.com/araujofrancisco/certwatch` — already matches all import paths.

## Commands
```bash
make build       # static binary → build/certwatch (CGO_ENABLED=0)
make run         # go run ./cmd/certwatch/
make test        # go test ./... -v -count=1
make lint        # golangci-lint v1.59.1 (auto-installed if missing)
make tidy        # go mod tidy
make docker-build / docker-run / docker-stop / docker-logs
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
- **Scanner registration** in `main.go` with priority: HTTPS → SMTP STARTTLS → IMAPS → LDAPS → POP3 → FTPS → TLS → CT
- **Scheduler**: not cron-daemon — polls every 30s via `time.NewTicker`
- **Notification dedup**: in-memory map `${certID}:${threshold}` (lost on restart)
- **Web UI**: Go embed (`//go:embed`), no build step. Templates at `internal/api/web/templates/`, static at `internal/api/web/static/`
- **CI** (`.github/workflows/ci.yml`): lint → test → build → check tidy. Go version 1.22.5 (doesn't match go.mod 1.25.0 or Dockerfile 1.25-alpine)
- **Route patterns**: Go 1.22+ syntax `"METHOD /path"` with `http.NewServeMux`
- **Logging**: `slog`, not logrus/zap

## Style
- Raw SQL with parameterized queries (no ORM)
- `html/template` auto-escapes all UI templates
- JWT in `internal/auth/`, middleware in `internal/middleware/`
- Scanner registry pattern with `sync.RWMutex`

## Docs
Start at `docs/_index.md`.
