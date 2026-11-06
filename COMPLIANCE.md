# Security Compliance & OSCAL Mapping

This document maps selected operator features and status signals to the **NIST 800-53 Rev 5** catalog. The mappings are not a control-effectiveness determination. The project uses **OSCAL (Open Security Controls Assessment Language)** for machine-readable observations and schema validation via **OSKAL**.

## 1. Control Mapping (NIST 800-53)

| Control ID | Control Name | Operator Feature | Condition / Evidence |
| :--- | :--- | :--- | :--- |
| **AC-4** | Information Flow Enforcement | NetworkPolicy / ToolSurface-related features (deployment and configuration dependent) | `hack/oscal-assessment/` checks namespace policy presence only |
| **AU-2** | Audit Events | Audit-related status signal, where implemented and enabled | `Compliance-AU-2` status condition · `hack/oscal-assessment/` |
| **CA-7** | Continuous Monitoring | Lifecycle (L) plane tracks active/healthy state via Deployment readiness | Lifecycle state = `active` |
| **IA-9** | Service Identification and Authentication | SPIRE registration support is optional and deployment dependent; not proof of issued SVIDs or authenticated workloads | `hack/oscal-assessment/` checks registration configuration |
| **SI-4** | System Monitoring | OIS-related status signal, where implemented and enabled | `Compliance-SI-4` status condition · `hack/oscal-assessment/` |
| **SI-7** | Software, Firmware, and Information Integrity | Integrity-related status signal, where implemented and enabled | `Compliance-SI-7` status condition · `hack/oscal-assessment/` |
| **SR-2** | Supply-Chain Risk Management Plan | Provenance-related status signal, where implemented and enabled | `Compliance-SR-2` status condition · `hack/oscal-assessment/` |
| **SR-4** | Provenance | Release workflow generates SLSA provenance + cosign attestation per artifact | Release OIDC-backed provenance artifacts; tag-driven GHA path |

## 2. Governed States (L|T|R)

The operator's internal state machine supports these controls, but not every signal should be interpreted as cryptographic proof:

- **Lifecycle (L)**: The `active` lifecycle state may be relevant to **CA-7** (continuous monitoring); it is not itself proof of continuous monitoring or a contingency plan (CP-2).
- **Trust (T)**: Trust-related signals may map to **SI-7** (software integrity) and **SR-4** (provenance). The resource checks do not independently verify signatures, provenance, or SBOM attestations. **AC-4** (information flow enforcement) belongs to the isolation mapping, not the Trust plane.
- **Risk (R)**: Risk-related signals may map to **SI-4**. These OSCAL checks do not establish OIS anomaly detection or automatic quarantine behavior.

## 3. Automated Assessment

OSKAL validates `oscal/component-definition.yaml` using a caller-supplied source
checkout. `hack/oscal-assessment` evaluates six resource checks against a JSON
snapshot or a live cluster and emits JSON. The live path requires Kubernetes
credentials, is bounded by a 30-second request timeout, and reads a snapshot of
at most 32 MiB. `dagger call oscal-checks` runs evaluator tests; it does not
assess a cluster or produce assessment results. `oscal-project` converts saved
check results into schema-validated OSCAL observations using the OSKAL SDK; its
caller supplies the scope, plan and evaluation time, which are not independently
verified by the adapter. Projection accepts check input up to 1 MiB. The
supplied RFC3339 evaluation time is not checked for freshness or compared with
the time checks actually ran.

AC-4 checks only that a NetworkPolicy exists in each service namespace; this
does not establish enforcement. AU-2, SI-4, SI-7 and SR-2 read status conditions;
these are not independent cryptographic verification. IA-9 checks registration
configuration, not SVID issuance or workload authentication.

For a full assessment or an IA-9-only check, the caller must specify the
identity mode. Use `--identity-mode direct` for a direct deployment, or
`--identity-mode vcluster --host-namespace <physical-namespace>` for a
vcluster deployment. This context is asserted by the caller and is not
independently verified by the check.

```bash
dagger call oscal-validate --source=. --oskal-source=/path/to/ckx-oskal-001
dagger call oscal-checks --source=. # evaluator conformance tests only
go run ./hack/oscal-assessment --live --identity-mode direct
```

The projected artifact includes the check-input digest and OSKAL checkout
content digest. It does not identify or bind the operator source revision.
