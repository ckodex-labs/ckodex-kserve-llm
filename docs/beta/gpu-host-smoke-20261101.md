# GPU host smoke test — 2026-11-01

## Observed execution

- Host: `mchorfa-1016-linux-01` (`192.168.2.198`), NVIDIA RTX 3500 Ada
  Generation Laptop GPU, 12,282 MiB, driver `580.178.04`.
- Docker passed the GPU to a cached NVIDIA CUDA 12.6 container. A separate
  host PyTorch CUDA matrix multiplication returned the expected finite result.
- The cached `vllm/vllm-openai:v0.32.0` image reported image ID and repository
  digest `sha256:befccf0e7e4089aa23926bb6f887a85c0d46c2a99e6de5c5ffdf5cb7957c5b34`.
- A temporary vLLM container served the existing local
  `Josiefied-Qwen2-0.5B-Instruct-abliterated` model, mounted read-only with
  Hugging Face offline mode. Its HTTP port was bound to host loopback only.
  `/v1/models` returned `ckodex-gpu-smoke`; an OpenAI-compatible completions
  request returned one nonempty choice and eight completion tokens.
- The container was stopped and removed. GPU usage returned to 245 MiB, with
  11,630 MiB free at the subsequent observation.

## Limits

This verifies a standalone GPU/container/vLLM inference path, not operator
reconciliation, KAI scheduling, authentication, model provenance, performance,
streaming, recovery, or the Qwen3 qualification sample. The local model's
artifact digest and provenance were not independently verified.

The three running kind clusters exposed no allocatable NVIDIA GPU during this
test. KAI was not found in those clusters. The separate `tprime-k3s` context
required interactive authentication and was not inspected further.
