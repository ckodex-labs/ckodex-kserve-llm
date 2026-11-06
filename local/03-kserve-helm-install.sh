#!/usr/bin/env bash
set -euo pipefail
# KServe v0.21 separates CRD charts from controller resources. Keep CRDs
# server-side-applied so Helm does not contend with the existing kubectl-owned
# CRD bundle. This profile needs KServe core APIs; CKODEX's own LLMInferenceService
# CRD remains under serving.ckodex.com and is installed separately.
HELM_REGISTRY_CONFIG=/dev/null helm template kserve-crd \
  oci://ghcr.io/kserve/charts/kserve-crd@sha256:36385f02ef5723910c83e2e04aea6892a3a54e7bdcdd88636ef7adbf7dff61f0 \
  --namespace kserve \
  | kubectl apply --server-side --force-conflicts -f -

# Resources + Standard mode + Gateway API (LLMInferenceService ready)
HELM_REGISTRY_CONFIG=/dev/null helm upgrade --install kserve \
  oci://ghcr.io/kserve/charts/kserve-resources@sha256:2d189c8c9d51e1ad658f9a973eaf9ea73b251305e7f93507adb1b559303f1e9a \
  --namespace kserve --create-namespace \
  --set kserve.controller.deploymentMode=Standard \
  --set kserve.controller.gateway.ingressGateway.enableGatewayApi=true \
  --set kserve.controller.gateway.ingressGateway.kserveGateway=kserve/kserve-ingress-gateway \
  --set kserve.controller.gateway.ingressGateway.createGateway=true
kubectl wait --for=condition=Available deployment/kserve-controller-manager -n kserve --timeout=300s
echo "KServe v0.21.0 installed in Standard mode"
