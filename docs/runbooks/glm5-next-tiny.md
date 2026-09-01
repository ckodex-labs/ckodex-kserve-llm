# Runbook: GLM-5.3-Flash Tiny Architecture Fixture

This is a local development fixture for the `glm5_next` model family. It is
intended to exercise model loading and operator wiring on a small machine.

The checkpoint is not a miniature of the production model's capability. Its
architecture family and major component types are retained, but its dimensions
are reduced and its weights were randomly initialized before a toy text
fine-tune. It must not be used for model-quality, NVFP4, GPU-performance,
long-context, or distributed-inference claims.

## Pinned fixture

```text
model:    inference-optimization/GLM-5.3-Flash-0.1B-A0.1B
revision: 8311399447eba9c9b215e3209ab6f25e59c7d21e
format:   BF16, unquantized
profile:  local architecture fixture
```

The model card requires `transformers >= 5.16.0` for `glm5_next` registration.
Use a runtime image that contains that support; the repository's generic vLLM
image is not automatically evidence of compatibility with this new architecture.
If the package index cannot resolve that version, keep the preflight failed and
use an approved runtime image or wheel that provides the required registration;
do not lower the requirement or edit the model configuration to bypass it.

Run the weight-free preflight first. It fetches only the pinned configuration
and tokenizer metadata; it does not load model weights:

```bash
bash run/glm5-next-tiny-preflight.sh
```

The smallest first check is a direct Transformers load on the workstation. It
does not start Kubernetes or claim serving performance:

```bash
python -m venv /tmp/ckodex-glm5-next-tiny
. /tmp/ckodex-glm5-next-tiny/bin/activate
python -m pip install --upgrade 'transformers>=5.16.0' torch safetensors
python - <<'PY'
from transformers import AutoTokenizer, Glm5NextForConditionalGeneration

model_id = "inference-optimization/GLM-5.3-Flash-0.1B-A0.1B"
revision = "8311399447eba9c9b215e3209ab6f25e59c7d21e"
tokenizer = AutoTokenizer.from_pretrained(model_id, revision=revision)
model = Glm5NextForConditionalGeneration.from_pretrained(model_id, revision=revision)
inputs = tokenizer("Say hello in one sentence.", return_tensors="pt")
outputs = model.generate(**inputs, max_new_tokens=32)
print(tokenizer.decode(outputs[0], skip_special_tokens=True))
PY
```

## Apply to a local cluster

After the operator and CRDs are installed and the operator watches the
`default` namespace:

```bash
kubectl apply -f config/samples/llminferenceservice_glm5_next_tiny.yaml
kubectl get llminferenceservice,pod -n default -w
```

The fixture deliberately omits GPU, quantization, parallelism, speculative
decoding, and scheduler settings. Those are separate capability surfaces and
are not needed to validate the small-machine control-plane path.

For a local API check, port-forward the generated Service and send a chat
request using the exact `spec.model.name` value:

```bash
kubectl port-forward service/glm53-next-tiny -n default 8000:80
curl -sS http://127.0.0.1:8000/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "inference-optimization/GLM-5.3-Flash-0.1B-A0.1B",
    "messages": [{"role": "user", "content": "Say hello in one sentence."}],
    "max_tokens": 32
  }'
```

## What a passing run proves

- the pinned Hugging Face model reference reaches the storage initializer;
- the declared `v1` resource reconciles into a workload;
- the selected runtime recognizes the `glm5_next` configuration;
- readiness and the OpenAI-compatible chat route work for this fixture.

It does not prove that the full GLM-5.3-Flash model, an NVFP4 checkpoint, or a
production GPU profile will start or produce useful output.

The latest local CPU result is recorded in
[`docs/evidence/glm5-next-tiny-local-2026-08-31.md`](../evidence/glm5-next-tiny-local-2026-08-31.md).
