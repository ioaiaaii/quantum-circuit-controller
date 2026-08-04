# Quantum Circuit Controller — root Makefile.
#
# Toolchain split:
#   * Go is pinned in go.mod (`go 1.25.7` + `toolchain go1.25.7`).  Any Go on
#     PATH with GOTOOLCHAIN=auto (the default) will auto-fetch the right one.
#   * Everything else (kubectl, kind, helm, kustomize, buf, controller-gen,
#     setup-envtest, golangci-lint, kubebuilder, python, uv) is pinned in
#     .mise.toml.  Run `make tools-install` (= `mise install`) after cloning.

# --- Repo-operator overrides ---
override OPERATOR_PATH  := build/repo-operator
override DEFAULT_BRANCH := main

# --- Repo-operator includes ---
# include ${OPERATOR_PATH}/makefiles/base.mk
# include ${OPERATOR_PATH}/makefiles/changelog.mk
# include ${OPERATOR_PATH}/makefiles/golang.mk

# Pin GOTOOLCHAIN to the version go.mod declares.  Without this, a bootstrap
# Go whose patch version differs from go.mod's pin (e.g. system 1.25.1 vs
# go.mod 1.25.7) races between itself and the auto-fetched toolchain during
# parallel compilation, producing "compile: version X does not match go tool
# version Y" errors.  Reading from go.mod keeps Makefile and module in sync.
GO_VERSION := $(shell awk '/^go [0-9]+(\.[0-9]+)+/ {print $$2; exit}' go.mod 2>/dev/null)
ifneq ($(GO_VERSION),)
export GOTOOLCHAIN ?= go$(GO_VERSION)
endif

# --- Images ---
IMG     ?= qcc-controller:latest
EXECUTOR_IMG ?= qcc-executor:latest
YEAR    ?= $(shell date +%Y)

# --- Component paths ---
CONTROLLER_DIR  := cmd/qcc-controller
CONTROLLER_DOCK := build/package/qcc-controller/Dockerfile
EXECUTOR_DIR    := qcc-executor
EXECUTOR_DOCK   := build/package/qcc-executor/Dockerfile
CLI_DIR         := cmd/qcc

# CLI_VERSION is injected into `qcc version` via -ldflags.  Order: explicit
# override, then `git describe`, then "dev".  Build info baked in by `go build`
# (debug.ReadBuildInfo) is the final runtime fallback if this is empty.
CLI_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# --- Container tool ---
CONTAINER_TOOL ?= docker

# --- Shell ---
SHELL := /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

#@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: tools-install
tools-install: ## Provision the toolchain pinned in .mise.toml
	mise install

.PHONY: tools-check
tools-check: ## Verify required tools are on PATH.
	@command -v go >/dev/null 2>&1 || { echo "missing tool: go (install via Homebrew or https://go.dev/dl/; go.mod's toolchain directive will auto-fetch the pinned version)"; exit 1; }
	@for t in kubectl kind helm kustomize buf golangci-lint controller-gen setup-envtest uv; do \
		command -v $$t >/dev/null 2>&1 || { echo "missing tool: $$t (run 'make tools-install')"; exit 1; }; \
	done
	@echo "All pinned tools are on PATH."

##@ Local Dev Platform (kind + observability stack)

DEV_CLUSTER ?= qcc-dev
DEV_NS      ?= monitoring

.PHONY: platform-up
platform-up: ## Bring up the kind cluster + observability stack (kps + Tempo + OTel Collector)
	@if ! kind get clusters 2>/dev/null | grep -qx "$(DEV_CLUSTER)"; then \
		kind create cluster --config deploy/platform/kind-config.yaml; \
	fi
	@helm repo add prometheus-community https://prometheus-community.github.io/helm-charts 2>/dev/null || true
	@helm repo add grafana              https://grafana.github.io/helm-charts              2>/dev/null || true
	@helm repo add open-telemetry       https://open-telemetry.github.io/opentelemetry-helm-charts 2>/dev/null || true
	@helm repo update prometheus-community grafana open-telemetry
	@kubectl get ns $(DEV_NS) >/dev/null 2>&1 || kubectl create ns $(DEV_NS)
	helm upgrade --install kps     prometheus-community/kube-prometheus-stack -n $(DEV_NS) -f deploy/platform/kps-values.yaml     --wait
	helm upgrade --install tempo   grafana/tempo                              -n $(DEV_NS) -f deploy/platform/tempo-values.yaml   --wait
	helm upgrade --install otelcol open-telemetry/opentelemetry-collector     -n $(DEV_NS) -f deploy/platform/otelcol-values.yaml --wait
	@echo ""
	@echo "Platform up. Useful next steps:"
	@echo "  Grafana (admin/admin): kubectl port-forward -n $(DEV_NS) svc/kps-grafana 3000:80"
	@echo "  OTLP gRPC ingress    : kubectl port-forward -n $(DEV_NS) svc/otelcol-opentelemetry-collector 4317:4317"

.PHONY: platform-down
platform-down: ## Tear down the platform stack and the kind cluster
	-helm uninstall otelcol -n $(DEV_NS)
	-helm uninstall tempo   -n $(DEV_NS)
	-helm uninstall kps     -n $(DEV_NS)
	-kind delete cluster --name $(DEV_CLUSTER)

.PHONY: platform-status
platform-status: ## Show platform stack status (clusters, releases, pods)
	@echo "--- kind clusters ---"
	@kind get clusters 2>/dev/null || true
	@echo ""
	@echo "--- helm releases in $(DEV_NS) ---"
	@helm list -n $(DEV_NS) 2>/dev/null || true
	@echo ""
	@echo "--- pods in $(DEV_NS) ---"
	@kubectl get pods -n $(DEV_NS) 2>/dev/null || true

##@ Development

.PHONY: manifests
manifests: ## Generate CRDs, RBAC, webhook manifests from kubebuilder markers.
	# allowDangerousTypes=true permits float64 fields in CRDs.
	# QPUErrorMedians stores float64 medians (single-qubit / two-qubit /
	# readout error rates); the OpenAPI cross-language interop concern
	# that motivates the default rejection is moot for the thesis
	# prototype (only Go and Python consumers, both handle JSON numbers).
	controller-gen rbac:roleName=manager-role "crd:allowDangerousTypes=true" webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: ## Regenerate DeepCopy methods.
	controller-gen object:headerFile="hack/boilerplate.go.txt",year=$(YEAR) paths="./..."

.PHONY: fmt
fmt: ## go fmt all packages.
	go fmt ./...

.PHONY: vet
vet: ## go vet all packages.
	go vet ./...

# ENVTEST_K8S_VERSION derives from the major.minor of k8s.io/api in go.mod.
ENVTEST_K8S_VERSION ?= $(shell v='$(call gomodver,k8s.io/api)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_K8S_VERSION manually (k8s.io/api replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')

.PHONY: test
test: manifests generate fmt vet ## Run unit tests via envtest (binaries cached by setup-envtest).
	KUBEBUILDER_ASSETS="$$(setup-envtest use $(ENVTEST_K8S_VERSION) -p path)" \
		go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

KIND_CLUSTER ?= quantum-circuit-controller-test-e2e

.PHONY: setup-test-e2e
setup-test-e2e: ## Create an isolated Kind cluster for e2e tests.
	@case "$$(kind get clusters)" in \
		*"$(KIND_CLUSTER)"*) echo "Kind cluster '$(KIND_CLUSTER)' already exists." ;; \
		*) kind create cluster --name $(KIND_CLUSTER) ;; \
	esac

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ## Run e2e tests against an isolated kind cluster.
	KIND_CLUSTER=$(KIND_CLUSTER) go test -tags=e2e ./test/e2e/ -v -ginkgo.v
	$(MAKE) cleanup-test-e2e

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the e2e Kind cluster.
	@kind delete cluster --name $(KIND_CLUSTER)

# When .custom-gcl.yml is present the project requires a custom golangci-lint
# binary that bundles the listed plugins (logcheck etc.).  We build it into a
# gitignored cache dir; lint targets prefer the custom binary if it exists.
GOLANGCI_LINT_CACHE := .cache/golangci-lint
GOLANGCI_LINT_BIN   := $(GOLANGCI_LINT_CACHE)/golangci-lint
GOLANGCI_LINT_EXEC  := $(shell test -x $(GOLANGCI_LINT_BIN) && echo $(GOLANGCI_LINT_BIN) || echo golangci-lint)

$(GOLANGCI_LINT_BIN): .custom-gcl.yml
	@mkdir -p $(GOLANGCI_LINT_CACHE)
	golangci-lint custom --destination $(GOLANGCI_LINT_CACHE) --name golangci-lint

.PHONY: lint-build
lint-build: $(GOLANGCI_LINT_BIN) ## Build the custom golangci-lint binary with project plugins.

.PHONY: lint
lint: $(GOLANGCI_LINT_BIN) ## Run golangci-lint (custom binary when plugins are needed).
	$(GOLANGCI_LINT_BIN) run

.PHONY: lint-fix
lint-fix: $(GOLANGCI_LINT_BIN) ## Run golangci-lint with --fix.
	$(GOLANGCI_LINT_BIN) run --fix

.PHONY: lint-config
lint-config: $(GOLANGCI_LINT_BIN) ## Verify the golangci-lint configuration.
	$(GOLANGCI_LINT_BIN) config verify

# Checks every git-tracked markdown file.
.PHONY: docs-check
docs-check: ## Verify links and heading anchors in all tracked markdown (offline).
	git ls-files -co --exclude-standard '*.md' | xargs lychee --offline --include-fragments --no-progress

.PHONY: docs-demo
docs-demo: ## Record the README demo GIF (needs a running deployment).
	vhs docs/assets/demo.tape
	@echo "Recorded docs/assets/figures/qcc-demo.gif — reference it at width=\"864\""
	@ls -lh docs/assets/figures/qcc-demo.gif

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build the qcc-controller binary into ./dist/.
	@mkdir -p dist
	go build -o dist/qcc-controller ./$(CONTROLLER_DIR)

.PHONY: run
run: manifests generate fmt vet ## Run the controller locally against the current kubeconfig.
	go run ./$(CONTROLLER_DIR)

.PHONY: qcc-build
qcc-build: fmt vet ## Build the qcc CLI binary into ./dist/ with version baked in.
	@mkdir -p dist
	go build -ldflags "-X main.version=$(CLI_VERSION)" -o dist/qcc ./$(CLI_DIR)

.PHONY: qcc-install
qcc-install: ## Install the qcc CLI into $GOBIN (or ~/go/bin).
	go install -ldflags "-X main.version=$(CLI_VERSION)" ./$(CLI_DIR)

.PHONY: docker-build
docker-build: ## Build the qcc-controller container image.
	$(CONTAINER_TOOL) build -t $(IMG) -f $(CONTROLLER_DOCK) .

.PHONY: docker-push
docker-push: ## Push the qcc-controller container image.
	$(CONTAINER_TOOL) push $(IMG)

.PHONY: docker-load
docker-load: ## Load the qcc-controller image into the local kind cluster.
	kind load docker-image $(IMG) --name $(DEV_CLUSTER)

PLATFORMS ?= linux/arm64,linux/amd64
.PHONY: docker-buildx
docker-buildx: ## Build multi-arch qcc-controller image with buildx.
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' $(CONTROLLER_DOCK) > $(CONTROLLER_DOCK).cross
	- $(CONTAINER_TOOL) buildx create --name quantum-circuit-controller-builder
	$(CONTAINER_TOOL) buildx use quantum-circuit-controller-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag $(IMG) -f $(CONTROLLER_DOCK).cross .
	- $(CONTAINER_TOOL) buildx rm quantum-circuit-controller-builder
	rm $(CONTROLLER_DOCK).cross

.PHONY: build-installer
build-installer: manifests generate ## Generate dist/install.yaml from Kustomize manifests.
	mkdir -p dist
	cd config/manager && kustomize edit set image qcc-controller=$(IMG)
	kustomize build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@out="$$( kustomize build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | kubectl apply -f -; else echo "No CRDs to install; skipping."; fi

.PHONY: uninstall
uninstall: manifests ## Uninstall CRDs.
	@out="$$( kustomize build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | kubectl delete --ignore-not-found=$(ignore-not-found) -f -; else echo "No CRDs to delete; skipping."; fi

.PHONY: deploy
deploy: manifests ## Deploy the controller to the cluster pointed to by KUBECONFIG.
	cd config/manager && kustomize edit set image qcc-controller=$(IMG)
	kustomize build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy the controller.
	kustomize build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

##@ Protobuf (buf) — QRM gRPC contract

PROTO_DIR ?= proto

.PHONY: proto-lint
proto-lint: ## Lint .proto files via buf.
	cd $(PROTO_DIR) && buf lint

.PHONY: proto-format
proto-format: ## Format .proto files via buf.
	cd $(PROTO_DIR) && buf format -w

.PHONY: proto-generate
proto-generate: ## Generate Go + Python stubs from .proto files.
	cd $(PROTO_DIR) && buf generate

.PHONY: proto-breaking
proto-breaking: ## Check for breaking proto changes vs main.
	cd $(PROTO_DIR) && buf breaking --against '.git#branch=main,subdir=proto'

##@ Executor (Python service)

.PHONY: executor-install
executor-install: ## Install qcc-executor Python dependencies via uv.
	cd $(EXECUTOR_DIR) && uv sync

.PHONY: executor-test
executor-test: ## Run qcc-executor unit tests via uv + pytest.
	cd $(EXECUTOR_DIR) && uv run pytest -v

.PHONY: executor-lint
executor-lint: ## Lint qcc-executor Python code via uv + ruff.
	cd $(EXECUTOR_DIR) && uv run ruff check .

.PHONY: executor-build
executor-build: ## Build the qcc-executor container image.
	$(CONTAINER_TOOL) build -t $(EXECUTOR_IMG) -f $(EXECUTOR_DOCK) $(EXECUTOR_DIR)

.PHONY: executor-load
executor-load: ## Load the qcc-executor image into the local kind cluster.
	kind load docker-image $(EXECUTOR_IMG) --name $(DEV_CLUSTER)

##@ Dev loop

.PHONY: dev-up
dev-up: platform-up install ## Bring up dev platform + install CRDs.
	@echo "Dev platform ready. Run 'make run' to start the controller against this cluster."

.PHONY: dev-down
dev-down: platform-down ## Tear down the dev platform.

.PHONY: dist-up
dist-up: docker-build executor-build docker-load executor-load install deploy ## Build both images, load into kind, install CRDs, deploy controller+executor.
	@echo ""
	@echo "QCC deployed to $(DEV_CLUSTER). Inspect with:"
	@echo "  kubectl get pods -n quantum-circuit-controller-system"
	@echo "  kubectl get circuits"
	@echo "Apply a sample:"
	@echo "  kubectl apply -f config/samples/qcc_v1alpha1_circuit.yaml"

.PHONY: dist-down
dist-down: undeploy uninstall ## Remove deployment and CRDs from the kind cluster.

# --- helpers -----------------------------------------------------------------

define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef
