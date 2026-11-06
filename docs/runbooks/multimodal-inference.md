# Multimodal inference runbook

This operational runbook provides instructions for deploying, configuring, and operating multimodal foundation models: Baidu Unlimited-OCR-2 (`task: ocr`) and Meta SAM 3 (`task: segmentation`) using `MultimodalInferenceService` (`serving.ckodex.com/v1alpha2`).

## Foundation model profiles

### 1. Baidu Unlimited-OCR-2
- **Model identifier**: `baidu/Unlimited-OCR-2`
- **Task**: `ocr`
- **Architecture**: 3B Vision-Language document parsing model equipped with Sparse Reference Sliding Window Attention (SR-SWA).
- **Core capabilities**: End-to-end multi-page OCR (100 to 300+ pages per document), table extraction to Markdown, and mathematical formula conversion to LaTeX.
- **Hardware requirements**: Minimum 1x NVIDIA GPU with 12 GiB VRAM (RTX 3500 Ada, L4, or A10) with CUDA 13.0.

### 2. Meta SAM 3
- **Model identifier**: `facebook/sam3`
- **Task**: `segmentation`
- **Architecture**: Segment Anything Model 3 foundation model for Promptable Concept Segmentation (PCS) and visual object tracking.
- **Core capabilities**: Zero-shot concept-guided segmentation masks, multi-object visual tracking, and bounding box polygon generation.
- **Hardware requirements**: Minimum 1x NVIDIA GPU with 16 to 24 GiB VRAM (RTX 4090, RTX PRO 6000 Ada, A100, or L40S) with CUDA 13.0.

## Deploying Baidu Unlimited-OCR-2

Create a `MultimodalInferenceService` manifest targeting the `ocr` task:

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: MultimodalInferenceService
metadata:
  name: unlimited-ocr-2
  namespace: ckodex-inference
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

Deploy and wait for readiness:

```bash
kubectl apply -f unlimited-ocr-2.yaml
kubectl rollout status deployment/unlimited-ocr-2 -n ckodex-inference
```

### Testing OCR inference

Query the OpenAI-compatible `/v1/chat/completions` endpoint with an image URL:

```bash
curl -X POST http://unlimited-ocr-2.ckodex-inference.svc.cluster.local:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "baidu-unlimited-ocr-2",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "Extract all text and tables from this document into clean Markdown format."},
          {"type": "image_url", "image_url": {"url": "https://example.com/invoice-sample.png"}}
        ]
      }
    ],
    "max_tokens": 2048
  }'
```

## Deploying Meta SAM 3

Create a `MultimodalInferenceService` manifest targeting the `segmentation` task:

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: MultimodalInferenceService
metadata:
  name: sam3-segmentation
  namespace: ckodex-inference
spec:
  task: segmentation
  runtime: vllm
  model:
    name: meta-sam3
    uri: hf://facebook/sam3
  replicas: 1
  template:
    spec:
      containers:
        - name: vllm
          image: vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e
          resources:
            requests:
              cpu: "12"
              memory: 24Gi
              nvidia.com/gpu: "1"
            limits:
              cpu: "12"
              memory: 24Gi
              nvidia.com/gpu: "1"
```

Apply and monitor rollout:

```bash
kubectl apply -f sam3-segmentation.yaml
kubectl rollout status deployment/sam3-segmentation -n ckodex-inference
```

### Testing concept segmentation

Invoke the `/v1/images/segmentations` endpoint:

```bash
curl -X POST http://sam3-segmentation.ckodex-inference.svc.cluster.local:8000/v1/images/segmentations \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-sam3",
    "image": "https://example.com/warehouse-scene.jpg",
    "concept": "forklift palette",
    "output_format": "rle"
  }'
```

## Troubleshooting multimodal workloads

### 1. CUDA out-of-memory on large documents
**Symptom**: Pod crashes with `torch.cuda.OutOfMemoryError: CUDA out of memory` during OCR prefill.
**Cause**: The input PDF has too many pages or resolution exceeds the GPU VRAM budget.
**Remediation**:
- Constrain `spec.maxImagesPerPrompt: 8` (or lower) in the service specification.
- Reduce GPU memory utilization by setting `VLLM_GPU_MEMORY_UTILIZATION: "0.80"` in container `env` to provide more overhead for dynamic tensor activations.
- If processing 100+ pages in a single request, chunk the document into sub-batches of 16 pages via client-side UTCP tools.

### 2. Image decoding failure
**Symptom**: HTTP 400 with message `Cannot decode image payload: unsupported format`.
**Cause**: Input image is encoded in an unsupported container format (for example, HEIC or corrupted WebP).
**Remediation**:
- Pre-process images to PNG or JPEG before sending to the endpoint.
- Verify the image URL is publicly accessible or reachable from within the cluster.

### 3. High time-to-first-token (TTFT)
**Symptom**: First token takes more than 10 seconds to generate.
**Cause**: High-resolution image feature extraction takes substantial compute before language generation begins.
**Remediation**:
- Enable LMCache prefix caching if multiple questions query the same document image.
- Resize source images to 1024x1024 maximum resolution unless fine print OCR requires native pixel fidelity.
