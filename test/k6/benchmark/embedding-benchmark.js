// test/k6/benchmark/embedding-benchmark.js
// Sustained 20-VU benchmark measuring embedding throughput (sentences/sec).
// Batch size = 32 simulates a production RAG indexing pipeline.
// Results are written to test/k6/results/embedding-benchmark-summary.json.
import http from 'k6/http';
import { check } from 'k6';
import { Trend, Counter } from 'k6/metrics';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.2/index.js';
import { EMBED_BASE_URL, DEFAULT_TIMEOUT } from '../lib/config.js';
import { buildEmbedPayload } from '../lib/inference.js';
import { BENCH_THRESHOLDS } from '../lib/thresholds.js';

const sentencesPerSecond = new Trend('sentences_per_second',    true);
const totalSentences     = new Counter('total_sentences_embedded');

const BATCH_SIZE = 32;

export const options = {
  vus: 20,
  duration: '5m',
  thresholds: {
    ...BENCH_THRESHOLDS,
    // Override token metrics (not applicable to embeddings) and add embedding SLO.
    tokens_per_second:      [],
    time_to_first_token_ms: [],
    sentences_per_second:   [{ threshold: 'avg>5' }],
  },
};

const BATCH = Array.from({ length: BATCH_SIZE }, (_, i) =>
  `Benchmark sentence ${i + 1}: the model encodes this efficiently for similarity search.`,
);

export default function () {
  const start = Date.now();
  const res = http.post(
    `${EMBED_BASE_URL}/v1/embeddings`,
    buildEmbedPayload(undefined, BATCH),
    { headers: { 'Content-Type': 'application/json' }, timeout: DEFAULT_TIMEOUT },
  );
  const elapsed = Date.now() - start;

  const ok = check(res, {
    'status 200':         (r) => r.status === 200,
    'correct batch size': (r) => JSON.parse(r.body)?.data?.length === BATCH_SIZE,
  });

  if (ok && elapsed > 0) {
    sentencesPerSecond.add(BATCH_SIZE / (elapsed / 1000));
    totalSentences.add(BATCH_SIZE);
  }
}

export function handleSummary(data) {
  return {
    'test/k6/results/embedding-benchmark-summary.json': JSON.stringify(data, null, 2),
    stdout: textSummary(data, { indent: ' ', enableColors: true }),
  };
}
