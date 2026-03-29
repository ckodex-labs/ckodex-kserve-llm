# Getting Started

The ckodex-kserve-llm operator manages LLM inference workloads on Kubernetes. It reconciles
`LLMInferenceService` custom resources into vLLM deployments, Gateway API routes, and an EPP
scheduler — giving you a single CR to go from model URI to a routed, observable inference endpoint.

## Prerequisites

- kind v0.24+ (`brew install kind`)
- kubectl v1.29+
- helm 3.x (`brew install helm`)
- Docker (for building the operator image)
- A HuggingFace account — free tier is sufficient for the gpt2 quickstart

## Quick Start (10 minutes)

### Step 1 — Create a local cluster

```bash
make kind-setup
```

This creates a single-node KIND cluster named `kserve-017` using `deploy/kind/kind-config.yaml`.
Ports 80→8080 and 443→8443 are mapped to the host for ingress access. The context is
automatically switched to `kind-kserve-017`.

### Step 2 — Build and install the operator

```bash
make docker-build
make kind-load
make deploy
```

`kind-load` depends on `docker-build` and loads the image `ghcr.io/ckodex/kserve-llm-operator:latest`
into the cluster. `deploy` applies CRDs, RBAC, the manager Deployment, and webhook configurations.

Verify the manager is running:

```bash
kubectl get pods -n ckodex-system
```

Expected output:

```
NAME                                          READY   STATUS    RESTARTS   AGE
ckodex-controller-manager-<hash>              1/1     Running   0          30s
```

### Step 3 — Configure credentials

For the gpt2 quickstart (public model on `openai-community/gpt2`), no credentials are required.

For private HuggingFace models, create a secret before applying the CR:

```bash
kubectl create secret generic hf-credentials \
  --from-literal=HF_TOKEN=<your-hf-token> \
  -n default
```

The storage initializer reads `HF_TOKEN` from this secret. If you use Vault, set `VAULT_PATH` on
the manager Deployment and ensure `VAULT_ADDR` and `VAULT_TOKEN` are also set — the operator will
exit at startup if `VAULT_PATH` is configured but either of the latter two is absent.

### Step 4 — Deploy your first model

```bash
kubectl apply -f config/samples/inference_test_gpt2.yaml
```

This creates an `LLMInferenceService` named `llama3-8b-test` in the `default` namespace using the
`openai-community/gpt2` model on CPU (2 vCPU, 4Gi RAM, no GPU required). The CPU-optimised vLLM
image `public.ecr.aws/q9t5s3a7/vllm-cpu-release-repo:v0.17.1` is used automatically.

Watch it become ready:

```bash
kubectl get llminferenceservice llama3-8b-test -w
```

Expected output (allow 2–5 minutes for the image pull on first run):

```
NAME              READY   STATUS      AGE
llama3-8b-test    False   Pending     10s
llama3-8b-test    True    Running     3m12s
```

### Step 5 — Send an inference request

```bash
kubectl port-forward svc/llama3-8b-test 8000:8000 &

curl -X POST http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt2","messages":[{"role":"user","content":"Hello!"}],"max_tokens":50}'
```

### Step 6 — Observe the operator

Prometheus metrics are exposed on port 8443 (metrics bind address) of the manager service:

```bash
kubectl port-forward svc/ckodex-controller-manager-metrics-service 8443:8443 -n ckodex-system
curl -sk https://localhost:8443/metrics | grep ckodex_
```

OpenTelemetry traces: set `CKODEX_OTEL_ENDPOINT=http://<your-otel-collector>:4317` in the manager
Deployment environment. The default OTLP endpoint is `localhost:4317` with a 10% sampling rate.

---

## CRD Reference

All CRDs are installed under the `serving.ckodex.com` API group, version `v1alpha2`.

| CRD | Kind | Purpose |
|---|---|---|
| `serving.ckodex.com_llminferenceservices.yaml` | `LLMInferenceService` | Core model serving CR — model URI, replicas, routing, scaling |
| `serving.ckodex.com_llminferenceserviceconfigs.yaml` | `LLMInferenceServiceConfig` | Cluster-wide operator defaults and compliance profiles |
| `serving.ckodex.com_llmloraadapters.yaml` | `LLMLoraAdapter` | Attach a LoRA adapter to a running `LLMInferenceService` |
| `serving.ckodex.com_endpointpickerconfigs.yaml` | `EndpointPickerConfig` | EPP scheduler plugin pipeline configuration |
| `serving.ckodex.com_inferencesessions.yaml` | `InferenceSession` | Stateful session tracking (requires `EnableSessions=true`) |
| `serving.ckodex.com_inferenceactors.yaml` | `InferenceActor` | Dapr actor wiring for session state (requires `EnableDapr=true`) |
| `serving.ckodex.com_coactorgroups.yaml` | `CoactorGroup` | Multi-actor coordination groups |
| `serving.ckodex.com_agents.yaml` | `Agent` | Agent registration for skill-based inference routing |
| `serving.ckodex.com_localmodelcaches.yaml` | `LocalModelCache` | Node-local model weight cache (requires `EnableLocalModelCache=true`) |
| `serving.ckodex.com_modelonboardings.yaml` | `ModelOnboarding` | Promotion gate workflow for new models |
| `serving.ckodex.com_skillregistries.yaml` | `SkillRegistry` | Registry of available inference skills |

---

## Environment Variables Reference

All variables are read at operator startup via `LoadFromEnv()`. Feature gate variables follow the
pattern `CKODEX_FEATURE_<GATE>=true|false`.

### Feature Gates

| Variable | Default | Description |
|---|---|---|
| `CKODEX_FEATURE_ENABLE_SCHEDULER` | `true` | Enable the EPP scheduler controller |
| `CKODEX_FEATURE_ENABLE_GATEWAY` | `true` | Enable Gateway API resource management |
| `CKODEX_FEATURE_ENABLE_AUTOSCALER` | `true` | Enable HPA/KEDA/WVA autoscaling |
| `CKODEX_FEATURE_ENABLE_SECURITY` | `false` | Enable SPIFFE/SPIRE infrastructure management |
| `CKODEX_FEATURE_ENABLE_CHAOS` | `false` | Enable the chaos engine controller |
| `CKODEX_FEATURE_ENABLE_DAPR` | `false` | Enable Dapr workflow/actor integration |
| `CKODEX_FEATURE_ENABLE_LOCAL_MODEL_CACHE` | `false` | Enable the LocalModelCache controller |
| `CKODEX_FEATURE_ENABLE_AUTH` | `false` | Enable JWT/OIDC authentication middleware |
| `CKODEX_FEATURE_ENABLE_OTEL_PIPELINE` | `true` | Enable end-to-end OpenTelemetry tracing |
| `CKODEX_FEATURE_ENABLE_SESSIONS` | `false` | Enable InferenceSession/Actor/CoactorGroup controllers |
| `CKODEX_FEATURE_ENABLE_GRPC` | `false` | Enable gRPC listener and GRPCRoute reconciliation (Triton only; not for vLLM) |
| `CKODEX_FEATURE_ENABLE_WEBHOOKS` | `false` | Enable mutating/validating admission webhooks (requires cert-manager) |

### Runtime Configuration

| Variable | Default | Description |
|---|---|---|
| `CKODEX_RUNTIME_IMAGE` | `public.ecr.aws/q9t5s3a7/vllm-cpu-release-repo:v0.17.1` | Default vLLM container image injected when no image is specified in the CR |
| `CKODEX_SCHEDULER_IMAGE` | `us-central1-docker.pkg.dev/k8s-staging-gateway-api/gateway-api-inference-extension/epp:main` | EPP scheduler container image |
| `CKODEX_AUTH_ISSUER_URL` | — | OIDC issuer URL. Required when `EnableAuth=true` |
| `CKODEX_AUTH_AUDIENCE` | — | Expected JWT audience. Required when `EnableAuth=true` |
| `CKODEX_OTEL_ENDPOINT` | `localhost:4317` | OTLP gRPC collector endpoint for traces |
| `CKODEX_OTEL_SERVICE_NAME` | `ckodex-kserve-llm-operator` | OTel service name tag on all spans |
| `CKODEX_OTEL_SAMPLING_RATE` | `0.1` | Trace sampling rate (0.0–1.0) |
| `CKODEX_HF_MIRROR_URL` | — | Override `huggingface.co` base URL for air-gapped HF Hub proxies (e.g. `https://hf-mirror.corp.internal`) |
| `CKODEX_SECURITY_FEDRAMP_MODE` | `false` | Reject `hf://` model URIs at admission. Requires `EnableWebhooks=true` |
| `CKODEX_SEMANTIC_CACHE_ADDR` | — | Redis/Valkey address for distributed semantic cache (e.g. `valkey:6379`). Empty = in-memory fallback |
| `CKODEX_SEMANTIC_CACHE_TTL` | `1h` | How long inference responses are cached (Go duration string, e.g. `30m`, `2h`) |
| `CKODEX_PROMETHEUS_URL` | — | Prometheus base URL for ModelOnboarding promotion gate metric queries (e.g. `http://prometheus.monitoring.svc:9090`). Empty = all metric checks pass unconditionally |
| `CKODEX_COMPLIANCE_PROFILES` | — | Comma-separated compliance profiles to enforce at startup: `hipaa`, `soc2`, `fedramp` |
| `HTTP_PROXY` | — | HTTP proxy for outbound storage and OIDC requests |
| `HTTPS_PROXY` | — | HTTPS proxy for outbound storage and OIDC requests |
| `NO_PROXY` | — | Comma-separated hosts/CIDRs that bypass the proxy |

---

## Minimum Viable CR Examples

### gpt2 (CPU, no GPU required)

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: LLMInferenceService
metadata:
  name: llama3-8b-test      # name used for the Service and route
  namespace: default
spec:
  model:
    uri: hf://openai-community/gpt2   # public model, no HF_TOKEN needed
    name: gpt2
  replicas: 1
  template:
    spec:
      containers:
      - name: vllm
        image: public.ecr.aws/q9t5s3a7/vllm-cpu-release-repo:v0.17.1
        resources:
          limits:
            cpu: "2"
            memory: 4Gi
          requests:
            cpu: "2"
            memory: 4Gi
        ports:
        - containerPort: 8000
  router:
    gateway:
      managed:
        gatewayClassName: envoy
    route:
      httpRoute:
        hostnames:
        - llama3-test.local     # Host header for Gateway API routing
    scheduler:
      pool:
        selector:
          app.kubernetes.io/instance: llama3-8b-test
  scaling:
    minReplicas: 1
```

### Llama 3.1 8B Instruct (GPU required)

Requires a node with at least one NVIDIA GPU. Uncomment the `nvidia.com/gpu` resource limit.
This model is gated on HuggingFace — ensure `HF_TOKEN` is set and you have accepted the license.

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: LLMInferenceService
metadata:
  name: llama3-8b
  namespace: default
spec:
  model:
    uri: hf://meta-llama/Llama-3.1-8B-Instruct
    name: meta-llama/Llama-3.1-8B-Instruct
  replicas: 2
  template:
    spec:
      containers:
      - name: vllm
        image: vllm/vllm-openai:latest
        resources:
          limits:
            cpu: "8"
            memory: 32Gi
            # nvidia.com/gpu: "1"   # Uncomment for GPU nodes
        securityContext:
          runAsNonRoot: true
          runAsUser: 1000
  router:
    gateway:
      managed:
        gatewayClassName: envoy
    route:
      httpRoute: {}
    scheduler:
      pool: {}
```

### LoRA Adapter

A `LLMLoraAdapter` attaches a fine-tuned LoRA adapter to an already-running `LLMInferenceService`.
The `targetService` must exist and be in `Ready` state before applying the adapter.

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: LLMLoraAdapter
metadata:
  name: sql-helper
  namespace: default
spec:
  targetService: llama2-7b       # name of an existing LLMInferenceService
  adapterName: sql-lora           # adapter ID used in inference requests
  model:
    uri: hf://yard1/llama-2-7b-sql-lora-test
    name: sql-lora
```

Inference requests targeting the adapter pass `"model": "sql-lora"` in the request body.

---

## Compliance Profiles

Set `CKODEX_COMPLIANCE_PROFILES=hipaa,soc2,fedramp` (comma-separated) on the manager Deployment.
`ApplyComplianceDefaults` auto-corrects compatible settings first; `EnforceComplianceProfiles`
then fails startup if any remaining constraint is unmet.

| Profile | Enforced Constraints | Auto-applied Defaults |
|---|---|---|
| `hipaa` | `EnableAuth=true`; `EnableLocalModelCache=false`; `AuditSink.RetentionDays >= 2555` (7 years) | Sets auth on, disables local cache, raises retention to 2555 days, enables PII redaction |
| `soc2` | `EnableSecurity=true`; `AuditSink.Type != stdout`; `AuditSink.PIIRedaction=true` | Enables SPIFFE/SPIRE, switches audit sink from `stdout` to `file`, raises retention to 365 days |
| `fedramp` | `EnableAuth=true`; `EnableSecurity=true` | Enables auth and security; replaces `AllowedRegistries` with FedRAMP-approved set (`ghcr.io/ckodex/`, `gcr.io/distroless/`) |

Multiple profiles can be combined. All violations across all profiles are reported together at startup.

---

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---|---|---|
| Pod stays `NotReady` / storage initializer crashes | `HF_TOKEN` not set for a private model | `kubectl create secret generic hf-credentials --from-literal=HF_TOKEN=<token> -n default` |
| `"private HuggingFace models will fail"` in manager logs | `HF_TOKEN` not configured | Create the `hf-credentials` secret (see above) |
| `VAULT_PATH set but VAULT_ADDR or VAULT_TOKEN missing` / operator exits at startup | Vault integration partially configured | Set all three: `VAULT_PATH`, `VAULT_ADDR`, and `VAULT_TOKEN` in the manager Deployment |
| Webhook certificate error / `x509` errors on admission | cert-manager not installed or webhooks enabled without TLS | Install cert-manager v1.14+, then set `CKODEX_FEATURE_ENABLE_WEBHOOKS=true` |
| Compliance profile check fails at startup | Required feature gate not set for the active profile | Read the error — each violation includes the exact env var to set |
| `hf://` URI rejected at admission under FedRAMP mode | `CKODEX_SECURITY_FEDRAMP_MODE=true` | Use an authorized internal registry URI instead of `hf://` |
| EPP scheduler pod not created | `CKODEX_FEATURE_ENABLE_SCHEDULER=false` | Set `CKODEX_FEATURE_ENABLE_SCHEDULER=true` (default is `true`) |
| Metrics endpoint returns TLS error | Metrics bind on `:8443` (HTTPS) | Use `curl -sk https://localhost:8443/metrics` or configure a plain HTTP metrics port |
