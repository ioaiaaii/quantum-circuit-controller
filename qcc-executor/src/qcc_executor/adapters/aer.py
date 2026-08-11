"""In-process Qiskit Aer simulator adapter (provider=`local`).

Resolves three families of backend names:

  - ``aer_<method>`` — method-pinned :class:`qiskit_aer.AerSimulator`
    where ``<method>`` is one of statevector / matrix_product_state /
    stabilizer / extended_stabilizer / density_matrix / unitary.  Use
    ``aer_statevector`` as the canonical ideal-noiseless reference for
    outcome-quality metrics (Hellinger fidelity vs ideal, TVD).
    Pinned-method profiles are deterministic — they don't shift across
    circuits the way Aer's ``automatic`` method does.

  - ``fake_*`` — a backend from :mod:`qiskit_ibm_runtime.fake_provider`,
    which wraps Aer with a frozen real-calibration snapshot (T1, T2,
    gate errors, real coupling map, real basis-gate set).  These give
    vendor-grade noise behaviour without IBM credentials and are the
    demo surface for calibration-aware selection in M2.

  - ``aer_simulator`` / empty / anything else — generic, noise-free
    :class:`qiskit_aer.AerSimulator` with automatic method selection.
    Convenient as a fallback; not ideal for thesis-citable runs because
    the method picked depends on the circuit.

The resolver runs at :class:`AerAdapter` construction time, so unknown
``fake_*`` names surface as :class:`AdapterUnavailable` → the servicer
translates that to a terminal ``NoEligibleBackend`` failure instead of
the transient-RPC requeue loop a bare exception would trigger.

Encoding the method choice in the resolver (rather than in YAML) is a
deliberate Composition-Principle decision: provider construction lives
at the adapter boundary; different constructions are different QPU CRs.
See QCC-Design-State.md §7a.
"""

from __future__ import annotations

import statistics
import uuid
from collections.abc import Mapping
from datetime import datetime
from typing import Any

from qiskit import qasm3, transpile
from qiskit_aer import AerSimulator

from .base import (
    Adapter,
    AdapterUnavailable,
    BackendMetadata,
    CircuitSchedule,
    FetchResult,
    JobHandle,
    JobStatus,
    ScheduledOp,
    TranspiledCircuit,
)

# Method-pinned Aer variants.  These give the operator (and the thesis)
# a *deterministic* simulator profile rather than Aer's automatic-method
# selection — which picks based on circuit heuristics and so produces
# results that can shift unexpectedly.  Encoded in the adapter resolver
# (not in YAML) per the Composition Principle: provider construction
# lives at the adapter boundary, dispatched by backendName.  Adding a
# new variant is one entry here; the corresponding QPU CR's spec.backendName
# selects it.  See QCC-Design-State.md §7a.
_AER_METHOD_BACKENDS = {
    "aer_statevector":         "statevector",
    "aer_mps":                 "matrix_product_state",
    "aer_stabilizer":          "stabilizer",
    "aer_extended_stabilizer": "extended_stabilizer",
    "aer_density_matrix":      "density_matrix",
    "aer_unitary":             "unitary",
}


def _resolve_local_backend(backend_name: str):
    """Map a backend name to the Qiskit Backend the adapter runs on.

    Three resolution paths, in order:

      1. ``aer_<method>`` (e.g. ``aer_statevector``) — pinned-method
         AerSimulator.  Use for a deterministic profile: an ideal
         noiseless reference (statevector), an approximate large-scale
         simulator (matrix_product_state), a Clifford-only at scale
         (stabilizer), etc.

      2. ``fake_<backend>`` (e.g. ``fake_brisbane``) — FakeProviderForBackendV2
         snapshot wrapping a frozen real-IBM calibration.

      3. Anything else (e.g. ``aer_simulator`` or empty) — bare
         AerSimulator() with automatic method selection.  Useful as a
         general-purpose fallback; not ideal for thesis-citable runs
         because the method may shift between circuits.
    """
    if backend_name in _AER_METHOD_BACKENDS:
        method = _AER_METHOD_BACKENDS[backend_name]
        return AerSimulator(method=method)

    if not backend_name.startswith("fake_"):
        return AerSimulator()

    try:
        # FakeProviderForBackendV2 is the modern entry point — V2 Backend
        # API + Target object.  V1 fakes are deprecated.
        from qiskit_ibm_runtime.fake_provider import FakeProviderForBackendV2  # noqa: PLC0415
    except ImportError as exc:  # pragma: no cover — install bug
        raise AdapterUnavailable(
            f"qiskit_ibm_runtime.fake_provider missing; cannot resolve {backend_name!r}"
        ) from exc
    try:
        return FakeProviderForBackendV2().backend(backend_name)
    except Exception as exc:  # noqa: BLE001 — surface as adapter error regardless of class
        # FakeProviderForBackendV2 raises QiskitBackendNotFoundError for
        # unknown names; catch broadly so any provider-side oddity reads
        # as a clean "this backend doesn't exist" to the controller.
        raise AdapterUnavailable(
            f"unknown fake backend {backend_name!r}: {exc}"
        ) from exc


class AerAdapter(Adapter):
    name = "local"

    def __init__(self, backend_name: str = "aer_simulator") -> None:
        self.backend_name = backend_name
        self.backend = _resolve_local_backend(backend_name)
        self._results: dict[str, dict[str, int]] = {}

    def transpile(
        self,
        qasm: str,
        target,
        options: Mapping[str, Any] | None = None,
    ) -> TranspiledCircuit:
        try:
            qc = qasm3.loads(qasm)
        except Exception as exc:  # qasm3 raises a variety of errors; normalise
            raise RuntimeError(f"Invalid OpenQASM 3: {exc}") from exc
        opt_level = getattr(target, "optimization_level", 1) or 1
        # Tier-2 passthrough: forward dict verbatim as kwargs.  Tier-1
        # optimization_level wins by default; if the user explicitly
        # set optimization_level in the passthrough block (smell, but
        # not forbidden), the dict-unpacking puts theirs *after* and
        # Python's resolution rules give Qiskit the last value.
        kwargs: dict[str, Any] = {"optimization_level": opt_level}
        if options:
            kwargs.update(options)
        transpiled = transpile(qc, self.backend, **kwargs)
        two_qubit = sum(1 for inst in transpiled.data if len(inst.qubits) >= 2)
        return TranspiledCircuit(
            qiskit_circuit=transpiled,
            depth=transpiled.depth(),
            total_gates=sum(transpiled.count_ops().values()),
            two_qubit_gates=two_qubit,
        )

    def submit(
        self,
        circuit: TranspiledCircuit,
        shots: int,
        options: Mapping[str, Any] | None = None,
        circuit_uid: str = "",
    ) -> JobHandle:
        # circuit_uid intentionally unused here.  AerSimulator has no
        # vendor-side job-tag surface to stamp; the linkage matters only
        # for backends with an external job system (IBM).  Local
        # simulations are fully observable via the K8s CRD's status.
        _ = circuit_uid
        # AerSimulator.run accepts seed_simulator, memory, parameter_binds,
        # and a handful of other kwargs.  Forward whatever the user set.
        run_kwargs: dict[str, Any] = {"shots": shots}
        if options:
            run_kwargs.update(options)
            run_kwargs["shots"] = shots  # shots is Tier-1; restore after merge
        job = self.backend.run(circuit.qiskit_circuit, **run_kwargs)
        result = job.result()
        counts = {str(k): int(v) for k, v in result.get_counts().items()}
        job_id = f"aer-{uuid.uuid4()}"
        self._results[job_id] = counts
        return JobHandle(provider_job_id=job_id, backend=self.backend_name)

    def poll(self, handle: JobHandle) -> JobStatus:
        return JobStatus.DONE if handle.provider_job_id in self._results else JobStatus.FAILED

    def fetch_result(self, handle: JobHandle) -> FetchResult:
        counts = self._results.pop(handle.provider_job_id, None)
        if counts is None:
            raise RuntimeError(f"unknown Aer job {handle.provider_job_id}")
        # Aer doesn't have a notion of billable "quantum-seconds" — it's
        # local CPU compute.  Returning 0.0 lets the controller treat
        # this as "not reported", so the qcc_circuit_usage_seconds metric
        # naturally distinguishes real-hardware runs (>0) from simulator
        # runs (no series emitted) without a separate flag.
        return FetchResult(counts=counts, usage_seconds=0.0)

    def inspect(self) -> BackendMetadata:
        """Introspect the resolved backend's Target for calibration data.

        For ``fake_*`` backends the Target carries the frozen real
        snapshot (last_update_date in the past, per-instruction errors,
        coupling map).  For plain :class:`AerSimulator` the Target has
        no calibration / coupling info — every numeric field comes back
        as zero and the controller treats those as "skip in scoring."
        """
        target = getattr(self.backend, "target", None)
        if target is None:
            # Generic Aer has no target; return a minimal report with
            # just the qubit count (still useful for spec.qubits validation).
            return BackendMetadata(num_qubits=int(getattr(self.backend, "num_qubits", 0)))

        t1, t2 = _coherence_medians(target)
        family, revision, segment = _processor_identity(self.backend)
        return BackendMetadata(
            num_qubits=int(self.backend.num_qubits),
            basis_gates=tuple(sorted(self.backend.operation_names)),
            coupling_edges=_count_coupling_edges(self.backend),
            last_calibration_time=_format_calibration_time(target, self.backend),
            single_qubit_error_median=_median_instruction_error(target, arity=1, skip={"measure"}),
            two_qubit_error_median=_median_instruction_error(target, arity=2),
            readout_error_median=_median_instruction_error(target, arity=1, only={"measure"}),
            t1_median_us=t1,
            t2_median_us=t2,
            dt_seconds=_backend_dt(self.backend),
            single_qubit_duration_median_seconds=_median_instruction_duration(
                target, arity=1, skip={"measure"}
            ),
            two_qubit_duration_median_seconds=_median_instruction_duration(target, arity=2),
            processor_family=family,
            processor_revision=revision,
            processor_segment=segment,
        )

    def schedule(self, qasm: str, target) -> CircuitSchedule:  # type: ignore[no-untyped-def]
        """Return the scheduled timeline for ``qasm`` on this backend.

        Re-transpiles with ``scheduling_method='asap'`` so each gate
        gets an explicit start time, then walks ``op_start_times`` and
        the backend's Target durations to build the structured
        timeline.  Times are in dt cycles; the servicer echoes
        ``backend.dt`` so the caller can convert to seconds.

        Generic ``AerSimulator`` doesn't carry gate durations on its
        Target and the scheduling pass refuses to run — that surfaces
        as ``RuntimeError("scheduling unsupported on this backend")``
        and the gRPC layer turns it into a terminal
        ``SchedulingUnsupported`` failure.
        """
        try:
            qc = qasm3.loads(qasm)
        except Exception as exc:
            raise RuntimeError(f"Invalid OpenQASM 3: {exc}") from exc
        opt_level = getattr(target, "optimization_level", 1) or 1

        backend_target = getattr(self.backend, "target", None)
        if backend_target is None or not _backend_has_durations(backend_target):
            # Generic Aer falls through to this path — no Target or a
            # Target without instruction durations means scheduling
            # can't be computed.  Surface the limitation honestly.
            raise RuntimeError(
                f"scheduling unsupported on backend {self.backend_name!r}: "
                "no instruction durations in the backend Target",
            )

        try:
            scheduled = transpile(
                qc,
                self.backend,
                optimization_level=opt_level,
                scheduling_method="asap",
            )
        except Exception as exc:  # noqa: BLE001 — surface as terminal scheduling failure
            raise RuntimeError(f"scheduling failed: {exc}") from exc

        starts = list(getattr(scheduled, "op_start_times", []) or [])
        if not starts:
            # If asap-scheduling silently degraded (older Qiskit, weird
            # backend) we still want a coherent response — emit an
            # empty schedule rather than crashing the RPC.
            return CircuitSchedule(
                ops=(),
                total_duration_dt=0,
                dt_seconds=_backend_dt(self.backend),
                num_qubits=int(getattr(self.backend, "num_qubits", 0)),
                backend_used=self.backend_name,
            )

        ops: list[ScheduledOp] = []
        total = 0
        for instr, start in zip(scheduled.data, starts, strict=True):
            qubits = tuple(scheduled.find_bit(q).index for q in instr.qubits)
            name = instr.operation.name
            duration_dt = _instruction_duration_dt(
                backend_target, name, qubits, instr.operation,
            )
            ops.append(
                ScheduledOp(
                    name=name,
                    qubits=qubits,
                    start_dt=int(start),
                    duration_dt=int(duration_dt),
                ),
            )
            end = int(start) + int(duration_dt)
            if end > total:
                total = end

        return CircuitSchedule(
            ops=tuple(ops),
            total_duration_dt=total,
            dt_seconds=_backend_dt(self.backend),
            num_qubits=int(getattr(self.backend, "num_qubits", 0)),
            backend_used=self.backend_name,
        )


# --- Backend introspection helpers (pure, target-API only) ---------------


def _count_coupling_edges(backend) -> int:  # type: ignore[no-untyped-def]
    """Return the number of physical 2-qubit edges, or 0 for all-to-all."""
    cm = getattr(backend, "coupling_map", None)
    if cm is None:
        return 0
    try:
        return len(cm.get_edges())
    except Exception:  # noqa: BLE001 — defensive, never raise to caller
        return 0


def _format_calibration_time(target, backend=None) -> str:  # type: ignore[no-untyped-def]
    """Return RFC 3339 timestamp string for the snapshot date, or ''.

    Looks in three places in order of preference:

      1. ``target.last_update_date`` — set on some hardware backends.
      2. ``backend.properties().last_update_date`` — V1-compat shim that
         the fake provider populates with the snapshot capture date.
      3. ``target.update_date`` — older naming on some backends.
    """
    candidates = [
        getattr(target, "last_update_date", None),
        getattr(target, "update_date", None),
    ]
    if backend is not None:
        try:
            props = backend.properties() if hasattr(backend, "properties") else None
        except Exception:  # noqa: BLE001 — properties() may raise on transient backends
            props = None
        if props is not None:
            candidates.append(getattr(props, "last_update_date", None))
    for raw in candidates:
        if raw is None:
            continue
        if isinstance(raw, datetime):
            return raw.isoformat()
        return str(raw)
    return ""


def _coherence_medians(target) -> tuple[float, float]:  # type: ignore[no-untyped-def]
    """Return median (T1, T2) across qubits in *microseconds*.

    Qiskit's ``QubitProperties.t1`` and ``.t2`` are in seconds; we
    convert to microseconds because that's the unit IBM publishes and
    that lets us render compact numbers in `qcc qpu` (e.g. "230 µs" vs
    "0.00023 s").  Returns (0.0, 0.0) when the backend reports no
    qubit_properties — generic Aer, or old fakes.
    """
    qps = getattr(target, "qubit_properties", None) or []
    t1s: list[float] = []
    t2s: list[float] = []
    for qp in qps:
        if qp is None:
            continue
        t1 = getattr(qp, "t1", None)
        if t1 is not None:
            t1s.append(float(t1) * 1e6)
        t2 = getattr(qp, "t2", None)
        if t2 is not None:
            t2s.append(float(t2) * 1e6)
    t1_med = statistics.median(t1s) if t1s else 0.0
    t2_med = statistics.median(t2s) if t2s else 0.0
    return t1_med, t2_med


def _backend_dt(backend) -> float:  # type: ignore[no-untyped-def]
    """Return the backend's control-electronics cycle period (seconds).

    Qiskit exposes this as ``backend.dt``.  Generic AerSimulator
    returns None (no real control hardware to clock); fake backends
    report the real device's dt (typically ~2.22e-10 s).
    """
    dt = getattr(backend, "dt", None)
    if dt is None:
        return 0.0
    return float(dt)


def _backend_has_durations(target) -> bool:  # type: ignore[no-untyped-def]
    """Cheap probe: does ``target`` carry instruction durations at all?

    Used by AerAdapter.schedule to fail fast when the resolved backend
    is generic Aer (whose Target has no durations) rather than letting
    the transpiler raise a confusing scheduling-pass error.  Walks at
    most a few operations to keep the probe O(1).
    """
    dt = getattr(target, "dt", None)
    if dt is None:
        return False
    for op_name in target.operation_names:
        try:
            instr_map = target[op_name]
        except Exception:  # noqa: BLE001, S112 — defensive
            continue
        for props in instr_map.values():
            if props is None:
                continue
            if getattr(props, "duration", None) is not None:
                return True
    return False


def _instruction_duration_dt(target, name, qubits, op) -> int:  # type: ignore[no-untyped-def]
    """Look up an instruction's duration on this backend, in dt cycles.

    Qiskit reports durations in seconds via ``target[name][qubits].duration``
    (and the Target's ``dt`` carries the conversion factor).  Tries the
    exact-qubits key first; if that fails (e.g. the routing pass picked
    a virtual entry) returns a per-arity median.  Returns 0 when no
    duration is reported anywhere for the op — that's rare on real
    fakes but possible for symbolic gates like ``barrier``.
    """
    # delay instructions carry their own dt-unit duration on the op.
    if name == "delay":
        d = getattr(op, "duration", None)
        if d is not None:
            return int(d)
    dt = getattr(target, "dt", None)
    if dt is None:
        return 0
    try:
        instr_map = target[name]
    except Exception:  # noqa: BLE001 — defensive
        return 0
    props = instr_map.get(tuple(qubits)) if isinstance(instr_map, dict) else None
    if props is not None and getattr(props, "duration", None) is not None:
        return int(round(float(props.duration) / float(dt)))
    # Fall back to the median across all qubit-tuples for this op.
    durations = [
        float(p.duration)
        for p in instr_map.values()
        if p is not None and getattr(p, "duration", None) is not None
    ]
    if not durations:
        return 0
    return int(round(statistics.median(durations) / float(dt)))


def _processor_identity(backend) -> tuple[str, str, str]:  # type: ignore[no-untyped-def]
    """Return ``(family, revision, segment)`` for the resolved backend.

    Qiskit exposes :attr:`backend.processor_type` as a dict that varies
    by provider — typically ``{family, revision}`` but with optional
    ``segment`` for sub-divided families like Falcon-T.  We normalise
    ``revision`` to a string (Qiskit reports a mix of ints and strings
    across families).  Returns three empty strings when the backend has
    no processor_type metadata (generic Aer).
    """
    pt = getattr(backend, "processor_type", None)
    if not isinstance(pt, dict):
        return "", "", ""
    family = str(pt.get("family", "") or "")
    revision_raw = pt.get("revision", "")
    revision = "" if revision_raw in (None, "") else str(revision_raw)
    segment = str(pt.get("segment", "") or "")
    return family, revision, segment


def _median_instruction_duration(
    target,  # type: ignore[no-untyped-def]
    *,
    arity: int,
    only: set[str] | None = None,
    skip: set[str] | None = None,
) -> float:
    """Median across ``InstructionProperties.duration`` (seconds) for
    operations of the given arity.  Same selector semantics as
    ``_median_instruction_error``.  Returns 0.0 when no duration data
    is available — generic Aer, or fakes whose Target lacks timings.
    """
    durations: list[float] = []
    for op_name in target.operation_names:
        if only is not None and op_name not in only:
            continue
        if skip is not None and op_name in skip:
            continue
        try:
            instr_map = target[op_name]
        except KeyError:
            continue
        for qubits, props in instr_map.items():
            if qubits is None or len(qubits) != arity:
                continue
            dur = getattr(props, "duration", None) if props is not None else None
            if dur is None:
                continue
            durations.append(float(dur))
    return statistics.median(durations) if durations else 0.0


def _median_instruction_error(
    target,  # type: ignore[no-untyped-def]
    *,
    arity: int,
    only: set[str] | None = None,
    skip: set[str] | None = None,
) -> float:
    """Median across InstructionProperties.error for operations of the given arity.

    ``only`` restricts to specific op names (e.g. {"measure"} for readout
    error); ``skip`` excludes them (e.g. skip {"measure"} when computing
    1-qubit gate error).  Returns 0.0 when no data is available — see
    BackendMetadata for the absence/zero convention.
    """
    errors: list[float] = []
    for op_name in target.operation_names:
        if only is not None and op_name not in only:
            continue
        if skip is not None and op_name in skip:
            continue
        try:
            instr_map = target[op_name]
        except KeyError:
            continue
        for qubits, props in instr_map.items():
            if qubits is None or len(qubits) != arity:
                continue
            err = getattr(props, "error", None) if props is not None else None
            if err is None:
                continue
            errors.append(float(err))
    return statistics.median(errors) if errors else 0.0
