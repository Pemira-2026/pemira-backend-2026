#!/usr/bin/env bash
# =============================================================================
# PEMIRA 2026 - PostgreSQL backup (custom format, checksum, optional GPG+offsite)
# =============================================================================
# Required: DATABASE_URL or PG* variables, and either pg_dump on PATH or
#           POSTGRES_CONTAINER set to the postgres container name.
# Optional: BACKUP_DIR, RETENTION_DAYS, GPG_RECIPIENT, RCLONE_REMOTE,
#           LOG_FILE.
# Never prints the database password.
# =============================================================================
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/var/backups/pemira}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
BASE="pemira-${STAMP}"
LOG_FILE="${LOG_FILE:-/var/log/pemira/backup.log}"

log() { printf '%s %s\n' "$(date -u +%FT%TZ)" "$*" | tee -a "$LOG_FILE"; }
fail() { log "ERROR: $*"; exit 1; }

mkdir -p "$BACKUP_DIR" "$(dirname "$LOG_FILE")"
umask 077

DUMP_PATH="${BACKUP_DIR}/${BASE}.dump"
[ -e "$DUMP_PATH" ] && fail "refusing to overwrite ${DUMP_PATH}"

log "Starting backup ${BASE}"

if [ -n "${POSTGRES_CONTAINER:-}" ]; then
  log "Backup source: container ${POSTGRES_CONTAINER}"
  docker exec -e PGPASSWORD="${PGPASSWORD:-}" "$POSTGRES_CONTAINER" \
    pg_dump -U "${PGUSER:-pemira_app}" -d "${PGDATABASE:-pemira_db}" \
    --format=custom --no-owner --no-privileges > "$DUMP_PATH"
else
  command -v pg_dump >/dev/null 2>&1 || fail "pg_dump not found and POSTGRES_CONTAINER unset"
  log "Backup source: direct connection"
  if [ -n "${DATABASE_URL:-}" ]; then
    pg_dump "$DATABASE_URL" --format=custom --no-owner --no-privileges > "$DUMP_PATH"
  else
    pg_dump --format=custom --no-owner --no-privileges > "$DUMP_PATH"
  fi
fi

[ -s "$DUMP_PATH" ] || fail "backup file is empty"

# Structural verification before we trust the artifact. Prefer pg_restore on the
# host; fall back to the postgres container when only that has the client tools.
if command -v pg_restore >/dev/null 2>&1; then
  pg_restore --list "$DUMP_PATH" >/dev/null 2>&1 || fail "pg_restore could not read the backup"
elif [ -n "${POSTGRES_CONTAINER:-}" ]; then
  docker exec -i "$POSTGRES_CONTAINER" pg_restore --list < "$DUMP_PATH" >/dev/null 2>&1 \
    || fail "pg_restore (container) could not read the backup"
else
  fail "pg_restore not found; cannot verify the backup archive"
fi

sha256sum "$DUMP_PATH" | awk '{print $1}' > "${DUMP_PATH}.sha256"
log "Checksum: $(cat "${DUMP_PATH}.sha256")"

if [ -n "${GPG_RECIPIENT:-}" ]; then
  command -v gpg >/dev/null 2>&1 || fail "GPG_RECIPIENT set but gpg not installed"
  gpg --batch --yes --trust-model always -r "$GPG_RECIPIENT" \
    --output "${DUMP_PATH}.gpg" --encrypt "$DUMP_PATH"
  rm -f "$DUMP_PATH"
  DUMP_PATH="${DUMP_PATH}.gpg"
  log "Encrypted backup -> ${DUMP_PATH}"
fi

if [ -n "${RCLONE_REMOTE:-}" ]; then
  command -v rclone >/dev/null 2>&1 || fail "RCLONE_REMOTE set but rclone not installed"
  rclone copy "$DUMP_PATH" "$RCLONE_REMOTE" --immutable
  [ -f "${DUMP_PATH}.sha256" ] && rclone copy "${DUMP_PATH}.sha256" "$RCLONE_REMOTE" --immutable
  log "Copied backup to offsite ${RCLONE_REMOTE}"
fi

find "$BACKUP_DIR" -maxdepth 1 -type f -name 'pemira-*' \
  -mtime "+${RETENTION_DAYS}" -print -delete | tee -a "$LOG_FILE"

log "Backup complete: ${DUMP_PATH}"
