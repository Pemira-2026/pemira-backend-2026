// =============================================================================
// PEMIRA 2026 - k6 concurrency guard test (double submit must yield ONE ballot)
// =============================================================================
// STAGING ONLY. For each iteration:
//   1. mock-login a fresh voter
//   2. send the same ballot twice in parallel
//   3. assert exactly one request succeeds and the other is rejected
// Usage:
//   k6 run -e API_BASE=https://staging-api.pemira.example.ac.id k6-double-submit.js
// =============================================================================
import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

const API_BASE = (__ENV.API_BASE || 'http://localhost:8080').replace(/\/+$/, '');
const RUN_ID = __ENV.RUN_ID || `${Date.now()}`;

export const doubleAccepted = new Counter('pemira_double_accept_bug');
export const correctRejections = new Counter('pemira_double_rejected_correctly');
export const noVoteRecorded = new Counter('pemira_no_vote_recorded');

export const options = {
  scenarios: {
    double_submit: {
      executor: 'constant-vus',
      vus: 50,
      duration: '2m',
    },
  },
  thresholds: {
    pemira_double_accept_bug: ['count==0'],
  },
};

const headers = (token) => ({
  'Content-Type': 'application/json',
  Authorization: `Bearer ${token}`,
});

export default function () {
  const id = `${__VU}-${__ITER}-${RUN_ID}`;
  const loginRes = http.post(
    `${API_BASE}/api/v1/auth/mock-login`,
    JSON.stringify({
      email: `race+${id}@stu.pnj.ac.id`,
      nim: `RC${id}`.slice(0, 20),
      name: 'Race Test',
      major: 'Teknik Informatika dan Komputer',
    }),
    { headers: { 'Content-Type': 'application/json' } }
  );
  if (loginRes.status !== 200) return;
  const token = loginRes.json('token');

  const chairs = http.get(`${API_BASE}/api/v1/booth/candidates`, { headers: headers(token) }).json() || [];
  const votes = [];
  for (const chair of chairs) {
    if ((chair.candidates || []).length > 0) {
      votes.push({ chairId: chair.id, candidateGroupId: chair.candidates[0].id });
    }
  }
  if (votes.length === 0) return;

  const payload = JSON.stringify({ turnstileToken: 'loadtest-bypass', votes });
  const req = { method: 'POST', url: `${API_BASE}/api/v1/vote/submit`, body: payload, params: { headers: headers(token) } };
  const responses = http.batch([req, req]);

  const accepted = responses.filter((r) => r.status === 200 || r.status === 201).length;
  const rejected = responses.filter((r) => r.status === 400 || r.status === 409 || r.status === 429).length;

  check(responses, {
    'exactly one accepted': () => accepted === 1,
    'exactly one rejected': () => rejected === 1,
  });

  if (accepted > 1) doubleAccepted.add(1);
  else if (accepted === 1 && rejected === 1) correctRejections.add(1);
  else noVoteRecorded.add(1);
}
