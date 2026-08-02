#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output="${1:-${root_dir}/dist/ckodex-crds.yaml}"
mkdir -p "$(dirname "${output}")"
scratch="$(mktemp)"
trap 'rm -f "${scratch}"' EXIT

for manifest in "${root_dir}"/config/crd/*.yaml; do
  printf '%s\n' '---' >> "${scratch}"
  sed '/^---$/d' "${manifest}" >> "${scratch}"
done
mv "${scratch}" "${output}"
trap - EXIT

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "${output}" | sed "s#${output}#$(basename "${output}")#" > "${output}.sha256"
else
  shasum -a 256 "${output}" | sed "s#${output}#$(basename "${output}")#" > "${output}.sha256"
fi
