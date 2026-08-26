#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kserve-017}"
OPERATOR_IMAGE="${OPERATOR_IMAGE:-ckodex/kserve-llm-operator:dev}"
STORAGE_INITIALIZER_IMAGE="${STORAGE_INITIALIZER_IMAGE:-ckodex/storage-initializer:v0.1.0}"

if [[ "$OPERATOR_IMAGE" != *:* || "$OPERATOR_IMAGE" == *@* ]]; then
  echo "OPERATOR_IMAGE must include a tag for the local Helm profile: ${OPERATOR_IMAGE}" >&2
  exit 1
fi

OPERATOR_IMAGE_REPOSITORY="${OPERATOR_IMAGE%:*}"
OPERATOR_IMAGE_TAG="${OPERATOR_IMAGE##*:}"

log() {
  printf '\n==> %s\n' "$*"
}

cluster_exists() {
  kind get clusters 2>/dev/null | grep -Fxq "$KIND_CLUSTER_NAME"
}

log "Preparing local KIND environment"
if cluster_exists; then
  echo "KIND cluster ${KIND_CLUSTER_NAME} already exists; reusing it."
  kubectl config use-context "kind-${KIND_CLUSTER_NAME}" >/dev/null
else
  bash local/01-kind-setup.sh
fi

log "Installing platform prerequisites"
bash local/02-prereqs.sh
bash local/03-kserve-helm-install.sh

log "Building and loading the operator image"
make docker-build IMG="$OPERATOR_IMAGE"
kind load docker-image "$OPERATOR_IMAGE" --name "$KIND_CLUSTER_NAME"

log "Building and loading the storage-initializer image"
make storage-initializer-load STORAGE_INITIALIZER_IMG="$STORAGE_INITIALIZER_IMAGE" KIND_CLUSTER_NAME="$KIND_CLUSTER_NAME"

log "Installing CRDs and deploying the operator"
kubectl apply --server-side -k config/crd/
helm upgrade --install ckodex-kserve-llm-operator deploy/helm \
  --namespace ckodex-system --create-namespace \
  --set fullnameOverride=ckodex-kserve-llm-operator \
  --set-string image.repository="$OPERATOR_IMAGE_REPOSITORY" \
  --set-string image.tag="$OPERATOR_IMAGE_TAG" \
  --set image.pullPolicy=IfNotPresent \
  --set replicaCount=1 \
  --set leaderElection.enabled=false \
  --set webhook.enabled=true \
  --set certManager.enabled=true
kubectl wait --for=condition=Ready \
  certificate/ckodex-kserve-llm-operator-webhook-cert \
  -n ckodex-system --timeout=300s
kubectl rollout status deployment/ckodex-kserve-llm-operator-controller-manager \
  -n ckodex-system --timeout=300s

log "Applying the sample inference service and probing the endpoint"
bash local/05-test-inference.sh

log "Local KIND E2E verification completed"
