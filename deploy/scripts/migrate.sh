#!/usr/bin/env bash
# =============================================================================
# PEMIRA 2026 - Apply SQL migrations in order (production pre-boot step)
# =============================================================================
# Usage:
#   DATABASE_URL=postgresql://... ./migrate.sh
#   POSTGRES_CONTAINER=pemira_postgres ./migrate.sh
#
# The SQL files are idempotent (CREATE TABLE/INDEX IF NOT EXISTS). This script
# still records applied versions in schema_migrations so operators can audit
# what ran and when. Run BEFORE starting the API, and keep DB_AUTO_MIGRATE=false.
# =============================================================================
set -euo pipefail

MIGRATIONS_DIR="${MIGRATIONS_DIR:-$(cd "$(dirname "$0")/../../internal/database/migrations" && pwd)}"
[ -d "$MIGRATIONS_DIR" ] || { echo "ERROR: migrations dir not found: $MIGRATIONS_DIR"; exit 2; }

PSQL=(psql -v ON_ERROR_STOP=1)
if [ -n "${POSTGRES_CONTAINER:-}" ]; then
  PSQL=(docker exec -i -e PGPASSWORD="${PGPASSWORD:-}" "$POSTGRES_CONTAINER" psql -U "${PGUSER:-pemira_app}" -d "${PGDATABASE:-pemira_db}" -v ON_ERROR_STOP=1)
elif [ -n "${DATABASE_URL:-}" ]; then
  PSQL=(psql "$DATABASE_URL" -v ON_ERROR_STOP=1)
fi

"${PSQL[@]}" <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     VARCHAR(255) PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
SQL

shopt -s nullglob
for file in "$MIGRATIONS_DIR"/*.sql; do
  version="$(basename "$file")"
  applied="$("${PSQL[@]}" -tAc "SELECT 1 FROM schema_migrations WHERE version = '${version}'" | tr -d '[:space:]')"
  if [ "$applied" = "1" ]; then
    echo "SKIP ${version} (already applied)"
    continue
  fi
  echo "APPLY ${version}"
  if [ -n "${POSTGRES_CONTAINER:-}" ]; then
    docker exec -i -e PGPASSWORD="${PGPASSWORD:-}" "$POSTGRES_CONTAINER" \
      psql -U "${PGUSER:-pemira_app}" -d "${PGDATABASE:-pemira_db}" -v ON_ERROR_STOP=1 \
      --single-transaction < "$file"
  elif [ -n "${DATABASE_URL:-}" ]; then
    psql "$DATABASE_URL" -v ON_ERROR_STOP=1 --single-transaction -f "$file"
  else
    psql -v ON_ERROR_STOP=1 --single-transaction -f "$file"
  fi
  "${PSQL[@]}" -c "INSERT INTO schema_migrations(version) VALUES ('${version}')" >/dev/null
done

echo "Migrations complete."
"${PSQL[@]}" -c "SELECT version, applied_at FROM schema_migrations ORDER BY version;"
