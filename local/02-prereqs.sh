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
# The v1.5.1 bundle also owns the safety ValidatingAdmissionPolicies. Force field
# ownership during an intentional version upgrade so a prior Helm-managed
# bundle does not make the profile non-idempotent.
kubectl apply --server-side --force-conflicts \
  -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.1/standard-install.yaml

# ── 2b. Gateway API Inference Extension CRDs ─────────────────────
# The operator's optional router.scheduler path reconciles the GA InferencePool
# API and the digest-pinned llm-d Router EPP. Install the CRDs explicitly; the
# EPP image alone does not register the APIs with the Kubernetes apiserver.
kubectl apply --server-side -f https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/download/v1.5.0/manifests.yaml

# llm-d Router owns the EPP and its request-policy CRDs after the EPP moved out
# of GIE. Keep the GIE v1.5.0 bundle above for InferencePool and the deprecated
# request-policy API during the compatibility window.
kubectl apply --server-side -f https://github.com/llm-d/llm-d-router/releases/download/v0.10.0/manifests.yaml

# Envoy Gateway keeps its provider-specific CRDs in a separate release asset.
# Apply them explicitly so upgrades remain correct even when Helm has already
# installed the release and therefore does not replay its CRD directory.
kubectl apply --server-side \
  -f https://github.com/envoyproxy/gateway/releases/download/v1.8.1/envoy-gateway-crds.yaml

# ── 2c. Envoy AI Gateway controller (InferencePool extension manager) ──
# Envoy Gateway does not natively resolve InferencePool backendRefs. The
# pinned AI Gateway charts provide the extension-manager service and CRDs used
# by the InferencePool-enabled Envoy Gateway profile below.
HELM_REGISTRY_CONFIG=/dev/null helm upgrade --install ai-gateway-crds \
  oci://docker.io/envoyproxy/ai-gateway-crds-helm \
  --version v1.1.0 \
  --namespace envoy-ai-gateway-system \
  --create-namespace
HELM_REGISTRY_CONFIG=/dev/null helm upgrade --install ai-gateway \
  oci://docker.io/envoyproxy/ai-gateway-helm \
  --version v1.1.0 \
  --namespace envoy-ai-gateway-system \
  --create-namespace
kubectl wait --for=condition=Available deployment/ai-gateway-controller \
  -n envoy-ai-gateway-system --timeout=180s

# ── 3. Envoy Gateway controller (provides "envoy" GatewayClass) ──
# Keep chart CRDs enabled so Envoy-specific APIs (Backend, HTTPRouteFilter,
# EnvoyProxy, and policy types) are installed. Existing standard Gateway API
# CRDs from step 2 are retained by Helm's CRD install semantics.
HELM_REGISTRY_CONFIG=/dev/null helm upgrade --install eg oci://docker.io/envoyproxy/gateway-helm \
  --version v1.8.1 \
  --namespace envoy-gateway-system \
  --create-namespace \
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
