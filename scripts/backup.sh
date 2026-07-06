#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

BACKUP_DIR="${BACKUP_DIR:-$PROJECT_DIR/backups}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
TIMESTAMP="$(date +%Y-%m-%dT%H-%M-%S)"
BACKUP_NAME="certwatch-${TIMESTAMP}"

mkdir -p "$BACKUP_DIR"

# Detect running container
CONTAINER=""
if docker compose ps -q certwatch 2>/dev/null | grep -q .; then
  CONTAINER="$(docker compose ps -q certwatch)"
elif docker ps --filter "name=certwatch" --format '{{.Names}}' | grep -q .; then
  CONTAINER="$(docker ps --filter "name=certwatch" --format '{{.Names}}' | head -1)"
fi
# Also check by container name directly
if [ -z "$CONTAINER" ]; then
  CONTAINER="$(docker ps -a --filter "name=certwatch" --format '{{.ID}}' | head -1)"
fi

cleanup() { rm -f /tmp/"$BACKUP_NAME"* /tmp/certwatch-config-*; }
trap cleanup EXIT

if [ -n "$CONTAINER" ]; then
  echo "Backing up CertWatch via Docker container: $CONTAINER"

  # Helper: copy DB + WAL files using docker cp (works with scratch images)
  copy_db() {
    local src="$1" dst="$2"
    docker cp "$CONTAINER":"$src/certwatch.db" "$dst/${BACKUP_NAME}.db" 2>/dev/null || return 1
    docker cp "$CONTAINER":"$src/certwatch.db-wal" "$dst/${BACKUP_NAME}.db-wal" 2>/dev/null || true
    docker cp "$CONTAINER":"$src/certwatch.db-shm" "$dst/${BACKUP_NAME}.db-shm" 2>/dev/null || true
    return 0
  }

  tar_files() {
    local dir="$1" name="$2" config="$3"
    local files=("${name}.db")
    [ -f "$dir/${name}.db-wal" ] && files+=("${name}.db-wal")
    [ -f "$dir/${name}.db-shm" ] && files+=("${name}.db-shm")
    [ -n "$config" ] && [ -f "$dir/$config" ] && files+=("$config")
    tar czf "$BACKUP_DIR/${name}.tar.gz" -C "$dir" "${files[@]}"
  }

  # Direct-write path: /backups volume mounted, DB visible on host
  if [ -d "$BACKUP_DIR" ] && \
     docker inspect "$CONTAINER" --format '{{range .Mounts}}{{if eq .Destination "/backups"}}{{.Source}}{{end}}{{end}}' 2>/dev/null | grep -q .; then
    echo "Direct backup volume detected — DB visible on host at $BACKUP_DIR"
    copy_db "/data" "$BACKUP_DIR"
    docker cp "$CONTAINER":/config/default.yaml "$BACKUP_DIR/${BACKUP_NAME}.yaml" 2>/dev/null || true
    tar_files "$BACKUP_DIR" "$BACKUP_NAME" "${BACKUP_NAME}.yaml"
    rm -f "$BACKUP_DIR/${BACKUP_NAME}.db" "$BACKUP_DIR/${BACKUP_NAME}.db-wal" \
          "$BACKUP_DIR/${BACKUP_NAME}.db-shm" "$BACKUP_DIR/${BACKUP_NAME}.yaml" 2>/dev/null || true
  else
    echo "Copying database and config from container to temp"
    copy_db "/data" "/tmp"
    docker cp "$CONTAINER":/config/default.yaml /tmp/certwatch-config.yaml 2>/dev/null || true
    tar_files "/tmp" "$BACKUP_NAME" "certwatch-config.yaml"
    rm -f /tmp/"${BACKUP_NAME}".db /tmp/"${BACKUP_NAME}".db-wal /tmp/"${BACKUP_NAME}".db-shm /tmp/certwatch-config.yaml 2>/dev/null || true
  fi

  if [ ! -f "$BACKUP_DIR/${BACKUP_NAME}.tar.gz" ]; then
    echo "ERROR: failed to create backup archive" >&2
    exit 1
  fi
else
  echo "Backing up CertWatch (standalone mode)"

  DB_PATH="${CERTWATCH_DATABASE_DSN:-certwatch.db}"
  CONFIG_PATH="${CERTWATCH_CONFIG:-config/default.yaml}"

  if [ ! -f "$DB_PATH" ]; then
    echo "ERROR: database not found at $DB_PATH" >&2
    exit 1
  fi

  sqlite3 "$DB_PATH" ".backup /tmp/${BACKUP_NAME}.db"
  cp "$CONFIG_PATH" /tmp/certwatch-config.yaml 2>/dev/null || true
  tar czf "$BACKUP_DIR/${BACKUP_NAME}.tar.gz" \
    -C /tmp "${BACKUP_NAME}.db" certwatch-config.yaml 2>/dev/null || \
  tar czf "$BACKUP_DIR/${BACKUP_NAME}.tar.gz" -C /tmp "${BACKUP_NAME}.db"
fi

echo "Backup created: $BACKUP_DIR/${BACKUP_NAME}.tar.gz"

# Retention cleanup
if [ "$RETENTION_DAYS" -gt 0 ]; then
  find "$BACKUP_DIR" -name 'certwatch-*.tar.gz' -mtime "+$RETENTION_DAYS" -delete 2>/dev/null || true
  echo "Removed backups older than $RETENTION_DAYS days"
fi
