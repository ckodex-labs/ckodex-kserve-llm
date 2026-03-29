#!/usr/bin/env bash
set -euo pipefail

# ── 1. Cert-manager ──────────────────────────────────────────────
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager --create-namespace \
  --version v1.16.1 --set crds.enabled=true
kubectl wait --for=condition=Available deployment/cert-manager \
  -n cert-manager --timeout=120s

# ── 2. Gateway API CRDs ──────────────────────────────────────────
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.1/standard-install.yaml

# ── 3. Envoy Gateway controller (provides "envoy" GatewayClass) ──
# --skip-crds because Gateway API CRDs are already installed in step 2.
helm install eg oci://docker.io/envoyproxy/gateway-helm \
  --version v1.3.0 \
  --namespace envoy-gateway-system \
  --create-namespace \
  --skip-crds
kubectl wait --for=condition=Available deployment/envoy-gateway \
  -n envoy-gateway-system --timeout=120s
echo "Envoy Gateway controller installed"

# ── 4. MetalLB (LoadBalancer IP allocation for KIND) ──────────────
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.14.9/config/manifests/metallb-native.yaml
kubectl wait --for=condition=Ready pod -l app=metallb -n metallb-system --timeout=120s

# Configure MetalLB address pool using the KIND Docker network subnet
SUBNET=$(docker network inspect kind -f '{{(index .IPAM.Config 0).Subnet}}')
BASE=$(echo "$SUBNET" | cut -d'.' -f1-2)
cat <<EOF | kubectl apply -f -
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: kind-pool
  namespace: metallb-system
spec:
  addresses:
  - ${BASE}.255.200-${BASE}.255.250
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: kind-l2
  namespace: metallb-system
EOF
echo "MetalLB configured with address pool ${BASE}.255.200-${BASE}.255.250"

# ── 5. HuggingFace CSI Driver (hf-mount: lazy model mounting) ──────
# Enables hf-mount:// URI scheme — mounts HF repos via NFS/FUSE with
# lazy byte-level loading. No full download needed.
helm install hf-csi oci://ghcr.io/huggingface/charts/hf-csi-driver \
  --namespace kube-system \
  --set logVerbosity=2
kubectl wait --for=condition=Ready pod -l app=hf-csi-node \
  -n kube-system --timeout=120s
echo "HuggingFace CSI driver (hf-mount) installed"
