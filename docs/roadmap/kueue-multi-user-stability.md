# Kueue and Multi-User Stability Assessment

**Status:** design proposal; Kueue is not currently integrated  
**Scope:** workload admission, resource fairness, request stability, and tenant boundaries  
**Recommendation:** integrate Kueue as an optional workload-admission layer, while keeping request-level fairness in the Gateway/data plane and identity/isolation in the platform security layer.

## Executive answer

Yes, Kueue can help this operator scale across multiple users and teams—but in
one specific part of the system:

```text
Kueue:       decides when a workload/pod may consume cluster resources
Operator:    reconciles the model service and its Deployments/routes/status
Gateway/EPP: decides where and how an admitted inference request is served
Auth/RBAC:   decides who may access which tenant and namespace
```

Kueue is a good fit for:

- CPU/GPU quota reservation;
- fair sharing between team queues;
- priority classes and controlled preemption;
- resource flavors for CPU/GPU/node pools;
- bounded borrowing between queues;
- admission visibility and pending-workload reason codes.

Kueue is not, by itself, a solution for:

- per-user HTTP rate limiting;
- token budgets;
- request-level queue depth or `429`/`Retry-After` behavior;
- model-level KV-cache-aware request routing;
- authentication, namespace authorization, or tenant identity;
- guaranteeing that every replica in a Deployment is admitted as one atomic
  serving unit.

The Kueue documentation explicitly says that Deployment integration represents
each Deployment pod as an independent workload. That permits scale-out and
scale-in, but a Deployment may temporarily run only a subset of its requested
replicas when quota is unavailable. [Kueue Deployment integration](https://kueue.sigs.k8s.io/docs/tasks/run/deployment/)

## Current repository posture

### Already implemented foundations

| Layer | Current repository capability | Evidence boundary |
|---|---|---|
| Namespace resource guard | `TenantQuotaReconciler` creates `ResourceQuota` and `LimitRange` for namespaces labelled `ckodex.com/tenant-id` | Unit/integration tests; live hosted acceptance remains open |
| Default resource caps | 5 LLM services, 8 GPUs, 64 CPU, 256 GiB memory per labelled namespace | `internal/controller/tenant_quota_controller.go`, `docs/model-capacity.md` |
| Replica autoscaling | HPA/KEDA/WVA reconcilers can target model Deployments | `internal/autoscaler/reconciler.go` |
| Request bulkhead primitive | In-memory per-model `Bulkhead` and resilience policy types exist | `internal/gateway/resilience.go` and tests |
| Tenant admission policy | Immutable evaluator can return admit, backpressure, or reject decisions | `internal/accessplane/policy.go` and tests |
| KV-aware endpoint selection | EPP/InferencePool resources and queue/load/cache scoring configuration exist | `internal/scheduler/`, `internal/gateway/` |
| Namespace access | Helm-managed namespace allow-list and EPP identity provisioning exist | `deploy/helm/values.yaml`, RBAC templates |

### Important gaps

1. The current tenant quota reconciler creates a quota object in each labelled
   namespace. It does not aggregate usage across multiple namespaces with the
   same tenant ID. It is therefore a **per-namespace guard**, unless the
   platform contract enforces one namespace per tenant.
2. The access-plane evaluator is not an external HTTP admission caller. Its
   backpressure result does not enqueue a request or emit a production `429`.
3. The bulkhead primitive is in-memory and does not provide a shared limit across
   operator replicas, Gateway replicas, or model pods.
4. Envoy AI Gateway rate-limit reconciliation is still marked TODO in the code.
5. Autoscaling is model/service-oriented. It is not yet a tested, tenant-aware
   admission system.
6. The operator currently has no Kueue queue-name API or generated Kueue labels.

## Proposed architecture

```text
                 authenticated tenant request
                              │
                              ▼
       Gateway request policy: identity, rate, concurrency, tokens
                              │
                  429 / Retry-After / timeout
                              │
                              ▼
                 HTTPRoute + EPP request routing
                              │
                              ▼
                 ready vLLM/SGLang serving replicas

  LLMInferenceService ──operator──► Deployment / Service / Route / status
           │                              │
           │                              └── Kueue queue label + resource requests
           ▼
  namespace tenant boundary          Kueue admission layer
  RBAC / NetworkPolicy                LocalQueue → ClusterQueue → ResourceFlavor
  ResourceQuota / LimitRange          fair sharing / borrowing / priority
```

The operator should remain the owner of model-service lifecycle and generated
resources. Kueue should remain the owner of workload admission and resource
reservation. The Gateway should remain the owner of request admission. This
avoids putting distributed queue state into the Kubernetes operator or using
Kueue as an HTTP request scheduler.

## Recommended queue topology

Start with one namespace per tenant or team. Do not claim aggregate tenant
quota across multiple namespaces until a separate accounting controller exists.

### Namespaces and LocalQueues

- Label explicitly managed inference namespaces, for example
  `ckodex.com/tenant-id=t-123` and `kueue-managed=true`.
- Create one namespaced `LocalQueue` per tenant/team.
- Use Kueue default LocalQueue only when the platform wants every eligible
  workload in that namespace to be managed automatically.
- Reject or suspend unqueued workloads in managed namespaces; do not allow an
  arbitrary namespace or label to bypass the queue policy.

Kueue provides opt-in namespace management and queue-name enforcement patterns
for this purpose. [Kueue namespace management](https://kueue.sigs.k8s.io/docs/tasks/manage/enforce_job_management/opt_in_namespace_management/)

### ClusterQueues

Use separate ClusterQueues for materially different service classes rather
than one global queue:

| Queue | Purpose | Preemption recommendation |
|---|---|---|
| `interactive-cpu` | Small local and CPU inference services | No preemption initially |
| `interactive-gpu` | User-facing GPU inference | No preemption of ready serving replicas initially |
| `batch-gpu` | Evaluation, indexing, offline jobs | May be preemptible |
| `system-reserved` | Operator, Gateway, Kueue, observability | Never borrowed by tenants |

Use `ResourceFlavor` to distinguish node pools, accelerator types, spot/on-
demand capacity, and topology constraints. Use cohorts only when controlled
borrowing is desired. Set explicit borrowing and lending limits; unrestricted
borrowing makes “fair” behavior difficult to explain.

Kueue ClusterQueues provide quotas, resource flavors, and fair-sharing policy;
cohorts allow controlled quota borrowing. [ClusterQueue concepts](https://kueue.sigs.k8s.io/docs/concepts/cluster_queue/)

### Priority

Define priority by service class, not by arbitrary user input:

- `interactive-critical`: reserved capacity, no tenant-level preemption of
  already-ready replicas;
- `interactive-standard`: normal admission;
- `batch-low`: can wait and can be preempted;
- `system`: platform-owned, outside tenant borrowing.

Kueue priority and preemption must be tested against graceful model shutdown.
Preempting a large model pod can cause a long cold-start and worsen stability.

## Operator integration design

### Phase 1: label propagation without changing the public API

Use namespace-level queue defaults first. This keeps the operator API stable and
lets the platform configure Kueue independently. The operator must preserve or
propagate the queue label onto generated model Deployments and pod templates if
the platform explicitly supplies it.

Required changes:

- define a reserved queue-label policy;
- copy `kueue.x-k8s.io/queue-name` only from an approved source;
- prevent user-supplied labels from changing the Deployment selector;
- preserve the queue label through Deployment updates;
- add conformance tests for single-node, prefill, and multi-node outputs;
- expose queue/admission state in operator status without claiming readiness.

### Phase 2: explicit service scheduling contract

If namespace defaults are insufficient, add a versioned field such as:

```yaml
spec:
  scheduling:
    queueName: interactive-gpu
    priorityClassName: interactive-standard
    admissionPolicy: required
```

Rules:

- `queueName` resolves only to a `LocalQueue` in the same namespace;
- `priorityClassName` is allow-listed by platform policy;
- `admissionPolicy: required` means the operator reports capacity blocked when
  Kueue is unavailable or the workload has no admission;
- `admissionPolicy: optional` is allowed only for explicitly non-production
  profiles;
- v1 and v1alpha2 conversion must preserve the scheduling contract;
- the operator must never create a ClusterQueue or grant quota to itself.

### Phase 3: replica and readiness semantics

Kueue’s Deployment integration can admit individual pods. The operator must
therefore distinguish at least:

```text
desiredReplicas
admittedReplicas
scheduledReplicas
readyReplicas
```

The service must not report a fully healthy state when the required minimum
replica count is not admitted and ready. Scale-out may remain pending under
quota pressure, but the status and metrics must make that visible.

For multi-node or tightly coupled inference, use a workload form that provides
an atomic pod group where possible. Kueue’s all-or-nothing and
`waitForPodsReady` mechanisms are relevant, but they must be tested against the
actual KServe/LeaderWorkerSet resource generated by this repository. [Kueue all-or-nothing scheduling](https://kueue.sigs.k8s.io/docs/concepts/all_or_nothing/)

## Request-level stability design

Kueue is necessary but insufficient for multi-user inference stability. Add a
separate request policy with a real production caller.

### Minimum controls

- authenticate the caller before deriving tenant identity;
- apply per-tenant and per-model max in-flight limits;
- apply bounded queue depth and deadline budgets;
- return deterministic `429` plus `Retry-After` when admission is refused;
- cap prompt/context and maximum output tokens per service class;
- use weighted fairness or separate queues for interactive versus batch traffic;
- avoid retry amplification at the Gateway and model client;
- expose queue wait, rejection, timeout, TTFT, tokens/sec, and error metrics;
- keep tenant labels bounded; never put raw user IDs in high-cardinality metrics.

### Ownership

| Concern | Preferred owner |
|---|---|
| Authentication and tenant identity | Gateway/identity platform |
| HTTP rate/concurrency/token limits | Gateway or dedicated shared request admission service |
| Endpoint selection and KV-cache locality | EPP / Gateway API Inference Extension |
| Pod/resource admission | Kueue |
| Namespace hard caps | Kubernetes ResourceQuota/LimitRange |
| Model lifecycle and status | CKodex operator |
| Cross-tenant audit | Observability/security platform |

The existing access-plane evaluator can become the policy kernel, but it needs
a production adapter that owns shared state and maps decisions to HTTP behavior.
Do not advertise current unit tests as multi-user runtime fairness.

## Actionable milestones

### K0 — Establish the tenancy contract

**Deliverables:**

- decide whether one tenant may own multiple namespaces;
- if yes, explicitly mark current quotas as per-namespace and design aggregate
  accounting separately;
- define tenant identity source, namespace label, queue classes, and service
  classes;
- choose whether interactive serving may ever be preempted.

**Exit evidence:** ADR with the tenancy model, authority owners, and failure
semantics. No code change is required.

### K1 — Install and qualify Kueue as a platform dependency

**Files:** `deploy/helm/values.yaml`, `docs/tenant-onboarding.md`,
`docs/runbooks/`, `docs/evidence/`.

**Actions:**

1. Keep Kueue installation platform-owned; do not silently install it from the
   operator chart.
2. Define opt-in managed namespaces.
3. Create LocalQueues, ClusterQueues, ResourceFlavors, and service priorities.
4. Reserve system capacity and set conservative borrowing/preemption defaults.
5. Add a disposable acceptance profile with the exact Kueue configuration.

**Exit criteria:** an unmanaged namespace bypasses neither platform policy nor
the intended queue scope; a managed namespace receives a visible pending state
when quota is unavailable.

### K2 — Propagate queue admission into generated workloads

**Files:** `api/v1/`, `api/v1alpha2/`, `api/v1alpha2/conversion*.go`,
`internal/controller/deployment/`, `internal/controller/llminferenceservice_*`,
`test/conformance/`, generated CRDs.

**Actions:**

1. Start with approved namespace/default queue labels.
2. Add explicit `spec.scheduling` only if needed after K1.
3. Propagate queue and approved priority to Deployment/pod metadata.
4. Preserve labels through reconciliation and scale changes.
5. Add status conditions for admission/capacity distinct from readiness.

**Exit criteria:** queue assignment is deterministic, same-namespace only,
selector-safe, conversion-safe, and visible in generated manifests and status.

### K3 — Qualify serving scale behavior

**Files:** `internal/autoscaler/`, `internal/controller/deployment/`,
`config/samples/`, `test/integration/`, `docs/evidence/`.

**Actions:**

1. Scale a Deployment from one to multiple replicas under insufficient quota.
2. Verify ready replicas remain truthful while extra replicas wait.
3. Scale back down and verify quota is released.
4. Test HPA/KEDA/WVA changes with Kueue-managed pods.
5. Test model restart and node drain without uncontrolled preemption.
6. Test multi-node/KServe workload admission separately.

**Exit criteria:** no false Ready state, no quota leak, no retry storm, and no
unexpected eviction of ready interactive replicas.

### K4 — Implement production request admission

**Files:** `internal/accessplane/`, `internal/inference/`, Gateway integration,
`internal/gateway/`, `test/k6/`, `docs/evidence/`.

**Actions:**

1. Connect the policy evaluator to the real ingress/request path.
2. Define shared state for limits across Gateway/operator replicas.
3. Enforce tenant/model in-flight and queue-depth limits.
4. Map rejection to stable HTTP status, `Retry-After`, and audit event.
5. Test deadlines, cancellation, retry amplification, and model fallback.

**Exit criteria:** two tenants competing for one model receive bounded and
observable behavior; one tenant cannot consume all request concurrency; no
claim relies on an in-memory single-process limiter.

### K5 — Measure multi-user stability

**Files:** `test/k6/`, `docs/evidence/`, `docs/model-capacity.md`.

Use a fixed workload matrix rather than a single throughput number:

| Scenario | Required observation |
|---|---|
| One tenant, low concurrency | Baseline TTFT, latency, errors |
| Two equal tenants | Fair share and bounded wait |
| One noisy tenant | Other tenant’s SLO remains within declared bound |
| Batch plus interactive | Interactive traffic is not starved |
| Quota exhaustion | Pending/admission state and request rejection are distinct |
| Replica scale-out | Admission delay and ready-replica truthfulness |
| Pod restart/node drain | Recovery time and request failure behavior |

**Exit criteria:** publish environment, model, prompt distribution, context
length, concurrency, resource requests, queue configuration, and raw results.
Do not publish “stable,” “scalable,” or “fair” without this artifact.

### K6 — Promote one supported multi-user profile

**Exit criteria:**

- K1–K5 pass for one declared CPU or GPU profile;
- tenant identity and namespace boundary are authenticated and attributable;
- Kueue admission and request admission are both observable;
- ResourceQuota/LimitRange and Kueue policy values do not contradict silently;
- the readiness ledger names remaining limitations;
- the profile is promoted only for the tested model, hardware, queue topology,
  and concurrency envelope.

## Risks and mitigations

| Risk | Consequence | Mitigation |
|---|---|---|
| Kueue and ResourceQuota have different limits | Confusing pending/forbidden behavior | One policy source renders both hard cap and queue quota, or document precedence |
| Deployment pods admitted independently | Partial serving fleet and false capacity assumptions | Separate admitted/ready counts; use atomic workload form for coupled inference |
| Preemption of model pods | Long cold starts and request failures | Disable interactive preemption initially; test graceful drain before enabling |
| Per-process bulkhead | Limits multiply with replicas | Shared Gateway/request-admission state and explicit global budgets |
| Same tenant spans namespaces | Tenant exceeds intended aggregate quota | One namespace per tenant or aggregate usage controller |
| Kueue queue label spoofing | Quota bypass or priority abuse | Opt-in namespace selector, allow-listed queues/priorities, admission policy |
| Autoscaler fights admission | Replica oscillation and quota churn | Scale on bounded signals; expose Kueue pending state to scaling policy |
| Long prompts consume KV/cache capacity | Noisy-neighbor latency and OOM | Token/context limits, KV-aware routing, per-tenant budgets, load tests |

## Final recommendation

Adopt Kueue, but make it optional and platform-owned. The first useful
integration is queue-label propagation plus truthful admission/replica status.
Do not begin by adding a large scheduler abstraction to the CRD.

The priority order is:

1. clarify that current tenant quotas are per namespace;
2. qualify one CPU serving profile;
3. add Kueue admission for that profile;
4. connect request-level tenant limits to a real Gateway path;
5. measure two-tenant/noisy-neighbor behavior;
6. only then extend to GPU/NVFP4 and multi-node profiles.

Kueue should improve resource fairness and admission stability. It should not be
described as proof of end-to-end multi-user stability until the request-level
and identity-level gates also pass.
