# CLI and YAML cheat sheet

A quick reference of copy-paste CLI commands and minimal YAML snippets for common operations.

## Common CLI commands

### Inspect custom resources

```bash
# List all LLM inference services cluster-wide
kubectl get llminferenceservices -A -o wide

# Check lifecycle states and conditions
kubectl get llminferenceservice -A \
  -o custom-columns="NAMESPACE:.metadata.namespace,NAME:.metadata.name,STATE:.status.state,READY:.status.conditions[?(@.type=='Ready')].status"

# Inspect detailed status and events
kubectl describe llminferenceservice <name> -n <namespace>

# List multimodal services and AIPacks
kubectl get multimodalinferenceservices -A
kubectl get aipacks -A
kubectl get agents -A
```

### Pod and container logs

```bash
# Operator controller manager logs
kubectl logs -n ckodex-system -l control-plane=controller-manager -c manager -f

# Storage initializer logs (weight downloads)
kubectl logs deploy/<service-name> -c storage-initializer -f

# Runtime inference engine logs (vLLM)
kubectl logs deploy/<service-name> -c vllm -f

# Tail recent Kubernetes events
kubectl get events -n <namespace> --sort-by=.lastTimestamp
```

### Ingress, routing, and ports

```bash
# Forward port 8000 for direct testing
kubectl port-forward svc/<service-name> 8000:8000 -n <namespace>

# Inspect Gateway API HTTPRoutes
kubectl get httproutes -A

# Test OpenAI chat completion endpoint
curl -s -X POST http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<model-name>",
    "messages": [{"role": "user", "content": "ping"}],
    "max_tokens": 16
  }' | jq .

# Test engine health and metrics
curl -s http://localhost:8000/health
curl -s http://localhost:8000/metrics | grep vllm:
```

### Dagger and local contract checks

```bash
# Fast lint and compile gate
dagger call all --source=.

# Run full race-detector test suite with coverage
dagger call test --source=.

# Run contract tests in hack/
go test -count=1 ./hack/helm-contract/...
go test -count=1 ./hack/public-release-contract/...
go test -count=1 ./hack/oscal-assessment/...
```

---

## Minimal YAML snippets

### 1. Minimal CPU specimen LLMInferenceService

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: specimen-service
  namespace: default
spec:
  model:
    name: tiny-specimen
    uri: hf://tiny-random/kimi-k3.1-pro
  replicas: 1
  template:
    spec:
      containers:
        - name: vllm
          image: vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e
          resources:
            requests:
              cpu: "2"
              memory: 4Gi
            limits:
              cpu: "2"
              memory: 4Gi
```

### 2. GPU production LLMInferenceService

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: qwen25-production
  namespace: default
  annotations:
    serving.ckodex.com/fast-restart: "true"
spec:
  model:
    name: qwen-2.5-7b-instruct
    uri: hf://Qwen/Qwen2.5-7B-Instruct
  replicas: 2
  template:
    spec:
      containers:
        - name: vllm
          image: vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e
          env:
            - name: VLLM_GPU_MEMORY_UTILIZATION
              value: "0.90"
          resources:
            requests:
              cpu: "8"
              memory: 32Gi
              nvidia.com/gpu: "1"
            limits:
              cpu: "8"
              memory: 32Gi
              nvidia.com/gpu: "1"
```

### 3. Multimodal OCR service (Baidu Unlimited-OCR-2)

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: MultimodalInferenceService
metadata:
  name: unlimited-ocr-2
  namespace: default
spec:
  task: ocr
  runtime: vllm
  model:
    name: baidu-unlimited-ocr-2
    uri: hf://baidu/Unlimited-OCR-2
  maxImagesPerPrompt: 16
  replicas: 1
  template:
    spec:
      containers:
        - name: vllm
          image: vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e
          resources:
            requests:
              cpu: "8"
              memory: 16Gi
              nvidia.com/gpu: "1"
            limits:
              cpu: "8"
              memory: 16Gi
              nvidia.com/gpu: "1"
```

### 4. AIPack for UTCP manual (kind code A15)

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: AIPack
metadata:
  name: aipack-vision-tools
  namespace: default
spec:
  kind: UTCPManual
  family: capability
  source:
    ref: registry.corp.internal/aipack/vision-tools@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  utcpManual:
    utcpVersion: "1.0.1"
    manualVersion: "1.0.0"
    endpoint: http://unlimited-ocr-2.default.svc.cluster.local:8000/utcp
```

### 5. Node-local weight cache (LocalModelCache)

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: LocalModelCache
metadata:
  name: qwen-node-cache
spec:
  modelName: qwen-2.5-7b-instruct
  source:
    uri: oci://registry.corp.internal/models/qwen25-7b:v1
  nodeSelector:
    accelerator: nvidia-rtx-ada
  storage:
    size: 50Gi
    path: /var/lib/ckodex/models/qwen25-7b
  retentionPolicy: Retain
```
