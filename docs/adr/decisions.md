# ADR-001: KServe v0.17 LLMInferenceService CRD Design

## Status

Accepted

## Context

We need a Kubernetes operator for managing LLM inference workloads. KServe v0.17 introduces the `LLMInferenceService` CRD pattern alongside the Gateway Inference Extension (InferencePool, InferenceModel).

## Decision

- Build a standalone operator inspired by KServe v0.17 `LLMInferenceService` but under `serving.ckodex.com/v1` API group
- Use Gateway API (HTTPRoute + GRPCRoute) instead of Knative for routing
- V2 Open Inference Protocol as the primary data plane protocol
- Support OCI model distribution alongside `hf://`, `s3://`, `gs://`, `pvc://`
- 3 additional CRDs: `AgentSpec`, `SkillRegistry`, `ModelOnboarding`

## Consequences

- Full control over lifecycle and features without upstream KServe dependency
- Must maintain CRD compatibility testing against KServe v0.17 release assets
- Can add domain-specific features (agents, skills, resilience, Dapr) not in upstream

---

# ADR-002: Gateway API over Knative

## Status

Accepted

## Context

KServe supports both Knative (Serverless mode) and Gateway API (Standard mode). Gateway API is GA and provides HTTPRoute + GRPCRoute.

## Decision

- Use Gateway API Standard mode exclusively
- GRPCRoute for V2 gRPC (GRPCInferenceService)
- HTTPRoute for V2 REST + OpenAI-compatible endpoints
- Managed Gateway creation with Envoy as default GatewayClass

## Consequences

- No Knative dependency
- GRPCRoute requires Gateway API v1.4+ (available in v1.5.1)
- Scale-to-zero requires KEDA instead of Knative autoscaler

---

# ADR-003: Native SPIFFE/SPIRE

## Status

Accepted

## Context

LLM inference workloads require secure identity and mTLS between all components.

## Decision

- Operator manages SPIRE Server (StatefulSet) and Agent (DaemonSet) directly
- SPIFFE ID format: `spiffe://ckodex.com/ns/{ns}/sa/{sa}/model/{model}`
- X.509 SVIDs for mTLS, JWT SVIDs for API auth
- Workload attestation via K8s namespace + service account selectors

## Consequences

- Full control over identity management without external SPIRE operator dependency
- Must handle SPIRE upgrades as part of operator lifecycle

---

# ADR-004: Dapr Workflows for Agent Management

## Status

Accepted

## Context

Model onboarding, agent scaling, and skill updates require multi-step orchestration with rollback.

## Decision

- Use Dapr Go SDK workflow engine
- Saga pattern with compensation for rollback
- CloudEvents → Dapr pub/sub → workflow triggers
- 4 workflows: model-onboarding, agent-scaling, skill-update, model-rollback

## Consequences

- Dapr runtime dependency (sidecar injection)
- State persistence via Dapr state store
- Portable across cloud providers
