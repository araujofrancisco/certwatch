# CertWatch Roadmap

> Planned improvements organized by phase. All changes are backward-compatible.

**Legend:** ⬜ Planned · 🔄 In progress · ✅ Done

---

## Phase 10 — Foundational

### 10.1 API Key Authentication

**Goal:** Static, revocable tokens for automation, CI/CD, and AI agents.

- New table `api_keys` with bcrypt-hashed keys, optional expiry, `last_used_at` tracking
- `POST /api/api-keys` — generates a `cw_`-prefixed key, shown once
- `DELETE /api/api-keys/{id}` — revoke
- `GET /api/api-keys` — list keys (prefix + name only, never hash)
- Auth middleware tries JWT first (`Authorization: Bearer`), falls back to API key; also accepts `X-API-Key` header
- Audit: no plaintext stored; hash verified at lookup time

### 10.2 Webhook Notifications

**Goal:** Deliver alerts to Slack, Discord, Teams, PagerDuty, etc. — not just email.

- New notification profile type `type: webhook`:
  ```yaml
  - name: Ops Slack
    type: webhook
    url: "https://hooks.slack.com/services/..."
    secret: "hmac-secret"          # optional HMAC-SHA256 signing
    thresholds: [30, 14, 7]
  ```
- `internal/notifier/webhook.go`: POST JSON payload, HMAC signing, 10s timeout
- Payload events: `cert_discovered`, `cert_expiring`, `cert_expired`, `scan_complete`, `scan_error`
- At-most-once delivery; optional `retry_count` up to 3 with 5s backoff

### 10.3 Pagination + Sorting

**Goal:** Handle large inventories without unbounded responses.

- Query params: `?limit=50&offset=0` (max 1000, default 100)
- `?sort_by=not_after&order=asc` with whitelist of allowed columns
- Response: `X-Total-Count` header + `Link` header for prev/next (RFC 8288)
- Repository layer: `ListFiltered` accepts `Limit`, `Offset`, `SortBy`, `SortOrder`

### 10.4 Version Endpoint

**Goal:** Standard endpoint for automation compatibility checks.

- `GET /api/version` → `{"version": "0.1.0", "build": "<commit-sha>"}`
- Version injected via `-ldflags` in Makefile

---

## Phase 11 — AI & Automation

### 11.1 MCP Server Mode

**Goal:** AI coding assistants query certs directly from the editor.

- `certwatch mcp` — JSON-RPC 2.0 over stdio (Model Context Protocol)
- Tools: `list_domains`, `list_certificates`, `scan_domain`, `get_inventory_report`, `get_certificate`, `purge_errors`
- Resources: `certwatch://expiring`, `certwatch://domains/{id}`, `certwatch://summary`
- Reuses existing service layer; ephemeral internal token for auth
- Runs until stdin closes — no HTTP server needed

### 11.2 Event Stream (SSE)

**Goal:** Push-based cert lifecycle events for AI agents and real-time dashboards.

- `GET /api/events?since=<cursor>` — Server-Sent Events
- In-memory `EventBus` with subscriber channels (non-blocking send, drops slow consumers)
- Events: `cert_discovered`, `cert_expiring`, `cert_expired`, `scan_complete`, `scan_error`
- 30s heartbeat comments to keep connection alive
- In-memory ring buffer for cursor replay (best-effort, lost on restart)

### 11.3 OpenAPI Enrichment

**Goal:** AI tools auto-generate reliable clients from the spec.

- Add `operationId` to every path (e.g. `operationId: listDomains`)
- Full `description` + `example` on all parameters
- `x-openai-isConsequential: false` on read-only endpoints
- Consistent error responses (400/401/404/500) across all paths
- Example values for every request/response schema

### 11.4 Semantic Certificate Search

**Goal:** Natural-language queries like "find google wildcard certs expiring soon".

- SQLite FTS5 virtual table on `subject`, `issuer`, `serial`
- Auto-populated on certificate insert (trigger or Go code)
- `POST /api/certificates/search` with `{"q":"google wildcard soon"}`
- Results ranked by FTS5 relevance score; `LIKE` fallback for older databases

---

## Phase 12 — Observability & Deployment

### 12.1 Prometheus Metrics

**Goal:** Standard observability for production operators.

- `github.com/prometheus/client_golang`
- Metrics: `certwatch_certificates_total`, `certwatch_domains_total`, `certwatch_scan_duration_seconds` (histogram), `certwatch_scan_errors_total`, `certwatch_certificates_expiring_total{threshold="30"}`
- `GET /metrics` endpoint
- Updated after each scan and on startup

### 12.2 Logging Improvements

**Goal:** Production-grade observability and debugging — correlation, auditability, performance insight.

**Request ID tracing:**
- Every request gets a unique `request_id` (nanoid) injected by middleware
- Passed through to all log lines during that request lifecycle
- Format: `request_id=cw_a1b2c3d4 method=GET path=/api/domains status=200 duration=12ms`
- New file: `internal/middleware/requestid.go`

**Slow query logging:**
- Database queries exceeding a configurable threshold logged with SQL text and parameters
- Config: `database.slow_query_threshold: "200ms"` in YAML or `CERTWATCH_DATABASE_SLOW_QUERY_THRESHOLD` env var
- Zero value (0) disables slow query logging

**Audit log:**
- Security-relevant events written with `"audit": true` in structured output
- Events: user registration, login (with IP), API key generation/revocation, domain deletion, token validation failures
- Filterable: `jq 'select(.audit==true)'` or `grep '"audit":true'`

**Runtime log level:**
- `POST /api/admin/log-level` with `{"level": "debug"}` (auth required)
- Also supports `SIGHUP` signal to reload log level from config

**Periodic stats log:**
- Background goroutine logs summary at configurable interval (default 1h):
  `"service stats: domains=50 enabled=48 certificates=142 valid=130 expiring_30d=5 expired=3 errors=4 uptime=3h"`

**Config consolidation:**
```yaml
logging:
  level: info
  format: json
  slow_query_threshold: 100ms
  stats_interval: 1h
  audit: true
```

### 12.3 PostgreSQL Support

**Goal:** Production-grade database with managed service compatibility (RDS, Cloud SQL).

- Add `github.com/jackc/pgx/v5`
- `internal/database/dialect.go` — abstract SQL differences:
  - `$N` vs `?` placeholders
  - `SERIAL` vs `INTEGER PRIMARY KEY AUTOINCREMENT`
  - `RETURNING id` vs `LastInsertId`
- Config: `driver: postgres`, `dsn: "postgres://user:pass@host:5432/certwatch"`
- Migration SQL files per driver (shared schema, dialect-specific syntax)

### 12.4 Helm Chart

**Goal:** Deploy to Kubernetes with `helm install`.

- `deploy/helm/certwatch/` — `Chart.yaml`, `values.yaml`, templates (deployment, service, ingress, PVC, configmap, secret, HPA)
- Values: replica count, image ref, config, Postgres/SQLite switch, ingress host + TLS, resources

### 12.5 Pre-built Docker Images

**Goal:** Pull from registry instead of building from source.

- GitHub Actions: `.github/workflows/docker-publish.yml`
- Multi-arch (`linux/amd64`, `linux/arm64`)
- Tags: `latest`, `v0.1.0`, `sha-<short>`
- Push to `ghcr.io/araujofrancisco/certwatch`

### 12.6 Detailed Health Endpoint

**Goal:** Richer `/health` for load balancers and operators.

```json
{
  "status": "ok",
  "db": "ok",
  "version": "0.1.0",
  "uptime_seconds": 3600,
  "certificates": { "total": 142, "valid": 130, "expiring_30d": 5, "expired": 3, "errors": 4 },
  "domains": { "total": 50, "enabled": 48 },
  "last_scan": "2026-07-07T12:00:00Z"
}
```

---

## Phase 13 — Polish

### 13.1 CLI Subcommands

**Goal:** Interactive scripting without HTTP boilerplate.

- `certwatch server` — start HTTP server (current default)
- `certwatch list-domains [--q=...] [--enabled]` — query DB, print table
- `certwatch list-certs [--expiring=30] [--status=valid]`
- `certwatch scan <domain-id>` — trigger scan, print result
- `certwatch export [--format=csv]` — export to stdout
- `certwatch version` — print version
- `certwatch mcp` — MCP server (Phase 11.1)
- Uses `github.com/spf13/cobra`; reuses service layer directly (no HTTP)

### 13.2 Server-side CSV Export

**Goal:** API consumers download CSV without client-side conversion.

- `GET /api/domains/export?format=csv&q=...&enabled=...`
- `GET /api/certificates/export?format=csv&status=expiring&expiring=30`
- Uses Go `encoding/csv`, streams response (no in-memory buffering)
- Same filter params as list endpoints

### 13.3 Dashboard Filter UX

**Goal:** Server-side group and tag filters in the web UI.

- Add `?group=` and `?tags=` filter params to `GET /api/domains` (tag filter uses AND logic)
- UI dropdowns for group and tags on domains and certificates pages
- Update `DomainFilter` struct with `Group` and `Tags` fields

### 13.4 JSON-LD Structured Summaries

**Goal:** Downstream AI pipelines consume cert data as linked data.

- Add `@context: "https://schema.org"` to report and certificate responses
- Minimal — just the context field, no heavy RDF

---

## New files summary

| Feature | New files |
|---------|-----------|
| API Key Auth | `internal/api/api_keys.go`, `internal/repository/api_keys.go` |
| Webhooks | `internal/notifier/webhook.go` |
| MCP Server | `internal/mcp/` (package, ~5 files) |
| Event Stream | `internal/api/events.go` |
| Semantic Search | handler + FTS5 migration |
| Prometheus Metrics | `internal/metrics/metrics.go` |
| Request ID | `internal/middleware/requestid.go` |
| Audit Log | `internal/logging/audit.go` |
| PostgreSQL | `internal/database/dialect.go` |
| Helm Chart | `deploy/helm/certwatch/` (~8 files) |
| Docker Publish | `.github/workflows/docker-publish.yml` |
| CLI Subcommands | `cmd/certwatch/` (split into ~6 files) |
| Server Export | `internal/api/export.go` |

```mermaid
gantt
    title CertWatch Roadmap
    dateFormat  YYYY-MM-DD
    section Phase 10
    API Keys, Webhooks, Pagination, Version    :p10, 2026-07-15, 14d
    section Phase 11
    MCP, SSE, OpenAPI Enrich, Semantic Search  :p11, after p10, 21d
    section Phase 12
    Metrics, Logging, Postgres, Helm, Docker   :p12, after p11, 28d
    section Phase 13
    CLI, Export, Filters, JSON-LD              :p13, after p12, 14d
```

---

## Notes

- **SQLite remains the default** throughout — PostgreSQL is opt-in.
- **No breaking API changes** in any phase.
- **No required database migrations** for existing deployments (except FTS5 for semantic search, which is optional).
- Each phase builds on the previous but can be implemented independently.
