// test/k6/lib/inference.js
// Request body builders for each inference endpoint type.
// ASR uses multipart/form-data; all others use application/json.

import http from 'k6/http';
import { LLM_MODEL, EMBED_MODEL, MM_MODEL } from './config.js';

export function buildChatPayload(model = LLM_MODEL, prompt = 'ping', maxTokens = 20) {
  return JSON.stringify({
    model,
    messages: [{ role: 'user', content: prompt }],
    max_tokens: maxTokens,
    temperature: 0.0,
  });
}

export function buildEmbedPayload(model = EMBED_MODEL, inputs = ['The quick brown fox']) {
  return JSON.stringify({ model, input: inputs });
}

// ASR uses multipart/form-data. audioBytes must be an ArrayBuffer or Uint8Array.
export function buildASRPayload(audioBytes, model = 'Systran/faster-whisper-tiny') {
  const fd = new FormData();
  fd.append('model', model);
  fd.append('language', 'en');
  fd.append('response_format', 'json');
  fd.append('file', http.file(audioBytes, 'sample.wav', 'audio/wav'));
  return fd;
}

export function buildMMPayload(model = MM_MODEL, imageUrl = '', prompt = 'Describe this image.') {
  return JSON.stringify({
    model,
    messages: [{
      role: 'user',
      content: [
        { type: 'image_url', image_url: { url: imageUrl } },
        { type: 'text', text: prompt },
      ],
    }],
    max_tokens: 30,
    temperature: 0.0,
  });
}
