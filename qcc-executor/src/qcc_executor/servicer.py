"""ExecutorServicer — gRPC method implementations.

Separated from server.py so the class can be unit-tested without constructing
a server.  Implements:

  - RunCircuit     — synchronous transpile + submit + fetch (Aer-friendly)
  - SubmitTask     — async submit; returns task_id immediately (real-hardware path)
  - WatchTask      — streaming status updates until terminal
  - FetchTaskResult — counts retrieval for a terminal task
  - ConvertSource  — translate any supported source format to OpenQASM 3
  - DrawCircuit    — render an ASCII drawing of the circuit
  - ScheduleCircuit — produce the scheduled timeline artifact
  - ProbeBackend   — introspect backend metadata for QPU status

The async trio maintains an in-memory `{task_id → (adapter, handle)}`
registry inside this servicer process.  Pod restarts lose that registry;
for thesis-PoC scope this is acceptable (the user re-submits).  Production
hardening (Redis-backed registry, retrieve-from-vendor-by-job-id) is
future-work — see `QCC-Design-State.md` §12 parked questions.
"""

from __future__ import annotations

import logging
import threading
import time
from typing import Any

import grpc
from google.protobuf import json_format
from google.protobuf import struct_pb2

from qcc_executor import qiskit_io
from qcc_executor.adapters import (
    Adapter,
    AdapterUnavailable,
    JobHandle,
    JobStatus,
    get_adapter,
)
from qcc_executor.protostubs import executor_pb2, executor_pb2_grpc

LOG = logging.getLogger(__name__)


def _circuit_uid_from_idempotency(idempotency_key: str) -> str:
    """Extract the Circuit's K8s UID from the controller's idempotency key.

    The controller derives ``idempotency_key`` from the Circuit's
    ``metadata.uid`` plus ``status.observedGeneration`` as
    ``"<uid>/<gen>"`` (see
    ``internal/controller/circuit_controller.go``'s
    ``runOnExecutor``).  Adapters that want to surface the UID
    (e.g. ``IBMAdapter`` stamping ``runtime_options.tags`` per
    ``QCC-Observability.md`` §6) read it from here so we don't have
    to grow the gRPC schema with a redundant field.

    Returns empty string when the key doesn't follow the
    ``<uid>/<gen>`` shape — callers must treat empty as "no UID
    available, skip cross-boundary stamping".
    """
    if not idempotency_key or "/" not in idempotency_key:
        return ""
    return idempotency_key.split("/", 1)[0]


def _struct_to_dict(s: struct_pb2.Struct | None) -> dict[str, Any]:
    """Decode a protobuf Struct into a plain Python dict.

    Returns an empty dict for an unset / empty Struct so adapters don't
    have to None-check.  Uses ``json_format.MessageToDict`` to flatten
    nested ``Struct`` / ``ListValue`` to plain dict/list/scalar — Qiskit
    kwargs can't accept the raw protobuf types.

    Numeric coercion: protobuf Struct's NumberValue is double-only, so
    an integer literal in the user's YAML (`seed_transpiler: 7`) arrives
    here as ``7.0``.  Qiskit's strict signatures reject that
    (``ValueError: Expected non-negative integer``).  We walk the decoded
    tree and convert whole-number floats to ``int``.  This is a
    deliberate, documented coercion at the wire boundary — the alternative
    (forcing users to set ``seed_transpiler: 7.0`` in YAML) violates the
    "pass snake_case to Qiskit unchanged" promise of Tier 2.  Decimal
    arguments survive because they're not whole numbers.
    """
    if s is None or len(s.fields) == 0:
        return {}
    raw = json_format.MessageToDict(s, preserving_proto_field_name=True)
    return _coerce_integers(raw)


def _coerce_integers(value: Any) -> Any:
    """Recursively convert whole-number floats to ints.

    See ``_struct_to_dict`` for why this exists.  Bools are subclasses
    of int in Python — explicitly preserve them so ``True`` doesn't
    become ``1``.
    """
    if isinstance(value, bool):
        return value
    if isinstance(value, float) and value.is_integer():
        return int(value)
    if isinstance(value, dict):
        return {k: _coerce_integers(v) for k, v in value.items()}
    if isinstance(value, list):
        return [_coerce_integers(v) for v in value]
    return value


def _strip_tier1(options: dict[str, Any], tier1_keys: set[str]) -> dict[str, Any]:
    """Drop Tier-1 keys from a passthrough options dict.

    Tier-1 fields (shots, etc.) live on dedicated TaskSpec fields and
    must not be overridable via passthrough — that would be two sources
    of truth for the same parameter.  We strip silently and log a
    warning rather than rejecting the request, because the user's
    intent is recoverable (they wanted the Tier-1 value either way)
    and a hard rejection on the M3 happy path adds friction without
    benefit.
    """
    if not options:
        return options
    leaked = [k for k in tier1_keys if k in options]
    if not leaked:
        return options
    LOG.warning(
        "Tier-2 passthrough leaked Tier-1 keys; ignoring: %s",
        leaked,
    )
    return {k: v for k, v in options.items() if k not in tier1_keys}


class _OptTarget:
    """Pass-through view onto a TaskSpec for adapters that don't depend on protobuf."""

    def __init__(self, spec: executor_pb2.TaskSpec) -> None:
        self.provider = spec.target.provider
        self.backend_name = spec.target.backend_name
        self.optimization_level = (
            spec.optimization_level if spec.HasField("optimization_level") else 1
        )


class ExecutorServicer(executor_pb2_grpc.ExecutorServicer):
    # Status mapping from adapter's JobStatus enum to the proto enum.
    # JobStatus is shared across all adapters (see adapters/base.py).
    _STATUS_MAP = {
        JobStatus.PENDING: executor_pb2.TASK_STATUS_PENDING,
        JobStatus.RUNNING: executor_pb2.TASK_STATUS_RUNNING,
        JobStatus.DONE:    executor_pb2.TASK_STATUS_DONE,
        JobStatus.FAILED:  executor_pb2.TASK_STATUS_FAILED,
    }
    # Terminal statuses end the WatchTask stream loop.
    _TERMINAL = {JobStatus.DONE, JobStatus.FAILED}

    # Poll cadence for the WatchTask streaming loop.  Real-hardware jobs
    # queue for minutes; polling once a second is too aggressive for the
    # provider API and creates noise in the executor logs.  5s is a
    # reasonable middle ground — the controller's reconciler also polls
    # at a similar cadence, so we're not adding meaningful latency.
    _WATCH_POLL_INTERVAL_S = 5.0
    # Hard ceiling on a WatchTask call.  If the job hasn't reached
    # terminal in this time, the stream ends and the client (controller)
    # reconnects.  Prevents long-lived gRPC streams that K8s sidecars
    # or service-meshes might cull.
    _WATCH_MAX_DURATION_S = 30 * 60.0

    def __init__(self) -> None:
        # In-memory task registry: provider_job_id → (adapter, handle).
        # Populated by SubmitTask; read by WatchTask + FetchTaskResult.
        # Cleaned up by FetchTaskResult after counts are returned (the
        # registry is for in-flight tasks only; results don't outlive
        # the fetch).  Thread-safe access via _tasks_lock — gRPC
        # invokes handlers on a worker pool so concurrent SubmitTask /
        # WatchTask / FetchTaskResult are real.
        self._tasks: dict[str, tuple[Adapter, JobHandle]] = {}
        self._tasks_lock = threading.Lock()

    def RunCircuit(  # noqa: N802 (gRPC method casing)
        self, request: executor_pb2.RunCircuitRequest, context: grpc.ServicerContext
    ) -> executor_pb2.RunCircuitResponse:
        spec = request.spec
        LOG.info(
            "RunCircuit request",
            extra={
                "idempotency_key": spec.idempotency_key,
                "provider": spec.target.provider,
                "shots": spec.shots,
            },
        )
        try:
            adapter = get_adapter(spec.target.provider, spec.target.backend_name)
        except AdapterUnavailable as exc:
            LOG.warning("Adapter unavailable: %s", exc)
            return executor_pb2.RunCircuitResponse(
                status=executor_pb2.TASK_STATUS_FAILED,
                error_reason="NoEligibleBackend",
                error_message=str(exc),
            )

        target = _OptTarget(spec)
        transpile_options = _struct_to_dict(spec.transpile_options)
        # shots is Tier-1 — strip it from execute_options so it cannot
        # accidentally override the dedicated field.  Logged as a warning
        # (not an error) so the user sees the smell without the call
        # failing on a configuration nit.
        execute_options = _strip_tier1(_struct_to_dict(spec.execute_options), {"shots"})

        try:
            transpiled = adapter.transpile(spec.qasm, target, options=transpile_options)
        except Exception as exc:
            LOG.exception("Transpilation failed")
            return executor_pb2.RunCircuitResponse(
                status=executor_pb2.TASK_STATUS_FAILED,
                error_reason="TranspilationFailed",
                error_message=str(exc),
            )

        try:
            handle = adapter.submit(
                transpiled, spec.shots,
                options=execute_options,
                circuit_uid=_circuit_uid_from_idempotency(spec.idempotency_key),
            )
            fetched = adapter.fetch_result(handle)
        except Exception as exc:
            LOG.exception("Provider submission failed")
            return executor_pb2.RunCircuitResponse(
                status=executor_pb2.TASK_STATUS_FAILED,
                error_reason="ProviderSubmissionFailed",
                error_message=str(exc),
            )

        return executor_pb2.RunCircuitResponse(
            status=executor_pb2.TASK_STATUS_DONE,
            task_id=handle.provider_job_id,
            counts={str(k): int(v) for k, v in fetched.counts.items()},
            transpile=executor_pb2.TranspileMetadata(
                depth=transpiled.depth,
                two_qubit_gates=transpiled.two_qubit_gates,
                total_gates=transpiled.total_gates,
            ),
            backend_used=handle.backend,
            usage_seconds=fetched.usage_seconds,
        )

    # --- Pure transforms (no provider interaction) ---

    def ConvertSource(  # noqa: N802
        self, request: executor_pb2.ConvertSourceRequest, context: grpc.ServicerContext
    ) -> executor_pb2.ConvertSourceResponse:
        LOG.info(
            "ConvertSource request",
            extra={"format": request.source.format, "body_len": len(request.source.body)},
        )
        try:
            qc = qiskit_io.load_circuit(request.source.format, request.source.body)
            qasm = qiskit_io.dump_qasm(qc)
        except qiskit_io.SourceError as exc:
            LOG.warning("ConvertSource failed: %s", exc)
            return executor_pb2.ConvertSourceResponse(
                status=executor_pb2.TASK_STATUS_FAILED,
                error_reason="SourceConversionFailed",
                error_message=str(exc),
            )
        return executor_pb2.ConvertSourceResponse(
            qasm=qasm,
            status=executor_pb2.TASK_STATUS_DONE,
        )

    def ScheduleCircuit(  # noqa: N802
        self, request: executor_pb2.ScheduleCircuitRequest, context: grpc.ServicerContext
    ) -> executor_pb2.ScheduleCircuitResponse:
        spec_target = request.target
        LOG.info(
            "ScheduleCircuit request",
            extra={
                "provider": spec_target.provider,
                "backend_name": spec_target.backend_name,
            },
        )
        try:
            adapter = get_adapter(spec_target.provider, spec_target.backend_name)
        except AdapterUnavailable as exc:
            LOG.warning("Adapter unavailable for schedule: %s", exc)
            return executor_pb2.ScheduleCircuitResponse(
                status=executor_pb2.TASK_STATUS_FAILED,
                error_reason="NoEligibleBackend",
                error_message=str(exc),
            )

        # Reuse _OptTarget so adapters get the same TargetLike object
        # they see from RunCircuit, despite ScheduleCircuitRequest
        # being a different message shape.
        class _ScheduleTarget:
            def __init__(self, req: executor_pb2.ScheduleCircuitRequest) -> None:
                self.provider = req.target.provider
                self.backend_name = req.target.backend_name
                self.optimization_level = (
                    req.optimization_level if req.HasField("optimization_level") else 1
                )

        target = _ScheduleTarget(request)

        try:
            sched = adapter.schedule(request.source.body, target)
        except Exception as exc:  # noqa: BLE001 — terminal scheduling failure
            LOG.exception("Scheduling failed")
            # Distinguish "this backend doesn't support scheduling" from
            # generic transpile errors — the message from AerAdapter
            # starts with "scheduling unsupported" when the target lacks
            # durations.  The CLI surfaces the reason to the user.
            reason = "SchedulingUnsupported" if "scheduling unsupported" in str(exc) \
                else "SchedulingFailed"
            return executor_pb2.ScheduleCircuitResponse(
                status=executor_pb2.TASK_STATUS_FAILED,
                error_reason=reason,
                error_message=str(exc),
            )

        return executor_pb2.ScheduleCircuitResponse(
            ops=[
                executor_pb2.ScheduledOp(
                    name=op.name,
                    qubits=list(op.qubits),
                    start_dt=op.start_dt,
                    duration_dt=op.duration_dt,
                )
                for op in sched.ops
            ],
            total_duration_dt=sched.total_duration_dt,
            dt_seconds=sched.dt_seconds,
            num_qubits=sched.num_qubits,
            backend_used=sched.backend_used,
            status=executor_pb2.TASK_STATUS_DONE,
        )

    def DrawCircuit(  # noqa: N802
        self, request: executor_pb2.DrawCircuitRequest, context: grpc.ServicerContext
    ) -> executor_pb2.DrawCircuitResponse:
        LOG.info(
            "DrawCircuit request",
            extra={"format": request.source.format, "body_len": len(request.source.body)},
        )
        try:
            qc = qiskit_io.load_circuit(request.source.format, request.source.body)
            drawing = qiskit_io.draw_text(qc)
        except qiskit_io.SourceError as exc:
            LOG.warning("DrawCircuit failed: %s", exc)
            return executor_pb2.DrawCircuitResponse(
                status=executor_pb2.TASK_STATUS_FAILED,
                error_reason="RenderingFailed",
                error_message=str(exc),
            )
        return executor_pb2.DrawCircuitResponse(
            drawing=drawing,
            status=executor_pb2.TASK_STATUS_DONE,
        )

    # --- Backend introspection (read-only, no shots) ---

    def ProbeBackend(  # noqa: N802
        self, request: executor_pb2.ProbeBackendRequest, context: grpc.ServicerContext
    ) -> executor_pb2.ProbeBackendResponse:
        LOG.info(
            "ProbeBackend request",
            extra={"provider": request.provider, "backend_name": request.backend_name},
        )
        try:
            adapter = get_adapter(request.provider, request.backend_name)
        except AdapterUnavailable as exc:
            LOG.warning("Adapter unavailable for probe: %s", exc)
            return executor_pb2.ProbeBackendResponse(
                status=executor_pb2.TASK_STATUS_FAILED,
                error_reason="NoEligibleBackend",
                error_message=str(exc),
            )

        try:
            meta = adapter.inspect()
        except Exception as exc:  # noqa: BLE001 — surface as terminal probe failure
            LOG.exception("Backend introspection failed")
            return executor_pb2.ProbeBackendResponse(
                status=executor_pb2.TASK_STATUS_FAILED,
                error_reason="ProviderProbeFailed",
                error_message=str(exc),
            )

        return executor_pb2.ProbeBackendResponse(
            status=executor_pb2.TASK_STATUS_DONE,
            num_qubits=meta.num_qubits,
            basis_gates=list(meta.basis_gates),
            coupling_edges=meta.coupling_edges,
            last_calibration_time=meta.last_calibration_time,
            single_qubit_error_median=meta.single_qubit_error_median,
            two_qubit_error_median=meta.two_qubit_error_median,
            readout_error_median=meta.readout_error_median,
            t1_median_us=meta.t1_median_us,
            t2_median_us=meta.t2_median_us,
            dt_seconds=meta.dt_seconds,
            single_qubit_duration_median_seconds=meta.single_qubit_duration_median_seconds,
            two_qubit_duration_median_seconds=meta.two_qubit_duration_median_seconds,
            processor_family=meta.processor_family,
            processor_revision=meta.processor_revision,
            processor_segment=meta.processor_segment,
        )

    # --- Async task-lifecycle RPCs ---
    #
    # Together with `RunCircuit` (sync) they cover the two execution
    # surfaces.  `RunCircuit` blocks until terminal — suits Aer + short
    # fake-* runs.  The async trio decouples submit / poll / fetch, which
    # is required for real-hardware where jobs queue and run for minutes.
    # The task registry on this servicer instance carries the in-flight
    # handles between calls.

    def SubmitTask(  # noqa: N802
        self, request: executor_pb2.SubmitTaskRequest, context: grpc.ServicerContext
    ) -> executor_pb2.SubmitTaskResponse:
        spec = request.spec
        LOG.info(
            "SubmitTask request",
            extra={
                "idempotency_key": spec.idempotency_key,
                "provider": spec.target.provider,
                "backend_name": spec.target.backend_name,
                "shots": spec.shots,
            },
        )
        try:
            adapter = get_adapter(spec.target.provider, spec.target.backend_name)
        except AdapterUnavailable as exc:
            LOG.warning("SubmitTask adapter unavailable: %s", exc)
            # Async failures still travel as a successful gRPC call with
            # task_id="" — the controller treats empty task_id as
            # "submission rejected" and surfaces error_reason via the
            # FetchTaskResult path.  For SubmitTask itself the cleanest
            # signal is to abort with details so the controller can map
            # to ProviderSubmissionFailed in one place.
            context.set_code(grpc.StatusCode.FAILED_PRECONDITION)
            context.set_details(f"NoEligibleBackend: {exc}")
            return executor_pb2.SubmitTaskResponse()

        target = _OptTarget(spec)
        transpile_options = _struct_to_dict(spec.transpile_options)
        execute_options = _strip_tier1(_struct_to_dict(spec.execute_options), {"shots"})

        try:
            transpiled = adapter.transpile(spec.qasm, target, options=transpile_options)
        except Exception as exc:  # noqa: BLE001
            LOG.exception("SubmitTask transpilation failed")
            context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
            context.set_details(f"TranspilationFailed: {exc}")
            return executor_pb2.SubmitTaskResponse()

        try:
            handle = adapter.submit(
                transpiled, spec.shots,
                options=execute_options,
                circuit_uid=_circuit_uid_from_idempotency(spec.idempotency_key),
            )
        except Exception as exc:  # noqa: BLE001
            LOG.exception("SubmitTask provider submission failed")
            context.set_code(grpc.StatusCode.UNAVAILABLE)
            context.set_details(f"ProviderSubmissionFailed: {exc}")
            return executor_pb2.SubmitTaskResponse()

        # Register the handle so subsequent WatchTask / FetchTaskResult
        # calls can find it.  Keyed on provider_job_id, which the
        # controller stamps onto Circuit.status.providerJobID and uses
        # in subsequent task_id-carrying requests.
        with self._tasks_lock:
            self._tasks[handle.provider_job_id] = (adapter, handle)

        LOG.info(
            "SubmitTask accepted",
            extra={
                "task_id": handle.provider_job_id,
                "backend_used": handle.backend,
            },
        )
        return executor_pb2.SubmitTaskResponse(
            task_id=handle.provider_job_id,
            backend_used=handle.backend,
            transpile=executor_pb2.TranspileMetadata(
                depth=transpiled.depth,
                two_qubit_gates=transpiled.two_qubit_gates,
                total_gates=transpiled.total_gates,
            ),
        )

    def WatchTask(  # noqa: N802
        self, request: executor_pb2.WatchTaskRequest, context: grpc.ServicerContext
    ):
        task_id = request.task_id
        LOG.info("WatchTask started", extra={"task_id": task_id})

        with self._tasks_lock:
            entry = self._tasks.get(task_id)
        if entry is None:
            context.set_code(grpc.StatusCode.NOT_FOUND)
            context.set_details(f"TaskNotFound: no registered task with id {task_id!r}")
            return

        adapter, handle = entry

        # Stream status updates at _WATCH_POLL_INTERVAL_S cadence until
        # terminal or _WATCH_MAX_DURATION_S elapses (whichever first).
        # The controller's reconciler tolerates the stream ending with a
        # non-terminal status — it'll reopen WatchTask on the next
        # reconcile.  This keeps individual gRPC streams short-lived
        # (sidecar-friendly) while still letting the client see status
        # transitions promptly.
        deadline = time.monotonic() + self._WATCH_MAX_DURATION_S
        while time.monotonic() < deadline:
            if not context.is_active():
                # Client (controller) cancelled / disconnected.  Stop polling.
                LOG.info("WatchTask client disconnected", extra={"task_id": task_id})
                return

            try:
                status = adapter.poll(handle)
            except Exception as exc:  # noqa: BLE001 — surface as terminal FAILED
                LOG.exception("WatchTask poll failed")
                yield executor_pb2.WatchTaskResponse(
                    status=executor_pb2.TASK_STATUS_FAILED,
                    message=f"poll failed: {exc}",
                )
                return

            proto_status = self._STATUS_MAP.get(
                status, executor_pb2.TASK_STATUS_UNSPECIFIED,
            )
            yield executor_pb2.WatchTaskResponse(
                status=proto_status,
                message=self._format_status_message(adapter, handle, status),
            )

            if status in self._TERMINAL:
                LOG.info(
                    "WatchTask reached terminal",
                    extra={"task_id": task_id, "status": status.value},
                )
                return

            time.sleep(self._WATCH_POLL_INTERVAL_S)

        LOG.info("WatchTask max duration reached", extra={"task_id": task_id})

    def FetchTaskResult(  # noqa: N802
        self, request: executor_pb2.FetchTaskResultRequest, context: grpc.ServicerContext
    ) -> executor_pb2.FetchTaskResultResponse:
        task_id = request.task_id
        LOG.info("FetchTaskResult request", extra={"task_id": task_id})

        with self._tasks_lock:
            entry = self._tasks.get(task_id)
        if entry is None:
            return executor_pb2.FetchTaskResultResponse(
                status=executor_pb2.TASK_STATUS_FAILED,
                error_reason="TaskNotFound",
                error_message=f"no registered task with id {task_id!r}",
            )

        adapter, handle = entry

        try:
            fetched = adapter.fetch_result(handle)
        except Exception as exc:  # noqa: BLE001
            LOG.exception("FetchTaskResult failed")
            return executor_pb2.FetchTaskResultResponse(
                status=executor_pb2.TASK_STATUS_FAILED,
                error_reason="ProviderSubmissionFailed",
                error_message=str(exc),
            )

        # Result delivered; remove from registry so the executor doesn't
        # accumulate stale handles indefinitely.  If the controller calls
        # FetchTaskResult a second time (idempotency retries, replays),
        # the task is gone — but the controller has the counts on
        # Circuit.status.results so it shouldn't need to.
        with self._tasks_lock:
            self._tasks.pop(task_id, None)

        return executor_pb2.FetchTaskResultResponse(
            status=executor_pb2.TASK_STATUS_DONE,
            counts={str(k): int(v) for k, v in fetched.counts.items()},
            usage_seconds=fetched.usage_seconds,
        )

    def _format_status_message(
        self,
        adapter: Adapter,
        handle: JobHandle,
        status: JobStatus,
    ) -> str:
        """Build a human-readable WatchTaskResponse.message.

        The proto declares message as adapter-supplied freeform text —
        queue position, running step, vendor-side status detail.  We try
        to surface useful info per status; falls back to a generic string
        when the adapter doesn't expose extras.
        """
        # Queue position is the most useful signal during PENDING.  IBM
        # SamplerV2 Job exposes it as job.queue_position() (returns int
        # or None).  Cheap try; bare except because the API varies.
        if status == JobStatus.PENDING and handle.payload is not None:
            try:
                pos = handle.payload.queue_position()
                if pos is not None:
                    return f"queued, position {pos}"
            except Exception:  # noqa: BLE001 — best-effort, never fail the stream
                pass
        if status == JobStatus.RUNNING:
            return "executing on backend"
        if status == JobStatus.DONE:
            return "completed"
        if status == JobStatus.FAILED:
            return "failed"
        return ""
