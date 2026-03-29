// test/k6/benchmark/llm-benchmark.js
// Sustained constant-VU benchmark for LLM /v1/chat/completions.
// Captures tokens/sec and a TTFT proxy metric (total latency / completion_tokens).
// Results are written to test/k6/results/llm-benchmark-summary.json.
import http from 'k6/http';
import { check } from 'k6';
import { Trend, Counter } from 'k6/metrics';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.2/index.js';
import { LLM_BASE_URL, LLM_MODEL, DEFAULT_TIMEOUT } from '../lib/config.js';
import { buildChatPayload } from '../lib/inference.js';
import { BENCH_THRESHOLDS } from '../lib/thresholds.js';

// Custom metrics — reported alongside built-in k6 metrics in the summary.
const tokensPerSecond  = new Trend('tokens_per_second',      true);
const timeToFirstToken = new Trend('time_to_first_token_ms', true);
const totalTokensOut   = new Counter('total_tokens_generated');

export const options = {
  vus: 10,
  duration: '10m',
  thresholds: BENCH_THRESHOLDS,
};

const HEADERS = { 'Content-Type': 'application/json' };

export default function () {
  const payload = buildChatPayload(LLM_MODEL, 'List three fruits.', 40);
  const start = Date.now();

  const res = http.post(
    `${LLM_BASE_URL}/v1/chat/completions`,
    payload,
    { headers: HEADERS, timeout: DEFAULT_TIMEOUT },
  );

  const elapsed = Date.now() - start;
  const ok = check(res, { 'status 200': (r) => r.status === 200 });

  if (ok) {
    const body = JSON.parse(res.body);
    const outTokens = body?.usage?.completion_tokens ?? 0;

    if (outTokens > 0 && elapsed > 0) {
      tokensPerSecond.add(outTokens / (elapsed / 1000));
      totalTokensOut.add(outTokens);
      // TTFT proxy: total latency / completion_tokens (ms per token).
      // Lower values indicate faster prefill + first-token decode speed.
      timeToFirstToken.add(elapsed / outTokens);
    }
  }
}

export function handleSummary(data) {
  return {
    'test/k6/results/llm-benchmark-summary.json': JSON.stringify(data, null, 2),
    stdout: textSummary(data, { indent: ' ', enableColors: true }),
  };
}
