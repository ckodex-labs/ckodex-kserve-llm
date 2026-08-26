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

The inference probe waits for the controller-created pod, sends the declared
HTTPRoute hostname when using the Gateway address, retries transient startup
failures, requires a non-empty OpenAI-compatible `choices` response, and removes
any fallback port-forward when the run exits.

The default proof uses the signed `hf://` storage-initializer path and does not
require privileged FUSE/CSI support. The optional `hf-mount://` profile is kept
in `local/04-llm-inference-service-hfmount.yaml`; enable its prerequisite with
`INSTALL_HF_CSI=1` before applying that profile.

The KIND bootstrap pins `kindest/node:v1.35.0` and requires Docker to expose at
least 65,536 file descriptors to the node container. If the preflight reports a
lower `nofile` limit, increase Docker Desktop/container limits before retrying;
the check prevents a misleading systemd boot failure.

The wrapper uses `make docker-build` and `make storage-initializer-load`; both
targets use BuildKit with `--load`, and the repository root ignores generated
console/build output so KIND setup does not transfer local dependency or bundle
directories into the operator image build.

The local KIND path is sized for repo-native verification. For GLM-5.2 and
Kimi K2.7 Code, use the static capacity planning notes in
[`docs/model-capacity.md`](../docs/model-capacity.md) before attempting any
real cluster sizing work.
