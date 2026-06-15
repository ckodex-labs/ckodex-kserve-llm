# KServe v0.17 World-Class Local Deployment (KIND + LLMInferenceService)

Zero-touch model serving:

- Model "upload" = just set `uri: hf://...` (auto-download)
- Production features: Gateway API, KV-cache scheduler, WVA autoscaling ready, Standard mode
- Tested on clean KIND cluster (March 2026)

Prerequisites: Docker, kind v0.26+, helm v3.16+, kubectl
