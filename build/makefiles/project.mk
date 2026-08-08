# Project-level settings and the cross-stack umbrellas.
#
# Nothing here belongs to kubebuilder's scaffold. Overrides live in this file
# rather than in the root Makefile so that root stays close to stock and
# `kubebuilder alpha update` merges it without conflicts.

# .SHELLFLAGS is silently ignored before GNU Make 3.82, so a recipe would run
# without -e and a failing line would not stop the target. macOS ships 3.81.
ifeq ($(filter 4.% 5.%,$(MAKE_VERSION)),)
$(error GNU Make $(MAKE_VERSION) is too old. Run `mise install`, then re-run)
endif

SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables

# Bare `make` prints help. Set explicitly so include order cannot decide it;
# the scaffold's first target is `all`, which would otherwise win.
.DEFAULT_GOAL := help

# Match GOTOOLCHAIN to go.mod. A bootstrap Go on a different patch version
# races with the auto-fetched one during parallel compilation.
GO_VERSION := $(shell awk '/^go [0-9]+(\.[0-9]+)+/ {print $$2; exit}' go.mod 2>/dev/null)
ifneq ($(GO_VERSION),)
export GOTOOLCHAIN ?= go$(GO_VERSION)
endif

# --- Components ---
# IMG comes from the root Makefile (kubebuilder owns it); the executor is ours.
EXECUTOR_IMG ?= qcc-executor:$(IMAGE_TAG)

CONTROLLER_DIR  := cmd/qcc-controller
CONTROLLER_DOCK := build/package/qcc-controller/Dockerfile
EXECUTOR_DIR    := qcc-executor
EXECUTOR_DOCK   := build/package/qcc-executor/Dockerfile
CLI_DIR         := cmd/qcc

# Baked into `qcc version` via -ldflags.
CLI_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# --- Cross-stack umbrellas ---
# QCC is polyglot; kubebuilder's generate, lint and test cover Go only. Adding
# prerequisites without a recipe *augments* an existing target instead of
# overriding it, so the scaffold's recipe still runs and no rename is needed.
# `make lint` therefore covers Go, proto, Python, Dockerfiles and docs.

generate: proto-generate
lint: proto-lint executor-lint images-lint docs-check
test: executor-test
