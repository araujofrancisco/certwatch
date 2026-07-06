# Directory layout

> Phase: 3 · Status: Updated — Web UI entries added

Repository root is `certwatch/` at `github.com/araujofrancisco/certwatch`.

Legend: ✅ exists now, ⬜ planned.

```
certwatch/
├── cmd/
│   └── certwatch/           ✅ Main entrypoint with DI wiring + background jobs
├── internal/
│   ├── api/                 ✅ REST API + Web UI handlers (Phase 2+3)
│   │   ├── api.go           ✅ REST handler registration
│   │   ├── auth.go          ✅ Auth endpoints
│   │   ├── domains.go       ✅ Domain CRUD endpoints
│   │   ├── certificates.go  ✅ Certificate endpoints
│   │   ├── ui.go            ✅ Go embed UI handler (Phase 3)
│   │   └── web/             ✅ Web UI assets (Phase 3)
│   │       ├── templates/   ✅ 8 HTML pages (Bootstrap 5)
│   │       └── static/      ✅ CSS + JS
│   ├── auth/                ✅ JWT authentication + bcrypt (Phase 2)
│   ├── config/              ✅ Configuration loader (YAML + env vars)
│   ├── database/            ✅ SQLite connection + auto-migration runner
│   ├── discovery/           ✅ Scanner registry + HTTPS scanner + 7 stubs (Phase 2)
│   │   ├── https/           (stubs: smtp, imap, pop3, ldap, ftp, tls, ct)
│   ├── logging/             ✅ Structured logger (slog)
│   ├── notifier/            ✅ SMTP notification engine + profile matcher (Phase 4)
│   ├── scheduler/           ✅ Cron-based job scheduler (Phase 4)
│   ├── templates/           ✅ Email templates (immediate, daily, weekly) (Phase 4)
│   ├── middleware/          ✅ Logging, recovery, auth, CORS middleware (Phase 2)
│   ├── models/             ✅ Domain types (Phase 2)
│   ├── repository/         ✅ CRUD data access layer (Phase 2)
│   └── services/           ✅ Business logic layer (Phase 2)
├── web/                     (moved to internal/api/web/ in Phase 3)
├── migrations/              ⬜ SQL migration files (Phase 2+)
├── config/                  ✅ Default YAML config with all sections
├── docs/                    ✅ Documentation — start at _index.md
├── scripts/                 ⬜ Utility scripts (Phase 6)
├── .github/workflows/       ✅ CI pipeline (lint + test + build)
├── Dockerfile               ✅ Multi-stage scratch build, runs as nobody
├── docker-compose.yml       ✅ App + SQLite volume + healthcheck
├── Makefile                 ✅ build, run, test, lint, docker targets
├── go.mod / go.sum          ✅ Module: github.com/certwatch/certwatch
├── .gitignore               ✅ Go project ignores
└── README.md                ✅ Project card with full API reference
```
