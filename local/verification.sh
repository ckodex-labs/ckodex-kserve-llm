#!/usr/bin/env bash
set -e
bash 01-kind-setup.sh
bash 02-prereqs.sh
bash 03-kserve-helm-install.sh
kubectl apply -f 04-llm-inference-service.yaml
bash 05-test-inference.sh
echo "All verification passed"
