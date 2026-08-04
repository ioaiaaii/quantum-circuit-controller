<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./docs/assets/images/readme-banner.svg">
  <source media="(prefers-color-scheme: light)" srcset="./docs/assets/images/readme-banner.svg">
  <img alt="QCC" src="./docs/assets/images/readme-banner.svg">
</picture>

# Quantum Circuit Controller

[![Tests](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/test.yml/badge.svg)](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/test.yml)
[![E2E](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/test-e2e.yml/badge.svg)](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/test-e2e.yml)
[![Executor](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/executor.yml/badge.svg)](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/executor.yml)
[![Lint](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/lint.yml/badge.svg)](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/lint.yml)
[![Proto](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/proto.yml/badge.svg)](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/proto.yml)
[![Docs](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/docs.yml/badge.svg)](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/docs.yml)

[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](./LICENSE)

QCC is a Kubernetes operator for hybrid quantum-classical execution. Quantum circuits and backends (QPUs) are modeled as custom resources. QCC registers and monitors QPU resources, and manages the orchestration of Circuit execution along with its observability instrumentation.


<img alt="Result card for shor-2vv42 on ibm-kingston: backend calibration context, transpiled depth 2048, error-exposure verdict, and outcomes 0000:41 and 1000:41 of 1024 shots" src="./docs/assets/figures/qcc-demo.gif">

<!-- 
```bash
qcc run shor.py --backend ibm-kingston --detach
qcc get circuit shor-2vv42
```

<img alt="Result card for shor-2vv42 on ibm-kingston: backend calibration context, transpiled depth 2048, error-exposure verdict, and outcomes 0000:41 and 1000:41 of 1024 shots" src="./docs/assets/figures/circuit_get_shor_kingstone_v1.webp" width="600">

*One execution under `qcc get circuit`: the calibration of the backend it
landed on, the transpiled shape, an error-budget verdict for the circuit
against that backend, and the measured outcome distribution. This is the
8.0% result described below.* -->

## Motivation

Quantum circuits executions are submitted through vendor SDKs, tracked in vendor
consoles, and scheduled on vendor backends, acting as a boundary between engineer and platform.

QCC applies cloud-native and SRE dicipline to provive the following features:

- Submit OpenQASM 3 or Qiskit Python as a Circuit and the execution
  becomes a Kubernetes object, carrying its phase, selected backend, and
  measured counts on `status`.
- Every registered QPU is probed the way a node exporter surfaces host
  facts, and each execution is scored against that live calibration
  before hardware time is spent.
- Executions emit `qcc_*` metrics, and the provider job ID resolves the
  Kubernetes resource and the vendor console to each other in both
  directions.
- The six-method adapter contract aims to formalize the provider seam the
  way [QRMI](https://arxiv.org/abs/2506.10052) does for resource
  management.
- Circuits take configuration in two tiers: the options QCC owns and
  validates (tier-1), and a passthrough tier (tier-2) for anything else the backend SDK accepts.

## Quickstart

Requires a container runtime, Go, and [mise](https://mise.jdx.dev/). The
local path runs on a laptop with kind:

```bash
make tools-install                    # pinned toolchain (kubectl, kind, helm, uv, ...)
make platform-up                      # kind cluster + Prometheus/Grafana/OTel
kubectl apply -f deploy/grafana/      # QCC dashboards
make dist-up                          # build images, load into kind, deploy QCC
kubectl apply -k config/samples/qpu/  # register simulators + IBM profiles
make qcc-build
./dist/qcc run examples/bell-state.qasm --backend aer-statevector
```

Full walkthrough, including real IBM hardware:
[getting-started](./docs/getting-started.md).

## Documentation

Start with the [tutorial](./docs/getting-started.md) to stand QCC up and
run a circuit, then the [demonstration](./docs/demonstration.md) for the
whole platform exercised on simulators and on real IBM hardware.

- [Architecture](./docs/architecture.md) covers the design, and where QCC
  sits relative to QCSC, Qubernetes, Qonductor, and QRMI.
- [API](./docs/api.md), [CLI](./docs/cli.md), and
  [metrics](./docs/observability.md) are the reference surfaces.
- [Operations](./docs/operations.md) covers deployment and troubleshooting,
  [engineering](./docs/engineering.md) covers the code and its internals,
  and [releasing](./docs/releasing.md) covers the branching model and the
  release runbook.
- [Implementation status](./docs/status.md) documents the implemented
  subset of the design, subsystem by subsystem.

## Status

Working proof of concept. The three public interfaces, `qcc.io/v1alpha1`,
the executor gRPC contract, and the metrics specification, are stable
within the `v1.x` line but carry no compatibility promise yet.

## Contributing

Contributions are welcome. Planned work is tracked in
[GitHub Projects](https://github.com/ioaiaaii/quantum-circuit-controller/projects),
and [CONTRIBUTING.md](./CONTRIBUTING.md) covers the setup, the conventions,
and the review process.

## Citation

QCC is the artifact of the MSc thesis
[*Interface between Quantum and Classical Computers*](https://ioaiaaii.github.io/project/msc-thesis/),
Democritus University of Thrace, 2026. Please use the citation metadata in [CITATION.cff](./CITATION.cff).

## License

QCC is licensed under the [Apache License 2.0](./LICENSE).
