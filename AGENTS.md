# Agent engineering guidance

Context for AI coding agents working in the `ckodex-kserve-llm-operator` repository.
Last updated: 2026-11-06 (Release `v0.20.0`, commit `9da7f18`).

## Repository scope and purpose

The `ckodex-kserve-llm-operator` is a Kubernetes operator managing large language model and multimodal inference services on KServe. It provides custom resources for:

- `LLMInferenceService` (`serving.ckodex.com/v1`): Core LLM workload managing vLLM v0.33.0, SGLang, and TensorFold runtimes.
- `AIPack` (`serving.ckodex.com/v1alpha2`): AI artifact packaging (16 total kinds: 15 atomic kinds A1-A15 plus 1 composite kind C1 `Agent`, including `BaseModel`, `LoRA`, `Skill`, and `UTCPManual`).
- `MultimodalInferenceService` (`serving.ckodex.com/v1alpha2`): Foundation vision models (Baidu Unlimited-OCR-2 for `task: ocr`, Meta SAM 3 for `task: segmentation`).
- `Agent` and `SkillRegistry` (`serving.ckodex.com/v1alpha2`): Autonomous agent identity and UTCP tool manual bindings.
- `LocalModelCache` (`serving.ckodex.com/v1alpha2`): Node-local model weight pre-staging.

Read `docs/explanation/overview.md` before modifying controller logic or CRD schemas.

## Four spaces architecture

The codebase enforces strict separation across Four Spaces:

- `internal/kernel/`: Pure domain logic, state machines, typed state vectors ($S(e,t)$), and UTCP protocol definitions. Must NOT import Kubernetes, cloud SDKs, databases, gRPC, HTTP, or specific telemetry providers.
- `internal/validation/`: Shared policy, admission gates, and boundary validations. Both the kernel and presentation layers call into validation.
- `internal/controller/`, `cmd/`, `internal/webhook/`: Presentation layer. Pure mapping between custom resources and Kubernetes primitives. Do NOT put business logic or validation into controllers.
- `internal/proof/`, `internal/evidence/`: Evidence generation, Ed25519 signing, RFC 9162 Merkle flight recorder, and OSCAL projection.

## Key files and components

| File or directory | Purpose |
|---|---|
| `cmd/manager/main.go` | Operator entrypoint; registers reconcilers and admission webhooks |
| `internal/controller/llminferenceservice_controller.go` | Primary reconciler for LLM workloads |
| `internal/controller/reconciler/diff.go` | Normalized pod and container diff engine (`L-OP-018`) |
| `internal/utcp/` | Universal Tool Calling Protocol (UTCP v1.0.1) engine and converters |
| `internal/runtime/vllm/adapter.go` | vLLM v0.33.0 runtime adapter and command generator |
| `deploy/helm` | Canonical Helm chart (`L-OP-013`) |
| `charts/ckodex-kserve-llm-operator` | Public chart synchronized via byte-parity contract |
| `hack/helm-contract/` | Enforces byte parity between Helm charts |
| `hack/oscal-assessment/` | Evaluates cluster compliance against NIST SP 800-53 controls |
| `docs/explanation/overview.md` | Canonical product narrative and architecture overview |

## CI/CD and verification commands

The repository uses Dagger pipelines as the primary CI implementation:

```bash
dagger call lint --source=.     # Runs go vet and golangci-lint
dagger call test --source=.     # Runs go test -race with coverage gates
dagger call build --source=.    # Builds operator container (amd64)
dagger call scan --source=.     # Trivy CVE scan (exit code 1 on HIGH/CRITICAL)
dagger call lula --source=. export --path=assessment-results.yaml  # OSCAL assessment
dagger call all --source=.      # Fast gate: lint and compilation check
```

Local contract checks:

```bash
go test -count=1 ./hack/helm-contract/...
go test -count=1 ./hack/public-release-contract/...
```

## Code style and engineering conventions

- **File and function limits**: Maximum file length 500 lines of code. Maximum function length 50 lines of code.
- **Unslop discipline**: Use sentence case for all headings. Use active voice. Ban buzzwords ("pivotal", "tapestry", "seamless", "delve", "foster"). Avoid em dashes entirely (use periods or commas). Do not use decorative emojis in headings. Use straight quotes (`"`).
- **Error handling**: Wrap Go errors with context using `%w`; never discard errors silently.
- **Diff discipline**: Reconcilers must use diff normalization in `internal/controller/reconciler/diff.go` to prevent infinite update loops.
- **Purity**: Domain state transitions must remain pure and testable without live cluster dependencies.
