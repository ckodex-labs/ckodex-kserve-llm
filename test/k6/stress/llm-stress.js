// test/k6/stress/llm-stress.js
// Ramping-VU stress test for the LLM /v1/chat/completions endpoint.
// GPT-2 on CPU saturates at ~3–5 concurrent requests; the ramp deliberately
// pushes past that to observe degradation characteristics.
// Accepts HTTP 429 (rate limited) as a valid response — not a service failure.
import http from 'k6/http';
import { check, sleep } from 'k6';
import { LLM_BASE_URL, LLM_MODEL, DEFAULT_TIMEOUT } from '../lib/config.js';
import { buildChatPayload } from '../lib/inference.js';
import { STRESS_THRESHOLDS } from '../lib/thresholds.js';

export const options = {
  stages: [
    { duration: '1m', target: 2  },  // warm-up
    { duration: '2m', target: 5  },  // light load
    { duration: '2m', target: 10 },  // moderate load
    { duration: '2m', target: 20 },  // stress — expect latency degradation
    { duration: '1m', target: 0  },  // cool-down
  ],
  thresholds: STRESS_THRESHOLDS,
};

const HEADERS = { 'Content-Type': 'application/json' };

export default function () {
  const res = http.post(
    `${LLM_BASE_URL}/v1/chat/completions`,
    buildChatPayload(LLM_MODEL, 'Count to 3.', 10),
    { headers: HEADERS, timeout: DEFAULT_TIMEOUT },
  );
  check(res, {
    'status 200 or 429': (r) => r.status === 200 || r.status === 429,
    'not 500':           (r) => r.status !== 500,
  });
  sleep(1);
}
