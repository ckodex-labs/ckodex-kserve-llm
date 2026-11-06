# Kimi K3.1 Specimen Model GPU Smoke Test — 2026-11-05

## 1. Test Environment

- **Host**: `mchorfa-1016-linux-01` (`192.168.2.198`), Linux x86_64.
- **Accelerator**: NVIDIA RTX 3500 Ada Generation Laptop GPU (12,282 MiB VRAM, Driver `580.178.04`, CUDA `13.0`).
- **Cluster**: `ckodex-gpu-validation` (Kind Kubernetes `v1.35.8`) with NVIDIA Device Plugin registered (`nvidia.com/gpu: 1`).
- **Target Specimen Model**: `tiny-random/kimi-k3.1-pro` ([Hugging Face](https://huggingface.co/tiny-random/kimi-k3.1-pro)).
- **Container Environment**: `vllm/vllm-openai:v0.32.0` with `fla-core 0.6.0` dependency.

---

## 2. Observed Execution

1. **Model Synchronization**:
   - Cloned `https://huggingface.co/tiny-random/kimi-k3.1-pro` into `/home/mchorfa/ckodex-inference-gme-001/models/kimi-k31-specimen` (37MB total footprint).
   - Validated safetensors tensor hierarchy: verified 17-layer hybrid structure containing KDA linear attention layers, DSA attention indexer layers (physical [3, 7, 11, 15, 16]), MoE block sparse layers, vision tower, and patch merger projection.

2. **Standalone GPU Execution**:
   - Loaded tokenizer and model into GPU VRAM using `fla-core` (`FusedRMSNormGated`, `ShortConvolution`).
   - Observed parameters: `18,197,776` parameters (18.2M).
   - Allocated VRAM on NVIDIA RTX 3500 Ada: `29.82 MB`.
   - Forward pass token generation and decoding verified on `cuda:0`.

3. **Kubernetes Cluster Execution (`ckodex-gpu-validation`)**:
   - Deployed test pod `kimi-specimen-gpu-test` in namespace `default` requesting `nvidia.com/gpu: 1`.
   - Pod scheduled on `ckodex-gpu-validation-control-plane`.
   - Successfully loaded the TikToken tokenizer (`tokenization_kimi.TikTokenTokenizer`) and `KimiK3ForConditionalGeneration` architecture onto the GPU inside the Kubernetes pod.
   - Forward pass and token validation completed with `=== Specimen Model Test PASS ===`.
   - Pod deleted, GPU memory returned to 13 MiB idle.

---

## 3. Evidence Status

- **Model Download & Checksum**: `VERIFIED`
- **GPU CUDA Device Access**: `VERIFIED`
- **Architecture & Tokenizer Loading**: `VERIFIED`
- **Kubernetes Pod Scheduling under `nvidia.com/gpu: 1`**: `VERIFIED`
- **Post-Execution Cleanup & Memory Reclaim**: `VERIFIED`
