# Backup & Restore Runbook (PEMIRA 2026)

## Objectives

| Metric | Target | Mechanism |
| --- | --- | --- |
| RPO (max data loss) | 15 minutes during election day | PostgreSQL WAL archiving or cloud PITR |
| RPO (normal) | 24 hours | Nightly encrypted `pg_dump` |
| RTO (time to restore) | 60 minutes | Documented restore on warm standby host |
| Retention | 14 daily, 8 weekly, 12 monthly | systemd timer + offsite copy |

The nightly logical backup is the minimum. For election day, enable managed
PostgreSQL point-in-time recovery (Supabase/Cloud SQL) **or** WAL archiving.
A nightly dump alone cannot recover the last few hours of ballots.

## What is backed up

- PostgreSQL: all election tables (`voters`, `chairs`, `candidate_groups`,
  `ballots`, `receipt_commitments`, `election_configs`, `audit_logs`,
  `admin_users`, `schema_migrations`).
- Not backed up here: Redis (ephemeral locks/state; rebuildable), TLS keys,
  `.env.production` (store in a secret manager, not in the backup bucket).

## Daily backup

Installed by `deploy/systemd/pemira-backup.{service,timer}` at 01:30 WIB.

```bash
# Manual run
sudo systemctl start pemira-backup.service
sudo journalctl -u pemira-backup.service -n 100
tail -n 100 /var/log/pemira/backup.log
```

Configuration (`/etc/pemira/backup.env`, root-readable only):

```bash
BACKUP_DIR=/var/backups/pemira
RETENTION_DAYS=14
POSTGRES_CONTAINER=pemira_postgres     # or unset and use DATABASE_URL
PGUSER=pemira_app
PGDATABASE=pemira_db
# Optional encryption (recommended)
GPG_RECIPIENT=pemira-backup@example.ac.id
# Optional offsite copy (recommended: different provider/region)
RCLONE_REMOTE=pemira-offsite:pemira-backups
```

The script fails loudly on empty dumps, unreadable archives, or checksum
mismatch. It never prints the database password.

## Offsite & encryption

1. Generate a backup GPG keypair on an offline admin machine.
2. Import only the **public** key on the DB host; keep the private key offline
   in a sealed envelope with KPU + Bawaslu custodians.
3. Set `GPG_RECIPIENT`; verify a decrypt on a separate machine quarterly.
4. Push encrypted artifacts to object storage in a different region/provider
   with immutability/object-lock enabled. Restrict delete permission.

## Restore procedure

> Restore is destructive. Always restore to a scratch/staging database first.

```bash
# 1. Pick the artifact and verify it
/opt/pemira/deploy/scripts/verify-backup.sh /var/backups/pemira/pemira-<stamp>.dump.gpg

# 2. Rehearse on staging
export PGDATABASE=pemira_staging_restore
/opt/pemira/deploy/scripts/restore-postgres.sh /var/backups/pemira/pemira-<stamp>.dump.gpg --yes

# 3. Sanity-check counts and invariants
psql "$PGDATABASE" -c "SELECT count(*) FROM voters;"
psql "$PGDATABASE" -c "SELECT count(*) FROM ballots;"
psql "$PGDATABASE" -c "SELECT count(*) FROM voters WHERE has_voted AND receipt_hash IS NULL;"
psql "$PGDATABASE" -c "SELECT count(*) FROM ballots b LEFT JOIN chairs c ON c.id=b.chair_id WHERE c.id IS NULL;"

# 4. Only then restore production during a declared incident
/opt/pemira/deploy/scripts/restore-postgres.sh /var/backups/pemira/pemira-<stamp>.dump.gpg --yes
sudo systemctl restart pemira-api
/opt/pemira/deploy/scripts/healthcheck.sh
```

Post-restore checklist:

- [ ] `/health/readiness` = 200.
- [ ] Voter count matches the DPT snapshot used for the election.
- [ ] `ballots` count is non-decreasing versus the last tally.
- [ ] No ballot row links to a voter identity (zero-linkability invariant).
- [ ] Audit log table has no gaps around the incident window.
- [ ] KPU + Bawaslu sign the restore incident report.

## Restore drills

- Before election day: full restore into staging and record duration.
- Quarterly thereafter: verify checksum + archive listing automatically.
- Keep the last successful drill report in the audit bundle.
