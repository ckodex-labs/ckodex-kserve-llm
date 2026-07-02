#!/usr/bin/env bash
# redeploy-harness.sh — High-Assurance DEVx/OX/GOVx Automation
# Purpose: Full teardown and redeploy cycle for stability and governance verification.

set -euo pipefail

# Configuration
NAMESPACE=${NAMESPACE:-default}
MODEL_URI=${MODEL_URI:-"hf://meta-llama/Llama-2-7b-hf"}
MODEL_NAME=${MODEL_NAME:-"llama2-7b"}
ITERATIONS=${ITERATIONS:-1}

echo "🚀 Starting High-Assurance Redeploy Harness (DEVx/OX/GOVx)"

function teardown() {
    echo "🧹 Teardown: Cleaning up existing resources..."
    
    # Force remove finalizers to prevent hanging if operator is down
    echo "🛡️ Cleaning finalizers for $MODEL_NAME..."
    kubectl patch llminferenceservice "$MODEL_NAME" -p '{"metadata":{"finalizers":null}}' --type=merge -n "$NAMESPACE" --ignore-not-found || true
    kubectl patch localmodelcache "$MODEL_NAME" -p '{"metadata":{"finalizers":null}}' --type=merge -n "$NAMESPACE" --ignore-not-found || true
    
    # Delete resources
    kubectl delete llminferenceservice --all -n "$NAMESPACE" --ignore-not-found --timeout=30s || true
    kubectl delete localmodelcache --all -n "$NAMESPACE" --ignore-not-found --timeout=30s || true
    
    # Optional cleanup for KEDA and Tetragon (if CRDs exist)
    kubectl delete scaledobject --all -n "$NAMESPACE" --ignore-not-found --timeout=30s 2>/dev/null || true
    kubectl delete tracingpolicy --all --ignore-not-found --timeout=30s 2>/dev/null || true
    
    # Final cleanup of any hanging pods
    kubectl delete pods -n "$NAMESPACE" -l "app.kubernetes.io/instance=$MODEL_NAME" --grace-period=0 --force --ignore-not-found 2>/dev/null || true
    
    echo "✅ Teardown complete."
}

function deploy() {
    echo "📦 Deploy: Building manager binary..."
    go build -o bin/manager -ldflags="-s -w" cmd/manager/main.go
    
    echo "📦 Deploy: Installing CRDs and RBAC..."
    make install
    
    echo "📦 Deploy: Starting operator in background..."
    # Killing any existing manager process
    pkill manager || true
    ./bin/manager & 
    MANAGER_PID=$!
    echo "🚀 Operator started (PID: $MANAGER_PID)"

    echo "📦 Deploy: Applying LLMInferenceService..."
    cat <<EOF | kubectl apply -f -
apiVersion: serving.ckodex.com/v1alpha2
kind: LLMInferenceService
metadata:
  name: "$MODEL_NAME"
  namespace: "$NAMESPACE"
spec:
  model:
    uri: "$MODEL_URI"
    name: "$MODEL_NAME"
  autoOptimize: true
  replicas: 1
  template:
    spec:
      containers:
      - name: vllm
        image: vllm/vllm-openai:v0.24.0
        resources:
          limits:
            cpu: "8"
            memory: "32Gi"
  router:
    gateway:
      managed:
        gatewayClassName: envoy
    route:
      httpRoute: {}
    scheduler:
      pool: {}
  scaling:
    minReplicas: 1
    maxReplicas: 5
    keda: {}
EOF
}

function verify() {
    echo "🔍 Verify: Waiting for ModelReady status..."
    # We wait up to 300s for the model to be ready. 
    # Use a loop if wait command fails on CRD issues
    for i in {1..30}; do
        READY=$(kubectl get llminferenceservice "$MODEL_NAME" -n "$NAMESPACE" -o jsonpath='{.status.modelReady}' 2>/dev/null || echo "false")
        if [[ "$READY" == "true" ]]; then
            echo "✅ Model is Ready!"
            break
        fi
        echo "⏳ Waiting... ($i/30)"
        sleep 10
    done
    
    if [[ "$READY" != "true" ]]; then
        echo "❌ Timeout waiting for model readiness."
        exit 1
    fi
    
    echo "📈 Verify: Running k6 load test..."
    # In a real environment, we'd wait for the Gateway URL. 
    # For KIND, we use the local service URL if reachable.
    k6 run -e MODEL_NAME="$MODEL_NAME" test/k6-load-test.js
}

function emit_evidence() {
    echo "📜 GOVx: Emitting Evidence Bundle..."
    TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    echo "{\"timestamp\": \"$TIMESTAMP\", \"status\": \"PASS\", \"iteration\": $1, \"model\": \"$MODEL_NAME\"}" > "evidence-iteration-$1.json"
}

# Run cycles
for i in $(seq 1 "$ITERATIONS"); do
    echo "🔄 --- Iteration $i of $ITERATIONS ---"
    teardown
    deploy
    verify
    emit_evidence "$i"
done

# Cleanup background manager on finish
if [[ -n "${MANAGER_PID:-}" ]]; then
    kill "$MANAGER_PID" || true
fi

echo "🎉 All $ITERATIONS cycles completed successfully!"
