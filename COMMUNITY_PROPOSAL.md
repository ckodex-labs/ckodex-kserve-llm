# Proposal: Hardened LLM Serving Profile for Regulated Environments

## Summary

This proposal introduces a hardened deployment profile for KServe-based LLM serving that is aimed at regulated environments such as financial services, healthcare, and public sector teams.

The goal is not to redefine KServe. The goal is to make it easier to adopt KServe in environments that need stronger identity, policy enforcement, provenance, and audit evidence before LLM workloads can be put into production.

## Why This Matters

Most LLM serving stacks focus on throughput and routing first. Regulated teams need something different:

- workload identity that can be verified end to end,
- traffic controls that are policy-driven rather than ad hoc,
- signed artifacts and provenance that survive audit review,
- machine-readable compliance evidence that can be generated in CI,
- and a deployment model that supports both connected and air-gapped environments.

Without those controls, security reviews become manual, slow, and expensive. That slows down the very teams that could benefit most from AI assistance.

## Proposal

We propose a hardened profile that layers the following capabilities onto a standard KServe deployment:

1. Native workload identity with SPIFFE/SPIRE.
2. Gateway API-based routing for HTTP and gRPC inference traffic.
3. Open Inference Signals for behavioral telemetry and policy decisions.
4. OSCAL/Lula validation for build-time compliance evidence.
5. Cosign signing and SLSA provenance hooks for release artifacts.
6. Optional air-gap support for disconnected environments.

The implementation in this repository demonstrates those ideas as a working operator, while some verification paths remain explicitly environment-dependent or pending stronger runtime proof wiring.

## What Should Be Upstreamed

The upstream contribution should focus on reusable primitives:

- optional hardened manifests and overlays,
- telemetry contracts that other operators can emit,
- policy and validation hooks that can be reused across deployments,
- and release-time signing/provenance patterns that are easy to audit.

The intent is to upstream the control points, not to force one opinionated enterprise stack on everyone.

## What Should Stay Optional

To keep adoption realistic, the hardened profile should remain opt-in:

- SPIRE should be enabled only when a cluster needs it,
- compliance validation should be profile-based,
- telemetry should be pluggable,
- and release signing should work without requiring a custom internal PKI.

## Success Criteria

This proposal is successful if a team can:

- review signed release artifacts and attached provenance,
- deploy a model with clear policy boundaries,
- generate evidence artifacts automatically,
- and hand that evidence to security, risk, and audit reviewers without manual reconstruction.

## Community Ask

We would value feedback on:

- which pieces are best suited for upstream KServe,
- how to keep the hardened profile composable,
- how to standardize AI workload telemetry without overfitting to one runtime,
- and how to make the adoption path clear for regulated enterprises.

## References

- [Security Architecture](docs/SECURITY_ARCHITECTURE.md)
- [API Deprecation Policy](docs/api-deprecation-policy.md)
- [Enterprise Marketing Kit](docs/MARKETING_KIT.md)
