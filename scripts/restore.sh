#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

BACKUP_DIR="${BACKUP_DIR:-$PROJECT_DIR/backups}"

list_backups() {
  local files=("$BACKUP_DIR"/certwatch-*.tar.gz)
  if [ ${#files[@]} -eq 0 ] || [ ! -f "${files[0]}" ]; then
    echo "No backups found in $BACKUP_DIR" >&2
    exit 1
  fi
  echo "Available backups:"
  echo ""
  local i=1
  for f in "${files[@]}"; do
    local size
    size="$(du -h "$f" | cut -f1)"
    local mtime
    mtime="$(date -r "$f" '+%Y-%m-%d %H:%M:%S' 2>/dev/null || stat -f '%Sm' "$f" 2>/dev/null)"
    echo "  [$i] $mtime  $size  $(basename "$f")"
    i=$((i + 1))
  done
  echo ""
}

select_backup() {
  if [ -n "${1:-}" ]; then
    local file="$BACKUP_DIR/$1"
    if [ -f "$file" ]; then
      echo "$file"
      return 0
    fi
    # Maybe it's an index
    local files=("$BACKUP_DIR"/certwatch-*.tar.gz)
    if [ "$1" -ge 1 ] && [ "$1" -le "${#files[@]}" ] 2>/dev/null; then
      local idx=$(( $1 - 1 ))
      echo "${files[$idx]}"
      return 0
    fi
    echo "ERROR: backup '$1' not found" >&2
    exit 1
  fi

  list_backups
  local files=("$BACKUP_DIR"/certwatch-*.tar.gz)
  echo -n "Select backup to restore [1-${#files[@]}]: "
  read -r sel
  if [ "$sel" -lt 1 ] || [ "$sel" -gt "${#files[@]}" ] 2>/dev/null; then
    echo "Invalid selection" >&2
    exit 1
  fi
  echo "${files[$((sel - 1))]}"
}

echo "=== CertWatch Restore ==="
BACKUP_FILE="$(select_backup "${1:-}")"
echo "Selected: $(basename "$BACKUP_FILE")"
echo ""

echo -n "WARNING: This will overwrite the current database. Continue? [y/N]: "
read -r confirm
if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
  echo "Restore cancelled."
  exit 0
fi

TMPDIR="$(mktemp -d)"
trap "rm -rf $TMPDIR" EXIT

tar xzf "$BACKUP_FILE" -C "$TMPDIR"
echo "Extracted backup to $TMPDIR"

DB_FILE=""
CONFIG_FILE=""
for f in "$TMPDIR"/*.db; do
  [ -f "$f" ] && DB_FILE="$f" && break
done
for f in "$TMPDIR"/*.yaml; do
  [ -f "$f" ] && CONFIG_FILE="$f" && break
done

if [ -z "$DB_FILE" ]; then
  echo "ERROR: no database file found in backup" >&2
  exit 1
fi

# Detect Docker
CONTAINER=""
if docker compose ps -q certwatch 2>/dev/null | grep -q .; then
  CONTAINER="$(docker compose ps -q certwatch)"
elif docker ps --filter "name=certwatch" --format '{{.Names}}' | grep -q .; then
  CONTAINER="$(docker ps --filter "name=certwatch" --format '{{.Names}}' | head -1)"
fi

if [ -n "$CONTAINER" ]; then
  echo "Stopping CertWatch container..."
  docker compose stop certwatch

  echo "Restoring database..."
  docker cp "$DB_FILE" "$CONTAINER":/data/certwatch.db

  if [ -n "$CONFIG_FILE" ]; then
    echo "Restoring config..."
    docker cp "$CONFIG_FILE" "$CONTAINER":/config/default.yaml
  fi

  echo "Starting CertWatch container..."
  docker compose start certwatch

  echo "Waiting for health check..."
  for i in $(seq 1 10); do
    if docker compose exec -T certwatch /certwatch -health 2>/dev/null; then
      echo "CertWatch is healthy."
      break
    fi
    if [ "$i" -eq 10 ]; then
      echo "WARNING: health check did not pass within 10 attempts" >&2
    fi
    sleep 2
  done
else
  echo "Stopping CertWatch (standalone)..."
  PID_FILE="${CERTWATCH_PID_FILE:-}"
  if [ -n "$PID_FILE" ] && [ -f "$PID_FILE" ]; then
    kill "$(cat "$PID_FILE")" 2>/dev/null || true
    sleep 2
  else
    echo "WARNING: cannot stop CertWatch automatically. Stop it manually, then press Enter"
    read -r
  fi

  DB_PATH="${CERTWATCH_DATABASE_DSN:-certwatch.db}"
  echo "Restoring database to $DB_PATH..."
  cp "$DB_FILE" "$DB_PATH"

  if [ -n "$CONFIG_FILE" ]; then
    CONFIG_PATH="${CERTWATCH_CONFIG:-config/default.yaml}"
    echo "Restoring config to $CONFIG_PATH..."
    cp "$CONFIG_FILE" "$CONFIG_PATH"
  fi

  echo "Restore complete. Start CertWatch manually."
fi

echo ""
echo "=== Restore completed successfully ==="
