# Local dev platform: a long-lived kind cluster with the observability stack
# QCC's metrics land in, plus the loops that deploy onto it.

##@ Local Dev Platform (kind + observability stack)

# One environment per branch, like the image tags. IMAGE_TAG is the
# DNS-safe form of the branch or tag.
DEV_CLUSTER ?= qcc-dev-$(IMAGE_TAG)
DEV_NS      ?= monitoring

.PHONY: platform-up
platform-up: ## Bring up the kind cluster + observability stack (kps + OTel Collector)
	@if ! kind get clusters 2>/dev/null | grep -qx "$(DEV_CLUSTER)"; then \
		kind create cluster --name "$(DEV_CLUSTER)" --config deploy/platform/kind-config.yaml; \
	fi
	@helm repo add prometheus-community https://prometheus-community.github.io/helm-charts 2>/dev/null || true
	@helm repo add open-telemetry       https://open-telemetry.github.io/opentelemetry-helm-charts 2>/dev/null || true
	@helm repo update prometheus-community open-telemetry
	@kubectl get ns $(DEV_NS) >/dev/null 2>&1 || kubectl create ns $(DEV_NS)
	helm upgrade --install kps     prometheus-community/kube-prometheus-stack -n $(DEV_NS) -f deploy/platform/kps-values.yaml     --wait
	helm upgrade --install otelcol open-telemetry/opentelemetry-collector     -n $(DEV_NS) -f deploy/platform/otelcol-values.yaml --wait
	@echo ""
	@echo "Platform up. Useful next steps:"
	@echo "  Grafana (admin/admin): kubectl port-forward -n $(DEV_NS) svc/kps-grafana 3000:80"
	@echo "  OTLP gRPC ingress    : kubectl port-forward -n $(DEV_NS) svc/otelcol-opentelemetry-collector 4317:4317"

.PHONY: platform-down
platform-down: ## Tear down the platform stack and the kind cluster
	-helm uninstall otelcol -n $(DEV_NS)
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

# Loading lives here, with the cluster it loads into.
.PHONY: controller-image-load
controller-image-load: ## Load the qcc-controller image into the local kind cluster.
	kind load docker-image $(IMG) --name $(DEV_CLUSTER)

.PHONY: executor-image-load
executor-image-load: ## Load the qcc-executor image into the local kind cluster.
	kind load docker-image $(EXECUTOR_IMG) --name $(DEV_CLUSTER)

.PHONY: images-load
images-load: controller-image-load executor-image-load ## Load both images into kind.

##@ Dev loop

.PHONY: dev-up
dev-up: platform-up install ## Bring up dev platform + install CRDs.
	@echo "Dev platform ready. Run 'make run' to start the controller against this cluster."

.PHONY: dev-down
dev-down: platform-down ## Tear down the dev platform.

.PHONY: dist-up
dist-up: images-build images-load install deploy ## Build both images, load into kind, deploy; safe to re-run to pick up changes.
	@# Images rebuild under the same tag, so apply alone does not roll the pods.
	@kubectl -n quantum-circuit-controller-system rollout restart deployment
	@kubectl -n quantum-circuit-controller-system rollout status deployment --timeout=180s
	@echo ""
	@echo "QCC deployed to $(DEV_CLUSTER). Inspect with:"
	@echo "  kubectl get pods -n quantum-circuit-controller-system"
	@echo "  kubectl get circuits"
	@echo "Apply a sample:"
	@echo "  kubectl apply -k config/samples/qpu/local/   # register simulators first"
	@echo "  kubectl apply -f config/samples/circuits/bell.yaml"

.PHONY: dist-down
dist-down: undeploy uninstall ## Remove deployment and CRDs from the kind cluster.
