#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
HELM_OUT_DIR="${DIST_DIR}/helm"
SUMMARY_FILE="${ROOT_DIR}/bin/release-readiness.json"

require_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required tool: $1" >&2
    exit 1
  fi
}

require_tool git
require_tool goreleaser
require_tool helm
require_tool jq

checksum_cmd=""
if command -v sha256sum >/dev/null 2>&1; then
  checksum_cmd="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  checksum_cmd="shasum -a 256"
else
  echo "missing required checksum tool: sha256sum or shasum" >&2
  exit 1
fi

mkdir -p "${ROOT_DIR}/bin"

before_diff="$(mktemp)"
after_diff="$(mktemp)"
trap 'rm -f "${before_diff}" "${after_diff}"' EXIT

(cd "${ROOT_DIR}" && git diff --name-only > "${before_diff}")

echo "==> validating release workflow contract"
grep -q "Run Dagger Release Pipeline" "${ROOT_DIR}/.github/workflows/release.yml"
grep -q "generator_container_slsa3.yml" "${ROOT_DIR}/.github/workflows/release.yml"
grep -q "generator_generic_slsa3.yml" "${ROOT_DIR}/.github/workflows/release.yml"
grep -q "helm push" "${ROOT_DIR}/.github/workflows/release.yml"

echo "==> running local goreleaser snapshot rehearsal"
(cd "${ROOT_DIR}" && goreleaser release --snapshot --clean --skip=publish,sign)

echo "==> packaging helm chart"
rm -rf "${HELM_OUT_DIR}"
mkdir -p "${HELM_OUT_DIR}"
(cd "${ROOT_DIR}" && helm lint deploy/helm)
(cd "${ROOT_DIR}" && helm package deploy/helm --destination "${HELM_OUT_DIR}")

checksum_file="$(find "${DIST_DIR}" -maxdepth 1 -name checksums.txt | head -n 1)"
if [[ -z "${checksum_file}" ]]; then
  echo "release rehearsal did not produce dist/checksums.txt" >&2
  exit 1
fi

archive_count="$(find "${DIST_DIR}" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' \) | wc -l | tr -d ' ')"
if [[ "${archive_count}" -eq 0 ]]; then
  echo "release rehearsal did not produce any binary archives" >&2
  exit 1
fi

echo "==> verifying checksum manifest against generated archives"
if [[ "${checksum_cmd}" == "sha256sum" ]]; then
  (cd "${DIST_DIR}" && sha256sum -c checksums.txt)
else
  (cd "${DIST_DIR}" && shasum -a 256 -c checksums.txt)
fi

helm_package="$(find "${HELM_OUT_DIR}" -maxdepth 1 -name 'ckodex-kserve-llm-operator-*.tgz' | head -n 1)"
if [[ -z "${helm_package}" ]]; then
  echo "helm packaging did not produce a chart archive" >&2
  exit 1
fi

(cd "${ROOT_DIR}" && git diff --name-only > "${after_diff}")
mutated_files="$(comm -13 <(sort "${before_diff}") <(sort "${after_diff}"))"
if [[ -n "${mutated_files}" ]]; then
  echo "release rehearsal mutated tracked files:" >&2
  echo "${mutated_files}" >&2
  exit 1
fi

jq -n \
  --arg checksum_file "${checksum_file#${ROOT_DIR}/}" \
  --arg helm_package "${helm_package#${ROOT_DIR}/}" \
  --argjson archive_count "${archive_count}" \
  '{
    status: "ok",
    mode: "local-snapshot",
    checksum_file: $checksum_file,
    helm_package: $helm_package,
    archive_count: $archive_count
  }' > "${SUMMARY_FILE}"

echo "release readiness snapshot complete"
echo "summary: ${SUMMARY_FILE}"
