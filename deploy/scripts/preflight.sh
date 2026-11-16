#!/usr/bin/env bash
# =============================================================================
# PEMIRA 2026 - Production preflight checks (run before first deploy)
# =============================================================================
# Usage: ./preflight.sh /path/to/.env.production
# Exit non-zero on any blocking issue. Never prints secret values.
# =============================================================================
set -uo pipefail

ENV_FILE="${1:-.env.production}"
failures=0

fail() { echo "FAIL: $*"; failures=$((failures + 1)); }
pass() { echo "PASS: $*"; }
warn() { echo "WARN: $*"; }

[ -f "$ENV_FILE" ] || { echo "FAIL: env file not found: $ENV_FILE"; exit 1; }

perm="$(stat -c '%a' "$ENV_FILE" 2>/dev/null || stat -f '%Lp' "$ENV_FILE" 2>/dev/null || echo unknown)"
case "$perm" in
  600|400) pass "env file permissions ($perm)" ;;
  *) fail "env file permissions are $perm; expected 600 (chmod 600 $ENV_FILE)" ;;
esac

if grep -q 'CHANGEME' "$ENV_FILE"; then
  fail "env file still contains CHANGEME placeholders"
fi
# Only URL-valued keys must be real hosts. TRUSTED_PROXIES legitimately contains
# 127.0.0.1 for a loopback reverse proxy.
if grep -E '^(DATABASE_URL|GOOGLE_REDIRECT_URL|VOTER_FRONTEND_URL|ADMIN_FRONTEND_URL|TURNSTILE_VERIFY_URL)=' "$ENV_FILE" \
     | grep -Eq 'localhost|127\.0\.0\.1'; then
  fail "a production URL still points at localhost; use the real domain"
fi
# Cloudflare publishes these test keys; they always pass and must never ship.
if grep -Eq '1x00000000000000000000AA|1x0000000000000000000000000000000AA|2x0000000000000000000000000000000AA' "$ENV_FILE"; then
  fail "Cloudflare Turnstile test key detected"
fi

required=(SERVER_ENV JWT_SECRET RECEIPT_SECRET_KEY RECEIPT_SALT KPU_MASTER_SECRET BAWASLU_AUDIT_SECRET REDIS_PASSWORD METRICS_TOKEN VOTER_FRONTEND_URL ADMIN_FRONTEND_URL DATABASE_URL POSTGRES_PASSWORD TLS_CERT_PATH TLS_KEY_PATH FRONTEND_DIST_PATH ADMIN_DIST_PATH)
for key in "${required[@]}"; do
  if grep -Eq "^${key}=.+" "$ENV_FILE"; then
    pass "${key} is set"
  else
    fail "${key} is missing or empty"
  fi
done

# Secret strength (never printed). Values are read via env_value below.
env_value() { grep -E "^$1=" "$ENV_FILE" | head -1 | cut -d= -f2-; }
for key in JWT_SECRET RECEIPT_SECRET_KEY RECEIPT_SALT KPU_MASTER_SECRET BAWASLU_AUDIT_SECRET; do
  value="$(env_value "$key")"
  len="${#value}"
  if [ "$len" -ge 32 ]; then
    pass "${key} length ${len}"
  else
    fail "${key} too short (${len}); use openssl rand -base64 48"
  fi
done

if [ -f "$(env_value TLS_CERT_PATH)" ] && [ -f "$(env_value TLS_KEY_PATH)" ]; then
  pass "TLS certificate files present"
else
  warn "TLS cert/key files not found on this host (expected when preflighting elsewhere)"
fi
if [ -d "$(env_value FRONTEND_DIST_PATH)" ] && [ -d "$(env_value ADMIN_DIST_PATH)" ]; then
  pass "frontend dist directories present"
else
  warn "frontend dist directories missing; copy built artifacts before compose up"
fi

grep -Eq '^SERVER_ENV=production$' "$ENV_FILE" && pass "SERVER_ENV=production" || fail "SERVER_ENV must be production"
grep -Eq '^SEED_DEV_DATA=false$' "$ENV_FILE" && pass "SEED_DEV_DATA=false" || fail "SEED_DEV_DATA must be false"
grep -Eq '^DB_AUTO_MIGRATE=false$' "$ENV_FILE" && pass "DB_AUTO_MIGRATE=false" || fail "DB_AUTO_MIGRATE must be false"
grep -Eq '^REDIS_REQUIRED=true$' "$ENV_FILE" && pass "REDIS_REQUIRED=true" || fail "REDIS_REQUIRED must be true"
grep -Eq '^ENABLE_TURNSTILE_VERIFY=true$' "$ENV_FILE" && pass "Turnstile enabled" || warn "Turnstile verification is not enabled"
grep -Eq '^MCP_ALLOW_MOCK_LOGIN=false$' "$ENV_FILE" && pass "MCP mock login disabled" || fail "MCP_ALLOW_MOCK_LOGIN must be false"
grep -Eq '^MCP_OPERATOR_ROLE=readonly$' "$ENV_FILE" && pass "MCP role readonly" || warn "MCP role is not readonly"

# Quorum secrets must differ (values are never printed).
kpu_val="$(env_value KPU_MASTER_SECRET)"
bawaslu_val="$(env_value BAWASLU_AUDIT_SECRET)"
if [ -n "$kpu_val" ] && [ -n "$bawaslu_val" ] && [ "$kpu_val" != "$bawaslu_val" ]; then
  pass "quorum secrets differ"
else
  fail "quorum secrets are missing or identical"
fi

for tool in docker curl openssl; do
  command -v "$tool" >/dev/null 2>&1 && pass "$tool installed" || fail "$tool not found"
done

if grep -Eq '^METRICS_TOKEN=$' "$ENV_FILE"; then
  fail "METRICS_TOKEN is empty while metrics are enabled"
fi

# Prometheus mounts deploy/monitoring as /etc/prometheus and reads the token
# from this file; missing it makes the scrape fail with 401.
if [ -f "monitoring/pemira_metrics_token" ]; then
  pass "monitoring metrics token file present"
else
  warn "monitoring/pemira_metrics_token missing; create it before the monitoring profile"
fi

if [ "$failures" -gt 0 ]; then
  echo "${failures} preflight check(s) failed. Fix before deploying."
  exit 1
fi
echo "Preflight passed."
