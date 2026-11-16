# Database Migrations (PEMIRA 2026)

PostgreSQL 16 DDL for the zero-linkability schema.

| File | Contents |
| --- | --- |
| `000001_init_schema.sql` | voters, chairs, candidate_groups, ballots, receipt_commitments, election_configs, audit_logs + indexes |
| `000002_create_admin_users.sql` | admin_users + indexes |

## Rules

- Forward-only. Never edit an applied migration; add a new numbered file.
- All statements must be idempotent (`IF NOT EXISTS`) so reruns are safe.
- Apply **before** starting the API in production, then keep
  `DB_AUTO_MIGRATE=false`.
- The API verifies required tables at startup and refuses to boot if missing.

## Apply

```bash
# From the repo:
cd deploy
DATABASE_URL='postgresql://user:pass@host:5432/pemira_db?sslmode=require' ./scripts/migrate.sh

# Or inside Docker:
POSTGRES_CONTAINER=pemira_postgres PGUSER=pemira_app PGDATABASE=pemira_db ./scripts/migrate.sh
```

`migrate.sh` records applied files in `schema_migrations(version, applied_at)`.

## Development

`DB_AUTO_MIGRATE=true` is the development default (GORM AutoMigrate). This is
for local convenience only; production uses the SQL files above.

## Emergency changes

Any manual DDL during the election requires a written incident record signed by
KPU and Bawaslu, a fresh backup, and a paired migration file committed
afterwards so the schema remains reproducible.
