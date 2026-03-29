import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend } from 'k6/metrics';

// Custom metric to track Time to First Token (TTFT) approximately
const ttftTrend = new Trend('ttft_ms');

export const options = {
    stages: [
        { duration: '30s', target: 100 },  // Ramp-up to 100 users
        { duration: '1m', target: 500 },   // Scale to 500 users
        { duration: '2m', target: 1000 },  // Target 1000 concurrent users
        { duration: '1m', target: 0 },     // Ramp-down
    ],
    thresholds: {
        http_req_duration: ['p(95)<500'], // 95% of requests should be below 500ms
        http_req_failed: ['rate<0.01'],   // Error rate should be < 1%
    },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const MODEL_NAME = __ENV.MODEL_NAME || 'llama3-8b';

export default function () {
    const payload = JSON.stringify({
        model: MODEL_NAME,
        messages: [
            { role: 'user', content: 'What is the capital of Canada?' }
        ],
        max_tokens: 50,
        stream: false,
    });

    const params = {
        headers: {
            'Content-Type': 'application/json',
        },
    };

    const startTime = Date.now();
    const res = http.post(`${BASE_URL}/v1/chat/completions`, payload, params);
    const endTime = Date.now();

    check(res, {
        'is status 200': (r) => r.status === 200,
    });

    // Log "TTFT" (for non-streaming, this is the full duration)
    ttftTrend.add(endTime - startTime);

    sleep(1);
}
