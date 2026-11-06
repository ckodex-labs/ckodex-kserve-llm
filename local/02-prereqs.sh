#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_HF_CSI="${INSTALL_HF_CSI:-0}"

# ── 1. Cert-manager ──────────────────────────────────────────────
helm repo add jetstack https://charts.jetstack.io
helm repo update jetstack
helm upgrade --install cert-manager jetstack/cert-manager \
  --namespace cert-manager --create-namespace \
  --version v1.21.1 --set crds.enabled=true
kubectl wait --for=condition=Available deployment/cert-manager \
  -n cert-manager --timeout=120s

# ── 2. Gateway API CRDs ──────────────────────────────────────────
# The v1.6.2 bundle aligns with Gateway API Go library v1.6.2, GIE v1.6.2,
# and Envoy Gateway v1.9.2 / Agent Router v1.2.0. Force field ownership during an
# intentional version upgrade so a prior Helm-managed bundle does not make
# the profile non-idempotent.
kubectl apply --server-side --force-conflicts \
  -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.6.2/standard-install.yaml

# ── 2b. Gateway API Inference Extension and llm-d Router CRDs ─────
# GIE v1.6.2 supplies the GA InferencePool API. The Router v1.0.0 bundle owns
# InferenceObjective and InferenceModelRewrite under llm-d.ai. Applying these
# bundles does not delete legacy CRDs or migrate existing objects on upgrades.
kubectl apply --server-side -f https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/download/v1.6.2/manifests.yaml
kubectl apply --server-side -f https://github.com/llm-d/llm-d-router/releases/download/v1.0.0/manifests.yaml

# Envoy Gateway keeps its provider-specific CRDs in a separate release asset.
# Apply them explicitly so upgrades remain correct even when Helm has already
# installed the release and therefore does not replay its CRD directory.
kubectl apply --server-side \
  -f https://github.com/envoyproxy/gateway/releases/download/v1.9.2/envoy-gateway-crds.yaml

# ── 2c. Agent Router controller (InferencePool extension manager) ──
# Envoy Gateway does not natively resolve InferencePool backendRefs. The
# pinned Agent Router (formerly Envoy AI Gateway) charts provide the extension-manager
# service and CRDs used by the InferencePool-enabled Envoy Gateway profile below.
# Baseline release is v1.2.0 (pre-release candidate v1.2.1-rc2 available).
HELM_REGISTRY_CONFIG=/dev/null helm upgrade --install ai-gateway-crds \
  oci://docker.io/envoyproxy/ai-gateway-crds-helm \
  --version v1.2.0 \
  --namespace envoy-ai-gateway-system \
  --create-namespace
HELM_REGISTRY_CONFIG=/dev/null helm upgrade --install ai-gateway \
  oci://docker.io/envoyproxy/ai-gateway-helm \
  --version v1.2.0 \
  --namespace envoy-ai-gateway-system \
  --create-namespace
kubectl wait --for=condition=Available deployment/ai-gateway-controller \
  -n envoy-ai-gateway-system --timeout=180s

# ── 3. Envoy Gateway controller (provides "envoy" GatewayClass) ──
# Gateway API and Envoy Gateway CRDs are installed in step 2. Skip the chart's
# CRD files and its separately templated Gateway API safe-upgrade policy so
# Helm does not contend with the kubectl-managed upstream bundle.
HELM_REGISTRY_CONFIG=/dev/null helm upgrade --install eg oci://docker.io/envoyproxy/gateway-helm \
  --version v1.9.2 \
  --namespace envoy-gateway-system \
  --create-namespace \
  --skip-crds \
  --set crds.gatewayAPI.safeUpgradePolicy.enabled=false \
  -f "${SCRIPT_DIR}/03-envoy-gateway-values.yaml"
kubectl wait --for=condition=Available deployment/envoy-gateway \
  -n envoy-gateway-system --timeout=120s
kubectl apply -f "${SCRIPT_DIR}/03-envoy-gatewayclass.yaml"
kubectl apply -f "${SCRIPT_DIR}/03-envoy-gateway-rbac.yaml"
kubectl wait --for=condition=Accepted gatewayclass/envoy --timeout=120s
echo "Envoy Gateway controller installed"

# ── 4. MetalLB (LoadBalancer IP allocation for KIND) ──────────────
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.14.9/config/manifests/metallb-native.yaml
kubectl wait --for=condition=Ready pod -l app=metallb -n metallb-system --timeout=120s

# Configure MetalLB address pool using the KIND Docker network subnet
SUBNET=$(docker network inspect kind | jq -r '.[0].IPAM.Config[] | select(.Subnet | test("^[0-9]+\\.[0-9]+\\.[0-9]+\\.[0-9]+/[0-9]+$")) | .Subnet' | head -n1)
if [ -z "$SUBNET" ]; then
  echo "Could not determine an IPv4 subnet for the kind network" >&2
  exit 1
fi
BASE=$(echo "$SUBNET" | cut -d'/' -f1 | cut -d'.' -f1-3)
cat <<EOF | kubectl apply -f -
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: kind-pool
  namespace: metallb-system
spec:
  addresses:
  - ${BASE}.200-${BASE}.250
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: kind-l2
  namespace: metallb-system
EOF
echo "MetalLB configured with address pool ${BASE}.200-${BASE}.250"

# ── 5. Optional HuggingFace CSI Driver (hf-mount: lazy mounting) ───
# The default CPU proof uses hf:// and the signed storage-initializer image.
# Install the privileged FUSE/CSI path only when explicitly testing
# hf-mount:// workloads: INSTALL_HF_CSI=1 ./run/e2e.sh
if [[ "$INSTALL_HF_CSI" == "1" ]]; then
  HELM_REGISTRY_CONFIG=/dev/null helm upgrade --install hf-csi oci://ghcr.io/huggingface/charts/hf-csi-driver \
    --namespace kube-system \
    --version 0.14.0 \
    --set logVerbosity=2
  kubectl wait --for=condition=Ready pod -l app=hf-csi-node \
    -n kube-system --timeout=120s
  echo "HuggingFace CSI driver (hf-mount) installed"
else
  echo "Skipping optional HuggingFace CSI driver; default proof uses hf://"
fi
