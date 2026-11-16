# Load & Resilience Testing (PEMIRA 2026)

All tests run against **staging only**. Never point k6 at the production domain
during the election window.

## Prerequisites

- k6 v0.50+ installed on the load generator (separate host from the API host).
- Staging backend with `SERVER_ENV=development`, mock login enabled, Turnstile
  verification disabled, and a disposable database.
- Monitoring open on `/metrics` and Prometheus scraping staging.

## Test matrix

| Script | Goal | Pass criteria |
| --- | --- | --- |
| `k6-vote-load.js` | 3k-5k concurrent voters | 5xx < 1%, p95 < 1.5s, vote success > 99% |
| `k6-double-submit.js` | Concurrency guard | exactly one ballot accepted per voter; zero double-accept |
| `k6-failure-drill.js` | Redis/DB degradation | public read availability > 95%, no 500 storms |

## Commands

```bash
# 1. Baseline surge
k6 run -e API_BASE=https://staging-api.pemira.example.ac.id \
       -e RUN_ID="$(date +%s)" k6-vote-load.js

# 2. Double-submit correctness
k6 run -e API_BASE=https://staging-api.pemira.example.ac.id k6-double-submit.js

# 3. Failure drill (pause Redis in another terminal first)
#    Compose service name is `redis` (see docker-compose.production.yml).
docker pause pemira-redis-1   # or: docker compose pause redis
k6 run -e API_BASE=https://staging-api.pemira.example.ac.id k6-failure-drill.js
docker unpause pemira-redis-1
```

## Evidence to archive

- k6 summary JSON (`k6 run --summary-export=evidence/load-YYYYMMDD.json ...`).
- Prometheus screenshots / query exports for the same window.
- API container CPU, memory, and DB connection counts.
- Signed note from the operator who ran the drill, with date/time and commit SHA.

Store the evidence in the audit bundle; do not include voter tokens or secrets.
