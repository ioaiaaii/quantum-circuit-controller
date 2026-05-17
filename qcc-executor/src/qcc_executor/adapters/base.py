"""Adapter contract for QRM vendor implementations.

Concrete adapters live in sibling modules (`aer.py`, `ibm.py`). The interface
matches the QCC system-design §7 "VendorAdapter Protocol" — transpile, submit,
poll, fetch_result — and is locked from M1 so swapping IBM for the local
simulator is configuration, not code.
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from collections.abc import Mapping
from dataclasses import dataclass
from enum import StrEnum
from typing import Any, Protocol


class AdapterUnavailable(RuntimeError):
    """Raised when an adapter cannot be constructed (missing credentials, etc.)."""


class JobStatus(StrEnum):
    PENDING = "PENDING"
    RUNNING = "RUNNING"
    DONE = "DONE"
    FAILED = "FAILED"


@dataclass
class TranspiledCircuit:
    qiskit_circuit: Any
    depth: int = 0
    two_qubit_gates: int = 0
    total_gates: int = 0


@dataclass
class JobHandle:
    provider_job_id: str
    backend: str
    payload: Any = None


@dataclass
class FetchResult:
    """Structured return type for Adapter.fetch_result.

    Carries the classical-outcome counts plus the substrate-reported
    billable compute time, so the controller can store both on
    Circuit.status without a second round-trip.

    ``usage_seconds`` is 0.0 when the substrate doesn't expose a
    usage handle (Aer / fake_* simulators) or when the API call
    fails for an unknown reason.  Distinct from 0.0 meaning "the
    QPU finished in less than 1 ms" — for real-hardware substrates
    that's effectively impossible at thesis-scale shot counts, so
    we treat 0.0 as a "not reported" sentinel on the controller side
    (the metric is emitted only when value > 0).
    """

    counts: Mapping[str, int]
    usage_seconds: float = 0.0


@dataclass(frozen=True)
class ScheduledOp:
    """One scheduled instruction in dt cycles, mirroring the proto type."""

    name: str
    qubits: tuple[int, ...]  # physical (post-layout) qubit indices
    start_dt: int            # start time in dt cycles
    duration_dt: int         # duration in dt cycles; 0 when not reported


@dataclass
class CircuitSchedule:
    """Adapter-side result of ScheduleCircuit, before proto serialisation.

    All times are in *dt cycles* — adapters never multiply by dt
    themselves, because the gRPC layer carries dt_seconds separately
    and the controller (or thesis) is the right place to convert.
    """

    ops: tuple[ScheduledOp, ...] = ()
    total_duration_dt: int = 0
    dt_seconds: float = 0.0
    num_qubits: int = 0
    backend_used: str = ""


@dataclass
class BackendMetadata:
    """Calibration-relevant introspection result returned by Adapter.inspect.

    Mirrors ProbeBackendResponse on the wire and ``QPU.status`` on the
    Kubernetes side: the QPUReconciler stamps these fields directly onto
    the resource so selection (M2) reads authoritative values rather
    than user-authored ones.  Floats are population medians in [0, 1];
    0 means "not reported by this backend" (the controller treats
    absence as "skip in scoring," never as "perfect").
    """

    num_qubits: int = 0
    basis_gates: tuple[str, ...] = ()
    coupling_edges: int = 0
    last_calibration_time: str = ""      # RFC 3339 / ISO 8601, "" if N/A
    single_qubit_error_median: float = 0.0
    two_qubit_error_median: float = 0.0
    readout_error_median: float = 0.0
    # Coherence times in microseconds — T1 = energy relaxation, T2 =
    # dephasing.  Set the coherence budget the executor's transpilation
    # has to live within.  Zero when the backend has no qubit_properties
    # (generic Aer) or when the field is missing (older fakes).
    t1_median_us: float = 0.0
    t2_median_us: float = 0.0
    # dt_seconds is the control-electronics cycle period — the smallest
    # time quantum the AWG can address.  Typical IBM: ~0.222 ns.  Zero
    # when the backend doesn't expose dt (generic Aer).  All gate
    # durations are integer multiples of dt; together they let the
    # controller estimate exec time from circuit depth.
    dt_seconds: float = 0.0
    # Per-instruction duration medians in seconds (single-qubit gates
    # excluding measure; two-qubit gates).  Zero when no durations are
    # reported.  Used to derive estimated execution time as
    # depth × max(1Q, 2Q duration) — a critical-path lower bound.
    single_qubit_duration_median_seconds: float = 0.0
    two_qubit_duration_median_seconds: float = 0.0
    # Hardware family identifiers from Qiskit's backend.processor_type.
    # processor_family: chip generation, e.g. "Eagle", "Heron", "Falcon".
    # processor_revision: stringified revision number ("3", "2", "1").
    # processor_segment: optional segment label (e.g. "T" for the
    # Falcon T-segment); "" when absent.  All three are "" for backends
    # without processor_type metadata (generic Aer).  Surfaced so the
    # selection logic can prefer one family over another and the thesis
    # can cite chip generations from system data rather than inference.
    processor_family: str = ""
    processor_revision: str = ""
    processor_segment: str = ""


class TargetLike(Protocol):
    provider: str
    backend_name: str
    optimization_level: int


class Adapter(ABC):
    name: str = ""

    @abstractmethod
    def transpile(
        self,
        qasm: str,
        target: TargetLike,
        options: Mapping[str, Any] | None = None,
    ) -> TranspiledCircuit:
        """Transpile ``qasm`` against ``target`` and return the result.

        ``options`` is the Tier-2 passthrough block from
        Circuit.spec.transpile (QCC-Design-State.md §7a Composition
        Principle).  Keys are forwarded verbatim as kwargs to
        ``qiskit.compiler.transpile`` — adapters must NOT translate them.
        Unknown keys propagate to Qiskit's own validation and surface as
        terminal ``TranspilationFailed`` failures.  Tier-1 fields
        (optimization_level) already on ``target`` take precedence; a
        Tier-2 override of optimization_level wins because Qiskit
        resolves the last kwarg.
        """

    @abstractmethod
    def submit(
        self,
        circuit: TranspiledCircuit,
        shots: int,
        options: Mapping[str, Any] | None = None,
        circuit_uid: str = "",
    ) -> JobHandle:
        """Submit ``circuit`` for execution with ``shots`` repetitions.

        ``circuit_uid`` is the Circuit's K8s UID, plumbed through from
        the servicer for cross-boundary identifier linkage
        (`QCC-Observability.md` §6).  Adapters that submit to a vendor
        with a tag-style metadata surface (e.g. IBM Quantum's
        ``runtime_options.tags``) stamp ``qcc.circuit.uid:<uid>`` onto
        the job so a user in IBM Quantum Console can resolve a job
        back to its owning Circuit.  Empty string when the controller
        doesn't pass it (e.g. legacy gRPC clients) — adapters MUST
        treat empty as "skip the stamp", never as a literal value.

        ``options`` is the Tier-2 passthrough block from
        Circuit.spec.execute, forwarded verbatim to the adapter's
        execute-stage call site (``SamplerV2.run`` for IBM,
        ``AerSimulator.run`` for Aer).  ``shots`` is Tier-1 and must
        win over a redundant ``shots`` key in ``options`` — the
        servicer drops shots from execute_options at decode time.
        """

    @abstractmethod
    def poll(self, handle: JobHandle) -> JobStatus: ...

    @abstractmethod
    def fetch_result(self, handle: JobHandle) -> FetchResult: ...

    @abstractmethod
    def inspect(self) -> BackendMetadata:
        """Return calibration-relevant metadata for the resolved backend.

        Called by the ProbeBackend gRPC handler — never during the job
        lifecycle.  No shots, no submissions, no side effects.  The
        controller may invoke this on every QPU reconcile, so the
        implementation must be fast (target object reads only) and
        deterministic (no clock-dependent answers for fake backends).
        """
        ...

    @abstractmethod
    def schedule(self, qasm: str, target: TargetLike) -> CircuitSchedule:
        """Return the scheduled per-instruction timeline for the source.

        Re-transpiles with ``scheduling_method='asap'`` and walks
        ``op_start_times`` plus the backend's Target durations to
        produce a structured timeline in dt cycles.  Called by the
        ScheduleCircuit gRPC handler — no shots consumed, no remote job.

        Adapters that don't support scheduled transpilation (generic
        AerSimulator, today) may raise ``RuntimeError``; the servicer
        surfaces that as TASK_STATUS_FAILED with
        error_reason="SchedulingUnsupported".
        """
        ...
