#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kserve-017}"

log() {
  printf '\n==> %s\n' "$*"
}

log "Cleaning up KIND cluster state"
if kind get clusters 2>/dev/null | grep -Fxq "$KIND_CLUSTER_NAME"; then
  bash local/06-cleanup-kind-space.sh || true
  kind delete cluster --name "$KIND_CLUSTER_NAME" || true
else
  echo "No KIND cluster named ${KIND_CLUSTER_NAME} found; skipping node cleanup."
fi

log "Pruning leftover Docker build and volume state"
docker builder prune -af || true
docker volume prune -f || true
docker image prune -af || true

log "Removing local cache scratch space"
rm -rf .cache/tmp

log "Cleanup complete"
