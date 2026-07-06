# Directory layout

> Phase: 5 · Status: Updated — Reports handler added

Repository root is `certwatch/` at `github.com/araujofrancisco/certwatch`.

```
certwatch/
├── cmd/
│   └── certwatch/            ✅ Main entrypoint with DI wiring + background jobs
├── internal/
│   ├── api/                  ✅ REST API + Web UI + Reports
│   │   ├── api.go            ✅ REST handler registration
│   │   ├── auth.go           ✅ Auth endpoints
│   │   ├── domains.go        ✅ Domain CRUD + scan + auto-scan on add
│   │   ├── certificates.go   ✅ Certificate endpoints + purge errors + filters
│   │   ├── reports.go        ✅ Inventory report API (Phase 5)
│   │   ├── ui.go             ✅ Go embed UI handler with per-page templates
│   │   └── web/
│   │       ├── templates/    ✅ 9 HTML pages (Bootstrap 5)
│   │       └── static/       ✅ CSS + JS
│   ├── auth/                 ✅ JWT authentication + bcrypt
│   ├── config/               ✅ Configuration loader (YAML + env vars)
│   ├── database/             ✅ SQLite connection + auto-migration runner
│   ├── discovery/            ✅ Scanner registry + HTTPS + CT + 6 stubs
│   ├── logging/              ✅ Structured logger (slog)
│   ├── notifier/             ✅ SMTP notification engine + profile matcher
│   ├── scheduler/            ✅ Cron-based job scheduler
│   ├── templates/            ✅ Email templates (immediate, daily, weekly)
│   ├── middleware/           ✅ Logging, recovery, auth, CORS, rate limit
│   ├── models/               ✅ Domain types + filter structs
│   ├── repository/           ✅ CRUD data access layer (parameterized SQL)
│   └── services/             ✅ Business logic layer
├── config/                   ✅ Default YAML config with all sections
├── docs/                     ✅ Documentation — start at _index.md
├── .github/workflows/        ✅ CI pipeline (lint → test → build → tidy)
├── Dockerfile                ✅ Multi-stage scratch build
├── docker-compose.yml        ✅ App + SQLite volume + healthcheck
├── Makefile                  ✅ build, run, test, lint, docker targets
├── go.mod / go.sum           ✅ Module: github.com/araujofrancisco/certwatch
├── .gitignore
└── README.md                 ✅ Project card with full API reference
```
