# Open Loops — ckodex-kserve-llm-operator

Structured tracker for incomplete work, pending decisions, and tracked deferrals.
Each entry carries an ID, status, priority, and owner note.

Status values: `open` | `in-progress` | `blocked` | `deferred` | `done`
Priority: `P0` (release-blocking) | `P1` (GA-quality) | `P2` (improvement) | `P3` (nice-to-have)

---

## CI/CD

### L-CI-001 — Migrate GHA workflows to `dagger call`
- **Status:** done
- **Priority:** P2
- **Context:** Both workflows migrated (2026-06-14):
  - `ci.yml`: replaced `go run ./ci/main.go` with `dagger develop` + `dagger call all --source=.` + `dagger call coverage --source=. export --path=coverage.out`.
  - `release.yml` `image-release` job: replaced `go run ./ci/main.go --skip-lint --skip-tests --image ... --push --sign` with `dagger call publish` + `cosign sign` (OIDC) + `dagger call sbom`.
  All `${{ }}` expressions routed through `env:` blocks per GHA injection-safety policy.
- **Reference:** ADR-008, `.github/workflows/ci.yml`, `.github/workflows/release.yml`

### L-CI-002 — Retire `ci/main.go` standalone path
- **Status:** deferred
- **Priority:** P3
- **Context:** After L-CI-001 lands, the `ci/main.go` standalone path becomes
  redundant. Keep it for one full release cycle to confirm no regressions, then
  remove `ci/main.go` + `ci/pkg/`.
- **Blockers:** Depends on L-CI-001 being done and stable.

### L-CI-003 — Add `dagger call lula` for OSCAL validation
- **Status:** done
- **Priority:** P2
- **Context:** `dagger/main.go:Lula` added (2026-06-13). Downloads binary,
  verifies checksum via sha256sum, runs `lula validate`, returns assessment file.
  Mirrors `ci/pkg/security.Lula()` exactly.
- **Reference:** `dagger/main.go:223`, `ci/pkg/security/security.go:33`

### L-CI-004 — Coverage gate in module `Test` function
- **Status:** done
- **Priority:** P1
- **Context:** `coverageGateScript()` added to `dagger/main.go` (2026-06-13).
  Enforces the same per-package thresholds as `ci/pkg/test/test.go`.
  Both files use the same constants (27% controller, 80% others).
- **Reference:** `dagger/main.go:85`, `ci/pkg/test/test.go:23`

---

## Operator

### L-OP-001 — Add `kustomization.yaml` to `config/crd/` and `config/rbac/`
- **Status:** done
- **Priority:** P2
- **Context:** `config/crd/kustomization.yaml` added (2026-06-13): lists all 16 CRDs.
  `config/rbac/kustomization.yaml` added (2026-06-13): lists role.yaml + tenant-role.yaml.
  Both verified: `kustomize build config/crd/` emits 16 CRDs;
  `kustomize build config/rbac/` emits ClusterRole + ClusterRoleBinding + RoleBinding.
- **Reference:** `config/crd/kustomization.yaml`, `config/rbac/kustomization.yaml`

### L-OP-002 — Inline ClusterRole in Helm chart
- **Status:** done
- **Priority:** P2
- **Context:** ClusterRole `ckodex-operator-cluster-role` added to
  `deploy/helm/templates/rbac.yaml` (2026-06-13). Rules mirror `config/rbac/role.yaml`
  exactly. `helm template` verified: ClusterRole + ClusterRoleBinding emitted,
  roleRef name matches ClusterRole name. Note: update Helm template whenever
  `make manifests` regenerates `config/rbac/role.yaml`.
- **Reference:** `deploy/helm/templates/rbac.yaml:6`

### L-OP-003 — AIPack → LLMInferenceService governance integration test
- **Status:** done
- **Priority:** P1
- **Context:** 7 fake-client integration tests added (2026-06-13) in
  `internal/controller/aipack_governance_integration_test.go`. Tests verify:
  binding label filter, fully-attested / missing-predicate / nil-attestation /
  mixed-state paths, and exclusion of unbound/wrong-label AIPacks.
  Also fixed: `ReconcileAIPacks` now always sets `Compliance-SR-2-AIPack`
  condition (previously returned early for empty pack list, leaving condition absent).
- **Reference:** `internal/controller/aipack_governance_integration_test.go`,
  `internal/controller/evidence/aipack_reconciler.go:28`

### L-OP-004 — Fix IPv6 listener in `TestRegisterWithTargetService_Success`
- **Status:** done
- **Priority:** P2
- **Context:** All 5 tests (`TestRegisterWithTargetService_Success`, `TestVaultHealthCheck_Active_200`,
  `TestInferenceFullPipeline`, `TestServerLive_True`, `TestFetchFileChecksums_NonOKStatus_Error`)
  were confirmed passing (2026-06-14) outside the Claude Code sandbox. The failures
  were sandbox-imposed socket restrictions, not code defects. `httptest.NewServer`
  already binds to IPv4 (`127.0.0.1`) — no code change required. CI (linux/amd64)
  has always passed cleanly.
- **Reference:** `internal/controller/llmloraadapter_controller_test.go:385`

---

## Supply Chain

### L-SC-001 — Generate `dagger develop` output in CI and commit `dagger/internal/`
- **Status:** done
- **Priority:** P1
- **Context:** `dagger develop` run successfully (2026-06-14) outside sandbox.
  `dagger/internal/dagger` is excluded by Dagger's generated `.gitignore` — this
  is the canonical Dagger pattern (regenerated per clone). `dagger/go.sum` is
  committed. Both workflows now run `dagger develop` before `dagger call` commands.
- **Reference:** `dagger/go.mod`, `dagger/.gitignore`, `docs/adr/008-dagger-module.md`

### L-SC-002 — Wire `dagger call sbom` into release workflow
- **Status:** done
- **Priority:** P1
- **Context:** `dagger call sbom` wired into `release.yml` `image-release` job (2026-06-14).
  Generates CycloneDX SBOM via Trivy for the published image ref. Output uploaded
  as `sbom/sbom.cdx.json` artifact (90-day retention). Runs after `dagger call publish`
  and `cosign sign`. Depends on L-CI-001 (now done).

---

## Documentation

### L-DOC-001 — Fix L|T|R NIST control table (SECURITY_ARCHITECTURE.md)
- **Status:** done
- **Priority:** P1
- **Context:** Replaced CP-2→CA-7, IA-2→IA-9, Trust(T)→SI-7+SR-4. AC-4 anchored to Isolation pillar. (2026-06-15)
- **Reference:** `docs/SECURITY_ARCHITECTURE.md`

### L-DOC-002 — Expand COMPLIANCE.md with IA-9, SR-4, CA-7 rows
- **Status:** done
- **Priority:** P1
- **Context:** Added 3 rows; fixed NIST 800-53r5 canonical control names (SI-7 firmware, SR-2 Plan, SR-4 Provenance); added implementation-status annotation for beta. (2026-06-15)
- **Reference:** `COMPLIANCE.md`

### L-DOC-003 — Add Lula validator for IA-9 (SPIFFE/SPIRE SVID issuance)
- **Status:** open
- **Priority:** P2
- **Context:** SPIFFE/SPIRE identity issuance is implemented in SPIREReconciler but has no Lula validation YAML yet. Deferred post-beta.
- **Reference:** `lula/` directory, `internal/security/spire_reconciler.go`

### L-DOC-004 — Rewire Mermaid architecture diagram
- **Status:** done
- **Priority:** P1
- **Context:** Added WH→CM, SS-.->V1/V2, CM→PROM, GR→V1/V2, LWS---V1/V2, CON→PROM. Removed subgraph-targeting RT→DP and GR→DP edges. Added neo/elk theme. (2026-06-15)
- **Reference:** `README.md`

### L-DOC-005 — Flesh out OSCAL SI-7 observation with subjects
- **Status:** done
- **Priority:** P1
- **Context:** Added statements, by-components, set-parameters to SI-7 requirement in lula-component.yaml. Fixed relevant-evidence nesting inside subjects entry per OSCAL schema. (2026-06-15)
- **Reference:** `lula/lula-component.yaml`
