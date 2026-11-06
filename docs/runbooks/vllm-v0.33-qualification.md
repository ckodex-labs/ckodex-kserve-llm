# vLLM v0.33.0 Runtime Acceptance

The operator default and vLLM adapter contract now use the same digest-pinned
v0.33.0 release. This runbook records the additional hardware, driver, TLS,
authentication, and recovery evidence needed to accept that release on a target
cluster. The image is active by request; its GPU runtime behavior is not yet
verified by this checkout.

The settings below follow the [vLLM configuration reference](https://docs.vllm.ai/en/stable/api/vllm/config/),
the [v0.33.0 CLI reference](https://docs.vllm.ai/en/v0.33.0/cli/serve/), and the
[vLLM security guide](https://docs.vllm.ai/en/latest/usage/security/). The
deployment controls address [OWASP API8:2023 Security Misconfiguration](https://owasp.org/API-Security/editions/2023/en/0xa8-security-misconfiguration/).

## Image identity

The sample at
[`config/samples/llminferenceservice_vllm_033_qualification.yaml`](../../config/samples/llminferenceservice_vllm_033_qualification.yaml)
uses the upstream CUDA 13.0 multi-architecture image index
`vllm/vllm-openai:v0.33.0@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e`.
The Docker Registry reported this digest on 2026-11-05; the index selects
`linux/amd64` or `linux/arm64` child manifests. Note that CUDA 12.9 has been
dropped upstream; release wheels and images are CUDA 13.0 only. Verify the node
driver and CUDA compatibility before scheduling the workload.

The qualification model is `Qwen/Qwen3-0.6B` at Hub revision
`c1899de289a04d12100db370d81485cdf75e47ca`. It is text-only, so the sample does
not enable remote multimodal media loading.

## Security defaults

| Setting | Upstream default or risk | Operator baseline |
|---|---|---|
| CORS origins | Empty in v0.33.0 | Explicit JSON `[]`; wildcard values are rejected |
| CORS methods and headers | Wildcard lists | Finite `GET`, `POST` methods and an explicit header list |
| CORS credentials | `False` | Explicitly disabled |
| FastAPI docs | Enabled | Disabled |
| `trust_remote_code` | `False` | Explicit `--no-trust-remote-code`; qualification validation rejects enabling it |
| Local media path | Empty; enabling it permits API clients to read server files | Remains empty |
| Allowed media domains | `None`, which does not restrict remote media hosts | This acceptance sample uses a text-only model. For multimodal serving, require explicit domains and network egress review |
| API key | Unset | Kubernetes Secret reference; the route also blocks vLLM endpoints outside the allowlist |
| Request sequence fan-out | `VLLM_MAX_N_SEQUENCES=16384` | `128`, within vLLM's recommended public-serving range |
| Media redirects | May be followed | `VLLM_MEDIA_URL_ALLOW_REDIRECTS=0` |
| Development endpoints | Enabled only by `VLLM_SERVER_DEV_MODE=1` | Explicit `0`; shared validation rejects nonzero and Secret-backed values |

The adapter applies safe CORS, docs, remote-code, request-cap, development-mode,
and redirect defaults to vLLM workloads. Shared validation rejects wildcard
CORS values, local media access, enabling development endpoints or redirects,
and sequence limits above 128. In v0.33.0, Medusa and MLP speculators are removed
and `--mamba-cache-mode` is deprecated; the adapter rejects these options at
admission. The explicit acceptance sample also requires a Secret-backed API key,
rejects container command overrides, imported environment variables, and external
CLI configuration, and requires an existing TLS Gateway. That sample uses the same
v0.33.0 digest as the default; the qualification annotation selects these additional
acceptance checks, not a different vLLM version.

The operator's HTTPRoute exposes only health and inference paths with their
required methods. It removes the `/version` and `/server_info` routes, applies
`Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, and
`Referrer-Policy: no-referrer`, and constrains LoRA sandbox routes to that same
path/method set. This does not authenticate or encrypt traffic by itself.

Before applying the sample:

1. Provision `vllm-api-credentials` in the service namespace using the cluster's
   secret manager. Do not put an API key in the manifest.
2. Provide an existing Gateway named `vllm-qualification-gateway` with TLS,
   client authentication, rate limits, and access logging configured. The
   sample intentionally does not create a plaintext Gateway.
3. Restrict direct access to the inference Service to authorized in-cluster
   clients and the Gateway. vLLM's API key protects documented API prefixes but
   does not cover every endpoint, so do not expose the pod or Service directly.
4. Confirm the selected CUDA 13.0 image is compatible with the target node driver.

For browser clients, keep CORS disabled unless a specific origin is approved.
Set `--allowed-origins` to that exact origin list, limit `--allowed-methods` to
`GET`/`POST`, keep `--allowed-headers` finite, and leave credentials disabled;
pass each list as a JSON string array and wildcard CORS values are rejected.
The operator keeps CORS origins empty. For multimodal models, set
`--allowed-media-domains` to the required source domains; the upstream default
`None` does not restrict media URLs. Also keep `VLLM_MEDIA_URL_ALLOW_REDIRECTS=0`
so a permitted URL cannot redirect to an unapproved host.

## Qualification procedure

1. Record GPU model, driver version, Kubernetes version, runtime class, and the
   resolved image ID on the selected node.
2. Apply the same-image acceptance sample after the Secret and Gateway prerequisites exist.
3. Verify the Deployment image is the exact pinned reference and the Pod's
   `status.containerStatuses[].imageID` resolves to the selected architecture's
   child manifest.
4. Verify `/health`, `/metrics`, and model readiness. A request to
   `/v1/chat/completions` without a key must return `401` or `403`; the same
   request with the Secret-backed key must succeed.
5. Verify the Gateway rejects unapproved paths and methods, including
   `/version`, `/server_info`, `/invocations`, `/pause`, `/collective_rpc`,
   `HEAD`, `PUT`, and `TRACE`. `OPTIONS` is routed only on approved API paths
   for CORS preflight; verify it returns no `Access-Control-Allow-Origin` for
   unapproved origins. Confirm inference responses carry the security headers
   above and are not cached.
6. Verify cross-origin requests receive no permissive CORS response. If a
   specific browser origin is configured, test that origin and a different
   origin, and confirm only the configured methods and headers are allowed.
7. Test request sequence bounds, near-context `max_tokens`, cancellation,
   Responses API streaming, model startup, and recovery after pod restart.
8. Save logs, request/response metadata without prompts, workload imageID,
   model revision, hardware/driver details, and measured outcomes as a dated
   evidence record. Do not mark the runtime accepted from unit tests or
   release notes alone.

## Current evidence state

The image index digest and Hub model revision were observed on 2026-11-05.
Controller tests can verify image selection and route construction. This
checkout does not contain a v0.33.0 GPU startup, inference, security-policy, or
recovery receipt; those remain required before claiming hardware/runtime
acceptance.
