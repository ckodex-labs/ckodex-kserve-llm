# Security Architecture: The Governed Trust Loop

The **ckodex-kserve-llm** operator provides components intended to support a governed deployment. Which identity, isolation, integrity, and evidence controls apply depends on enabled features and cluster configuration; this document does not assert that every installation enforces them.

## 1. Zero-Trust Foundations

The architecture is built on three pillars of security: **Identity**, **Isolation**, and **Integrity**.

### A. Workload Identity (SPIFFE/SPIRE)

When the optional SPIRE integration is configured, a registration can define a **SPIFFE ID** for an inference service based on its Kubernetes ServiceAccount and model name.

- **Identity Integration**: The `SPIREReconciler` can manage a node-local SPIRE Agent when enabled and configured.
- **Workload Identity**: A valid registration is configuration evidence only; it does not prove SVID issuance, workload authentication, or mTLS enforcement.
- **NIST Mapping**: **IA-9** (Service Identification and Authentication) — applies to non-person entities (workload SVIDs), not IA-2 which governs organizational user accounts.

### B. Traffic Isolation (Istio DPI)

When the relevant gateway integration is enabled and configured, the operator can generate Istio `ServiceEntry` and `VirtualService` resources for workloads that declare behavioral intent via the `ToolSurface`.

- **Egress Filtering**: Configured Istio policy may restrict declared tool traffic; resource presence alone does not prove effective filtering.
- **Network Policies**: Deployment-specific `NetworkPolicy` resources may restrict pod traffic. The OSCAL AC-4 check tests for policy presence only, not selection or enforcement.
- **NIST Mapping**: **AC-4** (Information Flow Enforcement) is a mapping for configured enforcement; the OSCAL resource check alone does not establish it.

### C. Resource Integrity (OPA Gatekeeper)

Where OPA Gatekeeper and the relevant policies are installed and configured, admission policies can constrain resources.

- **GPU Quotas**: Where corresponding policies are installed, quotas can constrain resource use.
- **Image Allowlist**: Where an allowlist policy is installed, it can constrain inference runtime images; this document does not establish signature verification.

## 2. The L|T|R Governance Model

Compliance is tracked via a composite state machine represented as three "State Planes":

| Plane | State | Trigger | NIST 800-53 Mapping |
| :--- | :--- | :--- | :--- |
| **Lifecycle (L)** | `active` | Successful deployment and continuous health checks pass. | **CA-7** (Continuous Monitoring) |
| **Trust (T)** | `asserted` → `verified` | `verified` requires a runtime record of successful signature, provenance, and SBOM attestation checks. | **SI-7** (Software & Information Integrity), **SR-4** (Provenance) |
| **Risk (R)** | `normal` | Real-time analysis of OIS behavioral signals. | **SI-4** (System Monitoring) |

## 3. Evidence-as-Code (OSCAL)

OSKAL validates the component definition against its embedded OSCAL schema;
schema validity is not evidence that mapped controls are effective. The
resource-check command evaluates six mapped controls using a JSON snapshot
or a live Kubernetes API and emits JSON results. Schema validity is not evidence
that a control is effective. The AC-4 check detects namespace policy presence;
other checks inspect status conditions or SPIRE registration data. OSKAL projects
saved check outcomes into OSCAL observations; the scope, plan reference and
evaluation time are supplied by the caller and are not independently verified.
The projected artifact records the input and OSKAL source digests, but not the
operator source revision. The caller-supplied timestamp is syntax-checked only,
with no freshness check. The live assessment timeout is 30 seconds; snapshot
input is limited to 32 MiB and projection input to 1 MiB.

See [CI current state](ci/current-state.md) for commands and limits.

## 4. Open Inference Signals (OIS) v0.1

Behavioral monitoring is standardized via **OIS v0.1**. The data plane emits structured telemetry that the operator uses to dynamically adjust the **Risk (R)** plane.

### Technical Proof: OIS Metric Signal

```json
{
  "ois.v0.1.signal": "InferenceMetric",
  "timestamp": "2026-04-11T02:45:30Z",
  "resource.name": "gemma-4-e2b",
  "metric.name": "p99_latency_ms",
  "metric.value": 42.0,
  "context.tenant": "finance-alpha"
}
```
