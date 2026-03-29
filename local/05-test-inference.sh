#!/usr/bin/env bash
set -euo pipefail
kubectl apply -f 04-llm-inference-service.yaml
kubectl wait --for=condition=Ready llminferenceservice/llama3-8b --timeout=600s

# Option A: via MetalLB IP (through Gateway)
GW_IP=$(kubectl get gateway llama3-8b-gateway -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)
if [ -n "$GW_IP" ]; then
  echo "Serving via Gateway at http://$GW_IP"
  curl -X POST "http://$GW_IP/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{
      "model": "qwen-0.5b",
      "messages": [{"role": "user", "content": "Hello KServe v0.17!"}],
      "max_tokens": 50
    }' | jq
else
  # Option B: via port-forward (direct to Service, bypasses Gateway)
  echo "Gateway IP not available, using port-forward to Service"
  kubectl port-forward svc/llama3-8b 8080:80 &
  PF_PID=$!
  sleep 2
  curl -X POST http://localhost:8080/v1/chat/completions \
    -H "Content-Type: application/json" \
    -d '{
      "model": "qwen-0.5b",
      "messages": [{"role": "user", "content": "Hello KServe v0.17!"}],
      "max_tokens": 50
    }' | jq
  kill $PF_PID 2>/dev/null || true
fi
