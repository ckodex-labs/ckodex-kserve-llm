# Nightly KIND failure evidence, checked 2026-10-14

FACT — the latest main and latest nightly checked here use commit
`845bd53063769815c578ae7412319c2fda66c619`. The latest updated PR is merged
[PR 64](https://github.com/ckodex-labs/ckodex-kserve-llm/pull/64); no evidence
attributes this failure to that PR.

FACT — [nightly run 38385225450](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/38385225450)
failed. [Job 115255226278](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/38385225450/job/115255226278)
shows successful cluster creation, Gateway CRD setup, operator and initializer
image builds/loads, and operator rollout. At 10:59:36 UTC the readiness wait
timed out: pod `llama3-8b-f5fb894f8-fjpcj` was `1/2`, in `CrashLoopBackOff`,
with five restarts. Its inference image was `vllm/vllm-openai-cpu:v0.28.0`.
The resource name is historical; the committed smoke model is GPT-2.
E2E lifecycle and chaos recovery tests were skipped after bootstrap failed.

FACT — the [failure artifact](https://github.com/ckodex-labs/ckodex-kserve-llm/actions/runs/38385225450/artifacts/11807116634)
contains only resources, events and controller logs. The old workflow requests
controller logs by label without `--tail=-1`, retaining only the default tail.
It does not request inference logs, pod describe or container exit status.
The job deletes the KIND cluster after upload, so the exact vLLM exception
cannot be recovered from that run's retained evidence.

S — runtime acceptance remains open. Increasing the readiness wait or changing
vLLM arguments/images is not justified by the retained diagnostic evidence.
No runtime change accompanies the diagnostic patch.

C — local diagnostic fixtures exercise current and previous vLLM logs from
all discovered replicas, full controller/container logs, status projection,
describe redaction, known env-literal and common credential masking, partial
collection, missing executables, unavailable previous logs, invalid discovery,
context isolation, bounded timeouts and stale-output rejection. The Dagger
`Test` function runs these fixtures using the repository's already pinned
Python image. Local test results are recorded separately with the delivered
patch; hosted collection is still unvalidated.

The collector writes only redacted outputs, never raw pod JSON or stderr.
`collection.json` records collection success/failure per operation. Discovery
failure retains only this manifest to avoid free text without literal env
scrubbing. Unlabelled or encoded secrets not present as literal pod env values
are outside the masking guarantee; review a collected bundle before sharing.

Next validation requires a separately authorized disposable hosted run on the
diagnostic patch. Verify the retained current/previous vLLM logs and exit
reason, inspect redaction, then decide whether a narrow runtime fix is supported.
Passing local fixtures does not close readiness, inference or recovery gates.
