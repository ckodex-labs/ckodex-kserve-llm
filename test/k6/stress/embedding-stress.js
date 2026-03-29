// test/k6/stress/embedding-stress.js
// Ramping-VU stress test for the Embedding /v1/embeddings endpoint.
// Embedding servers handle much higher concurrency than LLM — ramp to 100 VUs.
// Each VU sends a batch of 8 inputs, simulating a realistic RAG indexing workload.
import http from 'k6/http';
import { check, sleep } from 'k6';
import { EMBED_BASE_URL, DEFAULT_TIMEOUT } from '../lib/config.js';
import { buildEmbedPayload } from '../lib/inference.js';
import { STRESS_THRESHOLDS } from '../lib/thresholds.js';

export const options = {
  stages: [
    { duration: '30s', target: 5   },
    { duration: '1m',  target: 20  },
    { duration: '2m',  target: 50  },
    { duration: '1m',  target: 100 },
    { duration: '30s', target: 0   },
  ],
  thresholds: STRESS_THRESHOLDS,
};

// 8 inputs per request — realistic document-chunking batch size.
const BATCH = Array.from({ length: 8 }, (_, i) =>
  `Sample document sentence number ${i + 1} for stress testing embedding throughput.`,
);

export default function () {
  const res = http.post(
    `${EMBED_BASE_URL}/v1/embeddings`,
    buildEmbedPayload(undefined, BATCH),
    { headers: { 'Content-Type': 'application/json' }, timeout: DEFAULT_TIMEOUT },
  );
  check(res, {
    'status 200':     (r) => r.status === 200,
    'batch returned': (r) => JSON.parse(r.body)?.data?.length === BATCH.length,
  });
  sleep(0.2);
}
