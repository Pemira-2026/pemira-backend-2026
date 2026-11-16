#!/usr/bin/env bash
# =============================================================================
# PEMIRA 2026 - Post-deploy health verification
# =============================================================================
# Usage: API_BASE=https://api.pemira.example.ac.id ./healthcheck.sh
# Exits non-zero if any critical check fails. Safe to run from CI/cron.
# =============================================================================
set -uo pipefail

API_BASE="${API_BASE:-http://127.0.0.1:8080}"
METRICS_TOKEN="${METRICS_TOKEN:-}"
failures=0

check() {
  local name="$1" url="$2" expect="${3:-200}" extra="${4:-}"
  local code
  # shellcheck disable=SC2086
  code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 $extra "$url" || echo 000)"
  if [ "$code" = "$expect" ]; then
    echo "PASS ${name} (${code})"
  else
    echo "FAIL ${name}: expected ${expect}, got ${code} (${url})"
    failures=$((failures + 1))
  fi
}

check "liveness"  "${API_BASE}/health/liveness" 200
check "readiness" "${API_BASE}/health/readiness" 200
check "health"    "${API_BASE}/health" 200
check "candidates public" "${API_BASE}/api/v1/booth/all-chairs" 200

# Protected endpoints must reject anonymous access.
check "admin anonymous rejected" "${API_BASE}/api/v1/admin/turnout" 401
check "vote anonymous rejected"  "${API_BASE}/api/v1/vote/submit" 401 "-X POST -H 'Content-Type: application/json' -d '{}'"

if [ -n "$METRICS_TOKEN" ]; then
  check "metrics with token" "${API_BASE}/metrics" 200 "-H 'Authorization: Bearer ${METRICS_TOKEN}'"
  check "metrics without token rejected" "${API_BASE}/metrics" 401
fi

# Security headers (nginx or app).
headers="$(curl -sSI --max-time 10 "${API_BASE}/health" || true)"
for h in "Strict-Transport-Security" "X-Content-Type-Options" "X-Frame-Options" "Referrer-Policy"; do
  if printf '%s' "$headers" | grep -qi "^${h}:"; then
    echo "PASS header ${h}"
  else
    echo "WARN header ${h} missing at ${API_BASE} (may be terminated at nginx)"
  fi
done

if [ "$failures" -gt 0 ]; then
  echo "${failures} health check(s) failed"
  exit 1
fi
echo "All health checks passed."
