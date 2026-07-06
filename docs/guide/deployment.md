# Deployment

## Security

The container runs as `nobody` (UID 65534) for defense in depth. Database files and directories are created with `0700` permissions. The health check has a 5-second timeout to avoid hanging.

If the container cannot write to the database volume due to user permissions, set a matching user in `docker-compose.yml` by uncommenting the `user:` line.

## Docker

### Build image
```bash
docker compose build
```

### Run container
```bash
docker compose up -d
```

This starts the service on port 8080 with the database stored in a Docker volume.

### Verify
```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

### View logs
```bash
docker compose logs -f
```

### Stop
```bash
docker compose down
```

## Docker Compose reference

The included `docker-compose.yml` configures:

- **Named volume** `certwatch-data` for persistent SQLite storage
- **Config mount** at `/config` (read-only) — edit `config/default.yaml` locally
- **Environment overrides** for Docker paths (`CERTWATCH_DATABASE_DSN=/data/certwatch.db`, `CERTWATCH_LOGGING_FORMAT=json`)
- **Health check** every 30s via `certwatch -health` (respects `CERTWATCH_SERVER_PORT` if set)
- **Auto-restart** unless stopped manually
- **User**: commented out (`user: "1000:1000"`) — uncomment if the container runs as non-root and needs volume write access

## Production considerations

### Database
- **SQLite**: Default. Back up the `.db` file. Not suitable for concurrent write-heavy workloads. Use a persistent volume in Docker.
- **PostgreSQL**: Planned for Phase 2.

### Environment
- Timezone defaults to America/New_York for digest scheduling (configurable per profile)
- Log in JSON format (`CERTWATCH_LOGGING_FORMAT=json`) for production to integrate with log aggregators
- Use environment variables for secrets (never commit them)

