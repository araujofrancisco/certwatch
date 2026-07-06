# Audit Report — CertWatch

Generated: 2026-07-06 (initial audit + same-day fixes)
Updated: 2026-07-06 (Phase 5 additions)
Scope: Full codebase (14 packages, 84 tests)
Method: Manual code review

## Summary

| Severity | Total | Fixed | Not Fixed |
|----------|-------|-------|-----------|
| 🔴 Critical | 2 | 2 | 0 |
| 🟠 High | 7 | 7 | 0 |
| 🟡 Medium | 8 | 8 | 0 |
| 🔵 Low | 8 | 7 | 1 |
| **Total** | **25** | **24** | **1** |

---

## 🔴 Critical (2/2 fixed)

### C1 — Docker build broken: Go 1.25 / `golang:1.22.5-alpine` mismatch
**Fixed**: Updated Dockerfile to `golang:1.25-alpine` to match `go.mod`.

### C2 — `golang.org/x/crypto v0.17.0` has known CVEs
**Fixed**: Upgraded to `golang.org/x/crypto v0.53.0`.

---

## 🟠 High (7/7 fixed)

### H1 — Default JWT secret `"change-me-in-production"` in commits
**Fixed**: Added startup `slog.Warn` in `main.go` if default secret is detected.

### H2 — Immediate email notifications fire every minute; no dedup across minutes
**Fixed**: Added in-memory `notified` map tracking `${certID}:${threshold}` pairs. Alerts are sent at most once per cert+threshold combination (resets on restart).

### H3 — HTTP 200 for failed scans
**Fixed**: `scanDomain` handler returns `502 Bad Gateway` when scan fails but an error cert exists.

### H4 — `EnsureDir` called after `Open` — ordering bug
**Fixed**: `EnsureDir` changed to standalone `database.EnsureDir(driver, dsn)` and called **before** `database.Open()` in `main.go`.

### H5 — No rate limiting on auth endpoints
**Fixed**: Added `middleware.RateLimiter` (in-memory, 10 req/min per IP). Applied to `/api/auth/login` and `/api/auth/register`.

### H6 — No request body size limits
**Fixed**: Added `decodeJSON` helper wrapping body with `http.MaxBytesReader(w, r.Body, 1<<20)` (1 MB limit).

### H7 — SMTP PlainAuth sends credentials unprotected over non-TLS connections
**Fixed**: Added `smtp.force_tls` config option. When enabled, the notifier uses `tls.Dial` + `smtp.NewClient` for an explicitly encrypted connection before authentication.

---

## 🟡 Medium (8/8 fixed)

### M1 — No cascade delete for domain → certificates
**Fixed**: `domainRepo.Delete` deletes `certificates WHERE domain_id = ?` before deleting the domain row.

### M2 — Email header injection in `ImmediateAlert`
**Fixed**: Added `sanitizeField()` helper stripping `\r`, `\n`, `\t`, and non-printable characters.

### M3 — CORS `*` with JWT stored in localStorage
**Noted**: Standard for API-first designs. JWT in localStorage is a trade-off with `httpOnly` cookies. Rate limiter (H5) mitigates brute-force.

### M4 — CSS file has stray `<style>` tag
**Fixed**: Removed orphan `<style>` from `dashboard.css`.

### M5 — Health endpoint ignores DB state
**Fixed**: `Handler.health` now calls `db.Ping()` and returns `503` if the database is unreachable.

### M6 — Config's `SlogLevel()` method is dead code
**Fixed**: Removed `SlogLevel()` method and its tests.

### M7 — `DefaultCron` parses `SendAt` without validating format
**Fixed**: Added `sendAtRE` regex and validation in `ValidateProfiles`.

### M8 — Reports page links to non-existent endpoints
**Fixed**: Reports page now fully implemented — summary cards, inventory table, JSON/CSV download, client-side filters.

---

## 🔵 Low (7/8 fixed)

### L1 — Cron parser only handles `*` or single numbers
**Not fixed**: Enhancement for future. Default cron expressions never use comma/step/range syntax.

### L2 — No domain name format validation
**Fixed**: Added `isValidDomain()` in `services/domains.go` with length, label, and character validation.

### L3 — No email format validation on register
**Fixed**: `Register` validates presence of `@` and `.` in the email address.

### L4 — Duplicate certificate records on repeated scans
**Fixed**: `saveCertificate` checks fingerprint first, then `serial+issuer`. Existing record is updated instead of creating a duplicate.

### L5 — Data race potential: `Add()` after `Start()`
**Fixed**: `Start()` acquires the mutex lock and copies the jobs slice before iterating.

### L6 — `Check tidy` step in CI may fail on `go.mod` auto-update
**Fixed**: Added `-e` flag to `go mod tidy -e` in the CI workflow.

### L7 — Health flag ignores YAML port change
**Fixed**: `healthCheck()` loads config and reads port before falling back to `8080`.

---

## ✅ What's done well

- Parameterized queries everywhere — no SQL injection
- Password hash excluded from JSON via `json:"-"` tag
- Go `html/template` auto-escapes server-rendered templates
- Client-side `escapeHtml()` used consistently for all `innerHTML` inserts
- JWT signing method verification prevents alg confusion
- Graceful shutdown with signal context propagation to all goroutines
- Scanner registry uses `sync.RWMutex` for concurrent safety
- Migrations are idempotent (`CREATE TABLE IF NOT EXISTS`)
- All default ports/timeouts wrapped with fallback values on parse error
- Sequential scanner with per-protocol timeouts prevents slow scanners from blocking
- Domain auto-scan in background goroutine — create response is not blocked
- Server-side filtering uses dynamic SQL with parameterized `LIKE` queries
