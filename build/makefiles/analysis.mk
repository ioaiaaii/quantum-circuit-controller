
##@ Analysis

.PHONY: config-scan
config-scan: ## Scan manifests and Dockerfiles for misconfigurations.
	@$(MAKE) --no-print-directory op-scan TRIVY_ARGS="config --cache-dir build/ci/.cache/trivy ."

.PHONY: license-scan
license-scan: ## Scan dependency licenses.
	@$(MAKE) --no-print-directory op-scan TRIVY_ARGS="fs --scanners license --cache-dir build/ci/.cache/trivy ."

.PHONY: controller-image-scan
controller-image-scan: ## Scan the qcc-controller image for vulnerabilities.
	@mkdir -p build/ci/.cache/trivy && docker save $(IMG) -o build/ci/.cache/trivy/image.tar
	@$(MAKE) --no-print-directory op-scan TRIVY_ARGS="image --cache-dir build/ci/.cache/trivy --input build/ci/.cache/trivy/image.tar --scanners vuln --severity HIGH,CRITICAL --ignore-unfixed"
	@rm -f build/ci/.cache/trivy/image.tar

.PHONY: executor-image-scan
executor-image-scan: ## Scan the qcc-executor image for vulnerabilities.
	@mkdir -p build/ci/.cache/trivy && docker save $(EXECUTOR_IMG) -o build/ci/.cache/trivy/image.tar
	@$(MAKE) --no-print-directory op-scan TRIVY_ARGS="image --cache-dir build/ci/.cache/trivy --input build/ci/.cache/trivy/image.tar --scanners vuln --severity HIGH,CRITICAL --ignore-unfixed"
	@rm -f build/ci/.cache/trivy/image.tar

.PHONY: images-scan
images-scan: controller-image-scan executor-image-scan ## Scan both images.

.PHONY: controller-image-inspect
controller-image-inspect: ## Check qcc-controller layer efficiency with dive.
	@$(MAKE) --no-print-directory op-image-inspect IMAGE_NAME=qcc-controller

.PHONY: executor-image-inspect
executor-image-inspect: ## Check qcc-executor layer efficiency with dive.
	@$(MAKE) --no-print-directory op-image-inspect IMAGE_NAME=qcc-executor

.PHONY: images-inspect
images-inspect: controller-image-inspect executor-image-inspect ## Check both images' efficiency.
