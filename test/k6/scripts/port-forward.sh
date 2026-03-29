#!/usr/bin/env bash
# test/k6/scripts/port-forward.sh
# Applies test CRs, waits for Ready, then starts kubectl port-forwards.
# Port assignments:
#   8000 → k6-llm-test   (LLMInferenceService)
#   7997 → k6-embed-test (EmbeddingInferenceService)
#   8001 → k6-asr-test   (ASRInferenceService, aliased to avoid collision with LLM)
set -euo pipefail

NS="${K6_NAMESPACE:-ckodex-inference}"
PID_FILE="${TMPDIR:-/tmp}/k6-pf.pids"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

echo "==> Applying test CRs to namespace ${NS}"
kubectl apply -f "${REPO_ROOT}/test/k6/k8s/" -n "${NS}"

echo "==> Waiting for k6-llm-test Ready (up to 10m)"
kubectl wait llminferenceservice k6-llm-test -n "${NS}" \
  --for=condition=Ready --timeout=600s

echo "==> Waiting for k6-embed-test Ready (up to 5m)"
kubectl wait embeddinginferenceservice k6-embed-test -n "${NS}" \
  --for=condition=Ready --timeout=300s

echo "==> Waiting for k6-asr-test Ready (up to 5m)"
kubectl wait asrinferenceservice k6-asr-test -n "${NS}" \
  --for=condition=Ready --timeout=300s

# Clear any stale PID file from a previous run.
rm -f "${PID_FILE}"

echo "==> Starting port-forwards"
kubectl port-forward svc/k6-llm-test   8000:8000 -n "${NS}" \
  &>"${TMPDIR:-/tmp}/pf-llm.log"   & echo $! >> "${PID_FILE}"
kubectl port-forward svc/k6-embed-test 7997:7997 -n "${NS}" \
  &>"${TMPDIR:-/tmp}/pf-embed.log" & echo $! >> "${PID_FILE}"
kubectl port-forward svc/k6-asr-test   8001:8000 -n "${NS}" \
  &>"${TMPDIR:-/tmp}/pf-asr.log"   & echo $! >> "${PID_FILE}"

# Allow port-forwards time to bind before callers proceed.
sleep 3

echo "==> Port-forwards ready. PIDs: $(tr '\n' ' ' < "${PID_FILE}")"
echo "    LLM  → http://localhost:8000"
echo "    Embed → http://localhost:7997"
echo "    ASR  → http://localhost:8001"
