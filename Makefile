# CKodex KServe LLM Operator — Makefile
IMG ?= ghcr.io/ckodex/kserve-llm-operator:latest
STORAGE_INITIALIZER_IMG ?= ckodex/storage-initializer:latest
CONTROLLER_GEN ?= $(shell which controller-gen 2>/dev/null || echo $(GOBIN)/controller-gen)
ENVTEST ?= $(shell which setup-envtest 2>/dev/null || echo $(GOBIN)/setup-envtest)
GOLANGCI_LINT ?= $(shell which golangci-lint 2>/dev/null || echo $(GOBIN)/golangci-lint)
KIND_CLUSTER_NAME ?= kserve-017
GOBIN ?= $(shell go env GOPATH)/bin

.PHONY: all
all: generate fmt vet lint build

##@ Development

.PHONY: generate
generate: controller-gen ## Generate deepcopy, client, informer, lister code
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."

.PHONY: manifests
manifests: controller-gen ## Generate CRD manifests, RBAC, webhook configs
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd:allowDangerousTypes=true webhook paths="./..." output:crd:artifacts:config=config/crd output:rbac:artifacts:config=config/rbac output:webhook:artifacts:config=config/webhook

.PHONY: fmt
fmt: ## Run go fmt
	go fmt ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint
	$(GOLANGCI_LINT) run --timeout=5m

.PHONY: test
test: generate manifests envtest ## Run unit + integration tests
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use -p path 2>/dev/null)" go test -p 1 ./... -coverprofile cover.out -v -race
	@echo "Coverage:"
	@go tool cover -func cover.out | tail -1

.PHONY: e2e-test
e2e-test: ## Run E2E tests (requires KIND cluster)
	go test ./test/e2e/... -v -timeout 600s -tags=e2e

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary
	@version=$$( [ -n "$$VERSION" ] && echo "$$VERSION" || echo "dev" ); \
	go build -o bin/manager -ldflags="-s -w -X github.com/ckodex-labs/kserve-llm-operator/internal/version.Version=$$version" cmd/manager/main.go

.PHONY: run
run: generate fmt vet ## Run controller locally against cluster
	go run cmd/manager/main.go

.PHONY: docker-build
docker-build: ## Build docker image
	docker build --target manager -t $(IMG) .

.PHONY: docker-push
docker-push: ## Push docker image
	docker push $(IMG)

.PHONY: storage-initializer-img
storage-initializer-img: ## Build custom Go storage-initializer image
	docker build --target storage-initializer -t $(STORAGE_INITIALIZER_IMG) .

.PHONY: storage-initializer-load
storage-initializer-load: storage-initializer-img ## Build and load storage-initializer into KIND
	kind load docker-image $(STORAGE_INITIALIZER_IMG) --name $(KIND_CLUSTER_NAME)

##@ Deployment

.PHONY: install
install: manifests ## Install CRDs into cluster
	kubectl apply --server-side -f config/crd/

.PHONY: uninstall
uninstall: manifests ## Uninstall CRDs from cluster
	kubectl delete -f config/crd/ --ignore-not-found

.PHONY: deploy
deploy: manifests ## Deploy controller to cluster
	kubectl apply -f config/rbac/
	kubectl apply -f config/manager/
	kubectl apply -f config/webhook/

.PHONY: undeploy
undeploy: ## Undeploy controller from cluster
	kubectl delete -f config/rbac/ --ignore-not-found

.PHONY: redeploy
redeploy: undeploy deploy ## Fast redeploy of the controller

.PHONY: full-redeploy
full-redeploy: ## High-assurance 3x redeploy cycle
	ITERATIONS=3 bash hack/redeploy-harness.sh

##@ KIND

.PHONY: kind-setup
kind-setup: ## Create KIND cluster with port mappings
	kind create cluster --name $(KIND_CLUSTER_NAME) --config deploy/kind/kind-config.yaml
	kubectl config use-context kind-$(KIND_CLUSTER_NAME)

.PHONY: kind-load
kind-load: docker-build ## Load docker image into KIND cluster
	kind load docker-image $(IMG) --name $(KIND_CLUSTER_NAME)

.PHONY: kind-teardown
kind-teardown: ## Delete KIND cluster
	kind delete cluster --name $(KIND_CLUSTER_NAME)

.PHONY: kind-e2e
kind-e2e: ## Full local KIND E2E: cluster, dependencies, deploy, and live inference probe
	bash run/e2e.sh

.PHONY: kind-cleanup
kind-cleanup: ## Tear down KIND cluster and prune local state
	bash run/cleanup.sh

##@ Tools

.PHONY: controller-gen
controller-gen: ## Install controller-gen if not present
	@test -x $(CONTROLLER_GEN) || go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

.PHONY: envtest
envtest: ## Install setup-envtest if not present
	@test -x $(ENVTEST) || go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: golangci-lint
golangci-lint: ## Install golangci-lint if not present
	@test -x $(GOLANGCI_LINT) || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: e2e-setup
e2e-setup: kind-setup kind-load deploy ## Bootstrap E2E cluster from scratch

.PHONY: e2e-run
e2e-run: e2e-setup e2e-test ## Full E2E cycle: setup + test

##@ k6 Load Tests

.PHONY: k6-setup
k6-setup: ## Bootstrap k6: apply test CRs and start port-forwards
	$(MAKE) -f test/k6/Makefile k6-setup

.PHONY: k6-smoke
k6-smoke: ## Smoke tests (1 VU × 30s, all endpoints)
	$(MAKE) -f test/k6/Makefile k6-smoke

.PHONY: k6-stress
k6-stress: ## Stress tests (ramping VUs, LLM + Embedding)
	$(MAKE) -f test/k6/Makefile k6-stress

.PHONY: k6-bench
k6-bench: ## Benchmark (sustained VUs, custom tokens/sec metrics)
	$(MAKE) -f test/k6/Makefile k6-bench

.PHONY: k6-all
k6-all: ## Full k6 suite: smoke → stress → bench
	$(MAKE) -f test/k6/Makefile k6-all

.PHONY: k6-teardown
k6-teardown: ## Teardown k6: kill port-forwards and delete test CRs
	$(MAKE) -f test/k6/Makefile k6-teardown

##@ Console Dashboard

.PHONY: console-dev
console-dev: ## Start console in development mode
	cd console && npm run dev

.PHONY: console-build
console-build: ## Build console production bundle
	cd console && npm run build

.PHONY: console-check
console-check: console-build ## CI-visible console production gate

.PHONY: release-readiness
release-readiness: ## Rehearse local release artifacts and fail on hidden repo mutations
	bash hack/release-readiness.sh

.PHONY: console-img
console-img: ## Build console docker image
	docker build -t ckodex/console:latest ./console
