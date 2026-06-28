# API Deprecation Policy

**Project:** CKodex KServe LLM Operator
**Group:** `serving.ckodex.com`
**Effective:** 2026-03-24

---

## Guiding Principle

API versions are deprecated only after a stable successor is GA and operators have had
sufficient time to migrate. Deprecation is signalled in advance, enforced by a timeline,
and never silent.

---

## Current API Version Status

| API Version | Kind                    | Status            | Storage Version | Notes                          |
|-------------|-------------------------|-------------------|-----------------|-------------------------------|
| `v1`        | `LLMInferenceService`   | Stable            | Yes             | Use when the stable schema covers the workload |
| `v1`        | `LLMLoraAdapter`        | Stable            | Yes             | Use for new adapter resources |
| `v1`        | Agent/session family    | Stable schema     | Yes             | Product feature gates still apply |
| `v1alpha2`  | Core resources above    | Deprecated        | No              | Served during migration window |
| `v1alpha2`  | Specialized CRDs        | Current alpha API | Varies          | No v1 schema exists for several specialized kinds |

---

## Deprecation Timeline

```
v1 GA   ─────────────────────────────────────────────────────────
          │
          ├─ v1alpha2 deprecated (operator starts emitting warning at startup)
          │
          ├─ Release N+1 ── v1alpha2 still served via conversion webhook
          │                  Migration guide published
          │
          ├─ Release N+2 ── v1alpha2 removal window opens
          │                  CRD conversion webhook may be removed
          │                  Resources that have not been migrated become inaccessible
          │
          └─ Release N+3 ── v1alpha2 CRDs removed from chart
```

**Minimum support window:** v1alpha2 is guaranteed to be served for **2 full release cycles**
after v1 reaches General Availability.

---

## What Changes Between v1alpha2 and v1

| Field (v1alpha2)           | Field (v1)                            | Notes                              |
|----------------------------|---------------------------------------|------------------------------------|
| `spec.prefill`             | `spec.experimental.prefill`           | Moved under experimental sub-struct |
| `spec.worker`              | `spec.experimental.worker`            | Moved under experimental sub-struct |
| All other `spec.*` fields  | Identical path                        | 1:1 mapping, no data loss          |
| `status.*`                 | Identical                             | 1:1 mapping                        |

Conversion is handled automatically by the conversion webhook. Existing v1alpha2 resources
stored in etcd are transparently converted to v1 on read and written back as v1 on update.

---

## Migration Steps for Operators

### 1. Update manifests

Replace the API version in all YAML files:

```yaml
# Before
apiVersion: serving.ckodex.com/v1alpha2
kind: LLMInferenceService

# After
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
```

### 2. Move experimental fields (if used)

```yaml
# Before (v1alpha2)
spec:
  prefill:
    replicas: 2
  worker:
    template: ...

# After (v1)
spec:
  experimental:
    prefill:
      replicas: 2
    worker:
      template: ...
```

### 3. Trigger in-place migration

Apply updated manifests with `kubectl apply`. The API server writes the resource as v1.

```bash
# Verify all resources are v1 after migration
kubectl get llminferenceservices -A \
  -o jsonpath='{range .items[*]}{.apiVersion}{"\t"}{.metadata.namespace}{"\t"}{.metadata.name}{"\n"}{end}'
```

All `LLMInferenceService` entries should show `serving.ckodex.com/v1`.

### 4. Update Helm values (if chart-managed)

No Helm values changes are required — the chart generates v1 manifests automatically
after upgrading to the operator version that introduced v1.

---

## Deprecation Warning

The operator emits a startup warning when v1alpha2 resources are detected:

```
DEPRECATION WARNING: v1alpha2 LLMInferenceService resources detected
count=N action="Migrate to serving.ckodex.com/v1 before the v1alpha2 removal window"
```

This is **informational only** — the operator continues to reconcile v1alpha2 resources
normally during the supported window.

---

## Policy for Future API Versions

1. **Alpha → Beta:** Promoted when the API is considered functional and test coverage ≥ 80%.
   May contain breaking changes between minor versions.

2. **Beta → GA (v1):** Promoted when no breaking changes have been needed for one release
   cycle and conformance tests pass. Backwards-compatible for the full support window.

3. **GA → Deprecated:** Requires a GA successor. Minimum 2 release cycle support window
   before removal.

4. **Exceptions:** Security fixes may introduce breaking changes without the standard notice
   period. Such changes are announced via the security advisory channel with a 30-day
   migration window.

---

## References

- [Kubernetes API deprecation policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/)
- `api/v1/conversion.go` — Hub marker for v1 (storage version)
- `api/v1alpha2/conversion.go` — ConvertTo / ConvertFrom spoke implementation
- `internal/webhook/conversion.go` — Compile-time interface assertions
