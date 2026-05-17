# QCC Design State — Working Journal

**Author:** Ioannis Savvaidis (60663) — MSc Quantum Computing & Quantum Technologies, Democritus University of Thrace
**Supervisor:** Ioannis Karafyllidis
**Thesis title:** *Interface between Quantum and Classical Computers*

---

## Role of this document

This is the **working journal** for QCC implementation: milestone state, roadmap, design-decision log, parked thesis-context questions, and the critical-reading walk plan. It is the bridge between fresh conversations and the project's actual state so context survives session resets.

It is **not** a design source of truth. When this document and a source-of-truth document disagree, the source-of-truth wins:

| Source of truth | Owns |
|---|---|
| [`QCC-System-Design.md`](./QCC-System-Design.md) | Architecture, components, requirements (R1–R5), execution lifecycle, failure model |
| [`QCC-API.md`](./QCC-API.md) | `Circuit` and `QPU` CRD shapes, phases, conditions, reasons, artifact references |
| [`QCC-Observability.md`](./QCC-Observability.md) | OpenTelemetry `qcc.*` namespace, span shape, metric schema |

Citable from thesis chapters: the three above. Not citable: this journal.

---

## Roadmap and Implementation Status

Legend: ✅ shipped · ◐ partial · ⏳ pending (thesis-critical) · 🪪 post-thesis (engineering hygiene / distribution; not on thesis critical path) · ❌ explicit non-goal

*Last reality-checked: **2026-05-17 (evening)**, after the Path D+ thesis-scope decision narrowed the remaining work to ~3 days of code + writing (see the decision log for the rationale).  Status flags here reflect on-disk code, not aspirational doc text.*

### M1 — Local prototype on Aer

Single-node Kubernetes (kind), in-process Qiskit Aer simulator, no real-hardware path.  End-to-end correctness on the locked architecture and CRD contract.

| Item | Status |
|---|---|
| `Circuit` CRD (`qcc.io/v1alpha1`) — full schema per `QCC-API.md` §3 | ✅ |
| Controller (`cmd/qcc-controller`) — Go, kubebuilder v4, controller-runtime | ✅ |
| Executor (`qcc-executor`) — Python gRPC service, separate `Deployment` | ✅ |
| `RunCircuit`, `ConvertSource`, `DrawCircuit` RPCs | ✅ |
| CLI (`qcc`) — `run`, `draw`, `get`, `schedule`, `version` (5 verbs; `qpu` now reached via `qcc get qpu[s]` after the M1.5d kubectl-style restructure) | ✅ |
| Source formats: `openqasm3` + `qiskit` (server-side conversion via `ConvertSource`) | ✅ |
| Out-of-band artifact ConfigMaps (`drawingRef`, `convertedRef`) — `QCC-API.md` §3.7 | ✅ |
| `QPU` CRD — types + reconciler + bootstrap + selection wiring (Move 1) | ✅ |
| Examples (Bell, Deutsch, GHZ, Shor's N=15) | ✅ |
| Unit + envtest controller tests + Python pytest suite (controller 51%+, render 63%+, Python 30/30 green) | ✅ |
| Lint/build/test on `make` | ✅ |
| kustomize manifests + local-kind deploy targets | ✅ |

### M1.5 — Real fake-backend execution + observability surface + CLI polish

Bridges M1 (`QPU` CRD registered, selectable by name) to M2 (calibration-aware *scoring*) by making the executor honour `fake_*` backend names at *execution* time, surfacing the calibration data they carry, and polishing the CLI for kubectl-style consistency.  The empirical anchor for Ch7 — same circuit, two backends, visibly different histograms — works here, before full M2 lands.

| Item | Status |
|---|---|
| **M1.5a** — `AerAdapter` resolves `fake_*` names to `FakeProviderForBackendV2` backends (real coupling map, basis gates, noise model) | ✅ |
| **M1.5a** — `AdapterUnavailable` raised on unknown `fake_*` names → terminal `NoEligibleBackend` (not transient retry) | ✅ |
| **M1.5a** — Python tests covering both resolution paths + the Bell-on-FakeBrisbane end-to-end | ✅ |
| **M1.5b** — `ProbeBackend` executor RPC returning `num_qubits`, `basis_gates`, `coupling_edges`, calibration timestamp, error medians, T1/T2 medians, dt, instruction-duration medians, `processor_type` (family/revision/segment) | ✅ |
| **M1.5b** — `QPUReconciler` probes on registration; `spec.qubits` becomes optional, `status.qubits` becomes authoritative; same probe path works unchanged against live IBM hardware (verified M3 evening) | ✅ |
| **M1.5b** — `qcc qpu <name>` CLI subcommand rendering the probed status *(later subsumed by `qcc get qpu` in M1.5d)* | ✅ |
| **M1.5b** — `qcc get <circuit>` enriched with the resolved backend's specs + gate-error medians in the result-card | ✅ |
| **M1.5c** — Bundle of fake-backend sample QPUs (`fake-brisbane`, `fake-sherbrooke`, `fake-osaka`, `fake-torino`) in `config/samples/qpu/`; bundle later expanded to 7 fakes across 4 architecture generations (Eagle r3, Heron r1+r2, Falcon) | ✅ |
| **M1.5d** — Result-card unified renderer (Section, not Card) with Ch1-anchor rows; `dt`/duration plumbing through `QPU.status`; `Circuit.status.transpile.{depth,twoQubitGates,totalGates}` persisted for post-hoc reasoning | ✅ |
| **M1.5d** — CLI restructured kubectl-style: `qcc get <kind> [name]` with singular/plural and tab-completion; `qcc qpu fake-brisbane` → `qcc get qpu fake-brisbane`; Cobra error ergonomics (`SilenceErrors: false`, `argsWithHelp(...)` for missing-arg help) | ✅ |
| **M1.5d** — T1/T2 coherence medians through whole stack (Python `BackendMetadata` → proto → Go `BackendProfile` → `QPU.status.coherenceMedians` → render) | ✅ |
| **M1.5d** — Sectioned `qcc get qpu` + comparison list view; `processor_family` probe surfaces chip generation in the catalogue | ✅ |
| **M1.5e** — `mode=schedule` + `ScheduleCircuit` RPC: per-instruction timeline in dt cycles, ASCII timeline renderer, `scheduleRef` ConfigMap artifact, `qcc schedule <source>` + `qcc get circuit --schedule` | ✅ |

### M2 — Selection chain + observability

The five-move accuracy chain executed end-to-end, plus the OpenTelemetry instrumentation that makes the chain observable.  Develops against the fake-* + ideal-statevector backend set — **no real-hardware dependency** (M3 covers that).  This is the thesis-load-bearing milestone for R5 (calibration-aware selection), R2 (cross-boundary observability), and R4 (live cross-layer correlation).

| Item | Status |
|---|---|
| **Move 5 simple scoring** (predicted error budget = `errorMedians.twoQubit × transpile.twoQubitGates`); writer for `selectionSummary.score`.  R3 empirical-evidence path under Path D+ — the lighter formulation; the full composite formula stays 🪪 | ⏳ |
| Full Moves 2–4 + composite scoring (parallel calibrate, transpile per candidate, `mapomatic` layout, fidelity × freshness × queue weighting) | 🪪 |
| Per-`QPU` calibration cache (TTL ≈ 60 s) — only matters under load; thesis-scale runs don't stress it | 🪪 |
| OpenTelemetry traces — W3C Trace Context auto-propagation via `otelgrpc`; cross-boundary propagation through gRPC | 🪪 |
| Prometheus metrics — `qcc_*` namespace (8 Circuit + 6 QPU metrics; full inventory in `QCC-Observability.md` §5) | ✅ |
| `qcc_circuit_usage_seconds` (on-QPU billable) + `qcc_circuit_phase_duration_seconds_observed` (persistent gauge) | ✅ |
| `Circuit.status.traceId` populated by OTel (field reserved on schema; writer is OTel-trace work, deferred to Ch9) | 🪪 |
| Grafana dashboards (qpu + circuit, source-controlled YAML, cascading vars, sibling links, blue palette) | ✅ |
| Algorithm-grouping label convention (`qcc.io/algorithm`, `…-version`, `…experiment`, `…run-index`, `…source-sha256`) + controller auto-fill + metric propagation | ✅ |
| Cross-boundary identifier linkage (forward UID stamp into IBM job_tags + reverse `provider_job_id` as metric label) | ✅ |
| `honorLabels: true` on the ServiceMonitor so `namespace` reflects Circuit's namespace, not Collector's | ✅ |
| `qcc.*` semantic conventions documented (L0–L4 Kanazawa pyramid mapped) | ◐ |

### M2.5 — Composition Principle realization + outcome quality

Lands the Tier 2 per-stage passthrough that makes the §7a Composition Principle observable in user YAML, plus the outcome-quality metrics that depend on Tier 2's reproducibility primitives (seeded simulators against the `aer-statevector` ideal reference).  R5's "reproducible from the trace alone" property needs both halves.

| Item | Status |
|---|---|
| **Tier 1** typed CRD vocabulary (`shots`, `optimizationLevel`, `timeoutSeconds`, `backendSelector`, `mode`) | ✅ (shipped in M1) |
| **Tier 2** `Circuit.spec.transpile` opaque dict → `qiskit.compiler.transpile()` kwargs (snake_case, verbatim) | ✅ |
| **Tier 2** `Circuit.spec.execute` opaque dict → adapter submit kwargs (`SamplerV2.run` / `AerSimulator.run`) | ✅ |
| Proto carrier: `google.protobuf.Struct transpile_options` / `execute_options` on `TaskSpec`; CRD uses `+kubebuilder:pruning:PreserveUnknownFields` (`x-kubernetes-preserve-unknown-fields: true`) | ✅ |
| Wire-boundary subtleties: whole-number-float → int coercion on protobuf Struct's `NumberValue`; Tier-1 leakage into Tier-2 stripped with warning | ✅ |
| Sample CRs: `bell-state-seeded` (reproducibility on `aer-statevector`) + `bell-state-sabre` (non-seed kwargs on `fake-brisbane`) | ✅ |
| Multi-register `_extract_counts` (Teleport's `crz`/`crx`/`result`); today returns first register only | ⏳ |
| Outcome-quality metrics: Hellinger fidelity vs ideal, TVD against `aer-statevector` reference.  **Path D+ decision**: dropped — `aer-statevector` already provides an ideal reference visible against the noisy substrates in the Bell ladder figure; formalising the distance into a single number doesn't materially strengthen the Ch7 claim.  Reproducibility primitive is Tier 2's `seed` keyword on `spec.transpile` / `spec.execute`, already shipped | 🪪 |
| Outcome-quality CLI/RPC surface for structured cross-substrate comparison (ideal / fake / real ladder) | 🪪 |

### M3 — Real-hardware path via Qiskit provider ecosystem (in flight, mostly shipped)

Hardware adapter, async task-lifecycle RPCs (`SubmitTask` / `WatchTask` / `FetchTaskResult`), and the worked VQE demonstrator that closes Ch7 of the thesis.  Integration goes through the Qiskit provider ecosystem — **not via QRMI** (QRMI moves to Ch9 future-work as alternative substrate; see §7d).

| Item | Status |
|---|---|
| `IBMAdapter` (wraps `qiskit-ibm-runtime`: `QiskitRuntimeService` + `SamplerV2`) — credentials via `QISKIT_IBM_TOKEN` from Secret, channel via `QISKIT_IBM_CHANNEL` (defaults `ibm_quantum_platform`) | ✅ |
| Async path: `SubmitTask` / `WatchTask` / `FetchTaskResult` in servicer with in-memory `{task_id → (adapter, JobHandle)}` registry; 5s WatchTask poll cadence, 30-min stream deadline | ✅ |
| Controller reconciler refactor: sync (`runSync`) vs async (`submitAsync` + `pollAsyncJob`) dispatch by `QPU.spec.kind`; `PhaseRunning` now active (was no-op-equivalent) | ✅ |
| `--detach` flag on `qcc run`: exits when controller stamps provider job ID; default timeout bumped 5m → 30m for the blocking case | ✅ |
| `JOB` column in `qcc get circuits` + `job:` row in detail view (consistent provider-job-id display across simulator and real-hardware) | ✅ |
| Three IBM Heron r2 QPUs registered as samples (`ibm-fez`, `ibm-kingston`, `ibm-marrakesh`) | ✅ |
| `aer-statevector` ideal-noiseless reference promoted to first-class sample (resolver maps `aer_statevector` → `AerSimulator(method='statevector')`; provider construction encoded at the adapter boundary per §7a) | ✅ |
| Real-hardware probe path: `IBMAdapter.inspect()` reuses M1.5b Target-introspection helpers unchanged (substrate substitution at the adapter seam, observable in working code) | ✅ |
| `IBMAdapter.fetch_result` DataBin classical-register auto-detection (replaces brittle hard-coded `.meas`; handles `bit[n] c;` QASM 3 declarations, `measure_all()` Qiskit Python, named registers) | ✅ |
| **Empirical anchor**: `bell-state-8g5kf` on `ibm-kingston` (Heron r2, 156q, 2026-05-16) → `{00: 498, 01: 65, 10: 8, 11: 453}`, verified byte-identical to IBM Quantum console; full pipeline integrity `IBM Cloud → SamplerV2 → adapter → executor RPC → controller → CRD status → CLI` | ✅ |
| Queue position surfaced in `WatchTaskResponse.message` ("queued, position N") via `_format_status_message`; dedicated `Circuit.status.queuePosition` / `QPU.status.queueDepth` field is post-thesis polish | ✅ (stream message); 🪪 (dedicated field) |
| `QiskitProviderAdapter` generic + `qiskit-braket-provider` for IonQ/Rigetti/IQM/AQT/QuEra reach via AWS Braket aggregator | 🪪 |
| Worked VQE example: H₂ at canonical bond distance, hardware-efficient ansatz, end-to-end on IBM Quantum open tier.  **Path D+ decision**: dropped from thesis scope — the thesis claim is about orchestration, not algorithm design; the existing example set (Bell, Deutsch, GHZ, Shor N=15) is richer than the original outline projected, and adding VQE introduces a classical-optimiser iteration loop that's tangential to the orchestration story | 🪪 |

### M4 — Packaging + polish (post-thesis)

Production-readiness items for distribution and broader adoption.  **Explicitly post-thesis**: the manuscript ships code as a snapshot reference, not as a distributable artifact, and the thesis isn't being open-sourced at submission time.  R1's "Helm chart deploys cleanly on kind/k3d/EKS/AKS/GKE" sub-criterion is replaced for thesis purposes by the kustomize-based local-kind path that already works (M1).  R1's non-duplicating-submission property is argued structurally from the code in `QCC-API.md` §6 rather than verified by the restart test.  Both items will resurface if/when QCC is open-sourced or extended past the thesis.

| Item | Status |
|---|---|
| Helm chart (OCI registry distribution) | 🪪 |
| Cancellation finalizer on `Circuit` | 🪪 |
| `qcc list`, `qcc delete`, `qcc lint` CLI verbs | 🪪 |
| `make sync` target (`manifests → generate → install` in one step; prevents the "regen'd CRD on disk but not on cluster" footgun) | 🪪 |
| Examples-as-Ch7-experiments: scripts that generate Circuit YAMLs for systematic comparisons | 🪪 |
| Idempotency-under-restart test (R1 acceptance: 0 duplicate vendor submissions across 10 controller-restart trials) | 🪪 |

### Landscape view — % complete (thesis-critical scope, Path D+)

Single-table summary intended to be re-rendered after every meaningful slice.  Cross-references the per-milestone tables above; this view is the **scorecard**, those are the **spec**.

Scope is **Path D+** (see 2026-05-17 evening decision-log entry): all `🪪` items excluded from the denominator.  The 🪪 set is closed under the Path D+ decision; only the ⏳ items below remain.

| Milestone | Done | Total | % | Notes |
|---|---:|---:|---:|---|
| M1 — local prototype on Aer | 12 | 12 | **100%** | |
| M1.5 — fake backends + observability scaffolding | 10 | 10 | **100%** | |
| M2 — selection chain + observability | 6.5 | 8 | **81%** | Observability fully ✅; only **simple Move 5 scoring** + the `selectionSummary.score` writer remain (~2 days).  Full Moves 2–4 + composite formula + cal cache + OTel traces all → 🪪 |
| M2.5 — Composition Principle + outcome quality | 6 | 7 | **86%** | Tier 1+2 ✅; only **multi-register `_extract_counts`** remains (~1 day).  Hellinger/TVD/outcome-quality CLI all → 🪪 |
| M3 — real-hardware path | 11 | 11 | **100%** | Async path + IBM Heron r2 verified.  `QiskitProviderAdapter` generic + VQE worked example + dedicated queue-position field all → 🪪 |
| M4 — packaging + polish | — | — | **🪪** | Entire milestone post-thesis |
| **Total (thesis-critical, Path D+)** | **45.5** | **48** | **95%** | |

**Remaining 5% is two named items, ~3 days of code:**

1. **Move 5 simple scoring** (predicted error budget = 2Q error × 2Q gate count; populates `selectionSummary.score`) — gates R3 empirical evidence.  ~2 days.
2. **Multi-register `_extract_counts`** — unblocks Teleport-style multi-register demos if needed for Ch7 figures.  ~1 day.

After those, the only remaining thesis work is **writing Ch6 / Ch7 / Ch8 / Ch9** (~2–3 weeks).

When refreshing this table after a slice: update the per-milestone "Done" column, recompute the total, and add a one-line entry to the decision log noting *what slice moved which counter*.  Don't change the denominators without a separate scope decision documented in the log.

### Explicit non-goals (do not revisit without thesis-scope check)

❌ Multi-tenancy · ❌ multiple adapter implementations beyond the three named · ❌ error mitigation, circuit cutting, qubit reuse · ❌ near-time HPC interconnects (Phase 2/3 territory) · ❌ hardware multi-programming · ❌ pulse-level engineering / QEC · ❌ security as a contribution (inherited only) · ❌ OTEP submission of `qcc.*` schema (offered as candidate, not claimed)

---

## Design-decision log

Append-only record of design decisions and revisions, dated. Cross-references to source-of-truth docs are normative — this log is the *trail*, not the contract.

### 2026-05-17 · M2 application metrics shipped end-to-end via OTel SDK + OTLP push

The locked metric inventory from the 2026-05-16 (night, post-Tier-2) design entry shipped today, with one substantive direction change made during the implementation conversation: instead of registering metrics with controller-runtime's Prometheus registry (the scrape pattern), we use the OTel SDK with OTLP-gRPC push to the helm-deployed Collector.

**Why the architecture change**: the morning's pre-implementation discussion clarified that the canonical mid-2026 cloud-native pattern is OTel SDK in the application + OTLP push to an OpenTelemetry Collector + storage in Prometheus/Loki/Tempo (CNCF graduated OTel on 2026-05-11).  Choosing OTel SDK over `prometheus/client_golang` future-proofs the codebase for tracing and logs through the same pipeline — adding traces later is a config flip on the already-wired TracerProvider skeleton, not a new SDK install.  Exemplars on histograms then become automatic via `OTEL_METRICS_EXEMPLAR_FILTER=trace_based` when traces emit.

**What landed (one focused session, ~5h)**:

- **`internal/observability/` package** mirroring the `ioaiaaii.net` pattern (single orchestrator + per-signal subpackages):
  - `otel.go` — `Setup(ctx, cfg)` returns one shutdown closure; sets propagator globally; composes meter and tracer providers in order
  - `resource.go` — semconv-based resource builder pulling K8s downward-API attributes (`k8s.pod.{name,uid}`, `k8s.namespace.name`, `k8s.node.name`)
  - `config.go` — env-driven config with `OTEL_SDK_DISABLED` short-circuit for tests
  - `metrics/provider.go` — OTLP-gRPC exporter wired to PeriodicReader (30s push)
  - `metrics/qpu.go` — six QPU `ObservableGauge`s with one shared callback that lists from controller-runtime's informer cache (no apiserver round-trip on scrape)
  - `metrics/circuit.go` — four Circuit `ObservableGauge`s on the same cache pattern
  - `metrics/events.go` — synchronous `Int64Counter` + `Float64Histogram` with custom buckets (10ms–30m) for phase-transition events
  - `traces/provider.go` — skeleton TracerProvider, no-op exporter today (future tracing as config flip)
  - `logs/provider.go` — placeholder (slog → stdout primary; OTel logs deferred until v1 of the API)
- **`cmd/qcc-controller/main.go`** — replaces controller-runtime's zap logger with `slog` via `logr.FromSlogHandler(slog.NewJSONHandler(os.Stdout, ...))`; calls `observability.Setup` before `mgr.NewManager`; defers shutdown with bounded 10s timeout.
- **`internal/controller/circuit_controller.go`** — instruments `Reconcile` with a pre-phase capture + deferred `RecordPhaseTransition` so the counter increments and histogram observes once per actual phase change (not once per reconcile).
- **`qcc-executor/src/qcc_executor/adapters/ibm.py`** — cross-boundary identifier stamp: `Circuit.metadata.uid` (extracted from `TaskSpec.idempotency_key`) appended to `sampler.options.environment.job_tags` as `qcc.circuit.uid:<uid>`.  IBM Quantum Console now shows the originating Circuit on every job; reverse lookup is bidirectional.
- **`config/manager/manager.yaml`** — adds `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_INSECURE=true`, `OTEL_SERVICE_NAME`, plus downward-API env vars for K8s resource attributes.

**Two production-realism bugs caught and fixed in deploy iteration**:

1. **Schema URL conflict** — initial `semconv/v1.26.0` import conflicted with the SDK's internal default (v1.40.0) when `resource.WithSchemaURL` combined with `resource.WithFromEnv`/`resource.WithProcess`.  Fix: bump to `semconv/v1.40.0` to match the SDK.
2. **TLS handshake failure** — `otlpmetricgrpc.New(WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))` didn't disable TLS; the exporter's own option resolution took precedence.  Fix: use the canonical `otlpmetricgrpc.WithInsecure()` option + set `OTEL_EXPORTER_OTLP_INSECURE=true` in the Deployment env.

**End-to-end verification**:

```
QCC controller (OTel SDK)
   ↓ OTLP/gRPC :4317 (148 data points per push for QPU metrics alone)
otelcol-opentelemetry-collector (helm-deployed in monitoring ns)
   ↓ Prometheus exporter :8889
kube-prometheus-stack Prometheus
   ↓ PromQL
Grafana / direct queries
```

Verified `count(qcc_qpu_info) = 11`, `count by(processor_family)(qcc_qpu_info) = {Heron:6, Eagle:4, Falcon:1}`, all 12 metric names present, real values match IBM-published specs (ibm-kingston T1=174.58µs for Heron r2).

Applied a Bell circuit to drive phase transitions; the counter recorded `qcc_circuits_total{phase=Pending|Selecting|Transpiling|Submitting|Succeeded} = 1` for each phase entry, with the histogram capturing per-phase durations.

**What got intentionally deferred (Layer 2 from `QCC-Observability.md` §1)**:

- Direct scrape of controller-runtime's own `/metrics` endpoint (`controller_runtime_*`, `go_*`, `process_*`).  The kubebuilder scaffold's `config/prometheus/` directory is left commented out in `config/default/kustomization.yaml`; re-enabling needs a ClusterRoleBinding granting kube-prometheus-stack's Prometheus SA access to the controller's metrics-reader role.
- Executor-side (Python) instrumentation — per-RPC durations, per-adapter timings, provider-call latencies.  Today the executor emits nothing through OTel; its behaviour is observable transitively via the controller's view.

The user's framing for the separation: **application metrics (M2, shipped) answer thesis questions; QCC-internals observability is operational health and should be tackled as its own focused work later**.  This is the right scoping — `controller_runtime_*` belongs in production dashboards, not Ch7 figures.

**Files touched**:

- `go.mod` / `go.sum` — added `go.opentelemetry.io/otel/{,metric,sdk,sdk/metric,sdk/trace,trace,exporters/otlp/otlpmetric/otlpmetricgrpc,exporters/otlp/otlptrace/otlptracegrpc,semconv/v1.40.0}`
- NEW: `internal/observability/{config,resource,otel}.go`, `internal/observability/metrics/{provider,qpu,circuit,events}.go`, `internal/observability/traces/provider.go`, `internal/observability/logs/provider.go`
- `cmd/qcc-controller/main.go` — slog setup, observability.Setup wiring, deferred shutdown, Register* calls
- `internal/controller/circuit_controller.go` — defer-based phase-transition recording + two helpers (`latestConditionTime`, `latestConditionReason`)
- `qcc-executor/src/qcc_executor/adapters/{base.py,aer.py,ibm.py}` — `circuit_uid` parameter on `submit()`; IBM adapter stamps the job_tag
- `qcc-executor/src/qcc_executor/servicer.py` — `_circuit_uid_from_idempotency` helper; passes UID through to `adapter.submit`
- `config/manager/manager.yaml` — OTEL env vars + K8s downward-API
- `config/default/kustomization.yaml`, `config/prometheus/kustomization.yaml` — controller-runtime built-in scraping deferred (commented out, scaffold preserved)
- `docs/systems-design/QCC-Observability.md` — Layer 1/2/3/4/5 stratification, §12.5 marked deferred, §3 push-then-scrape architecture diagram refreshed

**What's still ahead in M2 / coming next**: Grafana dashboard JSON (committed in `deploy/grafana/`), the Layer 2 / QCC-internals observability work (its own focused session — instrument executor, surface controller-runtime built-ins, scrape RBAC), and tracing emission flip.

### 2026-05-17 (afternoon) · Dashboard polish + on/off-QPU decomposition + algorithm-grouping labels

Single focused session (~4h) that took the M2 metrics from "the metrics work" to "the observability layer the thesis evaluates against".  Five interlocking additions, each one earning its keep against a specific Ch7 figure or a Ch6 design claim.

#### 1. `qcc_circuit_usage_seconds` — substrate-reported billable compute time

New ObservableGauge derived from a new `Circuit.status.usageSeconds` field that the controller persists from the executor's `Job.usage()` call.  Plumbed end-to-end through the gRPC contract:

- **Proto** (`proto/qcc/executor/v1/executor.proto`): added `double usage_seconds` on `RunCircuitResponse.8` and `FetchTaskResultResponse.5`.
- **Python executor** (`qcc-executor/src/qcc_executor/adapters/`): new `FetchResult` dataclass (`counts: Mapping[str, int]`, `usage_seconds: float`) replaces the bare counts return from `fetch_result`.  IBM adapter calls `RuntimeJobV2.usage()` defensively — tries `job.usage()` as float, then as dict with `quantum_seconds`, then `job.metrics()["usage"]["quantum_seconds"]`; falls back to 0.0 on any failure (best-effort).  Aer returns 0.0 (no QPU-time concept for local CPU compute).
- **Go executor client** (`internal/executor/client.go`): new `TaskResult` struct widens `FetchTaskResult` from `(map[string]int64, error)` to `(TaskResult, error)`; `RunCircuit`'s `Result` struct gains a `UsageSeconds` field.
- **CRD field** (`api/v1alpha1/circuit_types.go`): `Status.UsageSeconds float64` with the omitempty doc note "zero / omitted on simulator paths".
- **Metric** (`internal/observability/metrics/circuit.go`): emitted **only when value > 0** — simulator runs produce no series, so any non-zero value in Prometheus reliably represents real-hardware compute.

The decisive feature is the pairing with `qcc_circuit_phase_duration_seconds_observed{phase="Running"}` (next item).  The difference between the two — `Running − usage_seconds` — is the orchestration-overhead window: queue wait + transit + IBM-side pre/post + controller poll cadence.  For `bell-state-4b98x` on `ibm-kingston` (2026-05-17), that came out to `11s − 3s = 8s` overhead, ratio 3.67×.  This is the Ch7 figure that quantifies the cost of going through QCC versus talking to the substrate directly.

#### 2. `qcc_circuit_phase_duration_seconds_observed` — persistent phase-timing gauge

A persistent companion to the synchronous histogram, derived from `Circuit.status.conditions[].lastTransitionTime` deltas on every scrape (not on transition).  Granularity is at K8s-condition level (4 phases: `Pending` from creationTimestamp → `Accepted`; `Selecting` from `Accepted` → `Selected`; `Submitting` from `Selected` → `Submitted`, folding Transpiling because conditions don't track it; `Running` from `Submitted` → `Completed`|`Failed`).

The histogram (kept) is the right primitive for fleet-wide percentiles via `histogram_quantile`.  The gauge is the right primitive for per-Circuit drill-down panels — it survives controller restarts and Prometheus's 5-minute staleness window because the source-of-truth is the CR, not the in-SDK histogram state.  Before this work, the Circuit dashboard's "Time spent per phase" panel showed empty for every Circuit except the most recent one, because no further transitions were firing and the synchronous emissions had aged out.  Now every Circuit in the cluster has phase data for as long as the CR exists.

#### 3. Algorithm-grouping label convention (`qcc.io/*`)

Five reserved labels on `Circuit` resources for cross-run correlation.  Convention documented in `QCC-API.md` §5.3:

- **User-authored** (set at submission): `qcc.io/algorithm`, `qcc.io/algorithm-version`, `qcc.io/experiment`
- **Controller-authored** (auto-filled on first reconcile, only when `algorithm` is present): `qcc.io/run-index`
- **Controller-authored** (auto-filled on first reconcile, always): `qcc.io/source-sha256` (truncated SHA-256 of `spec.source.body` — 16 hex chars to fit K8s's 63-char label-value cap; 64 bits of entropy)

**Auto-fill mechanics**: `ensureAlgorithmLabels` runs early in `Reconcile()`, before the phase dispatch.  Patches `metadata.labels` with whichever of `run-index` + `source-sha256` are missing, then returns Requeue to pick up the patched state.  `run-index` is `max(existing siblings' run-index) + 1` within the same namespace + algorithm + experiment scope; race-condition under high concurrency is documented but accepted at thesis scale (production-grade fix is a ConfigMap-backed atomic counter per algorithm).

**Two submission paths**: the `qcc run` CLI gains `--algorithm`, `--version`, `--experiment`, `--label key=value` (repeatable) flags that translate directly to the `qcc.io/*` labels.  Direct `kubectl create -f circuit.yaml` works equivalently by hand-authoring `metadata.labels`.  The controller auto-fill behaves identically for both paths.

**Metric propagation**: all five labels are promoted from `metadata.labels[qcc.io/*]` to `qcc_circuit_info` metric labels (under the bare names `algorithm`, `algorithm_version`, `experiment`, `run_index`, `source_sha256`) via an explicit allowlist in `circuitInfoAttrs`.  Cardinality-neutral — all five values are 1-to-1 with the Circuit.

**Truth-anchor property**: when a user runs `--version v2` against an unedited source body, the `source-sha256` label matches v1's.  `count by(algorithm, source_sha256, algorithm_version)(qcc_circuit_info)` will show the same hash appearing under two versions — the relabel is *visible* in the data.  This is a useful Ch7 / Ch8 diagnostic ("did v2 actually differ from v1, or just the label?").

#### 4. CLI surface evolution

`qcc run` gained the submission-side flags above (in §3) plus help-text examples.  `qcc get circuits` gained:
- `--algorithm`, `--version`, `--experiment` filter flags (server-side label selectors)
- Conditional `ALGORITHM / VER / RUN` columns — printed only when at least one Circuit in the list carries the corresponding label, so single-shot Circuits keep the lean three-column view
- Empty-list message now reports the active filter so users see *why* their list is empty

Both flag groups validate sensibly: `--version` / `--experiment` require `--algorithm` to be set (a version label without an anchoring algorithm is meaningless for grouping).

#### 5. Dashboard polish (Circuit + namespace label hygiene)

Two visual + structural changes:

- **`deploy/grafana/qcc-circuit-dashboard.yaml`**: Identity is now a clean key-value table with data links (`provider_job_id` → IBM Quantum Console, `qpu` → QPU substrate dashboard with `var-qpu` pre-filled).  Lifecycle row split into side-by-side panels — phase bargauge (left, persistent gauge) and on-QPU/off-QPU stat (right, the orchestration-overhead decomposition).  Outcomes panel kept as `barchart` (Qiskit `plot_histogram`-native), not `bargauge`.  New top-level `$algorithm` template variable cascades into the `$circuit` dropdown via `label_values(qcc_circuit_info{algorithm=~"$algorithm"}, circuit)` — same pattern as `$family → $qpu` on the QPU dashboard.  All panels reference a single `${datasource}` template variable instead of hardcoded uid.

- **`deploy/platform/otelcol-values.yaml`** namespace label fix: turned on `honorLabels: true` on the ServiceMonitor's metricsEndpoint.  Before this, Prometheus was renaming our metric's `namespace=<Circuit's namespace>` to `exported_namespace` (because the scrape target's `namespace=monitoring` collided), and we were *dropping* `exported_namespace` in the labeldrop regex — so every QCC metric had `namespace=monitoring` (the Collector's pod namespace) instead of the actual Circuit namespace.  Honor-labels flips the precedence so the metric's value wins, and `exported_namespace` no longer exists.  Verified by submitting a Circuit in a fresh `qcc-test` namespace and observing `qcc_circuit_info{namespace="qcc-test"}` — multi-namespace works correctly.  Also extended the labeldrop regex from `otel_scope_name` to `otel_scope_.*` to catch the whole OTel meter-scope metadata block.

#### Docs refreshed in this pass

- `QCC-API.md` §3.5 (added `usageSeconds`; corrected `traceId` description — it's a reserved field for future OTel trace propagation, NOT populated today as earlier text claimed), NEW §5.3 (reserved labels convention)
- `QCC-Observability.md` §2.1 (5 new question→signal rows, "Note" rewritten because the per-Circuit/aggregate split has softened with the new persistent gauge), §4.4 (type discipline table), §5.2 (Circuit metrics inventory 6→8), §5.4 (active-series envelope recomputed), §6 (cross-boundary linkage restructured around bidirectional flow), §7.1 (status fields), §10.2 (per-Circuit PromQL: phase durations now from the persistent gauge, not `_sum / _count`), §10.3 (aggregate PromQL refreshed — removed the unworkable "total shots per QPU" query because shots is a label not a value), NEW §10.4 (algorithm-grouping queries), §10.5 (renumbered from old §10.4), §11 (sketch → "Dashboards (shipped)"), §12.6.1 (honor-labels + relabeling rationale)
- `QCC-System-Design.md` §6.3 (adapter signature reflects `FetchResult` + `circuit_uid`), §11 (question table refreshed with new gauge + orchestration-overhead row + algorithm-grouping row)

#### What's intentionally not in this slice (deferred / declined)

- **`qcc list algorithms`** aggregated CLI view (rollups per algorithm: run count, success rate, avg QPU time) — discussed and explicitly declined by the user for thesis scope.
- **Grafana "fleet" dashboard** with per-algorithm panels grouped by `$algorithm` template var — discussed and explicitly declined; the existing Circuit-detail dashboard's `$algorithm` cascade variable was deemed sufficient.
- **CircuitTemplate / Experiment higher-level CRD** that generates child Circuits — Ch9 future work; convention documented in `QCC-API.md` §5.3.
- **In-file metadata comments** (`# qcc-algorithm: vqe-h2` at the top of a `.py` / `.qasm` source) — discussed, deferred.  The label-flag and CRD-authoring paths cover the same use case more transparently.
- **Path-convention auto-derivation** (`circuits/<alg>/<ver>.<ext>` → labels) — discussed, declined as "too magic"; explicit `--algorithm` / `--version` flags are the cleaner contract.
- **Monotonic-forever `run-index`** (ConfigMap-backed atomic counter, never reuses after deletes) — production-grade; deferred.  Thesis-scale max+1-within-current-siblings is documented.
- **Substrate-reported usage on simulators** (measuring local CPU wall-clock and surfacing it as a stand-in for `usage_seconds`) — deliberately not done.  Aer returns 0.0 so `qcc_circuit_usage_seconds` reliably means "real-hardware compute time" without a `substrate_kind` qualifier.

### 2026-05-17 (evening) · Path D+ thesis-scope decision — narrows remaining work to ~3 days of code

After a reality-check on what an MSc thesis actually needs to defend (vs what would make a stronger artifact), reclassified a substantial portion of M2 / M2.5 / M3 to `🪪 post-thesis`.  The thesis claim is **about orchestration**, not about quantum-algorithm selection optimality or about formal cross-substrate fidelity measurement.

**What moved `⏳ → 🪪` (with reasoning):**

- **Full Moves 2–4 + composite scoring formula** (`mapomatic` layout, fidelity × freshness × queue weight) — replaced for thesis purposes by a **simpler Move 5** (predicted error budget = `2Q error × 2Q gate count`).  All inputs already exist on `QPU.status` and `Circuit.status.transpile`; the simpler formulation gives R3 empirical evidence without committing to the full chain.  Full Moves 2–4 + composite formula become Ch9 future work.
- **Hellinger / TVD outcome-quality metrics** — `aer-statevector` already provides an ideal reference, and the Bell ladder figure already in the repository shows substrate-induced noise visibly against that ideal.  Formalising the distance into a single number doesn't materially strengthen the Ch7 claim; the visual comparison is publication-grade.  Reproducibility primitive is Tier 2's `seed` keyword on `spec.transpile` / `spec.execute`, already shipped via `bell-state-seeded`.
- **Outcome-quality CLI/RPC surface** (`qcc compare` verb) — without Hellinger/TVD as backing metrics, no need for a comparison verb.  PromQL queries from a dashboard panel are enough.
- **VQE H₂ worked example** — the thesis is about *orchestration*, not algorithms.  The existing example set (Bell, Deutsch, GHZ, Shor N=15) is already richer than the original outline projected.  Adding VQE introduces a classical-optimiser iteration loop tangential to the orchestration claim and risks scope creep into hybrid-algorithm territory that the thesis explicitly scopes out (R5 framing).
- **Per-QPU calibration cache (TTL)** — only matters under load; thesis-scale runs (~50 Circuits total) don't stress the unmediated path.
- **OpenTelemetry distributed tracing** (already deferred earlier) — restated as 🪪 for legend consistency; R4 satisfied via the UID + provider_job_id cross-boundary identifier linkage that shipped 2026-05-17 afternoon.
- **`QiskitProviderAdapter` generic + Braket reach** — already optional; cleanly belongs alongside QRMI as Ch9 future-work (vendor-coverage extension).
- **Dedicated `status.queuePosition` + `QPU.status.queueDepth` fields** — queue position is already in the WatchTask stream message; the typed field is polish, not evidence.

**What remains `⏳` (Path D+ critical):**

- Move 5 simple scoring + `selectionSummary.score` writer (~2 days; M2)
- Multi-register `_extract_counts` (~1 day; M2.5) — needed only if Ch7 figures lean on Teleport-style multi-register demos

**Total remaining code: ~3 days.**  Then writing Ch6 / Ch7 / Ch8 / Ch9 (~2–3 weeks).

**Scorecard moves from 82% → 95% thesis-critical** (because the denominator shrinks more than the numerator stays the same — most of the previously-`⏳` items leave the denominator entirely).

**What this commits**: doc-only.  Zero code change.  Reduces thesis scope to what an MSc artifact actually needs to defend, with the dropped items framed honestly as future work in Ch9.

**Examination-risk assessment**: the strongest examiner question is now *"why didn't you implement full Moves 2–4 / the composite-scoring formula?"*  Answer: "The simpler formulation evidences R3 with the same input data; the composite formula adds knob-tuning that's optimisation work, not architectural work.  The mapomatic layout pass is named future work in §9.X — the thesis claim is the chain *design* + the *minimal implementation that demonstrates the principle*, not the optimum scoring function."  That's a thesis-defensible answer for an MSc.

### 2026-05-17 (late afternoon) · M4 reclassified as post-thesis; Landscape scorecard added

Two small, related Roadmap-bookkeeping updates after today's reality-check conversation:

**M4 reclassified `⏳ → 🪪` (post-thesis).**  M4 was always framed as not-on-thesis-critical-path, but the items still carried `⏳` (pending) markers, conflating "thesis-critical pending" with "engineering-hygiene pending".  Today's decision: the thesis ships code as a snapshot reference, not as a distributable artifact, and isn't being open-sourced at submission time.  Therefore:

- Helm chart, cancellation finalizer, `qcc list/delete/lint` verbs, `make sync`, examples-as-experiments, idempotency-under-restart test → all `🪪`.
- R1's "Helm chart deploys cleanly across kind/k3d/EKS/AKS/GKE" sub-criterion is replaced for thesis purposes by the kustomize-based local-kind path that already works (M1).
- R1's non-duplicating-submission property is argued structurally from the code in `QCC-API.md` §6 rather than verified empirically by the restart test.  The argument is robust; the test would have been supporting evidence, not load-bearing.
- The 🪪 marker is new (5-state legend: ✅ ◐ ⏳ 🪪 ❌).  It means "deferred indefinitely; will resurface if/when QCC is open-sourced or extended past the thesis", distinct from ❌ which is "never going to do this" and from ⏳ which is "actively pending and thesis-critical".

**Landscape scorecard added.**  New subsection right after the M-tables ("Landscape view — % complete (thesis-critical scope)") with a single rolled-up table: Done / Total / % per milestone, weighted by item count.  Intended to be re-rendered after every meaningful slice — the table is the **scorecard**, the M-tables remain the **spec**.

Current scorecard reading: 46/56 items = **82% of thesis-critical scope**.  Remaining 18% concentrated in three named items: selection-chain Moves 2–5 (R5), Hellinger/TVD outcome quality (Ch7 figure), VQE H₂ worked example (Ch7 narrative).

**What this commits**: doc-only.  No code changes.  Removes the framing ambiguity around M4 and gives a single place to read "where we are" without re-tallying tables.

### 2026-05-16 (night, post-Tier-2) · M2 metrics design locked (pre-implementation)

Worked through the M2 observability surface in design space — explicitly *not* implementing yet.  Outcome: M2 observability is **Prometheus metrics only**, no OpenTelemetry distributed tracing.

**Why the pivot from the earlier OTel-heavy plan**:

- Initial M2.b plan assumed OTel + W3C trace context + Jaeger as the canonical observability substrate.  Pressure-tested it for thesis value and found: the implementation cost (~8–11h, two languages, multi-reconcile trace-propagation design problem) was disproportionate to the operational utility (the trace would mostly show what we already know — IBM queue wait dominates by orders of magnitude).
- The user's intuition for "what observability looks like" was the **circuit timeline** (`qiskit.visualization.timeline_drawer`), not OTel span trees.  That timeline already shipped in M1.5e (`mode=schedule` + `ScheduleCircuit` RPC + ASCII renderer).  The quantum-execution-domain visualization — which IS thesis-distinctive — is already done.
- Cardinality concerns about a `circuit` label on Prometheus metrics turned out to be production-SaaS-scale orthodoxy applied incorrectly to thesis scale (~hundreds of Circuits total, ~few hundred active series).  Pure label addition is fine and avoids the entire correlation-ID propagation tower I'd been proposing.
- Kanazawa et al. **do not endorse OTel** — the paper doesn't mention OpenTelemetry, distributed tracing, or correlated logging.  The "OTel + W3C + Prometheus" framing in R2's original wording was a thesis-side projection, not a Kanazawa requirement.  Kanazawa names a 5-layer pyramid (L0 hardware / L1 system / L2 job / L3 task / L4 domain); the *substrate* implementing it is open.
- R4's "single trace context propagates across the classical–quantum boundary" can be satisfied by a **cross-boundary identifier stamp** (Circuit UID in IBM `runtime_options.tags`) — a ~1h add — without distributed-tracing SDK weight.

**What landed (design only, no code)**:

- **`QCC-Observability.md` — rewritten as the canonical observability source-of-truth.**  Absorbs the M2 metric design + idiomatic principles + wiring + PromQL patterns + dashboards + evaluation mapping into the existing observability document.  15 sections, single-doc coherence, no parallel/competing artifacts.  12 QCC-specific metrics (6 QPU + 6 Circuit) plus controller-runtime freebies.  Documents the two-pattern split (KSM-style gauges for resource state; controller-runtime-style counters for events), naming / labelling / cardinality / bucket conventions, info-metric and condition patterns, controller-only scope, PromQL query patterns, and the cross-boundary identifier-linkage approach replacing OTel tracing.
- **Deleted `M2b-otel-tracing-plan.md` and `M2-metrics-design.md`** — superseded.  The M2-metrics-design.md was a transitional artifact; after the design landed it was migrated into Observability.md (the single source of truth), per user direction "use QCC-Observability.md as source of truth for observability story."
- **R2 and R4 rewordings in `01-requirements-re-evaluation.md`** — R2 dropped the "OpenTelemetry, W3C Trace Context" preordination, retains "Prometheus-compatible exposition + Kanazawa L0–L3 coverage."  R4 reframed from "single OTel trace context" to "single stable identifier (Circuit UID) propagating via gRPC metadata + IBM `runtime_options.tags`."  R5 dropped "OTel spans" wording from acceptance criterion (selection record now lives on `Circuit.status.selectionSummary` + Events + metrics).
- **`QCC-System-Design.md` §11 (observability model)** updated to point at the new canonical Observability doc and to explicitly name the no-OTel decision.

**The Kanazawa-pyramid mapping for QCC's revised stack**:

- L0 hardware → `qcc_qpu_*` node-exporter metrics (Seelam-aligned)
- L1 system → controller-runtime built-in process metrics + reconciler metrics
- L2 job → `qcc_circuits_total` counter + K8s Events on Circuit CRD
- L3 task → `Circuit.status` itself (per-task records on the CRD, queryable via `kubectl get`)
- L4 domain → out of scope today; M2.5 outcome-quality work (Hellinger fidelity, TVD) when it lands

L3-as-CRD-status mirrors Kanazawa's L3-as-Prefect-metadata: same architectural pattern, different K8s-native substrate.  That's the thesis-distinctive piece — same observability framework, demonstrably implementable on cloud-native primitives.

**R2 wording revision queued** for `01-requirements-re-evaluation.md`: drop the "OpenTelemetry + W3C Trace Context" requirement (it was preordained without design grounding); keep "Prometheus-compatible exposition + Kanazawa L0–L3 coverage."  The substrate substitution (K8s + Prometheus + CRDs in place of Prefect + Superset) is what we're claiming.

**Next session**: implement M2 metrics following `M2-metrics-design.md`.  Build QPU `_info` collector first (exercises the whole wiring path), then add the rest one collector at a time.  After metrics land, add the cross-boundary ID stamp (~1h).  Then M2.a (selection scoring) becomes the remaining M2 work.

**Files touched**: `docs/systems-design/M2-metrics-design.md` (new); `docs/systems-design/M2b-otel-tracing-plan.md` (deleted).

### 2026-05-16 (night) · Tier 2 per-stage passthrough shipped end-to-end

The Composition Principle's Tier 2 (§7a) — `Circuit.spec.transpile` and `Circuit.spec.execute` as opaque dicts forwarded verbatim to the upstream Qiskit functions — is now plumbed through every layer from CRD to adapter call site.  Tier 1 already existed (typed vocabulary: `shots`, `optimizationLevel`); Tier 2 was the planned next step named in the 2026-05-16 (late++) entry.

**What landed (one session, ~1 hour)**:

- **CRD fields** added on `Circuit.spec`: `transpile` and `execute`, typed as `*apiextensionsv1.JSON` with `+kubebuilder:pruning:PreserveUnknownFields` + `+kubebuilder:validation:Schemaless`.  The CRD schema carries `x-kubernetes-preserve-unknown-fields: true` so arbitrary user keys survive `kubectl apply` round-trips.
- **Proto carrier** added on `TaskSpec`: `google.protobuf.Struct transpile_options = 7` and `execute_options = 8`.  Struct is the canonical opaque-dict envelope across language boundaries.
- **Go client** (`internal/executor/client.go`) decodes the CRD's `apiextensionsv1.JSON` bytes via `json.Unmarshal → structpb.NewStruct`, applied identically in `RunCircuit` and `SubmitTask`.  Malformed JSON or unrepresentable values surface as `TaskError{Reason: InvalidCircuit}` — terminal, no requeue.
- **Python servicer** (`qcc-executor/src/qcc_executor/servicer.py`) decodes the proto Struct via `json_format.MessageToDict` (flattens nested Structs/Lists in one pass) and forwards as `options=` kwarg to `adapter.transpile()` and `adapter.submit()`.  Adapter base class signatures extended; AerAdapter + IBMAdapter merge options into the kwargs dict for `qiskit.compiler.transpile` and `AerSimulator.run` / `SamplerV2.run` respectively.

**Two wire-boundary subtleties surfaced and handled**:

1. **protobuf Struct's `NumberValue` is double-only.**  An integer literal in YAML (`seed_transpiler: 7`) reaches the Python side as `7.0`, which Qiskit's strict signature rejects (`ValueError: Expected non-negative integer`).  A `_coerce_integers` walker converts whole-number floats back to `int` at decode time.  This is a deliberate, documented coercion: the user wrote an integer in YAML, the alternative (forcing `7.0`) violates the snake_case-to-Qiskit promise.  Bools are subclass-checked first so `True` doesn't become `1`.
2. **Tier-1 leakage into Tier-2.**  A user-supplied `shots: 1` inside `execute` would either silently override the dedicated `shots` field or raise on duplicate kwarg.  The servicer strips Tier-1 keys (currently `{shots}`) from `execute_options` and logs a warning — the user's intent is recoverable (Tier-1 was their real intent either way) and a hard rejection on the happy path adds friction without benefit.  Tier-1 wins; that's the §7a contract.

**§7a casing decision: snake_case in passthrough blocks.**  The Composition Principle's casing rule (camelCase keys, verbatim values) applies to Tier-1 typed fields.  Tier-2 passthrough blocks now explicitly use **snake_case keys** to match Qiskit's parameter names directly — `seed_transpiler`, `layout_method`, `routing_method`, `seed_simulator`, `memory`.  No translation happens between YAML and Qiskit's call site; that is precisely the promise of passthrough.  The CRD-typed Tier-1 surface stays camelCase (`backendSelector`, `optimizationLevel`, etc.) — the two conventions don't collide because Tier 2 is a schemaless opaque block.

**Reproducibility smoke test landed** (`tests/test_server.py::test_run_circuit_forwards_tier2_passthrough`): runs the same Bell circuit twice on `aer_simulator` with `execute.seed_simulator: 42`, asserts identical counts; runs a third time with `seed_simulator: 43`, asserts different counts.  Verifies the dict reaches `AerSimulator.run` without translation.  Plus the Tier-1 precedence test (`test_run_circuit_drops_tier1_keys_in_execute_options`).  31/31 Python tests pass; full Go test suite stays green.

**One stale-test fix in passing**: `tests/test_server.py::test_submit_task_is_unimplemented` was asserting the async trio raises `UNIMPLEMENTED`, but M3 (evening) landed the implementation.  Replaced with `test_submit_watch_fetch_async_lifecycle` — submits a Bell circuit, watches to terminal, fetches counts, verifies registry cleanup on a second fetch.  Plus the QRMI adapter stub gained `schedule()` + the new options-aware `transpile`/`submit` signatures so the abstract-method check reaches the constructor's `AdapterUnavailable` (was: confusing `TypeError`).

**What this commits**: the §7a Composition Principle is no longer aspirational — Tier 1 *and* Tier 2 ship working code.  The architectural discipline "QCC composes upstream toolchains rather than re-implementing them" is now observable in user YAML: `Circuit.spec.execute.seed_simulator: 42` is the user reaching past QCC's vocabulary to Qiskit's, with QCC stepping aside to forward verbatim.

**Files touched**: `proto/qcc/executor/v1/executor.proto` (Struct fields); `gen/proto/qcc/executor/v1/*` + `qcc-executor/src/qcc_executor/proto/qcc/executor/v1/*` (regen); `api/v1alpha1/circuit_types.go` (CRD fields); `config/crd/bases/qcc.io_circuits.yaml` + deepcopy (regen); `internal/executor/client.go` (decode + forward); `qcc-executor/src/qcc_executor/adapters/{base,aer,ibm,qrmi}.py` (options-aware signatures + verbatim forwarding); `qcc-executor/src/qcc_executor/servicer.py` (Struct→dict + int-coercion + Tier-1 strip); `qcc-executor/tests/test_server.py` (reproducibility + precedence tests, async lifecycle test).

**What's still ahead**: thesis-side worked examples that exercise Tier 2 — VQE H₂ end-to-end demonstrator is the obvious one (`seed_simulator` for reproducibility across ansatz iterations); multi-register `_extract_counts` support; M2 selection scoring.  Tier 2 unblocks reproducible cross-substrate comparisons in Ch7 — the ideal/fake/real ladder now produces identical counts on each rerun when seeds are set.

### 2026-05-16 (late+++) · `QCC-System-Design.md` aligned with post-yesterday direction

The canonical engineering source-of-truth doc (`QCC-System-Design.md`) was 2 days stale relative to the discussions and decisions captured in this Design-State journal.  This entry records the alignment edits.

Updated sections, in source order:

- **§6 architecture diagram** — vendor edge label updated from "IBM Quantum, Pasqal via QRMI" to the Qiskit-provider-ecosystem reach (`qiskit-ibm-runtime`, `qiskit-braket-provider` aggregating IonQ/Rigetti/IQM/AQT/QuEra).
- **§6.2 async task-lifecycle RPCs rationale** — primary alignment is now Qiskit's `JobV1` contract (universal across provider plugins).  QRMI's `task_start/status/result` is mentioned as future-work alternative.
- **§6.3 adapter table** — dropped `IBMAdapter` and `QRMIAdapter` rows.  Replaced with `QiskitRuntimeAdapter` (M3 primary) and optional `QiskitProviderAdapter` (generic, M3).  Vendor coverage paragraph rewritten: comes from Qiskit's `qiskit.providers.Backend` ecosystem, not per-vendor adapter code.  Pointer added to §7d (QEI direction).
- **§7 component responsibilities** — qcc-executor row updated to reflect AerAdapter shipping today + QiskitRuntimeAdapter landing in M3.
- **§14 constraints** — replaced "two interchangeable paths IBMAdapter+QRMIAdapter" with "`QiskitRuntimeAdapter` primary, `QiskitProviderAdapter` optional generic, both wrap the Qiskit provider ecosystem".  QRMI + CUDA-Q named as Ch9 future-work alternative substrates.
- **§15 limitations** — vendor coverage description updated to reflect the Qiskit-provider-ecosystem reach + Ch9 future-work pointer.  The QRMI-integration paragraph rewritten as **"Qiskit provider ecosystem integration"** (the actual primary path); a new **"Alternative substrates (Ch9 future-work)"** paragraph names QRMI and CUDA-Q as deferred directions.
- **§16 thesis chapter mapping** — replaced "QRMI integration → Chapter 6" with "Qiskit provider ecosystem integration → Chapter 6/7" and added a "QEI direction → Chapter 9 future-work" row.
- **§17 thesis-safe summary** — the **positioning sentence** rewritten.  Old version cited QRMI as load-bearing for QCC's vendor-abstraction role (*"sharing the QRMI library for vendor abstraction"*).  New version: positions QCC as the **open-source K8s-native counterpart to managed proprietary quantum clouds** (IBM Quantum Platform, AWS Braket, Azure Quantum), sharing Qiskit's provider abstraction.  The Slurm SPANK + QRMI parallel is preserved but reframed as the HPC counterpart of QCC, not its vendor-abstraction substrate.  Adds the substrate-substitution future-work sentence pointing at Ch9.

**What this commits**: doc-only.  Zero code change.  Removes the contradiction between Design-State (live journal) and System-Design (canonical truth).  Both now reflect the post-yesterday direction (Composition Principle locked, QRMI deferred to Ch9, Qiskit provider ecosystem as M3 vendor-reach mechanism, M2/M2.5/M3/M4 Roadmap split).

**What still needs updating** (not done today, captured here as known staleness):

- `QCC-API.md` — still references `resultRef` (deleted), `openqasm3` + `dynamicCircuits` capability examples (removed), and the `IBMAdapter` / `QRMIAdapter` provider dispatch table.  Schema-level staleness; less load-bearing for thesis prose.  Defer to a focused fix session (~30 min) when convenient.
- `QCC-Observability.md` — mostly aligned, but does not yet incorporate the Kanazawa L0–L4 layered telemetry framework or outcome-quality metrics discussed but never landed.  Defer to M2.5 (observability implementation).

### 2026-05-16 (evening) · M3 async path validated end-to-end · first real-hardware Bell

**Milestone**: M3's core thesis-evidence claim — that QCC's architecture absorbs real-hardware substrates through the adapter seam — is now verified by working code, not just by §7a/§7d prose.

**What landed today (one session, ~5 hours)**:

- **`QiskitRuntimeAdapter` wired into the executor** — previously a stub since M2 planning; now active.  Reads `QISKIT_IBM_TOKEN` from the K8s Secret `ibm-quantum-token` (mounted via `config/manager/executor.yaml`'s env block with `optional: true` so the executor still starts cleanly without IBM credentials).  Channel defaults to `ibm_quantum_platform` (the current IBM Quantum Platform channel; legacy `ibm_quantum` is deprecated); overridable via `QISKIT_IBM_CHANNEL`.
- **QPU controller's probe gate extended** — `desiredQPUStatus` now recognises `provider: ibm` as optimistically Available (was: Unknown).  Live IBM calibration flows through the existing probe path with zero new Go code — the `inspect()` Target-introspection helpers (`_median_instruction_error`, `_coherence_medians`, `_processor_identity`, `_backend_dt`, `_median_instruction_duration`) work unchanged because Qiskit's `Backend.target` shape is the same for `FakeProviderForBackendV2` snapshots and live `QiskitRuntimeService` backends.  The Ch7 substrate-substitution argument lands here in working code.
- **Three IBM Heron r2 QPUs registered** (`ibm-fez`, `ibm-kingston`, `ibm-marrakesh`) as sample QPUs alongside the seven `fake-*` snapshots and the new `aer-statevector` ideal reference.  All three QPU families coexist in one `qcc get qpu` list view — the catalog is provider-heterogeneous by construction.
- **`aer-statevector` ideal reference promoted to a first-class sample** — moved from `config/qpu/` (operator default, kustomize-prefixed) to `config/samples/qpu/` (user-applied, clean name).  Adapter resolver in `qcc-executor/src/qcc_executor/adapters/aer.py` extended with `_AER_METHOD_BACKENDS` dict mapping `aer_statevector` → `AerSimulator(method='statevector')`.  This is the Composition-Principle §7a pattern made concrete: provider construction encoded in the resolver, not in CRD YAML.  Operator-default QPU bundle is now empty by design.
- **Async task-lifecycle RPCs implemented** — `SubmitTask` / `WatchTask` / `FetchTaskResult` handlers in `qcc-executor/src/qcc_executor/servicer.py` carry an in-memory `{task_id → (adapter, JobHandle)}` registry.  WatchTask streams status frames at 5s cadence with a 30-minute deadline; the controller reads one frame per reconcile and closes the stream — matches K8s's natural polling cadence while keeping the streaming API available for richer clients.
- **Controller refactor: sync-vs-async dispatch by `QPU.spec.kind`** — `runOnExecutor` now dispatches simulator backends to the sync `RunCircuit` path (today's behavior, fine because Aer returns in seconds) and hardware backends to a new `submitAsync` + `pollAsyncJob` pair.  The `PhaseRunning` phase is now active (was a no-op terminal-equivalent before): each reconcile opens a short-lived WatchTask, reads one status frame, requeues if non-terminal, or fetches results and transitions to Succeeded if terminal.
- **Go executor client extended** with `SubmitTask` / `WatchTask` / `FetchTaskResult` methods.  TaskError unpacking from gRPC status details preserves the existing TaskError-dispatch path so the controller's transient-vs-terminal logic keeps working unchanged.
- **`--detach` flag on `qcc run`** — submits and exits as soon as the controller stamps the provider job ID, instead of blocking for the (potentially-minutes) hardware queue wait.  The K8s-native "submit + walk away" pattern, applied to circuit execution.  Default `--timeout` bumped from 5m → 30m for the blocking case (hardware queues commonly run minutes).
- **`qcc get circuits` (list) gained a `JOB` column**; the detail view's `submitted` row renamed to `job`.  Consistent provider-job-id display across simulator (`aer-<uuid>`) and real hardware (vendor's native ID like `d8463bg0bvlc73d46tqg`); same field (`Circuit.status.providerJobID`), same label, no surface drift.

**Bug found and fixed in the first real-hardware run**: `IBMAdapter.fetch_result` previously hard-coded `pub_result.data.meas.get_counts()` — but `SamplerV2`'s `DataBin` exposes one attribute per classical register declared in the circuit, named after the register.  OpenQASM 3 examples use `bit[2] c;` → `data.c`; `measure_all()` Qiskit Python uses `data.meas`; the Teleport circuit uses three registers (`crz`, `crx`, `result`).  The hard-coded `.meas` lookup raised `AttributeError`, which the executor's `try/except Exception` mapped to a `ProviderSubmissionFailed` terminal failure — but the **vendor-side job had succeeded**, the result was just unreadable on our side.

Fixed by introspecting the `DataBin`: iterate attributes, find the first one with `get_counts()`, use that.  Robust against any classical-register naming.  Surfaced as a new module-level helper `_extract_counts` in `qcc-executor/src/qcc_executor/adapters/ibm.py`.  Multi-register circuits (Teleport) return one register's counts — full multi-register support is M2.5 outcome-quality work.

**Empirical anchor: first real-hardware Bell**.  `bell-state-8g5kf` ran on `ibm-kingston` (Heron r2, 156q, calibrated 2h ago at submission time) with 1024 shots.  Counts retrieved through QCC's full async pipeline match the IBM Quantum console's histogram exactly: `{00: 498, 01: 65, 10: 8, 11: 453}`.  Data integrity verified end-to-end across `IBM Cloud → SamplerV2.run() → QiskitRuntimeAdapter.fetch_result → executor FetchTaskResult RPC → controller pollAsyncJob → Circuit.status.results → CLI histogram render` — every shot preserved.

Three observations worth recording for Ch7:

1. **Real-hardware noise behaviour visible.**  Off-diagonal mass (65 + 8) / 1024 ≈ **7.1%** vs ideal-statevector 0.0%.  Asymmetric: 8× more `01` than `10` outcomes — physical (Heron r2 has direction-asymmetric readout error: `P(measure 1 | true 0) ≠ P(measure 0 | true 1)`, qubit-specific).  This artefact wouldn't appear under any depolarising-noise simulator; only real hardware shows it.  Useful for Ch7 to argue *"the substrate-substitution path delivers fidelity to physical noise, not a clean abstraction over it"*.
2. **Prediction-vs-observed gap is honest and bounded.**  The CLI predicted `error exposure ≈ 0.005 events/shot (within gate-error budget)`, with the explicit caveat "excludes readout & coherence".  Adding readout (2 × 1.65e-02 ≈ 3.3%) to gate-error (0.5%) gives a predicted 3.8% off-distribution mass; observed 7.1% — within a factor of 2.  Gap is dominated by qubit-specific readout outliers (median underestimates them) plus crosstalk; both are explicitly out-of-scope for the median-based formula.  Ch7 paragraph writes itself: *"the gate-error budget was respected; the observed deviation tracks readout error, which the prediction formula explicitly excludes."*
3. **Three-substrate side-by-side comparison is now reproducible.**  Same Bell circuit on `aer-statevector` (ideal, 513/0/0/511), `fake-marrakesh` (frozen Heron r2 snapshot, predicts ~3% off-diagonal), `ibm-kingston` (live Heron r2 hardware, observes 7.1% off-diagonal).  Three rungs of the dev → cloud-sim → real-hardware ladder, one Circuit YAML structure differentiated only by `BackendSelector`.  This is the Ch7 anchor figure.

**Files touched**: `qcc-executor/src/qcc_executor/adapters/{aer,ibm}.py` (resolver dict, fetch_result fix, lazy import); `qcc-executor/src/qcc_executor/servicer.py` (async handlers + task registry); `internal/controller/{circuit_controller,qpu_controller}.go` (async dispatch + IBM provider recognition); `internal/executor/client.go` (Go-side async wrappers); `cmd/qcc/commands/{run,get}.go` (--detach flag, timeout bump, JOB column); `api/v1alpha1/qpu_types.go` (no schema change; the IBM probe path uses existing fields); `config/manager/executor.yaml` (Secret env mount); `config/qpu/{kustomization.yaml,aer-statevector.yaml}` (operator default now empty); `config/samples/qpu/{aer-statevector,ibm-fez,ibm-kingston,ibm-marrakesh}.yaml` + `kustomization.yaml`; `internal/controller/qpu_controller_test.go` (probe-driven IBM Available test) + `circuit_controller_test.go` (fakeExecutor stubs for the new interface methods).

**What's still ahead in M3**: VQE H₂ end-to-end demonstrator (the Ch7 worked example).  Optional: multi-register circuit support in `_extract_counts` (Teleport-style); per-job seed for reproducible simulator comparisons (the Tier 2 `execute` passthrough work, Session 2.5).

### 2026-05-16 (late++) · §7a corrections + Roadmap restructure (M2 / M3 / M4)

Two known-deferred fixes from yesterday's design-discussion landed:

**§7a corrections** — the Composition Principle's Tier-2 passthrough table previously listed `Circuit.spec.aer`, `Circuit.spec.qrmi`, and `QPU.spec.aer` as provider-scoped blocks.  All three were wrong: provider construction is *not* per-Circuit (multiple Circuits share a QPU instance; they shouldn't re-configure how its backend was built), and isn't a Tier-2 passthrough at all.  Removed those rows; added an explicit *"Provider construction is not in the CRD"* paragraph naming the alternative pattern: construction is encoded by `QPU.spec.provider` + `QPU.spec.backendName` + the adapter's internal resolver (e.g. `aer_statevector` → `AerSimulator(method='statevector')`).  Different constructions = different QPU CRs.  Also fixed the *"substrate-agnostic"* paragraph to reflect the Qiskit-provider-ecosystem direction for M3 (not QRMI), and the *"implementation status"* paragraph to drop the now-removed `aer` block from the planned Tier-2 work.

**Roadmap restructure** — M2/M3 conflated *"selection + observability + hardware"* in one milestone, which was both unrealistic for the thesis time budget and inconsistent with the post-QRMI direction.  Split into:

- **M2** — Selection chain (Moves 2–5, composite scoring, calibration cache) + Observability (OTel, Prometheus, dashboards).  Develops against fake-* backends.  **No real-hardware dependency.**  This is where R3 + R2 evidence comes from.
- **M3** — Real-hardware path via Qiskit provider ecosystem (`qiskit-ibm-runtime` first, optionally `qiskit-braket-provider` for the Braket-aggregated vendors).  Async task-lifecycle RPCs (`SubmitTask` / `WatchTask` / `FetchTaskResult`) + controller async refactor.  VQE H₂ end-to-end demonstrator.  **QRMI is not the integration path — it's Ch9 future-work** (§7d).
- **M4** (new) — Helm chart, cancellation finalizer, remaining CLI verbs, Ch7 experiment-as-YAML examples.

**What this commits**: doc-only.  Zero code change.  Removes contradictions between §7a (architectural principle) and the Roadmap (implementation plan).  Aligns both with the post-yesterday architectural decisions (Composition Principle locked, QRMI deferred to Ch9, Qiskit provider ecosystem as the M3 vendor reach mechanism).

**What this doesn't change**: the working code, the CRDs, the controller, or the M2/M2.5/M3 work that comes next.  The roadmap now describes the work; the work itself happens in subsequent sessions.

### 2026-05-16 (late) · QEI architectural direction locked as §7d (deferred from implementation)

Captured an architectural insight that's worth recording for Ch9 future-work but not implementing this thesis cycle: the **Quantum Executor Interface (QEI)** — formalizing the adapter pattern as a public Kubernetes-style interface modelled on CRI/CNI/CSI/Device-Plugin precedents.  The full direction is documented in §7d above.

**Decision: doc-only, deferred to post-thesis implementation.**  Reasons in order of weight:

1. The Composition Principle (§7a) already captures the architectural claim that the adapter is the seam where vendor/SDK churn is absorbed.  QEI is the formalization, not a structurally distinct property.
2. R2/R3/R4 evidence for the Ch5 §5.8 requirements comes from M2 (selection), M2.5 (observability), M3 (real hardware) — not from QEI promotion.  Thesis time on those directly satisfies the locked requirements; time on QEI does not.
3. Demonstrating QEI convincingly requires ≥2 plugins.  `qei-aer` alone doesn't make the interface plausible.  The second plugin (`qei-qiskit-ibm`) lands naturally with M3.
4. The natural migration moment is M3 work itself — no separate refactor needed.  Today's adapter pattern is operationally equivalent to a single-plugin QEI deployment.
5. The controller↔executor gRPC contract already lives in `proto/qcc/executor/v1/executor.proto`.  That proto is the QEI starting point when implementation lands.

**What this commits today**: §7d with the full design (K8s precedent, QEI proto sketch, QPU CRD field shape, migration plan, blast-radius table, Ch9 prose seed).  Zero code change.  Zero schema change.

**What this enables for Ch9**: a strong, citation-worthy future-direction paragraph — *"QCC defines QEI as a public interface modelled on CRI; current implementation uses in-tree adapters which are operationally a single-plugin deployment of the same pattern."*  Stronger contribution claim than "QCC has an internal Adapter pattern".

**What it doesn't change**: Composition Principle (§7a), Roadmap (M1–M3), Circuit/QPU CRD schemas, working code, the M2 selection scoring work that comes next.

### 2026-05-16 · Dead-field cleanup + inline-vs-ref result-storage rationale

Companion patch to the Composition Principle lock (above): three dead fields removed from the CRDs.

**Removed:**

- `QPU.spec.capabilities.openqasm3` — declared "M1/M2 require true, future QASM-2-only adapters could be filtered out", but no code ever consumed it.  Selection in `filterEligibleQPUs` only checks `MaxShots`.  If a QASM-2-only backend ever appears, the principle says we add the filter when it's needed, not preemptively.
- `QPU.spec.capabilities.dynamicCircuits` — same shape: declared, no consumer.  When dynamic-circuit Circuits become first-class (currently nothing in the CRD distinguishes them), this can come back paired with a `Circuit.spec.requires.dynamicCircuits` selector.  Today it's dead weight.
- `Circuit.status.resultRef` + the `ResultRef` struct — intended as an out-of-band escape hatch for "what if results exceed etcd's value limit", but never wired (no controller produces it, no CLI reads it).

After this cleanup `QPUCapabilities` reduces to a single field (`maxShots`) — every field in the struct earns its keep.  Tests, lint, deepcopy/manifest regen all pass.

**Tradeoff recorded for `Circuit.status.results` (now confirmed inline-only).**

Architectural purity says all derived artifacts should follow the same out-of-band ConfigMap pattern we use for `drawingRef`, `convertedRef`, and `scheduleRef`.  Results don't, today — they live inline as `map[string]int64` on `.status.results`.  The inconsistency is *deliberate*: K8s itself doesn't enforce uniformity (Pod.spec.args is inline; container logs aren't; container images aren't); the principle is "out-of-band when the payload routinely exceeds inline limits", with the threshold set per data type.

For thesis-scale workloads, results stay small:

| Circuit shape | Distinct outcomes (≤ min(2^n, shots)) | Approx size |
|---|---|---|
| Bell / Deutsch / GHZ-2 to GHZ-5 | 2 – 32 | 40 B – 640 B |
| Shor's N=15 | 16 | ~320 B |
| Hypothetical VQE 10q × 8 192 shots | ≤ 1 024 | ~30 KB |
| Pathological 20q × 100 000 shots uniform | ≤ 100 000 | ~3 MB |

The pathological case (rightmost row) is the only one that crosses etcd's ~1.5 MiB hard limit — and it's unusual for real algorithms, which produce concentrated distributions, not uniform.  Migrating to a ConfigMap-backed `resultRef` now would buy uniformity but cost: extra K8s object per Circuit, dispatcher logic in controller + CLI, GC ordering, test surface — all for zero observable benefit at our scale.

**Trigger condition for revisiting.**  We add `resultRef` back when (a) a user's experiment surfaces `apply failed: etcd value too large`, or (b) a feature like `execute.memory: true` (per-shot bitstring sequences, not in scope today) lands.  Either is observable in the wild; the migration is strictly additive when it happens.

This rationale is recorded here, not at the field-level comment, so future audits can find the *why* alongside the rest of the decision history.

### 2026-05-16 · Composition Principle locked as §7a

After an extended design discussion across the prior session, the **Composition Principle** is locked into §7a (between Critical Design Choices and CLI Surface).  The principle commits QCC to a two-tier CRD model — Tier 1 typed vocabulary for QCC's own concepts, Tier 2 per-stage passthrough blocks (`transpile`, `execute`, `aer`) forwarded verbatim to upstream functions.  Together with the casing rule (camelCase keys, verbatim values), the conflict rule (vocabulary wins), the promotion path, and the CLI policy (orchestration-only flags), it fixes the architectural discipline that QCC composes upstream toolchains rather than re-implementing them.

The empirical anchor is `examples/upstream-test.py`, which already demonstrates substrate-substitution end-to-end (Qiskit script with hardcoded `FakeJakartaV2` + custom pass-manager runs through QCC routed to `fake-brisbane`, unchanged).  The Ch7-citable property the principle promises was observed in working code *before* the principle was stated — the lock is recognition, not invention.

Two consequences worth flagging:

1. **`options` vs `capabilities` are different concepts** — declarative contract (QPU side, what's possible) vs imperative config (Circuit side, values to forward).  Future tooling and prose should not conflate them.  The current `QPU.spec.capabilities` (with `maxShots`) is correctly typed; the planned Tier 2 blocks must not borrow the name.

2. **`optimizationLevel` is at the wrong tier** today — top-level on Circuit, but it's a Qiskit `transpile()` kwarg.  Migration to `transpile.optimization_level` is *deferred*; the field stays where it is until M2 work touches the area, at which point it moves with a deprecation alias.  The principle is honest about the drift.

**Implementation plan.**  Tier 1 already exists.  Tier 2 passthrough blocks ship as part of M1.5 polish or M2 foundation, depending on cadence — strictly additive (new optional fields with `x-kubernetes-preserve-unknown-fields`), no existing Circuit/QPU breaks.  Companion cleanup: three dead fields identified in the same session's audit (`QPU.spec.capabilities.openqasm3`, `QPU.spec.capabilities.dynamicCircuits`, `Circuit.status.resultRef`) to be removed in a small follow-up patch.

### 2026-05-15 (evening) · `mode=schedule` + `ScheduleCircuit` RPC + ASCII timeline renderer

Closes the originally-deferred Phase 4 from the morning's results-output work — the user "really liked the timeline_drawer" reference, and the new `exec time` row needed a way to show the actual per-instruction schedule behind that single µs number.  Pass 1 scope (ASCII summary, K8s-native).

**New invocation surface** — two entry points, one renderer:

```
qcc schedule bell-state.qasm --backend fake-sherbrooke    # build + render
qcc get circuit <name> --schedule                         # re-read existing artifact
```

`qcc schedule` mirrors `qcc draw` exactly (load → create Circuit → watch → render → cleanup unless `--keep`).  `qcc get circuit --schedule` mirrors `--qasm`/`--draw` for the K8s-native re-read.  The shared parallel structure is intentional — the two artifact-mode commands look the same so future commands following the pattern stay readable.

**End-to-end plumbing** (mirrors the existing `ProbeBackend`/`DrawCircuit` patterns; uses scheduled-transpile output, not a separate scheduler):

```
qcc CLI (schedule.go)
  → Circuit CR with mode=schedule + spec.backendSelector
     → controller selectBackend (Move 1, hard-constraint filter)
        → renderSchedule(): Executor.ScheduleCircuit RPC + JSON artifact
           → ConfigMap data["schedule.json"] (status.scheduleRef)
              → renderSchedule(c, *executor.ScheduleResult)
```

Phase machine for `mode=schedule`: `Pending → Selecting → Scheduling → Succeeded`.  Distinct from `Transpiling/Submitting` (run path) and `Rendering` (draw path) so `kubectl get circuit` makes the mode-specific work visible.

**Adapter strategy** — uses Qiskit's `transpile(qc, backend, scheduling_method='asap')` and walks `tqc.op_start_times` plus the backend's `Target` durations.  Each adapter implements one method:

```python
def schedule(self, qasm: str, target) -> CircuitSchedule:
    # AerAdapter path: lookups via backend.target durations + dt
    # IBMAdapter path: same helpers, real backend
```

Generic `AerSimulator` (no `Target`, no durations) returns `SchedulingUnsupported` rather than crashing the RPC.  The CLI surfaces the message directly to the user — the limitation is honest.

**ASCII renderer** — three sections per output (headline + summary + timeline):

```
▸ bell-state-s2gbj · schedule · on fake_sherbrooke · 1.86 µs scheduled envelope

  summary
    duration       8384 dt  (1.86 µs)
    cycle time     dt = 222 ps
    ops            139 total · 14 non-idle
    active qubits  q0, q1
    longest gate   measure on q[0] · 1.22 µs

  timeline
    q0  rz @ 0 ns  ·  sx @ 0 ns (57 ns)  ·  ecr @ 57 ns (533 ns)  ·  …  ·  measure @ 647 ns (1.22 µs)
    q1  rz @ 0 ns  ·  sx @ 0 ns (57 ns)  ·  rz @ 57 ns  ·  ecr @ 57 ns (533 ns)  ·  …
```

Idle `delay` instructions (the dominant op count on a 127-qubit backend) are dropped — they tell the reader nothing past what the summary's `total · non-idle` already showed.  Per-qubit lines that would exceed 12 events collapse to *first 6 · "(N more)" · last 6* so Shor's-on-Brisbane (10,555 non-idle ops across 8 of 127 qubits) still fits a terminal:

```
q123  sx @ 0 ns (60 ns)  ·  rz @ 60 ns  ·  ecr @ 60 ns (660 ns)  ·  …  ·  … (2077 more) …  ·  …  ·  measure @ 1.29 ms (1.30 µs)
```

**Thesis-citable findings** the renderer makes visible:

- **`fake_sherbrooke`'s 1.86 µs Bell envelope** at `dt = 222 ps` — close to but not identical to Ch1's "9466 dt ≈ 1.89 µs" (that was a Deutsch circuit on real Sherbrooke; Bell on the FakeSherbrooke snapshot is shorter).  The Ch1 anchor *"the schedule wall-clock is ~1.89 µs"* can now be cross-referenced against system data.
- **Brisbane's 2.08 µs Bell envelope** at `dt = 500 ps` — different physical sample rate, but the schedule is *shorter in dt cycles* (4160 vs 8384) because Brisbane's per-instruction wall durations dominate at this dt resolution.  The two backends in side-by-side mode visualise the Ch1 *"same Bell, different scheduled envelope"* finding.
- **Shor's-on-Brisbane lands at ~1.30 ms** — the controller's exec-time estimate (`depth × max(1Q_dur, 2Q_dur) = ~4.30 ms`) overshot because it didn't account for op parallelism across qubits.  The schedule is the ground truth and exposes the gap.  Move 5 scoring in M2 can use either, but the schedule is the better estimate where coherence budget matters.

**Pass 2 (matplotlib PNG via `timeline_drawer`) deferred.**  Pass 1 establishes the plumbing; PNG is a base64-into-ConfigMap addition that doesn't change the architecture.  Add it when a thesis figure needs the high-fidelity rendering.

**Files**: `proto/qcc/executor/v1/executor.proto` (rpc + 2 new messages); `qcc-executor/src/qcc_executor/adapters/{base,aer,ibm}.py` (`CircuitSchedule`, `ScheduledOp`, `schedule()` impl, `_processor_identity` helper, `_backend_has_durations`, `_instruction_duration_dt`); `qcc-executor/src/qcc_executor/servicer.py` (`ScheduleCircuit` handler); `api/v1alpha1/circuit_types.go` (`ModeSchedule`, `PhaseScheduling`, `ConditionScheduled`, three new Reason* constants, `ScheduleRef`, `ArtifactSuffixSchedule`, `ArtifactDataKeySchedule`); `internal/executor/client.go` (`ScheduleCircuit` method, `ScheduleResult`, `ScheduledOp`); `internal/controller/circuit_controller.go` (`renderSchedule` handler, phase dispatch, Executor interface extension); `cmd/qcc/commands/schedule.go` (new — entire `qcc schedule` command + ASCII renderer); `cmd/qcc/commands/get.go` (`--schedule` flag, `printScheduleArtifact`, schedule-mode message in `noArtifactMessage`, schedule hint in `printArtifactHints`); `cmd/qcc/commands/root.go` (register `newScheduleCmd`); `internal/controller/circuit_controller_test.go` (extend `fakeExecutor` with `ScheduleCircuit`).

### 2026-05-15 (PM) · `processor_family` probe + sectioned `qcc get qpu` + comparison list view

Closing the same-day loop on the result-output work: the chip-generation labels (Eagle r3 / Heron r2 / Falcon r4) used in the morning's discussion came from author intuition, not system data. Plumbed `backend.processor_type` end-to-end so those labels are real CRD fields.

**New plumbing path** (mirrors the existing `ErrorMedians` / `CoherenceMedians` / `InstructionDurationMedians` pattern):

```
backend.processor_type → BackendMetadata.processor_{family,revision,segment}
  → ProbeBackendResponse fields 16/17/18
  → BackendProfile.Processor{Family,Revision,Segment}
  → QPU.status.processor (new QPUProcessor sub-struct, nil when absent)
```

Two annoyances normalised at the wire boundary: Qiskit reports `revision` as int OR string across families, and Falcon adds an optional `segment` field other families don't have. Proto wire type is `string` for revision; controller never sees the mismatch.

**View overhaul** (`cmd/qcc/commands/get.go`):

1. **`qcc get qpu <name>` restructured** from three stacked `KV` blocks to the same headline+sections shape as `qcc get circuit`: `identity / calibration / gate errors / coherence / timing`. Headline reads `✓ fake-marrakesh · 156q Heron r2 · 352 edges · Available · calibrated 15 mo ago`. New `timing` section shows `dt (control-electronics sample period) · 1Q duration (median) · 2Q duration (median)`. Generic Aer degrades to headline + identity only.

2. **`qcc get qpu` list view replaced** (backwards-incompatible). Old: `NAME · PROVIDER · KIND · QUBITS · AVAILABLE · AGE`. New: `NAME · AVAILABLE · PROCESSOR · QUBITS · 2Q ERR · T1 · DT · CALIBRATED`. PROVIDER/KIND were degenerate across the fake-* set (all `local`/`simulator`); AGE was CR-creation, not calibration freshness — both replaced with axes that actually compare. User explicitly chose default-enriched over `-o wide` flag.

**Findings worth Ch5/Ch7 citations** — visible at-a-glance in the new list view:

- **`fake-kyoto` ships with `2Q ERR = 1.00e+00`** in Qiskit's frozen snapshot. Not a probe bug — a corrupted ECR entry in the FakeKyoto calibration. Good example for the observability chapter: *the observability layer surfaces bad data rather than hiding it*. The em-dash sentinel applies only to missing data; bad data we render as-is.
- **Sherbrooke is the only `dt = 222 ps` backend** in the fake-* set (Brisbane/Osaka/Kyoto all `500 ps`). The Ch1 *"9466 dt ≈ 1.89 µs"* is therefore Sherbrooke-specific — the list view makes this visible without anyone having to remember.
- **Heron's 2Q duration is 10× shorter than Eagle's** (~68 ns vs ~660 ns). Visible side-by-side in the `timing` section of `qcc get qpu fake-marrakesh` vs `fake-brisbane`. That's the CZ-on-Heron vs ECR-on-Eagle architectural advantage, citable from system data.

Re-probe was forced via `kubectl patch qpu <name> --subresource=status --type=json -p='[{"op":"replace","path":"/status/qubits","value":0}]'` per backend. M2's TTL-driven refresh removes the manual step.

The `processor_family` CRD field is now real but Move 1 doesn't read it yet — M2 selection can prefer Heron over Eagle for short-2Q-circuits once the scoring lands.

### 2026-05-15 (PM) · Results-output honesty pass + `dt`/duration plumbing + sectioned circuit layout

Driven by the user's direct catch on the morning's "expected err" framing — *"is the formula of prediction correct? feel kinda delusional about this"*. It wasn't, quite.

**Honesty pass on the formula** — the linear sum `1Q_count × e_1Q + 2Q_count × e_2Q` was labelled "expected err" and read as a probability. It isn't: once the sum exceeds 1 it has no per-shot-rate meaning. Two-part fix in `cmd/qcc/commands/get.go`:

1. **Renamed to `error exposure`** with units *events/shot*. Regime labels rewritten: *within gate-error budget* / *under pressure* / *exceeded* / *severe*.
2. **Added a fidelity-bound row** using `P(no gate error) ≈ exp(-exposure)` (Poisson approximation). That number *is* a probability and is the closest "fidelity" the available inputs can produce. Row reads `fidelity bound  P(no gate error) ≈ <value>  (excludes readout & coherence)` — both the bound nature and the missing terms are visible in the output, not buried.

`errorExposure` doc comment now documents what the metric *is* (regime indicator), *isn't* (probability, fidelity), what's missing (readout, coherence, layout), and intended use (Move 5 fast-reject in M2). Thesis can cite the row without overclaiming. On Shor's-on-Brisbane this now reads `error exposure ≈ 16 events/shot (severe — gate-error budget exceeded)` and `fidelity bound  P(no gate error) ≈ 1.4e-07`, both of which make the uniformly-scattered histogram predictable from metadata *before* the circuit runs.

**`dt` and per-instruction durations end-to-end** — `ProbeBackendResponse` gained `dt_seconds`, `single_qubit_duration_median_seconds`, `two_qubit_duration_median_seconds` (fields 13/14/15). Aer adapter populates from `backend.dt` and `target.target` durations via a new `_median_instruction_duration` helper mirroring the existing `_median_instruction_error`. CRD gained `QPUStatus.DtSeconds` and `QPUStatus.InstructionDurationMedians {SingleQubitSeconds, TwoQubitSeconds}`. CLI uses them to compute `exec time ≈ depth × max(1Q_dur, 2Q_dur)` — Bell on Brisbane lands at `~5.28 µs`, Shor's at `~4.30 ms`. The Sherbrooke `dt = 222 ps` cited in Ch1 is exact in the snapshot.

**Sectioned scientific-paper layout** — `buildCircuitSummary` restructured from flat KV table to one-line headline + four sections (`setup / backend / circuit / results`). Headline produced by `verdictFrom(exposure)` with stable thresholds (*signal preserved* ≤ 0.1, *degraded* ≤ 1, *partially lost* ≤ 5, *expected lost* > 5). Example:

```
✓ shor-7p4jp · run · on fake-brisbane · signal expected lost (error exposure ≈ 16)
```

Quotable in a figure caption. `qcc run` and `qcc get circuit` share the renderer — one source of truth, no surface drift.

**Files**: `proto/qcc/executor/v1/executor.proto`, `qcc-executor/src/qcc_executor/adapters/{base,aer,ibm}.py`, `qcc-executor/src/qcc_executor/servicer.py`, `internal/executor/client.go`, `api/v1alpha1/qpu_types.go`, `internal/controller/qpu_controller.go`, `cmd/qcc/commands/get.go`.

### 2026-05-15 · Result-card unified renderer + Ch1-aligned helpful details

> **Superseded by the 2026-05-15 (PM) entries above** for the *expected err → error exposure* rename, the new fidelity-bound row, and the sectioned layout. The plumbing decisions below (Ch1-anchor rows, unified renderer) still apply; only the labelling and regime-language evolved.

After reading the Introduction chapter end-to-end, three additional details emerged as the highest-value additions to `qcc run` / `qcc get circuit` output — each tied to a specific Ch1 finding:

| Row | Ch1 anchor | What it surfaces |
|---|---|---|
| `coherence  T1 X µs · T2 Y µs` | NISQ constraint; calibration-drift framing | Time budget of the backend |
| `calibrated  YYYY-MM-DD (N mo ago)` | Calibration-drift motivation | Reproducibility timestamp |
| `expected err  ≈ N err/shot  (regime label)` | Brisbane-vs-Sherbrooke counterintuitive finding | One-line failure prediction |

The expected-error row is the headline addition.  It computes `1Q_count × e_1Q + 2Q_count × e_2Q` from the executor-reported transpile metrics and the QPU's probed error medians.  Annotated in four regimes: *within budget* (< 0.1), *noticeable noise* (0.1–1), *approaching budget* (1–5), *severe — signal likely lost* (≥ 5).

**Empirical effect** — same circuit, two backends, two annotations that match the histograms:

```
Bell on fake-brisbane   expected err ≈ 0.01 (within budget)   →  6% off-diagonal
Shor's on fake-brisbane expected err ≈ 16   (signal likely lost) → uniform across 16 outcomes
```

This is the one-line predictive annotation Ch7's evaluation can cite directly.  The same calculation will become the kernel of Move 5 (composite scoring) in M2 — selection refuses matches whose predicted error exceeds budget.

**Plumbing fix**: routed `qcc run` and `qcc get circuit` through a single render function (`buildCircuitSummary` in get.go).  Previously the two surfaces had drifted — `qcc get` showed gate errors, `qcc run` did not.  Consolidating to one path closes the gap and gives one place to add future Ch1-derived metrics (layout fidelity, duration breakdown — M2).

The deleted `buildResultBody` had a `duration` row inside the Card.  Removed because duration is already shown via the spinner's `✓ completed · 2.02s` finish line — redundant in the Card.

### 2026-05-15 · M1.5d + CLI ergonomics overhaul

Bundled five separate improvements that compose into the M1.5 closing pass:

**1. T1/T2 coherence times through the whole stack** — `BackendMetadata` (Python) gained `t1_median_us` / `t2_median_us`; same in `BackendProfile` (Go) via the `ProbeBackend` RPC's new `t1_median_us` / `t2_median_us` fields; `QPU.status.coherenceMedians.{t1Micros,t2Micros}` carries them in the CRD; `qcc get qpu` renders them. Median over qubits, in microseconds (the unit IBM publishes). Generic Aer reports zero (no qubit_properties); fake_brisbane reports ~232 µs / 150 µs; old Belem fakes report ~78 µs / 64 µs — five years of IBM hardware progress visible in `qcc get qpus`.

**2. Transpile metrics on Circuits** — new `Circuit.status.transpile.{depth,twoQubitGates,totalGates}`. The executor was already returning these in `Result`; now we persist them so `qcc get circuit` can render them post-hoc. Critical for explaining the histogram: Shor's on fake-Brisbane transpiles to *1,824 two-qubit gates*; with per-2Q error 7.72e-03, the expected total error is ~14 — which is exactly why the histogram looks uniform. The thesis Ch7 anchor narrative now has the *multiplicand* (gate count) next to the *multiplier* (per-gate error) in one Card.

**3. CLI restructured to kubectl-style `qcc get <kind> [name]`** — backwards-incompatible. `qcc qpu fake-brisbane` is gone; `qcc get qpu fake-brisbane` replaces it. `qcc get qpus` and `qcc get circuits` are new list views with tabular `NAME PROVIDER KIND QUBITS AVAILABLE AGE`-style output. Plural and singular forms are interchangeable per kubectl convention. Rationale: matches the K8s-ecosystem the design state cites; one verb root (`get`) scales as we add `delete`, `list` etc.; reads naturally (`qcc get qpus`). Code-side, `cmd/qcc/commands/qpu.go` deleted; everything consolidated in `get.go` with kind-dispatching.

**4. Cobra ergonomics** — `SilenceErrors: false` so unknown flags surface as `✗ flag error unknown flag: --foo` plus the command's help block (via `SetFlagErrorFunc`). Missing-arg errors fire the same help via `argsWithHelp(cobra.ExactArgs(N))`. Static `ValidArgs` on `get` enables `qcc get <TAB>` to complete to `circuit / circuits / qpu / qpus`. The built-in `qcc completion {bash,zsh,fish,powershell}` continues to emit usable shell scripts (verified: 426 / 212 / 235 lines respectively).

**5. Card framing removed** — `render.Card` (rounded box) replaced by `render.Section` (borderless block). The rounded border ate terminal width, ate visual breath, and (worst) Lipgloss's width math gets confused by box-drawing Unicode in nested Qiskit text drawings — drawings could bisect the frame. Section is "title + blank line + 2-space-indented body" with breath at top and bottom. The old `Card` test was replaced with a `Section` test that asserts the *absence* of box-drawing glyphs so the bug can't regress.

**Also fixed**: `--drawing` → `--draw` (the user's natural typing) and the error path now explains *why* a Circuit might lack a drawing: `"circuit X has no drawing artifact — drawings are only produced by mode=draw, and this circuit's mode is \"run\". Render it with `qcc draw <source>`."` Previously the error was the bare "no drawing artifact" string, leaving users guessing.

**Sample-bundle growth (`config/samples/qpu/`)**:
- `fake-kyoto` (Eagle r3, 127q) — fourth Eagle for thicker calibration-variance comparator set
- `fake-marrakesh` (Heron r2, 156q) — newest IBM architecture, CZ basis, 3.29e-03 2Q error (best of the lot)
- `fake-belem` (Falcon, 5q, 2021 calibration!) — demonstrates qubit-count affinity and shows IBM hardware progress over five years

Bundle now ships 7 fake QPUs across 4 architecture generations.

### 2026-05-14 · M1.5c — Four-QPU sample bundle reveals architecture-vs-calibration interaction

[config/samples/qpu/](../../config/samples/qpu/) now ships `fake-brisbane`, `fake-sherbrooke`, `fake-osaka` (all Eagle r3, 127q, ECR basis) and `fake-torino` (Heron r1, 133q, CZ basis).  After M1.5b's auto-derivation each YAML is three lines (`metadata.name`, `spec.provider`, `spec.kind`); the probe fills everything else.  `kubectl apply -k config/samples/qpu/` registers the bundle; the controller probes each within 5 seconds.

**Same Bell circuit at 4096 shots produced**:

| Backend | Per-2Q gate error | Off-diagonal mass | Lesson |
|---|---|---|---|
| `fake-brisbane` (Eagle r3, Feb 2025) | 7.72e-03 | 6.25% | baseline |
| `fake-sherbrooke` (Eagle r3, Feb 2025) | 7.79e-03 | 3.66% | calibration variance vs Brisbane |
| `fake-osaka` (Eagle r3, **Feb 2024**) | 6.93e-03 | 4.37% | year-older snapshot |
| `fake-torino` (Heron r1, Feb 2025) | **4.19e-03** | **18.65%** | better per-gate error → worse total |

The Torino result is the interesting one and merits a thesis paragraph in Ch7.  Torino has **better per-2Q-gate error** (≈ ½ of Brisbane's) but a **3× worse Bell histogram**.  Cause: Torino uses CZ as the entangling primitive, so a `cx` gate decomposes to `CZ + H-sandwich` — more native gates per logical operation.  Per-gate error is lower; total transpiled depth is higher; net total error is higher.

This is exactly the signal Move 5 (composite scoring) in M2 must reason about: `score ∝ f(per-gate error × per-gate count)`, *not* per-gate error alone.  The bundle gives the thesis the empirical anchor for that argument — reproducibly, on a laptop, with no IBM credentials.

The Eagle r3 trio (Brisbane / Sherbrooke / Osaka) isolates the *calibration* axis: same architecture, same basis, different frozen snapshots → different histograms.  This is the Wilson et al. (2020) 3-304% accuracy variance claim, demonstrated at thesis-laptop scale.

**Bundle layout decisions**:
- Sub-directory `config/samples/qpu/` rather than flat `config/samples/qcc_v1alpha1_qpu_fake-*.yaml` — kustomize-clean, isolatable (`kubectl apply -k config/samples/qpu/`).
- Parent `config/samples/kustomization.yaml` includes the sub-bundle as a kustomize resource, so `kubectl apply -k config/samples/` registers everything.
- Dropped the now-redundant `qcc_v1alpha1_qpu_local.yaml` — `local-aer` ships by default via `config/qpu/`.
- Kept the IBM hardware placeholder (`qcc_v1alpha1_qpu_ibm_sherbrooke.yaml`) since it documents the M2 credential shape.

### 2026-05-14 · M1.5b — Backend introspection (`ProbeBackend` RPC + status enrichment + `qcc qpu`)

After M1.5a the executor *runs* on real-calibration noise but the controller had no way to *see* that calibration — selection had no scoring inputs, `kubectl get qpus` showed user-asserted qubit counts that might or might not match reality, and `qcc get <circuit>` couldn't explain why a histogram looked the way it did.

**The RPC** (`qcc.executor.v1.Executor.ProbeBackend`): read-only introspection of a named backend. Returns `num_qubits`, `basis_gates`, `coupling_edges`, `last_calibration_time`, and population medians for single-qubit / two-qubit / readout errors. Implemented uniformly across adapters via the new `Adapter.inspect()` ABC method — `AerAdapter` reads from the resolved backend's V2 `Target`, `IBMAdapter` does the same against the live IBM backend (M2 path) by reusing the same helpers. The QPUReconciler calls this on registration; M2 will add a TTL refresh per `QCC-System-Design.md` §7.1.

**Source-of-truth shift on the K8s side**: `spec.qubits` is now optional and treated as a *hint* — the QPUReconciler stamps `status.qubits` (along with `status.basisGates`, `status.couplingEdges`, `status.lastCalibrationTime`, `status.errorMedians`) from the probe. `QPU.EffectiveQubits()` is the canonical accessor and prefers status over spec. Printer column `QUBITS` now reads `.status.qubits`. This brings the QPU CRD in line with the standard K8s pattern: spec is desire (optional, often empty); status is observed truth.

**Empirical effect** — `qcc qpu fake-brisbane` now renders:

```
provider     local
backend      fake_brisbane
kind         simulator
available    Available
qubits       127
coupling     144 edges
basis gates  delay, ecr, for_loop, id, if_else, measure, reset, rz, switch_case, sx, x
calibrated   2025-02-26 (15 mo ago)

1Q gate error  1.99e-04
2Q gate error  7.72e-03
readout error  2.00e-02
```

…and `qcc get <bell-circuit>` shows the same gate-error medians inline alongside the histogram, which closes the explanation loop: the 5-6% off-diagonal mass on Bell is *literally that 2.00e-02 readout error plus that 7.72e-03 ECR error compounded across 2 measurements + 1 entangling gate*. Ch7 cites this directly.

**Failure semantics**: probe failures are non-fatal. If `ProbeBackend` returns `AdapterUnavailable` (unknown backend name) or any other error, the QPUReconciler logs the reason and proceeds to mark the QPU `Available` based on its provider — `status.qubits` stays zero, selection falls back to `spec.qubits` (the user hint). This guarantees a flaky probe doesn't lock a QPU out of selection.

**One Makefile change worth noting**: `controller-gen` rejects `float64` in CRDs by default (interop conservatism). Added `crd:allowDangerousTypes=true` so `QPUErrorMedians` can ship as native floats. The thesis-scale Go+Python consumer set makes the interop concern moot; if QCC ever ships to a third-party CRD consumer ecosystem, this becomes worth re-evaluating.

### 2026-05-14 · M1.5a — `fake_*` backend names execute through `FakeProviderForBackendV2`

Before this change, `Circuit.spec.backendSelector.backendName=fake_brisbane` would *select* the `fake-brisbane` QPU (Move 1 of the chain working as designed) but the executor's `AerAdapter` ignored the backend name and ran *every* Circuit on generic noise-free `AerSimulator`. So fake backends were registration-and-display names with no execution semantics — a known M1 limitation.

After: `AerAdapter._resolve_local_backend(backend_name)` returns `FakeProviderForBackendV2().backend(backend_name)` when the name starts with `fake_`, and plain `AerSimulator()` otherwise. The transpile + submit path is already backend-polymorphic, so no other code in the executor needs to change — the fake backend's `Target` carries the real Brisbane coupling map, basis gates, and frozen calibration snapshot, and Aer applies the derived noise model during simulation.

**Empirical effect** measured on the same Bell circuit at 4096 shots:

| Backend | `00` | `01` | `10` | `11` | off-diagonal |
|---|---|---|---|---|---|
| `local-aer` (`AerSimulator()`) | 2043 | 0 | 0 | 2053 | 0% |
| `fake-brisbane` (Brisbane snapshot via Aer) | 1916 | 125 | 132 | 1923 | **6.3%** |

The 6.3% off-diagonal mass on fake-brisbane is the readout + gate error from Brisbane's real calibration, not synthetic noise. This is the empirical claim Ch7 makes about calibration-driven variance — now demonstrable end-to-end from `make deploy`, with no IBM credentials.

**Failure-mode shape**: unknown `fake_*` names raise `AdapterUnavailable` from `AerAdapter.__init__`, the servicer catches and returns `TaskStatus.FAILED` with reason `NoEligibleBackend`. The Go controller's existing `TaskError` dispatch handles this terminally (no retry loop) — important because `QiskitBackendNotFoundError` is not a transient condition.

What this does *not* yet do: probe the backend to fill `QPU.status` with the calibration metadata. That's M1.5b — same execution surface, adds the observability surface so `qcc get qpu fake-brisbane` shows the gate errors that explain the 6.3% above. The CRD shape is unchanged either way; M1.5b is purely additive on `status`.

### 2026-05-14 · Selection-chain responsibility split (Move 1 controller, Moves 2–5 executor)

The five-move accuracy chain (`QCC-Design-State.md` §5) was originally framed as "executes inside QRM on every selection." Cross-referencing `QCC-System-Design.md` §7 during M1 implementation surfaced that this is too coarse: the executor has no Kubernetes API access (no ServiceAccount, no `client.Client`), so it cannot perform Move 1 (`r.List(&QPUList)`). Splitting the chain along the responsibility line in §7's component table:

- **Move 1** — enumerate registered QPUs + apply hard-constraint filter (provider, backendName, kind, minQubits, MaxShots capability, `status.availability == Available`). Controller-side; implemented in M1 via `CircuitReconciler.selectBackend` and the pure-function `filterEligibleQPUs` helper.
- **Moves 2–5** — calibrate (per-`QPU` TTL cache, see `QCC-System-Design.md` §7.1), transpile per backend, evaluate layout (`mapomatic`), compute composite score. Executor-side, M2.

The gRPC contract for M1 stays unchanged: `RunCircuit` takes a single `BackendTarget` derived from the controller's chosen QPU. When M2 lands, the contract extends — either a new `SelectBackend` RPC or an extended `RunCircuit` that takes a candidate list and returns the chosen one with decision evidence. M1's controller-side "first-match" policy is a placeholder for "the executor scored these and picked one" — the data flow is identical, only the scoring intelligence moves.

The `Executor` interface in `internal/controller/circuit_controller.go` now takes a `*qccv1alpha1.QPU` parameter explicitly so the resolution (the chosen QPU) and the intent (`Circuit.spec.BackendSelector`) are not conflated.

`QCC-System-Design.md` §9 was updated to mark which moves run where; `QCC-API.md` §4.1 carries the reference back.

---

## 1. Locked Architecture — Five Components

| # | Component | Role |
|---|---|---|
| 1 | `Circuit` CRD (`qcc.io/v1alpha1`) | Declarative submission unit |
| 2 | `QPU` CRD (`qcc.io/v1alpha1`, cluster-scoped, `kind: hardware \| simulator`) | Declared quantum resource |
| 3 | Controller (Go, kubebuilder v4, controller-runtime) — separate `Deployment` | Reconciles Circuits and QPUs |
| 4 | Executor (`qcc-executor`) — Python gRPC service, **separate `Deployment` + ClusterIP `Service`** | QRMI-shaped adapter ABC; converts, draws, runs circuits; selects QPU |
| 5 | CLI (`qcc`) — Go binary | Resource constructor: reads QASM/Python from disk and submits a `Circuit` to the Kubernetes API. **Does not import Qiskit** — Python→QASM conversion is server-side (Executor `ConvertSource` RPC) |

**Not components** (tooling/deployment concerns): OpenTelemetry instrumentation, Helm chart, Grafana dashboards, Prometheus metrics integration.

## 2. Four Goals (all required)

1. **Accuracy** — composed transpilation, qubit-layout selection, calibration-aware backend selection, freshness-aware queue handling
2. **Observability** — every decision in the accuracy chain visible through OpenTelemetry traces and Prometheus metrics
3. **Easy submission** — CLI accepting Qiskit Python (file/stdin/piped), OpenQASM 3, or YAML; auto-detection; client-side Python→QASM translation
4. **Deployable production shape** — real CNCF operator (Helm, RBAC, leader election, prometheus-operator integration, status conditions, finalisers)

## 3. Requirements (Ch5 §5.8 — locked, but trace to Seelam cross-cuts pending)

- **R1** — Cloud-native deployment patterns
- **R2** — Cross-boundary observability using open standards (OpenTelemetry, W3C Trace Context, Prometheus)
- **R3** — Vendor-neutral orchestration with pluggable backends
- **R4** — Integrated visibility across architectural layers (single trace context across classical-quantum boundary)

**Open question (parked):** Whether R1-R4 explicitly traces to Seelam's QCSC cross-cuts and layers in §5.8. If yes, the architectural anchor was already operational. If no, adding a paragraph that does the trace strengthens Ch5 considerably and makes Ch6 §6.2 read as natural continuation. Revisit after the four critical-reading walks.

## 4. Architectural Positioning

QCC is the **cloud-native fork of QCSC Layer 2 (System Orchestration)** in Seelam et al. (2026), complementing not competing with the Slurm/QRMI/SPANK HPC instantiation. QCC primarily contributes at the **System Management and Monitoring cross-cut**. Phase positioning: Phase 1 (quantum as cloud co-processor) of Seelam's three-phase roadmap.

Canonical positioning sentence:
> "QCC is a deployable cloud-native instantiation of the System Orchestration layer of Seelam et al.'s QCSC reference architecture, with vendor-neutral observability filling the System Management and Monitoring cross-cut that Seelam et al. identify as architecturally necessary but do not specify in implementation."

## 5. The Five-Move Accuracy Chain (executes inside QRM on every selection)

1. **Enumerate** — candidates across registered QPUs, filter by hard constraints (qubits, status). Budget: ~50 ms cached, several seconds fresh.
2. **Calibrate** — pull live calibration per candidate (timestamp captured). Budget: 0.5–2 s per candidate. Per-call deadline: 5 s.
3. **Transpile** — 10× at Qiskit optimisation level 3, pick run with fewest two-qubit gates. Budget: 1–5 s per attempt. Per-attempt deadline: 30 s.
4. **Layout** — `mapomatic.evaluate_layouts` per candidate. Budget: 0.1–1 s. Fallback to SabreLayout on failure with `qcc.layout.fallback=true`.
5. **Score** — composite: fidelity × freshness × queue weight. Budget: ~10 ms.

Total `Select` budget: 5–30 s for typical circuits, dominated by Moves 2 and 3.

## 6. Auto-Selection Mode

When `Circuit.spec.backendSelector` is omitted, the chain runs across all `Ready` hardware QPUs satisfying `qubits >= circuit.qubits`. Picks highest-scoring candidate via single composite score.

**Not Qonductor's NSGA-II Pareto-front search.** Qonductor optimises simultaneously across fidelity, JCT, and utilisation; QCC optimises a single composite. The thesis is honest about this in §K — Qonductor is more sophisticated; QCC's contribution is deployability + observability + integration.

Auto-selection emits dedicated span attributes: `qcc.selection.policy = auto | constrained | pinned`, `qcc.selection.candidates_total`, `qcc.selection.candidates_evaluated`.

## 7. Critical Design Choices (locked)

- **OpenQASM 3 only as wire format** — vendor-neutral, GitOps-friendly, IEEE-bound. QPY rejected (Qiskit-internal binary contradicts vendor neutrality).
- **Python→QASM translation is server-side, not client-side.** *Revised from the v2 design.* The CLI accepts Qiskit Python (`.py`) and OpenQASM 3 (`.qasm`) but does **not** import Qiskit; it sniffs the format by extension and submits a `Circuit` with `spec.source.{format, body}`. The Executor's `ConvertSource` gRPC (called transparently by the controller when `source.format=qiskit`) loads the user's source in an isolated module namespace, finds the top-level `QuantumCircuit` by convention (`circuit`, then any module-scope `QuantumCircuit`), and emits OpenQASM 3 via `qasm3.dumps` after a defensive `.decompose(reps=5)` (required because Qiskit's QASM 3 exporter rejects high-level library instructions like `QFT`). Rationale: the CLI ships as a Go binary with no Python runtime dependency; users without a Qiskit install can still submit Qiskit sources; the conversion happens in the same Python environment that will execute the circuit, eliminating version-skew bugs.
- **Controller and Executor run as separate `Deployment`s, bridged by a ClusterIP `Service`.** *Revised from the v2 design's single-Pod sidecar topology.* The Executor's Qiskit/Aer image is ≈ 1 GB and needs multi-worker concurrency for parallel circuits; the Controller is a small Go binary that should scale on reconciliation pressure, not vendor-SDK weight. Separate Deployments let each scale independently, and the controller→executor wire becomes a clean network boundary (`QCC_EXECUTOR_ADDR=qcc-executor.<ns>.svc:9000`) instead of localhost-loopback in a co-located pair. Helm value for future toggling is no longer needed — the topology *is* the design.
- **Streaming `WatchJob` RPC** replaces polling — QRM owns cadence, supports future push-based vendors. *(Async path `SubmitTask`/`WatchTask`/`FetchTaskResult` is defined in the proto for M2 with QRMI; M1 uses synchronous `RunCircuit`.)*
- **W3C Trace Context** auto-propagated via otelgrpc; stamped into IBM Runtime `runtime_options.tags` best-effort. *(Pending — OTel instrumentation not yet wired.)*
- **Per-call calibration freshness** (no caching for selection use). Justified by Wilson et al. (2020) 3-304% accuracy variance and Murali et al. (2019) 18× success-probability swing.
- **CRI/QRMI lineage framing** — Go interface shaped close to Seelam's QRMI method set; thesis ships one adapter (Qiskit/IBM); standardisation deferred to community.
- **Out-of-band artifact ConfigMaps** — `Circuit.status.drawingRef` and `Circuit.status.convertedRef` point at sibling `ConfigMap`s rather than carrying payloads inline (etcd value bound: ≤ 256 KiB practical, 1.5 MiB hard). Owned via `controllerutil.SetControllerReference` so GC reaps them with the parent Circuit. See `QCC-API.md` §3.7.

## 7a. Composition Principle (locked)

QCC composes over upstream toolchains (Kubernetes, Qiskit, OpenTelemetry, Helm, QRMI). Each upstream is wrapped, not re-implemented. The CRD spec follows a two-tier shape that captures this discipline explicitly.

**Tier 1 — Vocabulary.** Typed first-class fields that name QCC's own cross-cutting concepts. Small set, slow-moving. CLI flags map here 1:1.

| Resource | Tier 1 fields |
|---|---|
| `Circuit.spec` | `mode`, `source`, `shots`, `backendSelector`, `timeoutSeconds`, `optimizationLevel` (see *promotion path* below — this one is drifting) |
| `QPU.spec` | `provider`, `backendName`, `kind`, `region`, `qubits` (status takes precedence), `capabilities` (contract subset) |

**Tier 2 — Per-stage passthrough.** Blocks named after the upstream pipeline stage, containing options forwarded *verbatim* to the corresponding upstream function. Each block uses `x-kubernetes-preserve-unknown-fields: true` so options the CRD doesn't model pass through opaquely without schema bumps.

| Block | Forwards to | Scope |
|---|---|---|
| `Circuit.spec.transpile` | `qiskit.compiler.transpile(qc, backend, **this)` | operation-scoped (works on any provider) |
| `Circuit.spec.execute` | resolved-provider's run method — `backend.run(**this)` for Aer/qiskit-providers, `sampler.run(**this)` for Runtime | operation-scoped (best-effort: keys the resolved provider's `run()` accepts apply; unknown keys fail at runtime) |

**Provider construction is *not* in the CRD.** The Tier 2 table above contains *only operation-scoped* blocks — things forwarded to verbs (transpile / run) that have a per-Circuit invocation. Provider-specific *construction* config (e.g. `AerSimulator(method='statevector', precision='double')` or `QiskitRuntimeService(channel='ibm_quantum')`) is **not exposed as a YAML block on either CRD**. Instead, construction is encoded by:

- `QPU.spec.provider` — dispatches to the adapter (`local` → `AerAdapter`, `ibm` → `QiskitRuntimeAdapter`, …)
- `QPU.spec.backendName` — the adapter's internal resolver maps the name to a constructed Backend (e.g. `aer_statevector` → `AerSimulator(method='statevector')`; `fake_brisbane` → `FakeProviderForBackendV2().backend('fake_brisbane')`; `ibm_brisbane` → `QiskitRuntimeService().backend('ibm_brisbane')`)

The reason: provider construction is set *once per QPU registration*, not *per Circuit run*. Multiple Circuits run on the same QPU instance; they shouldn't each re-configure how the backend was built. Different constructions = different QPU CRs (e.g. `aer-statevector` and `aer-density-matrix` are two distinct QPU CRs that the AerAdapter resolves to two distinct `AerSimulator(method=...)` instances).

This keeps the CRDs minimal and the principle clean: **CRD describes orchestration (what to run, where, with which constraints); adapter encodes provider-specific construction (how this QPU's backend object is built).**

**Casing rule.** Tier 1 typed CRD fields are camelCase at the K8s/CRD boundary (K8s-idiomatic): `backendSelector`, `optimizationLevel`, `timeoutSeconds`. Tier 2 passthrough block contents are **snake_case** to match Qiskit's parameter names directly — `seed_transpiler`, `layout_method`, `routing_method`, `seed_simulator`, `memory`. The adapter forwards the dict to Python as `**kwargs` *without translation*; there is no translation layer to maintain, and that is precisely the promise of passthrough. The two conventions don't collide: Tier 1 keys are in the CRD schema (camelCase); Tier 2 keys live inside opaque `transpile` / `execute` blocks (snake_case). Values are verbatim regardless of tier.

**Conflict rule.** Tier 1 vocabulary fields express invariants. Tier 2 passthrough options that contradict the resolved vocabulary are rejected at admission. Concrete: `mode=schedule` requires `scheduling_method=asap`; a `transpile.scheduling_method: alap` on the same Circuit fails admission with a clear error message. **Vocabulary wins.**

**Promotion path.** A Tier 2 option becomes a Tier 1 typed field when it becomes load-bearing for selection, observability, or a thesis claim. Demotion is forbidden — once typed, stays typed (standard K8s API evolution). Today's `optimizationLevel` at top-level is at the *wrong tier* per this principle (it's a Qiskit `transpile()` kwarg, should be `transpile.optimization_level`); migration is deferred until M2 work touches the area.

**CLI policy.** CLI flags expose Tier 1 vocabulary only — orchestration concerns (where to run, how to wait). Algorithm-tuning knobs (Qiskit transpile/run options, Aer construction kwargs) live in YAML, not on the command line. The split is the same one `kubectl run` (ad-hoc) vs `kubectl apply -f` (full spec) makes. Reproducible thesis experiments commit their YAML; ad-hoc runs use the flag-friendly subset.

**`capabilities` vs `options` — disambiguation.** Two surfaces, two purposes, two locations:

| Concept | Location | Direction | Example |
|---|---|---|---|
| `capabilities` | `QPU.spec.capabilities` (declared), `QPU.status.capabilities` (discovered, future) | **declarative** — what's possible | `maxShots: 100000` |
| per-stage `options` blocks | `Circuit.spec.{transpile, execute}` | **imperative** — values to forward | `transpile.optimization_level: 3` |

The K8s precedent reserves "capabilities" for declarative contract (Linux capabilities, supported features). Don't conflate the two — the wrong word implies the wrong shape.

**Substrate-agnostic.** The principle applies to every upstream QCC composes over. Today: Qiskit. M2.5: OpenTelemetry — we adopt OTel semantic conventions, we don't re-invent them. M3: Qiskit provider ecosystem (`qiskit-ibm-runtime` first, optionally `qiskit-braket-provider`) — each provider plugin is a new adapter resolver branch + a new pip dependency, no CRD changes. M4: Helm — chart values follow Helm conventions. Ch9 future-work: QRMI as alternative substrate, CUDA-Q as alternative SDK loader (each adds at one well-defined seam). At every layer: own the orchestration vocabulary, forward the rest.

**Empirical anchor.** `examples/upstream-test.py` demonstrates substrate-substitution end-to-end. A Qiskit script hard-coded to `FakeJakartaV2`, building its own `generate_preset_pass_manager` with `ALAPScheduleAnalysis` + `PadDelay` and a teleportation circuit with dynamic-circuit `if_test` constructs, runs through QCC routed to `fake-brisbane` **without modification**. The script's backend and pass-manager work is discarded at the QCC composition boundary; the controller re-transpiles for the resolved QPU. Same script, backend switched imperatively via `BackendSelector` — the substrate-substitution argument from Ch5 §5.8 (R2) made concrete in working code, before the principle was even stated. This is the Ch7-citable property.

**What this principle is not.** Not a wrapper architecture. Not a translation layer. The CRD shape *inherits* Qiskit's shape via passthrough blocks, so QCC's API surface stays small even as Qiskit grows. When `qiskit.compiler.transpile()` gains its 31st parameter in a future release, users can use it the day the new Qiskit version ships in the executor pod — no CRD bump, no QCC code change.

**Implementation status.** **Tier 1 *and* Tier 2 are shipped as of 2026-05-16 (night).** Tier 1 typed CRD fields exist for `shots`, `optimizationLevel`, `timeoutSeconds`, `backendSelector`, `mode`. Tier 2 `transpile` and `execute` opaque blocks ship on `Circuit.spec` as `apiextensionsv1.JSON` with `x-kubernetes-preserve-unknown-fields: true`; the proto carrier is `google.protobuf.Struct`; the executor decodes via `json_format.MessageToDict` (with whole-number-float → int coercion at the wire boundary, because protobuf Struct's `NumberValue` is double-only and Qiskit signatures are strict); the adapter forwards verbatim as `**kwargs` to `qiskit.compiler.transpile()` / `AerSimulator.run()` / `SamplerV2.run()`. Tier-1 keys (`shots`) are silently stripped from Tier-2 blocks with a logged warning — Tier-1 wins, that's the contract. Migrations of drifting fields (`optimizationLevel` → `transpile.optimization_level`) happen incrementally, never as a one-shot refactor. Provider construction stays in the adapter resolver (not a CRD block) — adding a new QPU variant like `aer-statevector` is a new QPU CR + a new branch in the AerAdapter's `_resolve_local_backend`, no schema change.

## 7d. QEI — Adapter Pattern Formalized as a Public Interface (deferred · Ch9 future-work)

The Composition Principle (§7a) names the adapter as the seam where vendor and SDK knowledge are absorbed.  Today, the adapter is an internal Python class hierarchy inside the `qcc-executor` Deployment.  The natural architectural progression is to **promote the adapter from an internal pattern to a public interface** — modelled directly on Kubernetes' established pluggable-subsystem precedents (CRI, CNI, CSI, Device Plugin API).

This section locks the architectural intent.  Implementation is deferred to a post-thesis milestone for reasons documented below.  The future-work direction is named so Ch9 can cite it.

### K8s precedent

Kubernetes itself follows a consistent pattern for pluggable substrates:

| Interface | Spec | Plugin shape | Discovery |
|---|---|---|---|
| **CRI** (Container Runtime Interface) | gRPC proto | separate process (containerd, CRI-O, gVisor) | unix socket path in kubelet config |
| **CNI** (Container Network Interface) | JSON over stdio | binaries in `/opt/cni/bin/` | NetworkAttachmentDefinition CR |
| **CSI** (Container Storage Interface) | gRPC proto | sidecar Deployments + DaemonSets | StorageClass + CSIDriver CR |
| **Device Plugin API** | gRPC proto | DaemonSet per device type | kubelet unix-socket discovery |

Common shape across all four:

1. **Platform owns the interface specification**; not the implementations
2. **Plugin = separate process / image**; not statically linked into the platform
3. **Configured via K8s resources** (CR or annotations)
4. **Multiple implementations possible**; choice belongs to the operator
5. **Platform doesn't ship plugins** — kubelet doesn't bundle containerd

The platform owns the interface; anyone can ship a conformant plugin.

### Today's QCC architecture vs the CRI pattern

| | K8s + CRI | QCC today |
|---|---|---|
| Interface | CRI proto, owned by K8s, public | `Adapter` ABC in Python, owned by QCC, **inside the executor image** |
| Plugins | containerd, CRI-O, gVisor (separate processes) | AerAdapter (in-tree, soon QiskitProviderAdapter in-tree) |
| Plugin shape | separate process via gRPC | Python class inside `qcc-executor` Deployment |
| Multiplicity | choose 1 per node | one executor Deployment, all adapters in it |
| Discovery | unix socket path | hardcoded `QCC_EXECUTOR_ADDR` |
| Third-party plugins | yes, common | no — all adapters are in-tree |

Today's QCC has an internal abstract base class.  The K8s pattern would make the same seam a public gRPC interface implemented by independent Deployments.

### What QEI would be

A formal gRPC interface, owned by QCC, implemented by plugins.  The starting point is already in the repo — `proto/qcc/executor/v1/executor.proto` defines the operations (Transpile / Submit / Poll / FetchResult / Inspect / DrawCircuit / ScheduleCircuit).  QEI is that proto promoted from "internal QCC convention" to "public interface other tools can implement".

QPU CRDs would declare which QEI plugin handles them via a new `spec.executor.serviceRef`:

```yaml
apiVersion: qcc.io/v1alpha1
kind: QPU
spec:
  provider: qiskit-ibm
  backendName: ibm_brisbane
  executor:                      # NEW; absent today
    serviceRef:
      name: qei-qiskit-ibm
      namespace: qcc-system
      port: 9000
```

Each plugin would ship as a separate Deployment with its own image and dependency tree:

| Plugin | Image | Reaches |
|---|---|---|
| `qei-aer` | (today's `qcc-executor` rebadged) | local sim + fake_* |
| `qei-qiskit-ibm` | qiskit-ibm-runtime | IBM hardware |
| `qei-qiskit-braket` | qiskit-braket-provider | IonQ + Rigetti + IQM + AQT + QuEra (via Braket aggregator) |
| `qei-cudaq` (further future) | CUDA-Q runtime | CUDA-Q reachable vendors |
| `qei-qrmi` (further future) | QRMI binary (potentially Rust) | QRMI-supported vendors |

Any organization (IBM, NVIDIA, AWS, a university lab, a single contributor) could ship a QEI-conformant plugin without modifying QCC core.

### Why this is deferred (and not implemented this thesis cycle)

In order of significance:

1. **The Composition Principle (§7a) already captures the architectural claim.**  The adapter is named as the seam where churn is absorbed.  QEI is the formalization of that pattern, not a structurally different property.

2. **R2/R3/R4 evidence comes from M2/M2.5/M3 implementation**, not from QEI promotion.  Thesis time on selection scoring (R3), observability (R2), and real-hardware demonstration (R4) directly satisfies the Ch5 §5.8 requirements; QEI does not.

3. **Demonstrating QEI requires at least two plugins.**  A single plugin (`qei-aer`) doesn't make the interface plausible.  The convincing demonstration arrives naturally with M3 when `qei-qiskit-ibm` would land.

4. **The migration moment is M3, not a separate refactor.**  When M3 adds qiskit-ibm-runtime support, the executor topology already needs work; QEI formalization can land then with minimal additional cost — no throw-away work.

5. **Today's adapter pattern is operationally equivalent to a single-plugin QEI deployment.**  The controller→executor gRPC contract already lives in `proto/qcc/executor/v1/executor.proto`.  Renaming "executor" to "qei-aer" + publishing the proto as a public spec is the migration path; nothing about the running system changes.

### Migration plan (when implementation lands, post-thesis)

| Phase | What changes | Effort |
|---|---|---|
| **Today** (this entry) | Lock QEI design intent here.  No code change. | doc only |
| **At M3 / first non-Aer plugin** | Add `spec.executor.serviceRef` field to QPU CRD. Refactor controller to look up executor per QPU (executor pool with caching, ~80 LOC at one location).  Rebadge `qcc-executor` → `qei-aer`.  Add `qei-qiskit-ibm` plugin Deployment.  Publish `proto/qcc/executor/v1/executor.proto` as the QEI v1alpha1 spec. | 1–2 sessions of code |
| **Subsequently** | Add more plugins (`qei-qiskit-braket`, future `qei-cudaq`) as needed.  Each is purely additive — no controller change. | per plugin |

### Blast radius (when migration eventually happens)

| Surface | Change |
|---|---|
| Circuit CRD | **zero** — workload doesn't care which executor processes it |
| QPU CRD | +1 optional field (`spec.executor.serviceRef`); strictly additive |
| Controller (Go) | replace single executor client with per-QPU pool/cache (~80 LOC, one location) |
| Executor (Python) | code unchanged; just rebadged as `qei-aer` plugin |
| Helm/kustomize | split into core chart + per-plugin charts |
| User-facing behavior | unchanged — `qcc run`, `qcc get`, etc. work the same |
| Conceptual model | unchanged — Circuit / QPU / capabilities all stable |

The conceptual model stays.  The CRDs stay almost entirely stable.  The seam gets a name and a public spec.  That's the entire change.

### Thesis Ch9 future-work positioning

Ch9's future-work paragraph (to be written) will say something close to:

> *"QCC's executor architecture follows the Adapter pattern.  The natural formalization is the Quantum Executor Interface (QEI), modelled on Kubernetes' established pluggable-subsystem precedents (CRI, CNI, CSI).  Under QEI, the controller dials QEI-conformant plugin Deployments via gRPC, with each plugin shipping a specific provider/SDK combination as a separate Kubernetes workload.  This promotes the adapter from an internal pattern to a public interface, enabling third-party plugins (vendor-direct, NVIDIA, AWS, multi-substrate) without modifying QCC core.  Implementation is deferred to a post-thesis milestone; the controller↔executor gRPC contract defined in `proto/qcc/executor/v1/executor.proto` is the natural starting point for QEI v1alpha1."*

This is a citation-worthy future direction.  It positions QCC as defining a standard the quantum-platform field could converge on, not merely shipping one orchestrator implementation.

## 8. CLI Surface

The originally-locked surface (`submit, run, list, describe, delete, lint, visualize, version`) has been revised during implementation. Two commands were renamed for K8s-ecosystem familiarity, and four remain pending. The shipped surface today:

```
qcc run    <input> [flags]   # ✓ sync: submit + wait + describe; accepts .qasm or .py
qcc draw   <input> [flags]   # ✓ render ASCII via Executor.DrawCircuit, --keep to retain
qcc get    <name>  [flags]   # ✓ show Circuit + inline drawing; --qasm/--drawing for raw
qcc version                  # ✓
```

**Renames from the v2 design:**
- `describe` → `get` (matches `kubectl get foo bar` precedent; argo/helm/etc. use same)
- `visualize` → `draw` (verb-style mode name matches `Circuit.spec.mode=draw`)

**Pending verbs** (deferred; `kubectl` covers most of these for now):
- `submit` — async path; arrives with `SubmitTask`/`WatchTask`/`FetchTaskResult` (M2 with QRMI)
- `list` — `kubectl get circuits` works; native CLI version is nice-to-have, not load-bearing
- `delete` — `kubectl delete circuit foo` works; cancellation finaliser is the interesting case
- `lint` — static validation + statevector simulation; valuable for thesis demos, not blocking

**Deliberately omitted:** `trace`, `logs` — LGTM stack owns those surfaces; trace ID in `qcc get` is the correlation key.

## 9. Out of Scope (named explicitly)

- Multi-tenancy
- Multiple adapter implementations (interface for many, one ships)
- Error mitigation, circuit cutting, qubit reuse (application-layer concerns)
- Near-time HPC interconnects (Phase 2/3 territory)
- Hardware-level multi-programming (HyperQ-style; below cloud API)
- Pulse-level engineering, quantum error correction
- Security as a contribution (inherited only)
- OTEP submission of `qcc.*` schema (offered as candidate, not claimed)

## 10. Deliverables

*Superseded by the Roadmap and Implementation Status section at the top of this document.*

## 11. Synthesis from Critical Readings (Ioannis's framing — three of four papers)

**QCSC (Seelam et al. 2026):** the reference architecture; everything else maps relative to it. Not a comparator — the *map*. QCC follows QCSC semantics, instantiates Layer 2 as cloud-native fork.

**Qonductor (Giortamis et al. SC '25):** the first credible K8s-adjacent integration for classical-quantum, focused on circuits, optimization, scheduling. Sophisticated NSGA-II Pareto-front. Research prototype, not deployable, no observability. QCC is honest about not competing on scheduling sophistication; positioning is "Qonductor's selection logic ideas, made deployable, observable, integrated."

**Qubernetes (Stirbu et al. 2024):** SRE/DevOps mindset for quantum. Closest to engineering vocabulary. Articulates K8s-native vision and proposes `QuantumJob` CRD pattern but does not deliver the operator behind it. QCC delivers what Qubernetes pointed at. CRD lineage is conceptual, not technical.

**Kanazawa et al. (2025):** sets the ground for metrics and telemetry pipeline; introduces system-centric vs domain-centric telemetry distinction. Their Prefect+Superset stack is illustrative; LGTM stack fits better as the substrate. QCC adopts the architectural principle, substitutes the substrate. The `qcc.*` schema in §M of v2 is the implementation surface.

**Convergence claim (Ioannis's words):** "All papers are validating our direction." Worth testing during the walks by also surfacing where each paper would *push back* on QCC's design — every strong thesis has at least one paragraph that says "this paper would object, here is why, here is how we respond."

## 12. Two Parked Open Questions

**Open Question 1.** Whether to revise Ch1's motivation paragraph now to foreground the QCSC framework, or whether better done after v2 is fully locked into Ch6 first. Lean: lock Ch6 first because Ch1 should reflect what Ch6 actually argues. Revisit after section-three integration.

**Open Question 2.** Whether Ch5 §5.8 requirements derivation explicitly traces R1-R4 back to Seelam's cross-cuts and layers. If yes, the architectural anchor was already operational in the thinking. If no, adding the trace paragraph strengthens Ch5 and makes Ch6 §6.2 a natural continuation. Revisit after section-three; by then the requirements may have evolved beyond R1-R4.

## 13. Working Method for Critical-Reading Walks (agreed)

**Per-paper critical-reading file** in the repo, structured by paragraph-marks. Format:

```markdown
## Page X, §Y, paragraph Z — [short title]
QUOTE: "[short verbatim if useful]"
ROLE: Ch[N] [section] / [v2 design section]
WHY: [why this paragraph is load-bearing]
RELATED: [cross-references to other papers' marks]
GAP / OBJECTION: [if applicable]
```

**Walk cadence:** one paper per conversation. ~60-90 minutes per walk. At least a day between walks for consolidation. Walks should challenge as well as validate — surface places where the paper pushes back on QCC.

**Reading order:**
1. Seelam — QCSC reference architecture (framework anchor; walk first)
2. Qubernetes — K8s-CRD lineage (positions controller-pattern contribution)
3. Qonductor — accuracy-chain comparator (sharpens auto-selection claims)
4. Kanazawa — observability comparator (sharpens substrate-substitution argument and §M schema)

**Wilson + Murali:** focused reading later (abstracts plus key results); justify settled decisions, not load-bearing for design.
**HyperQ (Tao OSDI '25):** light-touch Ch5 sentence only; not a comparator. One BibTeX entry.

## 14. Section-Three Integration Plan (after walks)

Per-thesis-section sweep, not per-paper. Pick a section, gather paragraph-marks supporting it from across all four critical-reading files, revise the section to use them precisely.

Order of section sweeps (priority):
1. Ch5 §5.8 requirements derivation → traces to QCSC cross-cuts (Open Question 2)
2. Ch6 §6.2 QCSC mapping section
3. §M metric schema refinement
4. Ch1 §1.5 motivation paragraph (Open Question 1)
5. Ch9 future work structured around Seelam's three phases

Outcome: v3 design document, revised Ch1/Ch5/Ch6/Ch9 prose, settled requirements (possibly evolved beyond R1-R4).

## 15. SRE-Discipline References (deferred to section three)

To be cited at specific points in the thesis, not driving design decisions:

- Brendan Gregg, *Systems Performance* — USE method, latency analysis, OS-primitives-up methodology
- Google SRE Book + SRE Workbook — SLOs, error budgets, postmortem culture, toil/eng split
- OpenTelemetry semantic conventions documentation — standards basis for the `qcc.*` namespace
- Kubebuilder Book — operator-pattern technical grounding
- controller-runtime API documentation — workqueue, leader election, reconcile semantics

These ground the "SRE working in a quantum company" voice that is the thesis's writing position.

## 16. Bibliography Status

- v2 design has BibTeX entries for: Stirbu, Giortamis (Qonductor + QOS), Tao, Kanazawa, Mantha, Seelam, qrmi2025, wennersteen2025qrmi, Murali, Wilson, mapomatic, Kandala (VQE), openqasm3, kubebuilder, controllerruntime, otelsemconv, w3ctracecontext, prometheusoperator, cri, csi, qiskitruntime
- Pending: forward-citation sweep on Seelam/Kanazawa/Nguyen, venue sweep OSDI/SOSP/ATC/EuroSys/ISCA/ASPLOS/MICRO/IEEE QCE/SC 2024+, arXiv recency Jan 2026+

## 17. Files in Project / Repo

- `Interface_between_Quantum_and_Classical_Computers_-_Workplan.pdf` — original workplan
- `Interface_between_Quantum_and_Classical_Computers_-_Literature.pdf` — initial literature survey
- `Introduction_to_Quantum_Computing.pdf` — coursework reference
- `IBM-Robert-S_-Sutor-Dancing-with-Qubits...pdf` — coursework reference
- `Quantum_Computer_Architecture__Compilation__and_Cloud_Computing__MSc_Thesis_Research_Synthesis.md`
- `The_Quantum_Processing_Unit__Architecture__Compilation__and_the_Classical-Quantum_Interface.md`
- `Classical_Computing_Infrastructure__From_Hardware_to_Observability.md`
- `Chapter_5_Bibliographic_Verification__Quantum-Classical_Computing_Interface_References.md`
- `QCC_Systems_Design_v2___Quantum_Circuit_Controller.md` — **v2 design (lock reference)**
- `Ioannis_Savvaidis_Resume_2025.pdf`
- `MSc_Thesis_60663_Outline.pdf`

**To upload to project knowledge for the walks:**
- Seelam et al. 2026 — *Reference Architecture of a Quantum-Centric Supercomputer* (arXiv:2603.10970)
- Stirbu et al. 2024 — *Qubernetes* (arXiv:2408.01436)
- Giortamis et al. SC '25 — *Qonductor*
- Kanazawa et al. 2025 — *Observability Architecture for Quantum-Centric Supercomputing Workflows* (arXiv:2512.05484)

## 18. Voice / Writing Position

**The thesis is written from the position of an SRE working in a quantum company.** Not aspirationally — concretely. The SRE disciplines (hermeticity, velocity, scalability, observability) are the framework that makes QCC defensible as an *engineering* contribution rather than a research prototype. When a passage feels academic and removed, ask: would an SRE working at PsiQuantum or similar actually write this paragraph? If no, revise. If yes, in voice.

The author's 12+ years of K8s/OTel/Go/cloud-native experience is the differentiator. The thesis should not pretend otherwise.

---

*End of design state document.*
*Save to project knowledge before opening fresh conversations for the critical-reading walks.*
