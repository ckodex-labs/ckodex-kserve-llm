# Compliance as code with OSCAL and Lula

This document explains how the CKodex KServe LLM Operator implements continuous compliance through the Open Security Controls Assessment Language (OSCAL) and automated Lula verification against NIST SP 800-53 Rev 5 security controls.

## Overview

Traditional compliance audits rely on point-in-time screenshots and manual spreadsheets. In contrast, the operator treats compliance as executable code:

1. **Declarative component definition**: `oscal/component-definition.yaml` models platform controls using the NIST OSCAL v1.1.2 schema.
2. **Automated evidence evaluation**: Dedicated validation tools (`hack/oscal-assessment`) inspect live cluster objects and evaluate compliance rules.
3. **Machine-readable assessment results**: Dagger CI pipelines execute Lula to export verifiable compliance bundles before any release is tagged.

## NIST SP 800-53 Rev 5 control mappings

The operator actively implements and verifies the following security controls:

| Control ID | Control title | Implementation mechanism | Verification logic |
|---|---|---|---|
| **AC-4** | Information flow enforcement | Kubernetes NetworkPolicies fence workload namespaces | Checks that at least one `NetworkPolicy` exists in every namespace hosting an `LLMInferenceService`. |
| **AU-2** | Event logging | RFC 9162 Merkle flight recorder captures all mutation receipts | Evaluates whether audit events and execution receipts are committed to tamper-evident storage. |
| **IA-9** | Service identification and authentication | SPIRE registration reconciler issues X.509 SVIDs | Verifies SPIFFE ID formatting, workload selectors, SVID TTL, and DNS SAN configuration for all services. |
| **SC-7** | Boundary protection | Envoy Gateway 1.2.0 route fencing | Confirms external traffic terminates at Envoy Gateway with immediate route fencing capability. |
| **SC-8** | Transmission confidentiality and integrity | Mutual TLS with automated certificate rotation | Enforces mTLS across pod-to-pod and gateway-to-workload communications. |
| **SI-4** | Information system monitoring | Open Inference Signals (OIS) telemetry pipeline | Verifies Prometheus metric emission on `/metrics` for queue depths and GPU cache factors. |
| **SI-7** | Software and information integrity | Cosign signatures, SLSA Level 3 provenance, and CycloneDX SBOMs | Inspects `Compliance-SI-7` status condition on workloads, requiring reason `IntegrityVerified`. |
| **SR-2** | Supply chain risk management | Digest pinning and multi-architecture image verification | Rejects tag-based container or artifact references at webhook admission time. |

## The OSCAL component definition

The operator maintains `oscal/component-definition.yaml` as the authoritative source of compliance definitions.

An excerpt illustrating the SI-7 integrity requirement:

```yaml
control-implementations:
  - uuid: 33333333-3333-4333-8333-333333333333
    source: https://doi.org/10.6028/NIST.SP.800-53r5
    description: NIST 800-53 security controls for containerized inference
    implemented-requirements:
      - uuid: 44444444-4444-4444-8444-444444444447
        control-id: si-7
        description: >-
          Ensure software and information integrity via cosign signature
          verification, SLSA provenance attestation, and CycloneDX SBOM
          presence on all LLMInferenceService workloads.
        set-parameters:
          - param-id: si-7_prm_1
            values:
              - "cosign signature + SLSA provenance + CycloneDX SBOM"
        remarks: >-
          The operator records cryptographic verification results as
          Kubernetes status conditions (Compliance-SI-7). The resource check evaluates
          that this condition is True and has reason IntegrityVerified
          on all LLMInferenceService resources cluster-wide.
```

## Running automated assessments

The operator includes a dedicated assessment engine in `hack/oscal-assessment`:

```bash
# Run the local assessment suite against a cluster snapshot
go test -count=1 ./hack/oscal-assessment/...
```

To run a Lula assessment in CI and export the structured assessment results:

```bash
dagger call lula --source=. export --path=assessment-results.yaml
```

The output conforms to the OSCAL Assessment Results schema, ready for ingestion by sovereign compliance dashboards, enterprise risk management systems, and automated auditor pipelines.

## Next steps

- [Review the overall architecture and Four Spaces](overview.md)
- [Consult zero-trust security architecture](security-architecture.md)
- [Inspect component catalog and data flows](components.md)
