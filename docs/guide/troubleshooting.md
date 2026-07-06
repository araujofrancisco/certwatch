# Troubleshooting

## Startup failures

### "open database: ... no such file or directory"
The directory for the SQLite database path does not exist. CertWatch will create the directory **when using a relative path** (e.g. `certwatch.db`) but if you use an absolute path like `/data/certwatch.db`, the `/data` directory must exist.

**Fix**: Use a relative DSN, or ensure the parent directory exists before starting. Docker Compose uses `/data/certwatch.db` with a named volume — the volume mount creates the directory automatically.

### Port already in use
```
listen tcp 0.0.0.0:8080: bind: address already in use
```

**Fix**: Change the port via config or `CERTWATCH_SERVER_PORT`, or stop the process using the port:
```bash
lsof -i :8080
kill <PID>
```

### Config file not found
```
read config file: config/default.yaml: no such file or directory
```

**Fix**: Run from the project root, or set `CERTWATCH_CONFIG` to an absolute path.

## Runtime issues

### Health check failing in Docker
The health check runs `certwatch -health` which connects to the health endpoint (default `http://localhost:8080/health`, configurable via `CERTWATCH_SERVER_PORT`). If the container isn't ready yet, the check may fail temporarily.

**Troubleshoot**:
1. Check container logs: `docker compose logs certwatch`
2. Verify the port mapping: `docker compose ps`
3. Ensure the server is listening on `0.0.0.0` (not `127.0.0.1`) so it accepts connections from outside the container
4. If you changed `CERTWATCH_SERVER_PORT`, verify the health check uses the same port

### Database errors after unclean shutdown
SQLite can enter a recovery state after a crash. CertWatch uses `CREATE TABLE IF NOT EXISTS`, so migrations are idempotent. If the database file is corrupted:

```bash
rm certwatch.db   # delete (backup first) and restart
```

Or from Docker:
```bash
docker compose down
docker volume rm certwatch_certwatch-data
docker compose up -d
```

## Common Docker issues

### Container exits immediately
```bash
docker compose logs certwatch
```

Common causes: invalid config file, database path not writable (container runs as `nobody` — check volume/directory permissions), port conflict on the host. Check logs first:
```bash
make run              # development
docker compose logs   # Docker
```
