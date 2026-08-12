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
	"errors"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	qccv1alpha1 "github.com/ioaiaaii/quantum-circuit-controller/api/v1alpha1"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/executor"
)

// Reserved provider values recognised by the QPU reconciler.  Adding a
// new provider is just adding a const + a switch case in desiredQPUStatus;
// the actual adapter dispatch lives in the executor (Python side).  See
// QCC-API.md §4.4 and QCC-Design-State.md §7a (Composition Principle).
const (
	// providerLocal — in-process Qiskit Aer simulator (no credentials).
	// Trivially Available the moment the QPU CR exists.
	providerLocal = "local"
	// providerIBM — IBM Quantum Platform via qiskit-ibm-runtime.
	// Requires QISKIT_IBM_TOKEN env var on the executor pod.  Treated
	// as optimistically Available; the probe surfaces live calibration
	// from the IBM cloud.  Probe failures (bad token, network errors)
	// leave the QPU Available with empty calibration and land on
	// status.lastError and the MetadataFresh condition.
	providerIBM = "ibm"
)

// QPUReconciler manages QPU resources.  In M1 the reconciler's job is
// deliberately small: stamp status.availability on the QPU based on its
// provider, so the Circuit reconciler's selection chain (Move 1) has
// something to filter on.  Real-hardware provider probes, calibration
// refresh on a TTL, and queue-depth scraping are M2 (see
// QCC-Design-State.md Roadmap and QCC-System-Design.md §7.1).
type QPUReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Executor Executor
}

// +kubebuilder:rbac:groups=qcc.io,resources=qpus,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=qcc.io,resources=qpus/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=qcc.io,resources=qpus/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the QPU object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.24.1/pkg/reconcile
func (r *QPUReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	qpu := &qccv1alpha1.QPU{}
	if err := r.Get(ctx, req.NamespacedName, qpu); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !qpu.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	desired := desiredQPUStatus(qpu)

	// Probe the backend the first time we mark a QPU Available.  M2 will
	// replace this "probe once" policy with the TTL-driven refresh from
	// QCC-System-Design.md §7.1; for M1.5b a single probe suffices since
	// fake_* snapshots are frozen and don't drift between reconciles.
	needsProbe := desired.Availability == qccv1alpha1.QPUAvailable && qpu.Status.Qubits == 0

	if !needsProbe && qpuStatusMatches(qpu.Status, desired, qpu.Generation) {
		// Nothing to do — already in the right shape and probe data is
		// already present (or wasn't applicable).
		return ctrl.Result{}, nil
	}

	// Snapshot the QPU *before* any in-memory mutation so the eventual
	// MergeFrom captures both the desired-status fields and the probe
	// enrichment in one patch.  Mutating after the snapshot is the
	// canonical controller-runtime pattern.
	base := qpu.DeepCopy()

	// Provider policy first; the probe outcome below overrides it.
	qpu.Status.Availability = desired.Availability
	qpu.Status.ObservedGeneration = qpu.Generation
	for _, cond := range desired.Conditions {
		setQPUCondition(qpu, cond)
	}

	if needsProbe {
		if profile, err := r.probeBackend(ctx, qpu); err != nil {
			// Non-fatal: qubits stays zero, the next reconcile
			// retries, and the failure lands on status.lastError.
			log.Info("ProbeBackend failed; continuing without enrichment",
				"qpu", qpu.Name, "error", err.Error())
			setQPULastError(qpu, qccv1alpha1.ReasonProviderProbeFailed, err.Error())
			setQPUCondition(qpu, metav1.Condition{
				Type:               qccv1alpha1.ConditionMetadataFresh,
				Status:             metav1.ConditionFalse,
				Reason:             qccv1alpha1.ReasonProviderProbeFailed,
				Message:            err.Error(),
				LastTransitionTime: metav1.Now(),
			})
		} else {
			applyBackendProfile(qpu, profile)
			qpu.Status.LastError = nil
			setQPUCondition(qpu, metav1.Condition{
				Type:               qccv1alpha1.ConditionMetadataFresh,
				Status:             metav1.ConditionTrue,
				Reason:             qccv1alpha1.ReasonCalibrationRefreshed,
				Message:            "probe refreshed backend metadata",
				LastTransitionTime: metav1.Now(),
			})
		}
	}

	if err := r.Status().Patch(ctx, qpu, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// probeBackend wraps the Executor.ProbeBackend RPC with the standard
// transient-vs-terminal error categorisation.  Returns nil + error when
// the probe couldn't run; the caller logs and proceeds without
// enrichment (the QPU still goes Available based on its provider).
func (r *QPUReconciler) probeBackend(ctx context.Context, qpu *qccv1alpha1.QPU) (*executor.BackendProfile, error) {
	if r.Executor == nil {
		return nil, errors.New("executor client not configured")
	}
	rpcCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return r.Executor.ProbeBackend(rpcCtx, qpu.Spec.Provider, qpu.EffectiveBackendName())
}

// applyBackendProfile copies the probe result onto the QPU's status,
// converting wire types into the K8s shape (e.g. RFC 3339 string →
// metav1.Time).  Skips fields whose source is zero so a probe that
// returns partial data (e.g. generic Aer reports only num_qubits)
// doesn't blank out fields that may have been set earlier.
func applyBackendProfile(qpu *qccv1alpha1.QPU, p *executor.BackendProfile) {
	if p.NumQubits > 0 {
		qpu.Status.Qubits = p.NumQubits
	}
	if len(p.BasisGates) > 0 {
		qpu.Status.BasisGates = p.BasisGates
	}
	if p.CouplingEdges > 0 {
		qpu.Status.CouplingEdges = p.CouplingEdges
	}
	if p.LastCalibrationTime != "" {
		if t, err := time.Parse(time.RFC3339, p.LastCalibrationTime); err == nil {
			mt := metav1.NewTime(t)
			qpu.Status.LastCalibrationTime = &mt
		}
	}
	if p.SingleQubitErrorMedian > 0 || p.TwoQubitErrorMedian > 0 || p.ReadoutErrorMedian > 0 {
		qpu.Status.ErrorMedians = &qccv1alpha1.QPUErrorMedians{
			SingleQubit: p.SingleQubitErrorMedian,
			TwoQubit:    p.TwoQubitErrorMedian,
			Readout:     p.ReadoutErrorMedian,
		}
	}
	if p.T1MedianMicros > 0 || p.T2MedianMicros > 0 {
		qpu.Status.CoherenceMedians = &qccv1alpha1.QPUCoherenceMedians{
			T1Micros: p.T1MedianMicros,
			T2Micros: p.T2MedianMicros,
		}
	}
	if p.DtSeconds > 0 {
		qpu.Status.DtSeconds = p.DtSeconds
	}
	if p.SingleQubitDurationMedianSeconds > 0 || p.TwoQubitDurationMedianSeconds > 0 {
		qpu.Status.InstructionDurationMedians = &qccv1alpha1.QPUInstructionDurationMedians{
			SingleQubitSeconds: p.SingleQubitDurationMedianSeconds,
			TwoQubitSeconds:    p.TwoQubitDurationMedianSeconds,
		}
	}
	// Processor identity is non-blank only when the backend's
	// processor_type metadata is populated.  Generic Aer reports nothing
	// here, and we deliberately leave Status.Processor nil rather than
	// writing a {Family: "", Revision: ""} record so the renderer can
	// trivially detect absence with a nil check.
	if p.ProcessorFamily != "" {
		qpu.Status.Processor = &qccv1alpha1.QPUProcessor{
			Family:   p.ProcessorFamily,
			Revision: p.ProcessorRevision,
			Segment:  p.ProcessorSegment,
		}
	}
}

// desiredQPUStatus computes the status the reconciler wants the QPU to
// converge to, given its spec.  Pure function — no API calls, no clock
// reads — so it's trivially testable.
//
// Provider policies:
//
//   - provider=local → Available, with Ready and MetadataFresh conditions
//     True (Aer has no remote state, so freshness is degenerate).
//   - provider=ibm → Available (optimistic), Ready True.  MetadataFresh
//     not asserted because IBM calibration drifts live; the probe path
//     surfaces freshness via status.lastCalibrationTime.  If the probe
//     fails (bad credentials, network), the QPU stays Available with
//     empty calibration — the next reconcile retries.
//   - any other provider → Unknown.  Move-1 filter rejects Unknown
//     candidates from selection.
func desiredQPUStatus(qpu *qccv1alpha1.QPU) qccv1alpha1.QPUStatus {
	switch qpu.Spec.Provider {
	case providerLocal:
		now := metav1.Now()
		return qccv1alpha1.QPUStatus{
			Availability: qccv1alpha1.QPUAvailable,
			Conditions: []metav1.Condition{
				{
					Type:               qccv1alpha1.ConditionReady,
					Status:             metav1.ConditionTrue,
					Reason:             qccv1alpha1.ReasonProviderProbeOK,
					Message:            fmt.Sprintf("local provider %q is Available without probe", qpu.EffectiveBackendName()),
					LastTransitionTime: now,
				},
				{
					Type:               qccv1alpha1.ConditionMetadataFresh,
					Status:             metav1.ConditionTrue,
					Reason:             qccv1alpha1.ReasonCalibrationRefreshed,
					Message:            "local providers have no remote calibration; freshness is degenerate",
					LastTransitionTime: now,
				},
			},
		}

	case providerIBM:
		now := metav1.Now()
		return qccv1alpha1.QPUStatus{
			Availability: qccv1alpha1.QPUAvailable,
			Conditions: []metav1.Condition{
				{
					Type:   qccv1alpha1.ConditionReady,
					Status: metav1.ConditionTrue,
					Reason: qccv1alpha1.ReasonProviderProbeOK,
					Message: fmt.Sprintf(
						"ibm provider %q is Available; live calibration via probe (status.lastCalibrationTime)",
						qpu.EffectiveBackendName(),
					),
					LastTransitionTime: now,
				},
				// MetadataFresh intentionally omitted: real-hardware
				// calibration drifts over hours.  Freshness is tracked
				// via status.lastCalibrationTime, not a static condition.
			},
		}

	default:
		// Other providers — no adapter wired today.  Alternative
		// substrates (QRMI for Pasqal/multi-vendor, CUDA-Q for NVIDIA)
		// are Ch9 future-work per QCC-Design-State.md §7d.  Unknown is
		// the honest state; Move-1 filter rejects it from selection.
		return qccv1alpha1.QPUStatus{
			Availability: qccv1alpha1.QPUUnknown,
		}
	}
}

// qpuStatusMatches reports whether the observed status already reflects
// the desired one.  Lets the reconciler skip status patches on no-ops so
// requeues don't generate churn events.  Only the M1-relevant fields are
// compared; M2 will extend this.
func qpuStatusMatches(observed, desired qccv1alpha1.QPUStatus, generation int64) bool {
	if observed.ObservedGeneration != generation {
		return false
	}
	if observed.Availability != desired.Availability {
		return false
	}
	// Every condition the desired state names must be present and True
	// on the observed side.  Conditions we didn't compute (e.g. M2-set)
	// are intentionally ignored — we only own what we set.
	for _, want := range desired.Conditions {
		got := findCondition(observed.Conditions, want.Type)
		if got == nil || got.Status != want.Status || got.Reason != want.Reason {
			return false
		}
	}
	return true
}

// setQPULastError records a probe failure, keeping the timestamp when
// the same failure repeats.
func setQPULastError(qpu *qccv1alpha1.QPU, reason, message string) {
	if prev := qpu.Status.LastError; prev != nil && prev.Reason == reason && prev.Message == message {
		return
	}
	qpu.Status.LastError = &qccv1alpha1.QPULastError{
		Time:    metav1.Now(),
		Reason:  reason,
		Message: message,
	}
}

func findCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}

// setQPUCondition replaces or appends a condition on the QPU's status,
// preserving the LastTransitionTime when the (Type, Status, Reason) tuple
// hasn't changed.  This matches the controller-runtime convention so
// kubectl describe doesn't flip transition timestamps on every reconcile.
func setQPUCondition(qpu *qccv1alpha1.QPU, want metav1.Condition) {
	want.ObservedGeneration = qpu.Generation
	for i, existing := range qpu.Status.Conditions {
		if existing.Type != want.Type {
			continue
		}
		if existing.Status == want.Status && existing.Reason == want.Reason {
			// No meaningful change; keep the original LastTransitionTime.
			want.LastTransitionTime = existing.LastTransitionTime
		}
		qpu.Status.Conditions[i] = want
		return
	}
	qpu.Status.Conditions = append(qpu.Status.Conditions, want)
}

func (r *QPUReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&qccv1alpha1.QPU{}).
		Named("qpu").
		Complete(r)
}
