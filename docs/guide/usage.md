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
      type: immediate            # immediate | daily-digest | weekly-digest
      recipients:
        - ops@example.com
      thresholds: [30, 14, 7, 3, 1]  # days before expiry (immediate only)
    - name: Management
      enabled: true
      type: daily-digest
      recipients:
        - manager@example.com
      send_at: "08:00"           # HH:MM 24-hour (digest types)
    - name: Security
      enabled: true
      type: weekly-digest
      recipients:
        - security@example.com
      send_at: "09:00"
      day: Monday               # weekday name (weekly) or 1-28 (monthly)
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
| `CERTWATCH_SMTP_HOST` | `notifications.smtp.host` | `""` |
| `CERTWATCH_SMTP_PORT` | `notifications.smtp.port` | `587` |
| `CERTWATCH_SMTP_USERNAME` | `notifications.smtp.username` | `""` |
| `CERTWATCH_SMTP_PASSWORD` | `notifications.smtp.password` | `""` |
| `CERTWATCH_SMTP_FROM` | `notifications.smtp.from` | `""` |
| `CERTWATCH_SMTP_FORCE_TLS` | `notifications.smtp.force_tls` | `false` |

Environment variables take precedence over the config file.

## REST API

All API endpoints (except `/health` and auth endpoints) require a `Bearer` token in the `Authorization` header.

Auth endpoints (`/api/auth/login`, `/api/auth/register`) are rate-limited to **10 requests per minute per IP**. All request bodies are limited to **1 MB**.

### Health
```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

The health endpoint also verifies database connectivity — returns **503 Service Unavailable** if the database is unreachable.

### Auth
```bash
# Register
curl -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"secret123","name":"Admin"}'

# Login (returns JWT token)
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"secret123"}'
```

### Domains
```bash
# Add domain
curl -X POST http://localhost:8080/api/domains \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"domain":"example.com","description":"My site"}'

# List domains
curl http://localhost:8080/api/domains \
  -H "Authorization: Bearer <token>"

# Get domain
curl http://localhost:8080/api/domains/1 \
  -H "Authorization: Bearer <token>"

# Delete domain
curl -X DELETE http://localhost:8080/api/domains/1 \
  -H "Authorization: Bearer <token>"

# Trigger HTTPS scan
curl -X POST http://localhost:8080/api/domains/1/scan \
  -H "Authorization: Bearer <token>"
```

### Certificates
```bash
# List all certificates
curl http://localhost:8080/api/certificates \
  -H "Authorization: Bearer <token>"

# List certificates for a domain
curl http://localhost:8080/api/domains/1/certificates \
  -H "Authorization: Bearer <token>"
```

## Web UI

The built-in web dashboard is served via Go embed — no separate build step required. Access it at `http://localhost:8080/`.

### Pages

| Route | Description |
|-------|-------------|
| `/login` | Sign in with email/password (returns JWT stored in localStorage) |
| `/register` | Create a new account |
| `/dashboard` | Summary cards (healthy/warning/expired counts) + expiring certificates table |
| `/domains` | Domain list with add/scan/delete actions |
| `/domains/{id}` | Domain detail page with full certificate history |
| `/certificates` | All certificates sorted by days until expiry |
| `/reports` | Inventory view with links to JSON/CSV/metrics endpoints |

The UI communicates with the same REST API endpoints documented above, using the JWT token stored in `localStorage` after login.

## Health check

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

The binary also supports a `-health` flag for Docker health checks:

```bash
./certwatch -health     # exit 0 if healthy, 1 otherwise (5s timeout)
```

The health check respects `CERTWATCH_SERVER_PORT` (or reads the port from the YAML config as a fallback). Useful when running on a non-default port:
```bash
CERTWATCH_SERVER_PORT=9090 ./certwatch -health
```

## Verifying startup

On a successful start you should see:

```
INFO starting certwatch version=0.1.0
INFO database connected driver=sqlite
INFO running database migrations
INFO database migrations complete
INFO starting notification scheduler
INFO listening addr=0.0.0.0:8080
```

## Logs

Set `logging.format: json` for structured JSON output (recommended for Docker). Log level hierarchy: `debug` < `info` < `warn` < `error`.

## Background scans

The server periodically scans all enabled domains for HTTPS certificates. The interval is configured via `discovery.scan_interval` (default: 6h). Scans are also triggered per-domain via the `POST /api/domains/{id}/scan` endpoint.

## Notifications

Notifications are configured entirely via YAML. Manage domains through the REST API or the web dashboard. When SMTP is configured (host non-empty), the server:

- Checks every minute for certificates near expiry (immediate profiles), with **in-memory dedup** — each certificate+threshold combination triggers at most one alert (resets on restart)
- Sends daily summaries at the configured `send_at` time (daily-digest profiles)
- Sends weekly reports at the configured day and time (weekly-digest profiles)

All schedules use `America/New_York` timezone. If SMTP is not configured, notifications are skipped with a log warning.

The `send_at` field expects `HH:MM` format (24-hour). For `weekly-digest`, `day` accepts weekday names (`Monday`, `Tuesday`, etc.).

Set `smtp.force_tls: true` to require an explicit TLS connection to the SMTP server before authentication (recommended for public SMTP servers).
