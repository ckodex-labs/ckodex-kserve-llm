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
The operator's internal state machine directly supports these controls:

- **Trust (T)**: Directly maps to **SI-7** and **AC-4**. A workload achieves `verified` trust only after its supply-chain provenance (Cosign) and network isolation (DPI) are cryptographically asserted.
- **Risk (R)**: Real-time risk assessment maps to **SI-4**. If OIS signals detect anomalous tool usage or out-of-bounds metrics, the risk level escalates, potentially triggering automated quarantine.

## 3. Automated Assessment
Compliance is not a point-in-time snapshot but a continuous property of the build and runtime:

- **Build Time**: The Dagger CI pipeline runs `lula validate` against the **OSCAL Component Definition** in `lula/lula-component.yaml`.
- **Artifacts**: Every production build generates an `oscal-assessment-results.yaml` in the `bin/` directory, providing a verifiable receipt of compliance status.
- **Enforcement**: **FedRAMP Mode** (when enabled) acts as a high-integrity gateway, rejecting any model artifacts that do not meet the strict Supply-Chain Contract (SR-2).

---

> [!TIP]
> To run the compliance validation locally, use the following command:
> ```bash
> lula validate -f lula/lula-component.yaml
> ```
