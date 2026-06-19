# Quantum Circuit Controller

A Kubernetes controller for running quantum circuits. `Circuit` and `QPU` custom
resources let you submit OpenQASM 3 or Qiskit-Python workloads to Aer simulators,
fake IBM backends, or real IBM hardware, with controller-side metrics exported to
Prometheus and Grafana.

- `Circuit` is the user request and lifecycle record.
- `QPU` is the backend registry plus probed backend metadata.
- A Go controller owns orchestration; a Python executor owns Qiskit, adapters,
  provider calls, and artifacts.


## Requirements

`make dist-up` builds both images, loads them into a local kind cluster, and
deploys. You provide these on the host:

- **A container runtime** — Docker, or a Docker-compatible engine such as
  [Colima](https://github.com/abiosoft/colima) or Podman exposing the Docker
  socket (`CONTAINER_TOOL` defaults to `docker`).
- **Go** — version is pinned by `go.mod`; any Go on `PATH` with
  `GOTOOLCHAIN=auto` auto-fetches it.
- **[mise](https://mise.jdx.dev/)** — provisions everything else pinned in
  `.mise.toml` (`kubectl`, `kind`, `helm`, `kustomize`, `buf`, `golangci-lint`,
  `uv`, `controller-gen`) via `make tools-install`.

## Quickstart

The shortest supported local path is the kind-based flow driven by `make dist-up`.

```bash
make tools-install
make tools-check
make dist-up
kubectl apply -k config/samples/qpu/
make qcc-build
./dist/qcc run examples/bell-state.qasm --backend aer-statevector
```

Verify the deployment:

```bash
kubectl get pods -n quantum-circuit-controller-system
kubectl get qpus
./dist/qcc get circuits
```

Tear down:

```bash
make dist-down
make platform-down
```

## Commands

```bash
qcc run examples/bell-state.qasm                       # execute on a selected backend
qcc run examples/bell-state.qasm --backend fake-brisbane
qcc run examples/bell-state.qasm --detach              # async submit (IBM hardware)
qcc run examples/bell-state.qasm --performance-test    # compare across QPUs
qcc draw examples/bell-state.qasm                      # render circuit, no execution
qcc schedule examples/bell-state.qasm --backend fake-brisbane
qcc get circuits
qcc get circuit <name> --qasm | --draw | --schedule
qcc get qpus
```

## Documentation

Detailed docs live in [`docs/`](./docs/README.md):

| Doc | Purpose |
|---|---|
| [`docs/README.md`](./docs/README.md) | implementation guide, command reference, status |
| [`docs/architecture.md`](./docs/architecture.md) | runtime topology, request flow, controller/executor split |
| [`docs/api.md`](./docs/api.md) | `Circuit` and `QPU` contract, fields, status, artifacts |
| [`docs/observability.md`](./docs/observability.md) | metrics, logs, dashboards, debugging flow |
