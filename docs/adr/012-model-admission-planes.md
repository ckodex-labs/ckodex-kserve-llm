# ADR 012: Model Admission in Two Planes

**Implementation classification: S — proposed architecture; the full two-plane
admission controller is not implemented.**

## Status

Proposed

## Context

The operator has no model admission control. It has two adjacent things:

- A validating and mutating webhook pair (`internal/webhook`) that checks
  parallelism, GPU devices, KV transfer, and resources. It is gated off by
  default and its Helm rules route `v1` admission reviews to the `v1alpha2`
  handler path.
- A `ModelOnboarding` controller with staged execution, phases, rollback, and
  Prometheus-backed gates. Its gate evaluation correctly fails closed when the
  metrics backend is unreachable, which is the right posture and worth keeping.

The unknown-stage gap is now closed: unsupported stage types return an error.
One larger gap still makes the existing pipeline advisory rather than enforcing:

- Reaching a terminal phase changes status but does not gate serving. The
  Deployment and the route exist regardless.

Meanwhile the governance inputs an admission decision would need are either
unimplemented or partial. Predicate presence no longer produces a positive
verification verdict, but cosign verification, Rekor lookup, and artifact
digest binding are not implemented.

## Decision

Build admission as two distinct planes, and express both in the CKC-OBS
vocabulary rather than inventing a parallel evidence type.

### Synchronous plane

- A cluster-scoped `ModelAdmissionPolicy` holding an ordered hook chain
  evaluated before the object persists.
- Each hook carries its own `failurePolicy` and an `enforcementMode` of
  `Enforce`, `Audit`, or `DryRun`, so the chain can be rolled out against live
  traffic without breaking it.
- Built-in hooks: capability (from the ADR-010 adapter), provenance, policy,
  risk band, deprecation, quarantine, air-gap origin, evidence readiness, tenant
  quota, and resource sanity.
- `ModelAdmissionHook` points at an external HTTPS or gRPC service over a
  versioned request/response contract with a timeout, so a scanner can be added
  without forking the operator.
- A hook emits a `PolicyDecision`; the chain emits an `AdmissionReceipt`.

### Asynchronous plane

- `ModelOnboarding` graduates into the progressive-admission pipeline. Stage
  types extend from `validation | canary | gate | promotion` to
  `provenance | policy | smoke | eval | vad | canary | gate | promotion`.
- Each stage emits an `EvidenceCheckpoint`.
- An unrecognised stage type fails. It does not skip.
- `GateCriteria` extends beyond success rate and P99 to eval score thresholds
  against an `EvalProfile`, VAD class coverage, maximum risk band, and cost or
  token budget.

### Enforcement

- `spec.admission.required: true` withholds the Deployment and the route
  attachment until the pipeline reaches `Admitted`.
- `spec.admission.mode: audit` records the decision without withholding, for
  rollout.
- An `Admitted` condition carries the `AdmissionReceipt` reference.

## Consequences

- The synchronous plane runs inside an API request and must stay fast and
  deterministic. Anything requiring live traffic, an eval run, or a canary
  window belongs in the asynchronous plane, which may take hours and must
  survive restarts. Keeping the two separate is a hard constraint, not a
  preference.
- Admission depends on real attestation verification. Shipping the provenance
  hook against the current presence-check implementation would produce a gate
  satisfiable by typing strings into YAML. AIPACK-SPEC §6/§7 verification is a
  prerequisite, not a parallel workstream.
- Admission depends on the ADR-010 capability matrix for its first hook and the
  ADR-011 evidence plane for its receipts. It is the join point of both.
- Turning enforcement on converts silent passes into denials. Every hook ships
  with `Audit` first, and enforcement flips per namespace once the audit record
  is quiet.
- The webhook version-routing defect must be fixed first, or admission decisions
  will be computed against a decoded object of the wrong shape.

## References

- [ADR-010: Runtime engine contract](010-runtime-engine-contract.md)
- [ADR-011: Canonical observability planes](011-canonical-observability-planes.md)
- [Remediation plan](../remediation-plan.md)
