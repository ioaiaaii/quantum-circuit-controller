# Thesis demonstration kit

The manifests and algorithm sources behind the
[demonstration](../../docs/demonstration.md). Two workloads, Deutsch and
Shor, run against a fixed backend so that circuit depth is the variable
that separates the outcomes.

The kit is tooling rather than an algorithm contribution. It shows how to
submit a circuit, select a backend, read the calibration-aware score,
tune the run through the Circuit resource, and watch the metrics move.

## Layout

```
thesis/
├── algorithms/               # the quantum programs
│   ├── deutsch.qasm          #   OpenQASM 3, hand-written, deterministic
│   └── shor.py               #   Qiskit, period-finding for N=15 with a=4
├── qpus/
│   ├── fake-brisbane.yaml    # Eagle r3 snapshot
│   └── fake-fez.yaml         # Heron r2 snapshot, sibling of live ibm-fez
├── circuits/                 # one Circuit per run configuration
│   ├── deutsch.yaml          #   fake_brisbane, level 3
│   ├── shor-baseline.yaml    #   fake_brisbane, level 1, no passthrough
│   ├── shor-v2.yaml          #   ibm-kingston, level 3
│   └── shor-v3.yaml          #   ibm-kingston, level 3 plus scheduling_method alap
└── render.py                 # regenerates circuits/*.yaml from algorithms/
```

An *algorithm* file is the quantum program. A *Circuit* is the Kubernetes
resource that says to run that source, on that backend, with those
options. The manifests inline the source in `spec.source.body` because
the API has no ConfigMap-reference field, so each one carries a verbatim
copy produced by `render.py`. After editing an algorithm, re-run
`python3 render.py` to refresh the manifests.

## Running the kit

Register the backends first:

```bash
kubectl apply -f examples/thesis/qpus/
```

The CLI covers the untuned runs:

```bash
qcc run examples/thesis/algorithms/deutsch.qasm --backend fake_brisbane --shots 4096
qcc run examples/thesis/algorithms/shor.py --backend fake_brisbane --shots 4096
```

The Tier-2 runs need manifests, because the `transpile` and `execute`
passthrough blocks have no CLI flags. Tier 2 is described in the
[API reference](../../docs/api.md#the-two-tier-schema):

```bash
kubectl apply -f examples/thesis/circuits/shor-baseline.yaml
kubectl apply -f examples/thesis/circuits/shor-v2.yaml
kubectl apply -f examples/thesis/circuits/shor-v3.yaml
```

Read any run back with:

```bash
qcc get circuit shor-baseline
```

The hardware manifests target `ibm-kingston` and need IBM Quantum
credentials. [Getting started](../../docs/getting-started.md) covers that
setup.

## Results

The measured arc across the ideal simulator, three Heron r2 backends, and
the two tuned runs is in the
[demonstration](../../docs/demonstration.md#tune-the-transpiler), together
with the success metric and the screenshots for each run.

Deutsch is the contrast case. It transpiles to depth 6 or so, holds about
97% on the correct peak, and shows that a shallow circuit is unaffected by
the calibration age that degrades Shor.

Transpiler and simulator seeds are pinned to 42 in the manifests, so the
simulator runs reproduce exactly.

## Capturing figures

Terminal figures in the demonstration were rendered with
[freeze](https://github.com/charmbracelet/freeze), which is not part of
the pinned toolchain. Printing the command as the first line puts it
above the output:

```bash
freeze -t github --window \
  -x "printf '\033[36m❯\033[0m qcc get circuit shor-v3\n'; qcc get circuit shor-v3" \
  -o shor-v3.png
```

Save figures as lossless WebP and reference them with an explicit width,
following the convention in `docs/assets/figures/`.
