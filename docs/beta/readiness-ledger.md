# Beta readiness ledger

This ledger is intentionally evidence-first. A green unit test does not close a
live-cluster, hosted-release, provenance, browser, or human-identity gate.

## Current gate state

| Gate | State | Required evidence | Current action |
| --- | --- | --- | --- |
| Source reproducibility | local-ready | fresh clone contains console | Verify from the committed branch and hosted checkout |
| Console CI gate | local-ready | CI runs all console checks | Verify hosted CI cannot skip the gate |
| Dagger CI gate | local-lint-host-inconclusive-hosted-pending | Dagger lint, build, race tests, and coverage gates on the exact release head | Re-run the full Dagger module on a clean/hosted runner; this host was CPU-saturated by unrelated clusters and the isolated test did not produce a result |
| Published chart | local-ready | chart renders console profile | Verify the next published OCI chart |
| Console image | local-build-scan-hosted-pending | signed multi-architecture image plus HIGH/CRITICAL scan | Verify the next hosted image release |
| Runtime profile | partial | CPU and required GPU acceptance | Define matrix; execute live profile gates |
| Nightly KIND acceptance | hosted-failing-local-fixed | Workload lifecycle reaches readiness and inference without harness lookup races | Re-run `run/e2e.sh` from the fixed head; latest old-head failure was [run 31771937532](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/31771937532) |
| Inference probe qualification | local-fixed-hosted-pending | Gateway hostname binding, strict HTTP/JSON response validation, retry, and port-forward cleanup | Confirm the hosted rerun reaches the inference assertion rather than failing in transport setup |
| Local image build reproducibility | local-fixed-rerun-pending | Bounded Docker context and BuildKit image targets load into KIND | Re-run `run/e2e.sh` after the `.dockerignore` and `buildx --load` changes |
| Stable v1 conversion path | local-conversion-verified-runtime-pending | beta CRD bundle, lossless conversion round-trip, live v1 create/update/read | Install the fixed beta profile and prove storage plus webhook behavior in a cluster |
| Stable v1 admission | local-fixed-hosted-pending | v1 validator/defaulting registration, webhook routes, and live v1 admission | Re-run the exact-head hosted profile and retain API server/webhook evidence |
| Scheduler dependency and RBAC | local-runtime-verified-hosted-pending | Gateway API Inference Extension CRDs, EPP workload identity, and operator permissions | Re-run from a fresh hosted cluster and capture InferencePool/EPP reconciliation |
| Default CPU storage path | local-default-switched-rerun-pending | default `hf://` initializer path without optional privileged CSI/FUSE dependency | Re-run the default CPU profile; accept `hf-mount://` only under its explicit profile |
| Release image/chart alignment | local-fixed-hosted-pending | chart defaults resolve operator, console, and initializer images from release `appVersion` | Verify the packaged OCI chart and public release contract for the next RC |
| KIND environment reproducibility | local-fixed-host-blocked-hosted-pending | disposable cluster uses an explicitly qualified node image; this host currently fails node boot with Docker `Too many open files` | Re-run the default fresh cluster on a host with sufficient Docker file descriptors and align hosted KIND image/version |
| Human access | unresolved | authenticated boundary tests | Select authenticated-ingress or OIDC posture |
| Readiness explanation | partial | causal investigation acceptance | Add correlated dependency context |
| Promotion semantics | excluded | explicit route mutation proof | Keep readiness-only language for beta |
| Evidence verification | partial | signed verification result | Complete cosign path or downgrade claims |
| Browser accessibility | local-browser-proven-hosted-and-assistive-tech-pending | packaged image browser pass: CSS, heading order, target sizes, mobile overflow, command palette focus/escape, theme switching, no console warnings | Repeat in hosted CI and complete assistive-technology/reduced-motion evidence |
| Console dependency security | local-ready | zero production advisories from npm audit | Re-run in hosted CI and retain development-only advisory context |
| Chart source alignment | partial | one authoritative chart publish path | Consolidate or explicitly retire the duplicate root chart tree |
| Hosted release | partial | exact-tag public artifact acceptance | Run the updated tagged workflow |

## Change-control rule

Do not mark a gate green from a plan, source comment, generated manifest, or
local-only test when the gate requires hosted, hardware, browser, authentication,
or provenance evidence. Record the missing artifact and keep the gate partial.
