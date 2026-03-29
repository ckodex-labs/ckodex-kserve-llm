#!/bin/bash
# 05-upload-to-seaweed.sh
# Populates SeaweedFS with model weights and adapters using kubectl exec

set -euo pipefail

NAMESPACE="storage"
log() { echo "[$(date +'%H:%M:%S')] $*"; }

# 1. Wait for SeaweedFS Filer
log "Waiting for SeaweedFS Filer readiness..."
kubectl wait --for=condition=Ready pod -l app=seaweedfs -n $NAMESPACE --timeout=60s

# Get the pod name
POD=$(kubectl get pods -n $NAMESPACE -l app=seaweedfs -o jsonpath='{.items[0].metadata.name}')

# 2. Create Buckets via Filer API (internal)
log "Creating buckets..."
kubectl exec -n $NAMESPACE $POD -c filer -- curl -X POST "http://localhost:8888/models/"
kubectl exec -n $NAMESPACE $POD -c filer -- curl -X POST "http://localhost:8888/adapters/"

# 3. Upload Dummy Weights for llama3-8b
log "Uploading dummy weights for llama3-8b..."
echo "dummy-weights-for-llama3-8b" > /tmp/llama3-8b.bin
kubectl cp /tmp/llama3-8b.bin $NAMESPACE/$POD:/tmp/llama3-8b.bin -c filer
kubectl exec -n $NAMESPACE $POD -c filer -- curl -F "file=@/tmp/llama3-8b.bin" "http://localhost:8888/models/llama3-8b.bin"

# 4. Upload Dummy Adapter
log "Uploading dummy adapter..."
echo "dummy-adapter-data" > /tmp/test-lora.bin
kubectl cp /tmp/test-lora.bin $NAMESPACE/$POD:/tmp/test-lora.bin -c filer
kubectl exec -n $NAMESPACE $POD -c filer -- curl -F "file=@/tmp/test-lora.bin" "http://localhost:8888/adapters/test-lora.bin"

log "SeaweedFS population complete!"
