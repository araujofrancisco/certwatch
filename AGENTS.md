# CertWatch — Agent instructions

## Status

Phases 1–9 implemented (Go backend, REST API, JWT auth, SQLite, HTTPS+CT scanners, Bootstrap 5 web UI, cron notifications, reports, backup/restore, bulk import, groups, tags, OpenAPI docs). All tests pass. Security audit: 28/28 fixed.

Module: `github.com/araujofrancisco/certwatch`.

## Commands

```bash
make all         # lint → test → build (ordered)
make build       # static binary → build/certwatch (CGO_ENABLED=0)
make run         # go run ./cmd/certwatch/
make test        # go test ./... -v -count=1
make lint        # golangci-lint v1.64.8 (auto-installed if missing)
make tidy        # go mod tidy
make docker-{build,run,stop,logs}
make backup      # scripts/backup.sh — timestamped archive
make restore     # scripts/restore.sh — interactive restore
```

Single-package: `go test ./internal/services/ -v -count=1`

## Architecture

Clean architecture. DI wiring in `cmd/certwatch/main.go`:
`cmd/certwatch/` → `internal/api/` → `internal/services/` → `internal/repository/` → `internal/database/`

All packages under `internal/`. Raw SQL with parameterized queries (no ORM).

## Config & DB

Loading order: defaults → `config/default.yaml` → `CERTWATCH_*` env vars. `CERTWATCH_CONFIG` overrides config path.

SQLite via `modernc.org/sqlite` (pure Go, no CGO). **`EnsureDir` before `Open`** (audit bug H4). Inline migrations in `internal/database/database.go` (6 tables: `users`, `domains`, `certificates`, `notification_profiles`, `tags`, `domain_tags`). `migrations/` dir is empty placeholder.

## Key quirks

- **Scanner reg**: only HTTPS and CT registered in `main.go`. 6 protocol stubs in `internal/discovery/` are **not wired**.
- **Sequential scan** (`services/domains.go:138`): tries HTTPS (5s) then CT (10s). First success wins. Background scan caps at 10 concurrent via semaphore.
- **Auto-scan on add**: background goroutine on `POST /api/domains` and bulk import.
- **Cert dedup**: checks fingerprint first, then `serial+issuer`. Updates existing cert on match.
- **Scheduler** (`scheduler/scheduler.go:127`): cron-expression engine polls every 30s. Not a real cron daemon.
- **Notification dedup**: in-memory `${certID}:${threshold}` — lost on restart.
- **Web UI**: Go embed, no build step. 9 page templates (7 × `layout.html`, 2 × `auth-layout.html`) + raw `docs.html` for Scalar UI.
- **Server-side filtering**: `GET /api/domains` and `GET /api/certificates` accept `q`, `status`, `protocol`, `domain_id`, `expiring`, `expired`, `enabled`. Dynamic SQL with `LIKE`.
- **JWT**: default secret `change-me-in-production` triggers startup warning.
- **Rate limiting**: 10 req/min per IP, sliding window, auth endpoints only.
- **CORS**: config via `server.cors_allowed_origins` or `CERTWATCH_SERVER_CORS_ORIGINS` env. Defaults: `http://localhost:8080`, `http://127.0.0.1:8080`. Any localhost origin auto-accepted.
- **CI** (`.github/workflows/ci.yml`): lint → test → build → check tidy. Go 1.25, golangci-lint installed from source.
- **Tests**: real SQLite via `os.MkdirTemp` — no mocks or fixtures.
- **`-health` flag**: Docker healthcheck, tries config port → falls back to 8080.

## API docs

OpenAPI 3.0 spec at `internal/api/openapi.yaml`. Interactive Scalar UI at `GET /api/docs`. Raw YAML at `GET /api/docs/openapi.yaml`.

## More docs

- `docs/guide/usage.md` — full config reference, API examples, web UI
- `docs/guide/deployment.md` — Docker, backup/restore, production setup
- `docs/architecture.md` — layer diagram, scanner design, security
- `docs/audit-report.md` — security audit, 28/28 issues fixed
- `docs/roadmap.md` — planned phases 10–13
