# qcc-executor

`qcc-executor` is the Python execution service used by the Go controller.

It owns the Qiskit-heavy parts of QCC:

- OpenQASM 3 and Qiskit-Python source loading
- Qiskit-to-OpenQASM conversion
- ASCII drawing
- backend-specific scheduling
- transpilation
- adapter dispatch
- provider submission, polling, and result retrieval

The controller reaches it over the in-cluster gRPC service configured by
`QCC_EXECUTOR_ADDR`. The executor does not write Kubernetes objects directly.

## Local Development

```sh
uv sync
uv run pytest -v
uv run qcc-executor
```

`python -m qcc_executor` also works. The default bind address is
`0.0.0.0:9000`.

## Adapters

| provider | adapter | status |
|---|---|---|
| `local` or empty | `AerAdapter` | Qiskit Aer in-process execution, `fake_*` snapshots, and method-pinned variants such as `aer_statevector` |
| `ibm` | `IBMAdapter` | IBM Quantum through `qiskit-ibm-runtime`; async submit/watch/fetch path |
| future | generic `QiskitProviderAdapter` | possible path for Qiskit-provider plugins such as Amazon Braket through `qiskit-braket-provider` |
| future | OpenQASM runtime adapter | possible path for runtimes that accept OpenQASM payloads but are not exposed as Qiskit providers |
| future | substrate-specific adapter | possible path for QRMI, CUDA-Q, or vendor-direct integrations |

Future rows are design direction only. They are not registered runtime adapters
today. A new provider is not just a new string: it must implement backend
inspection, transpilation or capability handling, submit/watch/fetch semantics,
error mapping, and result normalization.

## Environment

| Variable | Default | Purpose |
|---|---|---|
| `QCC_EXECUTOR_ADDR` | `0.0.0.0:9000` | gRPC bind address |
| `QCC_EXECUTOR_WORKERS` | `8` | thread-pool size |
| `QCC_EXECUTOR_LOG_LEVEL` | `INFO` | Python logging level |
| `QISKIT_IBM_TOKEN` | unset | required by `IBMAdapter` |
| `QISKIT_IBM_CHANNEL` | `ibm_quantum_platform` | optional IBM Quantum channel override |

## Runtime Limits

- IBM credentials are read from executor environment variables, not from `QPU.spec.access.credentialSecretRef`.
- Async task handles are stored in memory, so hardware jobs are not restart-tolerant across executor restarts.
- The executor does not currently emit OpenTelemetry metrics or traces.
