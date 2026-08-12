# QCC settings and workflow umbrellas. The scaffold stays in the root
# Makefile; op-* interfaces come from build/repo-operator.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables

# Bare `make` prints help, not the scaffold's `all`.
.DEFAULT_GOAL := help

# A bootstrap Go on a different patch version races with the auto-fetched one.
GO_VERSION := $(shell awk '/^go [0-9]+(\.[0-9]+)+/ {print $$2; exit}' go.mod 2>/dev/null)
ifneq ($(GO_VERSION),)
export GOTOOLCHAIN ?= go$(GO_VERSION)
endif

# IMG is the scaffold's; the executor image is ours.
EXECUTOR_IMG ?= qcc-executor:$(IMAGE_TAG)
EXECUTOR_DIR := qcc-executor
CLI_DIR      := cmd/qcc
PROTO_DIR    ?= proto

# Baked into `qcc version` via -ldflags: the tag on releases, branch
# plus commit on dev builds, same source as the image tags.
CLI_VERSION ?= $(if $(OP_TAG),$(VERSION),$(VERSION)-$(OP_COMMIT))

##@ Workflows

.PHONY: ci
ci: tools-check lint proto-lint executor-lint images-lint config-scan docs-check test executor-test ## Stage 1, cluster-free: every lint and test. Stage 2 is test-e2e.

# Not part of `generate`: buf calls the Buf Schema Registry, and generate is a
# prerequisite of build, run, test, and build-installer.
.PHONY: generate-all
generate-all: generate proto-generate ## Run every generator, including proto.

##@ Toolchain

.PHONY: tools-install
tools-install: ## Provision the toolchain pinned in .mise.toml.
	mise install

.PHONY: tools-check
tools-check: ## Verify the pinned toolchain is installed.
	@command -v go >/dev/null 2>&1 || { echo "missing tool: go (https://go.dev/dl/)"; exit 1; }
	@missing="$$(mise ls --missing 2>/dev/null)"; \
	if [ -n "$$missing" ]; then echo "$$missing"; echo "run 'make tools-install'"; exit 1; fi
	@echo "Toolchain matches .mise.toml."

##@ CLI

.PHONY: qcc-build
qcc-build: fmt vet ## Build the qcc CLI into ./dist/ with version baked in.
	@mkdir -p dist
	go build -ldflags "-X main.version=$(CLI_VERSION)" -o dist/qcc ./$(CLI_DIR)

.PHONY: qcc-install
qcc-install: ## Install the qcc CLI into $GOBIN (or ~/go/bin).
	go install -ldflags "-X main.version=$(CLI_VERSION)" ./$(CLI_DIR)

QCC_DIST_LD_FLAGS := -s -w -X main.version=$(CLI_VERSION)

.PHONY: qcc-dist
qcc-dist: ## Build qcc release binaries for all platforms into dist/ with checksums.
	@mkdir -p dist
	@$(MAKE) --no-print-directory op-go-build GOOS=linux  GOARCH=amd64 BINARY_PATH=dist/qcc-linux-amd64  CMD_PATH=./$(CLI_DIR) LD_FLAGS="$(QCC_DIST_LD_FLAGS)"
	@$(MAKE) --no-print-directory op-go-build GOOS=linux  GOARCH=arm64 BINARY_PATH=dist/qcc-linux-arm64  CMD_PATH=./$(CLI_DIR) LD_FLAGS="$(QCC_DIST_LD_FLAGS)"
	@$(MAKE) --no-print-directory op-go-build GOOS=darwin GOARCH=amd64 BINARY_PATH=dist/qcc-darwin-amd64 CMD_PATH=./$(CLI_DIR) LD_FLAGS="$(QCC_DIST_LD_FLAGS)"
	@$(MAKE) --no-print-directory op-go-build GOOS=darwin GOARCH=arm64 BINARY_PATH=dist/qcc-darwin-arm64 CMD_PATH=./$(CLI_DIR) LD_FLAGS="$(QCC_DIST_LD_FLAGS)"
	@cd dist && shasum -a 256 qcc-linux-amd64 qcc-linux-arm64 qcc-darwin-amd64 qcc-darwin-arm64 > SHA256SUMS
	@cat dist/SHA256SUMS

##@ Protobuf

.PHONY: proto-lint
proto-lint: ## Lint .proto files via buf.
	cd $(PROTO_DIR) && buf lint

.PHONY: proto-format
proto-format: ## Format .proto files via buf.
	cd $(PROTO_DIR) && buf format -w

.PHONY: proto-generate
proto-generate: ## Generate Go + Python stubs from .proto files.
	cd $(PROTO_DIR) && buf generate

# Against the remote-tracking ref, not the local branch: CI checkouts are
# detached with no local main, and origin/main exists in both places.
.PHONY: proto-breaking
proto-breaking: ## Check for breaking proto changes vs main.
	cd $(PROTO_DIR) && buf breaking --against '../.git#ref=refs/remotes/origin/main,subdir=proto'

##@ Executor

.PHONY: executor-test
executor-test: ## Run executor unit tests via uv + pytest.
	cd $(EXECUTOR_DIR) && uv run pytest -v

.PHONY: executor-lint
executor-lint: ## Lint executor Python code via uv + ruff.
	cd $(EXECUTOR_DIR) && uv run ruff check .

##@ Documentation

# --offline keeps PR runs from flaking on third-party outages;
# --include-fragments verifies heading anchors.
.PHONY: docs-check
docs-check: ## Verify links and anchors in all tracked markdown (offline).
	git ls-files -co --exclude-standard '*.md' | while IFS= read -r f; do [ -f "$$f" ] && printf '%s\n' "$$f"; done | xargs lychee --offline --include-fragments --no-progress

# Manual: needs a running deployment, plus ttyd and ffmpeg alongside vhs.
.PHONY: docs-demo
docs-demo: ## Record the README demo GIF.
	vhs docs/assets/demo.tape
	@ls -lh docs/assets/figures/qcc-demo.gif
