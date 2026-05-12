# Security Compliance & OSCAL Mapping

This document outlines the security controls implemented by the **ckodex-kserve-llm** operator, mapped to the **NIST 800-53 Rev 5** catalog. The project uses **OSCAL (Open Security Controls Assessment Language)** for machine-readable transparency and automated validation via **Lula**.

## 1. Control Mapping (NIST 800-53)

| Control ID | Control Name | Operator Feature | Evidence / Validation |
| :--- | :--- | :--- | :--- |
| **AC-4** | Information Flow Enforcement | ToolSurface DPI & NetworkPolicies | `lula/network-policy-validation.yaml` |
| **AU-2** | Audit Events | Structured Audit Logging | `lula/governance-validation.yaml` |
| **SI-4** | System Monitoring | **Open Inference Signals (OIS) v0.1** | `lula/ois-validation.yaml` |
| **SI-7** | Software & Info Integrity | **Cosign Signatures** & SLSA Provenance | `lula/supply-chain-validation.yaml` |
| **SR-2** | Supply Chain Risk Mgmt | **Supply-Chain Contract** (v1.0) | `lula/supply-chain-validation.yaml` |

## 2. Governed States (L|T|R)
The operator's internal state machine supports these controls, but not every signal should be interpreted as cryptographic proof:

- **Trust (T)**: Maps to **SI-7** and **AC-4**. Most workloads are `asserted` unless the controller has recorded a verifiable provenance result in addition to network-isolation evidence.
- **Risk (R)**: Maps to **SI-4**. If OIS signals detect anomalous tool usage or out-of-bounds metrics, the risk level escalates and can trigger quarantine.

## 3. Automated Assessment
Compliance evidence is generated continuously, but the evidence surface is still environment-dependent:

- **Build Time**: The Dagger CI pipeline runs `lula validate` against the **OSCAL Component Definition** in `lula/lula-component.yaml`.
- **Artifacts**: CI exports `oscal-assessment-results.yaml` to `bin/`. On tagged releases, GitHub Actions is configured to publish signing and provenance artifacts for downstream review.
- **Enforcement**: **FedRAMP Mode** and admission controls enforce configuration policy, but supply-chain verification should only be treated as complete when `Compliance-SR-2` is backed by recorded cryptographic results.

---

> [!TIP]
> To run the compliance validation locally, use the following command:
> ```bash
> lula validate -f lula/lula-component.yaml
> ```
