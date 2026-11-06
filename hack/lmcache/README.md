# LMCache v0.5.6 backport build

`build-v056-backport.sh` builds a **local candidate** wheel from the v0.5.6
source archive and the checked-in CUDA backport. It does not fetch inputs,
publish an image, establish artifact provenance, or qualify the result.
The patch is based on an unmerged upstream change and the resulting wheel is
not an official LMCache release. Its local version is `0.5.6+ckodex.pr4467`.

Supply these files from an approved source:

| Input | SHA-256 |
| --- | --- |
| v0.5.6 source archive at `3e11e10812b448142c6cdb177a3058652bd826fa` | `6cbc61020b30609edec743a6c81c0a7ed91f1b5c081bbc513d75d0a01ee5b75e` |
| `grpcio_tools-1.78.0-cp312-cp312-manylinux2014_x86_64.manylinux_2_17_x86_64.whl` | `975d4cb48694e20ebd78e1643e5f1cd94cdb6a3d38e677a8e84ae43665aa4790` |
| `wheel-0.45.1-py3-none-any.whl` | `708e7481cc80179af0e556bbf0cc00b8444c7321e2700b8d8580231d13017248` |

The pinned source archive is available from
`https://codeload.github.com/LMCache/LMCache/tar.gz/3e11e10812b448142c6cdb177a3058652bd826fa`.
The SHA-256 check remains required when acquiring it.

The checked-in patch is also checked against SHA-256
`20fa673532530a42c87b988386f05f921b667b65ed0e67ddfa78a006c91e9859`.
The exact pinned vLLM base image must already exist locally; the command uses
`--pull never`. Docker, Python 3, and GNU `timeout` or `gtimeout` are required.

```bash
bash hack/lmcache/build-v056-backport.sh \
  --source /path/to/v0.5.6-source.tar.gz \
  --grpc-wheel /path/to/grpcio_tools.whl \
  --wheel-wheel /path/to/wheel.whl \
  --output-dir /path/to/new-build-directory
```

The output directory must not exist. The script creates it with mode 0700,
stages the inputs, checks their hashes on the host and again in the container,
checks and applies the patch with zero fuzz, and compiles the wheel. It retains the staged
inputs, and the output directory for diagnosis. Once the container starts, it
also records its compiler log and may leave temporary source, a build
environment, and an output wheel with `SHA256SUMS`. An unsuccessful attempt
leaves its output directory. Select a new directory for each attempt.

The container requests the `runc` runtime, `NVIDIA_VISIBLE_DEVICES=void`, one
CPU, 4 GiB memory with no additional swap, and no network. A 900-second timeout
stops the container. These settings require a compatible Docker host; their
enforcement depends on that host. The wheel targets Linux amd64, Python 3.12, CUDA, and compute capability
8.9. It is not a generic GPU build. A SHA-256 identifies a particular output;
bit-for-bit reproducibility across builds has not been established.

The corrected helper completed against the pinned release archive and patch on
2026-11-02. It produced a wheel with SHA-256
`99055229889aa9a9d059a8c62341e2c2b5ccc3c008c35b7a6e139611cf6034d8` and
loaded its native extension. Its assembled image subsequently passed CUDA
tensor and cache-parity checks on one RTX 3500. Earlier attempts exposed and
corrected missing `git` in the base image and a source-directory import
shadowing error. A later build must still be tested using the exact wheel and
image it produces.

The native import checks exercise packaging and selected symbols only. Before
use, independently test the actual resulting wheel and image on the intended
GPU and runtime, including cold/control/reload output parity, KV data
round trips, recovery and the applicable security boundaries. The installer
does not attest, sign, or promote the result.

The backport changes the native transfer contract for fused KV formats:
callers must specify `mem_obj_kv_layout`; omission now fails closed. The
backported in-tree V2/V3 and CacheBlend callers were updated, but external
callers have not been inventoried. XPU/SYCL, heterogeneous groups, and native
CUDA unilateral transfer have not been qualified by the recorded RTX tests.

To assemble an unsigned candidate image from a newly built wheel, supply its
actual 64-character lowercase SHA-256 from `SHA256SUMS`:

```bash
dagger call build-lmcache-v-056-backport-candidate \
  --wheel /path/to/lmcache-0.5.6+ckodex.pr4467-cp312-cp312-linux_x86_64.whl \
  --wheel-sha-256 "$WHEEL_SHA256" export --path /path/to/candidate.oci.tar
```

This checks that the supplied wheel matches the supplied digest; a caller's
digest is not proof of the source, identity, or correctness of the wheel. A
newly built wheel or resulting image requires its own verification. For the
specific wheel used in the recorded GPU experiment, the separate
`build-lmcache-v-056-backport-diagnostic` function requires that wheel's fixed
digest instead.
