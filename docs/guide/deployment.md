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

### Security
- The container uses a multi-stage scratch build (no shell, no package manager)
- All input is validated: domain format, email format, request body size (1 MB limit)
- Rate limiting on auth endpoints (10 req/min per IP)
- Parameterized SQL queries throughout (no injection risk)
- `html/template` auto-escapes all UI output
