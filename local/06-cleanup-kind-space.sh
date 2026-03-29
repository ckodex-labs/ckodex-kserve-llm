#!/bin/bash
# 06-cleanup-kind-space.sh
# Purges large vLLM images and unused objects to free up KIND node space

set -euo pipefail

log() { echo "[$(date +'%H:%M:%S')] $*"; }

KIND_NODES=$(kubectl get nodes -o jsonpath='{.items[*].metadata.name}')

for NODE in $KIND_NODES; do
    log "Cleaning up node: $NODE..."
    
    # 1. Prune unused images (forcefully for large ones if needed)
    # docker exec $NODE crictl rmi --prune
    
    # 2. Specifically target large vLLM images if they are not in use
    # Note: This is a destructive operation if the pod is being restarted
    log "Searching for large unused images on $NODE..."
    docker exec "$NODE" crictl images | grep -E "vllm|pytorch" | awk '{print $3}' | xargs -r docker exec "$NODE" crictl rmi || true
    
    # 3. Final system prune
    docker exec "$NODE" crictl ps -a | grep Exited | awk '{print $1}' | xargs -r docker exec "$NODE" crictl rm || true
done

log "Cleanup complete! Disk space reclaimed on KIND nodes."
