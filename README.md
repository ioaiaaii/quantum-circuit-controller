<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./docs/assets/images/readme-banner.svg">
  <source media="(prefers-color-scheme: light)" srcset="./docs/assets/images/readme-banner.svg">
  <img alt="QCC" src="./docs/assets/images/readme-banner.svg">
</picture>

# Quantum Circuit Controller

[![CI](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/ci.yml/badge.svg)](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/ci.yml)
[![E2E](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/test-e2e.yml/badge.svg)](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/test-e2e.yml)
[![Proto](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/proto.yml/badge.svg)](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/proto.yml)
[![Docs](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/docs.yml/badge.svg)](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/docs.yml)
[![Analysis](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/analysis.yml/badge.svg)](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/analysis.yml)
[![CD](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/cd.yml/badge.svg)](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/cd.yml)
[![Release](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/release.yml/badge.svg)](https://github.com/ioaiaaii/quantum-circuit-controller/actions/workflows/release.yml)

[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](./LICENSE)
[![DOI](https://zenodo.org/badge/DOI/10.5281/zenodo.21906210.svg)](https://doi.org/10.5281/zenodo.21906210)

QCC is a Kubernetes operator for hybrid quantum-classical execution. Quantum circuits and backends (QPUs) are modeled as custom resources. QCC registers and monitors QPU resources, and manages the orchestration of Circuit execution along with its observability instrumentation.

<img alt="Terminal session: kubectl get qpu listing simulators and IBM hardware with calibration, qcc get qpu on a real backend, the Shor circuit drawn, an execution on a fake-brisbane snapshot, a detached submission to ibm-kingston, and the result card with transpiled shape, error-exposure verdict, and measured outcomes" src="./docs/assets/figures/qcc-demo.gif" width="864">

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

## Getting Started

Start with the [Getting Started Kit](./docs/getting-started.md) to deploy QCC, run circuit and to get familiar with the system. Then check the [demonstration](./docs/demonstration.md) for the whole platform exercised on simulators and on real IBM hardware.

Further readings:

- [Architecture](./docs/architecture.md) covers the design and the QCC reference relative to QCSC, and QRMI.
- [API](./docs/api.md), [CLI](./docs/cli.md), and
  [metrics](./docs/observability.md) are the reference surfaces.
- [Operations](./docs/operations.md) covers deployment and troubleshooting,
  [engineering](./docs/engineering.md) covers the code and its internals,
  and [releasing](./docs/releasing.md) covers the branching model and the
  release runbook.

## Contributing

Contributions are welcome. Planned work is tracked in
[GitHub Projects](https://github.com/ioaiaaii/quantum-circuit-controller/projects), and [CONTRIBUTING.md](./CONTRIBUTING.md) covers the internals, the conventions, and the review process.

## Citation

QCC is the artifact of the MSc thesis [*Interface between Quantum and Classical Computers*](https://ioaiaaii.github.io/project/msc-thesis/),
Democritus University of Thrace, 2026.
Please use the *Cite this repository* button above, to generate it in APA or BibTex format.

## License

QCC is licensed under the [Apache License 2.0](./LICENSE).
