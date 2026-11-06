# Deploy multimodal foundation models with OCR and SAM 3

This guide describes how to deploy, configure, and query multimodal foundation models using the `MultimodalInferenceService` custom resource (`serving.ckodex.com/v1alpha2`).

## Overview

The operator provides dedicated controllers and admission policies for multimodal foundation workloads:

- **Baidu Unlimited-OCR-2** (`task: ocr`): A 3B parameter Vision-Language model featuring Sparse Reference Sliding Window Attention (SR-SWA) for multi-page document parsing, Markdown table extraction, and mathematical LaTeX parsing.
- **Meta SAM 3** (`task: segmentation`): Segment Anything Model 3 for Promptable Concept Segmentation (PCS) and visual tracking across video and images.

## Prerequisites

- Kubernetes cluster with GPU nodes (NVIDIA CUDA 13.0 driver version 580.178.04 or higher).
- GPU VRAM requirements:
  - Unlimited-OCR-2: Minimum 1x GPU with 12 GiB VRAM (RTX 3500 Ada, L4, or A10).
  - Meta SAM 3: Minimum 1x GPU with 16 to 24 GiB VRAM (RTX 4090, RTX PRO 6000 Ada, L40S, or A100).
- Operator version v0.20.0 installed with CRD `multimodalinferenceservices.serving.ckodex.com` present.

## Step 1: Deploy Baidu Unlimited-OCR-2

Create a file named `unlimited-ocr-2.yaml`:

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

Apply the service manifest:

```bash
kubectl apply -f unlimited-ocr-2.yaml
```

Monitor the deployment rollout:

```bash
kubectl rollout status deployment/unlimited-ocr-2 -n ckodex-inference --timeout=300s
```

## Step 2: Query Baidu Unlimited-OCR-2

The service exposes an OpenAI-compatible vision completions endpoint on port 8000.

Send an image document to extract text and tables:

```bash
curl -X POST http://unlimited-ocr-2.ckodex-inference.svc.cluster.local:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "baidu-unlimited-ocr-2",
    "messages": [
      {
        "role": "user",
        "content": [
          {
            "type": "text",
            "text": "Extract all text, headers, and tables from this document into clean Markdown format."
          },
          {
            "type": "image_url",
            "image_url": {
              "url": "https://example.com/sample-invoice.png"
            }
          }
        ]
      }
    ],
    "max_tokens": 2048
  }' | jq .
```

The response includes Markdown structured tables and extracted text bodies.

## Step 3: Deploy Meta SAM 3

Create a file named `sam3-segmentation.yaml`:

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

Apply the SAM 3 manifest:

```bash
kubectl apply -f sam3-segmentation.yaml
```

Wait for readiness:

```bash
kubectl rollout status deployment/sam3-segmentation -n ckodex-inference --timeout=300s
```

## Step 4: Query Meta SAM 3 concept segmentation

Invoke the segmentation endpoint with an image URL and a target concept string:

```bash
curl -X POST http://sam3-segmentation.ckodex-inference.svc.cluster.local:8000/v1/images/segmentations \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-sam3",
    "image": "https://example.com/warehouse-scene.jpg",
    "concept": "forklift palette",
    "output_format": "rle"
  }' | jq .
```

The endpoint returns run-length encoded (RLE) binary masks identifying the concept regions.

## Step 5: Handle high-resolution documents and memory limits

When processing multi-page high-resolution PDFs or large image sets:

1. **Constrain maximum images**: Set `spec.maxImagesPerPrompt: 8` (or 4) to prevent memory allocation spikes during prefill.
2. **Adjust GPU memory utilization**: Set container environment variable `VLLM_GPU_MEMORY_UTILIZATION: "0.80"` to leave head-room for dynamic image activation tensors.
3. **Chunk multi-page inputs**: For documents exceeding 50 pages, process chunks of 8 to 16 pages through client-side UTCP tools rather than in a single request.

## Next steps

- [Author and bind UTCP tools](author-utcp-tools.md)
- [Consult the CRD reference](../reference/crds.md)
- [Troubleshoot workload failures](troubleshoot-workload-failures.md)
