#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kserve-017}"

kind create cluster --name "$KIND_CLUSTER_NAME" --config "${ROOT_DIR}/deploy/kind/kind-config.yaml"
kubectl config use-context "kind-${KIND_CLUSTER_NAME}"
echo "KIND cluster ready with port-forward to 8080/8443"
