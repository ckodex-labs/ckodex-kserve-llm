#!/usr/bin/env bash
set -euo pipefail
# OCI used now

# CRDs
kubectl apply --server-side -f https://github.com/kserve/kserve/releases/download/v0.20.0/kserve-crds.yaml
# Skipping helm upgrade for kserve-crd to avoid ownership conflicts with kubectl apply
# helm upgrade --install kserve-crd oci://ghcr.io/kserve/charts/kserve-crd --version v0.20.0 \
#   --namespace kserve --create-namespace
# Resources + Standard mode + Gateway API (LLMInferenceService ready)
HELM_REGISTRY_CONFIG=/dev/null helm upgrade --install kserve oci://ghcr.io/kserve/charts/kserve-resources --version v0.20.0 \
  --namespace kserve --create-namespace \
  --set kserve.controller.deploymentMode=Standard \
  --set kserve.controller.gateway.ingressGateway.enableGatewayApi=true \
  --set kserve.controller.gateway.ingressGateway.kserveGateway=kserve/kserve-ingress-gateway \
  --set kserve.controller.gateway.ingressGateway.createGateway=true
kubectl wait --for=condition=Available deployment/kserve-controller-manager -n kserve --timeout=300s
echo "KServe v0.20.0 installed in Standard mode"
