#!/usr/bin/env bash
# test/k6/scripts/teardown.sh
# Kills all port-forwards started by port-forward.sh and deletes test CRs.
set -euo pipefail

PID_FILE="${TMPDIR:-/tmp}/k6-pf.pids"
NS="${K6_NAMESPACE:-ckodex-inference}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

if [[ -f "${PID_FILE}" ]]; then
  echo "==> Killing port-forwards"
  while read -r pid; do
    kill "${pid}" 2>/dev/null || true
  done < "${PID_FILE}"
  rm -f "${PID_FILE}"
else
  echo "==> No PID file found — port-forwards may already be down"
fi

echo "==> Deleting test CRs"
kubectl delete -f "${REPO_ROOT}/test/k6/k8s/" -n "${NS}" --ignore-not-found

echo "==> Teardown complete"
