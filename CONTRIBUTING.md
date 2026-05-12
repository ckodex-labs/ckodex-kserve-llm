# Contributing to CKodex KServe LLM Operator

First off, thank you for considering contributing to the CKodex KServe LLM Operator! It's people like you that make the Kube-AI ecosystem such a great place.

## Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## How Can I Contribute?

### Reporting Bugs
* **Check the existing issues** to see if the bug has already been reported.
* **Open a new issue** with a clear title and description. Include as much relevant information as possible, such as:
    * Kubernetes version
    * GPU type and driver version
    * vLLM version
    * Operator logs
    * A minimal reproducible example (YAML spec)

### Suggesting Enhancements
* **Open an issue** with the tag `enhancement`.
* Provide a clear use case and describe how the feature would work.

### Pull Requests
* **Fork the repository** and create your branch from `main`.
* **Install dependencies**: `make install-deps`
* **Run tests**: `make test` (We aim for ≥80% coverage).
* **Lint your code**: `make lint`.
* **Ensure all commits are signed** (DCO). We use the Developer Certificate of Origin to track ownership.
* **Keep PRs focused**: One feature or bug fix per PR.
* **Hardened Contribution Rules**:
    * **Supply-Chain Enforcement**: All code must be pushed to SLSA-compatible build paths; floating tags are prohibited.
    * **Compliance-as-Code**: PRs introducing security controls (RBAC, NetworkPolicy, DPI) MUST include a corresponding **Lula Validation** in the `lula/` directory.
    * **OIS Instrumentation**: Any new data-plane features must emit **Open Inference Signals (OIS) v0.1** telemetry via OpenTelemetry.

## Development Setup

1. **Clone the repo**:
   ```bash
   git clone https://github.com/ckodex-labs/ckodex-kserve-llm.git
   cd ckodex-kserve-llm
   ```

2. **Generate code/CRDs**:
   ```bash
   make generate
   make manifests
   ```

3. **Run locally** (using a `kind` cluster):
   ```bash
   make install
   make run
   ```

## Developer Certificate of Origin (DCO)

All commits must include a `Signed-off-by` line in the commit message (`git commit -s`). This indicates that you have the right to submit the code under the Apache 2.0 license.

---
*Questions? Join us on our community Slack or start a Discussion on GitHub!*
