#!/usr/bin/env bash
set -euo pipefail

base_image='vllm/vllm-openai@sha256:d825baf9ce4b1b6bfc5ecc7b79880c7e6748c65a72d697ce3d47607aafa84c6e'
source_sha='6cbc61020b30609edec743a6c81c0a7ed91f1b5c081bbc513d75d0a01ee5b75e'
patch_sha='20fa673532530a42c87b988386f05f921b667b65ed0e67ddfa78a006c91e9859'
grpc_sha='975d4cb48694e20ebd78e1643e5f1cd94cdb6a3d38e677a8e84ae43665aa4790'
wheel_sha='708e7481cc80179af0e556bbf0cc00b8444c7321e2700b8d8580231d13017248'
grpc_name='grpcio_tools-1.78.0-cp312-cp312-manylinux2014_x86_64.manylinux_2_17_x86_64.whl'
wheel_name='wheel-0.45.1-py3-none-any.whl'
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
source_file='' grpc_file='' wheel_file='' output_dir=''

usage() {
  printf '%s\n' 'usage: build-v056-backport.sh --source ARCHIVE --grpc-wheel FILE --wheel-wheel FILE --output-dir NEW_DIRECTORY'
}
while (( $# )); do
  case "$1" in
    --source|--grpc-wheel|--wheel-wheel|--output-dir)
      if (( $# < 2 )) || [[ -z "$2" ]]; then usage >&2; exit 2; fi
      case "$1" in
        --source) source_file="$2" ;;
        --grpc-wheel) grpc_file="$2" ;;
        --wheel-wheel) wheel_file="$2" ;;
        --output-dir) output_dir="$2" ;;
      esac
      shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done
if [[ -z "$source_file" || -z "$grpc_file" || -z "$wheel_file" || -z "$output_dir" ]]; then usage >&2; exit 2; fi
for cmd in docker python3; do command -v "$cmd" >/dev/null || { echo "$cmd is required" >&2; exit 1; }; done
if command -v gtimeout >/dev/null; then timeout_cmd=gtimeout
elif command -v timeout >/dev/null; then timeout_cmd=timeout
else echo 'GNU timeout or gtimeout is required' >&2; exit 1
fi
for file in "$source_file" "$grpc_file" "$wheel_file" "$script_dir/patches/v0.5.6-pr4467-cuda-backport.patch" "$script_dir/build-v056-backport-inner.sh"; do
  [[ -f "$file" && -r "$file" ]] || { echo "input is not a readable regular file: $file" >&2; exit 1; }
done
[[ ! -e "$output_dir" && ! -L "$output_dir" ]] || { echo 'output directory must not exist' >&2; exit 1; }
parent="$(cd -- "$(dirname -- "$output_dir")" && pwd -P)"
output_dir="$parent/$(basename -- "$output_dir")"
[[ "$output_dir" != *','* && "$output_dir" != *$'\n'* ]] || { echo 'output path cannot contain a comma or newline' >&2; exit 1; }
mkdir -m 700 "$output_dir"
mkdir -m 700 "$output_dir/inputs" "$output_dir/work"
cp -- "$source_file" "$output_dir/inputs/source.tar.gz"
cp -- "$grpc_file" "$output_dir/inputs/$grpc_name"
cp -- "$wheel_file" "$output_dir/inputs/$wheel_name"
cp -- "$script_dir/patches/v0.5.6-pr4467-cuda-backport.patch" "$output_dir/inputs/backport.patch"
cp -- "$script_dir/build-v056-backport-inner.sh" "$output_dir/inputs/build-inner.sh"
printf '%s  %s\n' "$source_sha" source.tar.gz "$patch_sha" backport.patch \
  "$grpc_sha" "$grpc_name" "$wheel_sha" "$wheel_name" > "$output_dir/inputs/checksums.sha256"
python3 - "$output_dir/inputs" "$source_sha" "$patch_sha" "$grpc_sha" "$wheel_sha" "$grpc_name" "$wheel_name" <<'PY'
import hashlib
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
for name, expected in zip(("source.tar.gz", "backport.patch", sys.argv[6], sys.argv[7]), sys.argv[2:6]):
    digest = hashlib.sha256((root / name).read_bytes()).hexdigest()
    if digest != expected:
        raise SystemExit(f"SHA-256 mismatch: {name}")
print("Pinned input hashes verified")
PY
chmod a-w "$output_dir/inputs/"*
docker image inspect "$base_image" --format '{{.Id}}' > "$output_dir/base-image-id.txt"
cid_file="$output_dir/container.cid"
stop_container() {
  if [[ -s "$cid_file" ]]; then
    cid="$(cat "$cid_file")"
    if [[ "$cid" =~ ^[0-9a-f]{64}$ ]]; then docker stop --time 5 "$cid" >/dev/null 2>&1 || true; fi
  fi
}
trap stop_container EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
echo 'Building locally; output, log and staged inputs will remain in the new directory.'
set +e
"$timeout_cmd" --kill-after=15s 900s docker run --rm --init --cidfile "$cid_file" \
  --runtime runc --env NVIDIA_VISIBLE_DEVICES=void \
  --network none --memory 4g --memory-swap 4g --cpus 1 --pull never --platform linux/amd64 \
  --user "$(id -u):$(id -g)" \
  --mount "type=bind,src=$output_dir/inputs,dst=/inputs,readonly" \
  --mount "type=bind,src=$output_dir/work,dst=/work" \
  --entrypoint /bin/sh "$base_image" /inputs/build-inner.sh >"$output_dir/build.log" 2>&1
status=$?
set -e
if (( status != 0 )); then
  echo "Build failed or timed out (exit $status); inspect $output_dir/build.log" >&2
  exit "$status"
fi
trap - EXIT INT TERM
echo "Build completed; inspect $output_dir/work/wheels/SHA256SUMS and build.log. GPU correctness was not tested."
