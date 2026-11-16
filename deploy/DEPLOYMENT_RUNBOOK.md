# Production Deployment Runbook (PEMIRA 2026)

> Status: reference runbook. Values in `*.example` files are placeholders and
> **must be replaced by UPT TIK / KPU** before this can be called a real
> production deployment. No real domain, IP, or certificate is included.

## 1. Architecture (single host reference)

```
Internet
  │ 443/TLS
  ▼
nginx  ──► static voter SPA (pemira.example.ac.id)
       ──► static admin SPA (admin.pemira.example.ac.id)
       ──► reverse proxy /api/* ──► pemira-api:8080
                                      │
                          ┌───────────┴───────────┐
                          ▼                       ▼
                     postgres:5432           redis:6379
                     (private bridge)        (private bridge)
```

All stateful services stay on the private Docker network. Only 80/443 are
public. `/metrics` is reachable only from the monitoring subnet.

## 2. Placeholders requiring real values

| Placeholder | Owner | Notes |
| --- | --- | --- |
| `pemira.example.ac.id` | UPT TIK | voter domain + DNS A/AAAA |
| `admin.pemira.example.ac.id` | UPT TIK | admin domain, preferably VPN/IP-restricted |
| `api.pemira.example.ac.id` | UPT TIK | API domain |
| Server IP / hostname | UPT TIK | VM sizing: 4 vCPU / 8 GB RAM minimum |
| TLS certificate paths | UPT TIK | Let's Encrypt or campus CA |
| Google OAuth client | KPU + UPT TIK | redirect URI must match exactly |
| Turnstile site/secret key | KPU | production keypair, not test keys |
| `KPU_MASTER_SECRET` / `BAWASLU_AUDIT_SECRET` | KPU + Bawaslu | split custody, secret manager |
| SMTP/notification channel | KPU | optional |
| Monitoring contact | KPU | alert routing |

## 3. Pre-flight

- [ ] DNS resolves for all three domains.
- [ ] VM patched, time sync enabled (`timedatectl`), Docker Engine + compose plugin installed.
- [ ] `deploy/` copied to `/opt/pemira/deploy`, repo checkout at a tagged commit.
- [ ] Scripts executable: `chmod +x /opt/pemira/deploy/scripts/*.sh`.
- [ ] All secrets generated (`openssl rand -base64 48`) and stored in the secret manager.
- [ ] `.env.production` created from `deploy/.env.production.example` (mode 0600, owner `pemira`).
- [ ] Backups of any existing database taken before migration.

## 4. Build frontends (on CI or a build host)

Vite inlines env at build time; production builds intentionally fail without
`VITE_API_BASE_URL`.

```bash
cd pemira-frontend
VITE_API_BASE_URL=https://api.pemira.example.ac.id/api/v1 \
VITE_TURNSTILE_SITE_KEY=<real-site-key> \
npm ci && npm run build

cd ../pemira-admin
VITE_API_BASE_URL=https://api.pemira.example.ac.id/api/v1 \
npm ci && npm run build
```

Copy `pemira-frontend/dist` and `pemira-admin/dist` to the host and set
`FRONTEND_DIST_PATH` / `ADMIN_DIST_PATH` in `.env.production`.

## 5. Database migrations (before first boot)

```bash
cd /opt/pemira/deploy
export DATABASE_URL='postgresql://pemira_app:<password>@127.0.0.1:5432/pemira_db?sslmode=require'
./scripts/migrate.sh
```

`migrate.sh` applies `internal/database/migrations/*.sql` in
lexical order inside a transaction and records versions in `schema_migrations`.
Keep `DB_AUTO_MIGRATE=false` in production. The API also verifies required
tables exist at startup and refuses to boot if any are missing.

## 6. Configure and start the stack

```bash
cd /opt/pemira/deploy
cp ../.env.production.example .env.production
chmod 600 .env.production
# Fill every CHANGEME + POSTGRES_PASSWORD, REDIS_PASSWORD,
# TLS_CERT_PATH, TLS_KEY_PATH, FRONTEND_DIST_PATH, ADMIN_DIST_PATH.

./scripts/preflight.sh .env.production
docker compose -f docker-compose.production.yml --env-file .env.production config >/dev/null
docker compose -f docker-compose.production.yml --env-file .env.production up -d --build
docker compose -f docker-compose.production.yml --env-file .env.production ps
```

TLS (example with certbot on the host):

```bash
sudo certbot certonly --standalone \
  -d pemira.example.ac.id -d admin.pemira.example.ac.id -d api.pemira.example.ac.id
sudo cp /etc/letsencrypt/live/pemira.example.ac.id/fullchain.pem /etc/pemira/tls/
sudo cp /etc/letsencrypt/live/pemira.example.ac.id/privkey.pem  /etc/pemira/tls/
sudo chmod 600 /etc/pemira/tls/privkey.pem
```

## 7. Firewall

```bash
sudo SSH_PORT=22 MONITORING_CIDR=10.0.0.0/8 ./scripts/firewall-ufw.sh
ss -lntp | grep -E ':(5432|6379|8080)'   # must show only loopback/private, never 0.0.0.0
```

## 8. Smoke test

```bash
API_BASE=https://api.pemira.example.ac.id \
METRICS_TOKEN='<token>' ./scripts/healthcheck.sh
```

Manual checks:

- Voter SPA loads over HTTPS; Google button hits `https://api.../api/v1/auth/google/login`.
- Admin SPA loads; `/admin/turnout` without a token returns 401.
- Turnstile widget renders with the production site key.
- Response headers include HSTS, `X-Content-Type-Options`, `X-Frame-Options`.
- One complete mock/staging ballot works end-to-end (never in production with real DPT).

## 9. Backup timer

```bash
sudo install -m 0644 systemd/pemira-backup.service /etc/systemd/system/
sudo install -m 0644 systemd/pemira-backup.timer   /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now pemira-backup.timer
sudo systemctl start pemira-backup.service
sudo systemctl list-timers pemira-backup.timer
```

Run the restore drill in `BACKUP_RESTORE.md` before election day.

## 10. Monitoring

```bash
# Prometheus authenticates to /metrics with METRICS_TOKEN. The file is mounted
# into the container, so create it before starting the profile.
install -m 0400 /dev/stdin monitoring/pemira_metrics_token <<< "$METRICS_TOKEN"
docker compose -f docker-compose.production.yml --profile monitoring up -d prometheus
```

See `MONITORING.md`. Import `monitoring/grafana-dashboard.json`, wire alert
routing, and confirm `PemiraApiDown` fires when the API is stopped in staging.

## 11. MCP operator server

`pemira-backend/cmd/mcp` is a **local stdio** tool server, not a network
service. Do not expose it. On an operator workstation:

```bash
export MCP_OPERATOR_ROLE=readonly
export MCP_ALLOW_MOCK_LOGIN=false
# all MCP_ALLOW_* false unless a signed change request approves otherwise
go run ./cmd/mcp
```

Production defaults: role `readonly`, all privileged flags false, tally
fail-closed, quorum secrets read only from `KPU_MASTER_SECRET` /
`BAWASLU_AUDIT_SECRET` (never tool parameters). Mock login is rejected when
`SERVER_ENV=production`. Because MCP is stdio, authorization is process-level:
run each operator role under a separate OS account and do not share shell
sessions; there is no network listener to authenticate.

## 12. Rollback

```bash
# Code rollback
cd /opt/pemira && git checkout <previous-tag>
docker compose -f deploy/docker-compose.production.yml --env-file deploy/.env.production up -d --build

# Database rollback: restore the pre-deploy backup (destructive)
deploy/scripts/restore-postgres.sh /var/backups/pemira/pemira-<predeploy>.dump.gpg --yes
deploy/scripts/healthcheck.sh
```

Schema migrations are forward-only; never hand-edit production tables without a
signed KPU/Bawaslu incident record.

## 13. Election-day runbook (summary)

| Time (WIB) | Action |
| --- | --- |
| H-1 20:00 | freeze deploys, take final backup, verify DPT count |
| H 06:00 | healthcheck, confirm TLS/DNS, open monitoring wall |
| H 07:00 | polls open (`election_config.start_time`) |
| every 15 min | check 5xx, vote rate, readiness, DB connections |
| H 13:00 | verify backup from 01:30 exists offsite + checksum |
| H 17:00 | polls close; confirm no writes after `end_time` |
| H 17:15 | dual-control tally unlock (KPU + Bawaslu present) |
| H 18:00 | export tally + audit log, print berita acara |
| H+1 | post-election backup, rotate quorum secrets, archive evidence |

## 14. Evidence

After every gate in this runbook, fill `EVIDENCE_TEMPLATE.md`:

```bash
mkdir -p /opt/pemira/deploy/evidence/production-$(date -u +%Y%m%d)
cp /opt/pemira/deploy/EVIDENCE_TEMPLATE.md /opt/pemira/deploy/evidence/production-$(date -u +%Y%m%d)/EVIDENCE.md
```

Attach the referenced logs, then collect signatures from KPU, Bawaslu, and
UPT TIK. The evidence directory is the audit artifact; it must not contain
secrets or `.env.production`.

## 15. Explicitly unresolved

These cannot be completed from the repository alone and remain UPT TIK/KPU
dependencies: real domains + DNS, server provisioning/IP, TLS issuance, Google
OAuth consent screen, production Turnstile keys, secret custody ceremony,
staging load-test execution, external penetration test, and signed deployment
evidence.
