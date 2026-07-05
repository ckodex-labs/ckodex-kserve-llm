# ADR 008: Dagger Module for Typed CI/CD Functions

## Status

Accepted

## Context

The repository originally implemented Dagger CI as a standalone Go program
that opened its own engine connection. That path was not discoverable through
`dagger call`, duplicated tool pins, and allowed local and hosted policy logic
to drift.

CI/CD requires:

- `dagger call <function>` as the local and hosted verification interface
- `dagger.json` at the repository root
- typed functions for lint, test, coverage, build, scan, SBOM, publish, and the
  hosted fast gate
- cache volumes and two-phase source mounting for dependency and build reuse
- bounded parallel execution for independent checks

## Decision

The Dagger Module in `dagger/` is the sole CI implementation. GitHub Actions
and local development invoke the same typed functions from `dagger/main.go`.
The former standalone `ci/` program was retired after the module-backed hosted
pipeline completed its stabilization period.

The module keeps its own `go.mod` because Dagger generates
`dagger/internal/dagger/` as module-scoped SDK code. Keeping the module source
under `dagger/` prevents generated types from colliding with the operator's Go
module.

## Consequences

**Positive:**

- One implementation owns tool versions, cache policy, parallelism, and gates.
- Function signatures are typed and discoverable through `dagger call --help`.
- Local runs execute the same graph as GitHub Actions.
- Dagger observability and cache reuse apply to every hosted stage.

**Operational constraint:**

- Run `dagger develop` once after cloning or changing the Dagger SDK version.
- The generated `dagger/internal/dagger/` tree remains ignored and reproducible.

## Commands

```bash
dagger develop
dagger call lint --source=.
dagger call test --source=.
dagger call build --source=. --version=dev
dagger call scan --source=.
dagger call lula --source=. export --path=assessment-results.yaml
dagger call all --source=.
```

## References

- [Dagger Go Module SDK](https://docs.dagger.io/api/sdk/go)
- [Dagger Module Reference](https://docs.dagger.io/reference/dagger-module)
- `dagger/main.go`
- `dagger.json`
