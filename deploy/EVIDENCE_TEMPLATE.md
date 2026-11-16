# Deployment Evidence Template (PEMIRA 2026)

Fill one copy per environment (staging, production) and store the signed result in
the audit bundle. Copy to `deploy/evidence/<env>-<YYYYMMDD>/EVIDENCE.md` and attach
raw artifacts next to it. **Never** attach secrets, JWTs, voter tokens, or
`.env.production`.

```bash
mkdir -p deploy/evidence/production-$(date -u +%Y%m%d)
cp deploy/EVIDENCE_TEMPLATE.md deploy/evidence/production-$(date -u +%Y%m%d)/EVIDENCE.md
```

## 1. Release identity

| Field | Value |
| --- | --- |
| Environment | `staging` / `production` |
| Commit SHA | `<git rev-parse HEAD>` |
| Release tag | `<tag>` |
| Build date (UTC) | `<YYYY-MM-DDTHH:MM:SSZ>` |
| Builder | `<name>` |
| Backend binary SHA-256 | `<sha256sum pemira-api>` |
| Frontend bundle SHA-256 | `<sha256sum dist/assets/*.js>` |
| Admin bundle SHA-256 | `<sha256sum dist/assets/*.js>` |

## 2. Build & test gates

| Gate | Command | Result | Evidence file |
| --- | --- | --- | --- |
| Backend build | `cd pemira-backend && go build ./...` | PASS / FAIL | `backend-build.log` |
| Backend vet | `go vet ./...` | PASS / FAIL | `backend-vet.log` |
| Backend tests | `go test ./...` | PASS / FAIL | `backend-test.log` |
| Voter frontend build | `VITE_API_BASE_URL=... npm run build` | PASS / FAIL | `frontend-build.log` |
| Admin frontend build | `VITE_API_BASE_URL=... npm run build` | PASS / FAIL | `admin-build.log` |

## 3. Database migration evidence

| Check | Command | Result |
| --- | --- | --- |
| Pre-migration backup exists | `ls -l /var/backups/pemira/` | |
| Migration applied | `./deploy/scripts/migrate.sh` | |
| Recorded versions | `psql -c "SELECT version, applied_at FROM schema_migrations ORDER BY version;"` | |
| Tables present | `psql -c "\dt"` | |
| Startup table verification passed | API boot log | |

Paste `schema_migrations` output:

```
<output>
```

## 4. Preflight

| Check | Command | Result |
| --- | --- | --- |
| Env file mode 0600 | `stat -c '%a' .env.production` | |
| No CHANGEME / localhost URLs | `./deploy/scripts/preflight.sh .env.production` | |
| Compose config validates | `docker compose ... config >/dev/null` | |
| Firewall baseline | `sudo ./deploy/scripts/firewall-ufw.sh` | |
| Closed ports | `ss -lntp \| grep -E ':(5432\|6379\|8080)'` | |

## 5. Smoke test

Paste full `deploy/scripts/healthcheck.sh` output:

```
<output>
```

Manual confirmations:

- [ ] Voter SPA loads over HTTPS; Google button targets `/api/v1/auth/google/login`.
- [ ] Admin SPA loads; `/admin/turnout` without token → 401.
- [ ] Turnstile widget renders with the production site key.
- [ ] Response headers include HSTS, `X-Content-Type-Options`, `X-Frame-Options`.
- [ ] One staging ballot completes end-to-end.

## 6. Load & resilience

| Drill | Script | Threshold | Result |
| --- | --- | --- | --- |
| Surge 3k–5k | `k6-vote-load.js` | 5xx < 1%, p95 < 1.5s, success > 99% | |
| Double submit | `k6-double-submit.js` | zero double-accept | |
| Redis/DB failure | `k6-failure-drill.js` | read availability > 95% | |

Attach `--summary-export` JSON, Prometheus query exports, and host CPU/mem/DB
connection samples for the same window.

## 7. Backup & restore drill

| Check | Command | Result |
| --- | --- | --- |
| Nightly backup produced | `systemctl start pemira-backup.service` | |
| Artifact checksum | `./deploy/scripts/verify-backup.sh <file>` | |
| Restore into scratch DB | `PGDATABASE=pemira_restore ./deploy/scripts/restore-postgres.sh <file> --yes` | |
| Row counts sane | `SELECT count(*) FROM voters; SELECT count(*) FROM ballots;` | |
| Restore duration (RTO) | `<minutes>` vs 60 min target | |

## 8. Monitoring

- [ ] Prometheus scraping `pemira-api` (token mounted at `/etc/prometheus/pemira_metrics_token`).
- [ ] Grafana dashboard `pemira-api-ops` imported.
- [ ] `PemiraApiDown` test-fired (stop API in staging) and routed to on-call.
- [ ] `PemiraHigh5xxRate` test-fired with an induced error.

## 9. Security

| Item | Result | Reference |
| --- | --- | --- |
| Audit checklist complete | | `deploy/security/SECURITY_AUDIT_CHECKLIST.md` |
| Pentest checklist complete | | `deploy/security/PENTEST_CHECKLIST.md` |
| Secrets stored in secret manager, not git | | |
| `../.env` removed / DB password rotated | | |

## 10. Sign-off

| Role | Name | Signature | Date/time (WIB) |
| --- | --- | --- | --- |
| KPU Chair | | | |
| Bawaslu Coordinator | | | |
| UPT TIK Infra Lead | | | |
| Security Reviewer | | | |

## Unresolved placeholders at time of signing

List anything still using `example.ac.id` / `CHANGEME` and who owns closing it:

- `<item — owner — due date>`
