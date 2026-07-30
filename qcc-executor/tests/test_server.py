"""Executor gRPC integration test (in-process channel, Aer adapter)."""

from google.protobuf import struct_pb2

from qcc_executor.protostubs import executor_pb2, executor_pb2_grpc

BELL_QASM = """OPENQASM 3.0;
include "stdgates.inc";
qubit[2] q;
bit[2] c;
h q[0];
cx q[0], q[1];
c[0] = measure q[0];
c[1] = measure q[1];
"""


def test_run_circuit_bell_state_via_grpc(executor_channel):
    stub = executor_pb2_grpc.ExecutorStub(executor_channel)
    request = executor_pb2.RunCircuitRequest(
        spec=executor_pb2.TaskSpec(
            idempotency_key="test-1",
            qasm=BELL_QASM,
            shots=1000,
            target=executor_pb2.BackendTarget(
                provider="local",
                backend_name="aer_simulator",
                kind=executor_pb2.BACKEND_KIND_SIMULATOR,
            ),
        )
    )
    response = stub.RunCircuit(request)
    assert response.status == executor_pb2.TASK_STATUS_DONE, response.error_message
    assert sum(response.counts.values()) == 1000
    assert set(response.counts).issubset({"00", "11"})
    assert response.transpile.depth > 0


def test_submit_watch_fetch_async_lifecycle(executor_channel):
    """Async lifecycle works end-to-end on Aer (synchronous job-completion
    inside submit; WatchTask sees DONE on first poll; FetchTaskResult
    returns counts and removes the handle).  Covers the same control-flow
    path real IBM hardware takes via IBMAdapter, just without the queue
    wait — the executor's adapter contract is uniform across substrates."""
    stub = executor_pb2_grpc.ExecutorStub(executor_channel)
    request = executor_pb2.SubmitTaskRequest(
        spec=executor_pb2.TaskSpec(
            idempotency_key="test-submit-1",
            qasm=BELL_QASM,
            shots=200,
            target=executor_pb2.BackendTarget(
                provider="local",
                backend_name="aer_simulator",
                kind=executor_pb2.BACKEND_KIND_SIMULATOR,
            ),
        ),
    )
    submit = stub.SubmitTask(request)
    assert submit.task_id, "expected non-empty task_id"
    assert submit.transpile.depth > 0

    # WatchTask: expect at least one frame, terminal=DONE for Aer.
    statuses = []
    for resp in stub.WatchTask(executor_pb2.WatchTaskRequest(task_id=submit.task_id)):
        statuses.append(resp.status)
        if resp.status in (
            executor_pb2.TASK_STATUS_DONE,
            executor_pb2.TASK_STATUS_FAILED,
            executor_pb2.TASK_STATUS_CANCELLED,
        ):
            break
    assert statuses and statuses[-1] == executor_pb2.TASK_STATUS_DONE

    fetch = stub.FetchTaskResult(
        executor_pb2.FetchTaskResultRequest(task_id=submit.task_id),
    )
    assert fetch.status == executor_pb2.TASK_STATUS_DONE
    assert sum(fetch.counts.values()) == 200
    assert set(fetch.counts).issubset({"00", "11"})

    # Registry cleanup: a second fetch returns FAILED/TaskNotFound.
    again = stub.FetchTaskResult(
        executor_pb2.FetchTaskResultRequest(task_id=submit.task_id),
    )
    assert again.status == executor_pb2.TASK_STATUS_FAILED
    assert again.error_reason == "TaskNotFound"


def test_run_circuit_forwards_tier2_passthrough(executor_channel):
    """Tier 2 passthrough — seed_simulator on Aer makes two runs of the
    same circuit produce *identical* counts.  Verifies the dict reaches
    the adapter without translation (snake_case Qiskit kwargs)."""
    stub = executor_pb2_grpc.ExecutorStub(executor_channel)

    def run_with_seed(seed: int) -> dict[str, int]:
        execute_options = struct_pb2.Struct()
        execute_options.update({"seed_simulator": seed})
        # Force a deterministic basis so transpile_options is exercised
        # too.  basis_gates=['h','cx','measure'] keeps the Bell circuit
        # untouched but proves the kwarg reached transpile().
        transpile_options = struct_pb2.Struct()
        transpile_options.update({"seed_transpiler": 7})

        spec = executor_pb2.TaskSpec(
            idempotency_key=f"test-seeded-{seed}",
            qasm=BELL_QASM,
            shots=500,
            target=executor_pb2.BackendTarget(
                provider="local",
                backend_name="aer_simulator",
                kind=executor_pb2.BACKEND_KIND_SIMULATOR,
            ),
            transpile_options=transpile_options,
            execute_options=execute_options,
        )
        resp = stub.RunCircuit(executor_pb2.RunCircuitRequest(spec=spec))
        assert resp.status == executor_pb2.TASK_STATUS_DONE, resp.error_message
        return dict(resp.counts)

    first = run_with_seed(42)
    second = run_with_seed(42)
    assert first == second, (
        "seed_simulator did not reach AerSimulator.run — Tier-2 passthrough "
        f"is not forwarding kwargs. first={first} second={second}"
    )
    third = run_with_seed(43)
    # Different seed → almost certainly different counts (probabilistic
    # but the chance of collision on 500 shots is negligible).
    assert third != first, "different seed produced identical counts — suspect"


def test_run_circuit_drops_tier1_keys_in_execute_options(executor_channel):
    """shots in execute_options is silently dropped — Tier-1 wins."""
    stub = executor_pb2_grpc.ExecutorStub(executor_channel)
    execute_options = struct_pb2.Struct()
    # 1 shot inside execute_options; 500 shots on the dedicated field.
    # Without the strip, AerSimulator.run() would either accept the
    # spurious shots=1 (giving us 1 outcome) or raise on duplicate kwarg.
    execute_options.update({"shots": 1})

    spec = executor_pb2.TaskSpec(
        idempotency_key="test-tier1-precedence",
        qasm=BELL_QASM,
        shots=500,
        target=executor_pb2.BackendTarget(
            provider="local",
            backend_name="aer_simulator",
            kind=executor_pb2.BACKEND_KIND_SIMULATOR,
        ),
        execute_options=execute_options,
    )
    resp = stub.RunCircuit(executor_pb2.RunCircuitRequest(spec=spec))
    assert resp.status == executor_pb2.TASK_STATUS_DONE, resp.error_message
    assert sum(resp.counts.values()) == 500
