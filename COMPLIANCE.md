# Security compliance and OSCAL assessment

This specification details the compliance architecture for the CKodex KServe LLM Operator. It maps operator controls and runtime evidence to the **NIST SP 800-53 Rev 5** catalog and defines automated verification procedures using **OSCAL (Open Security Controls Assessment Language)** and **Lula**.

## NIST SP 800-53 Rev 5 control mapping

| Control ID | Control name | Implementation mechanism | Evidence and verification artifact |
|---|---|---|---|
| **AC-4** | Information flow enforcement | Envoy Gateway 1.2.0 route fencing (`internal/gateway/fence.go`), Istio authorization policies, and Kubernetes NetworkPolicies | Route fencing within 5 seconds; NetworkPolicy presence verified via `hack/oscal-assessment` |
| **AU-2** | Event logging | RFC 9162 Merkle flight recorder logging all admission, mutation, and tool execution receipts | Cryptographically signed Ed25519 execution receipts with SHA-256 state digests |
| **CA-7** | Continuous monitoring | 9-state lifecycle engine, Open Inference Signals (OIS v0.1), and Prometheus scrapers on port 8080/8000 | `Compliance-CA-7` condition; continuous readiness and liveness probe telemetry |
| **IA-9** | Service identification and authentication | SPIRE Agent/Server integration issuing X.509 SVIDs per workload; automatic 60-minute rotation | SPIFFE ID verification; mTLS handshakes between ingress gateway and model workers |
| **SC-7** | Boundary protection | Gateway API HTTPRoute isolation, non-root execution (UID 65532), and dropped container capabilities (`ALL`) | Pod security admission standards; route configuration assertions |
| **SC-8** | Transmission confidentiality and integrity | Mandatory TLS 1.3 at ingress gateway and mutual TLS across all pod-to-pod network traffic | Cryptographic cipher suite negotiation logs; certificate validity audits |
| **SI-4** | Information system monitoring | Real-time token consumption anomaly detection and token-aware priority queue backpressure | `Compliance-SI-4` condition; telemetry trace streams |
| **SI-7** | Software, firmware, and information integrity | Read-only container root filesystems, distroless images, safetensors serialization enforcement | `Compliance-SI-7` condition; offline Cosign image and artifact signature verification |
| **SR-2** | Supply chain risk management plan | Automated Trivy vulnerability scanning, CycloneDX and SPDX SBOM generation | Zero `CRITICAL` or `HIGH` vulnerabilities; committed SBOM release artifacts |
| **SR-4** | Provenance | SLSA provenance Level 3 generation via GitHub Actions OIDC identity federation | Signed provenance attestations verified with Sigstore Cosign |

## OSCAL component definitions

The operator provides a machine-readable OSCAL component definition in `oscal/component-definition.yaml`. The component definition declares how the operator satisfies each mapped NIST control and provides testable validation rules.

### Structure of the OSCAL component definition

```yaml
component-definition:
  uuid: 8f6e4255-5ad6-482b-84c0-38c1ca60a355
  metadata:
    title: CKodex KServe LLM Operator Component Definition
    last-modified: "2026-11-06T00:00:00Z"
    version: "v0.20.0"
    oscal-version: "1.1.2"
  components:
    - uuid: 11111111-2222-3333-4444-555555555555
      type: software
      title: ckodex-kserve-llm-operator
      description: Governed LLM inference control plane on Kubernetes
      control-implementations:
        - uuid: aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
          source: https://raw.githubusercontent.com/usnistgov/oscal-content/master/nist.gov/SP800-53/rev5/yaml/NIST_SP-800-53_rev5_catalog.yaml
          description: NIST SP 800-53 Rev 5 control implementations
          implemented-requirements:
            - uuid: 00000000-0000-0000-0000-000000000001
              control-id: ac-4
              description: Ingress route fencing and network policy enforcement
```

## Automated assessment with Lula

[Lula](https://github.com/defenseunicorns/lula) evaluates cluster resources directly against the OSCAL component definition to determine real-world compliance.

### Running compliance assessments

Execute Lula validation through the Dagger CI pipeline:

```bash
dagger call lula --source=. export --path=assessment-results.yaml
```

The pipeline inspects live or staged Kubernetes resources, evaluates control assertions, and exports a standardized OSCAL `assessment-results` document.

### Running local assessment checks

Run the local assessment tool to check control statuses directly against a cluster:

```bash
go run ./hack/oscal-assessment --live --identity-mode direct
```

The tool checks:
1. **AC-4**: Validates that every inference service namespace enforces a restrictive `NetworkPolicy`.
2. **IA-9**: Verifies SPIRE registration entries for deployed workloads.
3. **SI-7**: Confirms that running pod images match declared immutable digests and possess valid signatures.
4. **SR-2**: Validates provenance conditions on active `LLMInferenceService` status objects.

## Vector state compliance posture

Compliance is not treated as a static pass or fail boolean. The operator models compliance as a typed vector:

$$S(e,t) = \langle P, V, C, E, L, \tau \rangle$$

If an admitted workload deviates from its compliance baseline (for example, if a node policy fails or an unverified container starts), coherence transitions to `DECOHERENT`. The controller immediately:
1. Applies route fencing to the ingress gateway, dropping external traffic within 5 seconds.
2. Emits an adverse decision trace event.
3. Transitions workload lifecycle to `SAFE_HOLD` or `QUARANTINED`.
4. Writes an immutable audit entry to the flight recorder for post-incident review.
