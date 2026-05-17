/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package executor is the controller-side gRPC client for the qcc-executor
// service.  It wraps the generated stubs so callers in internal/controller/
// work in domain types (qccv1alpha1) rather than directly against protobuf.
package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	qccv1alpha1 "github.com/ioaiaaii/quantum-circuit-controller/api/v1alpha1"
	executorv1 "github.com/ioaiaaii/quantum-circuit-controller/gen/proto/qcc/executor/v1"
)

// DefaultAddr is the fallback controller→executor address used only when
// QCC_EXECUTOR_ADDR is not set.  In a real deployment the controller dials
// the in-cluster ClusterIP Service (e.g.
// quantum-circuit-controller-executor.<ns>.svc:9000); loopback is kept as
// the default purely as a convenience for unit tests and local runs.
const DefaultAddr = "127.0.0.1:9000"

// BackendProfile is the controller-facing view of an executor backend's
// calibration-relevant metadata, returned by ProbeBackend.  Mirrors
// ProbeBackendResponse on the wire and QPU.status.{qubits,basisGates,...}
// on the Kubernetes side.  Floats are population medians in [0, 1];
// zero means "not reported by this backend" — selection scoring (M2)
// treats absence as "skip," never as "perfect."
type BackendProfile struct {
	NumQubits              int32
	BasisGates             []string
	CouplingEdges          int32
	LastCalibrationTime    string // RFC 3339 / ISO 8601, "" if N/A
	SingleQubitErrorMedian float64
	TwoQubitErrorMedian    float64
	ReadoutErrorMedian     float64
	// Coherence-time medians in microseconds.  Zero when the backend
	// reports no qubit-coherence data (generic Aer).
	T1MedianMicros float64
	T2MedianMicros float64
	// DtSeconds is the backend's control-electronics cycle period.
	// Typical IBM: ~2.22e-10 s (0.222 ns).  Zero when not reported
	// (generic Aer).  Pair with the duration medians below to estimate
	// circuit execution time from depth.
	DtSeconds float64
	// Per-instruction duration medians in *seconds* — used to derive an
	// estimated execution-time floor from circuit depth (Move 4 layout
	// scoring in M2 will use these against T1/T2 for coherence-budget).
	SingleQubitDurationMedianSeconds float64
	TwoQubitDurationMedianSeconds    float64
	// Processor family identifiers from Qiskit's backend.processor_type.
	// Empty strings when the backend has no processor_type metadata
	// (generic Aer).  ProcessorRevision is a string because Qiskit
	// reports a mix of int and string revisions across families and we
	// normalise at the wire boundary.
	ProcessorFamily   string
	ProcessorRevision string
	ProcessorSegment  string
}

// ScheduledOp is one entry in the per-instruction schedule, in dt cycles.
// Mirrors the proto ScheduledOp and is JSON-serialisable so the artifact
// ConfigMap round-trips it cleanly to the CLI renderer.
type ScheduledOp struct {
	Name       string   `json:"name"`
	Qubits     []uint32 `json:"qubits"`
	StartDt    uint64   `json:"startDt"`
	DurationDt uint64   `json:"durationDt"`
}

// ScheduleResult is the controller-facing view of ScheduleCircuit's response.
// The JSON tags double as the on-wire format for the ConfigMap artifact
// (data["schedule.json"]) so the CLI can decode without redundant types.
type ScheduleResult struct {
	Ops             []ScheduledOp `json:"ops"`
	TotalDurationDt uint64        `json:"totalDurationDt"`
	DtSeconds       float64       `json:"dtSeconds"`
	NumQubits       uint32        `json:"numQubits"`
	BackendUsed     string        `json:"backendUsed"`
}

// Result is the controller-facing view of a single Circuit execution.
type Result struct {
	TaskID        string
	BackendUsed   string
	Counts        map[string]int64
	Depth         uint32
	TwoQubitGates uint32
	TotalGates    uint32

	// UsageSeconds is the substrate-reported billable compute time for
	// the execution (Qiskit Runtime Job.usage()).  Zero when the
	// substrate doesn't expose usage info (Aer / fake_*) or when the
	// API call failed.  See proto/qcc/executor/v1/executor.proto.
	UsageSeconds float64

	// ConvertedQASM holds the OpenQASM 3 form of a qiskit-format source,
	// populated as a byproduct of ConvertSource when the input was
	// SourceQiskit.  Empty for SourceOpenQASM3 inputs (the input *is* the
	// QASM — nothing to expose separately).  The controller stores this in
	// a sibling ConfigMap and surfaces it via Circuit.status.convertedRef
	// so users can inspect what was actually submitted.
	ConvertedQASM string
}

// TaskResult is the controller-facing view of a finished async task's
// payload, returned by FetchTaskResult.  Replaces the bare counts map
// so usage_seconds (and any future result metadata) ride along on the
// same RPC without requiring extra round trips.
type TaskResult struct {
	Counts       map[string]int64
	UsageSeconds float64
}

// TaskError signals an executor-reported failure with a Circuit condition
// reason (see api/v1alpha1.Reason*).  It is distinct from gRPC transport
// errors, which the caller treats as transient.
type TaskError struct {
	Reason  string
	Message string
}

func (e *TaskError) Error() string {
	return fmt.Sprintf("%s: %s", e.Reason, e.Message)
}

// Client owns a single long-lived gRPC connection to the executor service.
type Client struct {
	conn *grpc.ClientConn
	stub executorv1.ExecutorClient
}

// Dial creates a lazy gRPC connection to the executor at addr.  The connection
// is not established until the first RPC, so this never blocks at controller
// startup.  The caller is responsible for calling Close.
func Dial(addr string) (*Client, error) {
	if addr == "" {
		addr = DefaultAddr
	}
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("create executor client for %s: %w", addr, err)
	}
	return &Client{conn: conn, stub: executorv1.NewExecutorClient(conn)}, nil
}

// Close releases the underlying gRPC connection.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// RunCircuit runs a Circuit end-to-end on the executor (synchronous path).
// The provider/backend are taken from the resolved QPU (chosen by the
// controller during Move 1 of the selection chain — see
// QCC-System-Design.md §9), not re-derived from circuit.Spec.BackendSelector
// which represents user intent rather than resolution.
//
// For Qiskit-format sources the client first calls ConvertSource to obtain
// OpenQASM 3, then submits.  Conversion failures surface as TaskError with
// Reason=SourceConversionFailed so the controller's existing transient-vs-
// terminal dispatch keeps working unchanged.
func (c *Client) RunCircuit(ctx context.Context, idempotencyKey string, circuit *qccv1alpha1.Circuit, qpu *qccv1alpha1.QPU) (*Result, error) {
	qasm, err := c.resolveQASM(ctx, circuit.Spec.Source)
	if err != nil {
		return nil, err
	}
	spec := &executorv1.TaskSpec{
		IdempotencyKey: idempotencyKey,
		Qasm:           qasm,
		Shots:          uint32(circuit.Spec.Shots),
		Target:         backendTargetFromQPU(qpu),
	}
	if lvl := circuit.Spec.OptimizationLevel; lvl != nil {
		v := uint32(*lvl)
		spec.OptimizationLevel = &v
	}
	if t := circuit.Spec.TimeoutSeconds; t != nil {
		v := uint32(*t)
		spec.TimeoutSeconds = &v
	}
	if err := applyPassthrough(spec, circuit); err != nil {
		// Tier-2 passthrough that fails to decode is a terminal user
		// error (the CRD validates the dict survives a round-trip,
		// but doesn't validate the contents) — surface as TaskError
		// so the controller marks the Circuit Failed without
		// requeuing.  Reason mirrors the upstream Qiskit stage that
		// would have rejected it.
		return nil, &TaskError{Reason: qccv1alpha1.ReasonInvalidCircuit, Message: err.Error()}
	}

	resp, err := c.stub.RunCircuit(ctx, &executorv1.RunCircuitRequest{Spec: spec})
	if err != nil {
		return nil, fmt.Errorf("executor RunCircuit RPC failed: %w", err)
	}
	switch resp.GetStatus() {
	case executorv1.TaskStatus_TASK_STATUS_FAILED:
		reason := resp.GetErrorReason()
		if reason == "" {
			reason = qccv1alpha1.ReasonProviderSubmissionFailed
		}
		return nil, &TaskError{Reason: reason, Message: resp.GetErrorMessage()}
	case executorv1.TaskStatus_TASK_STATUS_DONE:
		// fall through to success path
	default:
		return nil, errors.New("executor returned non-terminal status for RunCircuit")
	}

	counts := make(map[string]int64, len(resp.GetCounts()))
	for k, v := range resp.GetCounts() {
		counts[k] = int64(v)
	}
	r := &Result{
		TaskID:       resp.GetTaskId(),
		BackendUsed:  resp.GetBackendUsed(),
		Counts:       counts,
		UsageSeconds: resp.GetUsageSeconds(),
	}
	// When the input was qiskit, resolveQASM produced the QASM 3 the
	// executor actually ran — surface it so the controller can persist it
	// as an artifact (Circuit.status.convertedRef).  Empty for native QASM
	// inputs (the input *is* the QASM).
	if circuit.Spec.Source.Format == qccv1alpha1.SourceQiskit {
		r.ConvertedQASM = qasm
	}
	if tm := resp.GetTranspile(); tm != nil {
		r.Depth = tm.GetDepth()
		r.TwoQubitGates = tm.GetTwoQubitGates()
		r.TotalGates = tm.GetTotalGates()
	}
	return r, nil
}

// SubmitResult is the controller-facing view of SubmitTask's response.
// Carries the provider job ID (stamped onto Circuit.status.providerJobID
// for subsequent WatchTask/FetchTaskResult round-trips), the transpile
// metrics produced during admission, and the resolved backend name.
type SubmitResult struct {
	TaskID        string // provider's job ID — stable across executor restarts
	BackendUsed   string
	Depth         uint32
	TwoQubitGates uint32
	TotalGates    uint32
	ConvertedQASM string // populated when source.format=qiskit, same as RunCircuit
}

// SubmitTask submits a Circuit asynchronously and returns immediately
// with the provider's task_id.  Use for real-hardware backends where
// jobs queue for minutes — the controller polls via WatchTask and
// fetches counts via FetchTaskResult on the next reconcile passes
// rather than blocking inside a single RPC.
//
// Same input-resolution convention as RunCircuit: the QPU is already
// chosen by Move 1 upstream; this client just hands it to the executor
// as a BackendTarget.  resolveQASM handles qiskit→qasm3 transparently.
//
// Failure modes mirror RunCircuit's: TaskError for terminal failures
// (NoEligibleBackend, TranspilationFailed, ProviderSubmissionFailed)
// so the controller's existing dispatch keeps working unchanged.
func (c *Client) SubmitTask(
	ctx context.Context, idempotencyKey string, circuit *qccv1alpha1.Circuit, qpu *qccv1alpha1.QPU,
) (*SubmitResult, error) {
	qasm, err := c.resolveQASM(ctx, circuit.Spec.Source)
	if err != nil {
		return nil, err
	}
	spec := &executorv1.TaskSpec{
		IdempotencyKey: idempotencyKey,
		Qasm:           qasm,
		Shots:          uint32(circuit.Spec.Shots),
		Target:         backendTargetFromQPU(qpu),
	}
	if lvl := circuit.Spec.OptimizationLevel; lvl != nil {
		v := uint32(*lvl)
		spec.OptimizationLevel = &v
	}
	if t := circuit.Spec.TimeoutSeconds; t != nil {
		v := uint32(*t)
		spec.TimeoutSeconds = &v
	}
	if err := applyPassthrough(spec, circuit); err != nil {
		return nil, &TaskError{Reason: qccv1alpha1.ReasonInvalidCircuit, Message: err.Error()}
	}

	resp, err := c.stub.SubmitTask(ctx, &executorv1.SubmitTaskRequest{Spec: spec})
	if err != nil {
		// gRPC-level error.  The executor maps terminal-failure
		// adapter errors (NoEligibleBackend, TranspilationFailed,
		// ProviderSubmissionFailed) to FAILED_PRECONDITION /
		// INVALID_ARGUMENT / UNAVAILABLE statuses with the reason
		// in the status details — unpack here so the controller
		// sees a TaskError, identical to the RunCircuit path.
		if st, ok := status.FromError(err); ok {
			details := st.Message()
			reason, msg := splitReasonMessage(details, qccv1alpha1.ReasonProviderSubmissionFailed)
			return nil, &TaskError{Reason: reason, Message: msg}
		}
		return nil, fmt.Errorf("executor SubmitTask RPC failed: %w", err)
	}
	r := &SubmitResult{
		TaskID:      resp.GetTaskId(),
		BackendUsed: resp.GetBackendUsed(),
	}
	if circuit.Spec.Source.Format == qccv1alpha1.SourceQiskit {
		r.ConvertedQASM = qasm
	}
	if tm := resp.GetTranspile(); tm != nil {
		r.Depth = tm.GetDepth()
		r.TwoQubitGates = tm.GetTwoQubitGates()
		r.TotalGates = tm.GetTotalGates()
	}
	return r, nil
}

// TaskStatus is the controller-facing view of WatchTaskResponse — the
// status enum plus an optional human-readable message (queue position,
// running step, etc.).
type TaskStatus struct {
	State   executorv1.TaskStatus // PENDING / RUNNING / DONE / FAILED / CANCELLED
	Message string                // adapter-supplied; may be empty
}

// IsTerminal reports whether this status ends the watch loop.  Callers
// should call FetchTaskResult once they see a terminal status.
func (s TaskStatus) IsTerminal() bool {
	return s.State == executorv1.TaskStatus_TASK_STATUS_DONE ||
		s.State == executorv1.TaskStatus_TASK_STATUS_FAILED ||
		s.State == executorv1.TaskStatus_TASK_STATUS_CANCELLED
}

// WatchTask streams TaskStatus updates from the executor until terminal
// or the stream closes (executor's max-watch-duration, network blip,
// context cancelled).  The controller calls this on reconcile of a
// running Circuit; the executor decides the poll cadence internally.
//
// Returns a channel that emits TaskStatus values until closed, and an
// error channel that emits at most one error.  Caller is responsible
// for cancelling ctx to stop the stream.  Closes both channels when
// the stream ends.
func (c *Client) WatchTask(
	ctx context.Context, taskID string,
) (<-chan TaskStatus, <-chan error, error) {
	stream, err := c.stub.WatchTask(ctx, &executorv1.WatchTaskRequest{TaskId: taskID})
	if err != nil {
		return nil, nil, fmt.Errorf("executor WatchTask RPC failed: %w", err)
	}

	statusCh := make(chan TaskStatus, 4)
	errCh := make(chan error, 1)

	go func() {
		defer close(statusCh)
		defer close(errCh)
		for {
			resp, recvErr := stream.Recv()
			if recvErr == io.EOF {
				return
			}
			if recvErr != nil {
				// Convert task-not-found to a TaskError so the
				// controller can surface it cleanly.  Other RPC
				// errors propagate as-is.
				if st, ok := status.FromError(recvErr); ok {
					if st.Code() == codes.NotFound {
						errCh <- &TaskError{
							Reason:  "TaskNotFound",
							Message: st.Message(),
						}
						return
					}
				}
				errCh <- fmt.Errorf("WatchTask stream recv: %w", recvErr)
				return
			}
			statusCh <- TaskStatus{
				State:   resp.GetStatus(),
				Message: resp.GetMessage(),
			}
		}
	}()

	return statusCh, errCh, nil
}

// FetchTaskResult retrieves counts for a terminal task.  Call after
// WatchTask has yielded a TASK_STATUS_DONE.  Once counts are returned
// the executor drops the task from its in-memory registry — a second
// call returns TaskNotFound.  The controller persists the counts on
// Circuit.status.results so the registry cleanup is safe.
func (c *Client) FetchTaskResult(ctx context.Context, taskID string) (TaskResult, error) {
	resp, err := c.stub.FetchTaskResult(ctx, &executorv1.FetchTaskResultRequest{TaskId: taskID})
	if err != nil {
		return TaskResult{}, fmt.Errorf("executor FetchTaskResult RPC failed: %w", err)
	}
	switch resp.GetStatus() {
	case executorv1.TaskStatus_TASK_STATUS_FAILED:
		reason := resp.GetErrorReason()
		if reason == "" {
			reason = qccv1alpha1.ReasonProviderSubmissionFailed
		}
		return TaskResult{}, &TaskError{Reason: reason, Message: resp.GetErrorMessage()}
	case executorv1.TaskStatus_TASK_STATUS_DONE:
		counts := make(map[string]int64, len(resp.GetCounts()))
		for k, v := range resp.GetCounts() {
			counts[k] = int64(v)
		}
		return TaskResult{
			Counts:       counts,
			UsageSeconds: resp.GetUsageSeconds(),
		}, nil
	default:
		return TaskResult{}, errors.New("executor returned non-terminal status for FetchTaskResult")
	}
}

// applyPassthrough decodes the Tier-2 passthrough blocks on the Circuit
// (spec.transpile, spec.execute) into the proto TaskSpec's Struct fields.
// The CRD declares them with x-kubernetes-preserve-unknown-fields=true so
// they round-trip through apiserver as raw JSON; here we decode the
// bytes to map[string]interface{} and hand them to structpb.NewStruct
// for the wire encoding.  Keys are forwarded verbatim — snake_case
// matching Qiskit's parameter names is the user's responsibility (see
// QCC-Design-State.md §7a, Composition Principle Tier 2).
//
// Returns an error only when the JSON is malformed or contains a value
// type structpb can't represent (e.g. complex numbers).  Empty/nil
// blocks are a no-op.
func applyPassthrough(spec *executorv1.TaskSpec, circuit *qccv1alpha1.Circuit) error {
	transpile, err := jsonToStruct(circuit.Spec.Transpile, "spec.transpile")
	if err != nil {
		return err
	}
	execute, err := jsonToStruct(circuit.Spec.Execute, "spec.execute")
	if err != nil {
		return err
	}
	spec.TranspileOptions = transpile
	spec.ExecuteOptions = execute
	return nil
}

// jsonToStruct converts a *apiextensionsv1.JSON (CRD opaque dict) into a
// *structpb.Struct (proto opaque dict).  Returns nil, nil for missing or
// empty input so callers don't have to nil-check.  The `field` parameter
// is used to scope error messages back to the CR field that failed
// ("spec.transpile" / "spec.execute") so the user can locate the offending
// key without guessing.
func jsonToStruct(j *apiextensionsv1.JSON, field string) (*structpb.Struct, error) {
	if j == nil || len(j.Raw) == 0 {
		return nil, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(j.Raw, &m); err != nil {
		return nil, fmt.Errorf("decode %s as JSON object: %w", field, err)
	}
	if len(m) == 0 {
		return nil, nil
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		return nil, fmt.Errorf("encode %s as protobuf Struct: %w", field, err)
	}
	return s, nil
}

// splitReasonMessage decodes the "Reason: message" format the executor
// uses to pack a Circuit-condition reason into gRPC status details.
// Falls back to (defaultReason, raw) when there's no colon delimiter.
func splitReasonMessage(details, defaultReason string) (string, string) {
	idx := strings.Index(details, ": ")
	if idx <= 0 {
		return defaultReason, details
	}
	return details[:idx], details[idx+2:]
}

// DrawCircuit renders the circuit source as ASCII via the executor's text
// drawer.  Supports both OpenQASM 3 and Qiskit-Python formats; the executor
// picks the loader.  Returns a TaskError with Reason=RenderingFailed (or
// whatever reason the executor reported) when the rendering RPC reports a
// terminal failure.
func (c *Client) DrawCircuit(ctx context.Context, source qccv1alpha1.CircuitSource) (string, error) {
	resp, err := c.stub.DrawCircuit(ctx, &executorv1.DrawCircuitRequest{
		Source: circuitSourceProto(source),
	})
	if err != nil {
		return "", fmt.Errorf("executor DrawCircuit RPC failed: %w", err)
	}
	switch resp.GetStatus() {
	case executorv1.TaskStatus_TASK_STATUS_FAILED:
		reason := resp.GetErrorReason()
		if reason == "" {
			reason = qccv1alpha1.ReasonRenderingFailed
		}
		return "", &TaskError{Reason: reason, Message: resp.GetErrorMessage()}
	case executorv1.TaskStatus_TASK_STATUS_DONE:
		return resp.GetDrawing(), nil
	default:
		return "", errors.New("executor returned non-terminal status for DrawCircuit")
	}
}

// ScheduleCircuit asks the executor to transpile + schedule the circuit
// for the chosen QPU and return the per-instruction timeline.  Same
// resolution convention as RunCircuit: the QPU was picked by Move 1
// upstream, so we just hand it to the executor as a BackendTarget.
// resolveQASM handles the qiskit→qasm3 conversion exactly as for run.
//
// Failure modes mirror DrawCircuit's: terminal failures (SchedulingFailed,
// SchedulingUnsupported, NoEligibleBackend) come back as TaskError so the
// controller marks the Circuit Failed without requeuing; transport
// failures are wrapped errors and trigger requeue.
func (c *Client) ScheduleCircuit(
	ctx context.Context, circuit *qccv1alpha1.Circuit, qpu *qccv1alpha1.QPU,
) (*ScheduleResult, error) {
	qasm, err := c.resolveQASM(ctx, circuit.Spec.Source)
	if err != nil {
		return nil, err
	}
	req := &executorv1.ScheduleCircuitRequest{
		Source: &executorv1.CircuitSource{
			Format: string(qccv1alpha1.SourceOpenQASM3),
			Body:   qasm,
		},
		Target: backendTargetFromQPU(qpu),
	}
	if lvl := circuit.Spec.OptimizationLevel; lvl != nil {
		v := *lvl
		req.OptimizationLevel = &v
	}

	resp, err := c.stub.ScheduleCircuit(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("executor ScheduleCircuit RPC failed: %w", err)
	}
	switch resp.GetStatus() {
	case executorv1.TaskStatus_TASK_STATUS_FAILED:
		reason := resp.GetErrorReason()
		if reason == "" {
			reason = qccv1alpha1.ReasonSchedulingFailed
		}
		return nil, &TaskError{Reason: reason, Message: resp.GetErrorMessage()}
	case executorv1.TaskStatus_TASK_STATUS_DONE:
		// fall through
	default:
		return nil, errors.New("executor returned non-terminal status for ScheduleCircuit")
	}

	ops := make([]ScheduledOp, 0, len(resp.GetOps()))
	for _, op := range resp.GetOps() {
		ops = append(ops, ScheduledOp{
			Name:       op.GetName(),
			Qubits:     op.GetQubits(),
			StartDt:    op.GetStartDt(),
			DurationDt: op.GetDurationDt(),
		})
	}
	return &ScheduleResult{
		Ops:             ops,
		TotalDurationDt: resp.GetTotalDurationDt(),
		DtSeconds:       resp.GetDtSeconds(),
		NumQubits:       resp.GetNumQubits(),
		BackendUsed:     resp.GetBackendUsed(),
	}, nil
}

// ProbeBackend asks the executor to introspect a named backend and return
// its calibration-relevant metadata: qubit count, native gate set, coupling
// edges, calibration timestamp, gate/readout error medians.  Pure
// introspection — no shots, no submission.  The QPUReconciler calls this
// when a QPU registers and stamps the response onto Circuit.status.
//
// Failure modes mirror the rest of the gRPC surface: unknown backend names
// (or adapters that don't yet exist) surface as TaskError; transport
// failures are returned as wrapped errors so the controller's transient
// requeue path applies.
func (c *Client) ProbeBackend(ctx context.Context, provider, backendName string) (*BackendProfile, error) {
	resp, err := c.stub.ProbeBackend(ctx, &executorv1.ProbeBackendRequest{
		Provider:    provider,
		BackendName: backendName,
	})
	if err != nil {
		return nil, fmt.Errorf("executor ProbeBackend RPC failed: %w", err)
	}
	switch resp.GetStatus() {
	case executorv1.TaskStatus_TASK_STATUS_FAILED:
		reason := resp.GetErrorReason()
		if reason == "" {
			reason = qccv1alpha1.ReasonProviderProbeFailed
		}
		return nil, &TaskError{Reason: reason, Message: resp.GetErrorMessage()}
	case executorv1.TaskStatus_TASK_STATUS_DONE:
		// fall through
	default:
		return nil, errors.New("executor returned non-terminal status for ProbeBackend")
	}
	return &BackendProfile{
		NumQubits:                        int32(resp.GetNumQubits()), //nolint:gosec // uint32→int32, kubebuilder validates Minimum
		BasisGates:                       resp.GetBasisGates(),
		CouplingEdges:                    int32(resp.GetCouplingEdges()), //nolint:gosec
		LastCalibrationTime:              resp.GetLastCalibrationTime(),
		SingleQubitErrorMedian:           resp.GetSingleQubitErrorMedian(),
		TwoQubitErrorMedian:              resp.GetTwoQubitErrorMedian(),
		ReadoutErrorMedian:               resp.GetReadoutErrorMedian(),
		T1MedianMicros:                   resp.GetT1MedianUs(),
		T2MedianMicros:                   resp.GetT2MedianUs(),
		DtSeconds:                        resp.GetDtSeconds(),
		SingleQubitDurationMedianSeconds: resp.GetSingleQubitDurationMedianSeconds(),
		TwoQubitDurationMedianSeconds:    resp.GetTwoQubitDurationMedianSeconds(),
		ProcessorFamily:                  resp.GetProcessorFamily(),
		ProcessorRevision:                resp.GetProcessorRevision(),
		ProcessorSegment:                 resp.GetProcessorSegment(),
	}, nil
}

// resolveQASM returns the OpenQASM 3 text for a Circuit source, transparently
// invoking ConvertSource for non-QASM formats.  This keeps the caller (the
// reconciler) ignorant of how the executor handles Qiskit-Python.
func (c *Client) resolveQASM(ctx context.Context, source qccv1alpha1.CircuitSource) (string, error) {
	if source.Format == qccv1alpha1.SourceOpenQASM3 || source.Format == "" {
		return source.Body, nil
	}
	resp, err := c.stub.ConvertSource(ctx, &executorv1.ConvertSourceRequest{
		Source: circuitSourceProto(source),
	})
	if err != nil {
		return "", fmt.Errorf("executor ConvertSource RPC failed: %w", err)
	}
	switch resp.GetStatus() {
	case executorv1.TaskStatus_TASK_STATUS_FAILED:
		reason := resp.GetErrorReason()
		if reason == "" {
			reason = qccv1alpha1.ReasonSourceConversionFailed
		}
		return "", &TaskError{Reason: reason, Message: resp.GetErrorMessage()}
	case executorv1.TaskStatus_TASK_STATUS_DONE:
		return resp.GetQasm(), nil
	default:
		return "", errors.New("executor returned non-terminal status for ConvertSource")
	}
}

func circuitSourceProto(s qccv1alpha1.CircuitSource) *executorv1.CircuitSource {
	return &executorv1.CircuitSource{
		Format: string(s.Format),
		Body:   s.Body,
	}
}

// backendTargetFromQPU translates a resolved QPU into the gRPC target the
// executor's adapter dispatch reads.  The Kind defaults to SIMULATOR when
// the QPU spec is somehow unset — defensive only; CRD validation requires
// it on the way in.
func backendTargetFromQPU(qpu *qccv1alpha1.QPU) *executorv1.BackendTarget {
	target := &executorv1.BackendTarget{
		Provider:    qpu.Spec.Provider,
		BackendName: qpu.EffectiveBackendName(),
		Kind:        executorv1.BackendKind_BACKEND_KIND_SIMULATOR,
	}
	switch qpu.Spec.Kind {
	case qccv1alpha1.BackendKindHardware:
		target.Kind = executorv1.BackendKind_BACKEND_KIND_HARDWARE
	case qccv1alpha1.BackendKindSimulator:
		target.Kind = executorv1.BackendKind_BACKEND_KIND_SIMULATOR
	}
	return target
}
