# Runbook: Upgrading to TensorFold Runtime & vLLM v0.31.0 for Decision-Class Inference

This runbook guides cluster operators and ML platform engineers through upgrading the
CKodex KServe LLM Operator data-plane stack to support **vLLM v0.31.0** and the **TensorFold
runtime**, standardizing serving configurations via the **ModelServingProfile** CRD, and deploying
**GLM-5.3-EXL3** across a 4x NVIDIA RTX PRO 6000 hardware cluster.

---

## 1. Executive Summary & Stack Upgrades

The upgraded stack introduces two major runtime enhancements:

1. **vLLM v0.31.0**:
   - Advanced PagedAttention v3 with reduced metadata overhead and enhanced FP8 KV cache utilization.
   - Synchronous and asynchronous chunked prefill scheduling for high-concurrency throughput.
   - Native structured decoding shims for rapid grammar and schema enforcement.

2. **TensorFold Runtime (v0.31.0)**:
   - Purpose-built inference engine optimized for extreme-quantization formats including EXL3.
   - High-efficiency tensor parallelism (`TP=4`) tailored for multi-GPU workstation-class interconnects (such as PCIe 5.0 and NVLink bridges on RTX PRO 6000 Ada / Blackwell systems).
   - First-class support for **Decision-Class Inference**: strictly deterministic decoding (`temperature=0.0`), structured output governance, and verifiable reasoning paths.

3. **ModelServingProfile CRD (`serving.ckodex.com/v1alpha2`)**:
   - Declarative abstraction separating hardware and runtime engineering constraints from tenant model declarations.
   - Standardizes admission rules, recommended engine flags, and decoding policies for workload classes such as `decision-evaluator`.

---

## 2. Hardware Planning: GLM-5.3-EXL3 on 4x RTX PRO 6000

### 2.1 Weight Footprint & Memory Envelope

| Metric | GLM-5.3 Full Precision | GLM-5.3 EXL3 Quantized |
|---|---|---|
| Parameter Count | ~130B (MoE active / dense backbone) | ~130B |
| Weight Footprint | ~260 GiB (BF16) | ~136 GiB (EXL3 dynamic) |
| Target Hardware | 8x H100 (80 GiB) | **4x RTX PRO 6000 (48 GiB each = 192 GiB total)** |
| Headroom for KV Cache & Activations | Insufficient on 4x 48 GiB | **~56 GiB headroom for KV cache at 32k context** |

### 2.2 Interconnect & Parallelism Configuration

- **Tensor Parallelism (`tensor: 4`)**: Distributes the projection matrices across the 4 RTX PRO 6000 cards.
- **Pipeline Parallelism (`pipeline: 1`)**: Single-node multi-GPU deployment eliminates inter-node networking overhead.
- **System Memory & Host CPU**: Minimum 32 vCPU cores and 128 GiB host RAM to support concurrent memory transfers and tokenization.

---

## 3. Sample Manifests Reference

Two declarative manifests in `config/samples/` demonstrate the complete configuration:

### 3.1 Model Serving Profile: Decision Evaluator

[`config/samples/modelservingprofile_decision_evaluator.yaml`](../../config/samples/modelservingprofile_decision_evaluator.yaml)
defines the standardized profile for high-fidelity evaluation and reasoning tasks:

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: ModelServingProfile
metadata:
  name: decision-evaluator
  namespace: default
  labels:
    serving.ckodex.com/profile: decision-evaluator
    serving.ckodex.com/engine: tensorfold
    serving.ckodex.com/workload-class: decision-evaluator
spec:
  workloadClass: decision
  decisionCapabilities:
    - predicate
    - choice
    - score
  runtime:
    engine: tensorfold
    image: ghcr.io/ckodex-labs/tensorfold-runtime:v0.31.0
    port: 8000
    protocol: http
  sla:
    maxLatencyMillis: 500
    timeoutSeconds: 60
    targetAvailability: "99.9%"
    targetThroughputRPS: 50
  applicability:
    supportedRuntimes:
      - tensorfold
    targetEngines:
      - tensorfold
    modelFamilies:
      - glm53
      - glm
    workloadSelector:
      serving.ckodex.com/profile: decision-evaluator
    hardwareRequirements:
      accelerator: "nvidia-rtx-pro-6000"
      minGPUCount: 4
      minMemory: "128Gi"
  evidence:
    minTrustLevel: "verified"
    requireSBOM: true
    requireSignature: true
```

### 3.2 LLMInferenceService: GLM-5.3-EXL3 on 4x RTX PRO 6000

[`config/samples/llminferenceservice_tensorfold_glm53_rtx6000.yaml`](../../config/samples/llminferenceservice_tensorfold_glm53_rtx6000.yaml)
deploys the service using the TensorFold runtime:

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: LLMInferenceService
metadata:
  name: glm53-tensorfold-rtx6000
  namespace: ckodex-inference
  labels:
    serving.ckodex.com/profile: decision-evaluator
    serving.ckodex.com/engine: tensorfold
    serving.ckodex.com/hardware: 4x-rtx-pro-6000
spec:
  model:
    name: THUDM/glm-5.3-exl3
    uri: hf://THUDM/glm-5.3-exl3
  engine: tensorfold
  replicas: 1
  parallelism:
    tensor: 4
    pipeline: 1
  quantization:
    method: exl3
  template:
    metadata:
      labels:
        serving.ckodex.com/role: llminferenceservice-workload
        serving.ckodex.com/engine: tensorfold
    spec:
      nodeSelector:
        nvidia.com/gpu.product: RTX-PRO-6000
      containers:
        - name: tensorfold
          image: ghcr.io/ckodex-labs/tensorfold-runtime:v0.31.0
          args:
            - --model
            - THUDM/glm-5.3-exl3
            - --tensor-parallel-size
            - "4"
            - --quantization
            - exl3
            - --max-model-len
            - "32768"
            - --max-num-seqs
            - "16"
            - --gpu-memory-utilization
            - "0.92"
            - --enforce-eager=false
          env:
            - name: TENSORFOLD_ATTENTION_BACKEND
              value: "FLASH_ATTN"
            - name: TENSORFOLD_DECISION_MODE
              value: "true"
            - name: CUDA_VISIBLE_DEVICES
              value: "0,1,2,3"
          resources:
            requests:
              cpu: "32"
              memory: 128Gi
              nvidia.com/gpu: "4"
            limits:
              cpu: "32"
              memory: 128Gi
              nvidia.com/gpu: "4"
          ports:
            - name: http
              containerPort: 8000
  router:
    gateway:
      managed:
        gatewayClassName: envoy
    route:
      httpRoute:
        hostnames:
          - glm53-eval.ckodex.internal
    scheduler:
      pool:
        selector:
          serving.ckodex.com/role: llminferenceservice-workload
```

---

## 4. Step-by-Step Upgrade Procedure

### Step 1: Install Custom Resource Definitions

Apply the CRDs including `modelservingprofiles` and updated `llminferenceservices`:

```bash
kubectl apply -k config/crd/
kubectl get crd modelservingprofiles.serving.ckodex.com llminferenceservices.serving.ckodex.com
```

### Step 2: Configure and Upgrade Helm Operator Deployment

Verify the Helm chart values configure admitted engines and hardware options:

```bash
helm template ckodex-operator deploy/helm \
  --set engines.tensorfold.enabled=true \
  --set engines.vllm.version="v0.31.0" \
  --namespace ckodex-system > /tmp/rendered-operator.yaml

helm upgrade --install ckodex-kserve-llm deploy/helm \
  --namespace ckodex-system \
  --create-namespace
```

### Step 3: Validate Target Node Labels

Ensure the target worker node has the 4x RTX PRO 6000 GPUs and correct labels:

```bash
kubectl get nodes -L nvidia.com/gpu.product,nvidia.com/gpu.count
# Expected label: nvidia.com/gpu.product=RTX-PRO-6000
```

### Step 4: Apply the Model Serving Profile

```bash
kubectl apply -f config/samples/modelservingprofile_decision_evaluator.yaml
kubectl get modelservingprofile decision-evaluator -o yaml
```

### Step 5: Deploy the LLMInferenceService

Ensure the destination namespace exists and is labelled for tenant management:

```bash
kubectl create namespace ckodex-inference --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace ckodex-inference ckodex.com/tenant-id=eval-team --overwrite

kubectl apply -f config/samples/llminferenceservice_tensorfold_glm53_rtx6000.yaml
```

---

## 5. Verification & Health Validation

### 5.1 Workload Status & Log Inspection

Watch the custom resource transition through initialization to Ready:

```bash
kubectl get llminferenceservice glm53-tensorfold-rtx6000 -n ckodex-inference -w
```

Check the engine logs during model weight loading:

```bash
kubectl logs -n ckodex-inference -l app.kubernetes.io/name=glm53-tensorfold-rtx6000 -c tensorfold -f
```

Look for successful tensor parallelism initialization across 4 GPUs:
```text
[TensorFold] Initialized process group with 4 ranks across device [0, 1, 2, 3]
[TensorFold] Loading EXL3 weights for model THUDM/glm-5.3-exl3 (layers=80, kv_heads=8)
[TensorFold] Allocated 136.2 GiB weights across 4 GPUs (~34.05 GiB / GPU)
[TensorFold] KV Cache initialized: 32768 tokens context window (~14.2 GiB / GPU)
[TensorFold] HTTP OpenAI-compatible server ready at 0.0.0.0:8000
```

### 5.2 Deterministic Decision Evaluation Request

Port-forward the service to verify OpenAI compatibility and strict determinism:

```bash
kubectl port-forward service/glm53-tensorfold-rtx6000 -n ckodex-inference 8000:8000 &
PID=$!

curl -sS http://127.0.0.1:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "THUDM/glm-5.3-exl3",
    "messages": [
      {"role": "system", "content": "You are a decision evaluator. Output strict JSON with verdict and score."},
      {"role": "user", "content": "Evaluate change PR-402: added tensorfold runtime adapter."}
    ],
    "temperature": 0.0,
    "max_tokens": 256,
    "response_format": {"type": "json_object"}
  }' | jq .

kill $PID
```

---

## 6. Troubleshooting & Rollback

| Symptom | Probable Cause | Remediation |
|---|---|---|
| `CUDA out of memory` during startup | KV cache allocation too high for remaining VRAM | Lower `--gpu-memory-utilization` from `0.92` to `0.88` or reduce `--max-model-len` to `16384`. |
| Pod stuck in `Pending` | Node selector mismatch or fewer than 4 GPUs available | Verify `kubectl describe pod` matches `nvidia.com/gpu.product: RTX-PRO-6000` and allocatable GPU count is >= 4. |
| Inconsistent evaluation outputs | Temperature > 0 or non-deterministic kernel | Enforce `temperature: 0.0` and set `TENSORFOLD_DECISION_MODE="true"` in container environment. |
| Gateway 503 / Route not ready | EndpointPicker or HTTPRoute pending ready replicas | Check `kubectl get inferencepool,httproute -n ckodex-inference`. Ensure at least 1 replica is healthy. |

### Rollback Procedure

If the upgraded stack needs to be rolled back to the previous stable baseline:

```bash
# Delete the TensorFold workload
kubectl delete -f config/samples/llminferenceservice_tensorfold_glm53_rtx6000.yaml

# Revert to standard vLLM profile if necessary
kubectl apply -f config/samples/llminferenceservice_basic.yaml
```
