# QCC Documentation

Seven documents, each with one job. Where design and implementation
differ, the docs say so explicitly rather than describing the aspiration.
The full set is about an hour of reading; the first two rows cover most
first visits in about fifteen minutes.

| Doc | Read it when you want to | Time |
|---|---|---|
| [getting-started.md](./getting-started.md) | stand QCC up, run your first circuit, look up any CLI command | 6 min |
| [demonstration.md](./demonstration.md) | see the whole platform exercised: Shor across simulators, real IBM hardware, dashboards, transpiler tuning | 8 min |
| [architecture.md](./architecture.md) | understand the design, the SRE principles behind it, and where QCC sits relative to QCSC, Qubernetes, Qonductor, and QRMI | 11 min |
| [api.md](./api.md) | author `Circuit`/`QPU` resources, consume status, or program against the gRPC contract | 8 min |
| [observability.md](./observability.md) | query the `qcc_*` metrics or use the dashboards | 6 min |
| [engineering.md](./engineering.md) | change the code: principles, toolchain, adapters, build and test, CI | 9 min |
| [operations.md](./operations.md) | deploy, configure credentials, understand the security posture, troubleshoot | 6 min |

## Implementation status

The single most useful table in these docs. Legend: **shipped** means
implemented and on the normal runtime path; **partial** means present but
caveated; **absent** means not implemented.

| Area | Status | Notes |
|---|---|---|
| `Circuit` CRD | shipped | namespaced, four modes |
| `QPU` CRD | shipped | cluster-scoped |
| Circuit phase machine | shipped | explicit phases in the controller |
| Artifact `ConfigMap` model | shipped | drawing, converted QASM, schedule |
| First-pass backend filtering | shipped | availability, provider, backend, kind, minQubits, maxShots |
| Calibration-aware backend scoring | absent | design exists; the runtime filters and picks first match |
| `allowedQPURefs` enforcement | absent | field exists on the schema only |
| `region` enforcement | absent | field exists on the schema only |
| Aer simulator execution | shipped | synchronous path |
| Fake IBM snapshot execution | shipped | through `AerAdapter` |
| IBM hardware execution | shipped | asynchronous path via `qiskit-ibm-runtime` |
| Generic Qiskit-provider adapter | absent | future path for Braket and similar |
| OpenQASM runtime adapter | absent | future path for non-Qiskit runtimes |
| QRMI / CUDA-Q adapters | absent | future alternative substrates |
| OpenQASM 3 input | shipped | inline source |
| Qiskit-Python input | shipped | executor converts to QASM 3 server-side |
| `qcc run` / `draw` / `schedule` / `get` | shipped | normal CLI surface |
| `qcc run --performance-test` | shipped | empirical comparison across QPUs |
| Controller-side OTLP metrics | shipped | the 14-metric `qcc_*` specification |
| Grafana dashboards | shipped | source-controlled YAML in `deploy/grafana/` |
| Cross-boundary IBM job tags | shipped | Circuit UID stamped into the provider job |
| QPU probing through the executor | shipped | qubits, basis, coupling, calibration, medians |
| IBM optimistic availability | partial | probe failure does not remove a QPU from selection |
| Per-QPU credential reference | absent | schema only; runtime uses executor env vars |
| Async restart tolerance | absent | executor task registry is in-memory |
| Executor-side metrics | absent | no Python OTel instrumentation |
| Distributed tracing | partial | provider scaffolding exists, no spans exported |
| Kubernetes `Event` emission | absent | RBAC exists, recorder path does not |
| Default ready-to-use QPUs on deploy | absent | apply `config/samples/qpu/` explicitly |
| Helm packaging | absent | kustomize is the shipped deployment path |

## Source-of-truth rule

If the docs disagree with anything: the code wins, then these docs. The
[thesis](https://ioaiaaii.github.io/project/msc-thesis/) documents the
evaluated v1.0.0 snapshot; the repository moves on.
