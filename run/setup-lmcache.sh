#!/usr/bin/env bash
set -euo pipefail

operator_version="operator-v0.1.1"
operator_url="https://github.com/LMCache/LMCache/releases/download/${operator_version}/install.yaml"
operator_sha256="8e5eab17ccead2915fc54465e485a9f2c6c947b9fb1301c2149ba1a94dc7e609"

mode=""
namespace=""
engine="ckodex-lmcache"
size_gb="20"
apply=false
ack_privileged=false

usage() {
  echo "usage: $0 --mode inProcess|multiprocess --namespace NAMESPACE [--engine NAME] [--size-gb N] [--apply --ack-privileged-namespace]"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) mode="${2:-}"; shift 2 ;;
    --namespace) namespace="${2:-}"; shift 2 ;;
    --engine) engine="${2:-}"; shift 2 ;;
    --size-gb) size_gb="${2:-}"; shift 2 ;;
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
        localCPUSizeGiB: 20
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

if [[ "${mode}" == "inProcess" ]]; then
  echo "Validated in-process LMCache configuration for namespace ${namespace}."
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
  exit 0
fi
if [[ "${ack_privileged}" != true ]]; then
  echo "refusing to apply multiprocess resources without --ack-privileged-namespace" >&2
  exit 2
fi
command -v kubectl >/dev/null 2>&1 || { echo "kubectl is required" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "curl is required" >&2; exit 1; }
kubectl version --request-timeout=10s >/dev/null

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
kubectl apply -f - <<YAML
apiVersion: lmcache.lmcache.ai/v1alpha1
kind: LMCacheEngine
metadata:
  name: ${engine}
  namespace: ${namespace}
spec:
  l1:
    sizeGB: ${size_gb}
YAML
kubectl wait --for=condition=Ready "lmcacheengine/${engine}" -n "${namespace}" --timeout=10m
for _ in $(seq 1 60); do
  if kubectl get configmap "${engine}-connection" -n "${namespace}" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
kubectl get configmap "${engine}-connection" -n "${namespace}" >/dev/null
echo "LMCache multiprocess connection is ready. Add this fragment to the LLMInferenceService in namespace ${namespace}:"
render_multiprocess
