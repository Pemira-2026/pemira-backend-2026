// =============================================================================
// PEMIRA 2026 - k6 load test: 3,000-5,000 concurrent voters (STAGING ONLY)
// =============================================================================
// Never run against production. Requires a staging backend with:
//   SERVER_ENV=development (mock login enabled) and ENABLE_TURNSTILE_VERIFY=false
// Usage:
//   k6 run -e API_BASE=https://staging-api.pemira.example.ac.id \
//          -e RUN_ID=$(date +%s) k6-vote-load.js
// Each virtual user: mock-login -> fetch candidates -> submit one ballot.
// =============================================================================
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const API_BASE = (__ENV.API_BASE || 'http://localhost:8080').replace(/\/+$/, '');
const RUN_ID = __ENV.RUN_ID || `${Date.now()}`;
const MAJOR = __ENV.MAJOR || 'Teknik Informatika dan Komputer';

export const loginFailures = new Counter('pemira_login_failures');
export const submitSuccess = new Counter('pemira_submit_success');
export const submitRejected = new Counter('pemira_submit_rejected');
export const submitLatency = new Trend('pemira_submit_latency', true);
export const successRate = new Rate('pemira_success_rate');

export const options = {
  scenarios: {
    surge: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '2m', target: 1000 },
        { duration: '2m', target: 3000 },
        { duration: '3m', target: 5000 },
        { duration: '5m', target: 5000 },
        { duration: '1m', target: 0 },
      ],
      gracefulRampDown: '30s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    'http_req_duration{expected_response:true}': ['p95<1500'],
    pemira_success_rate: ['rate>0.99'],
  },
};

function jsonHeaders(token) {
  const h = { 'Content-Type': 'application/json' };
  if (token) h.Authorization = `Bearer ${token}`;
  return h;
}

export default function () {
  const vu = __VU;
  const iter = __ITER;
  const email = `loadtest+${RUN_ID}-${vu}-${iter}@stu.pnj.ac.id`;
  const nim = `LT${RUN_ID}${vu}${iter}`.slice(0, 20);

  const loginRes = http.post(
    `${API_BASE}/api/v1/auth/mock-login`,
    JSON.stringify({ email, nim, name: `Load Test ${vu}`, major: MAJOR }),
    { headers: jsonHeaders(), tags: { endpoint: 'mock-login' } }
  );

  if (!check(loginRes, { 'login 200': (r) => r.status === 200 })) {
    loginFailures.add(1);
    successRate.add(false);
    sleep(1);
    return;
  }

  const token = loginRes.json('token');
  const chairsRes = http.get(`${API_BASE}/api/v1/booth/candidates`, {
    headers: jsonHeaders(token),
    tags: { endpoint: 'candidates' },
  });
  if (!check(chairsRes, { 'candidates 200': (r) => r.status === 200 })) {
    successRate.add(false);
    return;
  }

  const chairs = chairsRes.json() || [];
  const votes = [];
  for (const chair of chairs) {
    const candidates = chair.candidates || [];
    if (candidates.length > 0) {
      votes.push({ chairId: chair.id, candidateGroupId: candidates[iter % candidates.length].id });
    }
  }
  if (votes.length === 0) {
    successRate.add(false);
    return;
  }

  const started = Date.now();
  const voteRes = http.post(
    `${API_BASE}/api/v1/vote/submit`,
    JSON.stringify({ turnstileToken: 'loadtest-bypass', votes }),
    { headers: jsonHeaders(token), tags: { endpoint: 'vote-submit' } }
  );
  submitLatency.add(Date.now() - started);

  const ok = voteRes.status === 200 || voteRes.status === 201;
  if (ok) submitSuccess.add(1); else submitRejected.add(1);
  successRate.add(ok);
  check(voteRes, { 'vote accepted': () => ok });

  sleep(Math.random() * 2);
}
