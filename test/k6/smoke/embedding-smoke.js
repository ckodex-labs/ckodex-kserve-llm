// test/k6/smoke/embedding-smoke.js
// Single-VU sanity check for the Embedding /v1/embeddings endpoint.
// Uses bge-large-en-v1.5 via infinity runtime.
import http from 'k6/http';
import { check } from 'k6';
import { EMBED_BASE_URL, DEFAULT_TIMEOUT } from '../lib/config.js';
import { buildEmbedPayload } from '../lib/inference.js';
import { SMOKE_THRESHOLDS } from '../lib/thresholds.js';

export const options = {
  vus: 1,
  duration: '30s',
  thresholds: SMOKE_THRESHOLDS,
};

export default function () {
  const res = http.post(
    `${EMBED_BASE_URL}/v1/embeddings`,
    buildEmbedPayload(),
    { headers: { 'Content-Type': 'application/json' }, timeout: DEFAULT_TIMEOUT },
  );
  check(res, {
    'status 200':         (r) => r.status === 200,
    'has embedding data': (r) => JSON.parse(r.body)?.data?.[0]?.embedding?.length > 0,
  });
}
