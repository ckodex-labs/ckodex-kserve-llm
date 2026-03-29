// test/k6/lib/thresholds.js
// SLO threshold objects per test tier.
//
// Smoke: 1 VU, sanity baseline — any response under 30s is acceptable on CPU.
//   abortOnFail: true stops the run immediately on breach.
//
// Stress: ramping concurrency — allow higher latency, watch error rate.
//   No abortOnFail: stress deliberately pushes past comfortable operating point.
//
// Benchmark: sustained load — tight p99, capture custom throughput metrics.
//   tokens_per_second and time_to_first_token_ms are custom Trend metrics
//   defined in the benchmark scripts themselves.

export const SMOKE_THRESHOLDS = {
  http_req_failed:   [{ threshold: 'rate<0.01', abortOnFail: true }],
  http_req_duration: [{ threshold: 'p(99)<30000', abortOnFail: true }],
};

export const STRESS_THRESHOLDS = {
  http_req_failed:   [{ threshold: 'rate<0.05' }],
  http_req_duration: [{ threshold: 'p(95)<60000' }],
};

export const BENCH_THRESHOLDS = {
  http_req_failed:        [{ threshold: 'rate<0.01' }],
  http_req_duration:      [{ threshold: 'p(99)<45000' }, { threshold: 'p(50)<20000' }],
  tokens_per_second:      [{ threshold: 'avg>0.5' }],
  time_to_first_token_ms: [{ threshold: 'p(95)<5000' }],
};
