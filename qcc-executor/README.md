# qcc-executor

`qcc-executor` is the Python execution service behind the Go controller.
It owns the Qiskit-heavy work: source loading, conversion, drawing, scheduling,
transpilation, adapter dispatch, and provider submission, polling, and
result retrieval. The controller reaches it over gRPC, and it writes no
Kubernetes objects itself.

For how it fits the rest of the system, see the
[architecture](../docs/architecture.md#qcc-executor). For the RPC surface,
see the [API reference](../docs/api.md#the-executor-grpc-contract). For
configuration and credentials, see the
[operations guide](../docs/operations.md#configuration-reference).

## Working in this directory

```sh
uv sync
uv run pytest -v
uv run qcc-executor
```

`python -m qcc_executor` is equivalent. The default bind address is
`0.0.0.0:9000`.

The package lives in `src/qcc_executor/`: `server.py` binds the gRPC
server, `servicer.py` implements the eight RPCs, `qiskit_io.py` handles
source loading and conversion, `adapters/` holds the provider adapters,
and `proto/` holds the generated protobuf stubs. Regenerate the stubs
from the repository root with `make proto`, and never edit them by hand.

## Adding an adapter

The six-method adapter contract and the steps to add a provider are in the
[adapter guide](../docs/engineering.md#adding-a-provider-adapter). Two
adapters are registered today, `AerAdapter` for the `local` provider and
`IBMAdapter` for `ibm`.

## Sharp edges

IBM credentials come from executor environment variables rather than from
`QPU.spec.access.credentialSecretRef`. Async task handles live in memory,
so hardware jobs do not survive an executor restart. The executor exports
no OpenTelemetry metrics or traces yet. The
[implementation status](../docs/status.md) tracks
all three.
