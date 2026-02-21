import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '30s', target: 100 },   // ramp up to 100 rps
    { duration: '30s', target: 500 },   // ramp up to 500 rps
    { duration: '30s', target: 1000 },  // ramp up to 1000 rps
    { duration: '30s', target: 2000 },  // ramp up to 2000 rps
    { duration: '30s', target: 2000 },  // hold at 2000 rps
    { duration: '30s', target: 0 },     // ramp down
  ],
  thresholds: {
    http_req_failed: ['rate<0.10'],     // less than 10% errors
    http_req_duration: ['p(95)<2000'],  // 95th percentile < 2s
  },
};

const GATEWAY_URL = __ENV.GATEWAY_URL || 'http://localhost:18080';

const payloads = [
  JSON.stringify({ action: 'login', user: 'alice' }),
  JSON.stringify({ action: 'search', query: 'meshcap load test' }),
  JSON.stringify({ action: 'checkout', items: [1, 2, 3], total: 99.99 }),
  JSON.stringify({ action: 'update', field: 'email', value: 'test@example.com' }),
];

export default function () {
  const payload = payloads[Math.floor(Math.random() * payloads.length)];

  const res = http.post(`${GATEWAY_URL}/post`, payload, {
    headers: {
      'Host': 'httpbin.example.com',
      'Content-Type': 'application/json',
    },
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}
