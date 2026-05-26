#!/usr/bin/env python3
"""Regenerate circuits/*.yaml from the algorithm sources in algorithms/.

Circuit.spec.source.body must be inlined (the CRD has no ConfigMap-reference
field), so the manifests carry a verbatim copy of each algorithm.  Rather than
hand-paste — and risk the manifest drifting from the source — this script reads
each algorithm file and emits the manifests with the body as a YAML block
scalar.  Run it after editing anything in algorithms/:

    python3 render.py

The (algorithm, tier-1, tier-2) combinations below are exactly the ones whose
numbers are quoted in README.md; the pinned seeds keep those numbers
reproducible.
"""

from __future__ import annotations

import pathlib

import yaml

HERE = pathlib.Path(__file__).parent
ALG = HERE / "algorithms"
OUT = HERE / "circuits"

BACKEND = "fake_brisbane"  # all three demos share one backend (controlled variable)


def body(filename: str) -> str:
    return (ALG / filename).read_text()


def block_scalar(dumper: yaml.Dumper, data: str):
    """Render multi-line strings as readable `|` block scalars."""
    style = "|" if "\n" in data else None
    return dumper.represent_scalar("tag:yaml.org,2002:str", data, style=style)


yaml.add_representer(str, block_scalar)


def circuit(name: str, *, fmt: str, src: str, labels: dict,
            opt: int, transpile: dict | None = None,
            execute: dict | None = None, shots: int = 4096) -> dict:
    spec: dict = {
        "mode": "run",
        "shots": shots,
        "optimizationLevel": opt,
        "source": {"format": fmt, "body": body(src)},
        "backendSelector": {"backendName": BACKEND},
    }
    if transpile is not None:
        spec["transpile"] = transpile  # Tier-2 passthrough -> transpile() kwargs
    if execute is not None:
        spec["execute"] = execute      # Tier-2 passthrough -> AerSimulator.run() kwargs
    return {
        "apiVersion": "qcc.io/v1alpha1",
        "kind": "Circuit",
        "metadata": {"name": name, "labels": labels},
        "spec": spec,
    }


MANIFESTS = {
    # Deutsch — tiny, deterministic; survives stale calibration (~97% on the
    # balanced peak).  No Tier-2 block: nothing to tune on a depth-6 circuit.
    "deutsch.yaml": circuit(
        "deutsch", fmt="openqasm3", src="deutsch.qasm",
        labels={"qcc.io/algorithm": "deutsch", "qcc.io/algorithm-version": "v1",
                "qcc.io/experiment": "depth-vs-fidelity"},
        opt=3,
    ),

    # Shor — N=15, a=4 (the Ch1 coursework circuit).  Degraded at depth ~1700
    # on fake_brisbane (~27% on the two correct peaks vs 50% ideal).  Baseline
    # vs tuned shows Tier-2 trimming depth/2Q and nudging the result up a couple
    # of points while it stays honestly degraded.
    "shor-baseline.yaml": circuit(
        "shor-baseline", fmt="qiskit", src="shor.py",
        labels={"qcc.io/algorithm": "shor", "qcc.io/algorithm-version": "baseline",
                "qcc.io/experiment": "depth-vs-fidelity"},
        opt=1,
    ),
    "shor-tuned.yaml": circuit(
        "shor-tuned", fmt="qiskit", src="shor.py",
        labels={"qcc.io/algorithm": "shor", "qcc.io/algorithm-version": "tuned",
                "qcc.io/experiment": "depth-vs-fidelity"},
        opt=3,
        transpile={"layout_method": "sabre", "routing_method": "sabre",
                   "approximation_degree": 0.95, "seed_transpiler": 42},
        execute={"seed_simulator": 42},
    ),
}


def main() -> None:
    OUT.mkdir(exist_ok=True)
    for filename, doc in MANIFESTS.items():
        text = yaml.dump(doc, sort_keys=False, allow_unicode=True, width=10_000)
        (OUT / filename).write_text(text)
        print(f"wrote circuits/{filename}")


if __name__ == "__main__":
    main()
