#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

missing=()
for cmd in docker kind kubectl helm curl jq; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    missing+=("$cmd")
  fi
done

if ((${#missing[@]} > 0)); then
  printf 'Missing required tools for local KIND E2E: %s\n' "${missing[*]}" >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "Docker daemon is not reachable. Start Docker before running ./run/e2e.sh." >&2
  exit 1
fi

echo "Local KIND E2E prerequisites are available."
echo "Run ./run/e2e.sh for the full cluster bootstrap, deploy, and inference probe."
echo "Run ./run/cleanup.sh to tear down KIND and prune leftover local state."
