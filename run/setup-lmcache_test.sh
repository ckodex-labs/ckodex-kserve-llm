#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
renderer="${script_dir}/setup-lmcache.sh"

default_output="$(bash "${renderer}" --mode inProcess --namespace inference)"
[[ "${default_output}" == *"localCPUSizeGiB: 20"* ]]
[[ "${default_output}" == *"runtime behavior is not verified"* ]]

bounded_output="$(bash "${renderer}" --mode inProcess --namespace inference --size-gb 2)"
[[ "${bounded_output}" == *"localCPUSizeGiB: 2"* ]]
[[ "${bounded_output}" != *"localCPUSizeGiB: 20"* ]]

if bash "${renderer}" --mode inProcess --namespace inference --size-gb 0 >/dev/null 2>&1; then
  echo "zero cache size unexpectedly accepted" >&2
  exit 1
fi

missing_digest_output="$(bash "${renderer}" --mode multiprocess --namespace inference --size-gb 2)"
[[ "${missing_digest_output}" == *"explicit --image-digest"* ]]
[[ "${missing_digest_output}" != *"kind: LMCacheEngine"* ]]

printf -v digest_hex '%064d' 0
syntactic_digest="sha256:${digest_hex}"
for image_option in --image-repository --image-tag --image-digest; do
  case "${image_option}" in
    --image-repository) image_value='lmcache/vllm-openai' ;;
    --image-tag) image_value='latest' ;;
    --image-digest) image_value="${syntactic_digest}" ;;
  esac
  if invalid_output="$(bash "${renderer}" --mode inProcess --namespace inference "${image_option}" "${image_value}" 2>&1)"; then
    echo "inProcess mode unexpectedly accepted ${image_option}" >&2
    exit 1
  fi
  [[ "${invalid_output}" == *"image options apply only to multiprocess mode"* ]]
done
multiprocess_output="$(bash "${renderer}" --mode multiprocess --namespace inference --size-gb 2 --image-digest "${syntactic_digest}")"
[[ "${multiprocess_output}" == *"mode: multiprocess"* ]]
[[ "${multiprocess_output}" == *'repository: "lmcache/vllm-openai"'* ]]
[[ "${multiprocess_output}" == *"tag: \"latest@${syntactic_digest}\""* ]]
[[ "${multiprocess_output}" == *"pullPolicy: IfNotPresent"* ]]
[[ "${multiprocess_output}" == *'name: LMCACHE_TRACK_USAGE'* ]]
[[ "${multiprocess_output}" == *'value: "false"'* ]]
[[ "${multiprocess_output}" == *'name: DO_NOT_TRACK'* ]]
[[ "${multiprocess_output}" == *'value: "1"'* ]]

custom_output="$(bash "${renderer}" --mode multiprocess --namespace inference --image-repository 'docker.io/library/ckodex-lmcache-uid' --image-tag validation --image-digest "${syntactic_digest}")"
[[ "${custom_output}" == *'repository: "docker.io/library/ckodex-lmcache-uid"'* ]]
[[ "${custom_output}" == *"tag: \"validation@${syntactic_digest}\""* ]]

port_output="$(bash "${renderer}" --mode multiprocess --namespace inference --image-repository 'registry.local:5000/team/cache_engine' --image-tag release-1 --image-digest "${syntactic_digest}")"
[[ "${port_output}" == *'repository: "registry.local:5000/team/cache_engine"'* ]]
[[ "${port_output}" == *"tag: \"release-1@${syntactic_digest}\""* ]]

for invalid_repository in '' '/cache' 'cache/' 'UPPER/repo' 'cache:latest' 'cache@sha256:abc' 'a//b' $'cache\n  injected: true' 'cache#comment'; do
  if invalid_output="$(bash "${renderer}" --mode multiprocess --namespace inference --image-repository "${invalid_repository}" --image-digest "${syntactic_digest}" 2>&1)"; then
    echo "invalid image repository unexpectedly accepted" >&2
    exit 1
  fi
  [[ "${invalid_output}" == *"--image-repository must be"* ]]
  [[ "${invalid_output}" != *"kind: LMCacheEngine"* ]]
done
for invalid_tag in '' '-invalid' '.invalid' 'release@sha256:abc' $'v1\n  injected: true' "$(printf '%0129d' 0)"; do
  if invalid_output="$(bash "${renderer}" --mode multiprocess --namespace inference --image-tag "${invalid_tag}" --image-digest "${syntactic_digest}" 2>&1)"; then
    echo "invalid image tag unexpectedly accepted" >&2
    exit 1
  fi
  [[ "${invalid_output}" == *"--image-tag must be"* ]]
  [[ "${invalid_output}" != *"kind: LMCacheEngine"* ]]
done

for invalid_digest in sha256:abc "sha256:${digest_hex}A" "sha512:${digest_hex}" \
  sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA; do
  if invalid_output="$(bash "${renderer}" --mode multiprocess --namespace inference --image-digest "${invalid_digest}" --apply --ack-privileged-namespace 2>&1)"; then
    echo "invalid image digest unexpectedly accepted" >&2
    exit 1
  fi
  [[ "${invalid_output}" == *"--image-digest must be"* ]]
done
if missing_output="$(bash "${renderer}" --mode multiprocess --namespace inference --apply --ack-privileged-namespace 2>&1)"; then
  echo "missing image digest unexpectedly accepted" >&2
  exit 1
fi
[[ "${missing_output}" == *"requires an explicit --image-digest"* ]]

valid_output="$(bash "${renderer}" --mode multiprocess --namespace inference --engine cache.pool-1)"
[[ "${valid_output}" == *"name: cache.pool-1"* ]]

printf -v segment '%060d' 0
boundary_engine="${segment}.${segment}.${segment}.${segment:0:59}"
[[ "${#boundary_engine}" -eq 242 ]]
bash "${renderer}" --mode multiprocess --namespace inference --engine "${boundary_engine}" >/dev/null

for invalid_engine in "${boundary_engine}0" "Uppercase" "cache..pool" $'cache\n    injected: true'; do
  if invalid_output="$(bash "${renderer}" --mode multiprocess --namespace inference --engine "${invalid_engine}" 2>&1)"; then
    echo "invalid engine name unexpectedly accepted" >&2
    exit 1
  fi
  [[ "${invalid_output}" == *"--engine must be a lowercase DNS subdomain"* ]]
  [[ "${invalid_output}" != *"experimental:"* ]]
done

# Exercise the actual connection validator against JSON inputs without a cluster.
# shellcheck source=run/setup-lmcache.sh
source "${renderer}"
expected_host="tcp://shared-kv.inference.svc.cluster.local"
valid_connection='{"kv_connector":"LMCacheMPConnector","kv_role":"kv_both","kv_connector_extra_config":{"lmcache.mp.host":"tcp://shared-kv.inference.svc.cluster.local","lmcache.mp.port":"5555"}}'
valid_configmap="$(jq -cn --arg connection "${valid_connection}" '{data:{"kv-transfer-config.json":$connection}}')"
validator_output="$(validate_connection_config "${expected_host}" <<< "${valid_configmap}" 2>&1)"
[[ -z "${validator_output}" ]]

for invalid_connection in \
  '{' \
  '{"kv_connector":"Other","kv_role":"kv_both","kv_connector_extra_config":{"lmcache.mp.host":"tcp://shared-kv.inference.svc.cluster.local","lmcache.mp.port":"5555"}}' \
  '{"kv_connector":"LMCacheMPConnector","kv_role":"kv_producer","kv_connector_extra_config":{"lmcache.mp.host":"tcp://shared-kv.inference.svc.cluster.local","lmcache.mp.port":"5555"}}' \
  '{"kv_connector":"LMCacheMPConnector","kv_role":"kv_both","kv_connector_extra_config":{"lmcache.mp.host":"tcp://other.inference.svc.cluster.local","lmcache.mp.port":"5555"}}' \
  '{"kv_connector":"LMCacheMPConnector","kv_role":"kv_both","kv_connector_extra_config":{"lmcache.mp.host":"tcp://shared-kv.inference.svc.cluster.local","lmcache.mp.port":"5556"}}'; do
  invalid_configmap="$(jq -cn --arg connection "${invalid_connection}" '{data:{"kv-transfer-config.json":$connection}}')"
  if validator_output="$(validate_connection_config "${expected_host}" <<< "${invalid_configmap}" 2>&1)"; then
    echo "invalid connection configuration unexpectedly accepted" >&2
    exit 1
  fi
  [[ -z "${validator_output}" ]]
done
if validate_connection_config "${expected_host}" <<< '{"data":{}}'; then
  echo "missing connection key unexpectedly accepted" >&2
  exit 1
fi
echo "LMCache renderer checks passed"
