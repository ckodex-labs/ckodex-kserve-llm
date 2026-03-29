#!/usr/bin/env bash
set -euo pipefail
kind create cluster --name kserve-017 --config - <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 80
    hostPort: 8080
    protocol: TCP
  - containerPort: 443
    hostPort: 8443
    protocol: TCP
EOF
kubectl config use-context kind-kserve-017
echo "KIND cluster ready with port-forward to 8080/8443"
