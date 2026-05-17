"""qiskit_io unit tests — exercises both source formats and error paths."""

import pytest

from qcc_executor import qiskit_io

BELL_QASM = """OPENQASM 3.0;
include "stdgates.inc";
qubit[2] q;
bit[2] c;
h q[0];
cx q[0], q[1];
c[0] = measure q[0];
c[1] = measure q[1];
"""

BELL_PY = """from qiskit import QuantumCircuit
circuit = QuantumCircuit(2, 2)
circuit.h(0)
circuit.cx(0, 1)
circuit.measure([0, 1], [0, 1])
"""

BELL_PY_NO_NAMED_VAR = """from qiskit import QuantumCircuit
qc = QuantumCircuit(2, 2)
qc.h(0)
qc.cx(0, 1)
qc.measure([0, 1], [0, 1])
"""


def test_load_openqasm3():
    qc = qiskit_io.load_circuit("openqasm3", BELL_QASM)
    assert qc.num_qubits == 2
    assert qc.num_clbits == 2


def test_load_openqasm3_invalid_raises():
    with pytest.raises(qiskit_io.SourceError, match="OpenQASM"):
        qiskit_io.load_circuit("openqasm3", "this is not qasm")


def test_load_qiskit_named_circuit_variable():
    qc = qiskit_io.load_circuit("qiskit", BELL_PY)
    assert qc.num_qubits == 2


def test_load_qiskit_finds_any_QuantumCircuit():
    qc = qiskit_io.load_circuit("qiskit", BELL_PY_NO_NAMED_VAR)
    assert qc.num_qubits == 2


def test_load_qiskit_user_script_error_is_wrapped():
    body = "raise RuntimeError('boom')\n"
    with pytest.raises(qiskit_io.SourceError, match="RuntimeError"):
        qiskit_io.load_circuit("qiskit", body)


def test_load_qiskit_no_circuit_found():
    body = "x = 42\n"
    with pytest.raises(qiskit_io.SourceError, match="no QuantumCircuit"):
        qiskit_io.load_circuit("qiskit", body)


def test_load_unsupported_format():
    with pytest.raises(qiskit_io.SourceError, match="unsupported"):
        qiskit_io.load_circuit("cirq", "anything")


def test_dump_qasm_round_trip():
    qc = qiskit_io.load_circuit("qiskit", BELL_PY)
    qasm = qiskit_io.dump_qasm(qc)
    assert qasm.startswith("OPENQASM 3.0")
    assert "cx" in qasm


def test_dump_qasm_unrolls_high_level_library_gates():
    """The QASM3 exporter rejects library-instruction subroutines (e.g. QFT);
    dump_qasm must decompose them first or `qcc run` with qiskit sources
    that use such gates would fail with QASM3ExporterError."""
    from qiskit import QuantumCircuit
    from qiskit.circuit.library import QFT

    qc = QuantumCircuit(3)
    qc.h(0)
    qc.append(QFT(3, inverse=True), range(3))

    qasm = qiskit_io.dump_qasm(qc)
    assert qasm.startswith("OPENQASM 3.0")
    # IQFT should be unrolled — no 'IQFT' subroutine left, only primitives
    # (Qiskit emits Hadamard as `U(pi/2, 0, pi)` rather than the `h` macro,
    # so we check for the cx that the inverse-QFT decomposition produces).
    assert "IQFT" not in qasm
    assert "cx " in qasm


def test_draw_text_contains_gate_letters():
    qc = qiskit_io.load_circuit("openqasm3", BELL_QASM)
    drawing = qiskit_io.draw_text(qc)
    for want in ("H", "X", "q_0", "q_1"):
        assert want in drawing, f"draw output missing {want!r}\n{drawing}"
