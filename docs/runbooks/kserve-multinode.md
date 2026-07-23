# KServe v0.19 Multi-Node Serving

CKodex delegates explicit multi-node workloads to an upstream KServe
`serving.kserve.io/v1beta1 InferenceService`. KServe owns the Ray head/worker
lifecycle, GPU allocation, model mount, predictor Service, and readiness state.
CKodex continues to own its Gateway route, policy, evidence, and status
projection.

## Prerequisites

- KServe v0.19 installed in Standard mode.
- The `kserve-huggingfaceserver-multinode` `ClusterServingRuntime` installed,
  or `CKODEX_KSERVE_MULTINODE_RUNTIME` set to another compatible installed
  runtime.
- Model weights available through `pvc://` or `oci://`. A PVC must be readable
  from every selected node.
- GPU requests on both the head template and worker template.

Apply
[`config/samples/llminferenceservice-kserve-multinode.yaml`](../../config/samples/llminferenceservice-kserve-multinode.yaml)
after replacing the model URI, node selector, and resource sizes.

## Mapping

| CKodex field | KServe field |
|---|---|
| `spec.model.uri` | `spec.predictor.model.storageUri` |
| `spec.parallelism.tensor` | `spec.predictor.workerSpec.tensorParallelSize` |
| `spec.parallelism.pipeline` | `spec.predictor.workerSpec.pipelineParallelSize` |
| `spec.template` pod settings | predictor head pod overrides |
| `spec.experimental.worker.template` | predictor worker pod overrides |
| operator runtime setting | `spec.predictor.model.runtime` |

The generated KServe object always sets:

```yaml
metadata:
  annotations:
    serving.kserve.io/deploymentMode: Standard
    serving.kserve.io/autoscalerClass: none
spec:
  predictor:
    minReplicas: 1
    maxReplicas: 1
```

Exactly one head is required. CKodex scaling, canary, LoRA, disaggregated
prefill, data parallelism, expert parallelism, and EPLB are rejected on this
KServe v0.19 path because `workerSpec` cannot represent those contracts.

Do not set an image, command, or args in either template container. Those fields
belong to the selected KServe runtime and include the Ray and health-check
lifecycle. The templates may set resources, environment, mounts, scheduling,
security context, and service-account configuration.

## Verify

```bash
kubectl get inferenceservice.serving.kserve.io gemma4-multinode -n models -o yaml
kubectl get deploy,svc,pod -n models \
  -l serving.kserve.io/inferenceservice=gemma4-multinode
kubectl get llminferenceservice.serving.ckodex.com gemma4-multinode -n models
```

The upstream object must report `Ready=True`, the CKodex status URL must point
to the KServe URL (or the `-predictor` Service while pending), and no
CKodex-owned standalone `LeaderWorkerSet` or same-name Deployment may exist.

Live acceptance still requires at least two schedulable GPU nodes and a real
model load. Unit tests prove the emitted resource contract; they do not prove
Ray/NCCL behavior on hardware.
