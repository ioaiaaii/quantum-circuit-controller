# Getting started

This tutorial takes you from a clean machine to a circuit running on a
simulator, then to the dashboards, then to real IBM Quantum hardware. Each
step tells you what to expect before you move on.

## Before you begin

### Requirements

You need a container runtime, Docker or a compatible engine such as
[Colima](https://github.com/abiosoft/colima) or Podman, and any recent
[Go](https://go.dev/dl/), which fetches the version in `go.mod` on first
build. Every other
tool is pinned in `.mise.toml`, and the scaffold's own tools install
into `./bin` on first use.

From a checkout, initialise the build submodule first, since every make
target includes from it, then trust the pinned toolchain config and
install it:

```bash
git submodule update --init
mise trust
make tools-install
make tools-check
```

Python dependencies are locked in `uv.lock`. CI installs from the same
files, so a local run matches what CI and the published evaluation ran
on.

## Bring up the platform

The platform target creates a kind cluster named `qcc-dev-<branch>`,
one per branch, and installs kube-prometheus-stack and the OpenTelemetry
Collector into the `monitoring` namespace. Apply the dashboards
afterwards:

```bash
make platform-up
kubectl apply -f deploy/grafana/
```

The dashboards are ConfigMaps carrying the label `grafana_dashboard: "1"`,
which Grafana's sidecar picks up within a minute.

You need this stack to see the `qcc_*` metrics, but QCC does not depend on
it. Without a Collector the controller logs export errors and everything
else keeps working. Leave `controller.otel.endpoint` unset to keep
the exporter off.

## Deploy QCC

Install the chart from the OCI registry, wiring the controller's OTLP
exporter to the Collector from the previous step:

```bash
helm install qcc oci://ghcr.io/ioaiaaii/charts/qcc \
  -n qcc-system --create-namespace \
  --set controller.otel.endpoint=otelcol-opentelemetry-collector.monitoring.svc.cluster.local:4317
```

This installs the CRDs and deploys the controller and the executor into
the `qcc-system` namespace, pulling the released images. Confirm two
pods are running:

```bash
kubectl get pods -n qcc-system
```

To build and deploy from source instead, use `make dist-up`; the
[development guide](./development.md) covers that flow.

If a pod does not reach `Running`, check its logs and events. The
[troubleshooting section](./operations.md#troubleshooting) covers the
common causes.

## Register backends

A fresh deployment has no QPUs, so backend selection fails with
`NoEligibleBackend` until you register one. The sample bundle holds two
kinds of backend, and they differ in what they need from you.

Simulators (`spec.kind: simulator`) run inside the executor and need no
credentials. They are `aer-statevector`, an ideal noise-free reference,
and eight `fake_*` snapshots that replay frozen calibration data from real
IBM devices, so they reproduce a device's basis gates, coupling map, and
noise without touching it.

Real hardware (`spec.kind: hardware`) profiles live in
`config/samples/qpu/ibm/`: `ibm-fez`, `ibm-kingston`, and
`ibm-marrakesh`, each pointing at a live IBM device through
`spec.backendName`. These need an IBM Quantum account and are registered
in the [hardware section](#run-on-real-ibm-hardware) below.

Register the whole simulator bundle straight from the repository, no
checkout needed:

```bash
kubectl apply -k "github.com/ioaiaaii/quantum-circuit-controller/config/samples/qpu/local?ref=main"
```

Or register a single backend, here the ideal reference simulator:

```bash
kubectl apply -f https://raw.githubusercontent.com/ioaiaaii/quantum-circuit-controller/main/config/samples/qpu/local/aer-statevector.yaml
```

From a checkout, `kubectl apply -k config/samples/qpu/local/` does the
same.

The controller probes each backend through the executor as it is
registered, and records the qubit count, basis gates, coupling edges,
error medians, coherence times, and calibration date on the QPU's status.
List the registry once the probes settle:

```bash
kubectl get qpus
```

<img alt="kubectl get qpu listing: twelve backends with availability, processor family, qubit count, 2Q error, T1, dt, and calibration date" src="./assets/figures/qpu_get_all.webp" width="650">

Simulators fill in immediately. The IBM entries report `Available` whether
or not credentials exist, because the controller is optimistic about
providers it knows. Without a token their probe fails and the calibration
columns stay empty, which is how you tell them apart.

To see everything the probe found for one backend:

```bash
kubectl get qpu fake-brisbane -o yaml
```

## Run your first circuit

Download the released CLI and its checksums for your platform:

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
curl -LO "https://github.com/ioaiaaii/quantum-circuit-controller/releases/latest/download/qcc-${OS}-${ARCH}"
curl -LO "https://github.com/ioaiaaii/quantum-circuit-controller/releases/latest/download/SHA256SUMS"
```

Verify the binary against the checksum file, then install it:

```bash
grep "qcc-${OS}-${ARCH}" SHA256SUMS | shasum -a 256 -c -
```

```bash
chmod +x "qcc-${OS}-${ARCH}" && mv "qcc-${OS}-${ARCH}" qcc
```

Confirm the binary runs and reports the release version:

```bash
./qcc version
```

Submit a Bell state to the ideal simulator:

```bash
./qcc run examples/bell-state.qasm --backend aer-statevector
```

Releases also carry GitHub build provenance for every binary.
`go install github.com/ioaiaaii/quantum-circuit-controller/cmd/qcc@latest`
builds the same CLI through the module proxy, and `make qcc-build` from a
checkout.

The CLI creates a Circuit, streams the phase transitions, and prints a
_result card_: the backend's calibration context, the transpiled depth and
gate counts, an error-exposure verdict, and the measurement histogram. For
a Bell state you should see roughly half the shots on `00` and half on
`11`. The card looks like this, here for the larger Shor workload on the
same backend:

<img alt="Result card for a Shor run on aer-statevector: completed in 510 ms, transpiled depth 506, outcomes 0000:517 and 1000:507 of 1024 shots" src="./assets/figures/circuit_run_shor_aer_v1.webp" width="640">

Run the same circuit against a noisy calibration snapshot and compare the
histogram:

```bash
./qcc run examples/bell-state.qasm --backend fake-brisbane
```

Try the other modes now that a backend is registered:

```bash
./qcc draw examples/bell-state.qasm
./qcc schedule examples/bell-state.qasm --backend fake-brisbane
./qcc run examples/thesis/algorithms/shor.py --performance-test
./qcc get circuits
```

## Look at the dashboards

```bash
kubectl port-forward -n monitoring svc/kps-grafana 3000:80
```

Open http://localhost:3000 and sign in as `admin` / `admin`. Two
dashboards ship with QCC. **QCC · QPU substrate health** shows
availability, error medians, coherence times, and calibration age across
every registered backend. **QCC · Circuit detail** shows one circuit's
identity, transpile shape, phase timing, outcome distribution, and a link
to its provider job.

Every panel queries the `qcc_*` metrics described in the
[metrics reference](./observability.md).

## Run on real IBM hardware

This step is optional and needs an IBM Quantum Platform account. Create a
secret with your API token and restart the executor so it picks the
credential up:

```bash
kubectl create secret generic ibm-quantum-token \
  -n qcc-system \
  --from-literal=QISKIT_IBM_TOKEN='<your-token>'

kubectl rollout restart deployment/qcc-executor -n qcc-system
```

Register the hardware profiles:

```bash
kubectl apply -k "github.com/ioaiaaii/quantum-circuit-controller/config/samples/qpu/ibm?ref=main"
```

Hardware queues take minutes to hours, so submit with `--detach` and
come back later:

```bash
./qcc run examples/thesis/algorithms/shor.py --backend ibm-fez --detach
./qcc get circuits
./qcc get circuit <circuit-name>
```

where `<circuit-name>` is the name the CLI printed when it submitted the
Circuit.

<img alt="Detached submission output: Circuit name, UID, and provider job ID printed once the job is queued on ibm-kingston" src="./assets/figures/cli_detach_submission_kingston.webp" width="600">

The controller keeps polling the vendor queue and records the counts on
the Circuit when the job finishes. You get the provider job ID in
`status.providerJobId`, and the executor stamps the Circuit's UID onto the
IBM job as a tag, so you can move between the two systems in either
direction.

The executor keeps its task registry in memory, so restarting it mid-queue
orphans the watch, and IBM QPUs report as `Available` even when a probe
fails. Both appear under
[known limitations](./operations.md#known-limitations).

## Cleaning up

The first command removes QCC. Deleting the CRDs afterwards also
deletes every Circuit and QPU you created. The last removes the
observability stack and the kind cluster.

```bash
helm uninstall qcc -n qcc-system
kubectl delete crd circuits.qcc.io qpus.qcc.io
make platform-down
```

## What's next

The [demonstration](./demonstration.md) walks the whole platform with one
workload, from simulators through real hardware to the dashboards. The
[CLI reference](./cli.md) documents every command and flag, the
[API reference](./api.md) the Circuit and QPU fields, and the
[operations guide](./operations.md) deployment, credentials, and
troubleshooting.
