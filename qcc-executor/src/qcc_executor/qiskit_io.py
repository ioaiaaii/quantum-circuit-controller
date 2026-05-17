"""Qiskit input/output helpers for the executor.

This module owns the only place Qiskit's parsing/serialisation lives in QCC:
the executor.  The controller and CLI never touch Qiskit.  Two transforms are
exposed (the converse of each other for openqasm3):

  - load_circuit(format, body) -> QuantumCircuit
  - dump_qasm(qc) -> str            (always OpenQASM 3)
  - draw_text(qc) -> str            (Qiskit's text drawer)

For `format == "qiskit"` the body is executed in an isolated module namespace
and the first `QuantumCircuit` is extracted (preferring a top-level `circuit`
variable).  This is the *only* place we exec user-provided Python in QCC; in a
multi-tenant deployment the executor pod would be hardened (Pod Security
Standard, no host mounts).  For the thesis prototype it runs in the same
sandbox the user already gave us when they submitted the Circuit.
"""

from __future__ import annotations

import types

from qiskit import QuantumCircuit, qasm3

FORMAT_OPENQASM3 = "openqasm3"
FORMAT_QISKIT = "qiskit"


class SourceError(Exception):
    """Raised when a circuit source cannot be loaded into a QuantumCircuit."""


def load_circuit(source_format: str, body: str) -> QuantumCircuit:
    """Parse body into a QuantumCircuit according to source_format."""
    if source_format == FORMAT_OPENQASM3:
        try:
            return qasm3.loads(body)
        except Exception as exc:  # noqa: BLE001 — wrap for caller
            raise SourceError(
                f"failed to parse OpenQASM 3: {type(exc).__name__}: {exc}") from exc

    if source_format == FORMAT_QISKIT:
        module = types.ModuleType("qcc_user_circuit")
        try:
            exec(body, module.__dict__)  # noqa: S102 — user-submitted Python is the contract here
        except Exception as exc:  # noqa: BLE001
            raise SourceError(
                f"qiskit source raised {type(exc).__name__}: {exc}"
            ) from exc
        qc = getattr(module, "circuit", None)
        if qc is None:
            for value in vars(module).values():
                if isinstance(value, QuantumCircuit):
                    qc = value
                    break
        if qc is None:
            raise SourceError(
                "no QuantumCircuit found in qiskit source; expose a top-level "
                "variable named `circuit` or any QuantumCircuit at module scope"
            )
        return qc

    raise SourceError(
        f"unsupported source format: {source_format!r}; expected "
        f"'{FORMAT_OPENQASM3}' or '{FORMAT_QISKIT}'"
    )


def dump_qasm(qc: QuantumCircuit) -> str:
    """Return the OpenQASM 3 representation of qc.

    Custom gates and library-defined high-level instructions (Qiskit's
    ``QFT``, user-defined sub-circuits converted via ``.to_gate()``, etc.)
    are decomposed first because the OpenQASM 3 exporter currently rejects
    "non-unitary subroutine calls" — see Qiskit issue tracker.  Five
    decomposition rounds unrolls every nested library construction the
    thesis-scale examples use (Shor, QFT, mod-arithmetic blocks); already-
    primitive circuits are unaffected because ``decompose`` is a no-op on
    them.
    """
    return qasm3.dumps(qc.decompose(reps=5))


def draw_text(qc: QuantumCircuit) -> str:
    """Return the ASCII text drawing of qc (Qiskit's text drawer)."""
    return str(qc.draw("text"))
