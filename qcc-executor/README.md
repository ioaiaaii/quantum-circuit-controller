# qcc-executor

Python sibling container of `qcc-controller`. Holds the Qiskit dependency,
vendor adapters, and (in later milestones) the backend-selection chain
behind a gRPC interface.

## Relationship to QRMI

`qcc-executor` is the K8s-facing service inside QCC's Pod. **Vendor
abstraction is delegated to [QRMI](https://github.com/qiskit-community/qrmi)**
(Bacher et al., 2025) via a `QRMIAdapter` — currently a stub, fully wired in
M2. This positions QCC as the operator-pattern Kubernetes analog of what
Slurm + SPANK + QRMI does on HPC.

The adapter registry (`qcc_executor/adapters/`) carries the legacy IBMAdapter
and the AerAdapter (no QRMI equivalent for in-process simulators) alongside
the future QRMIAdapter.

## Local development

```sh
uv sync
uv run pytest -v
uv run qcc-executor   # python -m qcc_executor also works; listens on 0.0.0.0:9000
```

## Adapters

| provider | adapter | status |
|---|---|---|
| `local`  (or empty) | `AerAdapter` | in-process Qiskit Aer simulator, no credentials |
| `ibm`               | `IBMAdapter` | direct `qiskit-ibm-runtime`; needs `QISKIT_IBM_TOKEN` |
| `qrmi`              | `QRMIAdapter` | **WIP** — community QRMI library; arrives in M2 |

## Environment

| Variable | Default | Purpose |
|---|---|---|
| `QCC_EXECUTOR_ADDR`      | `0.0.0.0:9000` | gRPC bind address |
| `QCC_EXECUTOR_WORKERS`   | `8`            | thread-pool size |
| `QCC_EXECUTOR_LOG_LEVEL` | `INFO`         | Python logging level |
| `QISKIT_IBM_TOKEN`       | —              | required by `IBMAdapter` (and future `QRMIAdapter`) |
