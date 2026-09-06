# CKodex KServe LLM Operator beta contract

This directory is the beta release contract for the operator and its observe-only
console. It is intentionally narrower than the full CRD surface: a resource
schema or feature gate is not, by itself, a beta product promise.

## Beta posture

The beta is a controlled, authenticated deployment for platform teams operating
KServe-managed inference workloads. The console is observe-only. It does not
create, scale, promote, roll back, or delete workloads.

The beta must be installed from the published OCI Helm chart and image artifacts,
not from a working directory. The chart and console image are one release unit;
the console must not be silently omitted by checkout, CI, or packaging.

## Claim classes

- `C`: implemented, tested, and enforceable in the current release pipeline.
- `S`: design and partial implementation exist, but a required runtime or release
  acceptance artifact is still missing.
- `A`: aspirational or explicitly outside the beta boundary.

Every public beta claim is recorded in [acceptance-matrix.yaml](acceptance-matrix.yaml)
with an evidence path and an exit criterion.

The stable API install topology is defined in [install.md](install.md). It binds
the v1/v1alpha2 conversion webhook to the published chart's fixed beta identity;
an arbitrary Helm release name is not equivalent evidence.

The full assessment findings, phased execution plan, and local-versus-external
evidence boundary are in [plan.md](plan.md).

## Targeted beta promises

The beta contract targets a reproducible operator install, stable core LLM
inference resource handling, qualified readiness observation, workload
investigation, read-only telemetry and identity observations, and an advisory
assistant with no cluster tools or mutation authority. These are target
acceptance promises, not proof that every gate is closed in the current release
candidate; the readiness ledger below is authoritative for current evidence.

GPU, multi-node, LMCache, automated traffic promotion, cryptographic artifact
verification, public multi-tenancy, and Agent/SkillRegistry runtime execution
remain `S` or `A` until their specific acceptance evidence is present.
The console production dependency audit is enforced in CI and must remain clear
of production advisories before promotion.

## Release gate

The beta readiness ledger must be green before a beta tag is promoted:

1. A fresh checkout contains the complete source, including the console.
2. CI runs the console checks and cannot downgrade them to a notice.
3. The published chart renders the console profile and the published console
   image has provenance, SBOM, signature, and HIGH/CRITICAL vulnerability-scan
   evidence.
4. At least one supported runtime profile passes declaration, reconciliation,
   readiness, inference, restart, and recovery acceptance.
5. Authentication, namespace boundaries, and audit qualification are explicit.
6. Stable API documentation, samples, and generated CRDs agree.
7. Browser, keyboard, reduced-motion, high-contrast, and screen-reader checks
   cover the supported console journeys.
8. Known limitations and excluded capabilities are published.

The machine-readable matrix is the source for release review; this document is
the user-facing interpretation of that matrix.
