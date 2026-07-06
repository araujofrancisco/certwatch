# Usage

## Building

```bash
make build          # produces build/certwatch (static binary)
```

## Running

### From source
```bash
make run            # go run ./cmd/certwatch/
```

### From binary
```bash
./build/certwatch
```

## Configuration

CertWatch loads `config/default.yaml` by default. Override with `CERTWATCH_CONFIG` env var.

```yaml
server:
  host: "0.0.0.0"       # listen address
  port: 8080             # listen port

database:
  driver: sqlite         # sqlite only (postgres planned)
  dsn: "certwatch.db"

logging:
  level: info            # debug | info | warn | error
  format: text           # text | json

auth:
  secret: "change-me-in-production"  # JWT signing secret
  token_ttl: "24h"                   # token validity duration

discovery:
  scan_interval: "6h"    # background scan interval
  timeout: "30s"         # per-scan timeout

notifications:
  smtp:
    host: ""             # SMTP server (empty = skip sending)
    port: 587
    username: ""
    password: ""
    from: ""
    force_tls: false     # use explicit TLS before authentication
  profiles:
    - name: Operations
      enabled: true
      type: immediate
      recipients:
        - ops@example.com
      thresholds: [30, 14, 7, 3, 1]
    - name: Management
      enabled: true
      type: daily-digest
      recipients:
        - manager@example.com
      send_at: "08:00"
    - name: Security
      enabled: true
      type: weekly-digest
      recipients:
        - security@example.com
      send_at: "09:00"
      day: Monday
```

### Environment variables

| Variable | Overrides | Default |
|----------|-----------|---------|
| `CERTWATCH_CONFIG` | Config file path | `config/default.yaml` |
| `CERTWATCH_SERVER_HOST` | `server.host` | `0.0.0.0` |
| `CERTWATCH_SERVER_PORT` | `server.port` | `8080` |
| `CERTWATCH_DATABASE_DRIVER` | `database.driver` | `sqlite` |
| `CERTWATCH_DATABASE_DSN` | `database.dsn` | `certwatch.db` |
| `CERTWATCH_LOGGING_LEVEL` | `logging.level` | `info` |
| `CERTWATCH_LOGGING_FORMAT` | `logging.format` | `text` |
| `CERTWATCH_AUTH_SECRET` | `auth.secret` | `change-me-in-production` |
| `CERTWATCH_AUTH_TOKEN_TTL` | `auth.token_ttl` | `24h` |
| `CERTWATCH_DISCOVERY_SCAN_INTERVAL` | `discovery.scan_interval` | `6h` |
| `CERTWATCH_DISCOVERY_TIMEOUT` | `discovery.timeout` | `30s` |
| `CERTWATCH_SMTP_HOST` | `notifications.smtp.host` | `""` |
| `CERTWATCH_SMTP_PORT` | `notifications.smtp.port` | `587` |
| `CERTWATCH_SMTP_USERNAME` | `notifications.smtp.username` | `""` |
| `CERTWATCH_SMTP_PASSWORD` | `notifications.smtp.password` | `""` |
| `CERTWATCH_SMTP_FROM` | `notifications.smtp.from` | `""` |
| `CERTWATCH_SMTP_FORCE_TLS` | `notifications.smtp.force_tls` | `false` |

Environment variables take precedence over the config file.

## REST API

All API endpoints (except `/health` and auth endpoints) require a `Bearer` token in the `Authorization` header.

Auth endpoints are rate-limited to **10 requests per minute per IP**. All request bodies are limited to **1 MB**.

### Health
```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

### Auth
```bash
# Register
curl -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"CHANGE_ME","name":"Admin"}'

# Login (returns JWT token)
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"CHANGE_ME"}'
```

### Domains

Domains are automatically scanned in a background goroutine when created.

```bash
# Add domain (auto-scans in background)
curl -X POST http://localhost:8080/api/domains \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"domain":"example.com","description":"My site"}'

# List domains (with optional filters)
curl "http://localhost:8080/api/domains?q=example&enabled=true" \
  -H "Authorization: Bearer <token>"

# Get domain
curl http://localhost:8080/api/domains/1 \
  -H "Authorization: Bearer <token>"

# Delete domain (cascade deletes certificates)
curl -X DELETE http://localhost:8080/api/domains/1 \
  -H "Authorization: Bearer <token>"

# Trigger manual scan
curl -X POST http://localhost:8080/api/domains/1/scan \
  -H "Authorization: Bearer <token>"
```

### Certificates

```bash
# List all certificates (with optional filters)
curl "http://localhost:8080/api/certificates?status=valid&protocol=https&q=google&expiring=30&expired=true" \
  -H "Authorization: Bearer <token>"

# List certificates for a domain (with optional filters)
curl "http://localhost:8080/api/domains/1/certificates?status=valid" \
  -H "Authorization: Bearer <token>"
```

**Filter parameters for certificates:**
| Param | Example | Description |
|-------|---------|-------------|
| `q` | `?q=google` | Search subject + issuer (LIKE) |
| `domain_id` | `?domain_id=1` | Filter by domain |
| `status` | `?status=valid` | valid / expired / error |
| `protocol` | `?protocol=ct` | https / ct / smtp / ... |
| `expiring` | `?expiring=30` | Expiring within N days |
| `expired` | `?expired=true` | Only expired certs |

**Filter parameters for domains:**
| Param | Example | Description |
|-------|---------|-------------|
| `q` | `?q=example` | Search domain + description (LIKE) |
| `enabled` | `?enabled=true` | Filter by enabled state |

### Purge error certificates

```bash
# Delete all error certs
curl -X DELETE http://localhost:8080/api/certificates/errors \
  -H "Authorization: Bearer <token>"
# {"deleted": 3}

# Delete error certs for a specific domain
curl -X DELETE http://localhost:8080/api/domains/1/certificates/errors \
  -H "Authorization: Bearer <token>"
```

### Reports

```bash
# Full inventory report with summary stats
curl http://localhost:8080/api/reports/inventory \
  -H "Authorization: Bearer <token>"
```

## Web UI

The built-in web dashboard is served via Go embed — no separate build step required. Access it at `http://localhost:8080/`.

### Pages

| Route | Description |
|-------|-------------|
| `/login` | Sign in with email/password (returns JWT stored in localStorage) |
| `/register` | Create a new account |
| `/dashboard` | Summary cards (healthy/warning/expired counts) + expiring certificates |
| `/domains` | Domain list with search, enabled filter, scan/delete actions |
| `/domains/{id}` | Domain detail with certificate history and purge errors button |
| `/certificates` | All certificates with search, status/protocol/expiry filters |
| `/reports` | Inventory report with summary stats, client-side filters, JSON/CSV download |

The UI communicates with the same REST API endpoints using the JWT token stored in `localStorage` after login.

## Health check

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

The binary also supports a `-health` flag for Docker health checks:

```bash
./certwatch -health     # exit 0 if healthy, 1 otherwise (5s timeout)
```

## Background scans

The server periodically scans all enabled domains for certificates. The interval is configured via `discovery.scan_interval` (default: 6h). Scans are sequential — each protocol is tried in priority order (HTTPS → CT → stubs) with per-scanner timeouts. The first successful scan result is saved; if all fail, an error certificate is created.

Scans are also triggered:
- Automatically on domain creation (`POST /api/domains`)
- Manually per-domain via `POST /api/domains/{id}/scan`

## Notifications

Notifications are configured entirely via YAML. When SMTP is configured (host non-empty), the server:

- Checks every minute for certificates near expiry (immediate profiles), with **in-memory dedup** — each certificate+threshold combination triggers at most one alert (resets on restart)
- Sends daily summaries at the configured `send_at` time (daily-digest profiles)
- Sends weekly reports at the configured day and time (weekly-digest profiles)

All schedules use `America/New_York` timezone. If SMTP is not configured, notifications are skipped with a log warning.

Set `smtp.force_tls: true` to require an explicit TLS connection to the SMTP server before authentication (recommended for public SMTP servers).
