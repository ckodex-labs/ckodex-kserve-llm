# test/k6/Makefile — k6 load test orchestration for ckodex-kserve-llm
# Run from repo root: make -f test/k6/Makefile <target>
# Or via root Makefile shortcuts: make k6-smoke, make k6-bench, etc.

SCRIPTS_DIR := test/k6/scripts
RESULTS_DIR := test/k6/results
NS          ?= ckodex-inference

.PHONY: k6-setup k6-teardown k6-smoke k6-stress k6-bench k6-all

## Bootstrap: apply test CRs, wait for Ready, start port-forwards.
k6-setup:
	@echo "==> Setting up k6 test environment (namespace: $(NS))"
	@K6_NAMESPACE=$(NS) bash $(SCRIPTS_DIR)/port-forward.sh

## Run smoke tests (1 VU × 30s, all endpoints)
k6-smoke: | $(RESULTS_DIR)
	k6 run test/k6/smoke/llm-smoke.js \
	  --out json=$(RESULTS_DIR)/llm-smoke.json
	k6 run test/k6/smoke/embedding-smoke.js \
	  --out json=$(RESULTS_DIR)/embedding-smoke.json
	k6 run test/k6/smoke/asr-smoke.js \
	  --out json=$(RESULTS_DIR)/asr-smoke.json

## Run stress tests (ramping VUs: 8m LLM + 5m Embedding)
k6-stress: | $(RESULTS_DIR)
	k6 run test/k6/stress/llm-stress.js \
	  --out json=$(RESULTS_DIR)/llm-stress.json
	k6 run test/k6/stress/embedding-stress.js \
	  --out json=$(RESULTS_DIR)/embedding-stress.json

## Run benchmarks (sustained constant VUs with custom metrics)
k6-bench: | $(RESULTS_DIR)
	k6 run test/k6/benchmark/llm-benchmark.js \
	  --out json=$(RESULTS_DIR)/llm-bench.json
	k6 run test/k6/benchmark/embedding-benchmark.js \
	  --out json=$(RESULTS_DIR)/embedding-bench.json

## Full suite: smoke → stress → bench
k6-all: k6-smoke k6-stress k6-bench

## Teardown: kill port-forwards, delete test CRs
k6-teardown:
	@K6_NAMESPACE=$(NS) bash $(SCRIPTS_DIR)/teardown.sh

$(RESULTS_DIR):
	@mkdir -p $(RESULTS_DIR)
