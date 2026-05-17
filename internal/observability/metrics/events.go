/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package metrics

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Event-driven Circuit instruments live here (as opposed to the
// per-scrape ObservableGauges in circuit.go).  These are the
// synchronous counter and histogram from `QCC-Observability.md` §5.2
// rows 2 and 3 — incremented / observed from the reconciler at the
// same call sites that emit K8s Events.
//
// The instruments are package-level variables (guarded by a sync.Once)
// so the reconciler can call `metrics.RecordPhaseTransition(...)`
// directly without threading an instrument handle through the
// reconciler struct.  This mirrors the controller-runtime pattern of
// `metrics.Registry.MustRegister(...)` global state — simpler than
// dependency-injecting a struct of instruments into every reconciler.
var (
	eventsOnce       sync.Once
	eventsInitErr    error
	circuitsTotal    metric.Int64Counter
	phaseDurationSec metric.Float64Histogram
)

// phaseDurationBuckets covers the actual observed range of phase
// durations in QCC — from sub-second reconciler operations
// (Selecting, Transpiling on Aer) up to half-hour IBM hardware
// queue waits.  See `QCC-Observability.md` §4.7 for the rationale.
var phaseDurationBuckets = []float64{
	0.01, 0.1, 0.5, 1, 5, 30, 120, 600, 1800,
	// 10ms, 100ms, 500ms, 1s, 5s, 30s, 2m, 10m, 30m
}

// RegisterEvents allocates the event-driven Circuit instruments
// (qcc_circuits_total counter, qcc_circuit_phase_duration_seconds
// histogram) on the global meter provider.  Idempotent: calling
// twice is a no-op (guarded by sync.Once).
//
// Must be called after `observability.Setup` has set the global
// meter provider but before any reconciler tries to record events.
// Called from cmd/qcc-controller/main.go alongside the QPU/Circuit
// observable registrations.
func RegisterEvents() error {
	eventsOnce.Do(func() {
		meter := otel.Meter(meterName)

		circuitsTotal, eventsInitErr = meter.Int64Counter(
			"qcc_circuits_total",
			metric.WithDescription(
				"Cumulative count of Circuit phase transitions.  "+
					"Labelled by (circuit, namespace, uid, provider_job_id, "+
					"phase, reason, qpu, mode).  Aggregate across instances "+
					"via `sum without(circuit, namespace, uid, provider_job_id)`.",
			),
		)
		if eventsInitErr != nil {
			eventsInitErr = fmt.Errorf("declare qcc_circuits_total: %w", eventsInitErr)
			return
		}

		phaseDurationSec, eventsInitErr = meter.Float64Histogram(
			"qcc_circuit_phase_duration_seconds",
			metric.WithDescription(
				"Time spent in each Circuit lifecycle phase, in seconds.  "+
					"Custom buckets cover 10ms–30m (real-hardware queue waits).",
			),
			metric.WithUnit("s"),
			metric.WithExplicitBucketBoundaries(phaseDurationBuckets...),
		)
		if eventsInitErr != nil {
			eventsInitErr = fmt.Errorf("declare qcc_circuit_phase_duration_seconds: %w", eventsInitErr)
			return
		}
	})
	return eventsInitErr
}

// RecordPhaseTransition increments the per-Circuit phase-transition
// counter and records the previous phase's duration.  Called from
// the controller's reconcile loop at the same site that emits the
// K8s Event for a transition.
//
//   - phase: the phase the Circuit just entered (e.g. "Submitting").
//   - reason: the condition reason on the new phase (empty on success
//     paths; populated on failure like "TranspilationFailed").
//   - prevPhase / prevDurationSeconds: the phase being left and how
//     long it was active.  prevPhase is the empty string on the
//     first transition (no previous phase to attribute a duration to,
//     in which case the histogram observation is skipped).
//
// `uid` and `providerJobID` are passed through to both the counter
// and the histogram so cross-boundary linkage from substrate-side
// logs (`provider_job_id`) and reverse-linkage from K8s audit
// (`uid`) work on phase-timing series as well as the info series.
// `provider_job_id` may be the empty string for early phases that
// haven't yet reached Submitting; that's expected.
//
// Cardinality note: both extra labels are 1-to-1 with the Circuit
// they're attached to, so they enrich existing series without
// multiplying cardinality.
//
// Safe to call before RegisterEvents (no-op when instruments are nil,
// covers the brief window before main.go finishes wiring).
func RecordPhaseTransition(
	ctx context.Context,
	circuit, namespace, uid, providerJobID, phase, reason, qpu, mode, prevPhase string,
	prevDurationSeconds float64,
) {
	if circuitsTotal == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String("circuit", circuit),
		attribute.String("namespace", namespace),
		attribute.String("uid", uid),
		attribute.String("provider_job_id", providerJobID),
		attribute.String("phase", phase),
		attribute.String("reason", reason),
		attribute.String("qpu", qpu),
		attribute.String("mode", mode),
	)
	circuitsTotal.Add(ctx, 1, attrs)

	if prevPhase != "" && prevDurationSeconds >= 0 && phaseDurationSec != nil {
		phaseDurationSec.Record(ctx, prevDurationSeconds,
			metric.WithAttributes(
				attribute.String("circuit", circuit),
				attribute.String("namespace", namespace),
				attribute.String("uid", uid),
				attribute.String("provider_job_id", providerJobID),
				attribute.String("phase", prevPhase),
				attribute.String("qpu", qpu),
			),
		)
	}
}
