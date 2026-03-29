// test/k6/smoke/llm-smoke.js
// Single-VU sanity check for the LLM /v1/chat/completions endpoint.
// Uses GPT-2 (CPU image) — expects a response within 30s under no load.
import http from 'k6/http';
import { check } from 'k6';
import { LLM_BASE_URL, DEFAULT_TIMEOUT } from '../lib/config.js';
import { buildChatPayload } from '../lib/inference.js';
import { SMOKE_THRESHOLDS } from '../lib/thresholds.js';

export const options = {
  vus: 1,
  duration: '30s',
  thresholds: SMOKE_THRESHOLDS,
};

const HEADERS = { 'Content-Type': 'application/json' };

export default function () {
  const res = http.post(
    `${LLM_BASE_URL}/v1/chat/completions`,
    buildChatPayload(),
    { headers: HEADERS, timeout: DEFAULT_TIMEOUT },
  );
  check(res, {
    'status 200':          (r) => r.status === 200,
    'has choices':         (r) => JSON.parse(r.body)?.choices?.length > 0,
    'model field present': (r) => !!JSON.parse(r.body)?.model,
  });
}
