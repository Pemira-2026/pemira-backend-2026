# Security & Operational Remediation Status (PEMIRA 2026)

Status legend: **Fixed** = code/artifact in repo and verified locally.
**Blocked** = requires UPT TIK/KPU input or an external environment to execute.
**Accepted risk** = documented, not fully eliminable in code.

| # | Finding | Status | Evidence / verification |
| --- | --- | --- | --- |
| 1 | MCP server did not compile | Fixed | MCP moved to `cmd/mcp/main.go`; `go build ./...` passes |
| 2 | MCP tally failed open on config/error | Fixed | `tally` returns error when `err != nil \|\| electionCfg == nil`; `TestMCPTallyFailClosedWhenElectionConfigMissing` |
| 3 | MCP mock login usable in production | Fixed | production guard + `MCP_ALLOW_MOCK_LOGIN=false` enforced by config validation; tests |
| 4 | MCP privileged tools lacked authorization | Fixed | env-driven `MCP_OPERATOR_ROLE` + per-tool `MCP_ALLOW_*`; production defaults `readonly` |
| 5 | Quorum secrets accepted as MCP tool parameters | Fixed | removed from schema; read only from `KPU_MASTER_SECRET`/`BAWASLU_AUDIT_SECRET`, distinct + non-empty required |
| 6 | Voter frontend called wrong Google login URL | Fixed | `VotingBoothPage.tsx` now uses `api.getGoogleLoginUrl()` → `/api/v1/auth/google/login` |
| 7 | Production frontend silently used localhost API | Fixed | both frontends throw if `VITE_API_BASE_URL` missing in a production build |
| 8 | Admin dashboard showed sample tally + hardcoded quorum secrets | Fixed | defaults empty; API error surfaces instead of `SAMPLE_RESULTS` fallback |
| 9 | Admin turnout request missing bearer token | Fixed | `getTurnout()` now sends the admin auth header |
| 10 | SQL migrations appeared missing | Fixed (stale claim) | `internal/database/migrations/000001_init_schema.sql`, `000002_create_admin_users.sql`, plus runner + README |
| 11 | No application rate limiting | Fixed | Redis atomic `INCR`+`PEXPIRE` limiter with in-process fallback; per-route limits in `cmd/api/main.go`; tests |
| 12 | No security headers / HSTS | Fixed | global middleware + nginx headers/CSP; HSTS in production |
| 13 | Caches not invalidated after config/candidate changes | Fixed | update path calls cache invalidation and audit log; `POST /admin/cache/invalidate`; tests |
| 14 | No operational monitoring | Fixed (deploy pending) | `/metrics` Prometheus format + token; Grafana dashboard, Prometheus config, alert rules |
| 15 | No database backup/restore | Fixed (drill pending) | encrypted checksummed backup script, restore/verify scripts, systemd timer, documented RPO/RTO |
| 16 | Missing tests for critical edge cases | Fixed | MCP JSON-RPC/ACL/fail-closed, Redis-down, concurrent submit, duplicate candidate, MaxSelection>1, Turnstile failure/timeout, invalid receipt, OAuth replay; `go test ./...` passes |
| 17 | No production deployment artifacts | Fixed (values blocked) | `deploy/` runbook, compose, nginx, systemd, firewall, env templates |
| 18 | No load/pentest evidence | Blocked | k6 scripts + pentest/audit checklists provided; execution needs staging host and authorized window |
| 19 | Frontend env templates were uncommittable | Fixed | `pemira-frontend/.gitignore`, `pemira-admin/.gitignore` ignored `.env.*` including `.env.production.example`; negation rules added |
| 20 | Docker build context could include real secrets | Fixed | `.dockerignore` excludes `.env*` (templates excepted), binaries, docs |
| 21 | Prometheus had no metrics token in the compose stack | Fixed | `deploy/monitoring` mounted at `/etc/prometheus`; `pemira_metrics_token` documented, git-ignored, preflight-warned |
| 22 | nginx used deprecated `listen ... http2` | Fixed | per-server `http2 on;` plus `nginx/snippets/tls-hardening.conf` |
| 23 | Dockerfile compiled one file, ran as root | Fixed | builds `./cmd/api` with `-trimpath`, non-root `pemira` user, image `HEALTHCHECK` |
| 24 | No deployment evidence artifact | Fixed (execution blocked) | `deploy/EVIDENCE_TEMPLATE.md` + `deploy/README.md` evidence workflow |
| 25 | Preflight rejected legitimate loopback proxy config and under-checked secrets | Fixed | localhost check limited to URL keys; secret-length, TLS/dist-path, metrics-token, Turnstile test-key checks added |

## Remaining blockers (external input required)

| Blocker | Owner | Why it cannot be completed here |
| --- | --- | --- |
| Real domains, DNS, server IP | UPT TIK | No production host/domain provided |
| TLS certificates | UPT TIK | Requires DNS control / campus CA |
| Google OAuth client + consent screen | KPU + UPT TIK | Requires institutional Google Workspace admin |
| Production Turnstile keys | KPU | Requires Cloudflare account access |
| Quorum secret custody ceremony | KPU + Bawaslu | Must be performed in person and split |
| Staging deployment + k6 execution | Infra team | Needs provisioned staging environment |
| External penetration test | Authorized tester | Needs signed scope/window |
| Signed go-live evidence | KPU/Bawaslu/UPT TIK | Human sign-off |

## Secret hygiene

- `.env` in this repo contains a real Supabase URL/password. **Rotate the
  database password and remove the file from any shared/backed-up location.**
  It is not part of the deployment templates and must never be committed.
- Generate all production secrets with `openssl rand -base64 48`; store in a
  secret manager, not in git or chat.

## Local verification (run before go-live)

Paths are relative to the workspace root (`CumaEvos/`):

```bash
cd pemira-backend
go build ./...
go test ./...
go vet ./...

cd ../pemira-frontend
VITE_API_BASE_URL=https://api.example.ac.id/api/v1 npm run build

cd ../pemira-admin
VITE_API_BASE_URL=https://api.example.ac.id/api/v1 npm run build

# Deploy bundle sanity (bash required; does not contact production)
cd ../pemira-backend
bash -n deploy/scripts/*.sh
```

Last verified: backend build/test/vet pass; both frontend production builds pass;
deploy shell scripts pass `bash -n`; `preflight.sh` exercised against a synthetic
production env (only the expected host-dependent warnings/failures).

## Deployment evidence

Execution evidence cannot be produced without a real host. Use
`deploy/EVIDENCE_TEMPLATE.md` to capture build gates, migration output, smoke
tests, load results, backup/restore drills, monitoring test-fires, and sign-off.

