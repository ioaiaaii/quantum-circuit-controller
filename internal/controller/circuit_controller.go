/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	qccv1alpha1 "github.com/ioaiaaii/quantum-circuit-controller/api/v1alpha1"
	executorv1 "github.com/ioaiaaii/quantum-circuit-controller/gen/proto/qcc/executor/v1"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/executor"
	observabilitymetrics "github.com/ioaiaaii/quantum-circuit-controller/internal/observability/metrics"
)

// Artifact ConfigMap conventions are defined on the API package
// (qccv1alpha1.Artifact*) so the controller and CLI share a single source
// of truth.  See QCC-API.md §3.7.

// Executor is the controller's view onto the qcc-executor gRPC service.  The
// real implementation is *executor.Client; tests inject in-memory fakes.
//
// RunCircuit takes the resolved QPU explicitly rather than re-deriving from
// circuit.Spec.BackendSelector — by the time the controller calls into the
// executor, Move 1 of the selection chain (enumerate + hard-constraint
// filter, controller-side; see QCC-System-Design.md §9) has already chosen
// a candidate.  The Selector is user intent; the QPU is the controller's
// resolution.
type Executor interface {
	// RunCircuit runs a Circuit synchronously (blocks until results).
	// Suits simulator backends where jobs return in seconds.
	RunCircuit(ctx context.Context, idempotencyKey string, circuit *qccv1alpha1.Circuit, qpu *qccv1alpha1.QPU) (*executor.Result, error)
	// SubmitTask submits a Circuit asynchronously and returns
	// immediately with the provider job ID.  Used for hardware backends
	// where jobs queue for minutes; the reconciler polls via WatchTask
	// and fetches counts via FetchTaskResult on subsequent passes.
	SubmitTask(ctx context.Context, idempotencyKey string, circuit *qccv1alpha1.Circuit, qpu *qccv1alpha1.QPU) (*executor.SubmitResult, error)
	// WatchTask streams TaskStatus updates until the stream closes.
	// The reconciler reads one frame and closes the stream — pattern
	// matches the K8s reconcile-loop's natural polling cadence.
	WatchTask(ctx context.Context, taskID string) (<-chan executor.TaskStatus, <-chan error, error)
	// FetchTaskResult retrieves the terminal task's counts plus any
	// substrate-reported result metadata (currently usage_seconds).
	// Called once WatchTask has yielded TASK_STATUS_DONE.
	FetchTaskResult(ctx context.Context, taskID string) (executor.TaskResult, error)
	// DrawCircuit renders the circuit source as ASCII.  Pure transform.
	DrawCircuit(ctx context.Context, source qccv1alpha1.CircuitSource) (string, error)
	// ScheduleCircuit takes the resolved QPU explicitly (Move 1 already
	// happened) and returns the per-instruction scheduled timeline.
	// Same input-resolution convention as RunCircuit.
	ScheduleCircuit(ctx context.Context, circuit *qccv1alpha1.Circuit, qpu *qccv1alpha1.QPU) (*executor.ScheduleResult, error)
	// ProbeBackend returns calibration-relevant metadata for the named
	// backend.  Called by QPUReconciler on registration (and on M2's
	// TTL-driven refresh).  The QPUReconciler — not the
	// CircuitReconciler — is the primary caller.
	ProbeBackend(ctx context.Context, provider, backendName string) (*executor.BackendProfile, error)
}

// CircuitReconciler reconciles a Circuit through its phase state machine
// (see docs/systems-design/QCC-API.md §5.1).
type CircuitReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Executor Executor
}

// +kubebuilder:rbac:groups=qcc.io,resources=circuits,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=qcc.io,resources=circuits/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=qcc.io,resources=circuits/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *CircuitReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	circuit := &qccv1alpha1.Circuit{}
	if err := r.Get(ctx, req.NamespacedName, circuit); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !circuit.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// Auto-stamp algorithm-grouping + source-hash labels on first
	// reconcile.  Patches metadata.labels with run-index (when
	// qcc.io/algorithm is present) and source-sha256 (always), then
	// requeues so the rest of Reconcile works against the patched
	// state.  No-op on subsequent reconciles once the labels exist.
	// See `ensureAlgorithmLabels` for the precise semantics.
	if patched, err := r.ensureAlgorithmLabels(ctx, circuit); err != nil {
		log.Error(err, "ensureAlgorithmLabels failed; will requeue")
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	} else if patched {
		// Label patch succeeded — next reconcile picks up the new
		// state.  Skip phase work this pass to avoid double-doing
		// initial setup against half-stamped metadata.
		return ctrl.Result{Requeue: true}, nil
	}

	// Capture pre-reconcile phase + the timestamp of the most recent
	// condition transition, so the post-dispatch hook can detect a
	// phase change and emit the synchronous Counter + Histogram
	// metrics (qcc_circuits_total, qcc_circuit_phase_duration_seconds).
	// Handlers operate on `circuit` by pointer; any phase change they
	// commit is visible here when they return.  See
	// `internal/observability/metrics/events.go` for the metrics.
	prePhase := circuit.Status.Phase
	prePhaseEnteredAt := latestConditionTime(circuit)

	defer func() {
		// Emit only when phase actually changed.  Many reconciles
		// happen *within* a phase (requeues, polling); those don't
		// count as transitions.
		if circuit.Status.Phase == prePhase {
			return
		}
		duration := -1.0
		if !prePhaseEnteredAt.IsZero() {
			duration = time.Since(prePhaseEnteredAt).Seconds()
		}
		observabilitymetrics.RecordPhaseTransition(
			ctx,
			circuit.Name,
			circuit.Namespace,
			string(circuit.UID),
			circuit.Status.ProviderJobID,
			string(circuit.Status.Phase),
			latestConditionReason(circuit),
			circuit.Status.SelectedQPU,
			string(circuit.Spec.Mode),
			string(prePhase),
			duration,
		)
	}()

	switch circuit.Status.Phase {
	case "":
		return r.accept(ctx, circuit)
	case qccv1alpha1.PhasePending:
		return r.selectBackend(ctx, circuit)
	case qccv1alpha1.PhaseSelecting:
		return r.advanceToTranspiling(ctx, circuit)
	case qccv1alpha1.PhaseTranspiling:
		return r.advanceFromTranspiling(ctx, circuit)
	case qccv1alpha1.PhaseSubmitting:
		return r.runOnExecutor(ctx, circuit)
	case qccv1alpha1.PhaseRunning:
		// Async lifecycle: the circuit was SubmitTask'd to a hardware
		// backend and is now in the vendor's queue.  Poll status until
		// terminal (DONE/FAILED), then fetch results and transition.
		// Simulator backends never enter this phase — they complete
		// inline during PhaseSubmitting via the sync RunCircuit path.
		return r.pollAsyncJob(ctx, circuit)
	case qccv1alpha1.PhaseRendering:
		return r.renderDrawing(ctx, circuit)
	case qccv1alpha1.PhaseScheduling:
		return r.renderSchedule(ctx, circuit)
	case qccv1alpha1.PhaseSucceeded,
		qccv1alpha1.PhaseFailed:
		return ctrl.Result{}, nil
	default:
		log.Info("Unknown circuit phase, marking failed", "phase", circuit.Status.Phase)
		return r.fail(ctx, circuit, qccv1alpha1.ReasonInvalidCircuit,
			fmt.Sprintf("unknown phase %q", circuit.Status.Phase))
	}
}

// ensureAlgorithmLabels stamps the controller-managed grouping labels
// on first reconcile.  Two labels can be filled in:
//
//   - qcc.io/source-sha256 : SHA-256 of spec.source.body, hex-encoded.
//     Always stamped (when absent), regardless of whether the Circuit
//     is part of an algorithm.  Serves as the content truth-anchor
//     — a relabel that doesn't change the body produces the same
//     hash, exposing version-only diffs.
//
//   - qcc.io/run-index     : ordinal among siblings sharing the same
//     qcc.io/algorithm (+ optional qcc.io/experiment).  Computed as
//     max(existing run-index labels) + 1.  Only stamped when the
//     algorithm label is set — Circuits without an algorithm are
//     standalone runs and don't participate in indexing.
//
// Returns (true, nil) when at least one label was patched (caller
// should requeue to work against the updated state).  Returns
// (false, nil) when both labels are already present.  Errors are
// transient — the caller requeues and retries.
//
// Race-condition note: if two Circuits in the same algorithm are
// reconciled in parallel, both may compute the same `max + 1` against
// the same snapshot and end up with the same run-index.  Vanishingly
// rare at thesis scale; documented behaviour.  Production-grade fix
// would be a ConfigMap-backed atomic counter per algorithm.
func (r *CircuitReconciler) ensureAlgorithmLabels(ctx context.Context, circuit *qccv1alpha1.Circuit) (bool, error) {
	wantSHA := wantsSourceSHA(circuit)
	wantIdx := wantsRunIndex(circuit)
	if !wantSHA && !wantIdx {
		return false, nil
	}

	patch := client.MergeFrom(circuit.DeepCopy())
	if circuit.Labels == nil {
		circuit.Labels = map[string]string{}
	}

	if wantSHA {
		// K8s label values are capped at 63 characters; full SHA-256
		// hex is 64.  Truncate to 16 hex chars (64 bits of entropy)
		// — more than enough collision-resistance at thesis scale
		// (~10^19 unique values vs ~10^3 Circuits we'll ever run).
		// Full hash remains computable from spec.source.body if
		// anyone ever needs the long form; the label prefix is the
		// queryable identity anchor.
		sum := sha256.Sum256([]byte(circuit.Spec.Source.Body))
		circuit.Labels[qccv1alpha1.LabelSourceSHA256] = hex.EncodeToString(sum[:])[:16]
	}

	if wantIdx {
		next, err := r.nextRunIndex(ctx, circuit)
		if err != nil {
			return false, fmt.Errorf("compute run-index: %w", err)
		}
		circuit.Labels[qccv1alpha1.LabelRunIndex] = strconv.Itoa(next)
	}

	if err := r.Patch(ctx, circuit, patch); err != nil {
		return false, fmt.Errorf("patch algorithm labels: %w", err)
	}
	return true, nil
}

// wantsSourceSHA returns true when source-sha256 should be stamped.
// Requires a non-empty spec.source.body — empty bodies (e.g., the
// brief window before validation rejects an invalid Circuit) are
// skipped so we don't hash the empty string and confuse anyone
// comparing identities.
func wantsSourceSHA(circuit *qccv1alpha1.Circuit) bool {
	if circuit.Spec.Source.Body == "" {
		return false
	}
	_, has := circuit.Labels[qccv1alpha1.LabelSourceSHA256]
	return !has
}

// wantsRunIndex returns true when run-index should be stamped.
// Requires the algorithm label to be set (run-index is meaningful
// only as an ordinal within an algorithm cohort).
func wantsRunIndex(circuit *qccv1alpha1.Circuit) bool {
	if circuit.Labels[qccv1alpha1.LabelAlgorithm] == "" {
		return false
	}
	_, has := circuit.Labels[qccv1alpha1.LabelRunIndex]
	return !has
}

// nextRunIndex computes max(existing run-index labels) + 1 across
// sibling Circuits in the same namespace sharing this Circuit's
// algorithm (+ experiment, when set).  Returns 1 when no siblings
// exist or none carries a parseable run-index label.
//
// Listed scope is per-namespace — Circuits are namespaced, and
// "experiment" is naturally a per-namespace concept (different
// teams in different namespaces don't share run-index sequences).
func (r *CircuitReconciler) nextRunIndex(ctx context.Context, circuit *qccv1alpha1.Circuit) (int, error) {
	sel := client.MatchingLabels{
		qccv1alpha1.LabelAlgorithm: circuit.Labels[qccv1alpha1.LabelAlgorithm],
	}
	if exp := circuit.Labels[qccv1alpha1.LabelExperiment]; exp != "" {
		sel[qccv1alpha1.LabelExperiment] = exp
	}

	var siblings qccv1alpha1.CircuitList
	if err := r.List(ctx, &siblings,
		client.InNamespace(circuit.Namespace),
		sel,
	); err != nil {
		return 0, err
	}

	maxIdx := 0
	for i := range siblings.Items {
		s := &siblings.Items[i]
		// Skip self — when the controller picks up this Circuit's
		// own pending label patch on a retry, we don't want to
		// double-count it.
		if s.UID == circuit.UID {
			continue
		}
		if v, ok := s.Labels[qccv1alpha1.LabelRunIndex]; ok {
			if n, err := strconv.Atoi(v); err == nil && n > maxIdx {
				maxIdx = n
			}
		}
	}
	return maxIdx + 1, nil
}

func (r *CircuitReconciler) accept(ctx context.Context, circuit *qccv1alpha1.Circuit) (ctrl.Result, error) {
	if circuit.Spec.Source.Body == "" {
		return r.fail(ctx, circuit, qccv1alpha1.ReasonInvalidCircuit, "spec.source.body is required")
	}
	if circuit.Spec.Source.Format == "" {
		return r.fail(ctx, circuit, qccv1alpha1.ReasonInvalidCircuit, "spec.source.format is required")
	}
	if circuit.Spec.Mode == qccv1alpha1.ModeRun && circuit.Spec.Shots <= 0 {
		return r.fail(ctx, circuit, qccv1alpha1.ReasonInvalidCircuit, "spec.shots must be > 0 for mode=run")
	}

	// mode=draw skips selection/transpile/submit and goes straight to the
	// rendering phase where the executor's DrawCircuit RPC is invoked.
	nextPhase := qccv1alpha1.PhasePending
	if circuit.Spec.Mode == qccv1alpha1.ModeDraw {
		nextPhase = qccv1alpha1.PhaseRendering
	}

	patch := client.MergeFrom(circuit.DeepCopy())
	setCondition(circuit, qccv1alpha1.ConditionAccepted,
		"CircuitAccepted", "Circuit accepted by controller")
	setCondition(circuit, qccv1alpha1.ConditionValidated,
		"CircuitValidated", "Static validation succeeded")
	circuit.Status.Phase = nextPhase
	circuit.Status.ObservedGeneration = circuit.Generation
	if err := r.Status().Patch(ctx, circuit, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// selectBackend implements Move 1 of the five-move accuracy chain
// (QCC-System-Design.md §9): enumerate registered QPUs and filter by
// the Circuit's hard constraints.  Moves 2–5 (calibrate, transpile per
// backend, layout, score) live in the executor and arrive with M2.  For
// M1 we pick the first eligible candidate; multi-candidate scoring is
// explicitly future work.
func (r *CircuitReconciler) selectBackend(ctx context.Context, circuit *qccv1alpha1.Circuit) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var qpuList qccv1alpha1.QPUList
	if err := r.List(ctx, &qpuList); err != nil {
		log.Error(err, "List QPUs failed; will requeue")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	candidates := filterEligibleQPUs(qpuList.Items, circuit.Spec)
	if len(candidates) == 0 {
		return r.fail(ctx, circuit, qccv1alpha1.ReasonNoEligibleBackend,
			fmt.Sprintf("no Available QPU matches the selector (registered=%d)", len(qpuList.Items)))
	}

	chosen := candidates[0]

	patch := client.MergeFrom(circuit.DeepCopy())
	circuit.Status.SelectedQPU = chosen.Name
	circuit.Status.SelectionSummary = &qccv1alpha1.SelectionSummary{
		Candidates: int32(len(candidates)),
		Selected:   chosen.Name,
		// TODO(m2): replace with the composite score from Move 5 once
		// executor-side scoring lands (QCC-Design-State.md §5).
		Score: "n/a (M1: first-match policy; M2 adds calibration-aware scoring)",
	}
	setCondition(circuit, qccv1alpha1.ConditionSelected,
		qccv1alpha1.ReasonBackendSelected,
		fmt.Sprintf("Selected QPU %q (provider=%s, backend=%s)",
			chosen.Name, chosen.Spec.Provider, chosen.EffectiveBackendName()))
	circuit.Status.Phase = qccv1alpha1.PhaseSelecting
	if err := r.Status().Patch(ctx, circuit, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// filterEligibleQPUs applies the hard-constraint filter that constitutes
// Move 1 of the selection chain: status.availability == Available, plus
// any user-supplied BackendSelector constraints, plus QPU-declared
// capability ceilings (e.g. MaxShots).  Pure function — no API calls, no
// clock reads — so it's directly unit-testable.
func filterEligibleQPUs(qpus []qccv1alpha1.QPU, spec qccv1alpha1.CircuitSpec) []qccv1alpha1.QPU {
	out := make([]qccv1alpha1.QPU, 0, len(qpus))
	for _, qpu := range qpus {
		if qpu.Status.Availability != qccv1alpha1.QPUAvailable {
			continue
		}
		if sel := spec.BackendSelector; sel != nil {
			if sel.Provider != "" && qpu.Spec.Provider != sel.Provider {
				continue
			}
			// BackendName matches by *either* the QPU's CRD name
			// (kubectl-style, dashes only) *or* its effective backend
			// name (provider-native, may contain underscores; derived
			// from metadata.name when spec.backendName is omitted —
			// see QPU.EffectiveBackendName).
			if sel.BackendName != "" &&
				qpu.EffectiveBackendName() != sel.BackendName &&
				qpu.Name != sel.BackendName {
				continue
			}
			if sel.Kind != "" && qpu.Spec.Kind != sel.Kind {
				continue
			}
			if sel.MinQubits > 0 && qpu.EffectiveQubits() < sel.MinQubits {
				continue
			}
		}
		// QPU-declared capability ceiling: reject if the Circuit asks
		// for more shots than the backend supports.
		if caps := qpu.Spec.Capabilities; caps != nil && caps.MaxShots != nil {
			if spec.Shots > *caps.MaxShots {
				continue
			}
		}
		out = append(out, qpu)
	}
	return out
}

func (r *CircuitReconciler) advanceToTranspiling(ctx context.Context, circuit *qccv1alpha1.Circuit) (ctrl.Result, error) {
	patch := client.MergeFrom(circuit.DeepCopy())
	// mode=schedule bypasses the Transpiling/Submitting phases: the
	// executor's ScheduleCircuit RPC does its own transpile-with-
	// scheduling pass internally, so the controller-visible phase
	// transition goes Selecting → Scheduling → Succeeded.
	if circuit.Spec.Mode == qccv1alpha1.ModeSchedule {
		circuit.Status.Phase = qccv1alpha1.PhaseScheduling
	} else {
		circuit.Status.Phase = qccv1alpha1.PhaseTranspiling
	}
	if err := r.Status().Patch(ctx, circuit, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

func (r *CircuitReconciler) advanceFromTranspiling(ctx context.Context, circuit *qccv1alpha1.Circuit) (ctrl.Result, error) {
	if circuit.Spec.Mode == qccv1alpha1.ModeSelect {
		patch := client.MergeFrom(circuit.DeepCopy())
		setCondition(circuit, qccv1alpha1.ConditionCompleted,
			"SelectCompleted", "Selection completed in mode=select")
		circuit.Status.Phase = qccv1alpha1.PhaseSucceeded
		if err := r.Status().Patch(ctx, circuit, patch); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	patch := client.MergeFrom(circuit.DeepCopy())
	circuit.Status.Phase = qccv1alpha1.PhaseSubmitting
	if err := r.Status().Patch(ctx, circuit, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// runOnExecutor handles PhaseSubmitting.  Dispatches to the sync or
// async path based on the selected QPU's `kind`:
//
//   - kind=simulator → sync `RunCircuit` (blocks the reconcile until
//     results, but jobs return in seconds; today's behavior).
//   - kind=hardware  → async `SubmitTask` (returns immediately with a
//     provider job ID), then phase transitions to Running for the
//     reconciler to poll.
//
// Both paths share artifact persistence (convertedRef) and transpile-
// metric stamping; only the execution lifecycle differs.
func (r *CircuitReconciler) runOnExecutor(ctx context.Context, circuit *qccv1alpha1.Circuit) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if r.Executor == nil {
		return r.fail(ctx, circuit, qccv1alpha1.ReasonProviderSubmissionFailed,
			"executor client not configured")
	}

	// Re-fetch the QPU chosen during Move 1.  Cluster-scoped, so no
	// namespace.  If the QPU has been deleted between selectBackend and
	// here (rare, but possible), surface as a failure with the same
	// NoEligibleBackend reason — the user-visible explanation is the
	// same as if it had never been there.
	var qpu qccv1alpha1.QPU
	if err := r.Get(ctx, types.NamespacedName{Name: circuit.Status.SelectedQPU}, &qpu); err != nil {
		if apierrors.IsNotFound(err) {
			return r.fail(ctx, circuit, qccv1alpha1.ReasonNoEligibleBackend,
				fmt.Sprintf("QPU %q no longer exists", circuit.Status.SelectedQPU))
		}
		log.Error(err, "Fetch selected QPU failed; will requeue")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Dispatch by backend kind.  Hardware backends queue for minutes;
	// blocking inside one reconcile call is wrong (controller-runtime
	// expects reconciles to return quickly).  Simulator backends
	// complete in seconds; sync is simpler and ships today.
	if qpu.Spec.Kind == qccv1alpha1.BackendKindHardware {
		return r.submitAsync(ctx, circuit, &qpu)
	}
	return r.runSync(ctx, circuit, &qpu)
}

// runSync executes the circuit synchronously inside one RPC.  Used for
// simulator backends where blocking is acceptable.
func (r *CircuitReconciler) runSync(
	ctx context.Context, circuit *qccv1alpha1.Circuit, qpu *qccv1alpha1.QPU,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	idempotencyKey := fmt.Sprintf("%s/%d", circuit.UID, circuit.Status.ObservedGeneration)
	rpcCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	result, err := r.Executor.RunCircuit(rpcCtx, idempotencyKey, circuit, qpu)
	if err != nil {
		var taskErr *executor.TaskError
		if errors.As(err, &taskErr) {
			log.Info("Executor reported task failure",
				"reason", taskErr.Reason, "message", taskErr.Message)
			return r.fail(ctx, circuit, taskErr.Reason, taskErr.Message)
		}
		log.Error(err, "Executor RPC failed; will requeue")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// When the source was qiskit, the executor's ConvertSource ran as part
	// of RunCircuit and produced the QASM 3 that was actually submitted.
	// Persist it as an artifact so users can inspect what was executed
	// (`qcc get <name> --qasm`).  Treated as a non-fatal best-effort: if
	// the ConfigMap upsert transiently fails we requeue rather than
	// failing the run, since the execution itself already succeeded.
	var convertedRef *qccv1alpha1.ArtifactRef
	if result.ConvertedQASM != "" {
		cmName, upErr := r.upsertArtifactConfigMap(ctx, circuit,
			qccv1alpha1.ArtifactSuffixConverted, qccv1alpha1.ArtifactDataKeyQASM, result.ConvertedQASM)
		if upErr != nil {
			log.Error(upErr, "ConfigMap upsert for converted QASM failed; will requeue")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		convertedRef = &qccv1alpha1.ArtifactRef{Name: cmName}
	}

	patch := client.MergeFrom(circuit.DeepCopy())
	circuit.Status.ProviderJobID = result.TaskID
	circuit.Status.Results = result.Counts
	circuit.Status.UsageSeconds = result.UsageSeconds
	if convertedRef != nil {
		circuit.Status.ConvertedRef = convertedRef
	}
	if result.Depth > 0 || result.TwoQubitGates > 0 || result.TotalGates > 0 {
		circuit.Status.Transpile = &qccv1alpha1.CircuitTranspileMetrics{
			Depth:         result.Depth,
			TwoQubitGates: result.TwoQubitGates,
			TotalGates:    result.TotalGates,
		}
	}
	setCondition(circuit, qccv1alpha1.ConditionSubmitted,
		"ProviderSubmitted",
		fmt.Sprintf("Submitted to %s (taskID=%s)", result.BackendUsed, result.TaskID))
	setCondition(circuit, qccv1alpha1.ConditionCompleted,
		qccv1alpha1.ReasonExecutionCompleted,
		fmt.Sprintf("Execution completed with %d outcome(s)", len(result.Counts)))
	circuit.Status.Phase = qccv1alpha1.PhaseSucceeded
	if err := r.Status().Patch(ctx, circuit, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// submitAsync calls SubmitTask, stamps the provider job ID and
// transpile metrics, and transitions the Circuit to PhaseRunning.  The
// reconciler's PhaseRunning handler (`pollAsyncJob`) takes over the
// status-polling and result-fetching from there.
func (r *CircuitReconciler) submitAsync(
	ctx context.Context, circuit *qccv1alpha1.Circuit, qpu *qccv1alpha1.QPU,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	idempotencyKey := fmt.Sprintf("%s/%d", circuit.UID, circuit.Status.ObservedGeneration)
	// SubmitTask itself should return quickly (the executor doesn't
	// wait on the vendor); a 2-minute ceiling covers network blips
	// without holding the reconcile loop hostage.
	rpcCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	result, err := r.Executor.SubmitTask(rpcCtx, idempotencyKey, circuit, qpu)
	if err != nil {
		var taskErr *executor.TaskError
		if errors.As(err, &taskErr) {
			log.Info("Executor reported submit failure",
				"reason", taskErr.Reason, "message", taskErr.Message)
			return r.fail(ctx, circuit, taskErr.Reason, taskErr.Message)
		}
		log.Error(err, "Executor SubmitTask RPC failed; will requeue")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Persist converted QASM (qiskit-source path) as an artifact, same as
	// the sync runSync path.  Best-effort: requeue on transient errors.
	var convertedRef *qccv1alpha1.ArtifactRef
	if result.ConvertedQASM != "" {
		cmName, upErr := r.upsertArtifactConfigMap(ctx, circuit,
			qccv1alpha1.ArtifactSuffixConverted, qccv1alpha1.ArtifactDataKeyQASM, result.ConvertedQASM)
		if upErr != nil {
			log.Error(upErr, "ConfigMap upsert for converted QASM failed; will requeue")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		convertedRef = &qccv1alpha1.ArtifactRef{Name: cmName}
	}

	patch := client.MergeFrom(circuit.DeepCopy())
	circuit.Status.ProviderJobID = result.TaskID
	if convertedRef != nil {
		circuit.Status.ConvertedRef = convertedRef
	}
	if result.Depth > 0 || result.TwoQubitGates > 0 || result.TotalGates > 0 {
		circuit.Status.Transpile = &qccv1alpha1.CircuitTranspileMetrics{
			Depth:         result.Depth,
			TwoQubitGates: result.TwoQubitGates,
			TotalGates:    result.TotalGates,
		}
	}
	setCondition(circuit, qccv1alpha1.ConditionSubmitted,
		"ProviderSubmitted",
		fmt.Sprintf("Submitted to %s (taskID=%s); waiting in vendor queue",
			result.BackendUsed, result.TaskID))
	circuit.Status.Phase = qccv1alpha1.PhaseRunning
	if err := r.Status().Patch(ctx, circuit, patch); err != nil {
		return ctrl.Result{}, err
	}
	// Requeue to start polling.  Real-hardware queues vary from
	// seconds to many minutes; the poll cadence (10s here, 5s inside
	// the executor's WatchTask stream) gives reasonable UX without
	// hammering the provider API.  controller-runtime will throttle
	// further if the controller itself becomes contended.
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// pollAsyncJob handles PhaseRunning: opens a short-lived WatchTask
// stream, reads at most one status frame, decides:
//   - terminal DONE → call FetchTaskResult, stamp counts, transition to Succeeded
//   - terminal FAILED → mark Circuit Failed with the vendor's reason
//   - non-terminal → requeue with backoff so the next reconcile polls again
//
// Streaming-stream-with-single-read is intentional: it matches the
// reconcile-loop's natural polling cadence while letting the streaming
// API stay useful for richer clients (CLI live status) later.
func (r *CircuitReconciler) pollAsyncJob(ctx context.Context, circuit *qccv1alpha1.Circuit) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if r.Executor == nil {
		return r.fail(ctx, circuit, qccv1alpha1.ReasonProviderSubmissionFailed,
			"executor client not configured")
	}
	taskID := circuit.Status.ProviderJobID
	if taskID == "" {
		// Shouldn't happen — submitAsync stamps it before transitioning
		// to Running.  Fail loudly if state is inconsistent.
		return r.fail(ctx, circuit, qccv1alpha1.ReasonProviderSubmissionFailed,
			"Circuit is in Running phase but status.providerJobID is empty")
	}

	// Bounded watch deadline — read one frame and close the stream.
	rpcCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	statusCh, errCh, err := r.Executor.WatchTask(rpcCtx, taskID)
	if err != nil {
		log.Error(err, "WatchTask open failed; will requeue")
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	var taskStatus executor.TaskStatus
	select {
	case s, ok := <-statusCh:
		if !ok {
			// Stream closed before yielding a frame.  Requeue.
			log.Info("WatchTask closed without status; will requeue")
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		taskStatus = s
	case watchErr := <-errCh:
		var taskErr *executor.TaskError
		if errors.As(watchErr, &taskErr) {
			return r.fail(ctx, circuit, taskErr.Reason, taskErr.Message)
		}
		log.Error(watchErr, "WatchTask stream error; will requeue")
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	case <-rpcCtx.Done():
		log.Info("WatchTask deadline; will requeue")
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// Non-terminal → requeue.  Log the status message (queue position,
	// running step) for visibility in controller logs.
	if !taskStatus.IsTerminal() {
		log.Info("Task in progress",
			"task_id", taskID,
			"state", taskStatus.State.String(),
			"message", taskStatus.Message)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Terminal — fetch counts (DONE) or mark failed (FAILED/CANCELLED).
	if taskStatus.State != executorv1.TaskStatus_TASK_STATUS_DONE {
		return r.fail(ctx, circuit, qccv1alpha1.ReasonProviderSubmissionFailed,
			fmt.Sprintf("Vendor job ended in non-DONE state %s: %s",
				taskStatus.State.String(), taskStatus.Message))
	}

	// Fetch counts on a separate short-lived context.
	fetchCtx, fetchCancel := context.WithTimeout(ctx, 30*time.Second)
	defer fetchCancel()
	taskResult, err := r.Executor.FetchTaskResult(fetchCtx, taskID)
	if err != nil {
		var taskErr *executor.TaskError
		if errors.As(err, &taskErr) {
			return r.fail(ctx, circuit, taskErr.Reason, taskErr.Message)
		}
		log.Error(err, "FetchTaskResult failed; will requeue")
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	patch := client.MergeFrom(circuit.DeepCopy())
	circuit.Status.Results = taskResult.Counts
	circuit.Status.UsageSeconds = taskResult.UsageSeconds
	setCondition(circuit, qccv1alpha1.ConditionCompleted,
		qccv1alpha1.ReasonExecutionCompleted,
		fmt.Sprintf("Execution completed with %d outcome(s)", len(taskResult.Counts)))
	circuit.Status.Phase = qccv1alpha1.PhaseSucceeded
	if err := r.Status().Patch(ctx, circuit, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *CircuitReconciler) renderDrawing(ctx context.Context, circuit *qccv1alpha1.Circuit) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if r.Executor == nil {
		return r.fail(ctx, circuit, qccv1alpha1.ReasonRenderingFailed,
			"executor client not configured")
	}

	rpcCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	drawing, err := r.Executor.DrawCircuit(rpcCtx, circuit.Spec.Source)
	if err != nil {
		var taskErr *executor.TaskError
		if errors.As(err, &taskErr) {
			log.Info("Executor reported rendering failure",
				"reason", taskErr.Reason, "message", taskErr.Message)
			return r.fail(ctx, circuit, taskErr.Reason, taskErr.Message)
		}
		log.Error(err, "Executor DrawCircuit RPC failed; will requeue")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	cmName, err := r.upsertArtifactConfigMap(ctx, circuit,
		qccv1alpha1.ArtifactSuffixDrawing, qccv1alpha1.ArtifactDataKeyDrawing, drawing)
	if err != nil {
		log.Error(err, "ConfigMap upsert for drawing failed; will requeue")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	patch := client.MergeFrom(circuit.DeepCopy())
	circuit.Status.DrawingRef = &qccv1alpha1.ArtifactRef{Name: cmName}
	setCondition(circuit, qccv1alpha1.ConditionRendered,
		qccv1alpha1.ReasonDrawingRendered,
		fmt.Sprintf("Rendered %d bytes of ASCII into ConfigMap %s", len(drawing), cmName))
	setCondition(circuit, qccv1alpha1.ConditionCompleted,
		qccv1alpha1.ReasonDrawingRendered,
		"Drawing completed in mode=draw")
	circuit.Status.Phase = qccv1alpha1.PhaseSucceeded
	if err := r.Status().Patch(ctx, circuit, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// renderSchedule drives mode=schedule to terminal: calls
// Executor.ScheduleCircuit on the resolved QPU, serialises the
// per-instruction timeline to JSON, persists it as a ConfigMap
// artifact, and marks the Circuit Succeeded.  Mirrors renderDrawing
// in structure but is backend-aware — scheduling needs a Target.
func (r *CircuitReconciler) renderSchedule(ctx context.Context, circuit *qccv1alpha1.Circuit) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if r.Executor == nil {
		return r.fail(ctx, circuit, qccv1alpha1.ReasonSchedulingFailed,
			"executor client not configured")
	}

	// Re-fetch the QPU chosen during Move 1, same convention as runOnExecutor.
	var qpu qccv1alpha1.QPU
	if err := r.Get(ctx, types.NamespacedName{Name: circuit.Status.SelectedQPU}, &qpu); err != nil {
		if apierrors.IsNotFound(err) {
			return r.fail(ctx, circuit, qccv1alpha1.ReasonNoEligibleBackend,
				fmt.Sprintf("QPU %q no longer exists", circuit.Status.SelectedQPU))
		}
		log.Error(err, "Fetch selected QPU failed; will requeue")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	rpcCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	sched, err := r.Executor.ScheduleCircuit(rpcCtx, circuit, &qpu)
	if err != nil {
		var taskErr *executor.TaskError
		if errors.As(err, &taskErr) {
			log.Info("Executor reported scheduling failure",
				"reason", taskErr.Reason, "message", taskErr.Message)
			return r.fail(ctx, circuit, taskErr.Reason, taskErr.Message)
		}
		log.Error(err, "Executor ScheduleCircuit RPC failed; will requeue")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	payload, err := json.Marshal(sched)
	if err != nil {
		log.Error(err, "Schedule JSON encoding failed; will requeue")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	cmName, err := r.upsertArtifactConfigMap(ctx, circuit,
		qccv1alpha1.ArtifactSuffixSchedule, qccv1alpha1.ArtifactDataKeySchedule, string(payload))
	if err != nil {
		log.Error(err, "ConfigMap upsert for schedule failed; will requeue")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	patch := client.MergeFrom(circuit.DeepCopy())
	circuit.Status.ScheduleRef = &qccv1alpha1.ArtifactRef{Name: cmName}
	setCondition(circuit, qccv1alpha1.ConditionScheduled,
		qccv1alpha1.ReasonScheduleProduced,
		fmt.Sprintf("Scheduled %d ops (total %d dt) into ConfigMap %s",
			len(sched.Ops), sched.TotalDurationDt, cmName))
	setCondition(circuit, qccv1alpha1.ConditionCompleted,
		qccv1alpha1.ReasonScheduleProduced,
		"Schedule completed in mode=schedule")
	circuit.Status.Phase = qccv1alpha1.PhaseSucceeded
	if err := r.Status().Patch(ctx, circuit, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// upsertArtifactConfigMap stores an arbitrary generated artifact in a
// ConfigMap owned by the Circuit (so the GC reaps it when the Circuit is
// deleted).  The ConfigMap is named <circuit-name>-<suffix> for predictable
// kubectl lookup; suffix doubles as the `qcc.io/artifact` label value.
// Idempotent: if a previous reconcile already created the ConfigMap, the
// data is updated in place rather than re-created.
//
// See QCC-API.md §3.7 for the artifact-ref convention this implements.
func (r *CircuitReconciler) upsertArtifactConfigMap(
	ctx context.Context,
	circuit *qccv1alpha1.Circuit,
	suffix, dataKey, payload string,
) (string, error) {
	cmName := circuit.Name + "-" + suffix
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: circuit.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "quantum-circuit-controller",
				"app.kubernetes.io/managed-by": "qcc-controller",
				"qcc.io/circuit":               circuit.Name,
				"qcc.io/artifact":              suffix,
			},
		},
		Data: map[string]string{dataKey: payload},
	}
	if err := controllerutil.SetControllerReference(circuit, cm, r.Scheme); err != nil {
		return "", fmt.Errorf("set owner reference on %s ConfigMap: %w", suffix, err)
	}

	if err := r.Create(ctx, cm); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return "", fmt.Errorf("create %s ConfigMap: %w", suffix, err)
		}
		// Already exists (e.g. controller restart mid-reconcile) — patch
		// data in place.  Fetch first so we keep any unrelated labels.
		var existing corev1.ConfigMap
		nn := types.NamespacedName{Name: cmName, Namespace: circuit.Namespace}
		if err := r.Get(ctx, nn, &existing); err != nil {
			return "", fmt.Errorf("get existing %s ConfigMap: %w", suffix, err)
		}
		existing.Data = cm.Data
		if err := r.Update(ctx, &existing); err != nil {
			return "", fmt.Errorf("update existing %s ConfigMap: %w", suffix, err)
		}
	}
	return cmName, nil
}

func (r *CircuitReconciler) fail(ctx context.Context, circuit *qccv1alpha1.Circuit, reason, message string) (ctrl.Result, error) {
	patch := client.MergeFrom(circuit.DeepCopy())
	setCondition(circuit, qccv1alpha1.ConditionFailed, reason, message)
	circuit.Status.Phase = qccv1alpha1.PhaseFailed
	if circuit.Status.ObservedGeneration == 0 {
		circuit.Status.ObservedGeneration = circuit.Generation
	}
	if err := r.Status().Patch(ctx, circuit, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// latestConditionTime returns the most recent LastTransitionTime
// across all Conditions on the Circuit, or zero time if the Circuit
// has no conditions yet.  Used at the top of Reconcile to capture
// the "phase entered at" timestamp for the
// qcc_circuit_phase_duration_seconds histogram.
//
// Why most-recent-condition rather than a dedicated field: the
// CircuitStatus doesn't have a `PhaseEnteredAt` field, and every
// phase transition sets at least one Condition (Accepted, Validated,
// Selected, Submitted, Rendered, Scheduled, Completed, Failed).
// LastTransitionTime on the most recent condition is therefore a
// reasonable proxy for "when did the Circuit enter its current
// phase" — approximate but good enough for thesis-scale dashboards.
func latestConditionTime(circuit *qccv1alpha1.Circuit) time.Time {
	var latest time.Time
	for _, c := range circuit.Status.Conditions {
		if c.LastTransitionTime.After(latest) {
			latest = c.LastTransitionTime.Time
		}
	}
	return latest
}

// latestConditionReason returns the reason string of the most recent
// Condition transition, or empty string if there are no Conditions.
// Used to label qcc_circuits_total{reason=...}.
func latestConditionReason(circuit *qccv1alpha1.Circuit) string {
	var latest metav1.Condition
	for i := range circuit.Status.Conditions {
		c := circuit.Status.Conditions[i]
		if c.LastTransitionTime.After(latest.LastTransitionTime.Time) {
			latest = c
		}
	}
	return latest.Reason
}

// setCondition sets condType=True with the given reason/message.  When False
// conditions are needed, plumb status through.
func setCondition(circuit *qccv1alpha1.Circuit, condType, reason, message string) {
	for i, c := range circuit.Status.Conditions {
		if c.Type == condType {
			if c.Status == metav1.ConditionTrue && c.Reason == reason {
				return
			}
			circuit.Status.Conditions[i] = metav1.Condition{
				Type:               condType,
				Status:             metav1.ConditionTrue,
				Reason:             reason,
				Message:            message,
				ObservedGeneration: circuit.Generation,
				LastTransitionTime: metav1.Now(),
			}
			return
		}
	}
	circuit.Status.Conditions = append(circuit.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: circuit.Generation,
		LastTransitionTime: metav1.Now(),
	})
}

func (r *CircuitReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&qccv1alpha1.Circuit{}).
		Named("circuit").
		Complete(r)
}
