# QCC — Evidence Pack and Thesis Handoff

> Captured 2026-05-24 from `quantum-circuit-controller`. **Not cut as a git tag** — the pack is consumed by uploading the PNGs in this directory plus `README.md` and `RUNBOOK.md` directly to Chat Mode (no zip needed).
> Companion repo: `msc-thesis` — this pack is the empirical source for `Chapters/07-Implementation.tex` and the figure source for revisions to `Chapters/05-Literature-Review.tex` (R5 reframe), `Chapters/06-Architecture.tex`, `Chapters/08-Discussion.tex`, and `Chapters/09-Conclusions.tex`.
>
> **Companion file:** [`RUNBOOK.md`](./RUNBOOK.md) — the session run-book in execution order, with the base command preceding each captured figure. Use it as the paste-ready source for the `\begin{listing} … \end{listing}` blocks that should precede each `\includegraphics{...}` in Ch7.

## Purpose

This directory is the **single source of truth for what QCC actually produces on real hardware and across a heterogeneous simulator catalog**, as observed through the system's own CLI, status surface, metrics, and dashboards. Every figure the Ch7 evaluation cites lives here. Every claim in Ch7/Ch8 that depends on empirical behavior should be traceable to a file in this directory.

The pack covers:

- All thirteen registered QPUs (eight `fake_*` simulators, one ideal `aer-statevector`, one user-added `fake-fez`, three live IBM Heron r2 hardware backends)
- One real-hardware Shor N=15 a=4 evaluation campaign on `ibm_kingston` (depth ~1500–2000)
- The shipped cross-substrate evaluation primitive (`qcc run --performance-test`) exercised across the full simulator catalog
- All fourteen documented `qcc_*` metrics emitting and queryable
- Both shipped Grafana dashboards (`qcc-qpu-dashboard` / USE-Q, `qcc-circuit-dashboard` / RED-F)
- Cross-boundary identifier linkage proven bidirectionally at the UI level (IBM Quantum Console ↔ QCC dashboard)

## Updated R-framework — post-session reframe

The thesis R1–R5 requirements (Ch5 §5.8) are restated below with the **R5 reframe** developed during the 2026-05-24 evidence-capture session. The reframe propagates the Path D+ design decision (QCC-Design-State.md §7) into the requirement statement itself.

| Req | Statement (post-reframe) | Verdict | Primary evidence |
|---|---|---|---|
| **R1** | Production deployment patterns grounded in cloud-native operational practice | ✅ | `qpu_get_all.png`, `qpu_manifest_fake-fez.png`, `qpu_get_fale-fez.png`, `circuit_draw_shor.png` |
| **R2** | Cross-boundary observability using open standards at platform-operational scope | ✅ | `grafana_qcc_*_metrics.png` (all 14 metrics), `grafana_qpu_availability.png`, `grafana_qpu_coherence.png`, `grafana_circuit_get_shor_tuned_v{2,3}.png` |
| **R3** | Vendor-neutral orchestration: interface property, not demonstration breadth | ✅ | Typed adapter contract + Aer/IBM adapters shipped (see `docs/systems-design/QCC-System-Design.md` §6.3); `circuit_run_shor_perf-test.png` exercises both adapter paths uniformly |
| **R4** | Live cross-layer correlation via identifier linkage | ✅ | `ibm_console_job_tag_kingston.png` (forward: UID stamped on IBM job tags) + `grafana_circuit_provider_job_link.png` (reverse: Grafana clickable `provider_job_id`) — **hero pair** |
| **R5** | **Calibration-aware backend *evaluation* across heterogeneous, time-varying QPUs** | ✅ | `circuit_run_shor_perf-test.png` (cross-substrate primitive); `grafana_qpu_coherence.png` (family comparison); `grafana_shor_fake-{fez,kyoto,marrakesh}.png` (per-substrate outcomes) |

### R5 reframe in detail (paste-ready for Ch5 §5.8)

> **R5 — Calibration-aware backend evaluation across heterogeneous, time-varying QPUs.**
>
> When more than one QPU satisfies a circuit's hard constraints, the platform must expose per-candidate calibration data and outcome distributions so that backend choice can be made on informed empirical evidence — by automated scoring, by interactive operator review, or both. The decision must be reproducible from the trace.
>
> **Source.** Wilson 2020 (3–304% execution-fidelity variance from calibration drift); Murali 2019 (18× layout swing); Giortamis 2025 (38% spatial fidelity variance, ~100× queue imbalance); Seelam 2026 §III.E.2 ("system fidelities must be provided accurately to compilers").
>
> **Acceptance criterion.** Per-backend calibration is observable through documented metrics (`qcc_qpu_*` — six metrics including coherence, operation error medians, and calibration timestamps); cross-substrate outcome comparison is reproducible via `qcc run --performance-test` under a shared `qcc.io/experiment` label; the resulting evidence is reconstructible from the trace alone (the experiment label, the per-candidate `qcc_circuit_transpile_*` and `qcc_circuit_result_count` series, and the `Circuit.status.selectedQPU` field together specify the decision substrate).
>
> **Non-coverage.** Automated per-candidate composite scoring (Move 5 of the selection chain — NSGA-II multi-objective optimisation, formula-based predictive scoring, fake-twin empirical scoring) is deferred to Ch9. The empirical-evaluation primitive does not preclude these — it is the substrate they would consume. The shipped form satisfies R5 with *measurement rather than prediction*; the predictive variants are the next mode of evaluation layered on the same telemetry.

### Why this reframe matters

The pre-reframe wording bound R5 to *automated* selection — which would have made R5 a partial-evidence requirement (only Move 1 ships; Moves 2–5 are Ch9). The Path D+ decision (QCC-Design-State.md 2026-05-17 evening) explicitly traded predictive scoring for empirical cross-substrate evaluation. The reframed R5 captures what was actually built and aligns with the QCC docs' own framing of `qcc run --performance-test` as the shipped R5 evidence primitive.

### Design ↔ thesis R-numbering reminder

The QCC engineering docs use their own R1–R5 (QCC-System-Design.md §5) that map onto the thesis R1–R5 (Ch5 §5.8):

| Design Req | Thesis Req(s) |
|---|---|
| Design R1 — Declarative submission | Thesis R1 |
| Design R2 — Backend/QPU abstraction | Thesis R3 + R5 |
| Design R3 — Calibration-aware selection | **Thesis R5** (reframed) |
| Design R4 — Observable lifecycle | Thesis R2 + R4 |
| Design R5 — Separation of orchestration/quantum logic | Thesis R3 |

When QCC docs say "R3 evidence shipped as `--performance-test`," they mean *design* R3 → *thesis* R5.

---

## Evidence pack index

Every image with its source command, what it shows, the requirement(s) it evidences, and how it could be used in the thesis. Filenames are stable — cite them in `\includegraphics{Figures/ch7/<filename>}` after copying to the thesis repo.

### Setup, registration, and declarative API (R1, R3)

| File | Source command | Shows | Evidences | Thesis use |
|---|---|---|---|---|
| `qpu_get_all.png` | `kubectl get qpu` | 13 QPUs Available: `aer-statevector` (28q), `fake-belem` (5q), six `fake_*` Eagle/Heron sims (127–156q), `fake-fez` (156q, user-added), three `ibm-*` Heron r2 hardware (156q). | R1, R3 | Ch7 §"deployed QPU catalog" figure. Also a Ch6 reference for the QPU-as-K8s-resource argument. |
| `qpu_manifest_fake-fez.png` | `cat config/samples/qpu/fake-fez.yaml` | Minimal source YAML — 4 lines of spec (`provider: local`, `kind: simulator`). Everything else (basis gates, qubits, coherence, error medians) is auto-populated by the controller's probe. | R1, R3 | Ch6 §"declarative API" or Ch7 §"minimal QPU manifest" — argues that QCC's CRD design is genuinely declarative-minimal. |
| `qpu_get_fale-fez.png` | `kubectl get qpu fake-fez -o yaml` | Same QPU after probe completion: full status with basis gates `{cz, rz, sx, x, id, …}`, T1=144µs, T2=87µs, 352 coupling edges, `processor.family: Heron, revision: 2`, calibration timestamp. | R1, R3 | Ch7 §"probe-populated metadata" — pair with the manifest above to show the controller fills in the gaps. |
| `circuit_draw_shor.png` | `qcc draw examples/thesis/algorithms/shor.py` | ASCII rendering of the Shor circuit through the controller's `mode=draw` path: 4 control qubits with H, 4 target qubits with X on target_0, four `CU^{1,2,4,8}` oracle applications, inverse QFT, 4 measurements. Render time 259ms. | R1, R3 | Ch7 §"`mode=draw` demonstration" or §"the showcase circuit" — establishes what Shor looks like before any execution discussion. |

### Cross-substrate evaluation primitive (R3, R5) — *the centerpiece artifact*

| File | Source command | Shows | Evidences | Thesis use |
|---|---|---|---|---|
| **`circuit_run_shor_perf-test.png`** | `qcc run examples/thesis/algorithms/shor.py --performance-test --algorithm shor --version v1 --experiment thesis-perf-test` | One CLI invocation across all 10 candidate QPUs. `aer-statevector` ideal (top outcomes `0000:528 1000:504`). `fake-belem` correctly failed `TranspilationFailed` (5 qubits < Shor's 8). All other 8 simulators completed in 1.4s–23.3s. Per-substrate comparison table at bottom with depth/1Q/2Q/total/top-outcomes/time/job columns. Grafana deep link `/d/qcc-circuit?var-algorithm=shor&var-experiment=thesis-perf-test` at the end. | **R3, R5** | **Ch7 hero figure.** The complete demonstration of the shipped R5 evidence primitive. One image carries: cross-substrate observability, ideal-vs-noisy comparison, hard-constraint rejection (R5 Move 1), and the Grafana cascade convention. |
| `circuit_run_shor_aer_v1.png` | `qcc run shor.py --backend aer-statevector` | The ideal Shor reference on noiseless statevector simulation: depth 506, 665 gates, 288 2Q; results `0000:517 1000:507` (essentially 1024/1024 correct mass). 510ms wall-clock. | R3 | Ch7 §"ideal reference baseline" — defines what perfect Shor looks like, the anchor every noisy result is compared against. |
| `circuit_get_filtered_1.png` | `kubectl get circuits -l qcc.io/experiment=thesis` | Label-selector filtering on Circuits — single row visible at that capture moment (`shor-lpdb7`). | R1, R2 | Minor — supporting figure for §"the experiment-label discipline" or label-based dashboard cascade explanation. |
| `circuit_get_shor_lpdb7.png` | `kubectl get circuits shor-lpdb7 -o yaml` | Full YAML dump of a Circuit resource showing spec, source body inlined, status with conditions, results map, transpile shape, providerJobId. | R1, R2 | Ch7 §"Circuit lifecycle resource" — shows the full CRD shape for readers wanting to see the K8s-native artifact directly. |
| `grafana_shor_fake-fez.png` | Grafana Circuit dashboard, `circuit=shor-m4n5v` | Heron r2 simulator outcome — clear two-peak structure: `0000:236, 1000:217`, other bins 29–77. **Period structure recovered.** | R3, R5 | Ch7 §"per-substrate outcomes" — one of three side-by-side panels. |
| `grafana_shor-fake-marrakesh.png` | Grafana Circuit dashboard, `circuit=shor-qpp8j` | Heron r2 simulator outcome — `0000:289, 1000:239`, other bins 19–66. **Period structure recovered.** | R3, R5 | Same — middle panel of the cross-substrate triptych. |
| `grafana_shor_fake-kyoto.png` | Grafana Circuit dashboard, `circuit=shor-s8626` | Eagle r3 simulator outcome — all 16 bins 47–81, **essentially uniform noise**. The period structure is lost on Eagle r3 at this depth. | R3, R5 | Ch7 §"per-substrate outcomes" — right panel. **Pair the three Grafana panels as a single composite figure.** Directly evidences the hardware-generation argument visually. |

### Observability surface (R2) — *all 14 documented metrics emitting*

| File | Source command | Shows | Evidences | Thesis use |
|---|---|---|---|---|
| `grafana_qcc_qpu_metrics.png` | Grafana Explore, Prometheus datasource, `qcc_qpu` autocomplete | Metric browser dropdown listing all six documented `qcc_qpu_*` metrics: `coherence_seconds`, `condition`, `info`, `last_calibration_timestamp_seconds`, `operation_duration_median_seconds`, `operation_error_median`. **Schema complete.** | R2 | Ch7 §"observability schema implementation" or appendix — confirms QCC-Observability.md §5.1's documented inventory matches what actually emits. |
| `grafana_qcc_circuit_metrics.png` | Same, `qcc_circuit` autocomplete | All eight documented `qcc_circuit_*` metrics: `info`, `phase_duration_seconds_{bucket,count,observed,sum}`, `result_count`, `transpile_depth`, `transpile_gates`, `usage_seconds`, `circuits_total`. **Schema complete.** | R2 | Same — schema-completeness evidence. |
| `grafana_qpu_availability.png` | Grafana `qcc-qpu-dashboard`, S+U sections | USE-Q dashboard top: 13/13 Ready + Registered; QPU availability state timeline over the last 6h; calibration-age bargauges — `ibm-*` at 1.15 weeks (live, green), `fake_*` snapshots from 1.24 weeks to 5.55 years (yellow → red). | R2 | Ch7 §"USE-Q operational view" figure. Directly demonstrates the calibration-freshness signal that Ch5 R5's source citations (Wilson 2020) motivate. |
| **`grafana_qpu_coherence.png`** | Grafana `qcc-qpu-dashboard`, E section | USE-Q dashboard E: T1/T2 bargauges per QPU (`ibm-fez` T1=102µs, `ibm-kingston` T1=175µs, `ibm-marrakesh` T1=196µs); **family comparison panel showing 2Q error by processor family — Falcon 1.43%, Eagle 20.60%, Heron 0.32%**; registered-QPUs-by-family pie chart. | R2, R5 | **Ch7 hero figure (alongside the perf-test screenshot).** The family-comparison panel is a one-image proof of the hardware-generation argument: Heron r2's 2Q error is ~65× lower than Eagle r3 (dominated by `fake-belem`'s small qubit count + ancient calibration). Defends the "Heron r2 holds signal, Eagle r3 loses it" claim from a substrate-property direction in parallel with the outcome panels above. |
| `grafana_circuit_get_shor_tuned_v2.png` | Grafana `qcc-circuit-dashboard`, `circuit=shor-tuned-kingston-nb7q9` | RED-F per-Circuit detail (Tier-2 baseline, vanilla opt-3): transpile strip (depth 2K / 1Q 2K / 2Q 440 / total 2K); lifecycle phases (Submitting 8.00s, Running 11.00s); Running phase decomposition (on-QPU 3.00s vs off-QPU 8.00s); outcome histogram with 0000 (540), 1000 (530) peaks. | R2 | Ch7 §"RED-F per-Circuit observability" — left of a two-up Tier-2 comparison with v3 below. |
| `grafana_circuit_get_shor_tuned_v3.png` | Grafana same, `circuit=shor-tuned-kingston-62qzb` | RED-F per-Circuit detail (Tier-2 `scheduling_method: alap`): transpile strip (depth 2K / 1Q 2K / 2Q 442 / total 3K — total went up); lifecycle on-QPU 3.00s vs off-QPU 9.00s (alap added delays); outcome histogram with same period structure 0000 (554), 1000 (546). | R2 | Same — right of the Tier-2 comparison. The `total 3K` cell visually demonstrates the delay-injection effect of alap. |
| `grafana_circut_1.png` | Grafana Circuit dashboard, `algorithm=shor`, `experiment=thesis`, `circuit=shor-lpdb7` | Circuit dashboard with template-variable cascades working (algorithm → experiment → circuit). The aer-statevector run's outcome: only `0000` (~510) and `1000` (~500) bars visible — ideal Shor. | R2, R3 | Optional — §"dashboard variable cascade" or appendix. |

### Cross-boundary identifier linkage (R4) — *the strongest single thread in the pack*

| File | Source | Shows | Evidences | Thesis use |
|---|---|---|---|---|
| **`ibm_console_job_tag_kingston.png`** | IBM Quantum Console, job `d89dinp789is73935r0g` Details panel | IBM Console's own view of the QCC-submitted job. **Tags field: `qcc.circuit.uid:32afb048-6ce4-…`** (the forward UID stamp). Also visible: User `Ioannis Savvaidis`, Instance `qcc`, Quantum computer `ibm_kingston`, Mode `Job`, Program `sampler`, Status `Completed`, Total completion time 4m 22s, **Total usage 3s** (the substrate-billable compute time). | **R4 (forward)** | **Ch7 R4 hero figure (a)** — UI-level proof that the Circuit's K8s UID propagates into IBM Quantum Console as a job tag. The orchestration-overhead ratio falls out directly: 4m 22s wall-clock / 3s on-QPU = ~87× overhead. |
| **`grafana_circuit_provider_job_link.png`** | Grafana `qcc-circuit-dashboard`, Circuit identity panel for `shor-2vv42` | QCC-side Circuit identity table with clickable `Provider job id: d89dinp789is73…` cell, tooltip: **"Open on IBM Quantum (only valid for real-hardware job ids)"**, K8s UID column showing `afb048…`. | **R4 (reverse)** | **Ch7 R4 hero figure (b)** — UI-level proof that the QCC dashboard surfaces the IBM `provider_job_id` as a clickable link back to the IBM Console. **Pair (a)+(b) as a single composite "bidirectional identifier linkage" figure** — the strongest R4 demonstration the thesis produces. |
| `job_d89dinp789is73935r0g_results_measure_0.png` | IBM Quantum Console, same job, Results tab | IBM Console's histogram view of the same job — 16-bin distribution roughly matching the QCC card (0010 high, 0110 high, 0000/1000 low — noisy distribution at this depth without tuning). | R4 (supporting) | Optional — pair with the QCC-side card to demonstrate cross-system data parity. Not load-bearing once (a)+(b) above are in place. |
| `cli_detach_submission_kingston.png` | `qcc run examples/thesis/algorithms/shor.py --backend ibm-kingston --detach` | The `--detach` exit point: loading → submitted → targeting `ibm-kingston` → queued (`job d89dinp789is73935r0g`) → detached. CLI exits as soon as the provider job is queued; controller continues polling. | R1, R4 | Ch7 §"detached submission flow" — demonstrates the K8s-native "submit + walk away" pattern. Real-hardware jobs queue for minutes; the CLI doesn't block. |

### Real-hardware Shor on Heron r2 (v1 / vanilla opt-3, 1024 shots)

| File | Run | Backend | Transpile | Outcome | Use |
|---|---|---|---|---|---|
| `circuit_get_shor_fez_v1.png` | shor-wffxg | ibm-fez (Heron r2, 1Q 3.00e-04, 2Q 2.81e-03, T1 102µs, T2 90µs) | depth 2062, 2466 gates, 483 2Q | Correct mass ~10.8% (0000+1000 = 56+55=111/1024) | Ch7 §"real-hardware per-substrate variation" — one of three sibling runs. |
| `circuit_get_shor-wr9ds.png` | shor-wr9ds | ibm-marrakesh (Heron r2, 1Q 2.92e-04, 2Q 2.76e-03, T1 196µs, T2 99µs) | depth 2048, 2441 gates, 474 2Q | Correct mass ~8.4% (42+44=86/1024) | Same — middle of the three. |
| `circuit_detached_v1.png` | shor-2vv42 | ibm-kingston (Heron r2, 1Q 2.74e-04, 2Q 2.03e-03, T1 175µs, T2 117µs) — best gate errors of the three | depth 2048, 2441 gates, 474 2Q (`phase: Running` at capture) | Best calibration of the three Heron r2 hardware backends; later completed at ~8% correct mass | Ch7 §"async lifecycle in progress" + §"real-hardware substrate comparison." |
| `circuit_run_shor_fez_v1.png` | shor-wffxg | ibm-fez | The `qcc run` CLI output during the long-running fez submission (extremely wide image — many "submitting" status updates while the controller polled). | Minor — context for §"async lifecycle observability." |

Combined: **all three Heron r2 hardware backends produced essentially noise at 1024 shots without tuning.** Defends the "depth-1700 Shor on real NISQ hardware is intrinsically noisy" claim and motivates the Tier-2 evaluation below.

### Tier-2 evaluation on `ibm_kingston` (4096 shots) — *Sabre variance is the headline*

| File | Run | Tier-2 | Transpile | Outcome | Use |
|---|---|---|---|---|---|
| `circuit_get_shor-tuned.png` | shor-tuned-kingston-2nxgj (`phase: Running` at capture) | (early experiment) | **depth 1387, 1926 gates, 416 2Q** — the leanest transpile of the three | (not captured terminal) | Ch7 §"transpiler stochasticity" — this is the LEAN end of the Sabre-variance argument. |
| `circuit_get_shor_tuned_v2.png` | shor-tuned-kingston-nb7q9 (kit's `shor-v2.yaml`) | **none** (vanilla opt-3) | depth 1523, 2054 gates, 440 2Q | correct mass 26.1% (540+530=1070/4096) | Ch7 Tier-2 figure (a) — the baseline. |
| `circuit_get_shor_tuned_v3.png` | shor-tuned-kingston-62qzb (kit's `shor-v3.yaml`) | `scheduling_method: alap` | depth 1524, **2548 gates**, 442 2Q (+494 delay instructions) | correct mass 26.9% (554+546=1100/4096) | Ch7 Tier-2 figure (b) — the alap delta. |

**The Sabre-variance story.** Three vanilla-opt-3 runs on `ibm_kingston` produced depths **2048 → 1523 → 1387** (47% spread on the same source body and same backend). Without `seed_transpiler`, transpile-shape variance dominates any Tier-2 effect we observed. This motivates `seed_transpiler` as the most consequential Tier-2 key for thesis-citable reproducibility.

**The Tier-2 demonstration story (honest, not over-claimed).** alap visibly reaches the transpiler (the +494 delay instructions in `total_gates` are the observable signature of `scheduling_method` running). Correct-peak mass shifts within sampling noise (~0.8 pp on 4096 shots). The passthrough mechanism works end-to-end; outcome impact is neutral on this circuit/backend. *That is what a clean passthrough surface should look like — observable, predictable, no surprises.*

### Final state and supporting captures

| File | Source command | Shows | Use |
|---|---|---|---|
| `final_get_all_circuits.png` | `kubectl get circuits` | 19 Circuit resources, all labels populated (`algorithm=shor`, `version=v1/v2/v3`, `run-index` 1–12 auto-stamped by the controller), all `providerJobId`s present. | Ch7 §"experiment artifact summary" — proves the run-index auto-stamping + label propagation worked across the full session. |
| `circuit_all_after_run_v1.png` | `kubectl get circuits` (earlier in session) | 17 v1-only Circuits before v2/v3 manifests were authored. | Optional supporting figure. |

---

## Key findings from the 2026-05-24 session

These are the observations that emerged from actually running the kit and inspecting the artifacts. They should inform Ch7, Ch8, and Ch9 narrative beyond what the design docs already say.

### 1. The R4 bidirectional UI evidence is the strongest single thread

The pair `ibm_console_job_tag_kingston.png` + `grafana_circuit_provider_job_link.png` proves R4 at the level *the requirement actually demands*: a single identifier propagates across the classical–quantum boundary, observable in real time, queryable from both directions. Most R4 implementations in the literature stop at "we record a job ID somewhere." This pack shows the job ID being **clickable** from QCC's Grafana into IBM Quantum Console, and the QCC UID being **visible** in IBM Quantum Console's job tags. That's the contribution. Frame Ch7's R4 section around these two screenshots.

### 2. The family-comparison panel (`grafana_qpu_coherence.png`) is a hidden hero figure

A single Grafana panel shows Heron r2's 0.32% median 2Q error vs Eagle r3's 20.60% (the latter dragged up by `fake-belem`'s small qubit count + ancient calibration). Combined with the cross-substrate perf-test outcomes (Heron r2 sims keep period peaks, Eagle r3 loses them), this completes the hardware-generation argument the Ch1 SRE-diagnostic-on-real-hardware framing motivates. Worth a Ch7 paragraph in its own right.

### 3. Sabre stochasticity exceeds any Tier-2 effect we observed

Three vanilla-opt-3 runs on `ibm_kingston` produced depths 1387 / 1523 / 2048 — a 47% spread on the same source. The Tier-2 alap delta (+494 delay instructions) is smaller than this between-run variance. This makes `seed_transpiler: 42` the *most consequential* Tier-2 key for thesis-citable reproducibility, not for tuning. Recommend Ch7's Tier-2 section lead with this variance finding before any tuning narrative.

### 4. Path D+ (empirical R5 evidence) is fully evidenced

The `qcc run --performance-test` capture (`circuit_run_shor_perf-test.png`) is the complete demonstration. One CLI flag, 10 candidates, comparison table, Grafana deep link. `fake-belem` correctly rejected by Move 1 hard-constraint filter (`TranspilationFailed` for insufficient qubits). All other 8 simulators completed and produced distinguishable outcome shapes. Frame Ch7's R5 section around this — not around Move 5 (Ch9 future work) or composite scoring (also Ch9).

### 5. All fourteen documented `qcc_*` metrics are emitting

`grafana_qcc_qpu_metrics.png` and `grafana_qcc_circuit_metrics.png` together confirm the QCC-Observability.md §5 inventory is complete in implementation. No schema gap between documentation and reality. R2 is fully satisfied at the metric-vocabulary level.

### 6. Orchestration-overhead ratio is quantified end-to-end

From `ibm_console_job_tag_kingston.png`: IBM Console reports `Total usage 3s` for shor-2vv42, while `Total completion time` is `4m 22s` (262s). That's an **~87× overhead ratio** — wall-clock dominated by queue + transit + poll cadence, on-QPU compute is ~1% of the wall-clock. Matches QCC-Observability.md §13.1's quantitative target for the "orchestration-overhead window" panel. Use this number in Ch7/Ch8 directly.

### 7. The Tier-2 surface works as designed; "winning" Tier-2 keys are substrate-specific

alap on `ibm_kingston` adds delay instructions and produces neutral outcome shift. The kit's `approximation_degree: 0.95` was tuned against `fake_brisbane`'s Eagle r3 noise model and **degraded outcomes on Heron r2 hardware** during the iteration. `translation_method: synthesis` similarly degraded outcomes by re-decomposing Shor's structured QFT/controlled-mod-exp. **The Tier-2 surface exposes any Qiskit kwarg to the manifest — knowing which kwargs to add for a given (circuit, substrate) pair is the user-side empirical exercise the passthrough enables.** This is the honest scope of M3 and the natural setup for the Ch9 per-backend Tier-2 calibration bullet.

---

## Ch9 future-work bullets accumulated this session

Ready-to-paste bullets for `Chapters/09-Conclusions.tex`. Each is grounded in a specific gap observed during the evidence-capture session.

> **`samplerOptions:` Tier-2 block — sampler-instance options exposed declaratively.** The current Tier-2 `execute:` block plumbs `**kwargs` to `SamplerV2.run`, whose signature is strict (`pubs, *, shots`). Sampler-instance options that live on `sampler.options.*` — `dynamical_decoupling`, `twirling`, `resilience_level`, `default_shots`, `simulator.seed_simulator` — are unreachable from manifests today and would `TypeError` if attempted. Adding a `samplerOptions:` Tier-2 block routed onto `sampler.options.*` before `.run()` would unlock real-hardware noise mitigation (DD, twirling) and sampler-side reproducibility (`simulator.seed_simulator` when targeting IBM-side simulators), making the strongest single category of real-hardware tuning declaratively reachable.

> **Per-backend Tier-2 empirical calibration — `--performance-test --tier2-sweep`.** Tier-2 transpile-side options that improve outcomes on one substrate (e.g., `approximation_degree: 0.95` on `fake_brisbane` / Eagle r3) can degrade them on another (`ibm_kingston` / Heron r2). Knowing the right Tier-2 setting for a given substrate is currently a user-side empirical exercise. Extending the `qcc run --performance-test` primitive with a configured Tier-2 grid would let QCC produce per-backend Tier-2 recommendations alongside its existing cross-substrate ones — the natural in-substrate sibling of the cross-substrate primitive.

> **Calibration-aware layout via per-qubit error access + `mapomatic`.** The QPU CR exposes only median per-operation errors today. Picking low-error physical qubits for `initial_layout` requires per-qubit calibration data and a scoring procedure. Adding per-qubit error access to the QPU CR plus `mapomatic`-style scoring (Move 4 of the selection chain) would unlock the largest single Heron-r2-specific transpile-side win for deep circuits like Shor. Natural extension of the R5 acceptance criterion's "calibration vintage" requirement to per-qubit granularity.

> **Heron-r2 fractional-gate enablement.** `IBMAdapter` constructs the backend without `use_fractional_gates=True`, so Heron r2's native RZZ at arbitrary angles is invisible to the transpiler. Enabling it (a single constructor flag) plus an optional `qpu.spec.useFractionalGates` field would unlock the largest Heron-r2-specific transpile-side win for parameterized circuits (QFT-dominated, VQE ansätze). Conservative default-off preserves Eagle-era reproducibility.

> **Move 5 — Automated calibration-aware scoring layered on the empirical primitive.** The shipped `qcc run --performance-test` produces the per-candidate transpile shape and outcome distribution that an automated composite scorer would consume. Implementing the scorer — fidelity × freshness × queue-state — closes the loop from empirical evaluation to automated selection. The shipped telemetry surface is the substrate; the scoring formula is the missing layer. Naturally pairs with Giortamis 2025's NSGA-II Pareto-front approach as alternative scoring policies pluggable into the same observation substrate.

---

## Honest scope items the thesis should acknowledge

These observations support the Ch8 honest-assessment narrative directly.

- **Real-hardware Shor at depth ~1700 on Heron r2 is intrinsically noisy.** Three single runs on `ibm-fez` / `ibm-marrakesh` / `ibm-kingston` at 1024 shots produced 8–11% correct mass — at or below uniform-random. Without tuning, the period structure is lost. The 4096-shot Tier-2 runs recover it to ~26% — clear peaks but still well below the ideal 50%. This is the NISQ-era reality the thesis frames; the contribution is that QCC makes it *legible*, not that it eliminates it.

- **Tier-2 wins are substrate-specific and not transferable across IBM architecture generations.** The kit's `shor-tuned.yaml` settings (tuned against Eagle r3 simulation) degraded outcomes on Heron r2 hardware during this session. The passthrough surface exists; the substrate-specific knowledge does not yet live in QCC. This is the natural setup for the Ch9 per-backend Tier-2 calibration bullet.

- **Sabre transpiler stochasticity is large.** Run-to-run depth variance for the same source body on the same backend exceeded any Tier-2 effect observed. `seed_transpiler` is therefore not a "tuning" key but a **reproducibility** key — required for any thesis-citable comparison. Recommend Ch7 says this explicitly.

- **Move 1 (hard-constraint filter) ships; Moves 2–5 do not.** The `fake-belem` `TranspilationFailed` outcome in the perf-test is the visible evidence of Move 1 doing its job. Moves 2–5 (predictive scoring) are deferred to Ch9. The shipped form is empirical cross-substrate evaluation (`qcc run --performance-test`), which is the reframed R5 satisfaction path. Don't conflate "Move 5 is future work" with "R5 is partial" — the reframed R5 is fully satisfied.

- **Distributed tracing is deliberately not implemented.** R4 is satisfied via identifier linkage (UID forward + `provider_job_id` reverse), not via OTel spans. `Circuit.status.traceId` exists as a reserved field for the additive upgrade. The decision is documented in QCC-Design-State.md 2026-05-16 night entry. Frame this as a deliberate design choice in Ch8, not as a gap.

- **Executor-side OTel emission is Ch9.** The Python `qcc-executor` emits no OTel today; observability is transitively visible via the controller's view. This is acknowledged in QCC-Observability.md §1 scope table.

---

## Workflow notes for Chat Mode chapter-revision sessions

When uploading this pack to Claude Chat for thesis revision work, **do not zip the QCC repo** — drag in the specific files instead. Three uploads per session:

1. **The 31 evidence PNGs** from this `docs/` directory (drag-and-drop the lot — Chat Mode handles multi-file image uploads natively).
2. **This `docs/README.md`** + **`docs/RUNBOOK.md`** as two markdown files. They carry the framing context: R5 reframe wording, image index with per-figure thesis-use guidance, the new findings, the Ch9 bullets in paste-ready form, the per-chapter framing prompts below, and the command-by-figure mapping for the `\begin{listing}` blocks above each figure.
3. **`msc-thesis.zip`** (current chapter state — the only file that still benefits from being a zip, since the chapters and `_staging/` reference each other across files).

**Why no QCC zip?** The 31 figures plus the two markdown files carry everything Chat Mode needs for chapter-revision work; the rest of the QCC repo (code, manifests, full `docs/systems-design/` tree) is only needed for code-level questions, which belong in Claude Code in the QCC repo itself, not in chapter-writing Chat Mode sessions.

**Read this `docs/README.md` first** in the Chat Mode session — it carries the R5 reframe, the image index with thesis-use guidance, the empirical findings, and the Ch9 bullets in paste-ready form.

**Per-chapter framing prompts** (use verbatim):

- **Ch5 reframe.** *"Reframe R5 §5.8 from automated-selection wording to the calibration-aware-evaluation wording in `docs/README.md` §"R5 reframe in detail." Update the R5 statement, acceptance criterion, non-coverage paragraph, and the §5.8 mapping-table row. Move 5 → Ch9 as the automated specialization of empirical evaluation."*

- **Ch7 evidence integration.** *"Integrate the evidence pack indexed in `docs/README.md` into the Ch7 evaluation sections. Hero figures: `circuit_run_shor_perf-test.png` (cross-substrate primitive), the bidirectional R4 pair (`ibm_console_job_tag_kingston.png` + `grafana_circuit_provider_job_link.png`), `grafana_qpu_coherence.png` (family comparison), the Tier-2 v2/v3 comparison on `ibm_kingston`. Lead the Tier-2 narrative with Sabre variance (three v1 runs, depths 2048/1523/1387) rather than with tuning wins."*

- **Ch6 alignment pass.** *"Scan for places where Ch6 says 'selection' for the R5-aligned mechanism — rewrite to 'evaluation' where the empirical primitive is meant; keep 'selection' where Move 5 (Ch9) is the referent. Add a mention of `qcc run --performance-test` as the shipped R5/R3 evidence primitive. Cross-reference Ch7 figures."*

- **Ch8 reframe.** *"Add the honest-scope observations from `docs/README.md` §"Honest scope items the thesis should acknowledge". Specifically: Tier-2 wins are substrate-specific; Sabre stochasticity exceeds Tier-2 effects; distributed tracing is deliberately not shipped; real-hardware Shor is intrinsically noisy and QCC makes it legible without eliminating it."*

- **Ch9 update.** *"Add the five Ch9 future-work bullets from `docs/README.md` §"Ch9 future-work bullets accumulated this session" verbatim. Position them as the next-mode-of-evaluation roadmap: samplerOptions (declarative noise mitigation), --tier2-sweep (in-substrate empirical calibration), per-qubit/mapomatic (calibration-aware layout), fractional gates (Heron-r2 unlock), Move 5 (automated scoring layered on the empirical substrate)."*

**Staging convention** (unchanged): Chat Mode produces `.tex` artifacts named `Chapters--<file>--<section>.tex`, saved to `msc-thesis/_staging/chN/`. Claude Code in the msc-thesis repo merges them via the `merge staging chN` trigger.

---

*Pack captured 2026-05-24 by Ioannis Savvaidis. Companion thesis: "Interface between Quantum and Classical Computers," MSc Quantum Computing & Quantum Technologies, Democritus University of Thrace.*
