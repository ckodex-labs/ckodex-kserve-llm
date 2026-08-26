#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PF_PID=""

cleanup_port_forward() {
  if [[ -n "$PF_PID" ]]; then
    kill "$PF_PID" 2>/dev/null || true
    wait "$PF_PID" 2>/dev/null || true
  fi
}

trap cleanup_port_forward EXIT

wait_for_inference_pod() {
  local selector="$1"
  local timeout_seconds="$2"
  local deadline
  local pod_name
  local remaining

  deadline=$(( $(date +%s) + timeout_seconds ))
  while (( $(date +%s) < deadline )); do
    pod_name="$(kubectl get pods -n default -l "$selector" \
      -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
    if [[ -n "$pod_name" ]]; then
      remaining=$(( deadline - $(date +%s) ))
      if (( remaining < 1 )); then
        remaining=1
      fi
      kubectl wait --for=condition=Ready "pod/${pod_name}" \
        -n default --timeout="${remaining}s"
      return
    fi
    sleep 2
  done

  echo "timed out waiting for pod matching ${selector}" >&2
  kubectl get pods -n default -l "$selector" -o wide >&2 || true
  return 1
}

probe_inference_endpoint() {
  local endpoint="$1"
  local hostname="${2:-}"
  local -a curl_args=(
    --fail-with-body
    --silent
    --show-error
    --retry 10
    --retry-delay 2
    --retry-connrefused
    --connect-timeout 10
    --max-time 180
    -X POST
    -H "Content-Type: application/json"
  )
  local payload='{
    "model": "qwen-0.5b",
    "messages": [{"role": "user", "content": "Hello KServe v0.17!"}],
    "max_tokens": 50
  }'

  if [[ -n "$hostname" ]]; then
    curl_args+=( -H "Host: ${hostname}" )
  fi

  curl "${curl_args[@]}" \
    --data "$payload" \
    "$endpoint" \
    | jq -e 'if ((.choices? // []) | length) > 0 then . else error("inference response did not contain choices") end'
}

kubectl annotate llminferenceservice llama3-8b -n default \
  ckodex.dev/e2e-run="$(date +%s)" --overwrite >/dev/null 2>&1 || true
kubectl apply -f "${SCRIPT_DIR}/04-llm-inference-service.yaml"
wait_for_inference_pod "app.kubernetes.io/instance=llama3-8b" 600
kubectl annotate llminferenceservice llama3-8b -n default \
  ckodex.dev/e2e-ready="$(date +%s)" --overwrite >/dev/null 2>&1 || true
kubectl wait --for=condition=Ready llminferenceservice/llama3-8b \
  -n default --timeout=600s

# Option A: via MetalLB IP (through Gateway)
GW_IP=$(kubectl get gateway llama3-8b-gateway -n default \
  -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)
if [ -n "$GW_IP" ]; then
  echo "Serving via Gateway at http://$GW_IP"
  probe_inference_endpoint "http://${GW_IP}/v1/chat/completions" "llama3-8b.ckodex.com"
else
  # Option B: via port-forward (direct to Service, bypasses Gateway)
  echo "Gateway IP not available, using port-forward to Service"
  kubectl port-forward svc/llama3-8b -n default 8080:80 &
  PF_PID=$!
  probe_inference_endpoint "http://localhost:8080/v1/chat/completions"
fi
