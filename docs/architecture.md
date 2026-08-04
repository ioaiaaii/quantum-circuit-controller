# Architecture

QCC applies SRE discipline to hybrid quantum-classical execution: circuits
and the QPUs that execute them are modeled as observable, schedulable
Kubernetes resources, instrumented with the operational standards already
established in classical systems.

The design turns on one separation. The component that orchestrates work
(the controller) and the component that talks to quantum providers (the
executor) are kept apart, joined only by a typed gRPC contract. The user
states intent; the platform carries it across the quantum-classical
boundary and reports back through Kubernetes-native state and standard
telemetry.

Contents: [Requirements](#requirements) ·
[System overview](#system-overview) ·
[Components](#components) ·
[Execution lifecycle](#execution-lifecycle) ·
[SRE principles](#sre-principles-mapped) ·
[Requirements coverage](#how-the-requirements-are-met) ·
[Position and trajectory](#position-and-trajectory)

## Requirements

Five requirements drive the design. The first four hold a quantum workload
to expectations already standard for production systems; the fifth captures
what the quantum substrate adds.

| # | Name | Requirement |
|---|---|---|
| R1 | Declarative infrastructure | Stand up declaratively on commodity hardware (laptop, cloud VM, managed cluster) so one developer can operate backends and run circuits. HPC stays a valid target, not a prerequisite. |
| R2 | Observable through industry standards | Every circuit and backend surfaces operational state over standard telemetry that aggregates in any existing observability stack. Platform behavior is instrumented; algorithm internals stay with the workflow. |
| R3 | Portable and modular across providers | Each provider sits behind an adapter against a stable API contract, out-of-tree the way Kubernetes moved storage drivers to CSI. The controller imports no vendor code; adding a provider touches neither controller, schema, nor telemetry. |
| R4 | Correlated across the boundary in real time | Follow a circuit across the boundary and see its internals live: transpiled shape, queue position, on-QPU versus orchestration time, via one identifier that resolves in both directions from the telemetry. |
| R5 | Monitored backend quality | Backend quality is a measured signal. Coherence times, gate and readout error rates, calibration vintage, and each run's outcome distribution are first-class telemetry, so backend choice and result-trust rest on execution evidence. |

## System overview

![QCC system context](./assets/figures/qcc-system-context.png)

A request enters as a `Circuit` resource on the Kubernetes API server, the
system's only entry point and its durable boundary. The controller watches
it, selects one of the backends registered as `QPU` resources, and drives
the circuit through its lifecycle without loading a quantum SDK.
Quantum-facing work is delegated to the executor over gRPC; inside the
executor an adapter dispatches by provider, either to an in-cluster
simulator (Aer or a calibration-faithful `fake_*` snapshot) or to IBM
Quantum. Results return along the same path. All telemetry originates in
the controller.

| Component | Kind | Role |
|---|---|---|
| `qcc-controller` | Go binary, Deployment | Reconciles `Circuit` and `QPU`; drives the phase machine; patches status; emits telemetry |
| `qcc-executor` | Python gRPC service, Deployment | Source conversion, drawing, transpilation, submission, result retrieval behind the `Adapter` contract |
| `qcc` CLI | Go binary, user-side | Builds `Circuit` resources from local files; submits via the Kubernetes API |
| `Circuit` | CR, namespaced | Declarative submission unit: body, intent, constraints, observed status |
| `QPU` | CR, cluster-scoped | Registered backend: provider, kind, capabilities, calibration, availability |
| Executor gRPC API | `qcc.executor.v1` proto | Eight RPCs in three families (sync, async, utility) |
| `Adapter` | Python module | One class per provider, registered by `QPU.spec.provider` |

## Components

### qcc-controller

A Kubernetes operator with two reconcilers. The CircuitReconciler drives
each `Circuit` through its phases, patching status and conditions on every
transition; the circuit's history stays on the resource, and plain
`kubectl describe` renders it. It also stamps the controller-owned labels
(`qcc.io/run-index`, `qcc.io/source-sha256`) at admission. The
QPUReconciler probes each registered backend through the executor's
`ProbeBackend` RPC and fills in qubit count, basis gates, coupling map,
calibration vintage, and error medians; selection draws only from backends
it has marked `Available`.

Input: API watches. Output: status patches, OTLP metrics, gRPC calls.
Large artifacts (drawings, converted OpenQASM) go to `ConfigMap`s the
`Circuit` owns.

### qcc-executor

The only component that links a quantum SDK. Its eight RPCs group into
three scopes:

- Discovery. `ProbeBackend` returns backend characteristics (qubits,
  processor, gate and readout errors, T1 and T2) at registration.
- Source handling. `ConvertSource` translates Qiskit Python to OpenQASM 3,
  `DrawCircuit` returns an ASCII rendering, `ScheduleCircuit` returns a
  per-instruction timeline.
- Execution. A simulator job runs in-process through one `RunCircuit`
  call; a hardware job threads through `SubmitTask`, `WatchTask`, and
  `FetchTaskResult`, the same lifecycle as a Qiskit `JobV1`.

Every call stands alone: the request carries the source, the chosen
backend, and the correlation identifier; the response carries a result or
a job handle. The full RPC table is in
[api.md](./api.md#the-executor-grpc-contract).

### qcc CLI

Reads circuit source from local files, builds `Circuit` resources, submits
them through the Kubernetes API. It links no quantum SDK (conversion runs
server-side), so it ships as a single binary whose only trust boundary is
the Kubernetes API; everything it does is equally reachable with
`kubectl`. `run` blocks until completion, which suits simulators;
`--detach` submits, waits until a provider job is queued, and exits, with
the controller polling in the background. `--performance-test` fans the
same circuit out to every available simulator under one shared experiment
label. Full command reference:
[getting-started.md](./getting-started.md#command-reference).

### Adapter contract

Inside the executor, every vendor operation goes through an `Adapter` base
class with six methods: `transpile`, `submit`, `poll`, `fetch_result`,
`inspect`, `schedule`. Each returns a structured type that already carries
what the controller persists on status. A registry maps
`QPU.spec.provider` strings to adapters; adding a vendor is one subclass
plus one registry entry ([how-to](./engineering.md#adding-a-provider-adapter)).

| Provider | Adapter | Reach |
|---|---|---|
| `local` | `AerAdapter` | Qiskit Aer in-process plus `fake_*` snapshots via `FakeProviderForBackendV2`. No credentials. |
| `ibm` | `IBMAdapter` | IBM Quantum via `qiskit-ibm-runtime` (`QiskitRuntimeService` + `SamplerV2`). Credentials via the `QISKIT_IBM_TOKEN` Secret. |

## Execution lifecycle

Each `Circuit` follows one of four mode-conditioned paths from `Pending`
to `Succeeded` or `Failed`. The modes share their early phases and diverge
by how far they go: `run` executes end to end; `select` stops after
transpilation and uses no QPU time; `draw` renders ASCII
(`status.drawingRef`); `schedule` produces a per-instruction timeline
(`status.scheduleRef`).

![Circuit lifecycle](./assets/figures/qcc-lifecycle.png)

### Submission and the cross-boundary identifier

`Submitting` is the lifecycle's one external side effect. Before issuing
the call, the controller patches `phase=Submitting` to the cluster store,
so the intent to submit is durable across a controller restart. Each
submission carries an idempotency key formed from the `Circuit`'s UID and
observed generation; the executor reads the UID back from it and stamps it
onto the IBM job as a `qcc.circuit.uid` tag. The returned provider job ID
is recorded on `status.providerJobId` in the same patch that sets
`phase=Running`. The cross-boundary identifier lives on the resource
itself and survives restarts.

![Submission sequence and the cross-boundary identifier](./assets/figures/qcc-idempotency.png)

### Backend selection

The shipped selector applies hard constraints only. A `QPU` is eligible
when `status.availability` is `Available`, its `capabilities.maxShots`
covers the requested shots, and it matches the `backendSelector`
(provider, backend name, kind, minimum qubits). The controller takes the
first eligible candidate; if none qualifies the `Circuit` fails with
`NoEligibleBackend`. The selector ranks nothing; it filters and picks.
Scoring strategies would extend the same surface and consume the metrics
the lifecycle already exposes.

### The evaluation surface

Calibration enters after a run completes. `qcc get circuit` turns the
recorded numbers into a verdict, here for a completed Shor run on
`ibm-kingston`:

```text
shor-2vv42 · run · on ibm-kingston · degraded signal (error exposure ≈ 1.5)

  backend
    name         ibm-kingston  (156q · 352 edges · 10 basis gates)
    gate errors  1Q 2.74e-04 · 2Q 2.03e-03 · readout 1.65e-02
    coherence    T1 175 µs · T2 117 µs
    cycle time   dt = 4 ns

  circuit
    transpiled      depth 2048 · 2441 gates · 474 2Q
    exec time       ~139.26 µs (critical-path estimate)
    error exposure  ≈ 1.5 events/shot
    fidelity bound  P(no gate error) ≈ 0.22  (excludes readout & coherence)
```

The headline is an error-exposure indicator: the expected number of
gate-error events per shot, a first-order sum of transpiled gate counts
weighted by the backend's error medians.

```
exposure ≈ n_1Q·ε_1Q + n_2Q·ε_2Q
         = 1967 × 2.74e-4 + 474 × 2.03e-3 ≈ 0.54 + 0.96 ≈ 1.5
```

Four bands: signal preserved below 0.1, noisy signal from 0.1, degraded
signal from 1, signal expected lost at 5 and above. The paired bound
`P(no gate error) ≈ exp(-exposure) ≈ 0.22` says fewer than a quarter of
shots are expected free of any gate error. This is the SRE error-budget
framing carried to a quantum backend, and it is a regime signal, not a
fidelity model: only gate errors enter the sum, and the same card makes
the omission visible. The 139 µs critical-path estimate is already
comparable to the backend's T2 of 117 µs, so coherence decay is a real
error source the gate-only number does not capture.

## SRE principles, mapped

The thesis claim is SRE discipline applied to quantum execution. Where
each principle lands:

| Principle | Where it lives in QCC |
|---|---|
| Declarative desired state, reconciliation | `Circuit`/`QPU` CRDs; the phase machine converges observed toward declared |
| Error budgets | the error-exposure indicator and its regime bands, a budget verdict per run from measured backend data |
| Idempotency and safe retries | phase persisted before the external side effect; the UID+generation idempotency key; the terminal-versus-transient error rule |
| Observability as a first-class surface | the 14-metric specification; the USE-Q substrate dashboard and RED-style circuit dashboard |
| Toil reduction | backend probing, label stamping, artifact GC, and the perf-test fan-out are automation, not runbook steps |
| Least privilege, small blast radius | scoped RBAC, restricted-PSS pods, vendor code confined to the executor |
| Graceful degradation | probe failures do not block availability; the platform runs without the observability stack |
| Capacity discipline | metric cardinality budgeted per label (allowlist promotion, 2^q flagged) |

## How the requirements are met

| Requirement | Answered by |
|---|---|
| R1 Declarative infrastructure | the resource model plus the kind-deployable stack ([getting-started.md](./getting-started.md)) |
| R2 Standards-based observability | the 14-metric `qcc_*` specification over OTLP into Prometheus ([observability.md](./observability.md)) |
| R3 Provider modularity | the adapter contract behind the executor gRPC seam |
| R4 Cross-boundary correlation | `providerJobId` on status and `qcc_circuit_info`, plus the `qcc.circuit.uid` job tag, resolvable from both sides |
| R5 Monitored backend quality | QPU probe telemetry, per-run outcome metrics, and the evaluation surface, demonstrated end to end in [demonstration.md](./demonstration.md) |

## Position and trajectory

QCC's architectural claim is legibility: a hybrid run is inspectable from
declared intent to provider-side execution and back to recorded outcome.
The research behind this section is the MSc thesis
[*Interface between Quantum and Classical Computers*](https://ioaiaaii.github.io/project/msc-thesis/)
(Democritus University of Thrace, 2026), of which QCC is the
proof-of-concept artifact.

### The QCSC reference architecture

IBM's Quantum-Centric Supercomputing reference architecture (Seelam et
al., 2026, [arXiv:2603.10970](https://arxiv.org/abs/2603.10970)) organizes
the stack into four layers (Hardware Infrastructure, System Orchestration,
Application Middleware, Applications) with System Management and
Monitoring as a cross-cutting concern. It explicitly calls for
Kubernetes-based orchestration and Prometheus/Grafana-based monitoring of
quantum resources, without shipping a vendor-neutral implementation of
either.

QCC sits primarily in System Orchestration (the Kubernetes-native
controller), absorbs a deliberate slice of Application Middleware (each
circuit is transpiled against its selected backend, producing the shape
that quality evaluation needs), and puts its main weight on the management
and monitoring cross-cut: the per-instance lifecycle on `Circuit.status`
is the audit surface, and the `qcc_*` metrics specification aggregates
circuit and backend state across runs.

### Comparators

| System | What it is | What it optimizes | How QCC differs |
|---|---|---|---|
| [Qubernetes](https://arxiv.org/abs/2408.01436) (Stirbu et al., 2024) | quantum jobs on Kubernetes batch primitives | packaging quantum workloads into existing K8s Jobs | QCC models circuits and backends as purpose-built resources with a lifecycle, selection, and a metrics contract |
| [Qonductor](https://arxiv.org/abs/2408.04312) (Giortamis et al., SC '25) | cloud orchestrator for hybrid workloads | multi-objective scheduling over candidate backends | a different problem: Qonductor ranks placements; QCC makes runs legible, with every decision and outcome observable |
| [QRMI](https://arxiv.org/abs/2506.10052) (Bacher et al., 2025) | resource-management interface; device-plugin sketch | exposing QPUs as allocatable resources beneath the scheduler | complementary layer: QRMI allocates hardware, QCC orchestrates and observes execution above it; a QRMI adapter fits QCC's executor contract |
| IBM Quantum, AWS Braket, Azure Quantum | managed end-to-end services | vendor-integrated convenience | consumed through adapters, not peers of the control plane; QCC adds the vendor-neutral resource model and telemetry on top |
| Kanazawa et al., 2025 ([arXiv:2512.05484](https://arxiv.org/abs/2512.05484)) | observability architecture for QCSC workflows | structured telemetry inside an HPC/vendor context | the same architectural need implemented with open standards portable across providers |

The one-line answer to "why not X": Qubernetes has no resource model,
Qonductor solves scheduling rather than legibility, QRMI sits below the
scheduler, and the managed clouds are single-vendor.

### Toward an interface

Two QCC surfaces are deliberately shaped as candidate interfaces rather
than internal conveniences.

The executor contract: the async RPC trio (`SubmitTask`, `WatchTask`,
`FetchTaskResult`) follows the same lifecycle as Qiskit's `JobV1` and as
the QRMI task interface. That alignment is intentional. An adapter for a
QRMI-managed resource, a Qiskit-provider backend (Braket, IonQ, IQM
through their Qiskit plugins), or a bare OpenQASM runtime all implement
the same six-method contract without touching the controller, the schema,
or the telemetry.

The metrics specification: the OpenTelemetry semantic-conventions registry
has no quantum namespace today. The 14-metric `qcc_*` specification
([observability.md](./observability.md#metric-specification)) is a worked
proposal for what a `quantum.*` namespace would need to standardize:
backend quality as gauges, lifecycle as counters and histograms, one
info-metric join key, a cross-boundary identifier label. The names are
QCC's; the shape is the contribution.

Both are candidacies, not standards. The point of open-sourcing the
implementation is that an interface argument needs a reference
implementation others can run, extend, and disagree with.

### Scalability model

What the current architecture scales, and the honest limits (operational
detail in [operations.md](./operations.md#scaling-and-sizing)):

- Scales today: registered QPUs (cluster-scoped registry, one-shot
  probes), namespaced circuit submission, controller availability
  (leader-elected standbys), and the read path (metrics observe from
  informer caches, never the API server).
- Bounded today: one executor replica (the async task registry is
  process-local); the synchronous simulator path parks a reconcile worker
  for the duration of a run; selection is O(QPUs) per circuit, fine at
  registry scale.
- The path up: a durable task registry unlocks horizontal executor
  scaling; hardware allocation belongs below QCC in a
  QRMI/device-plugin/DRA layer as that matures; calibration-aware scoring
  extends the existing selection seam and consumes telemetry the platform
  already emits.

Field-by-field resource reference: [api.md](./api.md). Code-level detail,
toolchain choices, and the decision ledger:
[engineering.md](./engineering.md).
