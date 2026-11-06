# Operator-managed GPU smoke — 2026-11-01

## Environment and scope

- A dedicated kind cluster, `ckodex-gpu-validation`, ran Kubernetes `v1.35.8`
  on `mchorfa-1016-linux-01` (`192.168.2.198`). The node used an isolated
  per-process Docker wrapper for GPU access; the host Docker daemon configuration
  was not changed. A privileged NVIDIA device plugin using CDI registered one
  allocatable NVIDIA RTX 3500 Ada Generation Laptop GPU.
- A GPU-requesting CUDA pod ran `nvidia-smi` successfully. The operator CRDs
  and Gateway API types were installed in the isolated cluster.
- The manager ran as a host process using the isolated kubeconfig, with gateway,
  scheduler, autoscaler, and webhooks disabled. Its fixed binary SHA-256 was
  `1509d7d6216552dbcd787eb9cd88caeff1c2cc3a0261d34567ed8bb988b24b5b`.
  It was built from a dirty working tree; the commit alone does not identify
  all source inputs.
- A read-only static PV/PVC exposed an existing local Qwen2 0.5B model. Its
  digest and provenance were not independently verified. The service used a
  Secret-backed vLLM API key and requested one GPU.

## Observed execution

- The manager reconciled `LLMInferenceService/gpu-smoke` into a Deployment
  and Services. The pod ran on the GPU node with `VLLM_TARGET_DEVICE=cuda`;
  `VLLM::EngineCore` used approximately 5,408 MiB of GPU memory. Both vLLM
  and the Vector sidecar became ready, and the service reported `Ready=True`.
- Through a loopback Kubernetes Service port-forward, a request without an API
  key returned HTTP 401. An authenticated completion returned a nonempty
  choice. After a Deployment rollout, both checks passed again; an authenticated
  streaming completion returned seven chunks and a `[DONE]` marker.
- Changing the CR's `imagePullPolicy` from `Never` to `IfNotPresent` initially
  left the Deployment unchanged. After the comparator fix and manager restart,
  the Deployment changed to `IfNotPresent`, reached generation 2, and rolled out
  a ready replacement pod. This tests that transition, not all managed fields.
- The requested vLLM image index was
  `sha256:befccf0e7e4089aa23926bb6f887a85c0d46c2a99e6de5c5ffdf5cb7957c5b34`.
  The registry reported its linux/amd64 manifest as
  `sha256:6ee133356d49ed24cee03dd0cda3d3980279a11c107ce3e752c89e76540f41c9`.
  Because the local platform-only import reconstructed an OCI index, Kubernetes
  reported image ID `sha256:f176614426646f69b7b5f7c310307337346727e4f872234c92893b66630b0f10`;
  inspecting that local index showed the same linux/amd64 manifest digest.
  This observation is not signature or provenance verification.
- `Compliance-AC-4` and `Compliance-AU-2` reported `Unknown`, and
  `Compliance-SI-7` reported `False` (`IntegrityUnverified`).
- After verification, the test CR was scaled to zero, its pods exited, GPU
  memory returned to the pre-test level, the manager was stopped, and the
  loopback port-forwards were terminated. The isolated cluster and its test
  resources remain available for inspection.

## Limits

This is an isolated operator-managed GPU smoke test, not KAI scheduling or
production qualification. It does not prove TLS/Gateway routing, deployed
operator image provenance, Qwen3 revision qualification, streaming cancellation,
disaster recovery, or network-policy enforcement. Reconciliation of other
managed container fields remains open in L-OP-018.
