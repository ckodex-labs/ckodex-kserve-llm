#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kserve-017}"
KIND_NODE_IMAGE="${KIND_NODE_IMAGE:-kindest/node:v1.35.0}"

node_nofile="$(docker run --rm --entrypoint /bin/sh "$KIND_NODE_IMAGE" -c 'ulimit -Sn' 2>/dev/null || true)"
if ! [[ "$node_nofile" =~ ^[0-9]+$ ]] || (( node_nofile < 65536 )); then
  cat >&2 <<EOF
KIND cannot start reliably with Docker's node nofile limit: ${node_nofile:-unknown}
Required minimum: 65536
Increase Docker Desktop/container default nofile, then rerun this command.
EOF
  exit 1
fi

kind create cluster --name "$KIND_CLUSTER_NAME" \
  --image "$KIND_NODE_IMAGE" \
  --config "${ROOT_DIR}/deploy/kind/kind-config.yaml"
kubectl config use-context "kind-${KIND_CLUSTER_NAME}"
echo "KIND cluster ready with port-forward to 8080/8443"
