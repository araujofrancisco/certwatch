# Deployment

## Docker

### Build image
```bash
make docker-build
```

### Run container
```bash
make docker-run
```

This starts the service on port 8080 with the database stored in a Docker volume.

### Verify
```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

### View logs
```bash
make docker-logs
```

### Stop
```bash
make docker-stop
```

## Docker Compose reference

The included `docker-compose.yml` configures:

- **Named volume** `certwatch-data` for persistent SQLite storage
- **Config mount** at `/config` (read-only) — edit `config/default.yaml` locally
- **Environment overrides** for Docker paths (`CERTWATCH_DATABASE_DSN=/data/certwatch.db`, `CERTWATCH_LOGGING_FORMAT=json`)
- **Health check** every 30s via `certwatch -health`
- **Auto-restart** unless stopped manually

## Production considerations

### Database
- **SQLite**: Default. Back up the `.db` file. Not suitable for concurrent write-heavy workloads. Use a persistent volume in Docker.

### Environment
- Timezone defaults to America/New_York for digest scheduling (configurable per profile)
- Log in JSON format (`CERTWATCH_LOGGING_FORMAT=json`) for production to integrate with log aggregators
- Use environment variables for secrets (never commit them)
- Override `CERTWATCH_AUTH_SECRET` with a strong random value — the binary warns on startup if the default is detected
- Set `CERTWATCH_SERVER_CORS_ORIGINS` to your dashboard URL(s) — e.g. `https://certwatch.example.com`. Multiple origins: comma-separated

## Backup and restore

### Prerequisites
- `sqlite3` CLI tool installed on the host
- Docker environment or standalone binary

### Backup
```bash
make backup
```

Creates a timestamped archive in `backups/`:
```
backups/
└── certwatch-2026-07-06T12-00-00.tar.gz
```

The backup script:
1. Detects whether CertWatch is running in Docker or standalone
2. Uses `sqlite3 .backup` for a safe online snapshot (no downtime)
3. Includes the database + config file
4. Removes backups older than 30 days (configurable via `RETENTION_DAYS`)

**Environment variables:**

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKUP_DIR` | `backups/` (project root) | Where backups are stored |
| `RETENTION_DAYS` | `30` | Delete backups older than N days (0 = keep forever) |

**Direct host write (Docker only):**

Uncomment the backup volume in `docker-compose.yml`:
```yaml
volumes:
  # - ./backups:/backups   ← uncomment this line
```

When `/backups` is mounted inside the container, the backup script writes directly there, avoiding the `docker cp` step. The backup directory is then on the host filesystem.

### Restore
```bash
make restore              # interactive — lists backups and prompts
make restore certwatch-2026-07-06T12-00-00.tar.gz   # direct by filename
make restore 1            # or by index number from the list
```

The restore script:
1. Lists all available backups (or uses the provided filename/number)
2. Confirms before overwriting
3. Stops the CertWatch service
4. Copies the database + config back
5. Starts the service and waits for health check

### Manual backup

```bash
# Default location
scripts/backup.sh

# Custom location
BACKUP_DIR=/mnt/nfs/backups scripts/backup.sh

# Keep backups forever
RETENTION_DAYS=0 scripts/backup.sh
```

### Manual restore

```bash
scripts/restore.sh                    # interactive
scripts/restore.sh certwatch-2026-07-06T12-00-00.tar.gz
```

### Cron (optional)

Add to crontab for daily automated backups:
```cron
0 2 * * * /path/to/certwatch/scripts/backup.sh
```

### Security
- The container uses a multi-stage scratch build (no shell, no package manager)
- All input is validated: domain format, email format, request body size (1 MB limit)
- Rate limiting on auth endpoints (10 req/min per IP, sliding window, port-stripped)
- CORS restricted to configurable origins (default `http://localhost:8080`) — set `CERTWATCH_SERVER_CORS_ORIGINS` for custom domains
- Security headers sent on all responses: CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy
- Password minimum 8 characters on registration
- Input length limits: description ≤500, group ≤100
- Registration errors are generic — no email enumeration
- Parameterized SQL queries throughout (no injection risk)
- `html/template` auto-escapes all UI output
