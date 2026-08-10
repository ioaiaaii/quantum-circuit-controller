# Demonstration: Shor's Algorithm End to End

This walkthrough exercises the whole platform with one workload, Shor's
algorithm factoring N=15 with a=4. It registers backends, draws and runs
the circuit, compares it across simulators, and executes it on three live
IBM Heron r2 backends. It closes by reading the results through the
dashboards and tuning one transpiler setting, which shows the two-tier
schema absorbing a vendor feature without any platform change.

## Before you begin

A running QCC deployment, which the [tutorial](./getting-started.md) sets
up, with the `qcc` CLI on your path and `kubectl` pointed at the cluster.

The hardware sections need an IBM Quantum account and the executor
configured with a token. Everything through
[Compare the simulators](#compare-the-simulators) runs on simulators
alone, so you can follow most of this walkthrough without one.

The workload ships in the repository at
`examples/thesis/algorithms/shor.py`, alongside the manifests for the
tuned runs. See the [kit README](../examples/thesis/README.md).

Hardware runs queue for minutes to hours and consume your IBM Quantum
allocation. The five runs here use 1024 shots each for the first pass and
4096 for the two tuned runs.

## Register a backend

A QPU enters the platform through a minimal manifest, provider and kind;
everything else is discovered:

<img alt="fake-fez QPU manifest: spec declares only provider local and kind simulator, every other field is left to the probe" src="./assets/figures/qpu_manifest_fake-fez.webp" width="700">

After `kubectl apply -f fake-fez.yaml`, the controller probes the backend
through the executor and populates `status` with the qubit count, basis
gates, coupling map, calibration vintage, and per-operation error
medians:

```bash
kubectl get qpu fake-fez -o yaml
```

<img alt="Probe-populated fake-fez status: qubit count, Heron processor identity, basis gates, coupling edges, calibration timestamp, error and coherence medians" src="./assets/figures/qpu_get_fake-fez.webp" width="480">

The registry at a glance, live IBM backends alongside simulators, with
processor family, 2Q error, T1, and calibration date in one listing:

```bash
kubectl get qpu
```

<img alt="kubectl get qpu listing: three IBM hardware backends and the simulator catalog, with processor family, qubits, 2Q error, T1, dt, and calibration date per row" src="./assets/figures/qpu_get_all.webp" width="900">

## Draw the circuit

`mode=draw` renders the circuit without consuming QPU time, early
feedback on structure before anything runs:

```bash
qcc draw examples/thesis/algorithms/shor.py
```

<img alt="ASCII rendering of the Shor N=15 circuit: four-qubit control register in superposition, controlled modular-multiplication blocks on the target register, inverse QFT, measurement" src="./assets/figures/circuit_draw_shor.webp" width="820">

## Run on the ideal simulator

The noise-free baseline targets `aer-statevector`, with grouping labels
for later comparison:

```bash
qcc run examples/thesis/algorithms/shor.py \
    --algorithm shor --experiment thesis \
    --version v1 --backend aer-statevector
```

<img alt="Result card for shor-lpdb7 on aer-statevector: completed in 510 ms, transpiled depth 506, outcomes 0000:517 and 1000:507 of 1024 shots" src="./assets/figures/circuit_run_shor_aer_v1.webp" width="640">

The run completes in 510 ms after transpiling to depth 506. All 1024
shots land on the two expected period bitstrings `0000` and `1000`
(517 and 507). The difference is sampling variation.

The success metric used throughout is correct-period mass:

```
correct-period mass = (count[0000] + count[1000]) / shots
```

Ideal simulator: (517+507)/1024 = 100%. A fully decohered distribution
puts 2 of 16 outcomes in the period set, so 12.5% is the noise floor.

Every run is an ordinary Kubernetes resource.
`kubectl get circuit shor-lpdb7 -o yaml` shows the declared `spec`
(source, labels, backend) and the controller-assembled `status`
(conditions, counts, transpile shape).

## Compare the simulators

Before spending hardware time, fan the same circuit out across every
registered simulator:

```bash
qcc run examples/thesis/algorithms/shor.py \
    --performance-test --algorithm shor \
    --version v1 --experiment thesis-perf-test
```

One `Circuit` per available simulator, all sharing the same source body
and `qcc.io/experiment` label. A single labelled experiment, not
unrelated runs:

<img alt="Performance-test fan-out table: one row per simulator with phase, transpiled depth, gate counts, and top outcomes, fake-belem shows a terminal TranspilationFailed, the last line is a Grafana deep-link with the experiment filter" src="./assets/figures/circuit_run_shor_perf-test.webp" width="900">

Capability mismatches are first-class
results: the five-qubit `fake-belem` returns a terminal
`TranspilationFailed`, recorded rather than silently omitted. The
processor families also separate cleanly, with the Heron r2 snapshot
`fake-fez` keeping `0000` and `1000` dominant while the Eagle r3 snapshot
`fake-kyoto` spreads the mass until non-period bitstrings displace
them.

The survey directs the hardware budget at the Heron r2 family.

## Run on real hardware

Hardware jobs queue for minutes to hours, so submit detached. The CLI
exits once the provider job is queued and the controller polls in the
background:

```bash
qcc run examples/thesis/algorithms/shor.py \
    --algorithm shor --experiment thesis --version v1 \
    --backend ibm-fez --detach          # likewise ibm-kingston, ibm-marrakesh
```

<img alt="Detached submission output for ibm-kingston: Circuit shor-2vv42 and provider job d89dinp789is73935r0g printed once the job is queued" src="./assets/figures/cli_detach_submission_kingston.webp" width="800">

Each run uses 1024 shots and Qiskit's default transpilation (level 1, no
`optimizationLevel` set). Results, read back with
`qcc get circuit <name>`:

<img alt="ibm-kingston terminal result card: depth 2048, outcomes 0000:41 and 1000:41 of 1024 shots, 8.0 percent correct-period mass" src="./assets/figures/circuit_get_shor_kingstone_v1.webp" width="560">

| Backend | Depth | 0000+1000 | Correct-period mass |
|---|---|---|---|
| `ibm-fez` | 2062 | 56+55 | 10.8% |
| `ibm-marrakesh` | 2048 | 42+44 | 8.4% |
| `ibm-kingston` | 2048 | 41+41 | 8.0% |

All three sit at or below the 12.5% noise floor, so under default
transpilation the circuit is degraded on every backend and "best" means
least-degraded rather than successful. The signal is recovered under
[Tune the transpiler](#tune-the-transpiler) below.

T1 alone does not predict the ranking. Ordered by T1 the backends run
`ibm-marrakesh` at about 196 µs, `ibm-kingston` at about 175 µs, and
`ibm-fez` at about 102 µs, yet by outcome `ibm-fez` is the least
degraded. Backend quality does not reduce to a single calibration number,
which is why it is carried as telemetry rather than a specification.

The whole experiment reads from a single listing. Every Circuit carries
its algorithm label, selected QPU, provider job ID, and terminal phase:

```bash
qcc get circuits
```

<img alt="qcc get circuits listing after the sweep: every Circuit with its algorithm label, experiment, selected QPU, provider job ID, and terminal phase" src="./assets/figures/circuit_all_after_run_v1.webp" width="900">

## Read the dashboards

The `qcc_*` metrics render on the two Grafana dashboards described in the
[metrics reference](./observability.md).

QPU dashboard, substrate health: availability and ready-over-registered,
plus calibration age as the freshness signal:

<img alt="QPU dashboard saturation and utilisation rows: per-QPU availability state, ready-over-registered snapshot, availability over time, calibration age" src="./assets/figures/grafana_qpu_availability.webp" width="900">

The errors section shows per-operation gate and readout medians, T1 and
T2, and a family-comparison row: registered Heron r2 QPUs at 0.32% median
2Q error versus 20.60% for the Eagle-family snapshots. The same
generation-level contrast the simulator sweep exposed, now visible at
fleet level:

<img alt="QPU dashboard errors section: per-operation gate and readout medians, T1 and T2 coherence, and the family-comparison row showing Heron r2 at 0.32 percent versus Eagle at 20.60 percent median 2Q error" src="./assets/figures/grafana_qpu_coherence.webp" width="900">

Circuit dashboard, one run in detail: identity, transpile shape, phase
timing, outcome distribution. The `$experiment` template variable filters
every panel to the runs sharing an experiment label, so cross-substrate
comparison is a selector change rather than a PromQL exercise:

<img alt="Circuit dashboard for one experiment run: identity strip, transpile-shape stats, per-phase durations, and the outcome histogram" src="./assets/figures/grafana_circuit_dashboard.webp" width="700">

## Cross the boundary in both directions

The identity panel's `provider_job_id` cell carries an "Open on IBM
Quantum" data link, resolved from the `qcc_circuit_info` series:

<img alt="Circuit dashboard identity panel for shor-2vv42: the provider_job_id cell carries an Open on IBM Quantum data link" src="./assets/figures/grafana_circuit_provider_job_link.webp" width="900">

In the opposite direction, the executor stamps the Circuit's Kubernetes
UID onto the IBM job as a `qcc.circuit.uid` tag, so the vendor console
points back at the owning Circuit:

<img alt="IBM Quantum console job details for the ibm-kingston run: the Tags field shows qcc.circuit.uid with the Circuit's Kubernetes UID" src="./assets/figures/ibm_console_job_tag_kingston.webp" width="800">

Together the two identifiers make a run traceable from either side, live
and after termination, with standard tooling on both ends.

## Tune the transpiler

The v1 hardware runs left the signal under the noise floor. Two further
runs on `ibm-kingston` (4096 shots, so the comparison isolates
transpilation) change one thing each.

v2, a Tier-1 change: raise `optimizationLevel` to 3.

```yaml
spec:
  mode: run
  shots: 4096
  optimizationLevel: 3
```

v3, a Tier-2 change: add a passthrough `transpile` block with Qiskit's
as-late-as-possible scheduling pass. The key lands verbatim as a
`transpile()` kwarg, with no CRD change and no controller rebuild:

```yaml
spec:
  mode: run
  shots: 4096
  optimizationLevel: 3
  transpile:
    scheduling_method: alap
```

Both are submitted with `kubectl create -f` (the Tier-2 block is not
typed in the CRD and has no CLI flag. See
`examples/thesis/circuits/shor-v2.yaml` and `shor-v3.yaml`).

<img alt="v2 result card on ibm-kingston: optimizationLevel 3, depth 1523, outcomes 0000:540 and 1000:530 of 4096 shots, 26.1 percent correct-period mass" src="./assets/figures/circuit_get_shor_tuned_v2.webp" width="620">

<img alt="v3 result card on ibm-kingston: level 3 plus alap scheduling, gate count 2548, outcomes 0000:554 and 1000:546 of 4096 shots, 26.9 percent correct-period mass" src="./assets/figures/circuit_get_shor_tuned_v3.webp" width="620">

The full arc:

| Run | Transpilation | Shots | 0000+1000 | Success |
|---|---|---|---|---|
| Aer (ideal) | default | 1024 | 517+507 | about 100% |
| `ibm-fez` (v1) | default, L1 | 1024 | 56+55 | 10.8% |
| `ibm-marrakesh` (v1) | default, L1 | 1024 | 42+44 | 8.4% |
| `ibm-kingston` (v1) | default, L1 | 1024 | 41+41 | 8.0% |
| `ibm-kingston` (v2) | level 3 (Tier-1) | 4096 | 540+530 | **26.1%** |
| `ibm-kingston` (v3) | level 3 + `alap` (Tier-2) | 4096 | 554+546 | 26.9% |

The reading is operational, not an optimization claim. One stable Tier-1
field cuts depth from 2048 to 1523 and recovers the signal above the
noise floor, from 8.0% to 26.1%. The Tier-2 `alap` pass then inflates the
gate count from 2054 to 2548 (inserted delays) and drops the modeled
fidelity bound from 0.26 to 0.23, while the measured outcome stays within
sampling noise at 26.9%. Both the empirical outcome and the modeled
exposure are visible for every run on the same telemetry surface. The
model-versus-measurement gap is itself legible.

## Cleaning up

Circuits and their artifact ConfigMaps are removed together, because the
ConfigMap is owned by the Circuit:

```bash
kubectl delete circuits -l qcc.io/experiment=thesis
kubectl delete circuits -l qcc.io/experiment=thesis-perf-test
```

Deleting a Circuit does not cancel a queued provider job. Cancel any
still-running hardware job from the IBM Quantum console first, or let it
finish, so the allocation is not spent on results nobody reads.

QPUs are cluster-scoped and cheap to keep. Remove them with
`kubectl delete qpu <name>` when you are done.

## What's next

- [Metrics reference](./observability.md) for the `qcc_*` series behind
  the dashboards, and how to query them.
- [API reference](./api.md#the-two-tier-schema) for the two-tier schema
  the transpiler tuning relies on.
- [Architecture](./architecture.md) for the design behind the lifecycle,
  backend selection, and the cross-boundary identifier.
- [Adapter guide](./engineering.md#adding-a-provider-adapter) to run this
  same walkthrough against a provider QCC does not support yet.
