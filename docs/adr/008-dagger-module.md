# ADR 008: Dagger Module for Typed CI/CD Functions

## Status

Accepted

## Context

The repository had Dagger-powered CI as a Go program (`ci/main.go`) that called
`dagger.Connect()` and orchestrated stages through `ci/pkg/` packages. This is
Dagger's "standalone SDK" pattern: the binary is invoked directly
(`go run ./ci/main.go`) and manages its own Dagger engine connection.

The standalone pattern works but has two gaps:

1. **No `dagger call`**: Functions are not discoverable or invocable by name.
   There is no `dagger call lint`, `dagger call test`, etc.
2. **No `dagger.json`**: Without a module manifest, the Dagger CLI cannot inspect
   the pipeline, introspect function signatures, or integrate with Dagger Cloud
   tooling.

CI/CD policy (CLAUDE.md §3, Dagger CI/CD requirements) requires:

- `dagger call <function>` as the verification interface for each pipeline stage
- `dagger.json` present at the repo root
- Baseline typed functions: `lint`, `test`, `coverage`, `build`, `scan`, `sbom`,
  `publish`, `all`

## Decision

Add a Dagger Module (`dagger.json` + `dagger/main.go`) alongside the existing
`ci/` package. The two coexist:

| Path | Pattern | Invoked by |
|------|---------|------------|
| `ci/main.go` | Standalone SDK (dagger.Connect) | `go run ./ci/main.go` in GHA |
| `dagger/main.go` | Module SDK (dag global) | `dagger call <func>` |

**Why not replace `ci/main.go`?**

The `ci/` standalone path is the current GHA entrypoint. Migrating GHA to `dagger
call` is a follow-on task tracked in `docs/open-loops.md`. The migration requires
testing `dagger call` in the GHA runner environment (especially caching and
secrets injection), which cannot be validated locally in the same step.

**Why a separate `go.mod` for the module?**

Dagger Modules must declare their own `go.mod` because the Dagger engine code-generates
`internal/dagger/` (the Module SDK types and `dag` global) into the module source
directory. This generated code is module-scoped and would conflict with the main
project's module if co-located.

**Why `source: "dagger"` not `source: "."`?**

Placing module source in a `dagger/` subdirectory keeps the project root
unambiguous — a visitor sees `dagger.json` + `dagger/` and immediately understands
the module boundary without conflating the module's `go.mod` with the main
project's `go.mod`.

## Consequences

**Positive:**

- `dagger call lint`, `dagger call test`, `dagger call build`, `dagger call scan`,
  `dagger call sbom`, `dagger call publish`, `dagger call all` all work once
  `dagger develop` is run to generate `dagger/internal/dagger/`.
- Function signatures are typed and introspectable (`dagger call --help`).
- Dagger Cloud caching and observability apply to module function calls.
- CI/CD policy verification gates now have stable named functions.

**Negative / mitigations:**

- Logic duplication between `ci/pkg/` and `dagger/main.go`. Mitigation: pin
  the same tool versions via constants in both; comment the mirror. Long-term,
  migrate `ci/main.go` callers to `dagger call` and retire `ci/`.
- `dagger develop` must be run once on first checkout (generates `internal/`).
  GHA runner handles this automatically on the first `dagger call` invocation.

## First-time Setup

```bash
dagger develop          # generates dagger/internal/dagger/ (one-time per clone)
dagger call lint --source=.
dagger call test --source=.
dagger call build --source=. --version=dev
dagger call scan --source=.
```

## Migration Path (open loop)

See `docs/open-loops.md#L-CI-001` — GHA workflow migration from
`go run ./ci/main.go` to `dagger call all --source=.`.

## References

- [Dagger Go Module SDK](https://docs.dagger.io/api/sdk/go)
- [Dagger Module Reference](https://docs.dagger.io/reference/dagger-module)
- `ci/main.go` — standalone pipeline (backward-compatible entrypoint)
- `dagger/main.go` — module functions
- `docs/open-loops.md` — L-CI-001 (GHA migration)
