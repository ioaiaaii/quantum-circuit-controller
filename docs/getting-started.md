# Getting started

This tutorial takes you from a clean machine to a circuit running on a
simulator, then to the dashboards, then to real IBM Quantum hardware. Each
step tells you what to expect before you move on.

## Before you begin

### Requirements

| Requirement | Pinned in | Provided by |
|---|---|---|
| A container runtime: Docker, or a compatible engine such as [Colima](https://github.com/abiosoft/colima) or Podman | not pinned | your system; `CONTAINER_TOOL` defaults to `docker` |
| Go | `go.mod` (`toolchain` directive) | any recent Go on `PATH` fetches the pinned version |
| kubectl, kind, helm, kubebuilder, buf, python, uv, jq, make, lychee, vhs | `.mise.toml` | `make tools-install` |
| kustomize, controller-gen, setup-envtest, golangci-lint | `Makefile` | installed into `./bin` on first use |

Install the toolchain and check it:

```bash
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
else keeps working. To turn the SDK off entirely, set
`OTEL_SDK_DISABLED=true` on the controller Deployment.

## Deploy QCC

```bash
make dist-up
```

This builds both images, loads them into the kind cluster, installs the
CRDs, and deploys the controller and the executor into the
`quantum-circuit-controller-system` namespace. Confirm two pods are
running:

```bash
kubectl get pods -n quantum-circuit-controller-system
```

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
`config/samples/qpu/ibm/`: `ibm-fez`, `ibm-kingston`, `ibm-marrakesh`,
and `ibm-sherbrooke`, each pointing at a live IBM device through
`spec.backendName`. These need an IBM Quantum account and are registered
in the [hardware section](#run-on-real-ibm-hardware) below.

Register the simulators:

```bash
kubectl apply -k config/samples/qpu/local/
```

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

Build the CLI and submit a Bell state to the ideal simulator:

```bash
make qcc-build
./dist/qcc run examples/bell-state.qasm --backend aer-statevector
```

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
./dist/qcc run examples/bell-state.qasm --backend fake-brisbane
```

Try the other modes now that a backend is registered:

```bash
./dist/qcc draw examples/bell-state.qasm
./dist/qcc schedule examples/bell-state.qasm --backend fake-brisbane
./dist/qcc run examples/thesis/algorithms/shor.py --performance-test
./dist/qcc get circuits
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
  -n quantum-circuit-controller-system \
  --from-literal=QISKIT_IBM_TOKEN='<your-token>'

kubectl rollout restart deployment/quantum-circuit-controller-executor \
  -n quantum-circuit-controller-system
```

Register the hardware profiles:

```bash
kubectl apply -k config/samples/qpu/ibm/
```

Hardware queues take minutes to hours, so submit with `--detach` and
come back later:

```bash
./dist/qcc run examples/thesis/algorithms/shor.py --backend ibm-fez --detach
./dist/qcc get circuits
./dist/qcc get circuit <circuit-name>
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

The first target removes QCC and its CRDs, which also deletes every
Circuit and QPU you created. The second deletes the observability stack
and the kind cluster.

```bash
make dist-down
make platform-down
```

## What's next

The [demonstration](./demonstration.md) walks the whole platform with one
workload, from simulators through real hardware to the dashboards. The
[CLI reference](./cli.md) documents every command and flag, the
[API reference](./api.md) the Circuit and QPU fields, and the
[operations guide](./operations.md) deployment, credentials, and
troubleshooting.
