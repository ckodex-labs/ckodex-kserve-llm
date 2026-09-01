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
  local revision
  local observed_revision=""
  local deployment_generation
  local observed_generation

  deadline=$(( $(date +%s) + timeout_seconds ))
  while (( $(date +%s) < deadline )); do
    deployment_generation="$(kubectl get deployment llama3-8b -n default \
      -o jsonpath='{.metadata.generation}' 2>/dev/null || true)"
    observed_generation="$(kubectl get deployment llama3-8b -n default \
      -o jsonpath='{.status.observedGeneration}' 2>/dev/null || true)"
    if [[ -z "$deployment_generation" || "$deployment_generation" != "$observed_generation" ]]; then
      sleep 2
      continue
    fi
    # Resolve the newest ReplicaSet first. Reused clusters can retain an old
    # failing pod while a corrected Deployment revision is being rolled out.
    revision="$(kubectl get rs -n default -l "$selector" \
      --sort-by=.metadata.creationTimestamp \
      -o jsonpath='{.items[-1].metadata.labels.pod-template-hash}' 2>/dev/null || true)"
    if [[ -z "$revision" ]]; then
      sleep 2
      continue
    fi
    if [[ "$revision" != "$observed_revision" ]]; then
      echo "Waiting for inference pod from ReplicaSet ${revision}"
      observed_revision="$revision"
    fi
    pod_name="$(kubectl get pods -n default -l "$selector,pod-template-hash=$revision" \
      -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
    if [[ -n "$pod_name" ]]; then
      remaining=$(( deadline - $(date +%s) ))
      if (( remaining < 1 )); then
        remaining=1
      fi
      # Use short waits so a newer ReplicaSet can supersede this pod while the
      # model is initializing. The outer deadline remains the single budget.
      if kubectl wait --for=condition=Ready "pod/${pod_name}" \
        -n default --timeout=5s >/dev/null 2>&1; then
        return
      fi
    fi
    sleep 2
  done

  echo "timed out waiting for pod matching ${selector}" >&2
  kubectl get rs -n default -l "$selector" -o wide >&2 || true
  kubectl get pods -n default -l "$selector" -o wide >&2 || true
  return 1
}

probe_inference_endpoint() {
  local endpoint="$1"
  local hostname="${2:-}"
  local retry_count="${3:-10}"
  local connect_timeout="${4:-10}"
  local max_time="${5:-180}"
  local -a curl_args=(
    --fail-with-body
    --silent
    --show-error
    --retry "$retry_count"
    --retry-delay 2
    --retry-all-errors
    --retry-connrefused
    --connect-timeout "$connect_timeout"
    --max-time "$max_time"
    -X POST
    -H "Content-Type: application/json"
  )
  local payload='{
    "model": "gpt2",
    "prompt": "Hello KServe v0.17!",
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
  if ! probe_inference_endpoint "http://${GW_IP}/v1/completions" \
    "llama3-8b.ckodex.com" 0 3 5; then
    echo "Gateway address is not reachable from this host; using a port-forward to the Gateway proxy"
    GATEWAY_SERVICE=$(kubectl get svc -n envoy-gateway-system \
      -l 'gateway.envoyproxy.io/owning-gateway-name=llama3-8b-gateway,gateway.envoyproxy.io/owning-gateway-namespace=default' \
      -o jsonpath='{.items[0].metadata.name}')
    if [[ -z "$GATEWAY_SERVICE" ]]; then
      echo "no Envoy Gateway proxy Service found for llama3-8b-gateway" >&2
      exit 1
    fi
    kubectl port-forward "service/${GATEWAY_SERVICE}" -n envoy-gateway-system 8080:80 &
    PF_PID=$!
    probe_inference_endpoint "http://localhost:8080/v1/completions" \
      "llama3-8b.ckodex.com"
  fi
else
  # Option B: via port-forward (direct to Service, bypasses Gateway)
  echo "Gateway IP not available, using port-forward to Service"
  kubectl port-forward svc/llama3-8b -n default 8080:80 &
  PF_PID=$!
  probe_inference_endpoint "http://localhost:8080/v1/completions"
fi
