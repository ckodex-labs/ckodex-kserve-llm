# REPO-MODEL — ckodex-kserve-llm @ 845bd53
generated: 2026-11-01T21:17:41Z | adapters used: none (codegraph CLI detected) | directly inspected tracked files: at least 23/944

**Scope:** Lower bound of tracked files directly inspected by this author; excludes untracked files and other reviewers. The worktree was dirty, so the commit does not identify all inspected content.

## Entry points
- **manager** (bin) — Start the controller manager, setup and shutdown. `cmd/manager/main.go:40`
- **LLM reconcile** (api) — Reconcile a requested LLMInferenceService. `internal/controller/llminferenceservice_reconcile.go:49`
- **admission** (api) — Register validators and defaulters when webhook setup is enabled. `internal/webhook/webhook.go:33`
- **OSCAL checks** (cli) — Read snapshot or Kubernetes resources and evaluate checks. `hack/oscal-assessment/main.go:28`
- **Dagger OSCAL** (ci) — Validate component schema and project supplied check results. `dagger/module.go:159`
- **storage initializer** (bin) — Process a model URI and destination. `cmd/storage-initializer/main.go:46`

## Module graph
Selected observed dependencies; absence of an edge is not proof of no dependency.
- `api/` — other
- `cmd/manager/` — transport
- `internal/controller/` — transport
- `internal/validation/` — validation
- `internal/runtime/` — other
- `internal/webhook/` — transport
- `internal/gateway/` — transport
- `internal/scheduler/` — other
- `hack/oscal-assessment/` — tool
- `hack/oscal-projection/` — tool
- `dagger/` — infra
- manager → controller (build) `cmd/manager/main.go:52`
- controller → validation (import) `internal/controller/llminferenceservice_reconcile.go:74`
- webhook → validation (import) `internal/webhook/llminferenceservice_validation.go:40`
- controller → gateway (build) `internal/controller/llminferenceservice_reconcile_routing.go:17`
- dagger → oscal-projection (build) `dagger/module.go:223`

## Flows
### FLOW-001 — LLM reconciliation
Trigger: Controller receives an LLMInferenceService request
1. `cmd/manager/main.go:50` — Construct manager and reconciler and register controllers.
2. `internal/controller/llminferenceservice_setup.go:66` — Register resource watches.
3. `internal/controller/llminferenceservice_reconcile.go:59` — Fetch object and stop if missing or paused.
4. `internal/controller/llminferenceservice_reconcile.go:74` — Run engine and shared surface validation before resource setup.
5. `internal/controller/llminferenceservice_reconcile_workload.go:13` — Select single-node or multi-node workload reconciliation.
6. `internal/controller/llminferenceservice_reconcile.go:105` — Reconcile routing, isolation, governance, identity and status.
- Hidden behavior: Pause annotation skips later stages: internal/controller/llminferenceservice_reconcile.go:63
- Hidden behavior: AIPack governance errors are logged non-blocking: internal/controller/llminferenceservice_reconcile_governance.go:28

### FLOW-002 — Webhook validation
Trigger: Webhook validates a create or update request
1. `internal/webhook/webhook.go:33` — Register version-specific validators and defaulters.
2. `internal/webhook/llminferenceservice_v1.go:24` — Convert stable input and delegate to alpha validator.
3. `internal/webhook/llminferenceservice_validation.go:25` — Run shared and webhook validation.
4. `internal/validation/surface.go:101` — Reject unsupported configured fields and validate runtime.
- Hidden behavior: Webhook setup is feature-dependent; registration itself is not evidence the webhook is deployed: internal/webhook/webhook.go:33

### FLOW-003 — OSCAL resource check and projection
Trigger: Caller invokes resource check and then Dagger projection
1. `hack/oscal-assessment/main.go:28` — Validate arguments and select a live API or snapshot.
2. `hack/oscal-assessment/main.go:61` — Evaluate supplied data and emit JSON; fail if any check fails.
3. `hack/oscal-assessment/evaluate.go:41` — Evaluate the requested mapped checks.
4. `dagger/module.go:198` — Mount caller-supplied OSKAL source and saved check results.
5. `hack/oscal-projection/main.go:81` — Validate supplied inputs and construct resource-check observations.
6. `dagger/module.go:223` — Validate projected OSCAL with OSKAL before exporting.
- Hidden behavior: IA-9 context is caller asserted: hack/oscal-assessment/identity_context.go:10
- Hidden behavior: Checks establish limited resource observations, not enforcement: hack/oscal-assessment/evaluate.go:80

### FLOW-004 — Hosted CI
Trigger: Push on main/release branches or pull request
1. `.github/workflows/ci.yml:3` — Configure push and pull-request triggers.
2. `.github/workflows/ci.yml:100` — Run Dagger fast lint and build gate.
3. `.github/workflows/ci.yml:113` — Run Dagger test and coverage gate.
4. `.github/workflows/ci.yml:124` — Run vulnerability scan.
5. `.github/workflows/ci.yml:157` — Run envtest integration gate.
- Hidden behavior: OSKAL functions are documented separately and not in hosted all gate: docs/ci/current-state.md:131

## Patterns
- **PAT-001 Wrapped failures** — Operational stages wrap error context before propagating failures. `internal/controller/llminferenceservice_reconcile_workload.go:33` `internal/controller/llminferenceservice_reconcile_routing.go:15`
- **PAT-002 Explicit caller context** — OSCAL identity mode and plan references come from the caller and are not independently verified by the check. `hack/oscal-assessment/identity_context.go:10` `hack/oscal-projection/main.go:149`
- **PAT-003 Separate receipt assurance** — Controller legacy receipt commitments explicitly identify missing signature and producer binding. `internal/controller/llminferenceservice_reconcile_audit.go:23` `internal/observability/audit_receipts.go:27`

## Invariants
| ID | Claim | Check | Status |
|---|---|---|---|
| INV-001 | Unknown registered-engine names are rejected. | `GOCACHE=/tmp/ckodex-assessment-cache go test -count=1 ./internal/runtime/registry -run ^TestResolveUsesDefaultAndRejectsUnregisteredEngines$` | green |
| INV-002 | IA-9 identity mode validation rejects missing and invalid context. | `GOCACHE=/tmp/ckodex-assessment-cache go test -count=1 ./hack/oscal-assessment -run ^TestIdentityContextValidation$` | green |
| INV-003 | IA-9 registration checks reject wrong namespace and invalid TTL. | `GOCACHE=/tmp/ckodex-assessment-cache go test -count=1 ./hack/oscal-assessment -run ^TestRegistrationNamespaceAndTTL$` | green |
| INV-004 | AC-4 resource check reports its presence-only boundary. | `GOCACHE=/tmp/ckodex-assessment-cache go test -count=1 ./hack/oscal-assessment -run ^TestNetworkPolicyCheckReportsPresenceOnly$` | green |
| INV-005 | Deployment manifest check rejects containers lacking explicit non-root settings. | `GOCACHE=/tmp/ckodex-assessment-cache go test -count=1 ./hack/deployment-assessment -run ^TestDeploymentCheck$` | green |

## Discrepancies
### DIS-001 — high
Claimed: Blocked scheduler reconciliation calls its route fail-closed. `internal/controller/llminferenceservice_reconcile_scheduler.go:72`
Actual: HTTPRoute builder defaults to the direct service when SchedulerReady is not true; actual Gateway behavior requires runtime verification. `internal/gateway/httproute_builder.go:34`
Disposition: unknown

### DIS-002 — info
Claimed: CI documentation describes pull requests to main. `docs/ci/current-state.md:41`
Actual: Workflow pull_request trigger has no branch filter. `.github/workflows/ci.yml:6`
Disposition: fix-docs

## Do not touch
- `api/ and generated CRDs` — Deepcopy and CRD generation can overwrite existing modifications; review generator and diff before regeneration. `Makefile:21`
- `Dagger generated bindings` — Generated SDK is managed by Dagger code generation. `dagger/dagger.gen.go:1`
- `scheduler routes` — Direct-service fallback and stated blocked-route semantics need explicit review before behavioral change. `internal/gateway/httproute_builder.go:34`

## Open questions
- What current runtime evidence establishes actual Gateway and EPP behavior when the scheduler is unavailable? internal/gateway/httproute_builder.go:34
- What deployment-specific identity and keys, if any, establish signed receipt production and verification? internal/observability/audit_receipts.go:27
- Which OSKAL source revisions and plan references are approved for hosted use? dagger/module.go:159
- What Kubernetes and accelerator evidence qualifies the image and model profiles in this dirty checkout? internal/runtime/registry/registry.go:24
