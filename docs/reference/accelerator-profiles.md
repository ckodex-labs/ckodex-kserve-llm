# Hardware accelerator profiles

This document defines the hardware accelerator models, resource request formats, memory configurations, and execution constraints supported by the operator.

The type definitions align with `api/v1alpha2/accelerator_types.go`.

## Supported accelerator vendors

| Vendor family (`type`) | Platform description | Kubernetes resource domain |
|---|---|---|
| `nvidia` | NVIDIA CUDA GPUs (Ada Lovelace, Hopper, Blackwell) | `nvidia.com/gpu` |
| `google-tpu` | Google Cloud TPUs (v4, v5e, v5p, v6e/Trillium) | `google.com/tpu` |
| `aws-neuron` | AWS Trainium and Inferentia ASICs | `aws.amazon.com/neuron` |
| `intel-gaudi` | Intel Habana Gaudi accelerators (Gaudi2, Gaudi3) | `habana.ai/gaudi` |
| `apple-metal` / `apple-mlx` | Apple Silicon Unified Memory accelerators (M3/M4 Ultra) | `apple.com/metal` |
| `rocm` | AMD Instinct GPUs (CDNA architecture) | `amd.com/gpu` |
| `cpu` | Host CPU with AVX-512 or AMX extensions | Standard CPU requests |

---

## Hardware models and profiles

### 1. NVIDIA profiles

| Model identifier (`model`) | Architecture | Typical VRAM | Precision formats | Typical target workloads |
|---|---|---|---|---|
| `rtx-pro-6000` | Ada Lovelace (SM89) | 48 GiB GDDR6 | FP16, BF16, FP8 | 7B to 32B models, OCR document parsing |
| `dgx-spark` | Hopper / Spark cluster | 80 GiB HBM3 | FP16, BF16, FP8 | High-throughput distributed serving |
| `sm100` | Blackwell (B100/B200) | 192 GiB HBM3e | FP16, BF16, FP8, NVFP4 | 70B+ dense models, large MoE prefill |
| `sm103` | Blackwell Ultra | 288 GiB HBM3e | FP16, BF16, FP8, NVFP4 | Frontier multi-modal foundation models |
| `gb300` | Grace Blackwell Desktop Superchip | 384 GiB Unified | FP16, BF16, FP8, NVFP4 | Massive context window reasoning |

**Resource specification example**:
```yaml
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

---

### 2. Google Cloud TPU profiles

| Model identifier (`model`) | Generation | Interconnect bandwidth | Supported precisions |
|---|---|---|---|
| `tpu-v4` | TPU v4 | 4.8 Tbps ICI | BF16, INT8 |
| `tpu-v5e` | TPU v5e (Lite) | 1.6 Tbps ICI | BF16, INT8 |
| `tpu-v5p` | TPU v5p (Performance) | 4.8 Tbps ICI | BF16, INT8, FP8 |
| `tpu-v6e` | TPU v6e (Trillium) | High-speed optical ICI | BF16, FP8, INT4 |

**Resource specification example**:
```yaml
resources:
  requests:
    google.com/tpu: "4"
  limits:
    google.com/tpu: "4"
```

---

### 3. AWS Neuron profiles

| Model identifier (`model`) | Hardware target | Accelerator memory | Target inference tier |
|---|---|---|---|
| `trn1` | AWS Trainium 1 (EC2 `trn1.32xlarge`) | 512 GiB HBM2e (32 NeuronCores) | Large distributed LLM inference |
| `trn2` | AWS Trainium 2 (EC2 `trn2.48xlarge`) | High-bandwidth HBM3e | Ultra-low latency reasoning |
| `inf2` | AWS Inferentia 2 (EC2 `inf2.8xlarge`) | 32 GiB HBM2e (4 NeuronCores) | Latency-critical 7B to 13B models |

**Resource specification example**:
```yaml
resources:
  requests:
    aws.amazon.com/neuron: "1"
  limits:
    aws.amazon.com/neuron: "1"
```

---

### 4. Intel Habana Gaudi profiles

| Model identifier (`model`) | Hardware target | Accelerator memory | Interconnect |
|---|---|---|---|
| `gaudi2` | Habana Gaudi2 | 96 GiB HBM2e | 24x 100GbE integrated RoCE ports |
| `gaudi3` | Habana Gaudi3 | 128 GiB HBM2e | 24x 200GbE integrated RoCE ports |

**Resource specification example**:
```yaml
resources:
  requests:
    habana.ai/gaudi: "1"
  limits:
    habana.ai/gaudi: "1"
```

---

### 5. Apple Silicon profiles

| Model identifier (`model`) | Architecture | Unified memory pool | Bandwidth |
|---|---|---|---|
| `m3-ultra` | Apple M3 Ultra | Up to 192 GiB Unified | 800 GB/s |
| `m4-ultra` | Apple M4 Ultra | Up to 256 GiB Unified | 1000+ GB/s |

**Resource specification example**:
```yaml
spec:
  nodeSelector:
    kubernetes.io/arch: arm64
    apple.com/accelerator: metal
```

---

### 6. AMD ROCm profiles

| Model identifier (`model`) | Architecture | Accelerator memory | Interconnect |
|---|---|---|---|
| `mi300x` | CDNA 3 | 192 GiB HBM3 | Infinity Fabric 896 GB/s |
| `mi325x` | CDNA 3 Refresh | 256 GiB HBM3e | Infinity Fabric |

**Resource specification example**:
```yaml
resources:
  requests:
    amd.com/gpu: "1"
  limits:
    amd.com/gpu: "1"
```

---

## Accelerator configuration in ModelServingProfile

You can enforce hardware constraints across namespaces using a `ModelServingProfile`:

```yaml
apiVersion: serving.ckodex.com/v1alpha2
kind: ModelServingProfile
metadata:
  name: blackwell-nvfp4-profile
spec:
  accelerator:
    type: nvidia
    model: sm100
    count: 2
  engine:
    name: vllm
    tier: Tier3Governed
  defaultArguments:
    - --kv-cache-dtype=nvfp4
    - --enable-flashmla
```
