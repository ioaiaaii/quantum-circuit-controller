# API

QCC's API surface is two custom resources at `qcc.io/v1alpha1`: `Circuit`
(namespaced; one execution, draw, schedule, or selection request) and
`QPU` (cluster-scoped; one registered backend). The schema captures only
what the controller and executor act on. Algorithm semantics stay in user
code, and vendor-specific construction stays behind the executor's
adapters.

Where a field exists on the schema but is not enforced by the runtime, the
tables below say so. Treat those rows as reserved contract surface.

| Surface | Writer | Meaning |
|---|---|---|
| `Circuit.spec` | user / CLI | desired circuit operation |
| `Circuit.status` + artifact `ConfigMap`s | controller | what happened |
| `QPU.spec` | user / manifest | declared backend identity |
| `QPU.status` | controller | probed backend facts and availability |

The executor influences status through the controller; it never writes
Kubernetes objects itself.

## The two-tier schema

Vendor SDKs change faster than a CRD schema can. Modeling every Qiskit
parameter would force a schema version per new transpiler flag; modeling
too few would wall off the SDK's surface. QCC resolves this with two
tiers:

- Tier 1: typed camelCase fields QCC owns and validates. On `Circuit`:
  `mode`, `source`, `shots`, `optimizationLevel`, `backendSelector`,
  `timeoutSeconds`. On `QPU`: `provider`, `backendName`, `kind`,
  `qubits`, `access`, `capabilities`, `region`. Under a dozen per
  resource, slow-moving, exposed as CLI flags.
- Tier 2: two open passthrough blocks on `Circuit`. The keys of
  `spec.transpile` become kwargs to Qiskit's `transpile()`; the keys of
  `spec.execute` become kwargs to the backend's run call
  (`AerSimulator.run()` or `SamplerV2.run()`). Keys are snake_case,
  forwarded verbatim, uninterpreted. `shots` is Tier-1 and is stripped
  from `execute` so it cannot be set in two places.

When a vendor SDK gains a parameter, users can set it as soon as the new
SDK ships in the executor image. No CRD bump, no controller rebuild. The
cost: no API-server validation of Tier-2 keys; a bad key surfaces as a
terminal failure carrying Qiskit's own error message.

## Circuit

### Spec

| Field | Meaning | Status |
|---|---|---|
| `spec.mode` | `run`, `select`, `draw`, `schedule` | enforced |
| `spec.source.format` | `openqasm3` or `qiskit` | enforced |
| `spec.source.body` | inline source text | enforced |
| `spec.shots` | executions, required for `run` | enforced |
| `spec.optimizationLevel` | Qiskit preset 0-3 | enforced |
| `spec.backendSelector.provider` | provider filter | enforced |
| `spec.backendSelector.backendName` | exact backend (K8s name or provider-native) | enforced |
| `spec.backendSelector.kind` | `hardware` or `simulator` | enforced |
| `spec.backendSelector.minQubits` | minimum qubit count | enforced |
| `spec.backendSelector.allowedQPURefs` | QPU allow-list | schema only |
| `spec.backendSelector.region` | locality hint | schema only |
| `spec.timeoutSeconds` | execution bound | wire model only, no controller timeout policy |
| `spec.transpile` | Tier-2 passthrough to `transpile()` kwargs | enforced |
| `spec.execute` | Tier-2 passthrough to the run-call kwargs | enforced |

A minimal `run`:

```yaml
apiVersion: qcc.io/v1alpha1
kind: Circuit
metadata:
  name: bell-state
  labels:
    qcc.io/algorithm: bell-state
    qcc.io/algorithm-version: v1
spec:
  mode: run
  shots: 1024
  source:
    format: openqasm3
    body: |
      OPENQASM 3.0;
      include "stdgates.inc";
      qubit[2] q;
      bit[2] c;
      h q[0];
      cx q[0], q[1];
      c[0] = measure q[0];
      c[1] = measure q[1];
  backendSelector:
    kind: simulator
```

A Tier-2 example, Qiskit's ALAP scheduling pass reaching `transpile()`
untyped (from the Shor evaluation,
`examples/thesis/circuits/shor-v3.yaml`):

```yaml
spec:
  mode: run
  shots: 4096
  optimizationLevel: 3
  transpile:
    scheduling_method: alap
```

### Modes and their outputs

| Mode | Behavior | Outputs |
|---|---|---|
| `run` | select backend and execute | `status.results`, `providerJobId`, `transpile`, optional `convertedRef` |
| `select` | selection only, no QPU time | `selectedQPU`, `selectionSummary` |
| `draw` | ASCII rendering | `drawingRef` |
| `schedule` | per-instruction timeline (dt cycles) | `scheduleRef` |

### Status

| Field | Meaning |
|---|---|
| `status.phase` | `Pending`, `Selecting`, `Transpiling`, `Submitting`, `Running`, `Rendering`, `Scheduling`, `Succeeded`, `Failed` |
| `status.conditions` | eight types: `Accepted`, `Validated`, `Selected`, `Rendered`, `Scheduled`, `Submitted`, `Completed`, `Failed`; each with reason, message, timestamp |
| `status.selectedQPU` | chosen `QPU` name |
| `status.providerJobId` | the cross-boundary identifier (`aer-<uuid>` or the vendor's job ID) |
| `status.results` | measurement counts, inline (`"0000": 517`) |
| `status.usageSeconds` | substrate-reported billable on-QPU seconds (hardware only) |
| `status.transpile` | `{depth, twoQubitGates, totalGates}` post-transpile shape |
| `status.drawingRef` / `scheduleRef` / `convertedRef` | artifact ConfigMap pointers |
| `status.traceId` | reserved, unpopulated |

Failure reasons on the `Failed` condition: `InvalidCircuit`,
`NoEligibleBackend`, `TranspilationFailed`, `ProviderSubmissionFailed`,
`ProviderJobTimedOut`, `SourceConversionFailed`, `RenderingFailed`,
`SchedulingFailed`, `SchedulingUnsupported`.

### Artifacts

Bulky generated payloads live in ConfigMaps the `Circuit` owns (same
namespace, garbage-collected with it), named `<circuit-name>-<suffix>`:

| Ref | ConfigMap key | Read by |
|---|---|---|
| `drawingRef` | `data["drawing"]` | `qcc get circuit <name> --draw` |
| `convertedRef` | `data["qasm"]` | `qcc get circuit <name> --qasm` |
| `scheduleRef` | `data["schedule.json"]` | `qcc get circuit <name> --schedule` |

### Reserved labels

Five labels carry algorithm-grouping metadata that the controller
promotes into metric label-space
([observability.md](./observability.md#algorithm-aware-queries)):

| Label | Owner | Stamped at | Notes |
|---|---|---|---|
| `qcc.io/algorithm` | user | submission | algorithm family (`shor`); without it a Circuit is a one-off |
| `qcc.io/algorithm-version` | user | submission | iteration (`v2`); requires `algorithm` |
| `qcc.io/experiment` | user | submission | campaign grouping across algorithms |
| `qcc.io/run-index` | controller | first reconcile | ordinal within the algorithm cohort, max(existing)+1 |
| `qcc.io/source-sha256` | controller | first reconcile | first 16 hex chars of the SHA-256 of `source.body`; always stamped |

`source-sha256` answers whether the source actually changed between
versions: a label-only edit produces the same hash.

## QPU

The schema separates what the operator declares (which backend, how to
reach it) from what the controller discovers by probing it through the
executor.

### Spec

| Field | Meaning | Status |
|---|---|---|
| `spec.provider` | adapter selector: `local` (Aer + `fake_*`) or `ibm` | enforced |
| `spec.backendName` | provider-native name (`fake_brisbane`); derived from `metadata.name` (dashes become underscores) when omitted | enforced |
| `spec.kind` | `simulator` or `hardware`; selects the sync or async execution path | enforced |
| `spec.qubits` | user hint; the probe overwrites it | fallback only |
| `spec.capabilities.maxShots` | shot ceiling, enforced at selection | enforced |
| `spec.access.credentialSecretRef` | per-QPU credential reference | schema only; runtime uses executor env vars |
| `spec.region` | locality hint | schema only |

A simulator entry and a hardware entry, side by side:

```yaml
# Simulator: provider and kind; the probe fills in the rest.
apiVersion: qcc.io/v1alpha1
kind: QPU
metadata:
  name: fake-brisbane
  labels:
    qcc.io/provider: local
    qcc.io/family: eagle-r3
spec:
  provider: local
  kind: simulator
---
# Hardware: adds the provider-native name and declared caps.
apiVersion: qcc.io/v1alpha1
kind: QPU
metadata:
  name: ibm-kingston
  labels:
    qcc.io/provider: ibm
    qcc.io/family: heron-r2
spec:
  provider: ibm
  backendName: ibm_kingston
  kind: hardware
  capabilities:
    maxShots: 100000
```

### Status

The operator writes none of it. After the probe, the short spec expands
into a full backend description. Real output for `ibm-kingston`:

```yaml
status:
  availability: Available
  qubits: 156
  processor: {family: Heron, revision: "2"}
  couplingEdges: 352
  basisGates: [cz, delay, id, if_else, measure, measure_2, reset, rz, sx, x]
  lastCalibrationTime: "2026-05-16T10:35:26Z"
  coherenceMedians: {t1Micros: 174.6, t2Micros: 117.0}
  errorMedians: {singleQubit: 0.000274, twoQubit: 0.00203, readout: 0.0165}
  instructionDurationMedians: {singleQubitSeconds: 3.2e-08, twoQubitSeconds: 6.8e-08}
  dtSeconds: 4e-09
  conditions:
  - type: Ready
    status: "True"
    reason: ProviderProbeOK
```

Three groups. Capability metadata (`qubits`, `basisGates`,
`couplingEdges`, `processor`) describes what the backend is. Calibration
metadata (`lastCalibrationTime`, `errorMedians`, `coherenceMedians`,
`dtSeconds`, `instructionDurationMedians`) describes its current
characteristics, in the units the schema publishes: microseconds for
coherence, seconds for durations. Operational signals (`availability`,
plus schema-only `queueDepth` and `lastError`) describe live state;
`availability` (`Available`, `Unavailable`, `Unknown`) is what selection
reads.

Conditions: `Ready` (reason `ProviderProbeOK`) is always present.
`MetadataFresh` appears only where freshness is meaningful. Local
providers carry it permanently `True`, since a frozen snapshot cannot go
stale; IBM hardware does not carry it, since calibration drifts over
hours and freshness reads from `lastCalibrationTime` instead.

Availability today: `local` becomes `Available`; `ibm` becomes
`Available` optimistically (a failed probe does not remove it); unknown
providers stay `Unknown`, which selection rejects.

## The executor gRPC contract

The controller-executor seam is `qcc.executor.v1`
([`proto/qcc/executor/v1/executor.proto`](../proto/qcc/executor/v1/executor.proto),
the single source of truth; the tables here summarize it). Eight RPCs in
three families:

| RPC | Family | Request (essentials) | Response (essentials) |
|---|---|---|---|
| `RunCircuit` | sync | `TaskSpec` | terminal `status`, `task_id`, `counts`, `transpile{depth,2q,total}`, `backend_used`, `usage_seconds` |
| `SubmitTask` | async | `TaskSpec` | `task_id` (provider job ID), `transpile`, `backend_used`; returns immediately |
| `WatchTask` | async | `task_id` | stream of `{status, message}` until terminal or the stream ceiling |
| `FetchTaskResult` | async | `task_id` | `counts`, `usage_seconds`; the executor drops the task after delivery |
| `ConvertSource` | utility | `{format, body}` | OpenQASM 3 text |
| `DrawCircuit` | utility | `{format, body}` | ASCII drawing |
| `ScheduleCircuit` | utility | source, target, optional level | `ops[{name,qubits,start_dt,duration_dt}]`, `total_duration_dt`, `dt_seconds` |
| `ProbeBackend` | utility | `provider`, `backend_name` | qubits, basis gates, coupling edges, calibration time, error/coherence/duration medians, dt, processor identity |

`TaskSpec` carries: `idempotency_key`
(`<Circuit-UID>/<observedGeneration>`), `qasm`, `shots`,
`target{provider, backend_name, kind}`, optional `optimization_level` and
`timeout_seconds`, and the two Tier-2 passthrough Structs
(`transpile_options`, `execute_options`).

Error semantics, the contract's most important property: adapter and
provider failures are reported in-band, as `status=FAILED` plus an
`error_reason` that is a Circuit condition reason and an `error_message`.
`SubmitTask` is the one exception: it uses gRPC status codes with the
reason encoded as `Reason: message` in the status details.
Transport-level errors mean "transient, retry"; in-band failures mean
"terminal, record on the Circuit". Clients must preserve that split; the
controller's whole retry story rests on it
([engineering.md](./engineering.md#2-reliability)).

## Networking

| Path | Protocol and port | Security |
|---|---|---|
| CLI / kubectl to API server | Kubernetes API | kubeconfig auth, TLS (cluster-managed) |
| controller to executor | gRPC, ClusterIP `quantum-circuit-controller-executor:9000` | plaintext, in-cluster only, no mTLS yet |
| controller to OTel Collector | OTLP/gRPC `:4317` (`monitoring` namespace) | plaintext, in-cluster convention |
| Prometheus to Collector | scrape `:8889` | in-cluster |
| kubelet to controller | health probes `:8081` (`/healthz`, `/readyz`) | HTTP |
| controller-runtime metrics | `:8443`, authn/authz-filtered HTTPS | served, not scraped by default |
| executor to IBM Quantum | HTTPS egress | vendor TLS, token auth |

The CLI never needs network reach to the executor or a provider. The
plaintext gRPC and OTLP hops are an explicit single-tenant, in-cluster
assumption; read the [security posture](./operations.md#security-posture)
before changing that topology.

## Behavioral notes

- Qiskit input: `source.format: qiskit` bodies are executed server-side
  in an isolated module namespace and the first `QuantumCircuit` is
  extracted (prefer a top-level variable named `circuit`); the converted
  OpenQASM 3 is persisted as the `convertedRef` artifact.
- Backend-name matching: a selector's `backendName` matches either the
  QPU's Kubernetes name (`fake-brisbane`) or its provider-native name
  (`fake_brisbane`).
- Effective qubits: selection prefers probed `status.qubits`, falling
  back to the `spec.qubits` hint.
- Credentials: one executor-level IBM token serves all `ibm` QPUs today
  ([operations.md](./operations.md#ibm-credentials));
  `credentialSecretRef` is future contract surface.
