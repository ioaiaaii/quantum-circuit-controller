# Implementation Status

This document is the blunt feature matrix for the current repository.

Legend:

- `shipped`: implemented and part of the normal runtime path
- `partial`: present but with important caveats or only partly wired
- `absent`: not implemented

## Control Plane

| Area | Status | Notes |
|---|---|---|
| `Circuit` CRD | shipped | namespaced, four modes |
| `QPU` CRD | shipped | cluster-scoped |
| Circuit phase machine | shipped | explicit phases in controller |
| Artifact `ConfigMap` model | shipped | drawing, converted QASM, schedule |
| First-pass backend filtering | shipped | availability/provider/backend/kind/minQubits/maxShots |
| Calibration-aware backend scoring | absent | design exists, runtime does not |
| `allowedQPURefs` enforcement | absent | field exists on schema only |
| `region` enforcement | absent | field exists on schema only |
| Kubernetes `Event` emission | absent | RBAC exists, recorder path does not |

## Execution Backends

| Area | Status | Notes |
|---|---|---|
| Aer simulator execution | shipped | sync path |
| Fake IBM snapshot execution | shipped | through `AerAdapter` |
| Method-pinned Aer variants | shipped | `aer_statevector`, etc. |
| IBM hardware execution | shipped | async path via `qiskit-ibm-runtime` |
| Generic multi-provider adapter | absent | not implemented |
| QRMI adapter | absent | docs may mention it historically, code does not ship it |
| CUDA-Q adapter | absent | not implemented |

## Source Handling

| Area | Status | Notes |
|---|---|---|
| OpenQASM 3 input | shipped | inline source |
| Qiskit-Python input | shipped | executor converts to QASM 3 |
| Converted QASM artifact | shipped | stored as `convertedRef` |
| ASCII drawing | shipped | `mode=draw` |
| Backend schedule timeline | shipped | `mode=schedule` |

## CLI

| Command / behavior | Status | Notes |
|---|---|---|
| `qcc run` | shipped | create + wait |
| `qcc run --detach` | shipped | exits once queued |
| `qcc run --performance-test` | shipped | empirical comparison across QPUs |
| `qcc draw` | shipped | short-lived draw `Circuit` |
| `qcc schedule` | shipped | short-lived schedule `Circuit` |
| `qcc get` | shipped | circuit/qpu inspection |
| label-based grouping (`algorithm`, `version`, `experiment`) | shipped | plus auto `run-index` and `source-sha256` |
| `qcc delete` | absent | not implemented |
| `qcc lint` | absent | not implemented |

## Observability

| Area | Status | Notes |
|---|---|---|
| controller-side OTLP metrics | shipped | `qcc_*` namespace |
| Grafana dashboards | shipped | source-controlled YAML |
| cross-boundary IBM job tags | shipped | Circuit UID stamped into provider job |
| per-Circuit status/debug through CRD | shipped | main operational surface |
| executor-side metrics | absent | no Python OTel instrumentation |
| distributed tracing | partial | scaffolding exists, not operational |
| controller-runtime built-ins scraped by default | absent | endpoint exists, default scrape path disabled |

## Backend Metadata / QPU Model

| Area | Status | Notes |
|---|---|---|
| QPU probing through executor | shipped | qubits, basis, coupling, calibration, medians |
| `status.qubits` as authoritative count | shipped | falls back to `spec.qubits` |
| IBM optimistic availability | partial | probe failure does not remove from selection |
| `queueDepth` maintenance | partial | field exists, not central runtime behavior |
| `lastError` maintenance | partial | field exists, not a strong current path |
| per-QPU credential reference | absent | schema only; runtime uses env vars |

## Reliability / Recovery

| Area | Status | Notes |
|---|---|---|
| sync simulator execution | shipped | no async state recovery needed |
| async hardware execution | shipped | works while executor stays alive |
| async restart tolerance | absent | in-memory task registry only |
| idempotent generation-based controller submissions | partial | controller derives idempotency key; full crash-recovery semantics are not complete |

## Deployment / Dev Workflow

| Area | Status | Notes |
|---|---|---|
| kind-based local deployment (`make dist-up`) | shipped | easiest local path |
| separate controller/executor deployments | shipped | implemented in manifests |
| sample QPU catalog | shipped | `aer-statevector`, fake fleet, IBM samples |
| default ready-to-use QPUs on deploy | absent | must apply samples explicitly |
| Helm packaging | absent | post-thesis / not implemented here |

## What To Trust

If you need one sentence:

- Trust `run`, `draw`, `schedule`, `get`, Aer/fake backends, IBM async execution, QPU probing, and controller-side metrics.
- Do not trust the API to enforce every declared selector/credential field yet.
- Treat restart-tolerance, scoring-based selection, distributed tracing, and per-QPU credentials as unfinished areas.
