# Engineering

How QCC is built and how to work on it, organized by engineering domain:
build and test, packaging, release engineering, performance, security, and
the QCC internals themselves. The SRE principles behind these choices are
mapped in [SRE principles](./architecture.md#sre-principles-mapped); this
document is the concrete practice.

## Repository map

The directories you are most likely to touch, and what lives in each.

```
api/v1alpha1/            Circuit + QPU types, conditions, labels, CRD markers
cmd/qcc-controller/      manager entrypoint: OTel setup, reconciler wiring
cmd/qcc/                 CLI entrypoint + cobra command tree
internal/controller/     CircuitReconciler, QPUReconciler + envtest suites
internal/executor/       controller-side gRPC client (domain types, not protobuf)
internal/observability/  OTel SDK: resource, meter provider, metrics, tracer stub
internal/cli/            kubeclient + render (styles, spinner, tables, histogram)
qcc-executor/            Python service: servicer, adapters, qiskit_io, tests
proto/qcc/executor/v1/   the gRPC contract, single source of truth
gen/proto/               generated Go stubs (committed)
test/e2e/                end-to-end suite, build tag `e2e`
config/                  kustomize tree: CRDs, RBAC, manager + executor, samples
deploy/                  kind + observability platform values, Grafana dashboards
build/                   Dockerfiles, CI configs, changelog templates
hack/                    developer scripts and boilerplate
examples/thesis/         the Shor evaluation kit (algorithms, circuits, render.py)
```

## Build and test

Go is pinned by the `toolchain` directive
in `go.mod`; any recent Go auto-fetches it. Everything else (`kubectl`,
`kind`, `helm`, `kustomize`, `kubebuilder`, `buf`, `golangci-lint`,
`lychee`, `controller-gen`, `setup-envtest`, `python`, `uv`) is pinned in
`.mise.toml` and provisioned by `make tools-install`. CI uses the same
pins through `mise-action`, so local and CI toolchains cannot diverge.
Python dependencies are locked in `uv.lock` and installed with
`--frozen`, so the simulator results the evaluation rests on are not
exposed to silent dependency drift.

The everyday targets:

```bash
make build           # controller into dist/qcc-controller
make qcc-build       # CLI into dist/qcc (version from git describe)
make docker-build executor-build   # both images
make test            # Go unit + envtest suites
make executor-test   # Python: uv run pytest
make lint            # custom golangci-lint;  make executor-lint runs ruff
make docs-check      # link + anchor check over all markdown
make test-e2e        # full deploy against a dedicated kind cluster
```

Generated code is committed. Protobuf stubs (`gen/proto/`,
`qcc-executor/src/qcc_executor/proto/`) and controller-gen outputs (CRDs,
RBAC, DeepCopy) live in the tree: contract changes appear in diffs, and
consumers build without the generator toolchain. Never hand-edit
generated files; regenerate with `make proto-generate` and
`make manifests generate`.

Linting is a build product of its own. Go uses a custom golangci-lint
binary (built by `make lint-build` from `.custom-gcl.yml`) that bundles
project plugins such as logcheck. Python is ruff-clean with generated
code excluded.

What the suites cover:

- `internal/controller/` runs against envtest, a real kube-apiserver and
  etcd: the reconcilers driven through the full phase machine with a fake
  executor; selection semantics, artifact ownership, condition
  transitions, and the terminal-versus-transient error split.
- `qcc-executor/tests/` runs in-process gRPC round-trips with a real
  `AerAdapter`: run, convert, draw, the async lifecycle, and Tier-2
  passthrough reproducibility (seeded runs must produce identical
  counts).
- `test/e2e/` (build tag `e2e`) deploys the real images into an isolated
  kind cluster: a manager-up and metrics-served smoke test.

The tightest local loop runs the controller and executor as local
processes:

```bash
make dev-up                                            # kind + platform + CRDs
uv run --project qcc-executor python -m qcc_executor   # terminal 1: executor on :9000
make run                                               # terminal 2: controller
```

The controller dials `127.0.0.1:9000` when `QCC_EXECUTOR_ADDR` is unset,
which is exactly the local case. For an in-cluster loop, `make dist-up`
rebuilds and redeploys both images.

## Package

The controller builds to `gcr.io/distroless/static:nonroot`:
a static Go binary, no shell, no package manager. The executor is
`python:3.12-slim` with a dedicated non-root user and a `uv`-built venv,
dependency layer cached separately from source. Both deployments satisfy
the restricted Pod Security Standard (non-root, no privilege escalation,
capabilities dropped, seccomp RuntimeDefault).

Kustomize composes the deployment: `config/default` gathers CRDs,
RBAC, the controller, the executor, and the metrics Service under one
name prefix, the kubebuilder-standard layout. Helm packaging and
published images are roadmap items; today the install path is
build-and-load ([tutorial](./getting-started.md)).

## Release engineering

The `v1.0.x` line is the frozen thesis artifact (the
manuscript cites v1.0.0); new work targets `v1.1.0`. Interfaces
(`qcc.io/v1alpha1`, the executor gRPC contract, the `qcc_*` metrics
specification) are stable within a minor line but carry no compatibility
promise yet.

Commits follow Conventional Commits (`feat:`, `fix:`,
`docs:`, `chore:`), enforced by the commitlint configuration under
`build/changelog/`, which also carries the changelog templates used at
release time.

The gRPC contract is guarded by `buf`:
lint, format, and `make proto-breaking` against `main`, so the wire
contract gets the same review discipline as the CRD schema. CRD changes
stay additive within `v1alpha1`; any schema-only (unenforced) field is
documented as such in [API reference](./api.md). To change either contract:

```bash
# gRPC
make proto-lint proto-format proto-generate
make proto-breaking
# CRDs
make manifests generate && make install && make test
```

Every workflow pins its actions by commit SHA and runs with
least-privilege permissions:

| Workflow | Runs |
|---|---|
| `test.yml` | `make test` (envtest suites) |
| `test-e2e.yml` | `make test-e2e` (kind smoke) |
| `lint.yml` | `make lint-config` + `make lint` |
| `executor.yml` | `uv sync --frozen`, `ruff`, `pytest` |
| `proto.yml` | `buf lint` + format check |
| `docs.yml` | `make docs-check` (markdown-triggered) |

There is no image-publish or release workflow yet; images build locally.
GHCR publishing, vulnerability scanning, and release automation are
roadmap items.

## Performance

The reconcile hot path carries almost no telemetry work. Only two event
instruments record inside reconcile, `qcc_circuits_total` and the
phase-duration histogram, and only at transitions. Everything else is an
observable gauge read from the controller-runtime informer cache once per
export cycle, one `List()` per resource family, so the scrape path never
touches the API server. The cost is staleness: a gauge can lag a status
change by up to one cycle, thirty seconds by default.

Metric cardinality is budgeted per label rather than left to grow.
Per-Circuit labels such as `uid` and `provider_job_id` enrich existing
series one-to-one. The `bitstring` dimension grows as 2^q for a q-qubit
readout, which is fine at small-circuit scale and flagged for
re-evaluation beyond it. User labels reach metrics only through the
`qcc.io/*` allowlist, never wholesale, because forwarding arbitrary
labels would hand cardinality control to whoever creates Circuits.

Asynchronous polling reads one `WatchTask` frame per reconcile and closes
the stream. That matches the reconcile cadence and keeps streams
short-lived, which sidecars and service meshes tolerate far better than
held connections, at the cost of a little more chatter.

The executor's gRPC thread pool defaults to eight workers, set by
`QCC_EXECUTOR_WORKERS`, and the synchronous simulator path occupies one
for the duration of a run. Aer statevector memory grows exponentially
with qubit count, so sizing guidance lives under
[scaling and sizing](./operations.md#scaling-and-sizing). Selection is
O(registered QPUs) per Circuit, which is negligible at registry scale.

## Security

On the supply-chain side, actions are pinned by commit SHA, and toolchain
and dependency versions are pinned in `.mise.toml`, `go.mod`, and
`uv.lock`. Generated stubs are committed and reviewed rather than produced
at build time, and CI runs with `permissions: contents: read`.

The runtime posture, both the pod hardening and the single-tenant
assumptions behind it, is in
[security posture](./operations.md#security-posture). Vulnerability
reporting goes through [SECURITY.md](../SECURITY.md).

## QCC internals

The code-level rules and mechanisms specific to this codebase.

### The one error rule

Every error is classified exactly once, at the executor-client boundary
(`internal/executor/client.go`):

An `*executor.TaskError` carries a Circuit condition reason such as
`TranspilationFailed` or `NoEligibleBackend`, and it is terminal: the
reconciler marks the Circuit `Failed` and stops. Anything else is a
transport failure and therefore transient: log it, requeue, stay in the
current phase.

The Python side upholds the rule by reporting adapter failures in-band
(`status=FAILED` plus `error_reason`) instead of letting exceptions
become transport errors. No reconciler call-site carries its own retry
policy. Never collapse the two classes.

### Controller notes

`Reconcile` handles one phase per pass. It switches on `status.phase`,
and each handler does one thing, patches status, and requeues; a deferred
hook records the transition metrics only when the phase actually changed.

`qcc.io/run-index` is computed as max(siblings)+1 without a transaction,
so two concurrent reconciles can draw the same ordinal. That is accepted
rather than overlooked: an atomic counter resource costs more than a
duplicate ordinal does at this scale.

Probe failures are non-fatal. The QPU still becomes `Available` with empty
calibration, and the next reconcile retries.

Decision logic stays in pure functions, including selection filtering,
status derivation, and metric attribute building. Values in, values out,
no client and no clock, which is what lets them be tested without mocks.

### Executor notes

The asynchronous task registry is a lock-guarded in-memory dict mapping a
task ID to its adapter and job handle, and `FetchTaskResult` pops the
entry once it has delivered counts. A restart therefore loses in-flight
tasks, and replicas cannot share the registry. Both are accepted
proof-of-concept scope, because the Circuit resource is the durable
record.

`AerAdapter` resolves three backend families from the name it is given.
An `aer_<method>` name pins the simulation method, which beats Aer's
circuit-dependent automatic selection for reproducibility. A `fake_*`
name loads a calibration snapshot through `FakeProviderForBackendV2`.
Anything else falls back to generic noise-free Aer. The method lives in
the resolver rather than a CRD field, because provider construction
belongs at the adapter boundary.

`IBMAdapter` is written to survive SDK drift. Counts extraction probes the
SamplerV2 DataBin for the first attribute exposing `get_counts()`, the
`usage()` reading tries three known shapes, and the
`qcc.circuit.uid:<uid>` job tag is stamped best-effort so it can never
fail a submission.

The wire boundary fixes two type mismatches. Protobuf Struct numbers are
double-only, so a `seed_transpiler: 7` written in YAML arrives as `7.0`
and Qiskit rejects it; the servicer coerces whole-number floats back to
integers while preserving bools. It also strips `shots` from the Tier-2
`execute` block, leaving the Tier-1 field as the single source of truth.

`dump_qasm` decomposes five rounds before export, because Qiskit's
OpenQASM 3 exporter rejects library subroutines such as QFT. It is a no-op
on circuits that are already primitive.

Running `exec()` over a `format: qiskit` source is a trust decision rather
than an oversight. Circuit sources are code by nature, and the executor
pod is the sandbox the submitter is already trusted with. Multi-tenant use
needs real isolation first.

### Conventions

Comments explain why, not what: cardinality budgets, absence-versus-zero
conventions, upstream API quirks. A mechanism without its reasoning is
half done. Keep new decision logic in pure functions, and keep the
terminal-versus-transient split sacred.

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
   passthrough; hand them verbatim to your SDK. `shots` always wins over
   a `shots` key in options. If the vendor has a job-tag surface, stamp
   `qcc.circuit.uid:<circuit_uid>` best-effort.
5. Register a `QPU` with `spec.provider: myvendor` and run the
   [demonstration flow](./demonstration.md) against it. `inspect()` fills
   what it can and leaves the rest zero; the controller treats absence as
   "skip", never as "perfect".

Three adapter categories fit this contract: Qiskit-provider wrappers
(Braket, IonQ, IQM through their Qiskit plugins), OpenQASM runtime
adapters for non-Qiskit services, and alternative substrates (QRMI,
CUDA-Q).

## Decision ledger

Each load-bearing decision, the domain it belongs to, and the cost
accepted with it.

| Decision | Domain | Accepted cost |
|---|---|---|
| Controller/executor split over gRPC | internals | two deployables, a contract to version |
| Kubebuilder + controller-runtime + envtest | build and test | scaffold conventions to honor |
| Protobuf + buf, stubs committed | release engineering | regeneration discipline |
| Executor owns all Qiskit | internals | every new capability needs an RPC |
| Artifacts in owned ConfigMaps | internals | one indirection for readers |
| Results inline on status | internals | revisit for large readouts |
| First-match selection | internals | no quality-based placement yet |
| Tier-2 passthrough dicts | release engineering | no server-side key validation |
| One-frame WatchTask | performance | chattier than a held stream |
| In-memory task registry | internals | restart loses in-flight watches |
| Metrics from informer cache | performance | up to one export cycle of lag |
| No tracing in v1.0 | internals | no flame-graph view |
| `exec()` for Qiskit sources | security | unsafe multi-tenant as-is |
| mise + go.mod toolchain pinning | build and test | one bootstrap tool to install |
| Distroless, PSS-restricted images | security | no shell for in-container debugging |
| Kustomize, no Helm yet | package | no values-driven multi-env installs |

## Sharp edges

Operational consequences live in
[known limitations](./operations.md#known-limitations). Code-level ones:
`IBMAdapter.poll()` never returns `PENDING`, so the servicer's
queue-position message path is currently unreachable; multi-register
circuits collapse to one register's counts.
