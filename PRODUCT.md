# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

- Model scientists declare model artifacts, runtime requirements, and acceptance criteria, then observe readiness and call the resulting inference endpoint.
- Platform developers operate Kubernetes, KServe, GPU and storage infrastructure, feature gates, and the release path.
- Security and compliance reviewers inspect workload identity, policy state, runtime evidence, OSCAL results, and signed release artifacts.

## Product Purpose

The CKodex KServe LLM Operator reconciles model-serving intent into Kubernetes workloads, routing, scaling, identity, policy hooks, and observable status. The console provides an operator-facing view of that reconciliation state. Success means that declared services can be distinguished from ready services and that operational or governance claims remain traceable to their underlying runtime or release evidence.

## Positioning

The product keeps platform policy and evidence in the Kubernetes reconciliation loop around KServe model serving. It coordinates existing infrastructure; it does not replace Kubernetes, KServe, vLLM, model evaluation systems, or organizational governance.

## Operating Context

- The primary contract is `LLMInferenceService`; related resources cover specialized inference, LoRA adapters, artifact packs, onboarding, sessions, agents, skills, and coactors.
- Operators work from Kubernetes desired state, status conditions, events, metrics, and release evidence.
- Runtime capabilities depend on cluster feature gates and external services such as Gateway API, Prometheus, SPIFFE/SPIRE, policy engines, storage, and GPU infrastructure.
- The console is a Next.js web application under `console/` and reads operator state through server-side Kubernetes adapters.

## Capabilities and Constraints

- Stable and alpha API schemas coexist; API stability is separate from the beta chart version.
- A created resource is not equivalent to a ready model.
- Optional security, identity, policy, session, webhook, and observability features are environment-dependent.
- Experimental `Agent` and `SkillRegistry` resources validate references and publish status; they do not execute agents or tools.
- `ModelOnboarding` checks readiness and metrics-backed gates, while traffic weights remain a separate `LLMInferenceService` desired-state concern.
- Security status is evidence only when backed by the corresponding runtime or signed artifact. The interface must not fabricate proof, health, infrastructure, geography, cryptography, or performance claims.

## Brand Commitments

- Product name: CKodex KServe LLM Operator.
- Voice: terse, declarative, proof-literate, and sentence case.
- The CKODEX-DS-3 “Evidence Editorial” system is binding: semantic color, square quiet surfaces, proof-only violet, emergency-only red, and persistent evidence context.

## Evidence on Hand

- Product behavior and boundaries: `docs/overview.md`.
- Runtime state adapters: `console/src/lib/kserve.ts` and `console/src/lib/audit.server.ts`.
- CI and release evidence: `docs/ci/current-state.md`, `docs/release-verification.md`, and tag-driven release artifacts.
- No customer testimonials, performance benchmarks, production availability guarantee, or public console image is established by this record.

## Product Principles

1. Desired state, observed state, and attested state remain visibly distinct.
2. Claims carry evidence or state their qualification boundary.
3. Operators see the next consequential action without losing system context.
4. Environment-dependent capabilities never masquerade as universal product behavior.
5. The interface preserves Kubernetes and KServe terminology where precision matters.

## Accessibility & Inclusion

The interface is keyboard-complete, screen-reader-complete, responsive, and compatible with high-contrast and forced-color modes. Text and controls meet the CKODEX-DS-3 type, contrast, target-size, and reduced-motion gates.
