# Documentation

Welcome to the CKodex KServe LLM Operator documentation suite.

The documentation is organized using the [Diátaxis framework](https://diataxis.fr/), partitioning content across four canonical quadrants based on intent and audience needs.

---

## The Diátaxis documentation matrix

| | Learning-oriented (Tutorials) | Task-oriented (How-to guides) |
|---|---|---|
| **Practical steps** | **[Tutorials](tutorials/getting-started.md)**<br>Guided lessons taking you by the hand to run your first model in 5 minutes. | **[How-to guides](how-to/deploy-vllm-service.md)**<br>Actionable step-by-step recipes to solve specific operational tasks and production problems. |
| | **Information-oriented (Reference)** | **Understanding-oriented (Explanation)** |
| **Theoretical background** | **[Reference](reference/crds.md)**<br>Austere, exact technical specifications of CRDs, environment variables, engine contracts, and hardware profiles. | **[Explanation](explanation/overview.md)**<br>Architectural rationale, the Four Spaces, zero-trust security design, and continuous compliance. |

---

## 1. Tutorials (learning-oriented)

Follow guided lessons designed for newcomers to gain practical experience without cognitive overload:

- **[Getting started with the operator](tutorials/getting-started.md)**: Install the operator and deploy your first specimen LLM in under five minutes.

---

## 2. How-to guides (task- and problem-oriented)

Solve specific production tasks and operational challenges:

- **[Deploy a vLLM inference service](how-to/deploy-vllm-service.md)**: Deploy vLLM v0.33.0 workloads with GPU acceleration and tensor parallelism.
- **[Deploy multimodal foundation models with OCR and SAM 3](how-to/deploy-multimodal-ocr-sam3.md)**: Run Baidu Unlimited-OCR-2 and Meta SAM 3 for document parsing and concept segmentation.
- **[Author and bind UTCP tools](how-to/author-utcp-tools.md)**: Declare UTCP v1.0.1 manuals, package immutable AIPacks, and bind tools to autonomous agents.
- **[Configure LMCache prefix caching](how-to/configure-lmcache-prefix-caching.md)**: Reduce time-to-first-token latency by up to 85% with shared-memory KV caching.
- **[Upgrade the operator to v0.20.0](how-to/upgrade-to-v0.20.0.md)**: Execute a zero-downtime operator and workload upgrade.
- **[Troubleshoot inference workload failures](how-to/troubleshoot-workload-failures.md)**: Follow a diagnostic decision tree to resolve CUDA OOM, 503 errors, and initialization crashes.
- **[Deploy the operator in an air-gapped environment](how-to/airgap-deployment.md)**: Configure disconnected operation across Sovereignty Tiers 1 through 5.
- **[Recover from cluster and node failures](how-to/disaster-recovery.md)**: Restore the control plane, CRDs, and model PVCs following catastrophic failures.

---

## 3. Reference (information-oriented)

Consult complete, austere technical specifications:

- **[Custom resource definitions reference](reference/crds.md)**: Detailed schema reference for `LLMInferenceService`, `AIPack`, `MultimodalInferenceService`, and `Agent`.
- **[Operator configuration defaults](reference/operator-defaults.md)**: Environment variables, resource request defaults, and reconciler diff normalization rules.
- **[Inference engine runtime contract](reference/engine-contract.md)**: Conformance tiers, capability matrices, and runtime adapter requirements for vLLM, SGLang, and llama.cpp.
- **[Hardware accelerator profiles](reference/accelerator-profiles.md)**: Profiles and resource specs for NVIDIA, TPU v6e, Trainium 2, Gaudi 3, and Apple Silicon.
- **[CLI and YAML cheat sheet](reference/cheat-sheet.md)**: Ready-to-use kubectl commands and minimal production YAML manifests.
- **[API deprecation policy](reference/api-deprecation-policy.md)**: Lifecycle timeline, dual-serve window, and v1alpha2 to v1 migration rules.

---

## 4. Explanation (understanding-oriented)

Deepen your understanding of system design, security, and governance:

- **[Architecture and the Four Spaces](explanation/overview.md)**: The Four Spaces, the 9-state lifecycle machine, and 6D typed state vectors.
- **[Security architecture and zero-trust governance](explanation/security-architecture.md)**: Standing authority, capability leases, SPIRE workload identity, and route fencing.
- **[Compliance as code with OSCAL and Lula](explanation/compliance-oscal.md)**: NIST SP 800-53 Rev 5 control mappings and automated Lula CI evaluation.
- **[Component catalog and data flows](explanation/components.md)**: Detailed component interactions and sequence diagrams for deployment, inference, and fencing.

---

## Historical archive

Historical point-in-time documents, release verification records, and migration plans are preserved in the **[archive/](archive/)** directory:

- [Release verification](archive/release-verification.md)
- [Historical handoff ledger (2026-08-24)](archive/session-handoff-2026-08-24.md)
- [Remediation plan](archive/remediation-plan.md)
- [Gemma 4 performance report](archive/GEMMA_4_PERFORMANCE_REPORT.md)
- [Marketing kit](archive/MARKETING_KIT.md)

---

## Recommended reading paths

### For model scientists
1. [Getting started with the operator](tutorials/getting-started.md)
2. [Deploy a vLLM inference service](how-to/deploy-vllm-service.md)
3. [Deploy multimodal models](how-to/deploy-multimodal-ocr-sam3.md)
4. [Author and bind UTCP tools](how-to/author-utcp-tools.md)
5. [Custom resource definitions reference](reference/crds.md)

### For platform engineers and SREs
1. [Architecture and the Four Spaces](explanation/overview.md)
2. [Operator configuration defaults](reference/operator-defaults.md)
3. [Configure LMCache prefix caching](how-to/configure-lmcache-prefix-caching.md)
4. [Upgrade the operator to v0.20.0](how-to/upgrade-to-v0.20.0.md)
5. [Troubleshoot inference workload failures](how-to/troubleshoot-workload-failures.md)
6. [Recover from cluster and node failures](how-to/disaster-recovery.md)

### For security and compliance auditors
1. [Security architecture and zero-trust governance](explanation/security-architecture.md)
2. [Compliance as code with OSCAL and Lula](explanation/compliance-oscal.md)
3. [Deploy the operator in an air-gapped environment](how-to/airgap-deployment.md)
