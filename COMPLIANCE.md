# Security Compliance & OSCAL Mapping

This document outlines the security controls implemented by the **ckodex-kserve-llm** operator, mapped to the **NIST 800-53 Rev 5** catalog. The project uses **OSCAL (Open Security Controls Assessment Language)** for machine-readable transparency and automated validation via **Lula**.

## 1. Control Mapping (NIST 800-53)

| Control ID | Control Name | Operator Feature | Condition / Evidence |
| :--- | :--- | :--- | :--- |
| **AC-4** | Information Flow Enforcement | ToolSurface Istio DPI & default-deny NetworkPolicies | `Compliance-AC-4` condition · `lula/network-policy-validation.yaml` |
| **AU-2** | Audit Events | Structured JSONL audit logging to shared audit plane | `Compliance-AU-2` condition · `lula/governance-validation.yaml` |
| **CA-7** | Continuous Monitoring | Lifecycle (L) plane tracks active/healthy state via Deployment readiness | Lifecycle state = `active` |
| **IA-9** | Service Identification and Authentication | SPIFFE/SPIRE issues X.509 SVIDs for all inference workloads (non-person entity auth) | SVID issuance via `SPIREReconciler`; no Lula validator yet — tracked L-DOC-003 |
| **SI-4** | System Monitoring | Open Inference Signals (OIS) v0.1 behavioral telemetry | `Compliance-SI-4` condition · `lula/ois-validation.yaml` |
| **SI-7** | Software, Firmware, and Information Integrity | Cosign signatures + SLSA provenance + SBOM attestation | `Compliance-SI-7` condition · `lula/supply-chain-validation.yaml` |
| **SR-2** | Supply-Chain Risk Management Plan | Supply-Chain Contract v1.0 (the risk-management plan wrapper) | `Compliance-SR-2` condition · `lula/supply-chain-validation.yaml` |
| **SR-4** | Provenance | Release workflow generates SLSA provenance + cosign attestation per artifact | Release OIDC-backed provenance artifacts; tag-driven GHA path |

## 2. Governed States (L|T|R)
The operator's internal state machine supports these controls, but not every signal should be interpreted as cryptographic proof:

- **Lifecycle (L)**: Maps to **CA-7** (continuous monitoring) — the `active` lifecycle state reflects that health checks are passing continuously, not that a contingency plan (CP-2) exists.
- **Trust (T)**: Maps to **SI-7** (software integrity — signature/provenance/SBOM verification) and **SR-4** (provenance of artifacts in the supply chain). **AC-4** (information flow enforcement) belongs to the isolation pillar, not the Trust plane.
- **Risk (R)**: Maps to **SI-4**. If OIS signals detect anomalous tool usage or out-of-bounds metrics, the risk level escalates and can trigger quarantine.

## 3. Automated Assessment
Compliance evidence is generated continuously, but the evidence surface is still environment-dependent:

- **Build Time**: The Dagger CI pipeline runs `lula validate` against the **OSCAL Component Definition** in `lula/lula-component.yaml`.
- **Artifacts**: CI exports `oscal-assessment-results.yaml` to `bin/`. On tagged releases, GitHub Actions is configured to publish signing and provenance artifacts for downstream review.
- **Enforcement**: **FedRAMP Mode** and admission controls enforce configuration policy, but supply-chain verification should only be treated as complete when `Compliance-SR-2` is backed by recorded cryptographic results.

> **Implementation status (as of v0.18.0-beta.1):** AC-4, AU-2, SI-4, SI-7, and SR-2 are validated by Lula in CI. CA-7 and SR-4 are tracked via reconciler conditions and release artifacts respectively. IA-9 (SPIFFE/SPIRE SVID issuance) has no Lula validator yet — see L-DOC-003 in `docs/open-loops.md`.

---

> [!TIP]
> To run the compliance validation locally, use the following command:
> ```bash
> lula validate -f lula/lula-component.yaml
> ```
