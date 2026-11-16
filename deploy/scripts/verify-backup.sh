#!/usr/bin/env bash
# =============================================================================
# PEMIRA 2026 - Verify a backup artifact without touching production
# =============================================================================
# Usage: ./verify-backup.sh <backup-file>
# Checks: file exists, checksum matches (if sidecar present), pg_restore can
# list the archive, and required election tables are present in the TOC.
# Set VERIFY_RESTORE_DB to an empty scratch database name to also perform a
# full restore (requires DATABASE_URL/admin PG* access).
# =============================================================================
set -euo pipefail

BACKUP_FILE="${1:-}"
[ -n "$BACKUP_FILE" ] || { echo "usage: $0 <backup-file>"; exit 2; }
[ -f "$BACKUP_FILE" ] || { echo "ERROR: not found: $BACKUP_FILE"; exit 2; }

WORK_FILE="$BACKUP_FILE"
TMP_DECRYPT=""
cleanup() { [ -n "$TMP_DECRYPT" ] && rm -f "$TMP_DECRYPT"; }
trap cleanup EXIT

case "$BACKUP_FILE" in
  *.gpg)
    command -v gpg >/dev/null 2>&1 || { echo "ERROR: gpg required"; exit 1; }
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
  [ "$expected" = "$actual" ] || { echo "FAIL: checksum mismatch"; exit 1; }
  echo "PASS: checksum"
fi

TOC="$(mktemp)"
trap 'cleanup; rm -f "$TOC"' EXIT
pg_restore --list "$WORK_FILE" > "$TOC" 2>/dev/null || { echo "FAIL: unreadable archive"; exit 1; }
echo "PASS: archive readable"

for table in voters chairs candidate_groups ballots receipt_commitments election_configs audit_logs admin_users; do
  grep -q "TABLE DATA public ${table}" "$TOC" || { echo "FAIL: missing table data for ${table}"; exit 1; }
done
echo "PASS: required tables present"

if [ -n "${VERIFY_RESTORE_DB:-}" ]; then
  echo "Performing full restore into ${VERIFY_RESTORE_DB} ..."
  pg_restore --dbname="$VERIFY_RESTORE_DB" --clean --if-exists --no-owner --no-privileges \
    --single-transaction < "$WORK_FILE"
  psql "$VERIFY_RESTORE_DB" -v ON_ERROR_STOP=1 -c \
    "SELECT (SELECT count(*) FROM voters) AS voters, (SELECT count(*) FROM ballots) AS ballots;"
  echo "PASS: full restore"
fi

echo "Backup verification succeeded."
