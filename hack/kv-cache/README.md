# External KV-cache parity diagnostic

`verify_parity.py` drives a running vLLM OpenAI-compatible completion endpoint
and its Prometheus metrics. It creates synthetic, unique prompts, sends a cold
request followed by three different requests intended to cause GPU cache
eviction, and then repeats the original request. It requires a single external
prefix-cache hit metric series, no hit increase during the cold request, an
increase on reload, distinct completion request IDs, and identical prompt and
generated token IDs. It repeats this for three cases by default.

Run it against a dedicated test workload with no other clients. For example,
with a local Kubernetes port forward already running:

```bash
python3 hack/kv-cache/verify_parity.py \
  --endpoint http://127.0.0.1:18098 \
  --model MODEL_NAME \
  --api-key-file /path/to/api-key \
  --output /path/to/new-parity-result.json
```

The API key is read from the file and is never intentionally recorded. The
endpoint must not contain credentials, a query, or a fragment. Plain HTTP is
allowed only to loopback; other addresses require certificate-verified HTTPS.
Proxies and HTTP redirects are disabled. The output file must not already exist
and is created with mode `0600`. It contains prompt hashes, token IDs, request
IDs, times, metrics, and the failure reason; it does not record response bodies
or authorization headers. It is an unsigned diagnostic result. Protect it as
you would other workload diagnostics.

The service must be configured to make the three intervening prompts evict the
first request from the GPU cache while retaining its external cache. If the
reload does not increase the external hit counter the diagnostic fails rather
than assuming eviction occurred. Use a test model and GPU capacity appropriate
for the request lengths. `--cases` accepts 1 through 7 and `--timeout` specifies
a per-socket-operation timeout in seconds, from 1 to 120. It is not a hard
wall-clock deadline for a peer that continuously sends partial responses.

The counter is service-wide, not request-scoped. Concurrent callers can change
it, so a passing result only supports the intended claim when the workload is
isolated and logs corroborate the measured retrieval. These checks establish
bounded token parity for the prompts tested, not KV-byte equality, arbitrary
inputs, cross-node transfers, persistence, recovery, or production readiness.
