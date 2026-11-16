# Monitoring & Observability (PEMIRA 2026)

## Endpoints

| Endpoint | Purpose | Auth |
| --- | --- | --- |
| `/health/liveness` | process alive | public |
| `/health/readiness` | DB + Redis ready | public |
| `/health` | aggregate dependency check | public |
| `/metrics` | Prometheus text metrics | `Authorization: Bearer $METRICS_TOKEN` when set |

Set `METRICS_TOKEN` in production and scrape only over the private network.
Never expose `/metrics` through the public nginx server block.

## Metrics exposed

- `pemira_http_requests_total{method,path,status}`
- `pemira_http_request_duration_seconds_{sum,count}`
- `pemira_http_errors_total{...}` (5xx)
- `pemira_http_inflight_requests`
- `pemira_go_goroutines`, `pemira_go_memstats_alloc_bytes`
- `pemira_process_uptime_seconds`

## Stack

1. Start Prometheus with the compose monitoring profile:
   ```bash
   docker compose -f docker-compose.production.yml --profile monitoring up -d prometheus
   ```
2. Put the metrics token on the monitoring host:
   ```bash
   install -m 0400 /dev/stdin /etc/prometheus/pemira_metrics_token <<< "$METRICS_TOKEN"
   ```
   In the compose stack this file is mounted from
   `deploy/monitoring/pemira_metrics_token` (git-ignored). Create it with:
   ```bash
   install -m 0400 /dev/stdin deploy/monitoring/pemira_metrics_token <<< "$METRICS_TOKEN"
   ```
   It must contain exactly the token with no trailing newline.
3. Import `monitoring/grafana-dashboard.json` into Grafana.
4. Route alerts from `monitoring/alerts.yml` to the on-call channel.

## Alert response

| Alert | First action |
| --- | --- |
| `PemiraApiDown` | check `systemctl status pemira-api`, container logs, DB/Redis health |
| `PemiraHigh5xxRate` | inspect 5xx by route in Grafana, check DB pool/Redis, roll back last deploy if needed |
| `PemiraHighMeanLatency` | check DB connections, slow queries, Redis latency, host CPU |
| `PemiraInflightSaturation` | scale API replicas / raise `LimitNOFILE`, inspect downstream timeouts |
| `PemiraProcessRestarted` | read crash logs, check OOM killer, freeze deploys until root cause known |
| `PemiraNoTrafficDuringElection` | verify election window, DNS/TLS, campus network, nginx upstream |

## Logs

- API: `/var/log/pemira/api.log`, `/var/log/pemira/api.err.log`
- Backup: `/var/log/pemira/backup.log`
- nginx: container JSON logs (rotate via Docker `max-size`/`max-file`)

Ship logs to a central store if available. Never log JWTs, OAuth codes,
quorum secrets, Turnstile secrets, or full request bodies containing ballots.

## Election-day watch

- Dashboard on a wall screen in the KPU command post.
- Check every 15 minutes: 5xx ratio, vote submit rate, readiness, DB connections.
- Keep a written timeline of anomalies and operator actions for the audit report.
