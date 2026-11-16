// =============================================================================
// PEMIRA 2026 - k6 failure injection: read path under Redis/DB pressure
// =============================================================================
// Run against staging while an operator pauses Redis or throttles the database.
// Asserts the public read path stays available and returns valid JSON.
// Usage:
//   k6 run -e API_BASE=https://staging-api.pemira.example.ac.id k6-failure-drill.js
// =============================================================================
import http from 'k6/http';
import { check } from 'k6';
import { Rate } from 'k6/metrics';

const API_BASE = (__ENV.API_BASE || 'http://localhost:8080').replace(/\/+$/, '');
export const readAvailability = new Rate('pemira_read_availability');

export const options = {
  scenarios: {
    steady_reads: {
      executor: 'constant-arrival-rate',
      rate: 200,
      timeUnit: '1s',
      duration: '5m',
      preAllocatedVUs: 200,
      maxVUs: 500,
    },
  },
  thresholds: {
    pemira_read_availability: ['rate>0.95'],
  },
};

export default function () {
  const health = http.get(`${API_BASE}/health/readiness`);
  const chairs = http.get(`${API_BASE}/api/v1/booth/all-chairs`);

  const ok = health.status === 200 && chairs.status === 200;
  readAvailability.add(ok);

  check(health, { 'readiness 200': (r) => r.status === 200 });
  check(chairs, {
    'chairs 200 or 503': (r) => r.status === 200 || r.status === 503,
    'chairs json': (r) => {
      try { JSON.parse(r.body); return true; } catch (_) { return false; }
    },
  });
}
