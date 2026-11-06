#!/bin/sh
set -eu

export HOME=/work/home TMPDIR=/work/tmp PIP_NO_CACHE_DIR=1 PIP_DISABLE_PIP_VERSION_CHECK=1
export MAX_JOBS=1 NVCC_THREADS=1 TORCH_CUDA_ARCH_LIST=8.9 BUILD_WITH_CUDA=1
export CPATH=/usr/local/lib/python3.12/dist-packages/nvidia/cu13/include
# Local build identity; this is not an upstream release version.
export SETUPTOOLS_SCM_PRETEND_VERSION=0.5.6+ckodex.pr4467

cd /inputs
sha256sum --check --strict checksums.sha256
mkdir -p /work/home /work/tmp /work/src /work/wheels
tar -xzf /inputs/source.tar.gz -C /work/src --strip-components=1 --no-same-owner
cd /work/src
patch --batch --forward --fuzz=0 --dry-run -p1 < /inputs/backport.patch
patch --batch --forward --fuzz=0 -p1 < /inputs/backport.patch
python3 -m venv --system-site-packages /work/venv
/work/venv/bin/python -m pip install --no-index --no-deps \
  /inputs/grpcio_tools-1.78.0-cp312-cp312-manylinux2014_x86_64.manylinux_2_17_x86_64.whl \
  /inputs/wheel-0.45.1-py3-none-any.whl
/work/venv/bin/python -m pip wheel --no-index --no-deps --no-build-isolation --wheel-dir /work/wheels /work/src
/work/venv/bin/python -m pip install --no-index --no-deps /work/wheels/lmcache-*.whl
cd /work
/work/venv/bin/python -c 'import importlib.metadata as m; import lmcache, lmcache.cuda_ops, lmcache.lmcache_native; from lmcache.v1.gpu_connector.gpu_connectors import VLLMPagedMemGPUConnectorV2, VLLMPagedMemGPUConnectorV3; import lmcache.v1.platform.torch_ops.mem_kernels as t; print("version", m.version("lmcache")); print("module", lmcache.__file__); print("native", lmcache.cuda_ops.__file__); print("layout", lmcache.lmcache_native.MemObjKVLayout.SPLIT_KV_2LTD); assert hasattr(lmcache.cuda_ops, "multi_layer_kv_transfer_unilateral"); assert hasattr(lmcache.cuda_ops, "single_layer_kv_transfer_sgl"); assert hasattr(t, "multi_layer_kv_transfer_unilateral")'
cd /work/wheels
sha256sum lmcache-*.whl > SHA256SUMS
