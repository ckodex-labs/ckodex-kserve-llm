#!/usr/bin/env bash
set -euo pipefail

operator_version="operator-v0.1.1"
operator_url="https://github.com/LMCache/LMCache/releases/download/${operator_version}/install.yaml"
operator_sha256="8e5eab17ccead2915fc54465e485a9f2c6c947b9fb1301c2149ba1a94dc7e609"

mode=""
namespace=""
engine="ckodex-lmcache"
size_gb="20"
image_repository="lmcache/vllm-openai"
image_tag="latest"
image_digest=""
image_options_supplied=false
apply=false
ack_privileged=false

usage() {
  echo "usage: $0 --mode inProcess|multiprocess --namespace NAMESPACE [--engine NAME] [--size-gb N] [--image-repository REPOSITORY] [--image-tag TAG] [--image-digest sha256:DIGEST] [--apply --ack-privileged-namespace]"
}

# Reads a ConfigMap JSON object on stdin without exposing its contents.
validate_connection_config() {
  local expected_host="$1"
  jq -e --arg expected_host "${expected_host}" '
    .data["kv-transfer-config.json"] |
    if type == "string" then fromjson else null end |
    .kv_connector == "LMCacheMPConnector" and
    .kv_role == "kv_both" and
    .kv_connector_extra_config["lmcache.mp.host"] == $expected_host and
    .kv_connector_extra_config["lmcache.mp.port"] == "5555"
  ' >/dev/null 2>&1
}

if [[ "${BASH_SOURCE[0]}" != "$0" ]]; then
  return 0
fi

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) mode="${2:-}"; shift 2 ;;
    --namespace) namespace="${2:-}"; shift 2 ;;
    --engine) engine="${2:-}"; shift 2 ;;
    --size-gb) size_gb="${2:-}"; shift 2 ;;
    --image-repository) image_repository="${2:-}"; image_options_supplied=true; shift 2 ;;
    --image-tag) image_tag="${2:-}"; image_options_supplied=true; shift 2 ;;
    --image-digest) image_digest="${2:-}"; image_options_supplied=true; shift 2 ;;
    --apply) apply=true; shift ;;
    --ack-privileged-namespace) ack_privileged=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -z "${mode}" || -z "${namespace}" ]]; then
  echo "--mode and an explicit --namespace are required" >&2
  usage >&2
  exit 2
fi
if [[ ! "${namespace}" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]]; then
  echo "invalid Kubernetes namespace: ${namespace}" >&2
  exit 2
fi
if [[ ! "${size_gb}" =~ ^[1-9][0-9]*$ ]]; then
  echo "--size-gb must be a positive integer" >&2
  exit 2
fi
if (( ${#engine} > 242 )) || [[ ! "${engine}" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$ ]]; then
  echo "--engine must be a lowercase DNS subdomain of at most 242 characters (reserving space for -connection)" >&2
  exit 2
fi
if [[ -n "${image_digest}" && ! "${image_digest}" =~ ^sha256:[a-f0-9]{64}$ ]]; then
  echo "--image-digest must be sha256 followed by 64 lowercase hexadecimal digits" >&2
  exit 2
fi
# Use the common OCI repository-name subset, including an optional registry
# port. Tags and digests are separate fields and must not be embedded here.
if (( ${#image_repository} > 255 )) || [[ ! "${image_repository}" =~ ^([a-z0-9]+([.-][a-z0-9]+)*(:[0-9]+)?/)?[a-z0-9]+(([._]|__|-+)[a-z0-9]+)*(/[a-z0-9]+(([._]|__|-+)[a-z0-9]+)*)*$ ]]; then
  echo "--image-repository must be a lowercase image repository without a tag or digest" >&2
  exit 2
fi
if [[ ! "${image_tag}" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]]; then
  echo "--image-tag must be a valid image tag of at most 128 characters" >&2
  exit 2
fi

render_inprocess() {
  cat <<YAML
experimental:
  kvCache:
    transfer:
      connector: lmcache
      lmcache:
        mode: inProcess
        chunkSize: 256
        localCPU: true
        localCPUSizeGiB: ${size_gb}
YAML
}

render_multiprocess() {
  cat <<YAML
experimental:
  kvCache:
    transfer:
      connector: lmcache
      lmcache:
        mode: multiprocess
        engineRef:
          name: ${engine}
YAML
}

render_engine_resource() {
  cat <<YAML
apiVersion: lmcache.lmcache.ai/v1alpha1
kind: LMCacheEngine
metadata:
  name: ${engine}
  namespace: ${namespace}
spec:
  image:
    repository: "${image_repository}"
    tag: "${image_tag}@${image_digest}"
    pullPolicy: IfNotPresent
  l1:
    sizeGB: ${size_gb}
  env:
    - name: LMCACHE_TRACK_USAGE
      value: "false"
    - name: DO_NOT_TRACK
      value: "1"
YAML
}

if [[ "${mode}" == "inProcess" ]]; then
  if [[ "${image_options_supplied}" == true ]]; then
    echo "image options apply only to multiprocess mode" >&2
    exit 2
  fi
  echo "Rendered in-process LMCache configuration for namespace ${namespace}; runtime behavior is not verified."
  render_inprocess
  exit 0
fi
if [[ "${mode}" != "multiprocess" ]]; then
  echo "unsupported mode: ${mode}" >&2
  exit 2
fi

if [[ "${apply}" != true ]]; then
  echo "Dry run: multiprocess mode would install ${operator_version} and create LMCacheEngine ${namespace}/${engine}."
  echo "Applying requires --apply --ack-privileged-namespace because hostIPC needs privileged Pod Security admission."
  render_multiprocess
  if [[ -z "${image_digest}" ]]; then
    echo "An explicit --image-digest sha256:DIGEST is required for --apply; engine resource not rendered."
  else
    echo "LMCacheEngine resource that would be applied:"
    render_engine_resource
  fi
  exit 0
fi
if [[ -z "${image_digest}" ]]; then
  echo "multiprocess --apply requires an explicit --image-digest sha256:DIGEST" >&2
  exit 2
fi
if [[ "${ack_privileged}" != true ]]; then
  echo "refusing to apply multiprocess resources without --ack-privileged-namespace" >&2
  exit 2
fi
command -v kubectl >/dev/null 2>&1 || { echo "kubectl is required" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "curl is required" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }
kubectl version --request-timeout=10s >/dev/null

# The pinned multiprocess operator schedules its engine only to labeled GPU
# nodes and requests the NVIDIA runtime handler. Check declarations before
# installing it; this does not prove the handler or GPU workload functions.
if ! runtime_handler="$(kubectl get runtimeclass nvidia --request-timeout=10s -o jsonpath='{.handler}')"; then
  echo "multiprocess preflight failed: RuntimeClass nvidia is unavailable" >&2
  exit 1
fi
if [[ "${runtime_handler}" != "nvidia" ]]; then
  echo "multiprocess preflight failed: RuntimeClass nvidia must declare handler nvidia" >&2
  exit 1
fi
if ! gpu_counts="$(kubectl get nodes --request-timeout=10s -l nvidia.com/gpu.present=true -o 'jsonpath={range .items[*]}{.status.allocatable.nvidia\.com/gpu}{"\n"}{end}')"; then
  echo "multiprocess preflight failed: unable to inspect labeled GPU nodes" >&2
  exit 1
fi
has_gpu=false
while IFS= read -r count; do
  if [[ "${count}" =~ ^[1-9][0-9]*$ ]]; then
    has_gpu=true
    break
  fi
done <<< "${gpu_counts}"
if [[ "${has_gpu}" != true ]]; then
  echo "multiprocess preflight failed: no node labeled nvidia.com/gpu.present=true has an allocatable NVIDIA GPU" >&2
  exit 1
fi

scratch_dir="$(mktemp -d)"
trap 'rm -rf "${scratch_dir}"' EXIT
curl -fsSL "${operator_url}" -o "${scratch_dir}/install.yaml"
if command -v sha256sum >/dev/null 2>&1; then
  actual_sha="$(sha256sum "${scratch_dir}/install.yaml" | awk '{print $1}')"
else
  actual_sha="$(shasum -a 256 "${scratch_dir}/install.yaml" | awk '{print $1}')"
fi
if [[ "${actual_sha}" != "${operator_sha256}" ]]; then
  echo "LMCache operator installer checksum mismatch" >&2
  exit 1
fi

kubectl apply -f "${scratch_dir}/install.yaml"
kubectl wait --for=condition=Established crd/lmcacheengines.lmcache.lmcache.ai --timeout=120s
kubectl create namespace "${namespace}" --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace "${namespace}" pod-security.kubernetes.io/enforce=privileged --overwrite
render_engine_resource | kubectl apply -f -
# The pinned operator reports these three conditions, not Ready. Check them
# together from one observation, at the current generation, within one budget.
conditions_jsonpath='{.metadata.generation}{"|"}{.status.observedGeneration}{"|"}{range .status.conditions[?(@.type=="ConfigValid")]}{.status}{":"}{.observedGeneration}{end}{"|"}{range .status.conditions[?(@.type=="Available")]}{.status}{":"}{.observedGeneration}{end}{"|"}{range .status.conditions[?(@.type=="AllInstancesReady")]}{.status}{":"}{.observedGeneration}{end}'
deadline=$((SECONDS + 600))
ready=false
while (( SECONDS < deadline )); do
  remaining=$((deadline - SECONDS))
  request_timeout=$((remaining < 10 ? remaining : 10))
  if ! conditions="$(kubectl get "lmcacheengine/${engine}" -n "${namespace}" --request-timeout="${request_timeout}s" -o "jsonpath=${conditions_jsonpath}")"; then
    echo "unable to read LMCacheEngine conditions" >&2
    exit 1
  fi
  if (( SECONDS >= deadline )); then
    break
  fi
  IFS='|' read -r generation observed config_valid available instances_ready extra <<< "${conditions}"
  if [[ "${generation}" =~ ^[1-9][0-9]*$ && "${observed}" == "${generation}" &&
        "${config_valid}" == "True:${generation}" && "${available}" == "True:${generation}" &&
        "${instances_ready}" == "True:${generation}" && -z "${extra}" ]]; then
    ready=true
    break
  fi
  remaining=$((deadline - SECONDS))
  if (( remaining > 0 )); then
    sleep "$((remaining < 2 ? remaining : 2))"
  fi
done
if [[ "${ready}" != true ]]; then
  echo "timed out waiting for current-generation LMCacheEngine ConfigValid, Available, and AllInstancesReady conditions" >&2
  exit 1
fi
connection_deadline=$((SECONDS + 120))
connection_config=""
while (( SECONDS < connection_deadline )); do
  remaining=$((connection_deadline - SECONDS))
  request_timeout=$((remaining < 10 ? remaining : 10))
  if ! connection_config="$(kubectl get configmap "${engine}-connection" -n "${namespace}" --ignore-not-found --request-timeout="${request_timeout}s" -o json 2>/dev/null)"; then
    echo "unable to read LMCache connection ConfigMap" >&2
    exit 1
  fi
  if (( SECONDS >= connection_deadline )); then
    break
  fi
  if [[ -n "${connection_config}" ]]; then
    break
  fi
  remaining=$((connection_deadline - SECONDS))
  if (( remaining > 0 )); then
    sleep "$((remaining < 2 ? remaining : 2))"
  fi
done
if [[ -z "${connection_config}" ]] || (( SECONDS >= connection_deadline )); then
  echo "timed out waiting for LMCache connection ConfigMap" >&2
  exit 1
fi
expected_host="tcp://${engine}.${namespace}.svc.cluster.local"
if ! validate_connection_config "${expected_host}" <<< "${connection_config}"; then
  echo "LMCache connection ConfigMap does not match the required connector contract" >&2
  exit 1
fi
echo "LMCache engine and connection configuration are ready; GPU IPC and KV retrieval are not verified."
echo "Add this fragment to the LLMInferenceService in namespace ${namespace}:"
render_multiprocess
