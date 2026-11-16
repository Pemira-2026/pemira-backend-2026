# Security Audit Checklist (PEMIRA 2026)

Scope: `pemira-backend`, `pemira-frontend`, `pemira-admin`, MCP server, deployment
stack. Each item must have an owner, date, and evidence link before sign-off.

## 1. Configuration & secrets

- [ ] `SERVER_ENV=production`, `SEED_DEV_DATA=false`, `DB_AUTO_MIGRATE=false`.
- [ ] `JWT_SECRET`, `RECEIPT_SECRET_KEY`, `RECEIPT_SALT` are unique, ≥32/≥16 chars, not defaults.
- [ ] `KPU_MASTER_SECRET` ≠ `BAWASLU_AUDIT_SECRET`, both ≥16 chars, split custody.
- [ ] No secret in git history, frontend bundles, logs, or screenshots.
- [ ] `.env` files are mode 0600 and excluded from backups/source control.
- [ ] Production Turnstile keys are real; test keys rejected by config validation.
- [ ] `REDIS_REQUIRED=true` and Redis password set.
- [ ] `METRICS_TOKEN` set; `/metrics` not publicly reachable.
- [ ] `TRUSTED_PROXIES` limited to the actual reverse proxy.

## 2. MCP server

- [ ] `go build ./cmd/mcp` succeeds.
- [ ] `MCP_OPERATOR_ROLE=readonly` in production; privileged flags false.
- [ ] Tally fails closed on config/error (`err != nil || electionCfg == nil`).
- [ ] Tally reads quorum secrets only from env; no secret parameters in schema.
- [ ] `MCP_ALLOW_MOCK_LOGIN=false` in production; mock login rejects production.
- [ ] Tool authorization matrix enforced per role and per flag.
- [ ] Tool inputs validated; no arbitrary SQL/path/command execution.
- [ ] Tests cover JSON-RPC, auth, fail-closed tally, production mock login.

## 3. Authentication & session

- [ ] Google OAuth state is single-use; replay rejected.
- [ ] OAuth callback validates state, code, email domain, and issuer.
- [ ] JWT signature, expiry, issuer/audience, and role claims validated.
- [ ] Mock login disabled in production (both HTTP route and MCP).
- [ ] Single-session enforcement per voter works.
- [ ] Logout/rotation invalidates tokens where required.

## 4. Authorization

- [ ] Voter endpoints require voter role.
- [ ] Admin endpoints require `admin|kpu|bawaslu|saksi` as appropriate.
- [ ] Tally requires `admin|kpu|bawaslu` **and** valid dual-control headers.
- [ ] Config mutation and cache invalidation are admin-only and audit-logged.
- [ ] No IDOR on receipt, voter lookup, or audit log endpoints.
- [ ] Anonymous requests to every protected route return 401/403.

## 5. Ballot integrity

- [ ] One ballot per voter under concurrency (Redis lock + conditional SQL).
- [ ] Double-submit test: exactly one accepted.
- [ ] `ballots` has no voter identifier/FK (zero-linkability).
- [ ] Candidate belongs to the voter's chair/scope; duplicate candidate rejected.
- [ ] `MaxSelection` enforced for multi-select chairs.
- [ ] Votes rejected outside the election window; emergency pause honored.
- [ ] Receipt HMAC cannot be forged or used to derive the chosen candidate.
- [ ] Invalid/unknown receipt returns 404 without leaking existence.

## 6. Abuse & resilience

- [ ] Rate limits active on auth, vote, receipt, and admin routes.
- [ ] Edge rate limits in nginx complement app limits.
- [ ] Turnstile failure/timeout fails closed.
- [ ] Redis-down behavior documented and tested (fail-closed for writes).
- [ ] Body size limits, timeouts, and connection limits configured.
- [ ] 3k–5k load test passed with 5xx < 1%.

## 7. Web security

- [ ] HSTS, CSP, `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy` present.
- [ ] CORS allow-list contains only the real voter/admin origins.
- [ ] No `Access-Control-Allow-Origin: *` with credentials.
- [ ] Frontend has no localhost fallback in production builds.
- [ ] No secrets or sample tally data shipped in the admin bundle.
- [ ] Dependencies scanned (`govulncheck ./...`, `npm audit`).

## 8. Data & operations

- [ ] Encrypted backups run daily; retention and offsite copy verified.
- [ ] Restore drill completed with recorded RTO.
- [ ] PITR/WAL archiving enabled for election day.
- [ ] Audit log is append-only and includes config/tally/cache actions.
- [ ] Monitoring and alerts tested; on-call contact confirmed.
- [ ] Logs exclude secrets, tokens, and ballot payloads.
- [ ] Firewall exposes only 22/80/443; DB/Redis/API not public.
- [ ] TLS config ≥1.2, strong ciphers, valid certificate chain.

## 9. Sign-off

| Role | Name | Date | Signature |
| --- | --- | --- | --- |
| KPU Chair | | | |
| Bawaslu Coordinator | | | |
| UPT TIK Security | | | |
| External auditor (optional) | | | |
