# QCC ships two images. package.mk builds one at a time; which images exist
# and what each needs lives here.

##@ Images

# package.mk builds one image; which images exist and what each needs lives
# here. The controller is Go from the repo root; the executor is Python from
# its own directory, so no ldflags.

# LD_FLAGS comes from golang.mk and is -s -w here: QCC sets no VERSION_PKG,
# because the controller carries no version variable to stamp.

.PHONY: controller-image-build
controller-image-build: ## Build the qcc-controller image.
	@$(MAKE) --no-print-directory image-build IMAGE_NAME=qcc-controller IMAGE_CONTEXT=.

.PHONY: executor-image-build
executor-image-build: ## Build the qcc-executor image.
	@$(MAKE) --no-print-directory image-build IMAGE_NAME=qcc-executor IMAGE_CONTEXT=$(EXECUTOR_DIR) LD_FLAGS=

.PHONY: images-build
images-build: controller-image-build executor-image-build ## Build both container images.

.PHONY: controller-image-lint
controller-image-lint: ## Lint the qcc-controller Dockerfile.
	@$(MAKE) --no-print-directory image-lint IMAGE_NAME=qcc-controller

.PHONY: executor-image-lint
executor-image-lint: ## Lint the qcc-executor Dockerfile.
	@$(MAKE) --no-print-directory image-lint IMAGE_NAME=qcc-executor

.PHONY: images-lint
images-lint: controller-image-lint executor-image-lint ## Lint both Dockerfiles.

.PHONY: controller-image-scan
controller-image-scan: ## Scan the qcc-controller image for vulnerabilities.
	@mkdir -p build/ci/.cache/trivy && docker save $(IMG) -o build/ci/.cache/trivy/image.tar
	@$(MAKE) --no-print-directory trivy-scan TRIVY_ARGS="image --cache-dir build/ci/.cache/trivy --input build/ci/.cache/trivy/image.tar --scanners vuln --severity HIGH,CRITICAL --ignore-unfixed"
	@rm -f build/ci/.cache/trivy/image.tar

.PHONY: executor-image-scan
executor-image-scan: ## Scan the qcc-executor image for vulnerabilities.
	@mkdir -p build/ci/.cache/trivy && docker save $(EXECUTOR_IMG) -o build/ci/.cache/trivy/image.tar
	@$(MAKE) --no-print-directory trivy-scan TRIVY_ARGS="image --cache-dir build/ci/.cache/trivy --input build/ci/.cache/trivy/image.tar --scanners vuln --severity HIGH,CRITICAL --ignore-unfixed"
	@rm -f build/ci/.cache/trivy/image.tar

.PHONY: images-scan
images-scan: controller-image-scan executor-image-scan ## Scan both images.

.PHONY: controller-image-inspect
controller-image-inspect: ## Check qcc-controller layer efficiency with dive.
	@$(MAKE) --no-print-directory image-inspect IMAGE_NAME=qcc-controller

.PHONY: executor-image-inspect
executor-image-inspect: ## Check qcc-executor layer efficiency with dive.
	@$(MAKE) --no-print-directory image-inspect IMAGE_NAME=qcc-executor

.PHONY: images-inspect
images-inspect: controller-image-inspect executor-image-inspect ## Check both images' efficiency.
