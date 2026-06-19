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
| **`qcc run --performance-test`** — submits the same Circuit across all available simulator QPUs with a shared `qcc.io/experiment` label, prints comparison table, dashboards auto-group via `$algorithm` (`$experiment` cascade is a small dashboard follow-up).  Honest MSc-scope R3 evidence: the platform's empirical-evaluation primitive, exercisable on any user circuit.  Shipped 2026-05-18 morning; verified on Bell + Shor against the 9-simulator catalog — Heron r2 vs Eagle r3 gate-count + outcome-cleanliness deltas visible in the Ch7 figure. | ✅ |
| Move 5 simple scoring (predicted error budget formula) — design described in §5; implementation moves to Ch9 future work alongside the variants below | 🪪 |
| Full Moves 2–4 + composite scoring (parallel calibrate, transpile per candidate, `mapomatic` layout, fidelity × freshness × queue weighting) | 🪪 |
| Fake-twin empirical scoring (per-candidate run on calibration-faithful proxy, Hellinger vs `aer-statevector` ideal, pick best) | 🪪 |
| Per-`QPU` calibration cache (TTL ≈ 60 s) — only matters under load; thesis-scale runs don't stress it | 🪪 |
| OpenTelemetry traces — W3C Trace Context auto-propagation via `otelgrpc`; cross-boundary propagation through gRPC | 🪪 |
| Prometheus metrics — `qcc_*` namespace (8 Circuit + 6 QPU metrics; full inventory in `QCC-Observability.md` §5) | ✅ |
| `qcc_circuit_usage_seconds` (on-QPU billable) + `qcc_circuit_phase_duration_seconds_observed` (persistent gauge) | ✅ |
| `Circuit.status.traceId` populated by OTel (field reserved on schema; writer is OTel-trace work, deferred to Ch9) | 🪪 |
| Grafana dashboards (qpu + circuit, source-controlled YAML, cascading vars, sibling links, blue palette) | ✅ |
| Algorithm-grouping label convention (`qcc.io/algorithm`, `…-version`, `…experiment`, `…run-index`, `…source-sha256`) + controller auto-fill + metric propagation | ✅ |
| Cross-boundary identifier linkage (forward UID stamp into IBM job_tags + reverse `provider_job_id` as metric label) | ✅ |
| `honorLabels: true` on the ServiceMonitor so `namespace` reflects Circuit's namespace, not Collector's | ✅ |
| `qcc.*` semantic conventions documented (L0–L4 Kanazawa pyramid mapped, full schema in `QCC-Observability.md` §4.2 + §5).  Schema design IS the thesis contribution. | ✅ |
| OTEP submission of the `qcc.*` schema to the OpenTelemetry community (formal RFC process at `open-telemetry/oteps`).  Months of community engagement; explicit Ch9 future-work item. | 🪪 |

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
| Multi-register `_extract_counts` (Teleport's `crz`/`crx`/`result`); today returns first register only.  **Path D+ decision** (2026-05-17 evening, second pass): dropped from thesis scope — the current example set (Bell, Deutsch, GHZ, Shor N=15) is single-register; VQE doesn't need multi-register either (multiple Pauli-basis circuits each have one register).  Becomes Ch9 future work alongside Teleport demos. | 🪪 |
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
| M2 — selection chain + observability | 8 | 8 | **100%** | Observability fully ✅ + `qcc run --performance-test` shipped 2026-05-18 morning.  Move 5 scoring + full Moves 2–4 + composite formula + fake-twin scoring + cal cache + OTel traces + OTEP submission of the `qcc.*` schema all → 🪪 (collected under Ch9 "selection-chain extensions" and Ch9 OTel-ecosystem-engagement items). |
| M2.5 — Composition Principle + outcome quality | 6 | 6 | **100%** | Tier 1+2 ✅.  Multi-register `_extract_counts` + Hellinger/TVD/outcome-CLI all → 🪪 (current example set is single-register; VQE wouldn't need it either) |
| M3 — real-hardware path | 11 | 11 | **100%** | Async path + IBM Heron r2 verified.  `QiskitProviderAdapter` generic + VQE worked example + dedicated queue-position field all → 🪪 |
| M4 — packaging + polish | — | — | **🪪** | Entire milestone post-thesis |
| **Total (thesis-critical, Path D+)** | **47** | **47** | **100%** | |

**Thesis-critical code is complete.**  The only outstanding work for the thesis is **writing Ch6 / Ch7 / Ch8 / Ch9** (~2–3 weeks).

#### Verified 2026-05-18 morning — `qcc run --performance-test` end-to-end

- Bell ladder across 9 simulator QPUs: ideal `aer-statevector` 50/50 (528:496); Heron r2 fakes clean; `fake-kyoto` dramatically noisy (282/250 + 543 off-diagonal) — that's the cross-substrate quality signal visible at a glance.
- Shor N=15 ladder same catalog: Heron r2 routes with **~28% fewer total gates** (8202 vs 10504 on Eagle r3) and produces **~27% taller top outcomes** (95 vs 75) — closes the Ch1 motivation circle empirically.
- Validation: `--performance-test` correctly rejects `--backend` / `--provider` / `--detach` / `--select-only` combinations.
- Falcon `fake-belem` correctly returns `TranspilationFailed` (5 qubits < Shor's required) — selection-eligibility guardrails working.

#### Dashboard `$experiment` cascade — shipped same session

The `--performance-test` CLI surfaces a Grafana deep link `…?var-algorithm=…&var-experiment=…`.  Added the `$experiment` template variable to `qcc-circuit-dashboard.yaml` (cascades: `algorithm → experiment → circuit`, all multi-select with "All" default).  The deep link now fully resolves to a filtered view; users can also flip between perf-test runs from the dropdowns directly.

When refreshing this table after a slice: update the per-milestone "Done" column, recompute the total, and add a one-line entry to the decision log noting *what slice moved which counter*.  Don't change the denominators without a separate scope decision documented in the log.

### Explicit non-goals (do not revisit without thesis-scope check)

❌ Multi-tenancy · ❌ multiple adapter implementations beyond the three named · ❌ error mitigation, circuit cutting, qubit reuse · ❌ near-time HPC interconnects (Phase 2/3 territory) · ❌ hardware multi-programming · ❌ pulse-level engineering / QEC · ❌ security as a contribution (inherited only)

**Note**: the OTEP submission of the `qcc.*` schema was previously listed here as a ❌ non-goal but is more honestly a 🪪 *post-thesis* item — the schema design is the thesis contribution and is done; the formal community engagement to submit it as an OpenTelemetry Enhancement Proposal is Ch9 future work, not "never going to happen."  Tracked alongside the other 🪪 items in the M-tables above.

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

Files touched span `go.mod` (OTel SDK + OTLP exporters + semconv v1.40), the new `internal/observability/{config,resource,otel}.go` orchestrator + `metrics/{provider,qpu,circuit,events}.go` collectors + `traces/provider.go` skeleton + `logs/provider.go` placeholder, plus `cmd/qcc-controller/main.go` (slog + Setup wiring), `internal/controller/circuit_controller.go` (defer-based phase-transition recording), the IBM adapter (`circuit_uid` job-tag stamp), and `config/manager/manager.yaml` (OTEL env + downward API).

**What's still ahead in M2**: Grafana dashboards (shipped in the afternoon session below); Layer-2 / QCC-internals observability is its own focused work (controller-runtime built-ins + executor instrumentation + scrape RBAC) — Ch9 polish.

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

### 2026-05-18 (mid-morning) · Doc polish pass — accuracy + shrink + diagram refresh across all four canonical docs

Single focused pass to tighten the four canonical docs (`QCC-System-Design.md`, `QCC-API.md`, `QCC-Observability.md`, `QCC-Design-State.md`) ahead of writing the thesis chapters that consume them.  Four phases, summarised below.

#### Sizes before/after

| Doc | Before | After | Δ |
|---|---:|---:|---:|
| `QCC-System-Design.md` | 480 | 471 | -2% (structural polish, no big cuts) |
| `QCC-API.md` | 547 | 552 | +1% (added missing `Scheduling` phase + `Scheduled` condition) |
| `QCC-Observability.md` | 1030 | 824 | **-20%** (§1 scope table, §3.3 USE-Q+RED-F collapsed, §12 wiring code samples → code pointers, §15 implementation-sizing removed, §14 out-of-scope tightened) |
| `QCC-Design-State.md` | 1250 | 1014 | **-19%** (2026-05-14/15 verbose entries compressed into one table + thesis-citable findings bullets) |
| **Total** | **3307** | **2861** | **-13.5%** (~450 lines removed) |

The proposal targeted -33%; landed at -13.5%.  The shortfall is intentional — aggressive cuts on the locked-architecture spec sections (§1–§18 of Design-State, §6.x of System-Design, §5 inventory of Observability) would have lost trace links the thesis chapters will need.  Where shrinkage was conservative (System-Design, API), the win is structural correctness rather than raw line count.

#### Per-doc summary

**`QCC-System-Design.md`** — §9 backend-selection-model rewritten to reflect Path D+ (Move 1 ✅, Moves 2–5 🪪/Ch9, `qcc run --performance-test` as the shipped R3 evidence primitive); §6.2 pure-Python utilities prose compressed; §6.3 adapter table aligned (`QiskitProviderAdapter` marked 🪪 Ch9 instead of "planned post-M3"); §15 limitations + future-work paragraphs tightened; §17 thesis-safe summary split from one mega-paragraph into three readable paragraphs ("What QCC is / What QCC is not / Positioning").

**`QCC-API.md`** — §3.5 status fields refreshed (`traceId` description corrected — it's reserved-not-populated under Path D+, not "set by controller on first reconcile" as previously claimed); §4.1 QPU resource purpose updated (Moves 2–5 → Ch9, perf-test as shipped R3 evidence); §4.5 QPU status fields — removed stale "M2 will run a TTL refresh" claims; §5.1 phases table + state-machine mermaid + §5.2 conditions table all now include `Scheduling` / `Scheduled` (which had been missing despite `mode=schedule` shipping).

**`QCC-Observability.md`** — top status line trimmed (no more "as of 2026-05-17 (morning)"); §1 scope rewritten from 5 verbose layered bullet-lists to a single layer-status table with ✅/🪪 markers; §2.1 question→signal table footer "Note" rewritten because the per-Circuit vs aggregate split has softened with the new persistent-gauge metric; §3.3 USE-Q+RED-F collapsed from 5 subsections (3.3.1–3.3.5) to 2 (USE-Q + RED-F as compact tables; Match-Quality as a forward-pointer paragraph); §4.7.1 quantum-vs-Prometheus-histogram table compressed; §4.9 disallowed-labels section now correctly notes that `provider_job_id` IS allowed (the §6 reverse-linkage anchor) under the info-metric exception; §6 cross-boundary linkage already restructured around bidirectional flow (forward UID stamp + reverse provider_job_id label) from the prior pass; §10.3 PromQL refreshed; §11 dashboards reality-check done (Bell ladder figures from `--performance-test` verified 2026-05-18 morning); §12 wiring code samples (~150 lines) replaced with pointers at the actual code files + 5-line summaries; §12.7 "Files to add/touch" stale pre-implementation table removed entirely; §15 "Implementation sizing" stale pre-implementation table removed entirely.

**`QCC-Design-State.md`** — top header date refreshed; §5 Five-Move Chain now carries a Path D+ implementation-status disclaimer ("Move 1 ✅; Moves 2–5 🪪 Ch9"); M4 reclassified `⏳→🪪` with rationale paragraph; new `🪪 post-thesis` marker added to the 5-state legend (✅ ◐ ⏳ 🪪 ❌); all 41 status cells in M-tables normalised from `✓` to `✅` for visual consistency with `🪪`; OTEP submission reframed (schema design ✅ as thesis contribution; OTEP submission 🪪 as Ch9 ecosystem-engagement work) per 2026-05-17 conversation; **the 2026-05-14 + 2026-05-15 decision-log entries (~260 lines spanning 8 entries) compressed into a single 2026-05-14→2026-05-15 summary table + thesis-citable findings bullet list** — original detail is recoverable from git history, the summary captures the M1.5 work that's now fully reflected in the Roadmap M1.5 table.

#### Where to pick up

The Landscape view in `QCC-Design-State.md` ("§ Landscape view — % complete (thesis-critical scope, Path D+)") is the single "where am I" reference.  Current standing: **47/47 = 100% thesis-critical**.  Next phase is writing Ch6 / Ch7 / Ch8 / Ch9 — the polished docs are the engineering source-of-truth those chapters distil from.

Suggested chapter-writing order: Ch6 (Architecture, drawing from `QCC-System-Design.md` + `QCC-API.md`) → Ch7 (Implementation + Evaluation, anchored by the Bell + Shor `--performance-test` figures from 2026-05-18 morning) → Ch8 (Discussion, including Path D+ scope-honesty argument) → Ch9 (Future work, the 🪪 bundle organised as "selection-chain extensions" + "ecosystem engagement").

### 2026-05-18 (morning) · `qcc run --performance-test` shipped + verified — thesis-critical code complete

The R3 evidence primitive landed cleanly.  Single focused session (~2h): new file `cmd/qcc/commands/perftest.go` + 4 flag/dispatch lines in `run.go`, pure CLI work, zero proto / controller / executor changes.

**Implementation**:

- `--performance-test` discovers `Available` QPUs via the K8s API, default-filters to `spec.kind=simulator`; `--include-hardware` extends to real-hardware too.
- Auto-generates `qcc.io/experiment=perf-test-YYYYMMDD-HHMMSS` when the user doesn't pass `--experiment`.
- Auto-defaults `qcc.io/algorithm` to the filename basename (lowercase, hyphenated) when the user doesn't pass `--algorithm`, so the Grafana `$algorithm` filter always has something to group by.
- Validation rejects `--performance-test` + `{--backend, --provider, --detach, --select-only}` combinations before any network call.
- One Circuit per candidate submitted sequentially (cheap), per-Circuit goroutines poll to terminal in parallel, completion lines stream to the CLI as runs finish.
- Comparison table at the end: `BACKEND / PHASE / DEPTH / 1Q / 2Q / TOTAL / TOP OUTCOMES / TIME / JOB`.  Failed Circuits render `FAILED · <reason>` instead of empty cells — the comparison stays informative even on partial success.
- Grafana deep link path `?var-algorithm=<X>&var-experiment=<Y>` so the user can jump straight to the Circuit dashboard with the filters applied.

**Verification artifacts** (both came out as one-shot first runs):

- **Bell ladder** (9 candidates, 1024 shots each): aer-statevector ideal 528:496, Heron r2 fakes clean ~17 off-diagonal, `fake-kyoto` outlier with 543 off-diagonal — that's the visible quality ladder.
- **Shor N=15 ladder** (8 succeeded + 1 transpilation-rejected): Heron r2 routes with 8202 total gates vs Eagle r3 10504 (~28% fewer); top outcome ~95 on Heron r2 vs ~75 on Eagle r3 (~27% taller signal); Falcon `fake-belem` correctly rejected.  This is the Ch1 motivation circle closing empirically.

**Scoreboard moves 97% → 99%.**  One nice-to-have dashboard follow-up remains (adding `$experiment` template variable so the perf-test deep link is fully honoured — ~5 min when next at the dashboard YAML).  Thesis-critical code is complete; the next step is **writing Ch6 / Ch7 / Ch8 / Ch9**.

### 2026-05-17 (evening, third pass) · Move 5 → 🪪 / Ch9 future work; `qcc run --performance-test` replaces it as the R3 evidence primitive

After a sharper scope conversation: dropped Move 5 simple scoring (and the fake-twin variant we briefly considered) to 🪪.  The thesis claim is now framed honestly as *"orchestration platform with empirical cross-substrate evaluation"* rather than *"orchestration platform with predictive backend selection."*

**The new R3 evidence primitive** — `qcc run <file> --performance-test`:

- Discovers all simulator-class QPUs registered in the cluster
- Submits the same Circuit (same source body) to each, with a shared auto-generated `qcc.io/experiment=perf-test-<timestamp>` label
- Algorithm label and source-sha256 are stamped per existing auto-fill conventions
- Waits for all to reach terminal
- Prints a comparison table: backend × shape × top outcomes × job-id × wall-time
- Surfaces a Grafana link with the `$algorithm` + `$experiment` filters pre-applied so the Circuit dashboard renders the comparison automatically

**Why this is the better thesis story:**

1. **Empirical, not predictive.**  The thesis stops claiming an analytical scorer and instead surfaces *measurement* — which is honest about what the platform actually does today and what the dashboards already visualise.
2. **Generalises the Bell ladder.**  The Bell-state-on-three-substrates figure that already exists becomes the canonical example of what `--performance-test` produces *for any user circuit*.  M1.5c's 7 fake-* backends (Eagle r3, Heron r1+r2, Falcon) become the substrate ladder.
3. **No new hand-tuned formula to defend.**  The examiner question "why this scoring formula?" disappears.  The thesis claim is "the platform makes substrate comparison observable", which is what was actually built.
4. **Closes the Ch1 motivation loop without overclaiming.**  Ch7 figure becomes "Shor N=15 across the IBM-Heron-r2-faithful fake-* ladder, qualitative histogram degradation visible against the `aer-statevector` ideal."  The thesis doesn't need a number to make the point — the visual already does.
5. **Ch9 gains a coherent extension section.**  Move 5 simple scoring, fake-twin empirical scoring, full Moves 2–4 + mapomatic + composite weighting, and QEI/QRMI integration all read as "extensions of the architecture" rather than scattered loose ends.

**Pure CLI work** — no proto / controller / executor changes.  Existing infrastructure (algorithm-grouping labels, dashboards with `$algorithm`+`$experiment` filters, `qcc_circuit_result_count` etc.) provides everything needed.  ~1–2 days.

**Scoreboard moves from 97% → 97%** (same denominator/numerator — one ⏳ item swapped for another of equivalent weight); but the remaining item is now ~1–2 days instead of ~2 days, and the **post-implementation defensibility is materially stronger**.

**What this commits**: doc-only.  Reframes the last remaining code slice as "empirical evaluation primitive" rather than "predictive selection score."  Ch9 future-work section gains a "Selection-chain extensions" entry collecting Move 5 / mapomatic / fake-twin / QEI / VQE under a single architectural-extensions umbrella.

### 2026-05-17 (evening, second pass) · Multi-register `_extract_counts` → 🪪; scoreboard 95% → 97%

After a focused look at whether multi-register `_extract_counts` was actually load-bearing for the planned Ch7 figures: dropped to 🪪.  The current example set (Bell, Deutsch, GHZ, Shor N=15) is entirely single-register.  VQE — the canonical "would it need multi-register?" question — doesn't either, because VQE's measurement structure is *multiple separate circuits each with one register* (one per Pauli-basis term in the Hamiltonian decomposition), not one circuit with multiple registers.  Multi-register stays Ch9 future work alongside Teleport-style demonstrations.

Remaining thesis-critical work is now **one named item, ~2 days**: Move 5 simple scoring.  Scoreboard moves to **97%**, and after Move 5 ships it will be **99%** — at which point the only remaining thesis work is writing Ch6 / Ch7 / Ch8 / Ch9.

**Verification plan for Move 5** (committed in this entry so it doesn't drift): once the scoring writer lands, the verification slice is to run **Shor N=15 with auto-select** (no `backendName` in `Circuit.spec.backendSelector`) across the IBM Heron r2 catalog (`ibm-fez`, `ibm-kingston`, `ibm-marrakesh`) plus the fake-* sample set, and show the score breakdown on `Circuit.status.selectionSummary`.  This **closes the Ch1 motivation loop** — the `ibm_sherbrooke` Shor-noise diagnostic that started the thesis becomes empirically addressable: "QCC would have steered that workload to substrate X with predicted error budget 0.0Y instead of Z."

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

Design-only pivot to **Prometheus metrics + cross-boundary identifier stamp**, not OpenTelemetry distributed tracing. Implementation cost (~8–11h, two languages, multi-reconcile trace propagation) was disproportionate to thesis value: the trace would mostly show what we already know (IBM queue wait dominates by orders of magnitude). Kanazawa doesn't require OTel — the paper names a 5-layer pyramid; the substrate is open. R4's "single trace context" is satisfied by a Circuit-UID stamp on IBM `runtime_options.tags` (~1h) without SDK weight.

`QCC-Observability.md` became the canonical observability source-of-truth in this session; transitional artifacts (`M2-metrics-design.md`, `M2b-otel-tracing-plan.md`) deleted. R2/R4/R5 in `01-requirements-re-evaluation.md` reworded to drop preordained OTel framing. Kanazawa-pyramid mapping: L0 → `qcc_qpu_*`; L1 → controller-runtime built-ins; L2 → `qcc_circuits_total`; L3 → `Circuit.status` (substrate substitution for Kanazawa's Prefect metadata); L4 → outcome-quality (M2.5). Kubernetes Events remain future work.

Implementation followed in the 2026-05-17 entries below.

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

**What's still ahead**: VQE H₂ as a worked example exercising `seed_simulator` reproducibility (later moved to 🪪 Ch9 by the Path D+ decision); multi-register `_extract_counts` (also 🪪 Ch9). Tier 2 unblocks reproducible cross-substrate comparisons — the ideal/fake/real ladder produces identical counts on each rerun when seeds are set.

### 2026-05-16 (late+++) · `QCC-System-Design.md` aligned with post-yesterday direction

Doc-sync pass: `QCC-System-Design.md` (canonical engineering source-of-truth) was stale relative to the post-QRMI decisions captured below. Rewrote §6 architecture diagram (vendor edge labels), §6.2 async-lifecycle rationale (Qiskit `JobV1` primary, QRMI deferred), §6.3 adapter table (`QiskitRuntimeAdapter` + optional `QiskitProviderAdapter`, no per-vendor rows), §7 component responsibilities (Aer M1 + QiskitRuntime M3), §14 constraints (Qiskit provider ecosystem; QRMI/CUDA-Q → Ch9), §15 limitations (rewritten as "Qiskit provider ecosystem integration" + "Alternative substrates (Ch9)"), §16 thesis chapter mapping (QEI → Ch9 row added), §17 thesis-safe summary positioning sentence rewritten: **QCC as the open-source K8s-native counterpart to managed proprietary quantum clouds (IBM QP, AWS Braket, Azure Quantum), sharing Qiskit's provider abstraction**; HPC counterpart = SPANK+QRMI.

Doc-only, zero code change. Subsequent 2026-05-17 (afternoon) polish pass updated `QCC-API.md` and `QCC-Observability.md` to match.

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

**What's still ahead in M3**: VQE H₂ worked example, multi-register `_extract_counts` (Teleport-style). Both later moved to 🪪 Ch9 by the Path D+ decision (2026-05-17 evening).

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

### 2026-05-14 → 2026-05-15 · M1 + M1.5 sub-milestones (compressed)

The M1 / M1.5 work is fully reflected in the Roadmap table above and the locked-architecture sections below. The detailed per-entry decision logs from 2026-05-14 and 2026-05-15 are compressed here into one-line summaries; the *thesis-citable empirical findings* they surfaced are preserved as bullets at the end.

| Date | Slice | Outcome |
|---|---|---|
| 2026-05-14 | **Selection-chain split** | Move 1 controller-side (operates on K8s `QPU` resources); Moves 2–5 executor-side (Qiskit/SDK access). Documented in `QCC-System-Design.md` §9. Path D+ moved Moves 2–5 to 🪪 Ch9. |
| 2026-05-14 | **M1.5a** — `fake_*` execution | `AerAdapter._resolve_local_backend` routes `fake_*` names through `FakeProviderForBackendV2`; same-circuit ladder produces visibly noisier histograms than generic Aer (6.3% off-diagonal on Bell). |
| 2026-05-14 | **M1.5b** — `ProbeBackend` RPC | Read-only backend introspection (qubits, basis, coupling, calibration timestamp, error medians, T1/T2, dt, durations, `processor_type`). `QPU.status` becomes the source-of-truth; `spec.qubits` becomes a hint. `crd:allowDangerousTypes=true` so error medians can ship as `float64`. |
| 2026-05-14 | **M1.5c** — Sample-QPU bundle | 4 fake-* registered (Brisbane/Sherbrooke/Osaka — Eagle r3; Torino — Heron r1). Auto-derivation makes each YAML three lines. Reveals architecture-vs-calibration interaction (per-gate error vs total transpiled depth). |
| 2026-05-15 | **M1.5d** — CLI kubectl-style + T1/T2 + transpile metrics | `qcc get <kind> [name]` replaces `qcc qpu`; tab-complete + missing-arg help via Cobra ergonomics; `Circuit.status.transpile` persists post-hoc; `qcc get qpu` renders T1/T2 coherence times; sample bundle grows to 7 QPUs across Eagle r3 / Heron r1+r2 / Falcon. |
| 2026-05-15 | **Result-card honesty pass** | `expected err` renamed → `error exposure` (events/shot, not probability). Added `fidelity bound = exp(-exposure)` row. Sectioned scientific-paper layout (`setup / backend / circuit / results` + headline verdict). `dt` and per-instruction durations end-to-end. |
| 2026-05-15 | **`processor_family` probe** | Chip-generation labels become CRD fields (`status.processor.{family,revision,segment}`). New sectioned `qcc get qpu` view with `timing` section showing `dt · 1Q duration · 2Q duration`. List view replaces PROVIDER/KIND/AGE columns with `AVAILABLE / PROCESSOR / QUBITS / 2Q ERR / T1 / DT / CALIBRATED` (axes that actually compare). |
| 2026-05-15 (evening) | **`mode=schedule` + `ScheduleCircuit` RPC** | Per-instruction dt-cycle timeline artifact via `qiskit.compiler.transpile(scheduling_method='asap')`; new ASCII renderer; `qcc schedule <file>` + `qcc get circuit --schedule`; `status.scheduleRef` ConfigMap. Closes the originally-deferred Phase 4 from the result-card pass. |

**Thesis-citable findings from this period:**

- **Calibration variance under stable architecture**: Bell on `fake-brisbane` (Eagle r3, Feb 2025) vs `fake-sherbrooke` (Eagle r3, Feb 2025) vs `fake-osaka` (Eagle r3, Feb 2024) shows 6.25% / 3.66% / 4.37% off-diagonal mass at 4096 shots — same architecture, same basis, different snapshots → different histograms. The Wilson et al. (2020) 3–304% claim, demonstrated at thesis-laptop scale.
- **Architecture-vs-calibration interaction (the Torino paradox)**: Heron r1 `fake-torino` has half the per-2Q-gate error of Brisbane (4.19e-03 vs 7.72e-03) but 3× worse Bell histogram (18.65% vs 6.25%). Cause: CZ basis on Heron decomposes `cx` into `CZ + H-sandwich` → more native gates per logical op. Better per-gate error, worse total error. Move 5 scoring must reason about `f(per-gate error × per-gate count)`, not error alone.
- **`fake-kyoto` ships with corrupted calibration**: 2Q ERR = 1.00 (an unparseable ECR entry in the FakeKyoto snapshot). Useful Ch6 anchor — the observability layer surfaces bad data rather than hiding it; the em-dash sentinel applies only to missing data.
- **Heron's 2Q duration is 10× shorter than Eagle's** (~68 ns vs ~660 ns) — visible side-by-side in `qcc get qpu fake-marrakesh` vs `fake-brisbane`. The CZ-on-Heron vs ECR-on-Eagle architectural advantage, citable from system data.
- **`fake_sherbrooke` is the only `dt = 222 ps` backend** in the catalog (Brisbane/Osaka/Kyoto all `500 ps`). Ch1's "9466 dt ≈ 1.89 µs" anchor is therefore Sherbrooke-specific — visible in the list view.
- **Bell scheduled envelopes** (`mode=schedule`): `fake_sherbrooke` 1.86 µs (8384 dt × 222 ps); `fake_brisbane` 2.08 µs (4160 dt × 500 ps) — same Bell, different scheduled wall-clock. Cross-references Ch1's timing-data anchor against system output.

Detailed prose for any of the above is recoverable from git history (commits in the 2026-05-14 / 2026-05-15 window); the M1.5 table in the Roadmap section above is the canonical "what shipped" reference.


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

## 5. The Five-Move Accuracy Chain (design spec; partial implementation)

1. **Enumerate** — candidates across registered QPUs, filter by hard constraints (qubits, status). Budget: ~50 ms cached, several seconds fresh.
2. **Calibrate** — pull live calibration per candidate (timestamp captured). Budget: 0.5–2 s per candidate. Per-call deadline: 5 s.
3. **Transpile** — 10× at Qiskit optimisation level 3, pick run with fewest two-qubit gates. Budget: 1–5 s per attempt. Per-attempt deadline: 30 s.
4. **Layout** — `mapomatic.evaluate_layouts` per candidate. Budget: 0.1–1 s. Fallback to SabreLayout on failure with `qcc.layout.fallback=true`.
5. **Score** — composite: fidelity × freshness × queue weight. Budget: ~10 ms.

Total `Select` budget: 5–30 s for typical circuits, dominated by Moves 2 and 3.

**Implementation status (Path D+):** Move 1 is shipped (controller-side); Moves 2–5 are 🪪 Ch9 future-work. The shipped R3 evidence is `qcc run --performance-test` (empirical cross-substrate comparison) rather than a predictive scorer — see the 2026-05-17 evening decision-log entries and `QCC-System-Design.md` §9.

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
