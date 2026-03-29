// k6 performance benchmark for CKodex KServe LLM Operator
// Tests: V2 inference latency, throughput, /v1/chat/completions, /v1/embeddings
//
// Usage: k6 run --env BASE_URL=http://localhost:8080 test/benchmark/k6_benchmark.js
//
import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Trend, Counter, Rate } from 'k6/metrics';

// Custom metrics
const inferenceLatency = new Trend('inference_latency', true);
const tokensPerSecond = new Trend('tokens_per_second');
const inferenceErrors = new Rate('inference_errors');
const totalRequests = new Counter('total_inference_requests');

// Configuration
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const MODEL_NAME = __ENV.MODEL_NAME || 'test-llm';

export const options = {
  scenarios: {
    // Warmup
    warmup: {
      executor: 'constant-vus',
      vus: 2,
      duration: '30s',
      startTime: '0s',
      tags: { scenario: 'warmup' },
    },
    // Steady-state throughput test
    steady_state: {
      executor: 'constant-arrival-rate',
      rate: 50,
      timeUnit: '1s',
      duration: '2m',
      preAllocatedVUs: 20,
      maxVUs: 50,
      startTime: '30s',
      tags: { scenario: 'steady_state' },
    },
    // Spike test
    spike: {
      executor: 'ramping-arrival-rate',
      startRate: 10,
      timeUnit: '1s',
      stages: [
        { duration: '30s', target: 100 },
        { duration: '1m', target: 100 },
        { duration: '30s', target: 10 },
      ],
      preAllocatedVUs: 50,
      maxVUs: 100,
      startTime: '2m30s',
      tags: { scenario: 'spike' },
    },
  },
  thresholds: {
    'inference_latency': ['p(50)<500', 'p(95)<2000', 'p(99)<5000'],
    'inference_errors': ['rate<0.01'],
    'http_req_duration': ['p(95)<3000'],
  },
};

// V2 Protocol - Server Health
export function v2Health() {
  group('V2 Health', () => {
    const liveRes = http.get(`${BASE_URL}/v2/health/live`);
    check(liveRes, { 'server live': (r) => r.status === 200 });

    const readyRes = http.get(`${BASE_URL}/v2/health/ready`);
    check(readyRes, { 'server ready': (r) => r.status === 200 });

    const modelReadyRes = http.get(`${BASE_URL}/v2/models/${MODEL_NAME}/ready`);
    check(modelReadyRes, { 'model ready': (r) => r.status === 200 });
  });
}

// V2 Protocol - Inference
export function v2Infer() {
  const payload = JSON.stringify({
    inputs: [
      {
        name: 'text_input',
        shape: [1],
        datatype: 'BYTES',
        data: ['What is Kubernetes?'],
      },
    ],
    parameters: {
      max_tokens: 128,
      temperature: 0.7,
    },
  });

  const params = {
    headers: { 'Content-Type': 'application/json' },
    tags: { endpoint: 'v2_infer' },
  };

  const start = Date.now();
  const res = http.post(`${BASE_URL}/v2/models/${MODEL_NAME}/infer`, payload, params);
  const latency = Date.now() - start;

  inferenceLatency.add(latency);
  totalRequests.add(1);

  const success = check(res, {
    'v2 infer status 200': (r) => r.status === 200,
    'v2 infer has outputs': (r) => {
      try { return JSON.parse(r.body).outputs.length > 0; }
      catch { return false; }
    },
  });

  if (!success) inferenceErrors.add(1);
  else inferenceErrors.add(0);

  // Calculate tokens/sec if response has token count
  try {
    const body = JSON.parse(res.body);
    if (body.outputs && body.outputs[0] && body.outputs[0].data) {
      const tokenCount = body.outputs[0].data.length;
      tokensPerSecond.add(tokenCount / (latency / 1000));
    }
  } catch {}
}

// OpenAI-compatible /v1/chat/completions
export function chatCompletions() {
  const payload = JSON.stringify({
    model: MODEL_NAME,
    messages: [
      { role: 'user', content: 'Explain Kubernetes operators in one sentence.' },
    ],
    max_tokens: 64,
    temperature: 0.7,
  });

  const params = {
    headers: { 'Content-Type': 'application/json' },
    tags: { endpoint: 'chat_completions' },
  };

  const start = Date.now();
  const res = http.post(`${BASE_URL}/v1/chat/completions`, payload, params);
  const latency = Date.now() - start;

  inferenceLatency.add(latency);
  totalRequests.add(1);

  const success = check(res, {
    'chat status 200': (r) => r.status === 200,
    'chat has choices': (r) => {
      try { return JSON.parse(r.body).choices.length > 0; }
      catch { return false; }
    },
  });

  if (!success) inferenceErrors.add(1);
  else inferenceErrors.add(0);
}

// /v1/embeddings
export function embeddings() {
  const payload = JSON.stringify({
    model: MODEL_NAME,
    input: 'Kubernetes operator pattern',
  });

  const params = {
    headers: { 'Content-Type': 'application/json' },
    tags: { endpoint: 'embeddings' },
  };

  const res = http.post(`${BASE_URL}/v1/embeddings`, payload, params);

  check(res, {
    'embedding status 200': (r) => r.status === 200,
    'embedding has data': (r) => {
      try { return JSON.parse(r.body).data.length > 0; }
      catch { return false; }
    },
  });
}

// Main test function
export default function () {
  group('V2 Protocol', () => {
    v2Health();
    v2Infer();
  });

  group('OpenAI Compatible', () => {
    chatCompletions();
    embeddings();
  });

  sleep(0.1);
}

// Summary output
export function handleSummary(data) {
  return {
    'stdout': textSummary(data, { indent: ' ', enableColors: true }),
    'test/benchmark/results.json': JSON.stringify(data, null, 2),
  };
}

function textSummary(data) {
  const metrics = data.metrics;
  let out = '\n=== CKodex LLM Operator Benchmark Results ===\n\n';

  if (metrics.inference_latency) {
    const m = metrics.inference_latency.values;
    out += `Inference Latency:\n`;
    out += `  P50: ${m['p(50)']?.toFixed(1)}ms\n`;
    out += `  P95: ${m['p(95)']?.toFixed(1)}ms\n`;
    out += `  P99: ${m['p(99)']?.toFixed(1)}ms\n`;
    out += `  Avg: ${m['avg']?.toFixed(1)}ms\n\n`;
  }

  if (metrics.tokens_per_second) {
    const m = metrics.tokens_per_second.values;
    out += `Tokens/sec: avg=${m['avg']?.toFixed(1)}, p95=${m['p(95)']?.toFixed(1)}\n\n`;
  }

  if (metrics.total_inference_requests) {
    out += `Total Requests: ${metrics.total_inference_requests.values.count}\n`;
  }

  if (metrics.inference_errors) {
    out += `Error Rate: ${(metrics.inference_errors.values.rate * 100).toFixed(2)}%\n`;
  }

  return out;
}
