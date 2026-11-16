#!/usr/bin/env bash
# =============================================================================
# PEMIRA 2026 - Baseline UFW firewall (single-host deployment)
# =============================================================================
# Run as root on the production host. Review before executing: SSH port and
# monitoring source ranges are environment-specific.
# =============================================================================
set -euo pipefail

SSH_PORT="${SSH_PORT:-22}"
MONITORING_CIDR="${MONITORING_CIDR:-10.0.0.0/8}"

ufw default deny incoming
ufw default allow outgoing

ufw allow "${SSH_PORT}/tcp" comment 'SSH admin'
ufw allow 80/tcp comment 'HTTP (redirect/ACME)'
ufw allow 443/tcp comment 'HTTPS'

# Database/Redis/API must never be reachable from the internet. If services run
# on a private Docker network, these host rules stay closed by default.
# Allow metrics scraping only from the monitoring subnet.
ufw allow from "${MONITORING_CIDR}" to any port 9090 proto tcp comment 'Prometheus (private)'

ufw --force enable
ufw status verbose

echo "UFW baseline applied. Verify 5432/6379/8080 are NOT exposed externally:"
echo "  ss -lntp | grep -E ':(5432|6379|8080)'"
