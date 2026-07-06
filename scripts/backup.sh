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

cleanup() { rm -f /tmp/"$BACKUP_NAME"* /tmp/certwatch-config-*; }
trap cleanup EXIT

if [ -n "$CONTAINER" ]; then
  echo "Backing up CertWatch via Docker container: $CONTAINER"

  if docker exec "$CONTAINER" test -d /backups 2>/dev/null; then
    echo "Direct backup volume detected — DB visible on host at $BACKUP_DIR"
    docker exec "$CONTAINER" sqlite3 /data/certwatch.db ".backup /backups/${BACKUP_NAME}.db"
    docker cp "$CONTAINER":/config/default.yaml "$BACKUP_DIR/${BACKUP_NAME}.yaml" 2>/dev/null || true
    (cd "$BACKUP_DIR" && tar czf "${BACKUP_NAME}.tar.gz" "${BACKUP_NAME}.db" "${BACKUP_NAME}.yaml" 2>/dev/null) || \
      (cd "$BACKUP_DIR" && tar czf "${BACKUP_NAME}.tar.gz" "${BACKUP_NAME}.db")
  else
    echo "Writing backup to temp location and copying out"
    docker exec "$CONTAINER" sqlite3 /data/certwatch.db ".backup /tmp/${BACKUP_NAME}.db"
    docker cp "$CONTAINER":/tmp/"${BACKUP_NAME}".db /tmp/
    docker cp "$CONTAINER":/config/default.yaml /tmp/certwatch-config.yaml 2>/dev/null || true
    docker exec "$CONTAINER" rm -f "/tmp/${BACKUP_NAME}.db"
    (cd /tmp && tar czf "$BACKUP_DIR/${BACKUP_NAME}.tar.gz" "${BACKUP_NAME}.db" certwatch-config.yaml 2>/dev/null) || \
      (cd /tmp && tar czf "$BACKUP_DIR/${BACKUP_NAME}.tar.gz" "${BACKUP_NAME}.db")
  fi

  if [ ! -f "$BACKUP_DIR/${BACKUP_NAME}.tar.gz" ]; then
    echo "ERROR: failed to create backup archive" >&2
    rm -f "$BACKUP_DIR/${BACKUP_NAME}.db" "$BACKUP_DIR/${BACKUP_NAME}.yaml" 2>/dev/null || true
    exit 1
  fi
  rm -f "$BACKUP_DIR/${BACKUP_NAME}.db" "$BACKUP_DIR/${BACKUP_NAME}.yaml" 2>/dev/null || true
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
