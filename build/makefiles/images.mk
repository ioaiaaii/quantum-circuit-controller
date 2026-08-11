# QCC's two images, built one at a time through op-image-*.

##@ Images

# LD_FLAGS is go.mk's -s -w. The executor is Python, so it clears it.

.PHONY: controller-image-build
controller-image-build: ## Build the qcc-controller image.
	@$(MAKE) --no-print-directory op-image-build IMAGE_NAME=qcc-controller IMAGE_CONTEXT=.

.PHONY: executor-image-build
executor-image-build: ## Build the qcc-executor image.
	@$(MAKE) --no-print-directory op-image-build IMAGE_NAME=qcc-executor IMAGE_CONTEXT=$(EXECUTOR_DIR) LD_FLAGS=

.PHONY: images-build
images-build: controller-image-build executor-image-build ## Build both container images.

.PHONY: controller-image-lint
controller-image-lint: ## Lint the qcc-controller Dockerfile.
	@$(MAKE) --no-print-directory op-image-lint IMAGE_NAME=qcc-controller

.PHONY: executor-image-lint
executor-image-lint: ## Lint the qcc-executor Dockerfile.
	@$(MAKE) --no-print-directory op-image-lint IMAGE_NAME=qcc-executor

.PHONY: images-lint
images-lint: controller-image-lint executor-image-lint ## Lint both Dockerfiles.
