// test/k6/benchmark/hardening-overhead.js
// Side-by-side benchmark comparing "Vanilla" vs "Hardened" InferenceServices.
// Quantifies the latency overhead (P99 penalty) introduced by SPIRE mTLS + OPA.
import http from 'k6/http';
import { check, group } from 'k6';
import { Trend } from 'k6/metrics';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.2/index.js';
import { LLM_MODEL, DEFAULT_TIMEOUT } from '../lib/config.js';
import { buildChatPayload } from '../lib/inference.js';

// Custom metrics for side-by-side comparison
const vanillaLatencyTrend = new Trend('latency_vanilla_ms',  true);
const hardenedLatencyTrend = new Trend('latency_hardened_ms', true);

export const options = {
  vus: 10,
  duration: '5m',
};

const BASE_DOMAIN = 'ckodex.local';
const VANILLA_URL  = `http://llm-vanilla.default.svc.${BASE_DOMAIN}:8000`;
const HARDENED_URL = `https://llm-hardened.default.svc.${BASE_DOMAIN}:8443`;

export default function () {
  const payload = buildChatPayload(LLM_MODEL, 'Compare two systems.', 32, false);
  const params = {
    headers: { 'Content-Type': 'application/json' },
    timeout: DEFAULT_TIMEOUT,
  };

  // 1. Measure Vanilla (Baseline)
  group('Vanilla Baseline', function () {
    const start = Date.now();
    const res = http.post(`${VANILLA_URL}/v1/chat/completions`, payload, params);
    const end = Date.now();
    
    const ok = check(res, { 'vanilla status 200': (r) => r.status === 200 });
    if (ok) {
        vanillaLatencyTrend.add(end - start);
    }
  });

  // 2. Measure Hardened (With SPIRE mTLS + OPA Enforcement)
  group('Hardened Sovereign', function () {
    const start = Date.now();
    const res = http.post(`${HARDENED_URL}/v1/chat/completions`, payload, params);
    const end = Date.now();
    
    const ok = check(res, { 'hardened status 200': (r) => r.status === 200 });
    if (ok) {
        hardenedLatencyTrend.add(end - start);
    }
  });
}

export function handleSummary(data) {
  // We compute the "Hardening Overhead Penalty" from the metrics
  const vP50 = data.metrics.latency_vanilla_ms.values['p(50)'];
  const hP50 = data.metrics.latency_hardened_ms.values['p(50)'];
  const penalty = hP50 > 0 ? ((hP50 - vP50) / vP50) * 100 : 0;

  console.log(`---------------------------------------------------------`);
  console.log(`🛡️  Hardening Overhead Audit`);
  console.log(`   [Vanilla P50]:  ${vP50} ms`);
  console.log(`   [Hardened P50]: ${hP50} ms`);
  console.log(`   [Penalty]:     +${penalty.toFixed(2)}% Latency`);
  console.log(`---------------------------------------------------------`);

  return {
    'test/k6/results/hardening-overhead.json': JSON.stringify(data, null, 2),
    stdout: textSummary(data, { indent: ' ', enableColors: true }),
  };
}
