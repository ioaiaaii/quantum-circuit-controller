"""IBM Quantum adapter (provider=`ibm`).

The adapter constructor raises `AdapterUnavailable` when `QISKIT_IBM_TOKEN`
is absent so the executor can ship the code path without holding a live IBM
account.  When the token is present, the adapter uses qiskit-ibm-runtime's
SamplerV2 primitive against the configured backend.

Channel selection: defaults to ``ibm_quantum_platform`` (the current IBM
Quantum Platform channel as of mid-2026; the older ``ibm_quantum`` channel
is deprecated).  Override with ``QISKIT_IBM_CHANNEL`` env var if needed
(e.g. for IBM Cloud accounts on the ``ibm_cloud`` channel).
"""

from __future__ import annotations

import os
from collections.abc import Mapping
from typing import Any

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


class IBMAdapter(Adapter):
    name = "ibm"

    def __init__(self, backend_name: str | None = None) -> None:
        token = os.environ.get("QISKIT_IBM_TOKEN")
        if not token:
            raise AdapterUnavailable(
                "IBM Quantum adapter requires QISKIT_IBM_TOKEN environment variable"
            )
        from qiskit_ibm_runtime import QiskitRuntimeService  # noqa: PLC0415 — lazy import

        # ``ibm_quantum_platform`` is the current channel for IBM Quantum
        # Platform accounts (open and paid plans).  Override via env var
        # for ``ibm_cloud`` accounts or future channel renames; this keeps
        # the adapter usable across IBM's evolving account topology
        # without re-deploying the executor image for a one-string change.
        channel = os.environ.get("QISKIT_IBM_CHANNEL", "ibm_quantum_platform")
        self.service = QiskitRuntimeService(channel=channel, token=token)
        # Backend name must be supplied — there is no sensible default
        # across plans (open plan grants different backends than paid).
        # The QPU CR's spec.backendName flows in via the adapter resolver.
        if not backend_name:
            raise AdapterUnavailable(
                "IBM Quantum adapter requires backend_name (set QPU.spec.backendName)"
            )
        self.backend_name = backend_name
        self.backend = self.service.backend(self.backend_name)

    def transpile(
        self,
        qasm: str,
        target,
        options: Mapping[str, Any] | None = None,
    ) -> TranspiledCircuit:
        from qiskit import qasm3, transpile

        qc = qasm3.loads(qasm)
        opt_level = getattr(target, "optimization_level", 1) or 1
        # Tier-2 passthrough: forward dict verbatim as kwargs.
        # See QCC-Design-State.md §7a Composition Principle Tier 2.
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
        from qiskit_ibm_runtime import SamplerV2

        sampler = SamplerV2(mode=self.backend)

        # Cross-boundary identifier linkage (QCC-Observability.md §6).
        # Stamp the Circuit's K8s UID onto IBM Quantum's runtime
        # options as a job tag, so a user in IBM Quantum Console can
        # resolve a job back to its owning Circuit.  Best-effort: if
        # the SDK version doesn't expose this attribute path, swallow
        # the failure rather than break the submission — the metric +
        # CRD-status path still works without the tag.
        if circuit_uid:
            try:
                tags = list(getattr(sampler.options.environment, "job_tags", None) or [])
                tags.append(f"qcc.circuit.uid:{circuit_uid}")
                sampler.options.environment.job_tags = tags
            except Exception:  # noqa: BLE001, S110 — best-effort, never break submit on this
                pass

        # SamplerV2.run accepts shots + an `options` dict (different from
        # the kwargs surface AerSimulator uses).  We forward Tier-2
        # execute_options as kwargs because that's what `run` signatures
        # accept across qiskit-ibm-runtime versions; sampler-specific
        # options (resilience_level, default_shots) belong on the
        # SamplerV2 constructor and would need a separate sampler_options
        # block — out of scope for the M3 passthrough.
        run_kwargs: dict[str, Any] = {"shots": shots}
        if options:
            run_kwargs.update(options)
            run_kwargs["shots"] = shots  # Tier-1 wins
        job = sampler.run([circuit.qiskit_circuit], **run_kwargs)
        return JobHandle(provider_job_id=job.job_id(), backend=self.backend_name, payload=job)

    def poll(self, handle: JobHandle) -> JobStatus:
        job = handle.payload
        status_name = job.status().name if hasattr(job.status(), "name") else str(job.status())
        if status_name == "DONE":
            return JobStatus.DONE
        if status_name in ("ERROR", "CANCELLED"):
            return JobStatus.FAILED
        return JobStatus.RUNNING

    def fetch_result(self, handle: JobHandle) -> FetchResult:
        job = handle.payload
        result = job.result()
        pub_result = result[0]
        # SamplerV2's DataBin holds one attribute per classical register
        # in the original circuit.  The attribute name depends on what
        # the circuit author wrote:
        #
        #   - `bit[2] c;` (this thesis's OpenQASM 3 examples)  → .c
        #   - `measure_all()` in Qiskit Python                 → .meas
        #   - `ClassicalRegister(2, "crz")`                    → .crz
        #   - multi-register circuits (Teleport's crz/crx/result) →
        #     one attribute per register; counts must be joined or
        #     selected — out of scope here, see below
        #
        # The previous code hard-coded `.meas`, which broke for the
        # OpenQASM 3 examples that name their register `c`.  Probe the
        # DataBin instead — `dir()` on it exposes the register names
        # alongside the BitArray methods, so find the first attribute
        # that exposes get_counts() and use that.
        counts = _extract_counts(pub_result.data)
        return FetchResult(
            counts={str(k): int(v) for k, v in counts.items()},
            usage_seconds=_extract_usage_seconds(job),
        )

    def inspect(self) -> BackendMetadata:
        """Pull live calibration from the configured IBM backend.

        Uses the same Target-introspection pattern as AerAdapter; the
        difference is that ``last_update_date`` reflects the most recent
        live IBM Cloud refresh rather than a frozen snapshot.
        """
        # Imported lazily to mirror the rest of this adapter (keep cold
        # paths cold when QISKIT_IBM_TOKEN is missing in M1).
        from .aer import (  # noqa: PLC0415 — intentional lazy import
            _backend_dt,
            _coherence_medians,
            _count_coupling_edges,
            _format_calibration_time,
            _median_instruction_duration,
            _median_instruction_error,
            _processor_identity,
        )

        target = self.backend.target
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

    def schedule(self, qasm: str, target) -> CircuitSchedule:
        """Schedule ``qasm`` against this IBM backend (real hardware target).

        Real IBM backends always carry a populated Target with
        durations and a dt — unlike generic Aer — so the path is the
        same one AerAdapter uses for ``fake_*`` backends.  Delegated to
        the Aer-side helpers so there's one source of truth for the
        op_start_times → ScheduledOp conversion.
        """
        from qiskit import qasm3, transpile  # noqa: PLC0415 — lazy import

        from .aer import (  # noqa: PLC0415 — intentional lazy import
            _backend_dt,
            _instruction_duration_dt,
        )

        try:
            qc = qasm3.loads(qasm)
        except Exception as exc:
            raise RuntimeError(f"Invalid OpenQASM 3: {exc}") from exc
        opt_level = getattr(target, "optimization_level", 1) or 1
        try:
            scheduled = transpile(
                qc,
                self.backend,
                optimization_level=opt_level,
                scheduling_method="asap",
            )
        except Exception as exc:  # noqa: BLE001
            raise RuntimeError(f"scheduling failed: {exc}") from exc

        starts = list(getattr(scheduled, "op_start_times", []) or [])
        backend_target = self.backend.target
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
                    name=name, qubits=qubits,
                    start_dt=int(start), duration_dt=int(duration_dt),
                ),
            )
            end = int(start) + int(duration_dt)
            if end > total:
                total = end

        return CircuitSchedule(
            ops=tuple(ops),
            total_duration_dt=total,
            dt_seconds=_backend_dt(self.backend),
            num_qubits=int(self.backend.num_qubits),
            backend_used=self.backend_name,
        )


def _extract_counts(data) -> Mapping[str, int]:  # type: ignore[no-untyped-def]
    """Find the first classical register on a SamplerV2 DataBin and return its counts.

    DataBin exposes one attribute per classical register declared in the
    circuit (``data.c``, ``data.meas``, ``data.crz``, …).  Each is a
    ``BitArray`` with a ``get_counts()`` method.  This helper iterates
    attributes and returns the counts from the first BitArray-like one
    it finds — sufficient for single-register circuits (Bell, GHZ, Shor's).
    Multi-register circuits (e.g. Teleport's separate crz/crx/result)
    return one register's counts here; full multi-register support
    requires the controller-side schema work (M2.5).

    Raises ``RuntimeError`` when no readable register is found — that's
    a real condition (circuit had no measurement) that should surface as
    a terminal failure on the Circuit, not be silently absorbed.
    """
    # Probe attributes that look like BitArrays (have get_counts).
    # Filter dunders and known non-data attributes; the DataBin doesn't
    # publish a stable name-list API across qiskit-ibm-runtime versions.
    for name in dir(data):
        if name.startswith("_"):
            continue
        try:
            attr = getattr(data, name)
        except AttributeError:
            continue
        if hasattr(attr, "get_counts"):
            return attr.get_counts()
    raise RuntimeError(
        "no classical register with get_counts() found on SamplerV2 DataBin — "
        "did the circuit declare measurements?",
    )


def _extract_usage_seconds(job: Any) -> float:  # type: ignore[no-untyped-def]
    """Best-effort extraction of substrate-reported billable compute time.

    Qiskit Runtime's ``RuntimeJobV2.usage()`` returns a ``float`` of
    quantum-seconds used; the older ``RuntimeJob.usage()`` returned a
    ``dict`` with ``quantum_seconds`` instead.  Some SDK versions only
    expose the value via ``job.metrics()['usage']['quantum_seconds']``.
    We try each shape in order and fall back to 0.0 on any failure —
    the value is observability-only and a missing reading must not
    fail the run.

    Returns 0.0 for adapters that don't report usage (the IBMAdapter
    caller is the only one wiring this up today); callers that need
    to distinguish "0 = simulator" from "0 = call failed" should use
    the surrounding adapter identity instead of inspecting the value.
    """
    try:
        usage = job.usage()
    except Exception:  # noqa: BLE001 — defensive; observability is best-effort
        usage = None

    # New shape: float (quantum-seconds) directly.
    if isinstance(usage, (int, float)):
        return float(usage)

    # Older shape: dict carrying quantum_seconds.
    if isinstance(usage, dict):
        for key in ("quantum_seconds", "seconds"):
            if key in usage and isinstance(usage[key], (int, float)):
                return float(usage[key])

    # Last-resort shape: job.metrics()['usage']['quantum_seconds'].
    try:
        metrics = job.metrics()
        if isinstance(metrics, dict):
            u = metrics.get("usage")
            if isinstance(u, dict):
                qs = u.get("quantum_seconds")
                if isinstance(qs, (int, float)):
                    return float(qs)
    except Exception:  # noqa: BLE001, S110
        pass

    return 0.0
