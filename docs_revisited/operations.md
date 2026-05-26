# Operations

This document covers the practical way to run, inspect, and test QCC as it exists today.

## Recommended Modes Of Use

| Goal | Recommended path |
|---|---|
| fastest local bring-up | `make dist-up` |
| work on controller code | `make run` against a prepared cluster |
| work on executor code | run `qcc-executor` locally or rebuild the executor image |
| inspect behavior as a user | `qcc run`, `qcc draw`, `qcc schedule`, `qcc get` |
| compare backends | `qcc run --performance-test` |

## Repository Areas

- Go controller: [`../cmd/qcc-controller/`](../cmd/qcc-controller)
- CLI: [`../cmd/qcc/`](../cmd/qcc)
- Python executor: [`../qcc-executor/`](../qcc-executor)
- CRDs and manifests: [`../config/`](../config)
- sample circuits: [`../config/samples/`](../config/samples)
- sample QPUs: [`../config/samples/qpu/`](../config/samples/qpu)
- observability platform: [`../deploy/platform/`](../deploy/platform)

## Tooling

The repository expects:

- Go toolchain pinned by `go.mod`
- additional tools pinned in `.mise.toml`
- Python dependency management through `uv`

Useful targets:

```bash
make tools-install
make tools-check
make help
```

## Deploying The Operator

Basic deploy path:

```bash
make deploy IMG=<your-controller-image>
```

Important current behavior:

- the default deploy includes both the controller-manager and the executor manifests
- the operator-default QPU bundle is empty
- `make deploy` does not register ready-to-use sample QPUs for you

After deploying the operator, explicitly apply QPUs:

```bash
kubectl apply -k config/samples/qpu/
```

## Deploying The Executor

Executor manifests live in [`../config/manager/executor.yaml`](../config/manager/executor.yaml).

The executor is a separate `Deployment` plus `Service` and is expected to be reachable by the controller through `QCC_EXECUTOR_ADDR`.

The default deploy path already includes this manifest through [`../config/default/kustomization.yaml`](../config/default/kustomization.yaml). This section exists so the runtime shape and credential model are explicit.

## Recommended Local Workflow

For local development on the default kind-based path:

```bash
make dist-up
kubectl apply -k config/samples/qpu/
make qcc-build
./dist/qcc run examples/bell-state.qasm --backend aer-statevector
```

This is the most reliable path because it keeps the controller and executor deployment model close to what the repo expects.

## IBM Credentials

The current IBM path is configured through executor environment variables, not through `QPU.spec.access.credentialSecretRef`.

Create the secret expected by the executor deployment:

```bash
kubectl create secret generic ibm-quantum-token \
  -n quantum-circuit-controller-system \
  --from-literal=QISKIT_IBM_TOKEN='<your-token>'
```

Optional channel override:

```bash
kubectl create secret generic ibm-quantum-token \
  -n quantum-circuit-controller-system \
  --from-literal=QISKIT_IBM_TOKEN='<your-token>' \
  --from-literal=QISKIT_IBM_CHANNEL='ibm_quantum_platform'
```

## Registering QPUs

Sample QPUs are split into:

- ideal reference simulator: `aer-statevector`
- fake IBM snapshots: `fake-brisbane`, `fake-sherbrooke`, `fake-osaka`, `fake-kyoto`, `fake-torino`, `fake-marrakesh`, `fake-belem`
- live IBM samples: `ibm-fez`, `ibm-kingston`, `ibm-marrakesh`

Apply all sample QPUs:

```bash
kubectl apply -k config/samples/qpu/
```

Inspect them:

```bash
kubectl get qpus
qcc get qpus
qcc get qpu fake-brisbane
```

## CLI Usage

### Run a circuit

```bash
qcc run examples/bell-state.qasm
qcc run examples/shor.py --shots 4096
qcc run examples/bell-state.qasm --backend fake-brisbane
qcc run examples/bell-state.qasm --detach
qcc run examples/bell-state.qasm --algorithm bell --version v1
```

### Compare backends empirically

```bash
qcc run examples/bell-state.qasm --performance-test
qcc run examples/bell-state.qasm --performance-test --include-hardware
```

This is the current practical backend-comparison feature. It submits the same source to multiple QPUs and prints a comparison table after they finish.

### Draw a circuit

```bash
qcc draw examples/bell-state.qasm
qcc draw examples/shor.py --keep
```

### Schedule a circuit

```bash
qcc schedule examples/bell-state.qasm --backend fake-brisbane
qcc get circuit <name> --schedule
```

### Inspect resources

```bash
qcc get circuits
qcc get circuit <name>
qcc get circuit <name> --qasm
qcc get circuit <name> --draw
qcc get qpus
qcc get qpu <name>
```

### Label-based grouping

The CLI supports a small grouping model for repeated runs:

```bash
qcc run examples/bell-state.qasm --algorithm bell
qcc run examples/bell-state.qasm --algorithm bell --version v2
qcc get circuits --algorithm bell
```

The controller auto-fills:

- `qcc.io/run-index`
- `qcc.io/source-sha256`

## Observability Stack

Bring up a local monitoring stack:

```bash
make platform-up
make platform-status
```

This provisions:

- kube-prometheus-stack
- Tempo
- OpenTelemetry Collector

Dashboard and collector configuration lives under [`../deploy/platform/`](../deploy/platform).

Useful local access:

```bash
kubectl port-forward -n monitoring svc/kps-grafana 3000:80
```

## Testing

### Go tests

Main target:

```bash
make test
```

Notes:

- this runs `manifests`, `generate`, `fmt`, and `vet` first
- it requires `controller-gen` on `PATH`
- envtest assets must be available through `setup-envtest`
- envtest opens local ports, so some sandboxed environments will block it

### Executor tests

```bash
cd qcc-executor
uv run pytest -q
```

Notes:

- first-time runs may need network access to fetch Python build dependencies
- local cache location can matter in sandboxed environments

### E2E tests

```bash
make test-e2e
```

These require a dedicated kind cluster and should not be pointed at a real shared cluster.

## Current Operational Caveats

### No default QPU after deploy

If you deploy the operator and then submit a `Circuit` immediately, selection will fail unless you applied at least one sample or custom `QPU`.

### IBM QPUs may look healthier than they are

The `QPUReconciler` currently treats IBM providers as optimistically `Available`. Missing credentials or probe errors do not automatically remove them from selection.

### Async jobs are fragile across executor restarts

If the executor restarts during a queued or running hardware job, QCC cannot currently resume the in-flight task from stored provider job ID alone.

### Per-QPU credential references are not active

`QPU.spec.access.credentialSecretRef` is part of the schema but not part of the runtime path today.

### Some selector fields are not wired

`allowedQPURefs` and `region` should not be treated as enforced scheduling constraints yet.

## After Making Changes

If you touch CRD types or kubebuilder markers:

```bash
make manifests
make generate
```

If you touch Go code:

```bash
make lint-fix
make test
```

If you touch executor code:

```bash
make executor-test
make executor-build
```

## Good Files To Inspect First

If you are trying to understand or change the runtime behavior, start here:

- [`../internal/controller/circuit_controller.go`](../internal/controller/circuit_controller.go)
- [`../internal/controller/qpu_controller.go`](../internal/controller/qpu_controller.go)
- [`../internal/executor/client.go`](../internal/executor/client.go)
- [`../qcc-executor/src/qcc_executor/servicer.py`](../qcc-executor/src/qcc_executor/servicer.py)
- [`../qcc-executor/src/qcc_executor/adapters/aer.py`](../qcc-executor/src/qcc_executor/adapters/aer.py)
- [`../qcc-executor/src/qcc_executor/adapters/ibm.py`](../qcc-executor/src/qcc_executor/adapters/ibm.py)
