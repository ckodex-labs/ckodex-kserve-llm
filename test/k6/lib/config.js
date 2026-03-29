// test/k6/lib/config.js
// Base URLs resolved from environment variables — override per environment.
// CPU inference is slow; DEFAULT_TIMEOUT covers worst-case single-request latency.

export const LLM_BASE_URL   = __ENV.K6_LLM_BASE_URL   || 'http://localhost:8000';
export const EMBED_BASE_URL = __ENV.K6_EMBED_BASE_URL  || 'http://localhost:7997';
export const ASR_BASE_URL   = __ENV.K6_ASR_BASE_URL    || 'http://localhost:8001';
export const MM_BASE_URL    = __ENV.K6_MM_BASE_URL     || 'http://localhost:8002';

export const LLM_MODEL   = __ENV.K6_LLM_MODEL   || 'gpt2';
export const EMBED_MODEL  = __ENV.K6_EMBED_MODEL  || 'BAAI/bge-large-en-v1.5';
export const ASR_MODEL    = __ENV.K6_ASR_MODEL    || 'Systran/faster-whisper-tiny';
export const MM_MODEL     = __ENV.K6_MM_MODEL     || 'llava-hf/llava-v1.6-mistral-7b-hf';

// 60s covers CPU-only inference with no GPU acceleration.
export const DEFAULT_TIMEOUT = '60s';
