# CKodex KServe LLM Operator Console

The console is the observe-only operator surface for CKodex-managed KServe model serving. It distinguishes declared resources from reported readiness and qualifies every displayed source as observed, empty, partial, or unavailable.

## Implemented surfaces

- Reconciliation overview with qualified inventory, namespace-presence checks, attention items, and recent audit records.
- Cluster-scoped `serving.ckodex.com/v1` `LLMInferenceService` inventory with namespace, readiness, replica envelope, generation freshness, model revision, endpoint, and reported conditions.
- Namespaced workload records connecting declared state to managed Deployments, Pods, and retention-limited Kubernetes Events.
- Workload investigation paths carrying bounded search context into the audit and telemetry ledgers.
- Kubernetes node readiness and reported capacity from `core/v1`.
- Accelerator capacity derived only from supported `status.allocatable` resource keys. Utilization and device memory are withheld because this source does not expose them.
- Audit-event ledger backed by the operator's JSONL `AuditEvent` contract.
- Prometheus telemetry over a fixed allowlist of CKODEX recording rules and native vLLM gauges, with one-hour trends and active-alert triage.
- SPIRE registration inventory backed by operator-managed ConfigMaps, explicitly separated from SVID issuance and validity.
- Kubernetes execution-principal and effective-access observations backed by `SelfSubjectReview` and a fixed set of `SelfSubjectAccessReview` requests.
- Advisory AI surface backed by an explicitly configured gateway, secure transport by default, bounded requests, lifecycle cancellation, and an operator stop control. It has no cluster tools or mutation authority.
- Non-executing command catalog for copying reviewed read-only commands to a trusted shell.
- Governed-action readiness ledger exposing the missing human identity, mutation policy, confirmation, and durable receipt prerequisites instead of rendering inactive action controls.
- Ledger, Vault, high-contrast, and forced-colors presentation under CKODEX-DS-3.
- Evidence-aware loading, not-found, route-error, and root-shell recovery surfaces that preserve navigation and withhold unqualified source claims.

## Explicit boundaries

- The console does not create, scale, promote, roll back, or delete Kubernetes resources.
- SPIRE ConfigMaps prove registration intent only. Issued SVID inventory, expiry, revocation, workload attestation, and mTLS establishment remain unavailable.
- Effective-access results describe the server-side console principal. They do not describe an interactive human identity or grant mutation authority.
- Telemetry renders an unavailable source until a valid Prometheus endpoint is configured. Storage remains disabled until its data adapter exists.
- Namespace presence is not component health.
- Audit records are observations. They are not attestations unless a verifiable proof object and evidence envelope resolve.
- No public console image ships with the operator chart by default.

## Data sources

1. Kubernetes API through the active server-side kubeconfig.
2. `/var/log/ckodex/audit.jsonl`, or `CKODEX_AUDIT_LOG_PATH` when configured.
3. Optional AI gateway configured through environment variables.
4. Optional Prometheus HTTP API v1 endpoint for allowlisted CKODEX recording rules, native vLLM gauges, and active alerts.
5. Operator-managed SPIRE registration ConfigMaps in the configured registration namespace.
6. Kubernetes authentication and authorization self-review APIs for the console execution principal.

The audit reader accepts the runtime schema emitted by `internal/observability.AuditEvent`:

```json
{
  "action": "PolicyViolation",
  "resource": "LLMInferenceService/default/model-a",
  "actor": "opa-gatekeeper",
  "outcome": "Denied",
  "timestamp": "2026-08-02T14:31:05Z",
  "details": { "policy": "approved-model-source" },
  "reason": "Model source did not satisfy policy.",
  "exec.id": "urn:ckodex:exec:01",
  "exec.kind": "governance",
  "exec.reproducibility_class": "explanatory"
}
```

Malformed records are excluded from rendering and qualify the source as partial.

### Tensor Prime reference routing

Tensor Prime keeps its inference and platform-service data planes distinct:

- `http://api.tprime.vlans.ca/v1` reaches the dedicated Envoy AI Gateway. Its `AIGatewayRoute` parses the OpenAI `model` field and dispatches to the selected vLLM backend.
- Grafana, Prometheus, the LiteLLM administrative route, Alertmanager, Hubble, and storage services attach to the separate Envoy Platform Gateway.

The older platform-gateway route from `api.tprime.vlans.ca` to LiteLLM is superseded by the dedicated AI Gateway cutover and must not be used as the live topology contract.

## Configuration

| Variable | Purpose |
| --- | --- |
| `NEXT_PUBLIC_LATTICE_NAME` | Operator environment label shown in the shell. |
| `NEXT_PUBLIC_ENVIRONMENT` | `production`, `staging`, `development`, or `sandbox`. |
| `NEXT_PUBLIC_FEATURE_TERMINAL` | Set to `false` to hide the command catalog. |
| `CKODEX_AUDIT_LOG_PATH` | Server-side audit JSONL path. |
| `CKODEX_SPIRE_REGISTRATION_NAMESPACE` | Namespace containing labeled SPIRE registration ConfigMaps. Defaults to `spire`. |
| `CKODEX_KUBERNETES_REQUEST_TIMEOUT_MS` | Per-request Kubernetes API deadline in milliseconds. Defaults to `5000`; values are bounded to `250`–`30000`. |
| `CKODEX_PROMETHEUS_URL` | Server-side Prometheus base URL. HTTPS is required by default. |
| `CKODEX_PROMETHEUS_ALLOW_INSECURE` | Set to `true` only for a trusted internal HTTP endpoint. |
| `CKODEX_PROMETHEUS_BEARER_TOKEN` | Optional server-side Prometheus bearer token. |
| `CKODEX_GRAFANA_DASHBOARD_URL` | Optional authenticated Grafana deep link shown beside telemetry qualification. |
| `CKODEX_AI_GATEWAY_URL` | OpenAI-compatible gateway base URL, including `/v1` when required by the provider. HTTPS is required by default. |
| `CKODEX_AI_GATEWAY_ALLOW_INSECURE` | Explicitly allow a trusted internal HTTP gateway; required for the current Tensor Prime Envoy AI Gateway. |
| `CKODEX_AI_GATEWAY_API_KEY` | Server-side gateway credential. |
| `CKODEX_AI_MODEL` | Gateway model identifier. |
| `CKODEX_AI_GATEWAY_TIMEOUT_MS` | Total assistant gateway deadline in milliseconds. Defaults to `28000`; values are bounded to `1000`–`28000`. |

## Development and verification

```bash
npm ci --ignore-scripts
npm run test
npm run lint
npx tsc --noEmit
npm run build
npm run verify:ssr
npm run verify:populated
npm run dev
```

The production build uses webpack explicitly because the current project has not adopted the Turbopack production path.
The SSR verifier starts the built application on loopback, checks all operator routes, asserts semantic landmarks and skip-link order, verifies security headers, and stops the process. It does not replace rendered visual or screen-reader acceptance.
The populated-state verifier starts deterministic Kubernetes, SPIRE-registration, audit, and Prometheus fixtures at their real transport boundaries. It verifies that every operator route renders qualified source data through the production build and then stops both local processes. Fixtures remain test-only and never replace production adapters.

`GET /api/health` and `HEAD /api/health` report console-process liveness only. They do not assert Kubernetes, SPIRE, Prometheus, audit-log, or AI-gateway health.

When the Helm console is enabled, the chart creates a dedicated ServiceAccount by default. Its ClusterRole is read-only for console inventory resources; a namespace-scoped Role reads SPIRE registration ConfigMaps. The chart also grants access to the Kubernetes self-review APIs. Mutation verbs are intentionally absent.
