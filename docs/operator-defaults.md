# Operator workload defaults

The operator reads workload defaults at startup from its environment. Helm
values in both supported charts populate these variables; changing them
requires a normal operator Deployment rollout.

| Environment variable | Applies to |
| --- | --- |
| `CKODEX_RUNTIME_IMAGE` | vLLM runtime |
| `CKODEX_CUSTOM_STORAGE_INITIALIZER_IMAGE` | non-`hf://` model downloads and cache warmup Jobs |
| `CKODEX_HUGGING_FACE_INITIALIZER_IMAGE` | `hf://` model downloads |
| `CKODEX_QUANT_CPP_IMAGE` | quant-cpp/GGUF runtime |
| `CKODEX_VLLM_CPU_REQUEST`, `CKODEX_VLLM_MEMORY_REQUEST` | vLLM and reranker defaults |
| `CKODEX_TERMINATION_GRACE_PERIOD_SECONDS` | vLLM pod default |
| `CKODEX_ASR_CPU_REQUEST`, `CKODEX_ASR_MEMORY_REQUEST` | ASR defaults |
| `CKODEX_ASR_TERMINATION_GRACE_PERIOD_SECONDS` | ASR and cache-job grace default |
| `CKODEX_CACHE_CPU_REQUEST`, `CKODEX_CACHE_MEMORY_REQUEST` | LocalModelCache warmup Jobs |

Explicit per-service images, resources, and pod settings continue to take
precedence over these operator defaults. Values are not watched dynamically;
the operator Deployment must be rolled after changing the configuration.
