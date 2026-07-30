# Engineering

How QCC is built and how to work on it. The codebase is organized around six
engineering principles borrowed from SRE practice; every tooling choice and
convention below is filed under the principle it serves.

## Repository map

```
api/v1alpha1/            Circuit + QPU types, conditions, labels, CRD markers
cmd/qcc-controller/      manager entrypoint: OTel setup, reconciler wiring
cmd/qcc/                 CLI entrypoint + cobra command tree
internal/controller/     CircuitReconciler, QPUReconciler + envtest suites
internal/executor/       controller-side gRPC client (domain types, not protobuf)
internal/observability/  OTel SDK: resource, meter provider, metrics, tracer stub
internal/cli/            kubeclient + render (styles, spinner, tables, histogram)
proto/qcc/executor/v1/   the gRPC contract, single source of truth
gen/proto/               generated Go stubs (committed)
qcc-executor/            Python service: servicer, adapters, qiskit_io, tests
config/                  kustomize tree: CRDs, RBAC, manager + executor, samples
deploy/                  kind + observability platform values, Grafana dashboards
examples/thesis/         the Shor evaluation kit (algorithms, circuits, render.py)
```

## Principles

### 1. Reproducibility

A run that cannot be reproduced cannot be evaluated. Everything that could
drift is pinned.

- Go is pinned by the `toolchain` directive in `go.mod`; any recent Go
  auto-fetches it. Every other tool (`kubectl`, `kind`, `helm`, `kustomize`,
  `kubebuilder`, `buf`, `golangci-lint`, `controller-gen`, `setup-envtest`,
  `python`, `uv`) is pinned in `.mise.toml` and provisioned by
  `make tools-install`. CI uses the same pins through `mise-action`, so
  local and CI toolchains cannot diverge.
- The executor's dependency graph (Qiskit, Aer, `qiskit-ibm-runtime`,
  grpcio) is locked in `uv.lock` and installed with `--frozen` in CI and in
  the image. The simulator results the evaluation rests on are not exposed
  to silent dependency drift.
- Generated protobuf stubs are committed (`gen/proto/`,
  `qcc-executor/src/qcc_executor/proto/`). Contract changes appear in
  diffs, and consumers build without the protoc toolchain.
- Tests that involve randomness pin seeds; the Tier-2 test asserts that two
  seeded runs produce identical counts.

### 2. Reliability

Failure paths are designed as deliberately as success paths.

- **The one error rule.** Every error is classified exactly once, at the
  executor-client boundary (`internal/executor/client.go`). A
  `*executor.TaskError` carries a Circuit condition reason
  (`TranspilationFailed`, `NoEligibleBackend`) and is terminal: the
  reconciler marks the Circuit `Failed` and stops. Anything else is a
  transport failure and is transient: log, requeue, stay in phase. The
  Python side upholds the rule by reporting adapter failures in-band
  (`status=FAILED` plus `error_reason`) instead of letting exceptions
  become transport errors. No reconciler call-site carries its own retry
  policy. Never collapse the two classes.
- **Idempotency before side effects.** The controller persists
  `phase=Submitting` before the external submit call, and every submission
  carries an idempotency key built from the Circuit UID and observed
  generation. A controller restart cannot double-submit silently.
- **Graceful degradation.** A failed backend probe still leaves the QPU
  usable (empty calibration, retried next reconcile). The platform runs
  without the observability stack; the SDK can be disabled with one env
  var.
- **Bounded interactions.** Async polling reads one `WatchTask` frame per
  reconcile and closes the stream: it matches the reconcile cadence and
  keeps gRPC streams short-lived, which sidecars and meshes tolerate.

### 3. Boundaries and simplicity

The smallest number of moving parts, each with one job.

- The controller/executor split keeps Go operator idioms and the Python
  quantum stack each in their native ecosystem, joined by one typed
  protobuf contract. The controller and CLI link no quantum SDK.
- The operator is a standard kubebuilder scaffold: CRDs generated from Go
  types by `controller-gen` markers, RBAC from annotations, reconcilers on
  controller-runtime's manager (informer caches, leader election, probes
  for free). Platform engineers meet the conventions they already know.
- Decision logic lives in pure functions. Selection filtering, status
  derivation, and metric attribute builders take values and return values,
  no client, no clock, so the semantics sit in plain unit tests.
- Comments explain why, not what: cardinality budgets, absence-versus-zero
  conventions, upstream API quirks. A mechanism without its reasoning is
  half done.

### 4. Security and least privilege

- The controller image is distroless (`static:nonroot`): a static Go
  binary, no shell, no package manager. The executor runs as a dedicated
  non-root user on `python:3.12-slim`. Both deployments satisfy the
  restricted Pod Security Standard.
- RBAC is scoped to the QCC resources plus the ConfigMaps and Events the
  controller owns.
- CI workflows pin actions by commit SHA and run with
  `permissions: contents: read`.
- The deliberate trust decisions (plaintext in-cluster gRPC, `exec()` of
  circuit sources, one shared IBM token) are documented as posture, not
  hidden: [operations.md](./operations.md#security-posture).

### 5. Observability discipline

- Resource-state metrics are observable gauges read from the informer
  cache once per export cycle; the scrape path never touches the API
  server. Lifecycle events are synchronous counters and histograms at the
  transition.
- Cardinality is budgeted per label: per-Circuit labels enrich existing
  series one-to-one, the 2^q `bitstring` dimension is flagged for
  re-evaluation beyond small circuits, and user labels reach metrics only
  through the `qcc.io/*` allowlist.
- The tracer provider is a wired no-op (propagator set, provider global,
  no span processor). Enabling tracing is one exporter change; exemplars
  on the existing histograms come free.

### 6. Change safety

- The gRPC contract is guarded by `buf`: lint, format, and
  `make proto-breaking` against `main`. The wire contract gets the same
  review discipline as the CRD schema.
- CRD changes stay additive within `v1alpha1`; schema-only fields are
  documented as such in [api.md](./api.md).
- Go linting runs a custom golangci-lint binary with project plugins
  (built by `make lint-build` from `.custom-gcl.yml`); Python is
  ruff-clean with generated code excluded.

## Component notes

Facts about the implementation that do not follow from the architecture and
save the next engineer an afternoon.

**Controller.** `Reconcile` switches on `status.phase`; each handler does
one thing, patches status, requeues. A deferred hook compares the phase
before and after dispatch and records the transition metrics.
`qcc.io/run-index` is computed as max(siblings)+1 without a transaction:
two concurrent reconciles can draw the same ordinal, accepted because an
atomic counter costs more than a duplicate ordinal at this scale.

**Executor.** The async task registry is a lock-guarded dict
(`task_id` to adapter and job handle) in process memory;
`FetchTaskResult` pops the entry after delivering counts. `AerAdapter`
resolves three backend families from the name: `aer_<method>` for a
method-pinned simulator (pinning beats Aer's circuit-dependent automatic
selection for reproducibility), `fake_*` for calibration snapshots, and
anything else for generic noise-free Aer. `IBMAdapter` defends against SDK
drift: counts extraction probes the SamplerV2 DataBin for the first
attribute with `get_counts()`, `usage()` extraction tries three shapes,
and job-tag stamping can never fail a submission. At the wire boundary the
servicer coerces whole-number floats back to ints (protobuf Struct numbers
are double-only and Qiskit rejects `7.0` where it expects `7`) and strips
`shots` from Tier-2 `execute` so the Tier-1 field stays the single source
of truth. `dump_qasm` decomposes five rounds before export because
Qiskit's QASM 3 exporter rejects library subroutines such as QFT.

## Decision ledger

Each load-bearing decision, the principle it serves, and the cost accepted
with it.

| Decision | Principle | Accepted cost |
|---|---|---|
| Controller/executor split over gRPC | boundaries | two deployables, a contract to version |
| Kubebuilder + controller-runtime + envtest | simplicity, reproducibility | scaffold conventions to honor |
| Protobuf + buf, stubs committed | change safety, reproducibility | regeneration discipline |
| Executor owns all Qiskit | boundaries | every new capability needs an RPC |
| Artifacts in owned ConfigMaps | reliability (etcd size safety) | one indirection for readers |
| Results inline on status | simplicity | revisit for large readouts |
| First-match selection | honesty about scope | no quality-based placement yet |
| Tier-2 passthrough dicts | change safety (no CRD churn) | no server-side key validation |
| One-frame WatchTask | reliability | chattier than a held stream |
| In-memory task registry | simplicity (PoC scope) | restart loses in-flight watches |
| Metrics from informer cache | observability discipline | up to one export cycle of lag |
| No tracing in v1.0 | simplicity (metrics suffice for R4) | no flame-graph view |
| `exec()` for Qiskit sources | boundaries (sources are code) | unsafe multi-tenant as-is |
| mise + go.mod toolchain pinning | reproducibility | one bootstrap tool to install |
| Distroless, PSS-restricted images | least privilege | no shell for in-container debugging |
| Kustomize, no Helm yet | simplicity | no values-driven multi-env installs |

## Working on QCC

### Build, test, run

```bash
make tools-install   # provision the pinned toolchain (mise)
make build           # controller into dist/qcc-controller
make qcc-build       # CLI into dist/qcc (version from git describe)
make docker-build executor-build   # both images

make test            # Go unit + envtest suites
make executor-test   # Python: uv run pytest
make lint            # custom golangci-lint;  make executor-lint runs ruff
make test-e2e        # full deploy against a dedicated kind cluster
```

What the suites cover:

- `internal/controller/` runs against envtest, a real kube-apiserver and
  etcd: the reconcilers driven through the full phase machine with a fake
  executor; selection semantics, artifact ownership, condition
  transitions, and the terminal-versus-transient split.
- `qcc-executor/tests/` runs in-process gRPC round-trips with a real
  `AerAdapter`: run, convert, draw, the async lifecycle, and Tier-2
  passthrough reproducibility.
- `test/e2e/` (build tag `e2e`) deploys the real images into an isolated
  kind cluster: a manager-up and metrics-served smoke test.

Local dev loop, controller and executor as local processes:

```bash
make dev-up                                            # kind + platform + CRDs
uv run --project qcc-executor python -m qcc_executor   # terminal 1: executor on :9000
make run                                               # terminal 2: controller
```

The controller dials `127.0.0.1:9000` when `QCC_EXECUTOR_ADDR` is unset,
which is exactly the local case. For an in-cluster loop, `make dist-up`
rebuilds and redeploys both images.

### Adding a provider adapter

The modularity requirement (R3) in practice: one Python module plus one
registry entry. Controller, CRDs, CLI, and metrics stay untouched.

1. Implement the six-method contract
   (`qcc-executor/src/qcc_executor/adapters/base.py`):

```python
from .base import (Adapter, AdapterUnavailable, BackendMetadata,
                   CircuitSchedule, FetchResult, JobHandle, JobStatus,
                   TranspiledCircuit)

class MyVendorAdapter(Adapter):
    name = "myvendor"

    def __init__(self, backend_name: str | None = None) -> None:
        # Resolve credentials and backend HERE. If anything is missing,
        # raise AdapterUnavailable: the servicer turns it into a terminal
        # NoEligibleBackend instead of a retry loop.
        ...

    def transpile(self, qasm, target, options=None) -> TranspiledCircuit: ...
    def submit(self, circuit, shots, options=None, circuit_uid="") -> JobHandle: ...
    def poll(self, handle) -> JobStatus: ...            # PENDING/RUNNING/DONE/FAILED
    def fetch_result(self, handle) -> FetchResult: ...  # counts + usage_seconds
    def inspect(self) -> BackendMetadata: ...           # probe: no shots, no side effects
    def schedule(self, qasm, target) -> CircuitSchedule: ...
```

2. Register it in `adapters/__init__.py`:
   `_ADAPTERS["myvendor"] = MyVendorAdapter`.
3. Honor the error contract: constructor problems raise
   `AdapterUnavailable`; `transpile` exceptions surface as
   `TranspilationFailed`; `submit` and `fetch_result` exceptions as
   `ProviderSubmissionFailed`. Report failures by raising, never by
   returning partial results.
4. Forward, do not translate: the `options` dicts are the user's Tier-2
   passthrough; hand them verbatim to your SDK. `shots` always wins over a
   `shots` key in options. If the vendor has a job-tag surface, stamp
   `qcc.circuit.uid:<circuit_uid>` best-effort.
5. Register a `QPU` with `spec.provider: myvendor` and run the
   [demonstration flow](./demonstration.md) against it. `inspect()` fills
   what it can and leaves the rest zero; the controller treats absence as
   "skip", never as "perfect".

Three adapter categories fit this contract: Qiskit-provider wrappers
(Braket, IonQ, IQM through their Qiskit plugins), OpenQASM runtime
adapters for non-Qiskit services, and alternative substrates (QRMI,
CUDA-Q).

### Changing the contracts

CRDs: edit `api/v1alpha1/`, run `make manifests generate` (regenerates CRD
YAML, with `allowDangerousTypes=true` for the float64 calibration fields,
plus DeepCopy methods), then `make install` and `make test`.

gRPC: edit `proto/qcc/executor/v1/executor.proto`, run
`make proto-lint proto-format proto-generate`, and `make proto-breaking`
before merging. Both language stubs come from one generate run; never
hand-edit them.

### CI

Five workflows on every push and PR:

| Workflow | Runs |
|---|---|
| `test.yml` | `make test` (envtest suites) |
| `test-e2e.yml` | `make test-e2e` (kind smoke) |
| `lint.yml` | `make lint-config` + `make lint` |
| `executor.yml` | `uv sync --frozen`, `ruff`, `pytest` |
| `proto.yml` | `buf lint` + format check |

There is no image-publish or release workflow yet; images build locally.
Release automation is on the roadmap.

## Sharp edges

Operational consequences live in
[operations.md](./operations.md#known-limitations). Code-level ones:
`IBMAdapter.poll()` never returns `PENDING`, so the servicer's
queue-position message path is currently unreachable; multi-register
circuits collapse to one register's counts.
