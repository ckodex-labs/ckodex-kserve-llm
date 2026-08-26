# ADR 011: Canonical Observability and Evidence Planes (CKC-OBS)

**Implementation classification: S — proposed architecture with partial
evidence-health and audit-failure instrumentation.**

## Status

Proposed

## Context

CKC-OBS separates three planes: high-volume operational telemetry, low-volume
canonical signals, and integrity-protected evidence. The operator currently
serves all three roles from one component, which is why volume, retention, and
integrity cannot be reasoned about independently.

Real foundations already exist in `internal/observability`:

- `AuditLogger` carries PII redaction, a direct OTLP sink, and Kubernetes events.
- `LogRichInferenceSignal` is documented as recording a full inference-profile
  envelope.
- The `urn:ois:` scheme is a working correlation vocabulary.
- The outcome vocabulary — `allow`, `deny`, `degrade`, `quarantine` — is four
  fifths of the CKC-OBS §15 checkpoint disposition set.
- SPIRE already issues workload SVIDs, so producer identity is nearly free.

Four gaps block conformance:

1. **No integrity.** There is no signature, hash chain, sequence number, or
   Merkle commitment anywhere on the audit path.
2. **Content-bearing receipt types.** `ContentMessage` and `ContentPart` carry
   raw text and base64 payloads by construction. CKC-OBS §7 requires
   `content -> canonicalize -> hash/commit -> reference`. Redaction after the
   fact is not minimization.
3. **Incomplete failure semantics.** Kubernetes event-write errors are now
   logged, but profile-specific degrade/deny/quarantine semantics and direct
   OTLP export remain incomplete. CKC-OBS §13 still requires the final policy.
4. **Namespace drift.** Four Prometheus prefixes are in use — `ckodex_lmc_*`,
   `ckodex_governance_*`, `ckodex_inference_*`, `ckodex_resilience_*` — and
   events split between `ckodex.infer.*` and `ckodex.inference.*` for the same
   concept. CKC-OBS §9 requires `exp.*` / `infer.*` / `ops.*`.

A fifth, subtler problem: the promotion gate in the model-onboarding controller
queries `ckodex_inference_requests_total`, an operator-emitted metric that will
not survive the arrival of a second engine.

## Decision

- Split `internal/observability` into three packages with distinct retention and
  integrity postures: `internal/telemetry` (OTel, `infer.*` / `ops.*`),
  `internal/signal` (canonical, typed, causally linked), and
  `internal/evidence` (integrity-protected, independently verifiable).
- Adopt the canonicality rule as a registration-time check. A canonical signal
  type declares consumer, owner, latency, schema, purpose, integrity, retention,
  and failure behaviour. Registration of a type missing any of these is refused.
- Implement the canonical envelope as a Go type plus a JSON Schema in `schema/`,
  embedded by every canonical event.
- Produce `InferenceReceipt` from an evidence sidecar on the OpenAI path rather
  than from each engine. The Vector sidecar already injected by `injectVector`
  is the host; per-engine field derivations come from the `ReceiptContract`
  defined in ADR-010. Emitting from engines would produce one dialect per engine
  and put conformance at the mercy of upstream release cycles.
- Enforce content minimization in the type system. The receipt type must not be
  able to hold raw prompt, retrieved documents, memory, model output, or
  chain-of-thought. Where raw material must be retained, it lives in an
  independently governed store with its own classification, ACL, retention,
  deletion, and access evidence; the receipt references it.
- Add integrity: producer identity from the SPIRE workload SVID, a signature, a
  sequence chain, a Merkle commitment per window, and a transparency-proof
  reference.
- Migrate metric and event names to `exp.*` / `infer.*` / `ops.*` behind a
  deprecation window that emits both names, and add a lint rule preventing a
  fifth prefix.
- Source the required operational metric families through the adapter
  `MetricsContract`, so they are identical across engines.
- Add `EvidenceCheckpoint` at each declared state transition, with dispositions
  `PASS`, `DEGRADE`, `DENY`, `ESCALATE`, `QUARANTINE`. Extend the existing
  outcome vocabulary with `escalate`.
- Observe the evidence system itself: missing expected receipt, signature
  failure, sequence gap, broken causal edge, clock anomaly, collector loss,
  export failure, verification failure, unauthorized evidence access, retention
  or deletion failure.
- Declare graded failure semantics per deployment profile. A missing required
  receipt is an evidence failure; a missing authority binding denies; an invalid
  signature quarantines; an unavailable sink spools within bounds or fails
  closed. The existing FedRAMP and air-gap configuration are the profile hooks.
- Keep OpenTelemetry as transport and correlation only. Evidence schemas,
  signatures, and storage stay independent of the telemetry backend.

## Consequences

- Evidence survives replacement of the telemetry vendor, which is a stated
  conformance requirement and is not satisfiable if OTel is the ledger.
- Receipt production sits on the inference path and costs latency. The
  canonicality rule is what bounds this: not every invocation needs a retained
  receipt. The sampling policy must be explicit and measured in the cross-engine
  acceptance matrix.
- Enabling failure semantics converts silent drops into denials. Every graded
  response ships with an audit mode first.
- Several AIPack risk-valence signals become available as a side effect of the
  `infer.*` drift and safety families, reducing the cost of AIPACK-SPEC §13.
- Admission needs no bespoke evidence type. Its hooks emit `PolicyDecision`, its
  chain emits `AdmissionReceipt`, and its asynchronous stages emit
  `EvidenceCheckpoint` — all producers on this plane. See ADR-012.

## References

- [ADR-010: Runtime engine contract](010-runtime-engine-contract.md)
- [ADR-012: Model admission planes](012-model-admission-planes.md)
- [Remediation plan](../remediation-plan.md)
