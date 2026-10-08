# Beta readiness ledger

This ledger is intentionally evidence-first. A green unit test does not close a
live-cluster, hosted-release, provenance, browser, or human-identity gate.

## Current gate state

| Gate | State | Required evidence | Current action |
| --- | --- | --- | --- |
| Source reproducibility | hosted-verified | fresh checkout contains console | Retain CI and RC7 release evidence |
| Console CI gate | hosted-verified | CI runs all console checks | Retain CI evidence on merge commit `eccb4d71` |
| Dagger CI gate | hosted-verified | Dagger lint, build, race tests, and coverage gates on the exact release head | Retain [CI run 33455614072](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/33455614072) |
| Published chart | hosted-verified | chart renders console profile | Retain [RC7 release run 33457020052](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/33457020052) |
| Console image | hosted-verified | signed multi-architecture image plus HIGH/CRITICAL scan | Retain RC7 release evidence |
| Runtime profile | partial | CPU and required GPU acceptance | Define matrix; execute live profile gates |
| Nightly KIND acceptance | hosted-failing-local-change-unverified | Workload lifecycle reaches readiness and inference without harness lookup races | Latest checked main-head run [36406335463](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/36406335463) failed before E2E on Gateway API CRD ownership; rerun the aligned KIND v0.33.0/Kubernetes v1.35.8 and Envoy Gateway v1.8.4 profile from the changed head |
| Inference probe qualification | local-fixed-hosted-pending | Gateway hostname binding, strict HTTP/JSON response validation, retry, and port-forward cleanup | Local result is recorded in [the KIND acceptance record](../evidence/local-kind-2026-08-26.md); confirm the hosted rerun reaches the inference assertion |
| Local image build reproducibility | local-fixed-rerun-pending | Bounded Docker context and BuildKit image targets load into KIND | Re-run `run/e2e.sh` after the `.dockerignore` and `buildx --load` changes |
| Stable v1 conversion path | local-conversion-verified-runtime-pending | beta CRD bundle, lossless conversion round-trip, live v1 create/update/read | Install the fixed beta profile and prove storage plus webhook behavior in a cluster |
| Stable v1 admission | local-fixed-hosted-pending | v1 validator/defaulting registration, webhook routes, and live v1 admission | Re-run the exact-head hosted profile and retain API server/webhook evidence |
| Scheduler dependency and RBAC | local-runtime-verified-hosted-pending | GIE `v1.5.0` and llm-d Router `v0.10.0` CRDs, EPP workload identity, and operator permissions | Local result is recorded in [the KIND acceptance record](../evidence/local-kind-2026-08-26.md); dependency decisions are recorded in [the refresh record](../evidence/dependency-refresh-2026-08-27.md); re-run from a fresh hosted cluster |
| Default CPU storage path | local-runtime-verified-hosted-pending | default `hf://` initializer path without optional privileged CSI/FUSE dependency | Local CPU result is recorded in [the KIND acceptance record](../evidence/local-kind-2026-08-26.md); accept `hf-mount://` only under its explicit profile |
| llm-d agentic profile | live-acceptance-required | disaggregated prefill, KV transfer, speculative decoding, and bursty long-context behavior | Execute `docs/runbooks/llm-d-agentic-readiness.md` on target hardware and retain sustained-load, cache-hit, scheduler, and recovery evidence |
| Release image/chart alignment | hosted-verified | chart defaults resolve operator, console, and initializer images from release `appVersion` | Retain RC7 packaged chart and public release evidence |
| KIND environment reproducibility | static-contract-verified-runtime-pending | disposable cluster uses the digest-pinned node image recorded in `deploy/kind/acceptance-node-image.txt`; after starting Colima, a container's Docker soft `nofile` limit measured 1,024 against the 65,536 preflight requirement | Re-run the aligned KIND v0.33.0 / Kubernetes v1.35.8 profile on a qualifying host and in Nightly; see [the earlier failure evidence](../ci/hosted-exact-head-2026-08-28.md) |
| Human access | unresolved | authenticated boundary tests | Select authenticated-ingress or OIDC posture |
| Readiness explanation | partial | causal investigation acceptance | Add correlated dependency context |
| Promotion semantics | excluded | explicit route mutation proof | Keep readiness-only language for beta |
| Evidence verification | partial | signed verification result | Complete cosign path or downgrade claims |
| Browser accessibility | local-browser-proven-hosted-and-assistive-tech-pending | packaged image browser pass: CSS, heading order, target sizes, mobile overflow, command palette focus/escape, theme switching, no console warnings | Repeat in hosted CI and complete assistive-technology/reduced-motion evidence |
| Console dependency security | local-ready | zero production advisories from npm audit | Re-run in hosted CI and retain development-only advisory context |
| Chart source alignment | partial | one authoritative chart publish path | Consolidate or explicitly retire the duplicate root chart tree |
| Hosted release | passed | successful exact-head CI, injected release version, and exact-tag public artifact acceptance | Retain [RC7 release run 33457020052](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/33457020052); downstream signature/provenance verification remains separate |

## Change-control rule

Do not mark a gate green from a plan, source comment, generated manifest, or
local-only test when the gate requires hosted, hardware, browser, authentication,
or provenance evidence. Record the missing artifact and keep the gate partial.
