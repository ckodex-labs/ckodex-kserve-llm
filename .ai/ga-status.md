# GA Status — ckodex-kserve-llm Operator
<!-- Last updated: 2026-06-13 -->

## AIPACK-SPEC v0.1.1 — Full Integration Status

### ✅ Complete (GA)

| Area | Artifact | Status |
|------|----------|--------|
| CRD | `config/crd/serving.ckodex.com_aipacks.yaml` | Generated, 1093 lines |
| API types | `api/v1alpha2/aipack_types.go` + 6 sibling files | All 15 kinds, 6 families |
| Internal domain | `internal/aipack/` (kinds, predicates, composition, errors, etc.) | Complete |
| Governance | `internal/governance/conformance.go`, `evidence.go` | AIPack validators wired |
| Evidence reconciler | `internal/controller/evidence/aipack_reconciler.go` | `ReconcileAIPacks` |
| Webhook | `internal/webhook/aipack_webhook.go` | Validates kind, source.ref, composition |
| Webhook wiring | `internal/webhook/webhook.go:46` | `SetupAIPackWebhook` called |
| Controller | `internal/controller/aipack_controller.go` | Status reconciler |
| Manager wiring | `cmd/manager/main.go:387` | Unconditionally registered |
| LLM reconcile loop | `internal/controller/llminferenceservice_controller.go` | AIPack list + ReconcileAIPacks |
| RBAC | `config/rbac/role.yaml:148,174` | Dedicated rule, no create/delete |
| JSON schemas | `schema/` (24 files) | CycloneDX + SPDX compatible |
| Conformance tests | `test/conformance/aipack/` | 100 vectors, all pass (0.345s) |
| Sample manifests | `config/samples/aipack_basemodel_llama3.yaml` | Kind=BaseModel, workload label |
| Sample manifests | `config/samples/aipack_agent_example.yaml` | Kind=Agent, composition ref |
| DeepCopy | `api/v1alpha2/zz_generated.deepcopy.go` | AIPack + AIPackList |

### Test Results (sandbox, 2026-06-13)

```
ok  internal/controller/deployment
ok  internal/controller/status
ok  internal/webhook                    ← includes AIPack webhook tests
ok  test/conformance/aipack             ← 100 vectors pass
ok  test/conformance/vector-state
FAIL internal/controller               ← 1 pre-existing failure: TestRegisterWithTargetService_Success
                                          (IPv6 bind prohibited in sandbox, unrelated to AIPack)
```

All AIPack-specific tests pass:
- `TestAIPackReconciler_ValidKind_BaseModel` PASS
- `TestAIPackReconciler_AllKinds` PASS (15 subtests, one per ArtifactKind)
- `TestAIPackReconciler_UnknownKind` PASS
- AIPack webhook validation tests PASS
- 100 conformance vectors PASS

### Build

```
go build ./api/... ./internal/... ./cmd/...  → BUILD OK
go vet ./api/... ./internal/... ./cmd/...    → clean
```

### Pre-existing failures (sandbox network restriction, NOT our code)

| Test | Package | Cause |
|------|---------|-------|
| `TestRegisterWithTargetService_Success` | internal/controller | `listen tcp6 [::1]:0: bind: operation not permitted` |
| `TestVaultHealthCheck_Active_200` | internal/health | same |
| `TestInferenceFullPipeline` | internal/inference | same |
| `TestServerLive_True` | internal/protocol/v2 | same |
| `TestFetchFileChecksums_NonOKStatus_Error` | internal/storage | same |

All pre-date this integration work. CI environment (linux amd64) does not have this restriction.

### Deployment path

- **Primary:** Helm chart at `deploy/helm/` + ClusterRole from `config/rbac/role.yaml`
- **The Helm chart `rbac.yaml` template only creates bindings** (ClusterRoleBinding, RoleBinding)
- **ClusterRole `manager-role`** must be applied separately via kustomize or `kubectl apply -f config/rbac/role.yaml`
- No `kustomization.yaml` in `config/crd/` or `config/rbac/` — apply CRDs and RBAC directly

### Label binding

AIPack ↔ LLMInferenceService association is by label:
```yaml
metadata:
  labels:
    serving.ckodex.com/workload: <llminferenceservice-name>
```

### Open items (post-GA)

- [ ] Add `kustomization.yaml` to `config/crd/` and `config/rbac/` for cleaner kustomize deployment
- [ ] Helm chart inline ClusterRole template (currently separate from Helm flow)
- [ ] E2E test for AIPack → LLMInferenceService governance flow
- [ ] `TestRegisterWithTargetService_Success` — fix IPv6 listener to use `127.0.0.1:0` instead of `[::1]:0`
