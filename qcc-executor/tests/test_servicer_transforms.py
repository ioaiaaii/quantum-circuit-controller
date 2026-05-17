"""Servicer tests for ConvertSource and DrawCircuit (no live cluster needed)."""

from qcc_executor.protostubs import executor_pb2, executor_pb2_grpc
from qcc_executor.servicer import ExecutorServicer

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


def _convert_via_grpc(executor_channel, source_format: str, body: str):
    stub = executor_pb2_grpc.ExecutorStub(executor_channel)
    return stub.ConvertSource(
        executor_pb2.ConvertSourceRequest(
            source=executor_pb2.CircuitSource(format=source_format, body=body),
        )
    )


def _draw_via_grpc(executor_channel, source_format: str, body: str):
    stub = executor_pb2_grpc.ExecutorStub(executor_channel)
    return stub.DrawCircuit(
        executor_pb2.DrawCircuitRequest(
            source=executor_pb2.CircuitSource(format=source_format, body=body),
        )
    )


def test_convert_source_openqasm3_round_trip(executor_channel):
    resp = _convert_via_grpc(executor_channel, "openqasm3", BELL_QASM)
    assert resp.status == executor_pb2.TASK_STATUS_DONE, resp.error_message
    assert resp.qasm.startswith("OPENQASM 3.0")


def test_convert_source_qiskit_to_qasm(executor_channel):
    resp = _convert_via_grpc(executor_channel, "qiskit", BELL_PY)
    assert resp.status == executor_pb2.TASK_STATUS_DONE, resp.error_message
    assert resp.qasm.startswith("OPENQASM 3.0")
    assert "cx" in resp.qasm


def test_convert_source_failure_carries_reason(executor_channel):
    resp = _convert_via_grpc(executor_channel, "openqasm3", "not qasm")
    assert resp.status == executor_pb2.TASK_STATUS_FAILED
    assert resp.error_reason == "SourceConversionFailed"
    assert "OpenQASM" in resp.error_message or "parse" in resp.error_message


def test_draw_circuit_openqasm3(executor_channel):
    resp = _draw_via_grpc(executor_channel, "openqasm3", BELL_QASM)
    assert resp.status == executor_pb2.TASK_STATUS_DONE, resp.error_message
    for want in ("H", "X", "q_0", "q_1"):
        assert want in resp.drawing, f"drawing missing {want}: {resp.drawing}"


def test_draw_circuit_qiskit(executor_channel):
    resp = _draw_via_grpc(executor_channel, "qiskit", BELL_PY)
    assert resp.status == executor_pb2.TASK_STATUS_DONE, resp.error_message
    assert "H" in resp.drawing


def test_draw_circuit_failure_carries_reason(executor_channel):
    resp = _draw_via_grpc(executor_channel, "qiskit", "x = 42\n")
    assert resp.status == executor_pb2.TASK_STATUS_FAILED
    assert resp.error_reason == "RenderingFailed"


def _unused_imports():
    # Keep the ExecutorServicer import alive for IDEs that prune unused.
    assert ExecutorServicer is not None
