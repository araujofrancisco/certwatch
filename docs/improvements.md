# CertWatch — Improvement Proposals

> Architectural review by a senior system architect.
> 45 items across 7 categories with priority ratings.
>
> **Legend:** P0=Critical · P1=High · P2=Medium · P3=Low

---

## P0 — Critical

### 1. Move `notifiedSet` from `main.go` into `notifier` package


**Status: ✅ Done** — dedup state now lives behind `DedupStore`, implemented persistently in `internal/repository/dedup.go`; `main.go` no longer owns any notification logic.
**File:** `cmd/certwatch/main.go:193-230`

The in-memory notification dedup struct is pure business logic that leaks into the entrypoint. It should live in `internal/notifier/` alongside the rest of the notification engine.

**Why:** Enables unit testing, prevents import cycles, and keeps the entrypoint focused on wiring.

**Effort:** <1h

---

### 2. Extract notification orchestration from `main.go`


**Status: ✅ Done** — extracted into `internal/notifier/runner.go` (`Runner.Run`).
**File:** `cmd/certwatch/main.go:232-387`

`runNotifications` is ~75 lines of orchestration logic (profile validation, cron scheduling, immediate check, digest dispatch). Extract into a `notifier.Runner` or `services.NotificationService`.

**Why:** `main.go` should wire dependencies, not implement business flows.

**Effort:** 2–3h

---

### 3. Replace scheduler polling (30s) with sleep-until-next-match


**Status: ✅ Done** — `CronExpr.Next` computes the next match; each job sleeps until then.
**File:** `internal/scheduler/scheduler.go:127`

Each job wakes up every 30 seconds (~2,880 wakeups/day) just to check if the cron expression matches. Compute `time.Until(nextMatch)` and sleep instead.

**Why:** Reduces CPU waste significantly when many profiles are registered.

**Effort:** 3–4h

---

### 4. Add graceful goroutine tracking for background scan

**File:** `cmd/certwatch/main.go:161`

`runBackgroundScan` launches goroutines via `ScanAllDomains` but has no completion tracking. Server shutdown can interrupt mid-scan, leaving partial results and no log.

**Why:** Prevents data inconsistency on shutdown.

**Effort:** 2h

---

### 5. Support concurrent scanner execution per domain

**File:** `internal/services/domains.go:138-161`

HTTPS (5s) → CT (10s) sequentially = up to 15s per domain. With 10 concurrent domains via semaphore, worst-case latency for 100 domains = 150s. Run scanners in parallel with a timeout race (first success wins).

**Why:** Halves per-domain scan time for domains where both protocols are enabled.

**Status: ✅ Done** — superseded by the CT multi-provider aggregator (`internal/ctsearch`). CT providers (CertSpotter, ctlogs.dev) are queried **concurrently** behind a shared 1 QPS limiter with failover, per-provider cooldown, and partial-results-on-deadline, which removes the single slow-provider bottleneck; sequential HTTPS→CT ordering is retained.

**Effort:** 3–4h

---

### 6. Deduplicate filter clause building in repository layer

**Files:** `internal/repository/domains.go:44-72`, `internal/repository/certificates.go:63-108`

`ListFiltered` and `CountFiltered` in both repositories build identical WHERE clauses from scratch. Extract into a shared `queryBuilder` helper or `filterClause` function.

**Why:** ~60 lines of duplicated code; every new filter field must be added in 4 places.

**Effort:** 2h

---

### 7. Wrap bulk imports in a SQL transaction


**Status: ✅ Done** — validation/dedup-check phase followed by atomic insert via `DomainRepository.CreateMany` (transactional, rollback on failure).
**File:** `internal/services/domains.go:328-396`

`BulkAddDomains` inserts domains one-by-one. A mid-import crash leaves the database in an inconsistent half-imported state.

**Why:** Atomicity guarantee for bulk operations.

**Effort:** 1h

---

### 8. Add context propagation through service & repository layers


**Status: ✅ Done** — all repository/service methods take `ctx`; handlers pass `r.Context()`.
**Files:** `internal/services/domains.go`, `internal/services/certificates.go`, `internal/repository/`

Methods like `ListCertificates()`, `ListDomains()`, `GetDomain()` don't accept `context.Context`. This prevents cancellation, deadline propagation, and distributed tracing.

**Why:** Essential for production observability and proper HTTP request lifecycle management.

**Effort:** 4–6h (touches most service + repository methods)

---

## P1 — High

### 9. Add database connection pool limits


**Status: ✅ Done**
**File:** `internal/database/database.go:19`

`sql.Open` is called with no pool configuration. SQLite in WAL mode still benefits from limiting concurrent connections. Add:

```go
db.SetMaxOpenConns(1)   // SQLite serializes writes
db.SetMaxIdleConns(1)
db.SetConnMaxLifetime(time.Hour)
```

**Why:** Prevents `database is locked` errors under concurrent load.

**Effort:** <1h

---

### 10. Add persistent notification dedup


**Status: ✅ Done** — `notification_dedup` table (migration 4) + hourly TTL cleanup in the notifier Runner.
Current in-memory dedup (`map[string]time.Time` in `main.go`) is lost on restart. A restart within the 24h dedup window means all currently-matching certs get re-alerted.

**Why:** Prevents alert storms after deployments or crashes.

**Effort:** 3–4h (new `notification_dedup` table, TTL cleanup)

---

### 11. Replace dashboard/report in-memory aggregation with SQL


**Status: ✅ Done** — `CountExpiryBuckets`/`ListExpiringSoon`/`ListLatestByDomain` aggregate in SQL; inventory loads one cert per domain instead of all rows.
**Files:** `internal/api/dashboard.go:26-84`, `internal/api/reports.go:31-97`

Both endpoints load ALL certificates into memory to compute counts. With 50k+ certs this becomes slow and memory-heavy.

**Why:** Keeps dashboard O(1) regardless of data size. Uses SQL `COUNT`, `GROUP BY`, `SUM`.

**Effort:** 4h

---

### 12. Add config validation at startup


**Status: ✅ Done** — `Config.Validate()` (hard errors) and `Config.Warnings()` (non-fatal) run before startup.
**File:** `internal/config/config.go`

`config.Load()` reads the file and applies env overrides but never validates. Catch these early:
- Empty SMTP host but notification profiles enabled
- Invalid duration strings (caught via `ParseDuration` at runtime — should fail at config load)
- JWT secret < 32 characters
- Negative timeout values

**Why:** Fail fast at startup, not after the server is accepting requests.

**Effort:** 3h

---

### 13. Add global request timeout middleware


**Status: ✅ Done** — `middleware.Timeout`, configurable via `server.request_timeout`.
Each handler manages its own timeout or deadline inconsistently. Some use context, some don't.

**Why:** Ensures every HTTP request has a hard deadline. Prevents resource leaks from hung connections.

**Effort:** 1h

---

### 14. Improve API error messages with structured codes

Generic errors like `"failed to list domains"` give operators no diagnostic signal. Add a consistent error envelope with machine-readable codes:

```json
{"error": {"code": "DB_QUERY", "message": "failed to list domains", "detail": "SQL error: ..."}}
```

**Why:** Lets automation (monitoring, scripts) handle errors programmatically instead of string-matching error messages.

**Effort:** 4–6h

---

### 15. Add loading states and error toasts to web UI

**Files:** `internal/api/web/static/js/dashboard.js`, all HTML templates

- The UI currently renders blank screens until API calls resolve
- Failed API calls are silently swallowed — users get no feedback

**Why:** Basic UX requirement. Users need to know something is happening and when something goes wrong.

**Effort:** 3–4h

---

### 16. Add database indexing strategy


**Status: ✅ Done** — migration 5 adds six indexes.
No explicit indexes beyond PRIMARY KEYs. Add:
- `certificates(domain_id)` — already a FK but no index
- `certificates(not_after)` — dashboard expiry queries
- `certificates(status)` — filter queries
- `domains(enabled)` — background scan only reads enabled
- `domains(group_name)` — group filtering

**Why:** Query plans for filtered certificate/domain lists will do full table scans on datasets > 10k rows.

**Effort:** 2h

---

### 17. Add database migration versioning


**Status: ✅ Done** — `schema_version` table with ordered, transactionally-applied migrations.
Current migrations are `CREATE TABLE IF NOT EXISTS` with no version tracking. Adding columns later requires manual ALTER TABLE tracking.

**Why:** Allows safe schema evolution without manual intervention.

**Effort:** 3h

---

## P2 — Medium

### 18. Implement the documented `CERTWATCH_SMTP_FORCE_TLS` env var


**Status: ✅ Done**
`docs/guide/usage.md:106` documents the env var but `internal/config/config.go:131-190` never reads it.

**Effort:** <1h

---

### 19. Make rate limiter settings configurable


**Status: ✅ Done** — `server.rate_limit`, `server.read_rate_limit`, `server.rate_limit_window` (+env overrides).
`main.go:126` hardcodes 10 req/min. Add `server.rate_limit` and `server.rate_limit_window` to config.

**Effort:** 1h

---

### 20. Add configurable DB pool settings

Add to config:
```yaml
database:
  pool:
    max_open: 1
    max_idle: 1
    max_lifetime: "1h"
```

**Effort:** 1h

---

### 21. Add `Stop()` method to Scheduler

Currently the scheduler only stops via context cancellation. A `Stop()` method would:
- Let tests cleanly stop scheduler goroutines without context plumbing
- Enable hot-reload of notification profiles

**Effort:** 1h

---

### 22. Add structured error types to repository layer


**Status: ✅ Done** — `repository.ErrNotFound`/`ErrConflict` sentinels; services and API use `errors.Is` (404 vs 500 now correct).
Repository methods return `fmt.Errorf(...)` — callers must string-match to distinguish "not found" from "DB error". Introduce `ErrNotFound`, `ErrConflict`, `ErrInternal`.

**Effort:** 4h

---

### 23. Wrap scanner HTTP transport in a shared instance


**Status: ✅ Done** — package-level `sharedTransport` reused by CT scanners.
`discovery/ct.go:27-34` creates a new `http.Transport` per scanner. Use `http.DefaultTransport` or a shared configured transport.

**Effort:** <1h

---

### 24. Add RFC 8288 pagination headers to API

Add `Link` header (prev/next/first/last) and `X-Total-Count` header to paginated list endpoints.

**Effort:** 2h

---

### 25. Add scan history endpoint

`GET /api/domains/{id}/scans` — return all scan attempts with duration, protocol, status, error message. Currently only the latest cert is accessible.

**Effort:** 3–4h

---

### 26. Add static asset caching headers

`api/ui.go:74` serves static files with no cache headers. Add `Cache-Control: public, max-age=31536000, immutable` with content-hashed filenames.

**Effort:** 2h

---

### 27. Enhance health endpoint with scan status

Add to `/health` response:
- `last_scan_at`
- `scan_in_progress`
- `last_scan_duration_seconds`

**Effort:** 1h

---

### 28. Add account lockout on brute force

10 req/min global rate limiting doesn't prevent targeted password guessing against a single account. Lock account after 5 failed attempts within 15 minutes.

**Effort:** 4h

---

### 29. Move `rand` seed for tag color generation


**Status: ✅ Done** — deterministic FNV-1a hash of the tag name; no `math/rand`.
`services/domains.go:245-251` uses `math/rand` without explicit seeding in newer Go versions. Use `crypto/rand` or ensure `math/rand` is seeded.

**Effort:** <1h

---

### 30. Add startup config log with secrets redacted


**Status: ✅ Done** — `Config.LogValue` (slog.LogValuer) redacts secret/password automatically.
No startup log shows which configuration was loaded. Log all config values at startup with passwords/secrets masked.

**Effort:** 1h

---

## P3 — Low

### 31. Add notification profile config validation at startup

`notifier.ValidateProfiles` is called inside `runNotifications` (lines 247-249), after the server is already up. Move to config loading.

**Effort:** 1h

---

### 32. Add JWT refresh token flow

Current single token with 24h TTL forces frequent logins. Add `POST /api/auth/refresh` with a long-lived refresh token stored as httpOnly cookie.

**Effort:** 4h

---

### 33. Add SMTP password support via env/secret file

Support `CERTWATCH_SMTP_PASSWORD_FILE` and `CERTWATCH_SMTP_PASSWORD_CMD` so passwords don't need to be in YAML.

**Effort:** 2h

---

### 34. Add password reset flow

`POST /api/auth/forgot-password` and `POST /api/auth/reset-password`. No recovery path currently exists for forgotten passwords.

**Effort:** 4–6h

---

### 35. Add dashboard trend data

A `GET /api/dashboard/history?days=7` endpoint returning daily healthy/warning/expired snapshots helps users see trends.

**Effort:** 3h

---

### 36. Add session invalidation on password change

`PUT /api/auth/password` changes the password but existing JWTs remain valid until they expire. Add a `token_version` field to the user model and include it in JWT claims.

**Effort:** 2h

---

### 37. Switch JWT storage from localStorage to httpOnly cookie

localStorage is accessible to any JS on the same origin, making it XSS-vulnerable. httpOnly cookies prevent client-side scripts from reading the token.

**Effort:** 3h (frontend + backend changes)

---

### 38. Add `robots.txt` and `security.txt`

- `GET /robots.txt` → `User-agent: * Disallow: /`
- `GET /.well-known/security.txt` → security contact

**Effort:** <1h

---

### 39. Add bulk operations in web UI

Select-all checkboxes, bulk delete, bulk disable/enable, bulk tag assignment. Currently users must delete/edit each domain individually.

**Effort:** 4–6h

---

### 40. Add integration test for full scan pipeline

Existing tests cover components individually but no end-to-end test that: creates a domain → triggers scan → verifies certificate in DB → checks dashboard counts.

**Effort:** 3h

---

### 41. Add `go run`-compatible hot reload for development

Add `make dev` that uses `air` or `rivet` for file-watching auto-reload. Developers currently restart manually on every Go change.

**Effort:** 1h

---

### 42. Eliminate hardcoded config path duplication

`config/default.yaml` path is hardcoded in `main.go:70` and `main.go:45` (healthCheck). Define as a single package-level constant.

**Effort:** <1h

---

## Quick Wins (< 1h each)

| # | Description | Est. |
|---|-------------|------|
| 1 | Move `notifiedSet` to `notifier` package | 30m |
| 9 | Add DB pool config | 20m |
| 18 | Wire `CERTWATCH_SMTP_FORCE_TLS` env var | 10m |
| 23 | Share HTTP transport for CT scanner | 10m |
| 28 | Seed `math/rand` or use `crypto/rand` | 5m |
| 29 | Startup config log with redaction | 30m |
| 32 | Add `robots.txt` + `security.txt` | 15m |
| 36 | Deduplicate config path constant | 5m |
| 37 | Add `Stop()` to scheduler | 30m |
| 10 | Rate limit configurable | 30m |
| 13 | Global request timeout middleware | 45m |
| 26 | Static asset caching | 30m |

---

## Priority Summary

| Priority | Count | Est. Effort |
|----------|-------|-------------|
| **P0 — Critical** | 8 | ~18–26h |
| **P1 — High** | 9 | ~22–30h |
| **P2 — Medium** | 13 | ~24–34h |
| **P3 — Low** | 12 | ~28–38h |
| **Total** | **42** | **~92–128h** |

> 3 items (bulk UI operations, password reset, JWT refresh) are counted as single items but could be decomposed into sub-tasks.

---

## Dependency Graph

```
Phase A (P0+P1 quick wins)
└── Phase B (P0 architectural)
    ├── Phase C (P1 remaining)
    │   ├── Phase D (P2)
    │   └── Phase E (P3)
```

- **Phase A** should include items 1, 9, 18, 23, 28, 29, 32, 36, 37, 10 (easy wins)
- **Phase B** should include items 2, 3, 4, 5, 6, 7, 8 (architectural)
- **Phase C** can run in parallel with Phase B for non-conflicting items
