# Design deepening — Thread 01: Requirements re-evaluation

**Thread.** Re-evaluate R1–R4 in Ch5 §5.8. Challenge whether more are needed. Make them concrete, clear, reasonable.

**Inputs.** `QCC-Design.md` (merged spec + Appendix N working notes); the four walk files in `_staging/critical-readings/`; current Ch5 §5.8 prose.

**Mode.** Chat Mode 2 — proposal only. No LaTeX. The §5.8 rewrite and design-doc updates are downstream Claude Code work once decisions land.

---

## 1. The core finding

R1–R4 cover three of the four design-state goals. **Goal 1 (Accuracy) has no requirement-level anchor.** The five-move accuracy chain — QCC's most distinguishing technical apparatus — is an architectural choice the design *makes* without a requirement that *demands* it. The merged design doc specifies *how* (§A.4 auto-selection mode, §E.4 chain budgets, §M.6 selection attributes) but Ch5 never says *why it must be there*.

This is the strongest argument for adding R5. Two smaller fixes follow from the walks:

- **R3 needs reframing** (Qubernetes OBJECTION 2). Current R3 applies a multi-vendor-evaluation bar measured by demonstrated breadth. Reframe: vendor-neutrality as a property of the interface and schema. The prototype ships `AerAdapter` (in-process Aer + `fake_*` snapshots + method-pinned variants) and `IBMAdapter` (real IBM Quantum hardware via `qiskit-ibm-runtime`); alternative substrates (QRMI for Pasqal/multi-vendor, CUDA-Q for NVIDIA) are Ch9 future-work.  The design claim does not depend on counting backends.
- **R4 needs the live-vs-post-hoc property** (per merged §K row on Kanazawa). Without it, R4 could be satisfied by reconstructing the trace from logs after the fact.

Plus: **Open Q2 closes affirmatively** — R1–R5 trace cleanly to Seelam's three convergence developments + Qubernetes' six objectives + Kanazawa's L0–L4 pyramid. The trace turns four parallel paragraphs into a structured response.

---

## 2. Proposed R1–R5 (final form)

Each requirement gets four parts: **statement / source / acceptance criterion / non-coverage**. Acceptance criteria make Ch7 evaluation testable rather than rhetorical. Each non-coverage statement traces to merged §L.3 ("What it does NOT claim").

### R1 — Production deployment patterns grounded in cloud-native operational practice

- **Statement.** Single Helm chart installs the controller, executor, RBAC, and CRDs with leader election, status conditions, finalisers, and prometheus-operator integration. Portable across managed and unmanaged clusters without node-level customisation. The controller is idempotent at every reconciliation phase, with non-duplicating submission semantics under restart.
- **Source.** Qubernetes O1+O5; `qubernetes.md` SCAFFOLD on node-level vs namespace-level wiring (§7.5 admission p. 24); Kanazawa OBJECTION 3 on platform separation.
- **Acceptance criterion.** Helm chart deploys cleanly on kind/k3d/EKS/AKS/GKE without per-cluster modification. `vendorJobId` is written to status before phase transition, preventing double-submission on reconciler restart (§I.1).
- **v2 reference.** §A.5, §H, §I (twelve failure modes), §J.3 SLO, §J.5 runbook.
- **Non-coverage.** Production-grade HA with chaos testing, multi-tenancy, Pod Security Standards, full mTLS controller↔executor. *(§L.3: "single-replica, no chaos testing"; "multi-tenancy… out of scope".)*

### R2 — Cross-boundary observability using open standards at platform-operational scope

- **Statement.** Instrument the classical–quantum boundary using cloud-native open-standard primitives: Kubernetes CRDs (status + Conditions + Events) for per-instance lifecycle observability, Prometheus-compatible metric exposition for aggregate behaviour, and a documented `qcc.*` metric vocabulary covering Kanazawa's L0–L3 layers (system, job, task).  Delegate L4 (domain) to workflow code.
- **Source.** Seelam §III.E.2 specialised-node-exporters; Kanazawa §II metrics pyramid; Kanazawa OBJECTION 1 on workflow-specific L4.
- **Acceptance criterion.** The `qcc.*` semantic conventions document (in `QCC-Observability.md` §5) specifies metric names, attributes, units, stability tiers, and cardinality discipline for L0–L3.  A reference implementation emits these from the controller (executor behaviour observable transitively via controller-runtime gRPC instrumentation).
- **v2 reference.** `QCC-Observability.md` §4 (idiomatic principles), §5 (locked metric inventory), §10 (PromQL query patterns), §11 (dashboard sketch).
- **Non-coverage.** OpenTelemetry distributed tracing infrastructure (considered, rejected — see `QCC-Design-State.md` 2026-05-16 night entry; cross-boundary correlation is satisfied via the identifier-linkage approach in R4 below).  Profiling tools (Seelam #3); analytical/ETL retrospective queries (Kanazawa OBJECTION 2); domain L4.  The `qcc.*` namespace is offered as a candidate proposal, not claimed as a community standard.

### R3 — Vendor-neutral orchestration: interface property, not demonstration breadth

- **Statement.** The vendor-specific runtime stays behind a typed adapter boundary. Adding a second vendor requires a new adapter only — no controller code change, no CRD schema change, no observability surface change. Vendor-neutrality is a property of the interface and schema, not of the count of demonstrated backends.
- **Source.** Qubernetes OBJECTION 2 (`qubernetes.md` line 685, single-vendor admission §8.1 p. 26); ADD on namespace-vs-adapter distinction line 215.
- **Acceptance criterion.** Testable by inspecting the `QPU.spec.provider` discriminator and the adapter protobuf service definition, not by counting backends.  The prototype ships two adapters (`AerAdapter`, `IBMAdapter`), demonstrating that vendor coverage is interface-driven; alternative substrates land via the same seam (Ch9 future-work).
- **v2 reference.** §E.2 Go interface, §E.3 protobuf contract, §E.11 QRMI/CRI lineage, §B.8 OQASM3, §J.4 maintainability test.
- **Non-coverage.** UQA-conformant adapter (future work). *(§L.3: "vendor coverage follows QRMI upstream; QCC inherits new vendors without per-vendor code".)*

### R4 — Live cross-layer correlation operationalising Seelam's convergence development #2

- **Statement.** A single stable identifier propagates across the classical–quantum boundary linking algorithm control flow, controller reconciliation, executor work, vendor submission, and result retrieval.  Correlation is **live and end-to-end** — the identifier is queryable in real time as the circuit progresses, not reconstructed post-hoc from persisted logs.  The identifier convention is the Circuit's K8s UID, carried through gRPC calls and stamped onto IBM `runtime_options.tags`.
- **Source.** Seelam §III.E.2 cross-layer-observability quote; Kanazawa §II.D infrastructure-aware framing; merged §K row on Kanazawa naming the live-vs-post-hoc distinction.
- **Acceptance criterion.** The Circuit's K8s UID appears on `Circuit.status.traceId` (already populated), in controller log lines (controller-runtime convention), and in IBM Quantum Console job tags via `runtime_options.tags` (best-effort).  Reverse lookup is bidirectional: a job in IBM Console resolves to its owning Circuit, and a Circuit resolves to its IBM job ID via `Circuit.status.providerJobId`.
- **v2 reference.** `QCC-Observability.md` §6 (cross-boundary identifier linkage), §7 (Kubernetes status and events).
- **Non-coverage.** OpenTelemetry distributed tracing / W3C trace context propagation — considered, rejected as disproportionate to operational utility at thesis scale (see `QCC-Design-State.md` 2026-05-16 night entry).  The identifier-linkage approach satisfies the *live cross-layer correlation* requirement with materially less infrastructure cost.  Profiling tools (Seelam #3); domain-level cross-correlation (Kanazawa's L4).

### R5 *(new)* — Calibration-aware backend selection across heterogeneous, time-varying QPUs

- **Statement.** When more than one QPU satisfies a circuit's hard constraints, selection uses live calibration data, layout fidelity estimates, and queue-state signals — not static configuration or first-come-first-served. The selection must be reproducible from the trace alone.
- **Source.** Wilson 2020 (3–304% accuracy variance from drift); Murali 2019 (18× layout swing); Qonductor §3 p. 4 (38% spatial fidelity variance, ~100× queue imbalance); Seelam §III.E.2 ("system fidelities… must be provided accurately to compilers").
- **Acceptance criterion.** The controller's selection step records on `Circuit.status.selectionSummary` the count of candidates evaluated, the selected backend's composite score, the calibration vintage used per candidate, and the per-candidate transpile-attempt results.  Decision is reconstructible from the CR + K8s Events trail.  Selection-related metrics (`qcc_qpu_*` for the inputs; `qcc_circuits_total{mode="select"}` for the outcomes) corroborate the per-instance record.
- **v2 reference.** §A.4 auto-selection mode, §E.4 five-move chain, §E.10 Python sketch, §M.6 selection attributes.
- **Non-coverage.** NSGA-II Pareto-front simultaneous optimisation (Qonductor's territory; future work as alternative scoring policy). *(§L.3: "the 5-move accuracy chain is not optimal — only composable and observable".)*

**Why R5 is not a literature gap.** Accuracy-aware selection is well-explored (mapomatic, Qiskit, Qonductor's NSGA-II, Murali's noise-aware compilation). R5 is grounded in empirical reality the literature documents but each existing system addresses differently; the QCC contribution at R5 is the **deployable, observable instantiation**, not the selection mechanism itself. Ch5 §5.8 should explicitly distinguish this from R1–R4's gap-fill framing.

---

## 3. R1–R5 traced to external enumerations (closes Open Q2)

Single framing paragraph for §5.8, between the table and R1:

| Req. | Seelam dev. | Qubernetes obj. | Kanazawa pyramid | Genuinely new |
|---|---|---|---|---|
| R1 | (cross-cut) | O1 + O5 | (deployment) | No — extends prior practice |
| R2 | #1 node exporters | O6 monitoring | L0–L1 system-centric | Standards extension |
| R3 | (cross-cut) | O2 + O3 | (substrate) | Interface extension |
| R4 | #2 cross-layer | (no analogue) | L2–L3 cross-layer | **Yes** — operationalises Seelam #2 |
| R5 | (implicit in #2) | (below O-level) | L3 freshness | Operationalisation |

R4 is the genuinely-new requirement; R5 operationalises empirical reality. Seelam #3 (profiling) is explicit non-coverage.

---

## 4. Non-requirements (decision pending — see D3)

Five recurring non-coverage points worth naming explicitly in §5.8 (parallel numbering NR1–NR5) or keeping in prose:

| # | Non-requirement | Source |
|---|---|---|
| NR1 | Profiling tools | Seelam convergence dev. #3 |
| NR2 | Domain-level (L4) telemetry | Kanazawa OBJECTION 1 |
| NR3 | Multi-programming / circuit cutting / qubit reuse | QOS territory; shared deferral with Qonductor |
| NR4 | HPC tight coupling (Phase 2/3) | Seelam phase axis |
| NR5 | Analytical / retrospective deep-storage queries | Kanazawa OBJECTION 2 |

All trace to merged §L.3.

---

## 5. Decisions

| # | Question | Recommendation |
|---|---|---|
| D1 | Add R5? | **Yes.** Goal 1 needs requirement-level grounding; the five-move chain otherwise reads as engineering enthusiasm. |
| D2 | R5 numbering — append (R1–R5) or interleave? | **Append.** Preserves existing reference structure; R5 reads naturally as the empirical-reality requirement complementing the four gap-fill ones. |
| D3 | Numbered NR1–NR5, or non-coverage in prose? | **Numbered NR1–NR5.** Gives Ch7 a clean "not testing" surface; matches Kanazawa's pattern of explicit boundary-naming. |
| D4 | Sharpen R3 per Qubernetes OBJECTION 2? | **Yes.** Current R3 reads as a gap claim that doesn't survive the walk's authorial self-acknowledgement. |
| D5 | Close Open Q2 affirmatively? | **Yes.** Walks have produced enough evidence to anchor R1–R5 to all three external enumerations. |
| D6 | Update merged doc Appendix N when Thread 01 lands? | **Yes.** N.2 gets R1–R5 in the four-part form; N.3 marks Q2 resolved; N.5 picks up the §K Qubernetes erratum. |
| D7 | Add explicit resilience clause to R1 acceptance criterion? | **Yes, lightly.** One sentence: "controller idempotent at every phase; non-duplicating submission under restart (§I.1)." Costs little, gives Ch7 a clean property to test. |

---

## 6. What this triggers downstream

When D1–D7 are decided, Claude Code does:
1. **Ch5 §5.8 rewrite** — add framing paragraph (table); rewrite R1–R4 to four-part form; add R5; add NR1–NR5 (per D3); update the §5.8 mapping table at line 897 with R5 row.
2. **Ch1 §1.4 alignment fix** — line 420–421 currently describes Ch7 PoC as *"a VQE workflow orchestrated as K8s Jobs"*; the locked QCC design uses CRDs + controller, not standard Jobs. Replace with *"orchestrated through Kubernetes Custom Resources reconciled by an operator"* (or similar). This is the Ch1-side counterpart to the same conceptual blur the Qubernetes walk fixed in Ch5 line 685; fixing them together keeps the lineage clean. Independent of design-state Open Q1 (which is about §1.1 motivation paragraph, parked until Ch6 locks).
3. **Merged doc Appendix N update** — N.2 rewritten with R1–R5; N.3 marks Q2 resolved; N.5 retains item 1 (the §K Qubernetes `QuantumJob` CRD erratum) on the queue for a separate deliberate revision.
4. **Bibliography corrections** — separate thread; not Thread 01's job.

---

## 7. Next thread suggestion

Thread 02 best candidate: **R5 ↔ design-state Goal 1 reconciliation** — map Goal 1's four sub-properties (composed transpilation, layout selection, calibration-aware selection, freshness-aware queue handling) onto R5's acceptance criterion and the five-move chain's Moves 1–5. Tightens Appendix N.2 and feeds Ch6 §6.2.

Alternative: **Ch6 §6.2 architecture mapping** using R1–R5 as inputs, with cross-cut framing (Q3 resolved) and the §K Qubernetes erratum applied.

---

*End of Thread 01 proposal.*
*Stage to repo at `_staging/design-deepening/01-requirements-re-evaluation.md`. Decisions D1–D7 → Claude Code merge.*
