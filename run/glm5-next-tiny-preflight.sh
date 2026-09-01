#!/usr/bin/env bash
set -euo pipefail

MODEL_ID="inference-optimization/GLM-5.3-Flash-0.1B-A0.1B"
MODEL_REVISION="8311399447eba9c9b215e3209ab6f25e59c7d21e"
PYTHON_BIN="${PYTHON_BIN:-python3}"

echo "==> checking local glm5_next preflight support"
"$PYTHON_BIN" - "$MODEL_ID" "$MODEL_REVISION" <<'PY'
import sys

model_id, revision = sys.argv[1:]

try:
    import transformers
except ImportError as exc:
    raise SystemExit("transformers is required; install transformers>=5.16.0 in a virtual environment") from exc

def version_tuple(value):
    parts = []
    for component in value.split(".")[:3]:
        digits = "".join(character for character in component if character.isdigit())
        parts.append(int(digits or 0))
    return tuple(parts + [0] * (3 - len(parts)))

if version_tuple(transformers.__version__) < (5, 16, 0):
    raise SystemExit(f"transformers {transformers.__version__} is too old; require >= 5.16.0")

from transformers import AutoConfig, AutoTokenizer

config = AutoConfig.from_pretrained(model_id, revision=revision)
if config.model_type != "glm5_next":
    raise SystemExit(f"unexpected model_type: {config.model_type!r}")
if type(config).__name__ != "Glm5NextConfig":
    raise SystemExit(f"unexpected config class: {type(config).__name__!r}")

tokenizer = AutoTokenizer.from_pretrained(model_id, revision=revision)
if tokenizer.chat_template is None:
    raise SystemExit("the pinned fixture does not expose a chat template")

print(f"transformers={transformers.__version__}")
print(f"model_type={config.model_type}")
print(f"config_class={type(config).__name__}")
print(f"revision={revision}")
print("weights_loaded=false")
print("glm5_next preflight passed")
PY
