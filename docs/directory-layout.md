# Directory layout

> Phase: 9 · Status: Updated — OpenAPI docs, Scalar UI

Repository root is `certwatch/` at `github.com/araujofrancisco/certwatch`.

```
certwatch/
├── cmd/
│   └── certwatch/            ✅ Main entrypoint with DI wiring + background jobs
├── scripts/                  🔹 Backup & restore utilities (Phase 6)
│   ├── backup.sh             ✅ Timestamped backup of DB + config
│   └── restore.sh            ✅ Interactive restore from backup
├── internal/
│   ├── api/                  ✅ REST API + Web UI + Reports
│   │   ├── api.go            ✅ REST handler registration
│   │   ├── docs.go           ✅ OpenAPI docs + Scalar UI (Phase 9)
│   │   ├── openapi.yaml      ✅ OpenAPI 3.0 spec (15 endpoints, 26 schemas)
│   │   ├── auth.go           ✅ Auth endpoints
│   │   ├── domains.go        ✅ Domain CRUD + scan + auto-scan + bulk import + update + tags
│   │   ├── certificates.go   ✅ Certificate endpoints + purge errors + filters
│   │   ├── reports.go        ✅ Inventory report API (Phase 5)
│   │   ├── ui.go             ✅ Go embed UI handler with per-page templates
│   │   └── web/
│   │       ├── templates/    ✅ 11 HTML pages (Bootstrap 5) — added import.html, docs.html, group+tags fields
│   │       │   ├── layout.html / dashboard.html / domains.html / domain-detail.html
│   │       │   ├── certificates.html / reports.html / import.html
│   │       │   ├── auth-layout.html / login.html / register.html
│   │       │   └── docs.html          ✅ Scalar UI wrapper (CDN-loaded)
│   │       └── static/       ✅ CSS + JS
│   ├── auth/                 ✅ JWT authentication + bcrypt
│   ├── config/               ✅ Configuration loader (YAML + env vars) + CORS origins
│   ├── database/             ✅ SQLite connection + auto-migration runner (6 tables)
│   ├── discovery/            ✅ Scanner registry + HTTPS + CT + 6 stubs
│   ├── logging/              ✅ Structured logger (slog)
│   ├── notifier/             ✅ SMTP notification engine + profile matcher
│   ├── scheduler/            ✅ Cron-based job scheduler
│   ├── templates/            ✅ Email templates (immediate, daily, weekly)
│   ├── middleware/
│   │   ├── middleware.go     ✅ Logging, recovery, auth, CORS (configurable), rate limit (sliding window)
│   │   └── security.go      ✅ Security headers (CSP, XFO, etc.)
│   ├── models/               ✅ Domain types + filter structs + Tag struct
│   ├── repository/
│   │   ├── repository.go    ✅ Interfaces + constructors
│   │   ├── users.go         ✅ User CRUD
│   │   ├── domains.go       ✅ Domain CRUD with group_name
│   │   ├── certificates.go  ✅ Certificate CRUD + filters
│   │   ├── tags.go          ✅ Tag CRUD + SetDomainTags + GetDomainTags
│   │   └── notif_profiles.go ✅ Notification profile CRUD
│   └── services/
│       ├── services.go      ✅ Service structs with tags field
│       ├── domains.go       ✅ Domain logic + group/tags + update + bulk import
│       ├── certificates.go  ✅ Certificate logic
│       └── auth.go          ✅ Auth logic (password policy, info disclosure fix)
├── backups/                  ⬜ Backup archives (created by backup.sh)
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
