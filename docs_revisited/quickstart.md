# Quickstart

This is the shortest practical path to a working local QCC deployment and a successful circuit run.

The quickest supported local flow is the kind-based path driven by `make dist-up`.

## What This Gets You

By the end of this guide you will have:

- a local kind cluster
- the controller and executor running
- sample QPUs registered
- the `qcc` CLI built locally
- a successful Bell-state run
- a drawing, a schedule, and a multi-backend comparison example

## 1. Install Tooling

```bash
make tools-install
make tools-check
```

Notes:

- Go is pinned by `go.mod`.
- most other tools are pinned in `.mise.toml`
- Python-side work uses `uv`

## 2. Bring Up The Local Stack

```bash
make dist-up
```

`dist-up` does all of the following:

- builds the controller image
- builds the executor image
- loads both into the local kind cluster
- installs the CRDs
- deploys the controller and executor manifests

Verify the namespace:

```bash
kubectl get pods -n quantum-circuit-controller-system
```

You should see both:

- controller-manager pod
- executor pod

## 3. Register Backends

QCC does not currently ship default QPUs in the operator install. Apply the sample backend catalog explicitly:

```bash
kubectl apply -k config/samples/qpu/
```

Verify:

```bash
kubectl get qpus
```

You should see a mix of:

- `aer-statevector`
- `fake-*` backends
- `ibm-*` backends

If you have not configured IBM credentials yet, the IBM entries may still appear available at the schema/status level, but real execution on them will fail later. For a first run, use `aer-statevector` or a `fake-*` backend.

## 4. Build The CLI

```bash
make qcc-build
```

This produces:

```bash
./dist/qcc
```

You can also use:

```bash
go run ./cmd/qcc --help
```

## 5. Run A Circuit

Use the Bell-state sample against the ideal reference backend:

```bash
./dist/qcc run examples/bell-state.qasm --backend aer-statevector
```

Expected behavior:

- the CLI creates a `Circuit`
- the controller selects `aer-statevector`
- the executor runs the circuit synchronously
- the CLI prints a result summary and histogram

Inspect the resulting resource:

```bash
./dist/qcc get circuits
./dist/qcc get circuit <name>
```

## 6. Draw A Circuit

```bash
./dist/qcc draw examples/bell-state.qasm
```

What happens:

- the CLI creates a short-lived `Circuit` with `mode=draw`
- the executor renders ASCII output
- the controller stores the drawing in a sibling `ConfigMap`
- the CLI prints the drawing

## 7. Schedule A Circuit

Use a backend with topology and timing information:

```bash
./dist/qcc schedule examples/bell-state.qasm --backend fake-brisbane
```

What happens:

- the CLI creates a short-lived `Circuit` with `mode=schedule`
- the controller resolves the requested QPU
- the executor produces a scheduled timeline
- the controller stores the JSON schedule artifact
- the CLI renders a terminal-friendly schedule view

## 8. Compare Multiple Backends

```bash
./dist/qcc run examples/bell-state.qasm --performance-test
```

This submits the same source to every available simulator QPU, waits for them all to finish, and prints a comparison table.

This is the current practical backend-comparison feature in QCC. It is more representative of the current system than `mode=select`, which still uses first-match selection.

## 9. Optional: Bring Up Observability

If you want Prometheus/Grafana/Collector locally:

```bash
make platform-up
```

Useful follow-up:

```bash
kubectl port-forward -n monitoring svc/kps-grafana 3000:80
```

Then inspect the dashboards defined under:

- [`../deploy/grafana/qcc-circuit-dashboard.yaml`](../deploy/grafana/qcc-circuit-dashboard.yaml)
- [`../deploy/grafana/qcc-qpu-dashboard.yaml`](../deploy/grafana/qcc-qpu-dashboard.yaml)

## 10. Optional: Enable IBM Hardware

Create the secret expected by the executor deployment:

```bash
kubectl create secret generic ibm-quantum-token \
  -n quantum-circuit-controller-system \
  --from-literal=QISKIT_IBM_TOKEN='<your-token>'
```

Then restart the executor or redeploy so the env vars are present:

```bash
kubectl rollout restart deployment/quantum-circuit-controller-executor \
  -n quantum-circuit-controller-system
```

Important current limitation:

- credentials are loaded through executor env vars
- `QPU.spec.access.credentialSecretRef` is not the live runtime path yet

## Useful Next Commands

```bash
./dist/qcc get qpus
./dist/qcc get qpu fake-brisbane
./dist/qcc get circuit <name> --qasm
./dist/qcc get circuit <name> --draw
./dist/qcc get circuit <name> --schedule
```

## Tear Down

```bash
make dist-down
make platform-down
```

## If Something Fails

Start here:

```bash
kubectl get pods -n quantum-circuit-controller-system
kubectl get qpus
kubectl get circuits
kubectl get circuit <name> -o yaml
kubectl logs -n quantum-circuit-controller-system deployment/quantum-circuit-controller-controller-manager -c manager
kubectl logs -n quantum-circuit-controller-system deployment/quantum-circuit-controller-executor
```

Then read:

- [`architecture.md`](./architecture.md)
- [`observability.md`](./observability.md)
- [`operations.md`](./operations.md)
