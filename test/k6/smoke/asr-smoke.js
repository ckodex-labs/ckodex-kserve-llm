// test/k6/smoke/asr-smoke.js
// Single-VU health check for the ASR service.
// Full audio transcription requires a real WAV binary; smoke uses GET /health
// to verify the service is reachable and responding before committing to load.
import http from 'k6/http';
import { check } from 'k6';
import { ASR_BASE_URL, DEFAULT_TIMEOUT } from '../lib/config.js';
import { SMOKE_THRESHOLDS } from '../lib/thresholds.js';

export const options = {
  vus: 1,
  duration: '30s',
  thresholds: SMOKE_THRESHOLDS,
};

export default function () {
  const res = http.get(`${ASR_BASE_URL}/health`, { timeout: DEFAULT_TIMEOUT });
  check(res, {
    'ASR health 200': (r) => r.status === 200,
  });
}
