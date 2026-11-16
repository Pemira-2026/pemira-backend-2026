# PEMIRA 2026 Deployment Bundle

Production-oriented artifacts for the PEMIRA 2026 e-voting stack. Nothing here
contains real credentials or domains; all values marked `CHANGEME` or
`example.ac.id` must be replaced by UPT TIK / KPU.

## Contents

> After copying to the host, make the scripts executable:
> `chmod +x deploy/scripts/*.sh`

```
deploy/
├── DEPLOYMENT_RUNBOOK.md        step-by-step production deployment + election day
├── BACKUP_RESTORE.md            backup policy, encryption, restore drill
├── MONITORING.md                metrics, Prometheus/Grafana, alert response
├── EVIDENCE_TEMPLATE.md         fill per environment; signed go-live evidence
├── docker-compose.production.yml
├── nginx/
│   ├── pemira.conf              TLS, security headers, edge rate limits
│   └── snippets/                headers, proxy params, TLS hardening
├── systemd/                     API unit + nightly backup timer
├── scripts/
│   ├── preflight.sh             blocking production env checks
│   ├── migrate.sh               ordered SQL migrations + schema_migrations
│   ├── backup-postgres.sh       encrypted, checksummed, offsite-capable
│   ├── restore-postgres.sh      destructive restore (requires --yes)
│   ├── verify-backup.sh         checksum + archive + table checks
│   ├── healthcheck.sh           post-deploy smoke checks
│   └── firewall-ufw.sh          baseline host firewall
├── monitoring/                  prometheus.yml, alerts.yml, Grafana dashboard
├── logrotate/                   /etc/logrotate.d/pemira
├── loadtest/                    k6 surge / double-submit / failure drill
└── security/                    audit checklist + pentest plan
```

Environment templates live in the repo root:

- `.env.production.example` (backend + compose)
- `../pemira-frontend/.env.production.example`
- `../pemira-admin/.env.production.example`

`.gitignore` files allow `*.env.example` / `*.env.production.example` while still
ignoring real `.env` files. Never add real values to a template.
`pemira-backend/.dockerignore` keeps `.env*`, binaries, and docs out of the Docker
build context so secrets never end up in an image layer.

## Monitoring token

Prometheus authenticates with `/etc/prometheus/pemira_metrics_token`, mounted from
`monitoring/pemira_metrics_token`. Create it from `METRICS_TOKEN` before starting
the monitoring profile:

```bash
install -m 0400 /dev/stdin monitoring/pemira_metrics_token <<< "$METRICS_TOKEN"
```

The file must contain the token with no trailing newline. It is git-ignored.

## Evidence

Copy `EVIDENCE_TEMPLATE.md` into `deploy/evidence/<env>-<YYYYMMDD>/EVIDENCE.md`,
attach the logs listed in it, and collect signatures. This is the artifact the
go-live gates below refer to.

## Minimum go-live gates

1. `go build ./... && go test ./... && go vet ./...` pass.
2. `npm run build` passes for both frontends with production env.
3. `migrate.sh` applied; `schema_migrations` lists both files.
4. `healthcheck.sh` all PASS against staging.
5. k6 3k–5k surge passed (5xx < 1%, p95 < 1.5s).
6. Double-submit drill: zero double-accept.
7. Encrypted backup + restore drill completed and signed.
8. Monitoring alerts wired and test-fired.
9. Security audit + pentest checklists signed by KPU/Bawaslu/UPT TIK.
10. `EVIDENCE_TEMPLATE.md` filled and signed for the environment.

## Not included (blocked on external input)

Real domains/DNS, server IP, TLS certificates, Google OAuth credentials,
production Turnstile keys, secret custody ceremony, and actual staging/production
execution evidence. These are tracked in `DEPLOYMENT_RUNBOOK.md` §15.
