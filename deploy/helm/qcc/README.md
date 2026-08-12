# qcc

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v1.1.1](https://img.shields.io/badge/AppVersion-v1.1.1-informational?style=flat-square)

Quantum Circuit Controller, a Kubernetes operator that runs quantum circuits as declarative resources.

The chart deploys two workloads: the controller, which reconciles
Circuit and QPU resources, and the executor, the Qiskit gRPC service
the controller dials for transpilation and execution. CRDs install
from the chart's `crds/` directory.

## Install

From the OCI registry:

```bash
helm install qcc oci://ghcr.io/ioaiaaii/charts/qcc --version 0.1.0 -n qcc-system --create-namespace
```

From a repository checkout:

```bash
helm install qcc deploy/helm/qcc -n qcc-system --create-namespace
```

## Run a first circuit

Register a local simulator QPU, submit the bell sample, and watch it
complete:

```bash
kubectl apply -f https://raw.githubusercontent.com/ioaiaaii/quantum-circuit-controller/v1.1.1/config/samples/qpu/local/aer-statevector.yaml
kubectl apply -f https://raw.githubusercontent.com/ioaiaaii/quantum-circuit-controller/v1.1.1/config/samples/circuits/bell.yaml
kubectl get circuits -w
```

Local simulator QPUs run without credentials. IBM Quantum hardware
reads a Secret referenced by `executor.env`:

```bash
kubectl create secret generic ibm-quantum-token -n qcc-system \
  --from-literal=QISKIT_IBM_TOKEN=<token> \
  --from-literal=QISKIT_IBM_CHANNEL=<channel>
```

## Upgrades and CRDs

Helm installs CRDs on first install and never upgrades or deletes
them. When a release changes the CRDs, apply them from the matching
tag before `helm upgrade`:

```bash
kubectl apply -k "github.com/ioaiaaii/quantum-circuit-controller/config/crd?ref=v1.1.1"
```

## Scaling limits

`executor.replicaCount` is fixed at 1: job results live in executor
process memory, and a second replica would receive fetches for jobs it
never ran. Rendering fails on any other value, and the executor
updates with a Recreate strategy for the same reason. Controller
replicas above 1 require `controller.leaderElection` to stay true.

## Uninstall

```bash
helm uninstall qcc -n qcc-system
```

Helm leaves the CRDs in place. Deleting them removes every Circuit and
QPU in the cluster:

```bash
kubectl delete crd circuits.qcc.io qpus.qcc.io
```

## Security

Both pods run as non-root under the RuntimeDefault seccomp profile
with all capabilities dropped. The controller's root filesystem is
read-only; the executor's stays writable for Qiskit caches. The
executor does not mount its ServiceAccount token.

The executor's gRPC endpoint carries no authentication: any workload
in the cluster can reach it through its Service. Restrict ingress to
the controller with a NetworkPolicy; the chart does not ship one.

## Requirements

Kubernetes: `>=1.26.0-0`

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| controller | object | see the controller.* keys below | Settings for the qcc-controller Deployment, the operator reconciling Circuits and QPUs. |
| controller.affinity | object | `{}` | Affinity rules for pod scheduling. (corev1.Affinity) |
| controller.containerSecurityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true}` | Container-level security context. (corev1.SecurityContext) |
| controller.extraEnvVars | list | `[]` | Environment variables appended to the controller container. (corev1.EnvVar[]) |
| controller.image.repository | string | `"ioaiaaii/qcc-controller"` | Repository (image name) within the registry. |
| controller.image.tag | string | `""` | Image tag to deploy. Leave empty to use the chart appVersion. |
| controller.leaderElection | bool | `true` | If true, enable leader election and its Role and RoleBinding. (bool) |
| controller.nodeSelector | object | `{}` | Node selector for scheduling. (map[string]string) |
| controller.otel.endpoint | string | `""` | OTLP gRPC endpoint for metrics and traces, e.g. otelcol.monitoring.svc.cluster.local:4317. Empty disables the exporter. (string) |
| controller.otel.insecure | bool | `true` | If true, ship OTLP without TLS. (bool) |
| controller.podAnnotations | object | `{}` | Extra annotations for controller pods. (map[string]string) |
| controller.podSecurityContext | object | `{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}` | Pod-level security context. (corev1.PodSecurityContext) |
| controller.priorityClassName | string | `""` | PriorityClass for controller pods. (string) |
| controller.replicaCount | int | `1` | Number of controller replicas. More than one requires leaderElection. (int) |
| controller.resources | object | `{"limits":{"cpu":"500m","memory":"128Mi"},"requests":{"cpu":"10m","memory":"64Mi"}}` | Container resource requests and limits. (corev1.ResourceRequirements) |
| controller.terminationGracePeriodSeconds | int | `10` | Graceful termination window for the controller. (int seconds) |
| controller.tolerations | list | `[]` | Tolerations for tainted nodes. (corev1.Toleration[]) |
| controller.topologySpreadConstraints | list | `[]` | Topology spread constraints. (corev1.TopologySpreadConstraint[]) |
| executor | object | see the executor.* keys below | Settings for the qcc-executor Deployment, the Qiskit gRPC service the controller dials for transpilation and execution. |
| executor.affinity | object | `{}` | Affinity rules for pod scheduling. (corev1.Affinity) |
| executor.automountServiceAccountToken | bool | `false` | Mount the ServiceAccount token. The executor never calls the Kubernetes API, so it is off by default. (bool) |
| executor.containerSecurityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":false}` | Container-level security context. Qiskit writes caches, so the root filesystem stays writable. (corev1.SecurityContext) |
| executor.env | list | `[{"name":"QCC_EXECUTOR_LOG_LEVEL","value":"INFO"},{"name":"QCC_EXECUTOR_WORKERS","value":"8"},{"name":"QISKIT_IBM_TOKEN","valueFrom":{"secretKeyRef":{"key":"QISKIT_IBM_TOKEN","name":"ibm-quantum-token","optional":true}}},{"name":"QISKIT_IBM_CHANNEL","valueFrom":{"secretKeyRef":{"key":"QISKIT_IBM_CHANNEL","name":"ibm-quantum-token","optional":true}}}]` | Environment for the executor container. QCC_EXECUTOR_LOG_LEVEL and QCC_EXECUTOR_WORKERS tune the service. The QISKIT_IBM_* pair reads an optional Secret for IBM Quantum hardware, local simulators need none. (corev1.EnvVar[]) |
| executor.image.repository | string | `"ioaiaaii/qcc-executor"` | Repository (image name) within the registry. |
| executor.image.tag | string | `""` | Image tag to deploy. Leave empty to use the chart appVersion. |
| executor.nodeSelector | object | `{}` | Node selector for scheduling. (map[string]string) |
| executor.podAnnotations | object | `{}` | Extra annotations for executor pods. (map[string]string) |
| executor.podSecurityContext | object | `{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}` | Pod-level security context. (corev1.PodSecurityContext) |
| executor.priorityClassName | string | `""` | PriorityClass for executor pods. (string) |
| executor.replicaCount | int | `1` | Number of executor replicas. Must stay 1: job results live in process memory, so a second replica would lose them. (int) |
| executor.resources | object | `{"limits":{"cpu":"2","memory":"1Gi"},"requests":{"cpu":"100m","memory":"256Mi"}}` | Container resource requests and limits. Transpilation is CPU-heavy. (corev1.ResourceRequirements) |
| executor.service.port | int | `9000` | gRPC port the Service exposes and the controller dials. (int) |
| executor.service.type | string | `"ClusterIP"` | Service type for the executor gRPC endpoint. (string) |
| executor.terminationGracePeriodSeconds | int | `10` | Graceful termination window for in-flight circuit jobs. (int seconds) |
| executor.tolerations | list | `[]` | Tolerations for tainted nodes. (corev1.Toleration[]) |
| executor.topologySpreadConstraints | list | `[]` | Topology spread constraints. (corev1.TopologySpreadConstraint[]) |
| executor.updateStrategy | object | `{"type":"Recreate"}` | Update strategy. Recreate: two executors must not overlap while results live in process memory. (object) |
| fullnameOverride | string | `""` | Override the full release name. Leave empty to use the chart's computed fullname. |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy. One of: Always|IfNotPresent|Never. |
| image.pullSecrets | list | `[]` | List of imagePullSecrets to use for private registries. Example: ["my-regcred"] |
| image.registry | string | `"ghcr.io"` | Container registry hosting both images. |
| rbac.create | bool | `true` | If true, create the controller ClusterRole, Role, and bindings. (bool) |
| serviceAccount.annotations | object | `{}` | Extra annotations for the ServiceAccount. (map[string]string) |
| serviceAccount.create | bool | `true` | If true, create the controller ServiceAccount. (bool) |
| serviceAccount.name | string | `""` | ServiceAccount name. Leave empty to derive from the fullname. (string) |

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| ioaiaaii |  | <https://github.com/ioaiaaii> |

## Source Code

* <https://github.com/ioaiaaii/quantum-circuit-controller>
