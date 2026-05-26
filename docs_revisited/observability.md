# Observability

This document describes the telemetry and operational surfaces that exist in the current implementation.

## Short Version

Today, QCC is observable primarily through:

- `Circuit.status`
- `QPU.status`
- artifact `ConfigMap`s
- controller-side `qcc_*` metrics
- controller and executor logs

It is not yet observable through end-to-end traces or executor-side telemetry.

## What Exists Today

| Surface | Status | Notes |
|---|---|---|
| `Circuit.status` | implemented | primary per-run lifecycle surface |
| `QPU.status` | implemented | primary backend metadata surface |
| Artifact `ConfigMap`s | implemented | drawings, converted QASM, schedules |
| `qcc_*` metrics via OTLP | implemented | controller-side instrumentation |
| Grafana dashboards | implemented | source-controlled dashboards under `deploy/grafana/` |
| Cross-boundary IBM job tagging | implemented | Circuit UID stamped into IBM job tags |
| Executor-side OTel instrumentation | not implemented | no Python-side metrics/traces yet |
| Real distributed tracing | scaffolded only | tracer provider skeleton, no active exporter path in use |
| Kubernetes `Event` emission | not implemented | docs and RBAC mention it, runtime does not emit them today |
| Direct scrape of controller-runtime built-ins | disabled by default | manifests exist but are not enabled in default deployment |

## The Current Observability Model

QCC is observable through three main channels.

### 1. Resource Status

`Circuit.status` and `QPU.status` are the primary operational truth.

Examples:

- current phase
- selected backend
- provider job ID
- transpile depth and gate counts
- backend error medians
- backend calibration timestamp

### 2. Artifact `ConfigMap`s

Generated large payloads live in owned `ConfigMap`s and are read by the CLI.

- ASCII drawings
- converted OpenQASM 3
- scheduled timelines

### 3. Metrics

The controller exports OTLP metrics to a collector.

Pipeline:

```text
qcc-controller
    -> OpenTelemetry SDK
    -> OTLP/gRPC
    -> OpenTelemetry Collector
    -> Prometheus exporter
    -> Prometheus scrape
    -> Grafana dashboards
```

## Operational Questions And Where To Look

| Question | Best current surface |
|---|---|
| Which phase is my circuit in? | `Circuit.status.phase` |
| Which backend was chosen? | `Circuit.status.selectedQPU` |
| Why did it fail? | `Circuit.status.conditions` |
| What exactly was run? | `status.convertedRef` for Qiskit inputs, plus `status.transpile` |
| What did the backend look like? | `QPU.status` |
| How long did phases take? | `qcc_circuit_phase_duration_seconds_observed` and `qcc_circuit_phase_duration_seconds` |
| Which IBM job corresponds to this Circuit? | `status.providerJobId` and `qcc_circuit_info{provider_job_id=...}` |
| Which Circuit produced this IBM job? | IBM job tag + `status.providerJobId` |

## Circuit Metrics

### Observable gauges

- `qcc_circuit_info`
- `qcc_circuit_transpile_depth`
- `qcc_circuit_transpile_gates`
- `qcc_circuit_result_count`
- `qcc_circuit_phase_duration_seconds_observed`
- `qcc_circuit_usage_seconds`

### Event-driven instruments

- `qcc_circuits_total`
- `qcc_circuit_phase_duration_seconds`

### Circuit Metric Inventory

| Metric | Type | Purpose |
|---|---|---|
| `qcc_circuit_info` | observable gauge | identity row for joins |
| `qcc_circuit_transpile_depth` | observable gauge | post-transpile depth |
| `qcc_circuit_transpile_gates` | observable gauge | gate counts by kind |
| `qcc_circuit_result_count` | observable gauge | per-bitstring outcome counts |
| `qcc_circuit_phase_duration_seconds_observed` | observable gauge | per-Circuit phase durations from status timestamps |
| `qcc_circuit_usage_seconds` | observable gauge | hardware-reported compute time |
| `qcc_circuits_total` | counter | phase-transition count |
| `qcc_circuit_phase_duration_seconds` | histogram | fleet-level phase-duration distribution |

## QPU Metrics

- `qcc_qpu_info`
- `qcc_qpu_operation_error_median`
- `qcc_qpu_operation_duration_median_seconds`
- `qcc_qpu_coherence_seconds`
- `qcc_qpu_last_calibration_timestamp_seconds`
- `qcc_qpu_condition`

### QPU Metric Inventory

| Metric | Type | Purpose |
|---|---|---|
| `qcc_qpu_info` | observable gauge | identity row for joins |
| `qcc_qpu_operation_error_median` | observable gauge | 1Q/2Q/readout medians |
| `qcc_qpu_operation_duration_median_seconds` | observable gauge | 1Q/2Q median durations |
| `qcc_qpu_coherence_seconds` | observable gauge | T1/T2 medians |
| `qcc_qpu_last_calibration_timestamp_seconds` | observable gauge | calibration freshness |
| `qcc_qpu_condition` | observable gauge | KSM-style condition matrix |

## What The Metrics Mean

### Circuit-side

- `qcc_circuit_info`: identity row for joins, with labels such as mode, source format, shots, qpu, and provider job ID
- `qcc_circuit_transpile_depth`: post-transpile depth
- `qcc_circuit_transpile_gates`: gate counts split by `kind=single_qubit|two_qubit|total`
- `qcc_circuit_result_count`: per-bitstring result counts
- `qcc_circuit_phase_duration_seconds_observed`: per-Circuit phase durations derived from condition timestamps
- `qcc_circuit_usage_seconds`: hardware-reported billable compute time when available
- `qcc_circuits_total`: phase-transition counter
- `qcc_circuit_phase_duration_seconds`: fleet-level histogram of phase durations

### QPU-side

- `qcc_qpu_info`: identity row for joins
- `qcc_qpu_operation_error_median`: median 1Q, 2Q, and readout error rates
- `qcc_qpu_operation_duration_median_seconds`: median 1Q and 2Q gate durations
- `qcc_qpu_coherence_seconds`: T1 and T2 in seconds
- `qcc_qpu_last_calibration_timestamp_seconds`: last calibration Unix timestamp
- `qcc_qpu_condition`: KSM-style condition matrix

## Dashboards

Dashboard manifests live here:

- [`../deploy/grafana/qcc-circuit-dashboard.yaml`](../deploy/grafana/qcc-circuit-dashboard.yaml)
- [`../deploy/grafana/qcc-qpu-dashboard.yaml`](../deploy/grafana/qcc-qpu-dashboard.yaml)

The platform deployment values for Prometheus, Tempo, and the collector live under [`../deploy/platform/`](../deploy/platform).

## Logs

There are two important log streams:

- controller logs
- executor logs

Typical commands:

```bash
kubectl logs -n quantum-circuit-controller-system deployment/quantum-circuit-controller-controller-manager -c manager
kubectl logs -n quantum-circuit-controller-system deployment/quantum-circuit-controller-executor
```

Use logs when:

- the executor RPC failed transiently
- backend probing failed
- a provider-specific adapter raised an exception
- you need details not stored in CR status

## Cross-Boundary Identity

QCC exposes both sides of the "which Kubernetes run produced which provider job?" problem.

### Forward linkage

For IBM jobs, the executor stamps the Circuit UID into the provider job tags.

### Reverse linkage

The controller stores the provider job ID in:

- `Circuit.status.providerJobId`
- `qcc_circuit_info{provider_job_id="..."}`
- `qcc_circuits_total{provider_job_id="..."}`

This makes both Kubernetes-side and provider-side lookup practical.

## What Is Not There Yet

### No real traces

The repository has tracing scaffolding, but the useful end-to-end trace path is not active yet.

### No executor-side telemetry

The Python executor currently does not emit its own OpenTelemetry metrics or traces.

### No Kubernetes Events

The code does not currently use an event recorder. Operational visibility is therefore status-and-metrics-centric.

### No default controller-runtime metrics scrape

The default deployment enables the controller's metrics endpoint, but the direct Prometheus scrape route for `controller_runtime_*`, `go_*`, and `process_*` is intentionally not wired in by default.

## Practical Reading Order

When debugging a Circuit today, the most useful sequence is:

1. `qcc get circuit <name>`
2. `kubectl get circuit <name> -o yaml`
3. `qcc get circuit <name> --qasm`, `--draw`, or `--schedule` when relevant
4. Grafana dashboards for trend and comparison views
5. controller and executor logs

## Debugging Playbook

### A Circuit never leaves `Pending`

Check:

- is the controller running?
- did you apply any QPUs?
- does the namespace contain the `Circuit` you think it does?

### A Circuit fails with `NoEligibleBackend`

Check:

- `kubectl get qpus`
- `status.availability`
- selector mismatch on provider/backend/kind/minQubits
- whether you assumed `allowedQPURefs` or `region` are enforced

### A Circuit fails at submission on IBM

Check:

- executor deployment env vars / secret
- executor logs
- whether the IBM QPU was marked available optimistically even though probing failed

### A queued hardware run disappears after executor restart

This is a known limitation of the current implementation. The async task registry is in-memory only.
