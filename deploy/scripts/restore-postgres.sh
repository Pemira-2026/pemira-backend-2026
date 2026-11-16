#!/usr/bin/env bash
# =============================================================================
# PEMIRA 2026 - PostgreSQL restore (DESTRUCTIVE; requires explicit confirmation)
# =============================================================================
# Usage:
#   ./restore-postgres.sh /var/backups/pemira/pemira-20260101T013000Z.dump.gpg
#   CONFIRM_RESTORE=pemira-restore ./restore-postgres.sh <file> --yes
#
# Restores into the database selected by DATABASE_URL / PG* variables.
# Decrypts .gpg automatically when found. Always test on a staging database
# first: see BACKUP_RESTORE.md.
# =============================================================================
set -euo pipefail

BACKUP_FILE="${1:-}"
CONFIRM="${2:-}"
[ -n "$BACKUP_FILE" ] || { echo "usage: $0 <backup-file> [--yes]"; exit 2; }
[ -f "$BACKUP_FILE" ] || { echo "ERROR: backup file not found: $BACKUP_FILE"; exit 2; }

if [ "$CONFIRM" != "--yes" ] && [ "${CONFIRM_RESTORE:-}" != "pemira-restore" ]; then
  echo "Refusing destructive restore."
  echo "Re-run with --yes or CONFIRM_RESTORE=pemira-restore after verifying the target database."
  exit 2
fi

WORK_FILE="$BACKUP_FILE"
TMP_DECRYPT=""
cleanup() { [ -n "$TMP_DECRYPT" ] && rm -f "$TMP_DECRYPT"; }
trap cleanup EXIT

case "$BACKUP_FILE" in
  *.gpg)
    TMP_DECRYPT="$(mktemp)"
    gpg --batch --yes --output "$TMP_DECRYPT" --decrypt "$BACKUP_FILE"
    WORK_FILE="$TMP_DECRYPT"
    ;;
esac

sidecar_for() {
  case "$1" in
    *.gpg) printf '%s.sha256\n' "${1%.gpg}" ;;
    *)     printf '%s.sha256\n' "$1" ;;
  esac
}

SIDECAR="$(sidecar_for "$BACKUP_FILE")"
if [ -f "$SIDECAR" ]; then
  expected="$(cat "$SIDECAR")"
  actual="$(sha256sum "$WORK_FILE" | awk '{print $1}')"
  [ "$expected" = "$actual" ] || { echo "ERROR: checksum mismatch"; exit 1; }
  echo "Checksum OK"
fi

echo "Target: ${PGDATABASE:-${DATABASE_URL:+DATABASE_URL}}"
echo "Restoring $WORK_FILE ..."

if [ -n "${POSTGRES_CONTAINER:-}" ]; then
  docker exec -i -e PGPASSWORD="${PGPASSWORD:-}" "$POSTGRES_CONTAINER" \
    pg_restore -U "${PGUSER:-pemira_app}" -d "${PGDATABASE:-pemira_db}" \
    --clean --if-exists --no-owner --no-privileges --single-transaction < "$WORK_FILE"
elif [ -n "${DATABASE_URL:-}" ]; then
  pg_restore "$DATABASE_URL" --clean --if-exists --no-owner --no-privileges \
    --single-transaction < "$WORK_FILE"
else
  pg_restore --clean --if-exists --no-owner --no-privileges --single-transaction < "$WORK_FILE"
fi

echo "Restore completed. Run verification queries in BACKUP_RESTORE.md."
