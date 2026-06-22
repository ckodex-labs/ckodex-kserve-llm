# Local KIND E2E Deployment

This directory contains the repo-native local bootstrap for a real KIND-based
end-to-end run of the operator.

The main entrypoints are:

- `./run/setup.sh` - validates local prerequisites on worktree creation
- `./run/e2e.sh` - creates or reuses the KIND cluster, installs dependencies,
  builds and loads the operator image, deploys the operator, applies the sample
  `LLMInferenceService`, and probes `/v1/chat/completions`
- `./run/capacity-plan.sh` - prints a static fit assessment for frontier models
  without downloading weights or starting pods
- `./run/cleanup.sh` - removes the KIND cluster and prunes leftover local state

The lower-level steps remain in `local/` and are used by the wrapper:

- `local/01-kind-setup.sh`
- `local/02-prereqs.sh`
- `local/03-kserve-helm-install.sh`
- `local/04-llm-inference-service.yaml`
- `local/05-test-inference.sh`
- `local/06-cleanup-kind-space.sh`

The local KIND path is sized for repo-native verification. For GLM-5.2 and
Kimi K2.7 Code, use the static capacity planning notes in
[`docs/model-capacity.md`](../docs/model-capacity.md) before attempting any
real cluster sizing work.
