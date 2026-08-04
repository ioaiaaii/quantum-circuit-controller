# Getting Started

From a clean machine to a running circuit, then to the dashboards, then to
real IBM hardware. Every command is copy-paste ready; the expected outcome
follows each step. The command reference for the `qcc` CLI is at the
[end of this page](#command-reference).

## Prerequisites

Three things come from you; everything else is pinned and provisioned.

- A container runtime: Docker, or a Docker-compatible engine such as
  [Colima](https://github.com/abiosoft/colima) or Podman exposing the
  Docker socket (`CONTAINER_TOOL` defaults to `docker`).
- Go. The version is pinned in `go.mod`; any recent Go on `PATH` fetches
  the right toolchain automatically.
- [mise](https://mise.jdx.dev/), which provisions the rest of the pinned
  toolchain: `kubectl`, `kind`, `helm`, `kustomize`, `buf`,
  `golangci-lint`, `uv`, `controller-gen`.

```bash
make tools-install   # mise install
make tools-check     # verifies everything is on PATH
```

Tested with the pinned versions in this repo: Go 1.25.7 (`go.mod`),
Python 3.12, kind 0.27.0, kubectl 1.31.4, helm 3.16.4, kustomize 5.8.1
(`.mise.toml`); Kubernetes libraries at 1.35 / controller-runtime 0.23
(`go.mod`); Qiskit 2.4.1, qiskit-aer 0.17.2, qiskit-ibm-runtime 0.46.1
(`uv.lock`). Other versions may work; these are the ones the evaluation
ran on.

## 1. Bring up the platform

`platform-up` creates the kind cluster (`qcc-dev`) and installs the
observability stack into the `monitoring` namespace: kube-prometheus-stack,
Tempo, and the OpenTelemetry Collector.

```bash
make platform-up
kubectl apply -f deploy/grafana/    # the two QCC dashboards
```

The dashboards are ConfigMaps labeled `grafana_dashboard: "1"`; Grafana's
sidecar picks them up within a minute.

The observability stack is where `qcc_*` metrics land, but QCC does not
depend on it: without a Collector the controller logs export errors and
everything else works. Set `OTEL_SDK_DISABLED=true` on the controller
Deployment to silence the SDK entirely.

## 2. Deploy QCC

```bash
make dist-up
```

This builds both images, loads them into the kind cluster, installs the
CRDs, and deploys the controller and executor into
`quantum-circuit-controller-system`.

```bash
kubectl get pods -n quantum-circuit-controller-system
```

Expect two pods `Running`: the controller-manager and the executor.

## 3. Register backends

A fresh deploy has no QPUs, so backend selection would fail with
`NoEligibleBackend`. Registering backends is an explicit step:

```bash
kubectl apply -k config/samples/qpu/
kubectl get qpus
```

The samples register the ideal `aer-statevector` reference, a catalog of
`fake_*` snapshots (frozen real-IBM calibration: real basis gates,
coupling maps, and noise, no credentials needed), and three IBM hardware
profiles that stay inert until credentials exist.

The controller immediately probes each simulator through the executor and
records qubit count, basis gates, error medians, coherence times, and the
calibration snapshot date on `QPU.status`:

![kubectl get qpu listing: IBM hardware backends and simulators with processor family, qubit count, 2Q error, T1, dt, and calibration date](./assets/figures/qpu_get_all.png)

Inspect one in full:

```bash
kubectl get qpu fake-brisbane -o yaml
```

## 4. Run your first circuit

Build the CLI and submit a Bell state to the ideal simulator:

```bash
make qcc-build
./dist/qcc run examples/bell-state.qasm --backend aer-statevector
```

The CLI creates a `Circuit` resource, streams the phase transitions, and
prints a result card: backend calibration context, transpiled depth and
gate counts, an error-exposure verdict, and the measurement histogram. For
a Bell state, roughly half `00` and half `11`. A result card looks like
this (here for the Shor demonstration workload on the same backend):

![Result card for a Shor run on aer-statevector: completed in 510 ms, transpiled depth 506, outcomes 0000:517 and 1000:507 of 1024 shots](./assets/figures/circuit_run_shor_aer_v1.png)

Now run the same circuit against a noisy calibration snapshot and compare:

```bash
./dist/qcc run examples/bell-state.qasm --backend fake-brisbane
```

Other modes worth trying:

```bash
./dist/qcc draw examples/bell-state.qasm                               # ASCII circuit
./dist/qcc schedule examples/bell-state.qasm --backend fake-brisbane   # µs timeline
./dist/qcc run examples/thesis/algorithms/shor.py --performance-test   # every simulator at once
./dist/qcc get circuits
```

## 5. Look at the dashboards

```bash
kubectl port-forward -n monitoring svc/kps-grafana 3000:80
```

Open http://localhost:3000 (admin / admin). Two QCC dashboards are
installed:

- QCC · QPU substrate health: availability, error medians, coherence
  times, calibration age, across every registered backend.
- QCC · Circuit detail: one circuit's identity, transpile shape, phase
  timing, outcome distribution, and a clickable provider-job link.

Every panel is a PromQL query over the `qcc_*` metrics documented in
[observability.md](./observability.md).

## 6. Real IBM hardware (optional)

You need an IBM Quantum Platform account and its API token. Create the
secret and restart the executor so it picks the credential up:

```bash
kubectl create secret generic ibm-quantum-token \
  -n quantum-circuit-controller-system \
  --from-literal=QISKIT_IBM_TOKEN='<your-token>'

kubectl rollout restart deployment/quantum-circuit-controller-executor \
  -n quantum-circuit-controller-system
```

The sample IBM QPUs (`ibm-fez`, `ibm-kingston`, `ibm-marrakesh`) are
already registered from step 3. Hardware queues take minutes, so submit
detached:

```bash
./dist/qcc run examples/thesis/algorithms/shor.py --backend ibm-fez --detach
./dist/qcc get circuits          # check back later
./dist/qcc get circuit <name>    # results once the vendor job completes
```

![Detached submission output: Circuit name, UID, and provider job ID printed once the job is queued on ibm-kingston](./assets/figures/cli_detach_submission_kingston.png)

The controller polls the vendor queue in the background and records the
counts on the `Circuit` when the job finishes. The provider job ID lands
in `status.providerJobId`, and the executor stamps the Circuit's
Kubernetes UID onto the IBM job as a tag, so the run resolves from either
side.

Two caveats before relying on this path (details in
[operations.md](./operations.md)): the executor's async task registry is
in-memory, so an executor restart mid-queue orphans the watch; and IBM
QPUs are marked `Available` optimistically even when a probe fails.

## 7. Tear down

```bash
make dist-down      # remove QCC deployment + CRDs
make platform-down  # remove the observability stack + kind cluster
```

## Command reference

The `qcc` binary is a Kubernetes client: every command reads or writes
`Circuit`/`QPU` resources through the API server. Nothing talks to the
executor or a provider directly, so anything the CLI does, `kubectl` can
do too.

Global flags: `--kubeconfig` (defaults to `KUBECONFIG`, then
`~/.kube/config`) and `-n/--namespace` (default `default`; Circuits only,
QPUs are cluster-scoped).

### qcc run

Submit a circuit and stream progress to a result card. The file extension
picks the format: `.qasm` is OpenQASM 3, `.py` is Qiskit Python (converted
server-side).

| Flag | Default | Meaning |
|---|---|---|
| `--backend` | none | target one QPU, by K8s name (`fake-brisbane`) or provider-native name (`fake_brisbane`) |
| `--provider` | none | constrain selection to a provider (`local`, `ibm`) |
| `--shots` | `1024` | shot count |
| `--detach` | off | exit once the provider job is queued; the controller keeps polling (use for hardware) |
| `--select-only` | off | `mode=select`: run backend selection, execute nothing; a dry-run of eligibility before spending QPU time |
| `--performance-test` | off | fan the circuit out to every `Available` simulator under one shared experiment label; prints a comparison table and a Grafana deep-link |
| `--include-hardware` | off | performance-test only: include hardware QPUs (spends real credits) |
| `--algorithm`, `--version`, `--experiment` | none | stamp the `qcc.io/*` grouping labels (version and experiment require `--algorithm`) |
| `-l/--label k=v` | none | extra labels, repeatable |
| `--timeout` | `30m` | wall-clock ceiling (hardware queues take minutes) |
| `--poll` | `500ms` | status poll interval |

### qcc draw

Render the circuit as ASCII through the executor. No selection, no
execution, no QPU time; first feedback on a circuit's structure.

```
qcc draw <file> [--keep] [--timeout 60s] [--poll 250ms]
```

The ephemeral `Circuit` is deleted afterward unless `--keep`.

### qcc schedule

Transpile and schedule against a real backend `Target`, then print a
per-qubit timeline in wall-clock units: gate starts, durations, the
critical path. Needs a backend with instruction durations (`fake_*` or
hardware; generic Aer fails with `SchedulingUnsupported`).

```
qcc schedule <file> --backend fake-brisbane [--provider local] [--keep] [--timeout 120s]
```

### qcc get

kubectl-style inspection. Kinds: `circuit(s)` and `qpu(s)`, singular and
plural interchangeable.

```
qcc get circuits [--algorithm shor] [--version v2] [--experiment thesis]
qcc get circuit <name>              # result card: backend, transpile shape, verdict, histogram
qcc get circuit <name> --qasm       # raw converted OpenQASM 3 (pipe-friendly)
qcc get circuit <name> --draw       # raw ASCII drawing (pipe-friendly)
qcc get circuit <name> --schedule   # rendered timeline from the schedule artifact
qcc get qpus                        # registry: processor, qubits, 2Q error, T1, dt, calibrated
qcc get qpu <name>                  # full characterization of one backend
```

The three artifact flags are mutually exclusive; label filters apply to
Circuit lists only.

### Which command when

| You want to | Use |
|---|---|
| sanity-check a circuit's structure | `qcc draw` |
| know if any backend would accept it, without running | `qcc run --select-only` |
| see the µs-scale timeline on a target backend | `qcc schedule --backend ...` |
| run on the ideal simulator | `qcc run --backend aer-statevector` |
| compare across every simulator | `qcc run --performance-test` |
| run on IBM hardware | `qcc run --backend ibm-... --detach`, then `qcc get circuit` |
| inspect any past run or backend | `qcc get` |

The full journey with output screenshots is
[demonstration.md](./demonstration.md).
