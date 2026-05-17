# QCC systems-design workspace

This directory contains the disciplined engineering design material for the Quantum Circuit Controller (QCC). It is intentionally separate from the thesis chapter files.

The files in this directory are the engineering source of truth. `Chapters/06-Architecture.tex` should later distil this material into academic prose, and `Chapters/07-Implementation.tex` should describe only the prototype that is actually implemented and evaluated.

## Files

| File | Role |
|---|---|
| `QCC-System-Design.md` | Canonical system design: purpose, scope, context, requirements, architecture, lifecycle, backend selection, failure model, limitations, and thesis mapping. |
| `QCC-API.md` | API design for the `Circuit` and `QPU` resources, including schema shape, field rationale, status model, and examples. |
| `QCC-Observability.md` | Observability design for status, events, metrics, traces, operational questions, and telemetry boundaries. |

## Supporting files

Two further files live in this directory:

| File | Role |
|---|---|
| `01-requirements-re-evaluation.md` | Working notes — requirements rationale and literature-aligned argument for R1–R5. Feeds Ch5 §5.8 prose. |
| `QCC-Initial-Design.md` | Historical staging — richer and longer than the canonical design needs. Pre-rename terminology; kept for traceability. |

## Design rule

QCC should be presented as a directional cloud-native control-plane prototype, not as a production quantum cloud platform.

The system is allowed to be engineered rigorously, but the thesis claim must remain scoped:

> QCC demonstrates how declarative orchestration, backend selection, and observability patterns from cloud-native systems can be applied to the quantum--classical execution interface.

## Suggested downstream flow

```text
_staging/systems-design/qcc/*.md
        ↓
Chapters/06-Architecture.tex
        ↓
Chapters/07-Implementation.tex
        ↓
Chapters/08-Discussion.tex / Chapters/09-Conclusions.tex
```

## Current status

The three canonical files (`QCC-System-Design.md`, `QCC-API.md`, `QCC-Observability.md`) have been reviewed across multiple sessions for cross-document consistency and against the literature:

- Lifecycle diagrams agree (`Transpiling → Succeeded` for `mode=select` is shown in both System-Design §8 and API §5.1).
- Resource-model field names agree across docs (`Circuit.spec.mode` taking `run` | `select` | `draw` | `schedule`; `Circuit.spec.backendSelector`; `QPU.spec.kind`).
- The five **design** requirements (System-Design §5) are explicitly mapped to the five **thesis** requirements from Ch5 §5.8.
- QCSC reference architecture positioning is in System-Design §4.2: QCC sits primarily at QCSC Layer 2 (System Orchestration) and absorbs a deliberate slice of Layer 3 (Application Middleware) — specifically per-candidate transpilation and layout evaluation — because calibration-aware selection requires these as scoring inputs.
- The Executor uses an internal `Adapter` ABC with two registered providers: `AerAdapter` (in-process Qiskit Aer + `fake_*` snapshots + method-pinned variants like `aer_statevector`, no credentials) and `IBMAdapter` (direct `qiskit-ibm-runtime` via `QiskitRuntimeService` + `SamplerV2`, shipped M3 2026-05-16 with end-to-end verification on `ibm_kingston`).  Alternative substrates (QRMI for Pasqal/multi-vendor, CUDA-Q for NVIDIA) are Ch9 future-work; see `QCC-Design-State.md` §7d (QEI direction) for the public-interface promotion path.
- Idempotency mechanics are illustrated once in API §6 (sequence diagram + client-key strategy) and referenced from System-Design §12.

The next step is to use these files as the source for `Chapters/06-Architecture.tex`, drawing the academic prose from the engineering material without re-specifying it. Use role-based personas ("the cluster administrator", "the user") rather than fictional names — the convention used by both Bacher et al. and Seelam et al.
