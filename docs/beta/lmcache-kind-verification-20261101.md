# LMCache kind verification — 2026-11-01

## Verdict

**FAILED / PARTIALLY VERIFIED.** Built-in vLLM prefix caching produced observed
hits. In-process LMCache stored and reloaded KV tokens, but the tested
cold-versus-reloaded outputs did not match. The multiprocess cache engine became
ready, but its GPU client failed during CUDA IPC mapping. Neither LMCache path
is qualified for this configuration.

## Environment

- Dedicated `ckodex-gpu-validation` kind cluster on `192.168.2.198`, Kubernetes
  `v1.35.8`, one NVIDIA RTX 3500 Ada Generation Laptop GPU, driver `580.178.04`.
- Helm-installed CKodex operator built from the current dirty worktree; cert-manager
  and the conversion webhook were enabled. A v1alpha2 KV-cache configuration
  survived a v1 storage/readback round trip through the webhook.
- vLLM image index
  `sha256:befccf0e7e4089aa23926bb6f887a85c0d46c2a99e6de5c5ffdf5cb7957c5b34`,
  linux/amd64 manifest
  `sha256:6ee133356d49ed24cee03dd0cda3d3980279a11c107ce3e752c89e76540f41c9`;
  vLLM `0.32.0`, LMCache `0.5.6-g3e11e108`, CUDA `13.0`.
- Local Qwen2 0.5B model weights SHA-256
  `8741f6ae8b48c6566466bac576d3bd9f7ec46e1086c0a5dc7330c3ccf64a57c4`;
  tokenizer JSON SHA-256
  `f7c9b2dba4a296b1aa76c16a34b8225c0c118978400d4bb66bff0902d702f5b8`.
  These digests identify local files; their provenance was not verified.

### Release package comparison — 2026-11-02

The official v0.5.6 Git tag resolved to
`3e11e10812b448142c6cdb177a3058652bd826fa`. The published CPython 3.12
Linux x86-64 wheel downloaded from the [GitHub release](https://github.com/LMCache/LMCache/releases/tag/v0.5.6)
had SHA-256 `c7892944ca91ec965c3448b2e30e0869f86a2499bdd01b70eb988e3436d71e4e`,
matching both GitHub release and [PyPI metadata](https://pypi.org/pypi/lmcache/0.5.6/json).
All 892 regular `lmcache/` wheel files, including four native shared libraries,
matched the installed files in the running multiprocess engine and in a
network-disabled container from the exact vLLM base image. Neither comparison
found missing, mismatched, or extra non-bytecode files under `lmcache/`.

These are content comparisons, not proof that either image was built from that
wheel. They exclude `.dist-info`, Python bytecode, dependencies, and other image
layers, and follow symlinks. Image signatures, build provenance and custody
remain unverified.

- [Engine comparison](verification-artifacts/lmcache-release-wheel-engine-20261102.json):
  `7a5650a67b513ad9da878b7a84666d9f04bd3abb8e60b5b4e2ecd0276d524c62`.
- [vLLM base comparison](verification-artifacts/lmcache-release-wheel-vllm-20261102.json):
  `c26da3f8cac7e1e19fd2049240f4b2e44a4d06b622c11493a7b1f304c72bdedf`.

## Prefix and in-process cache

With built-in prefix caching, two identical 519-token requests increased the
vLLM prefix-hit counter from 0 to 512 on the second request. This does not by
itself establish LMCache retrieval.

For the LMCache experiment, the operator rendered `LMCacheConnectorV1`, 256-token
chunks and a 2 GiB CPU cache. GPU blocks were limited to 80 to force eviction;
requests were serial, `temperature=0`, `seed=0`, and
`VLLM_BATCH_INVARIANT=1` was configured. After four distinct prompts, the
original prompt was sent again. Its 519 prompt token IDs matched the cold run,
and the external-cache hit counter increased by 512. LMCache logs reported
retrieving all 512 required tokens.

The no-LMCache control produced identical generated token IDs before and after
eviction. Its cold result matched the LMCache cold result. The LMCache reloaded
result differed:

- Cold/control: `[362,3974,13876,38835,34208,916,279,15678]`.
- Reloaded: `[330,785,3974,13876,38835,34208,916,279]`.

The same text divergence was also observed in an earlier cold-cache run. A
later warm-cache comparison matched. This is a failed parity check, not proof
of a particular corruption mechanism. The similar upstream
[LMCache issue #4463](https://github.com/LMCache/LMCache/issues/4463) is an
investigation lead, not a confirmed diagnosis. The active workload was returned
to a configuration without the LMCache connector.

The installed connector classified the tested KV tensors as
`NL_X_NB_BS_NH_CS`; its format implementation identifies this as fused and
packed K/V, the layout described by the upstream issue. The proposed upstream
[fix](https://github.com/LMCache/LMCache/pull/4467) was open when checked.
This correspondence does not establish that the reported mechanism caused the
observed divergence: no direct KV-byte round trip was collected.

An experimental build of the unmerged PR head
`e05c2ea9781b4021081f34dfb982189ec0fabc15` was prepared against the same
base image. Its source archive SHA-256 is
`644ef364f3eb9aa4f5a0362e00435f6d141b6a831a90ae09bd37e448982b7780`, and its
locally built wheel SHA-256 is
`4c936485327e1152667290fe83cd27f08379c7edbf628272a6d011d5fcbbd21b`.
The first build failed because the compiler did not search the installed
NVIDIA headers; a bounded retry with their existing include directory compiled
the wheel and imported the native extension without a GPU. This is an unsigned
development build, not an upstream release or proof of cache correctness.

The first GPU startup of this build failed: vLLM 0.32's built-in
`LMCacheConnectorV1` imported the old `lmcache.integration.vllm` adapter
path, while the development build places that adapter under
`lmcache.inprocess.integration.vllm`. The failure log is retained on the test
host at `/tmp/ckodex-gpu-kind.E4bxFLrj/lmcache-pr4467-startup-failure.log`.
For a second diagnostic, the workload explicitly selected the development
build's `LMCacheConnectorV1Dynamic` through vLLM's connector-module API; this
did not modify the operator's default connector configuration.

That workload started on the GPU and, on the original 519-token test, retrieved
512 external-cache tokens and matched the cold/control generated token IDs.
Three additional fresh prompt cases each observed 480 external-cache hits and
matched their cold and reloaded token IDs, with distinct request IDs. Logs
identified the fused `NL_X_NB_BS_NH_CS` format and actual retrieval. The
original collector's output matches an earlier artifact byte for byte because
it omits request IDs and timestamps; the three-case artifact retains request
IDs. The diagnostic image is local and unsigned, and this does not establish a
direct KV-byte round trip, broad correctness, production recovery, or a fix
in the released default connector. The test workload was restored to the pinned
base image with zero replicas afterward.

- [Original parity case](verification-artifacts/lmcache-inprocess-pr4467-20261101.json):
  `c7d3bb9c8768144be039b31d6426d408239ae1c17c25df2b117b971bab94f4ed`.
- [Three new prompt cases](verification-artifacts/lmcache-inprocess-pr4467-repeat-20261101.json):
  `a2caaefec3a6d0ac9653e475814259c9dc5dff27d87758ca6898de48716fff8d`.

On 2026-11-02 the same development wheel was installed into the pinned base
image using `dagger call build-lmcache-pr-4467-diagnostic`, exported and loaded
locally into the isolated kind node. The resulting image index digest was
`sha256:8d8789cf84141c7cac9ce8dcd5d3409d084810b5f3506ca250a6514ed614deae`.
The node reported that exact image ID in the vLLM container. With the dynamic
connector explicitly selected, the repository parity diagnostic passed seven
fresh cases, each with an external-hit increase and matching cold/reloaded
token IDs; the runtime logs recorded seven 512-token retrieves. The test
workload remains configured with this local diagnostic image at zero replicas.

This image contains the entire unmerged PR-head development wheel, which is
122 commits ahead of v0.5.6; it is not a source backport of the PR onto the
release. The Dagger function validates the input wheel identity and installs it
without resolving dependencies; it does not build that wheel from source or
establish its provenance, signature, or portability beyond the tested GPU.

- [Dagger image seven-case diagnostic](verification-artifacts/lmcache-inprocess-dagger-pr4467-20261102.json):
  `44bd300cab042959434644027e75e7bd9aad02706012d76e8e0183ce8cbeada2`.

### v0.5.6 source backport — 2026-11-02

The upstream PR does not apply directly to release v0.5.6. Its 23 changed
paths were backported to the release source, retaining release-specific
unilateral and SGL APIs. The resulting [source patch](../../hack/lmcache/patches/v0.5.6-pr4467-cuda-backport.patch)
has SHA-256 `20fa673532530a42c87b988386f05f921b667b65ed0e67ddfa78a006c91e9859`
and passed `git apply --check` against a fresh v0.5.6 extraction. It replaces
new upstream test doubles with real Torch and production objects.

A bounded, network-free, one-CPU native build produced an unsigned local
`0.5.6+ckodex.pr4467` wheel, SHA-256
`b00f42303684cf7d572cc356b39d82c9120686ded916c27820106585a517a6d2`.
The production source in that build was byte-identical to the final patch;
the build used an earlier copy of two test files that were subsequently changed
to use real objects. Against the wheel, 108 real-Torch CPU tests passed and
17 CUDA cases were skipped in the CPU-only environment. Separate CPU tensor
checks exercised unilateral K/V and MLA transfer in both directions.

An independent comparison of the pristine-release-plus-final-patch build tree
against the earlier GPU-tested source archive found all 943 common archived
production and build-source files identical, with none missing. The generated
version and protobuf Python files also matched the tested wheel for all 33
files checked. Native binary and full-wheel reproducibility were not
established by those comparisons. Exactly two test files differed, as described
above; the upstream blend test retains existing mock scaffolding and was not
counted as integration evidence.

The checked-in clean-build helper was corrected after an import check ran from
the source directory and then completed end to end from the original release
archive and final patch. It produced and imported a wheel with SHA-256
`99055229889aa9a9d059a8c62341e2c2b5ccc3c008c35b7a6e139611cf6034d8`.
This differs from the first GPU-tested wheel, so it was verified separately.
The helper requested `runc`, no network or NVIDIA device visibility, one CPU,
and 4 GiB RAM without extra swap; those settings were inspected on the test
host. The Dagger-assembled image index was
`sha256:bd4aebc0ea10985f5efd013a71cd23258b3e3886ad0b6fb0401d17c1cf93afe0`.
With that image on the RTX 3500, 225 direct tensor checks passed, the original
failing prompt matched after a 512-token external reload, and seven additional
cases each observed external hits and matching cold/reloaded token IDs. The
operator used its normal `LMCacheConnectorV1`; logs corroborated eight cache
retrievals. The isolated workload retains this image at zero replicas.

- [Rebuilt wheel CUDA checks](verification-artifacts/lmcache-v056-backport-v4-gpu-oracles-20261102.log):
  `5aca96f12c9cff68fa62b4e81d11841ce4328362eb0ae1237d4aeadc0bf85e11`.
- [Rebuilt wheel seven-case parity](verification-artifacts/lmcache-v056-backport-v4-seven-20261102.json):
  `70625596ef969facfc6fb7aa391d7b1293a796d7ba11789da322388ac05e9abd`.
- [Rebuilt wheel original case](verification-artifacts/lmcache-v056-backport-v4-original-20261102.json):
  `85fee67178b022a81c4a3d8cedbe2abe3858e50285b9bd4e3dbfac8726787b8f`.

The wheel was installed into the pinned vLLM base with Dagger. The resulting
local image index digest is
`sha256:9e9afb546901bb542211fbe5d92c41d918992bcb274ee51e82e6598502368af2`.
On the isolated RTX 3500 kind node, **225 direct tensor tests passed** with a
CUDA availability preflight, including fused layout, split and packed memory,
host-to-device and device-to-host paths, and padded-stride cases. The operator's
original `LMCacheConnectorV1` then passed the previously failing 519-token
case and seven fresh cache-reload cases, with observed external hits and matching
token IDs. Logs confirmed the fused format and actual retrievals. The workload
was then scaled to zero before testing the separately rebuilt image above.

These results are limited to this one GPU, model, CUDA image, and test suite.
The local image and wheel are unsigned; XPU/SYCL, production artifact
provenance, multi-node behavior, and full recovery remain unverified. The
backport is not an upstream v0.5.6 release or a merged upstream patch.

- [CUDA tensor test log](verification-artifacts/lmcache-v056-backport-gpu-oracles-20261102.log):
  `c02757e9c2e2beba8e38df48d312203b891bfbb69d39ac9570255a86b73f6429`.
- [Previously failing case](verification-artifacts/lmcache-v056-backport-original-20261102.json):
  `c7d3bb9c8768144be039b31d6426d408239ae1c17c25df2b117b971bab94f4ed`.
- [Seven fresh cases](verification-artifacts/lmcache-v056-backport-seven-20261102.json):
  `7eec75cfea731bac03efb702c28d2181593b7996ef88f332d63ac636b099c16f`.

Raw diagnostic results are stored locally with these SHA-256 digests:

- [`lmcache-control-20261101.json`](verification-artifacts/lmcache-control-20261101.json):
  `7cff7a695e0b9bab87d41996705cc62307366d3f17541bf52d9c1b522f6e5825`.
- [`lmcache-reload-20261101.json`](verification-artifacts/lmcache-reload-20261101.json):
  `33965efbc49a8805ff4d8840a9ceb6e0936b4fe6e6fe12641b5fa33e8da08d09`.

The control collector initially exited nonzero because it looked for prompt
token IDs at the response top level; the IDs are in each `choice`. The saved
responses were subsequently compared using that actual field. These files are
unsigned test artifacts, not attestations.

## Multiprocess cache

The pinned operator-v0.1.1 installer created a 2 GiB LMCacheEngine. Its
`ConfigValid`, `Available`, and `AllInstancesReady` conditions were `True` at
the current generation, and its connection ConfigMap contained the expected
`kv-transfer-config.json` and `LMCacheMPConnector` settings. The NVIDIA
RuntimeClass was configured only inside the dedicated kind node and verified by
a real `nvidia-smi` pod. The engine image was pinned by registry digest, but its
signature and provenance were not verified.

An operator-created client pod used `hostIPC` and the real connection ConfigMap.
The client and engine saw the same GPU UUID and IPC namespace, and both reported
PyTorch `2.13.0+cu130` and LMCache `0.5.6`. During registration the engine
reported `CUDA error: mapping of buffer object failed`; the client did not
become ready. The cause remains under investigation. Readiness of the engine
alone is not evidence of successful GPU IPC, KV retrieval, parity, or recovery.

A direct CUDA IPC diagnostic passed exact 4,096-element tensor equality between
two processes in one container. Across containers and across pods it failed
when the producer was UID/GID 65532 and the consumer was UID/GID 0, including
when the root consumer's effective capabilities were dropped. It passed across
pods when both used UID/GID 65532. The producer used the GPU device-plugin CDI
path and the consumer used the NVIDIA RuntimeClass. These observations identify
a UID boundary as a strong lead; they do not prove the driver's underlying
failure mechanism. PyTorch also emitted a producer-lifetime warning after the
tests; resource-lifetime correctness was not established by this diagnostic.

For a **temporary diagnostic only**, the LMCache operator was paused and its
DaemonSet was overridden to run the server as UID/GID 65532, matching the vLLM
client. The real multiprocess connector then registered the GPU cache, stored
and reloaded 512 tokens after eviction, and generated the same token IDs as the
no-cache control for the tested prompt. This was one parity case, with no
failover or recovery proof. The pinned upstream installer does not expose that
UID setting; the override is not a supported deployment configuration.
After the test the client was scaled to zero and the LMCache operator was
restored. It reconciled the server back to its upstream root configuration;
the temporary override is not active.

A later server-side admission check against a rebuilt CKodex operator accepted
`kv_both` and rejected `kv_producer` and nonempty `extraConfig` for the
multiprocess configuration. The engine's checked connection configuration uses
`kv_both`, so this guard prevents those settings from being silently ignored.
A second rebuilt operator also accepted pipeline parallelism 1 and rejected 2,
because the KServe multi-node path does not configure this cache integration.
The guard does not authenticate the ConfigMap or inspect arbitrary opaque shell
code.

- [`lmcache-multiprocess-sameuid-20261101.json`](verification-artifacts/lmcache-multiprocess-sameuid-20261101.json):
  `c7d3bb9c8768144be039b31d6426d408239ae1c17c25df2b117b971bab94f4ed`.

### Operator-reconciled image retry

A locally built derivative of the pinned engine image was tested through the
upstream operator's image field while its controller remained active. The
derivative retained the base filesystem layers and set the image user to
`65532:65532` and `HOME=/tmp`. Its local OCI index digest is
`sha256:ff34d0580df06ea667a0556d67ae874af09cd9ad3d4456003f821c55367c353a`.
The running engine reported UID 65532 and GID 0; this experiment establishes
neither image provenance nor removal of the upstream privileged/host-IPC
requirements. The local image is not a published release artifact.

The real client registered its GPU cache and, after eviction, retrieved 512
tokens with the same prompt and generated token IDs as its cold request. After
the engine was restarted, a second run registered the cache again, stored and
retrieved 512 tokens, and matched generated IDs. The latter run's external hit
counter increased from 512 to 1024. The fresh engine logs independently record
registration and retrieval. The first new result serialized identically to the
earlier same-UID result because the collector omits request IDs and timestamps;
the restart result has different cumulative counters and a distinct digest.
These checks cover one prompt and one GPU, not general accuracy, in-flight
recovery, multi-node behavior, or production qualification.

- [Post-restart result](verification-artifacts/lmcache-multiprocess-derivative-recovery-20261101.json):
  `f3eeb191133f0e45cafdda35dad3d1ca4e206cb4878d70d52679628ee539f07c`.

### Additional prompts and an in-flight restart

Three fresh prompt cases each observed an increase of 480 external-cache hits
after eviction, used distinct completion request IDs, and matched prompt and
generated token IDs between cold and reloaded requests. A separate streaming
request triggered a DaemonSet rollout after its first generated token and
finished 200 tokens with HTTP 200 and a complete stream. The replacement pod
was created before the stream ended. This does not establish that the stream
needed cache access while the engine was unavailable, or that cached contents
survived the restart.

After rollout, engine logs recorded renewed GPU-cache registration. Three new
prompt cases again produced external-cache hits and matching generated IDs.
These six cases are bounded observations on the same GPU, model, engine build,
and cache topology, not a general correctness or failover guarantee.

- [Pre-restart cases](verification-artifacts/lmcache-multiprocess-repeat-20261101.json):
  `d4ee6fb124af017e5f791255c2278934d12bcb0243eba5d300f9a6b7b200ce71`.
- [Streaming interruption](verification-artifacts/lmcache-multiprocess-interruption-20261101.json):
  `b73e2735932230f0b0a04dfe0b047fdcbd713273cf1e43c72bfc552e79fbbfe4`.
- [Post-restart cases](verification-artifacts/lmcache-multiprocess-interruption-recovery-20261101.json):
  `6fd811b51837dc6dc08ce5638fa86565d42861b171ee2f9105d9fd36f002a40a`.

The revised setup script was subsequently applied in this isolated cluster
with the local derivative's repository, tag, and digest. The upstream operator
and engine both reported ready at the current generation. This verifies that
the script accepts that explicit local image; it does not sign, publish, or
establish trust in it. The `kubectl` executable used for this application was
v1.31 and warned of version skew against the v1.35 server.

On 2026-11-02 the repository's new [parity diagnostic](../../hack/kv-cache/README.md)
was run against the isolated multiprocess workload for seven fresh cases. Each
cold request left the external-hit counter unchanged, each reload increased it
by 480, request IDs were distinct, and prompt and generated token IDs matched.
Engine logs independently recorded seven 512-token retrievals during the run.
The evidence is a service-wide metric plus separate engine logs; it is not a
per-request signed receipt or a no-LMCache control. The workload was scaled to
zero afterward.

- [Seven-case diagnostic](verification-artifacts/lmcache-multiprocess-parity-harness-20261102.json):
  `dac2301769209cd32afc2f8a4b511c41bc47ee5579daa67ca42cb6a77a4a3f28`.

As a negative check of the diagnostic, the same model was run without an
LMCache connector. The diagnostic exited 1 and recorded `fail` because the
external-hit counter remained zero on reload. This verifies that this observed
absence of a hit is not reported as success; it is not a complete assessment
of every failure mode.

- [No-external-cache negative check](verification-artifacts/lmcache-parity-harness-no-cache-20261102.json):
  `2e50e9037602b088d5c179e5a0193fe716582359712df9e57c350d4ec8ab5d5a`.

## Privacy and remaining scope

The upstream engine initialized anonymous usage reporting on its first start,
before the opt-out was set. Whether a report reached the remote service is
unknown. Both `LMCACHE_TRACK_USAGE=false` and `DO_NOT_TRACK=1` were subsequently
applied; the installed package's source confirms those values disable its usage
tracking path. No claim is made about previously sent data.

Gateway API, Envoy Gateway, Envoy AI Gateway, and inference-extension resources
were installed in the isolated cluster. Authenticated TLS routing and scheduler
failure and recovery were verified as described in the
[gateway verification record](gateway-scheduler-kind-20261101.md). Multiprocess
retrieval was observed with both same-UID diagnostic profiles described above.
Supported multiprocess deployment, failover, multi-node transfer, and production
qualification remain unverified.
