# Model Onboarding Guide

This guide moves a model from artifact selection to an observable serving
resource and an optional promotion gate.

## Responsibility Split

The model scientist owns the model, serving requirements, and acceptance
criteria. The platform team owns storage access, cluster capacity, Gateway API,
metrics, and feature gates.

## 1. Plan Capacity

Estimate weight memory, KV cache, runtime overhead, parallelism, and expected
traffic before creating a workload. For the large models covered by this
repository:

```bash
./run/capacity-plan.sh
```

The local KIND environment is not a capacity test for large models.

## 2. Choose an Artifact URI

`LLMInferenceService.spec.model.uri` supports:

- `hf://` for Hugging Face download;
- `hf-mount://` for the Hugging Face CSI path;
- `oci://` and `ocis://` for OCI artifacts;
- `s3://`, `gs://`, and `swfs://` for object or distributed storage;
- `pvc://` for an existing PersistentVolumeClaim;
- `modelpack://` for a model package;
- `https://` for a direct HTTPS source.

Use `spec.model.storage` references for credentials. Do not put tokens in the
manifest.

Start with the download-based `hf://` path. The optional `hf-mount://` path
requires privileged cluster-wide CSI/FUSE prerequisites. See
[Hugging Face: First Model](huggingface.md) for a public CPU smoke test and the
single `HF_TOKEN` Secret shape used by both paths.

## 3. Declare the Service

Start from a maintained sample. Use the stable
`serving.ckodex.com/v1` API when its schema covers the workload.

```yaml
apiVersion: serving.ckodex.com/v1
kind: LLMInferenceService
metadata:
  name: llama-3-8b
  namespace: inference
spec:
  model:
    name: llama-3-8b
    uri: hf://meta-llama/Meta-Llama-3-8B-Instruct
  replicas: 1
  template:
    spec:
      containers:
        - name: vllm
          resources:
            limits:
              cpu: "8"
              memory: 32Gi
              nvidia.com/gpu: "1"
  router:
    gateway:
      managed:
        gatewayClassName: envoy
    route:
      httpRoute: {}
    scheduler:
      pool: {}
```

Apply and inspect:

```bash
kubectl apply -f model.yaml
kubectl get llminferenceservice llama-3-8b -n inference -w
kubectl describe llminferenceservice llama-3-8b -n inference
kubectl get deployment,service,gateway,httproute -n inference
```

Readiness means the model and generated runtime resources are ready. Object
creation alone is not readiness.

## 4. Pre-Stage Weights When Needed

`LocalModelCache` can create node-targeted warm-up jobs. It is cluster-scoped
and requires a working storage class and model download path.

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: LocalModelCache
metadata:
  name: llama-3-8b-cache
spec:
  sourceModelUri: hf://meta-llama/Meta-Llama-3-8B-Instruct
  modelSize: 15Gi
  warmNodes:
    - gpu-node-01
```

Observe it before pointing a service at the resulting storage:

```bash
kubectl get localmodelcache llama-3-8b-cache -w
kubectl describe localmodelcache llama-3-8b-cache
```

The platform team must confirm the actual generated PVC names and access policy.
Do not assume the cache object name is itself a valid `pvc://` claim.

## 5. Add a Promotion Sequence

`ModelOnboarding` sequences checks against an existing
`LLMInferenceService`.

```yaml
apiVersion: serving.ckodex.com/v1
kind: ModelOnboarding
metadata:
  name: llama-3-8b-promotion
  namespace: inference
spec:
  modelRef: llama-3-8b
  rollbackOnFailure: true
  stages:
    - name: model-ready
      type: validation
    - name: canary-ready
      type: canary
    - name: service-objectives
      type: gate
      gate:
        minSuccessRate: 99
        maxLatencyP99: 500
    - name: promotion-ready
      type: promotion
```

Current behavior:

- `validation` requires `status.modelReady`;
- `canary` requires at least one ready replica;
- `gate` queries Prometheus when criteria are present;
- `promotion` requires the model to remain ready;
- missing Prometheus configuration causes metric gates to fail closed.

The controller does not change traffic weights in canary or promotion stages.
Configure desired weights through `LLMInferenceService.spec.canary` and let the
Gateway reconciler apply them.

Observe the sequence:

```bash
kubectl get modelonboarding llama-3-8b-promotion -n inference -w
kubectl describe modelonboarding llama-3-8b-promotion -n inference
```

## Failure Diagnosis

| Symptom | Check |
|---|---|
| Model never becomes ready | Pod logs, model URI, credentials, node resources |
| Route is absent | Gateway feature gate, GatewayClass, HTTPRoute events |
| Cache does not warm | Node name, storage class, warm-up Job logs |
| Gate rolls back | Prometheus URL, query data, ready replicas, declared thresholds |
| Request uses wrong model | Request `model` must equal `spec.model.name` |

Continue with the [Model Deployment Runbook](runbooks/model-deployment.md) for
operational troubleshooting.
