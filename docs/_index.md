# CertWatch Documentation

Project: **CertWatch** — lightweight, self-hosted SSL certificate inventory and expiration monitoring.

Status: **Phases 1–9 implemented** (Go backend, REST API, JWT auth, SQLite, HTTPS+CT scanners, Bootstrap 5 web UI, cron notifications, inventory reports, backup/restore scripts, bulk import, groups, tags, domain update, OpenAPI docs, 84 tests pass). Security audit completed — **28/28 issues fixed**.

## Quick nav

| Document | Status | Summary |
|----------|--------|---------|
| [architecture.md](architecture.md) | ✅ Updated | Clean architecture, DI, Web UI, scanner design |
| [directory-layout.md](directory-layout.md) | ✅ Updated | All paths, including reports |
| [guide/usage.md](guide/usage.md) | ✅ Updated | Full config ref, API filters, reports endpoint, bulk import |
| [guide/deployment.md](guide/deployment.md) | ✅ Updated | Docker, compose, production deployment, backup/restore |
| [guide/troubleshooting.md](guide/troubleshooting.md) | ✅ Updated | Common issues, fixes |
| [openapi.yaml](openapi.yaml) | ✅ Complete | OpenAPI 3.0 spec for all 15 REST endpoints |
| [audit-report.md](audit-report.md) | ✅ Complete | Security audit, 28/28 fixes applied |
