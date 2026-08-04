# Operations

Day-2 reference: deploying, configuring, sizing, troubleshooting, and the
limitations to plan around. Written for the person running QCC rather than
the person developing it.

## Deployment

The shipped deployment path is kustomize:

```bash
make deploy IMG=<controller-image>       # kustomize build config/default | kubectl apply
kubectl apply -k config/samples/qpu/     # register backends; deploy ships none
```

`config/default` composes the CRDs, RBAC, the controller Deployment, the
executor Deployment and Service, and the metrics Service, all under the
`quantum-circuit-controller-` name prefix in the
`quantum-circuit-controller-system` namespace.

No QPUs are installed by default. The operator-defaults directory,
`config/qpu/`, is intentionally empty, so every backend is an explicit
registration from `config/samples/qpu/` or from your own manifests. Until
one QPU reports `Available`, a `mode=run` Circuit fails with
`NoEligibleBackend`.

The observability stack is deployed separately. `make platform-up` manages
the kind cluster and the monitoring namespace for local development. On a
real cluster, bring your own Prometheus, Grafana, and Collector, and point
the controller's `OTEL_EXPORTER_OTLP_ENDPOINT` at that Collector.

For the local kind flow see
[tutorial](./getting-started.md).

## Configuration reference

### Controller (`quantum-circuit-controller-controller-manager`)

| Env var | Default | Meaning |
|---|---|---|
| `QCC_EXECUTOR_ADDR` | `quantum-circuit-controller-executor:9000` | executor gRPC target |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | in-cluster Collector DNS `:4317` | OTLP/gRPC metrics target |
| `OTEL_EXPORTER_OTLP_INSECURE` | `true` | plaintext OTLP (in-cluster convention) |
| `OTEL_SDK_DISABLED` | unset | `true` disables all OTel setup |
| `OTEL_SERVICE_NAME` | `qcc-controller` | `service.name` resource attribute |
| `OTEL_SERVICE_VERSION` | unset | `service.version` resource attribute |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` (slog JSON to stdout) |
| `K8S_POD_NAME`, `K8S_POD_UID`, `K8S_NAMESPACE_NAME`, `K8S_NODE_NAME` | downward API | pod identity on every metric |

Flags: `--leader-elect` (on in the shipped manifest),
`--health-probe-bind-address=:8081` (liveness `/healthz`, readiness
`/readyz`), `--metrics-bind-address` (controller-runtime's own endpoint;
the `qcc_*` metrics do not use it).

### Executor (`quantum-circuit-controller-executor`)

| Env var | Default | Meaning |
|---|---|---|
| `QCC_EXECUTOR_ADDR` | `0.0.0.0:9000` | gRPC bind address |
| `QCC_EXECUTOR_WORKERS` | `8` | gRPC thread-pool size |
| `QCC_EXECUTOR_LOG_LEVEL` | `INFO` | Python logging level |
| `QISKIT_IBM_TOKEN` | unset | IBM Quantum Platform API token (from Secret, optional) |
| `QISKIT_IBM_CHANNEL` | `ibm_quantum_platform` | override for `ibm_cloud` accounts |

## IBM credentials

Credentials are executor-level environment, not per-QPU. The
`QPU.spec.access.credentialSecretRef` field exists on the schema but is
not consumed by the runtime; one token serves all `provider: ibm` QPUs.

```bash
kubectl create secret generic ibm-quantum-token \
  -n quantum-circuit-controller-system \
  --from-literal=QISKIT_IBM_TOKEN='<your-token>'

kubectl rollout restart deployment/quantum-circuit-controller-executor \
  -n quantum-circuit-controller-system
```

The Secret is referenced with `optional: true`, so clusters without it
start cleanly; the `IBMAdapter` then refuses construction and
IBM-targeted Circuits fail with `NoEligibleBackend` while Aer-backed
Circuits are unaffected.

## Security posture

QCC v1.0.x assumes a single-tenant, trusted cluster. Four properties
follow from that assumption and matter before deploying anywhere shared.

Circuit sources are code. A `source.format: qiskit` body is executed with
`exec()` inside the executor pod, so whoever can create a Circuit can run
Python there. This is a deliberate trust decision, since circuit sources
are programs by nature, and it holds only while Circuit authors and
cluster operators sit in the same trust domain.

The controller-executor channel is plaintext gRPC on a ClusterIP Service,
and the executor performs no caller authentication, so anything with
in-cluster network reach to port 9000 can submit work. Mutual TLS is
future work, and until it lands NetworkPolicy is the available control.

One IBM token serves the whole cluster, mounted into the executor from a
Secret. Per-QPU and per-tenant credential isolation is not implemented;
`credentialSecretRef` exists on the schema only.

The pods are hardened even though the boundary is not. Both deployments
satisfy the restricted Pod Security Standard, the controller image is
distroless, and RBAC is scoped to the QCC resources. None of that
substitutes for the tenancy boundary above.

Multi-tenant deployment therefore needs executor sandboxing or per-tenant
executors, mTLS on the gRPC seam, per-tenant credentials, and
NetworkPolicy, all of which remain future work.

## Scaling and sizing

Run exactly one executor replica. The async task registry (task ID to job
handle) lives in executor process memory. With more than one replica
behind the Service, `SubmitTask` can land on one pod and the follow-up
`WatchTask` or `FetchTaskResult` on another, which returns `TaskNotFound`
for a job that is actually running. Scale vertically instead; horizontal
scaling needs the durable registry listed under future work.

The controller runs a single replica with leader election on; additional
replicas are safe standbys.

The controller is light, with shipped requests of `10m` CPU and `64Mi`
memory. The executor is where memory goes, because Aer statevector
simulation is exponential in qubit count: roughly 16 bytes times 2^n
amplitudes, which puts 25 qubits near 0.5 GiB and 26 near 1 GiB. The
shipped limit of `1Gi` covers thesis-scale circuits, and simulating beyond
about 25 statevector qubits needs it raised. A `fake_*` backend is Aer
plus a noise model, so it holds the same memory envelope and costs more
CPU.

## Health

The controller serves `/healthz` and `/readyz` over HTTP on port 8081,
both wired to Kubernetes probes. The executor is checked with TCP-socket
probes on port 9000; a gRPC health-checking service is planned, so an open
port is the strongest liveness signal available today.

## Upgrades

1. `make manifests` and `kubectl apply` for CRD changes (additive fields
   only in the v1alpha1 line so far).
2. Roll the images (`make deploy IMG=...` or edit the Deployments).
3. Mind the executor restart limitation below when hardware jobs are in
   flight.

## Troubleshooting

Work down this list; each step names the surface that answers it. The
metric and status semantics behind these surfaces are in
[metrics reference](./observability.md).

### A Circuit never leaves `Pending`

- Is the controller running?
  `kubectl get pods -n quantum-circuit-controller-system`
- Controller logs:
  `kubectl logs -n quantum-circuit-controller-system deploy/quantum-circuit-controller-controller-manager`
- Is the Circuit in the namespace you think it is?

### A Circuit fails with `NoEligibleBackend`

- `kubectl get qpus`: is anything registered and `Available`?
- Selector mismatch: check `provider`, `backendName`, `kind`,
  `minQubits` against the QPU's actual fields. `backendName` matches
  either the QPU's Kubernetes name (`fake-brisbane`) or its
  provider-native name (`fake_brisbane`).
- `spec.shots` above the QPU's `capabilities.maxShots` also rejects it.
- `allowedQPURefs` and `region` do not narrow selection; both are
  schema-only fields.

### A Circuit fails with `TranspilationFailed` or `InvalidCircuit`

- The condition message carries the Qiskit error verbatim:
  `kubectl get circuit <name> -o jsonpath='{.status.conditions}'`
- Typical causes: the circuit needs more qubits than the backend has
  (`fake-belem` is 5-qubit); a Tier-2 passthrough key Qiskit rejects.

### An IBM submission fails

- Does the secret exist, and was the executor restarted after creating
  it?
- Executor logs:
  `kubectl logs -n quantum-circuit-controller-system deploy/quantum-circuit-controller-executor`
- Note the availability caveat: IBM QPUs show `Available` optimistically
  even when probing fails, so `Available` does not guarantee a working
  credential.

### A queued hardware run disappeared after an executor restart

Known limitation, not a field-fixable bug: the task registry is
in-memory. The vendor job itself is still running; find it in the IBM
console via the `qcc.circuit.uid:<uid>` job tag. The Circuit will not
converge, so delete it and resubmit, or record the counts manually from
the console.

### Metrics are missing in Prometheus

- Is the Collector reachable from the controller?
  (`OTEL_EXPORTER_OTLP_ENDPOINT`; controller logs show export errors.)
- Is Prometheus scraping the Collector? The kps values set
  `serviceMonitorSelectorNilUsesHelmValues: false` so the Collector's
  ServiceMonitor is picked up regardless of release labels.
- Gauges are observed from the controller's informer cache on each
  export cycle (30 s); a metric can lag a status change by up to one
  cycle.

## Known limitations

The bounds of the current release, ordered by operational impact. Each is
planned work rather than a permanent property.

1. An executor restart loses in-flight asynchronous tasks, because the
   registry is in memory. A durable, vendor-recoverable registry is
   future work.
2. A controller restart between a successful `SubmitTask` and the status
   patch recording `providerJobId` orphans the vendor job. The
   [idempotency key](./architecture.md#submission-and-the-cross-boundary-identifier)
   bounds duplicates rather than orphans.
3. The executor runs as a single replica, for the reason given under
   [scaling and sizing](#scaling-and-sizing).
4. IBM availability is optimistic, so a failed probe leaves the QPU
   selectable.
5. One credential serves the cluster; the per-QPU `credentialSecretRef`
   is not yet wired to the runtime.
6. Kubernetes Events are not emitted, so lifecycle detail lives in
   `status.conditions` and the logs.
7. Calibration is read at registration, so live-hardware values age
   between probes until TTL-based re-probing lands.
