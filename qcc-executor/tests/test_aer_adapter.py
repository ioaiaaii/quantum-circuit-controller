"""Aer adapter unit tests — Bell state on the in-process simulator."""

import pytest

from qcc_executor.adapters import AdapterUnavailable, get_adapter

BELL_QASM = """OPENQASM 3.0;
include "stdgates.inc";
qubit[2] q;
bit[2] c;
h q[0];
cx q[0], q[1];
c[0] = measure q[0];
c[1] = measure q[1];
"""


class FakeTarget:
    optimization_level = 1


def test_aer_executes_bell_state():
    adapter = get_adapter("local")
    transpiled = adapter.transpile(BELL_QASM, FakeTarget())
    assert transpiled.depth > 0
    handle = adapter.submit(transpiled, shots=1000)
    result = adapter.fetch_result(handle)
    assert sum(result.counts.values()) == 1000
    assert set(result.counts).issubset({"00", "11"}), (
        f"unexpected Bell outcomes: {result.counts}"
    )
    # Aer reports no billable quantum-seconds; 0.0 is the documented
    # "not reported" sentinel (see FetchResult in adapters/base.py).
    assert result.usage_seconds == 0.0


def test_aer_default_provider_alias():
    """Empty provider string falls back to Aer."""
    adapter = get_adapter("")
    assert adapter.name == "local"


def test_ibm_adapter_unavailable_without_token(monkeypatch):
    monkeypatch.delenv("QISKIT_IBM_TOKEN", raising=False)
    with pytest.raises(AdapterUnavailable):
        get_adapter("ibm")


def test_unknown_provider_raises():
    with pytest.raises(AdapterUnavailable, match="unknown provider"):
        get_adapter("rigetti")


# --- fake-backend resolution (M1.5a) ------------------------------------


def test_aer_resolves_fake_brisbane_to_real_calibration_backend():
    """`backendName=fake_brisbane` must instantiate the FakeBrisbane
    backend (V2), not generic Aer — that's what makes mode=run produce
    real-calibration noise instead of clean Aer counts."""
    from qiskit_ibm_runtime.fake_provider import FakeBrisbane

    adapter = get_adapter("local", "fake_brisbane")
    assert isinstance(adapter.backend, FakeBrisbane)
    # The fake backend exposes the real Brisbane qubit count and basis
    # gates — same shape the QPUReconciler will probe in M1.5b.
    assert adapter.backend.num_qubits == 127
    assert {"ecr", "rz", "sx", "x"}.issubset(set(adapter.backend.operation_names))


def test_aer_falls_back_to_plain_aer_for_non_fake_names():
    """Generic Aer for everything that doesn't start with `fake_`."""
    from qiskit_aer import AerSimulator

    for name in ("aer_simulator", "", "anything_else"):
        adapter = get_adapter("local", name)
        assert isinstance(adapter.backend, AerSimulator), (
            f"expected plain Aer for backend_name={name!r}"
        )


def test_aer_rejects_unknown_fake_backend_as_adapter_unavailable():
    """Unknown fake_* names must surface as AdapterUnavailable so the
    servicer can return a terminal NoEligibleBackend instead of leaking
    the QiskitBackendNotFoundError into a transient-RPC retry loop."""
    with pytest.raises(AdapterUnavailable, match="unknown fake backend"):
        get_adapter("local", "fake_definitely_not_a_backend")


def test_inspect_returns_real_calibration_for_fake_brisbane():
    """The probe must return Brisbane's real qubit count, real basis gate
    set, and non-zero error medians — these are the numbers M2's scoring
    layer multiplies into the composite score."""
    adapter = get_adapter("local", "fake_brisbane")
    meta = adapter.inspect()

    assert meta.num_qubits == 127
    # Eagle r3 native gate set; we tolerate any superset to stay
    # robust against upstream Qiskit adding op names.
    assert {"ecr", "rz", "sx", "x"}.issubset(set(meta.basis_gates))
    # Heavy-hex Brisbane → many edges; "many" is enough, exact count is
    # a Qiskit-internal detail we don't want to pin.
    assert meta.coupling_edges > 0
    # Real-calibration snapshot has a known capture time.
    assert meta.last_calibration_time != ""
    # All three error medians populated and physically plausible
    # (Brisbane's published gate errors are well under 10% across the board).
    assert 0.0 < meta.single_qubit_error_median < 0.1
    assert 0.0 < meta.two_qubit_error_median < 0.1
    assert 0.0 < meta.readout_error_median < 0.1


def test_inspect_returns_zeros_for_generic_aer():
    """Plain Aer has no calibration — the probe reports the qubit count
    via the simulator's Target, every other field is zero/empty.  The
    controller's scoring layer treats zero as 'skip,' not 'perfect.'"""
    adapter = get_adapter("local")  # default → AerSimulator()
    meta = adapter.inspect()

    # AerSimulator's default num_qubits is 0 (allocates per circuit);
    # accept any non-negative value.
    assert meta.num_qubits >= 0
    assert meta.coupling_edges == 0
    assert meta.last_calibration_time == ""
    assert meta.single_qubit_error_median == 0.0
    assert meta.two_qubit_error_median == 0.0
    assert meta.readout_error_median == 0.0


def test_fake_brisbane_executes_bell_and_returns_counts():
    """End-to-end sanity: fake-backed AerAdapter still transpiles +
    submits + returns counts.  The counts are noisy (real calibration)
    but must still sum to the shot count and stay in the 2-bit space."""
    adapter = get_adapter("local", "fake_brisbane")
    transpiled = adapter.transpile(BELL_QASM, FakeTarget())
    assert transpiled.depth > 0
    handle = adapter.submit(transpiled, shots=1000)
    result = adapter.fetch_result(handle)
    assert sum(result.counts.values()) == 1000
    # Real noise → small probability of `01`/`10` outcomes; the support
    # is the full 2-bit space, not just {`00`,`11`} like noise-free Aer.
    assert set(result.counts).issubset({"00", "01", "10", "11"})
