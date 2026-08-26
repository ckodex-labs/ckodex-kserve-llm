#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output="${1:-${root_dir}/dist/ckodex-crds.yaml}"
mkdir -p "$(dirname "${output}")"
scratch="$(mktemp)"
trap 'rm -f "${scratch}"' EXIT

if ! command -v kubectl >/dev/null 2>&1; then
  echo "missing required tool: kubectl (needed to render the beta CRD conversion profile)" >&2
  exit 1
fi

# The release CRD artifact is the beta profile, not the raw webhook-disabled
# development files. Kustomize applies the conversion patch and preserves a
# single source of truth for the generated CRD schemas.
kubectl kustomize "${root_dir}/config/crd" > "${scratch}"
for required in \
  'strategy: Webhook' \
  'name: ckodex-kserve-llm-operator-webhook-service' \
  'namespace: ckodex-system' \
  'path: /convert' \
  'cert-manager.io/inject-ca-from: ckodex-system/ckodex-kserve-llm-operator-webhook-cert'; do
  if ! grep -qF "${required}" "${scratch}"; then
    echo "beta CRD bundle is missing conversion contract: ${required}" >&2
    exit 1
  fi
done
mv "${scratch}" "${output}"
trap - EXIT

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "${output}" | sed "s#${output}#$(basename "${output}")#" > "${output}.sha256"
else
  shasum -a 256 "${output}" | sed "s#${output}#$(basename "${output}")#" > "${output}.sha256"
fi
