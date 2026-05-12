// test/k6/stress/latency-ramp.js
// Ramping load test to find the saturation point (The "Knee").
// Measures P50, P90, P99 latency and tokens/sec under increasing load.
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.2/index.js';
import { LLM_BASE_URL, LLM_MODEL, DEFAULT_TIMEOUT } from '../lib/config.js';
import { buildChatPayload } from '../lib/inference.js';
import { STRESS_THRESHOLDS } from '../lib/thresholds.js';

// Custom trends for deep latency analysis
const ttftTrend = new Trend('ttft_ms', true);
const tpsTrend  = new Trend('tokens_per_sec', true);
const errorCounter = new Counter('errors');

export const options = {
  stages: [
    { duration: '2m', target: 20 },  // Phase 1: Warmup / Low load
    { duration: '5m', target: 50 },  // Phase 2: Ramping to saturation
    { duration: '3m', target: 100 }, // Phase 3: Stress / Breaking point
    { duration: '2m', target: 0 },   // Phase 4: Cool down
  ],
  thresholds: STRESS_THRESHOLDS,
};

export default function () {
  // We use stream: true to simulate real-world streaming deployments
  const payload = buildChatPayload(LLM_MODEL, 'Discuss the future of sovereign AI.', 128, true);
  const params = {
    headers: { 'Content-Type': 'application/json' },
    timeout: DEFAULT_TIMEOUT,
  };

  const start = Date.now();
  const response = http.post(`${LLM_BASE_URL}/v1/chat/completions`, payload, params);
  const end = Date.now();

  const success = check(response, {
    'is status 200': (r) => r.status === 200,
    'has data': (r) => r.body.length > 0,
  });

  if (!success) {
    errorCounter.add(1);
    return;
  }

  try {
    // In k6, for SSE, we parse the chunks if k6 collected them. 
    // Standard vLLM SSE format: data: {"id": "...", "choices": [{"delta": {"content": "..."}}]}
    const chunks = response.body.split('\n\n');
    let firstTokenTime = 0;
    let totalTokens = 0;

    for (const chunk of chunks) {
      if (chunk.startsWith('data: ')) {
        const dataStr = chunk.slice(6);
        if (dataStr === '[DONE]') break;
        
        const data = JSON.parse(dataStr);
        totalTokens++;
        
        // The first chunk with content is our TTFT proxy in this simulation
        if (firstTokenTime === 0 && data.choices && data.choices[0].delta && data.choices[0].delta.content) {
          // Note: Standard k6 http.post returns the FULL body after completion.
          // To get REAL TTFT, we would need k6/experimental/streams or similar.
          // Here we use a conservative estimate based on the response timing.
          firstTokenTime = end - start; 
        }
      }
    }

    if (totalTokens > 0) {
      ttftTrend.add(firstTokenTime);
      tpsTrend.add(totalTokens / ((end - start) / 1000));
    }
  } catch (e) {
    // Parsing might fail if the response isn't pure SSE or is truncated
    errorCounter.add(1);
  }

  // Pacing: wait a bit between requests to simulate human interaction
  sleep(Math.random() * 2 + 1);
}

export function handleSummary(data) {
  return {
    'test/k6/results/stress-ramp-summary.json': JSON.stringify(data, null, 2),
    stdout: textSummary(data, { indent: ' ', enableColors: true }),
  };
}
