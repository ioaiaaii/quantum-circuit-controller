# QCC's own components: the pinned toolchain, the qcc CLI, the gRPC
# contract, and the Python executor.  Nothing here comes from kubebuilder.

##@ QCC Tooling

.PHONY: tools-install
tools-install: ## Provision the toolchain pinned in .mise.toml
	mise install

.PHONY: tools-check
tools-check: ## Verify the pinned toolchain is installed.
	@command -v go >/dev/null 2>&1 || { echo "missing tool: go — install Go (https://go.dev/dl/); go.mod's toolchain directive then fetches the exact version"; exit 1; }
	@missing="$$(mise ls --missing 2>/dev/null)"; \
	if [ -n "$$missing" ]; then echo "$$missing"; echo "run 'make tools-install'"; exit 1; fi
	@echo "Toolchain matches .mise.toml."	

##@ QCC CLI

.PHONY: qcc-build
qcc-build: fmt vet ## Build the qcc CLI binary into ./dist/ with version baked in.
	@mkdir -p dist
	go build -ldflags "-X main.version=$(CLI_VERSION)" -o dist/qcc ./$(CLI_DIR)

.PHONY: qcc-install
qcc-install: ## Install the qcc CLI into $GOBIN (or ~/go/bin).
	go install -ldflags "-X main.version=$(CLI_VERSION)" ./$(CLI_DIR)

##@ Protobuf (buf) — executor gRPC contract

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
