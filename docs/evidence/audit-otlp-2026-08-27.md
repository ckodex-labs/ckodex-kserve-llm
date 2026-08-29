# Audit OTLP evidence — 2026-08-27

## Scope and verdict

This record covers the bounded OTLP/HTTP audit-log exporter added to the
operator. It closes the previous “configured endpoint only reports
unavailable” implementation gap, but it does not claim a complete signed
evidence fabric.

**Verdict:** the optional OTLP/HTTP path is implemented and locally verified.
The existing structured-log, JSONL, Kubernetes Event, and active-span sinks
remain in place. Integrity chaining, producer signatures, content-free
receipt types, and runtime evidence-health wiring remain open.

## Configuration contract

- Configure `audit.otlpEndpoint` in `deploy/helm/values.yaml`, or set
  `CKODEX_AUDIT_OTLP_ENDPOINT` / `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT`.
- An absolute `http://` or `https://` URL is required. A missing path is
  normalized to `/v1/logs`.
- HTTP is explicit plaintext transport for local/dev collectors; HTTPS is the
  expected protected deployment transport.
- Records use a bounded batch processor: 500 ms export interval, 2,048-record
  queue, 128-record batch, and 5-second export timeout.
- Manager shutdown calls the provider shutdown path after initialization errors
  are rejected before controller startup.

## Implementation evidence

| Requirement | Evidence |
|---|---|
| Real OTLP exporter | `internal/observability/audit_otlp.go` uses the official OTLP/HTTP exporter and SDK batch processor |
| Audit records exported | `internal/observability/audit.go` emits each redacted audit event to the OTLP log logger |
| Endpoint validation | `normalizeAuditOTLPEndpoint` rejects missing/unsupported schemes and adds `/v1/logs` |
| Export failures visible | exporter wrapper logs batch export errors and `Flush` returns the failure to its caller |
| Lifecycle ownership | `AuditLogger.Flush` and `AuditLogger.Shutdown` expose bounded lifecycle operations; manager startup uses the checked constructor |
| Chart wiring | both Helm value trees conditionally inject `CKODEX_AUDIT_OTLP_ENDPOINT` |

## Verification

- `go test ./internal/observability ./internal/config` passed with a real
  `httptest` OTLP receiver and a failing `503` receiver.
- `go test -race ./...` passed after the exporter and configuration changes.
- `go vet -a ./...` passed.
- `helm lint deploy/helm` and `helm lint
  charts/ckodex-kserve-llm-operator` passed.
- `git diff --check` passed.

## Remaining evidence boundary

The following are intentionally not marked complete by this record:

- cryptographic hash chaining, signatures, or Merkle commitments on audit
  records;
- schema-level prevention of raw content in receipt types;
- unified metric/event namespace migration;
- runtime evidence-health tracker integration and broken-causal-edge checks;
- hosted collector delivery, retention, alerting, and incident-replay proof.
