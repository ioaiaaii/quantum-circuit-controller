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
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	qccv1alpha1 "github.com/ioaiaaii/quantum-circuit-controller/api/v1alpha1"
)

// circuitInstruments bundles the Circuit-side observable instruments.
// All read from `Circuit.status` via the cache on each scrape — same
// pattern as QPU metrics.  The synchronous Counter + Histogram for
// phase-transition events are declared in events.go.
//
// Note: `qcc_circuit_shots` was dropped in favour of carrying `shots`
// as a label on `qcc_circuit_info` (set-once, identity-like value).
// The `sum by(qpu)(qcc_circuit_shots)` use case is recoverable via
// CRD aggregation (`kubectl get circuits -o json`) and isn't load-
// bearing for Ch7 figures.
type circuitInstruments struct {
	info               metric.Int64ObservableGauge
	transpileDepth     metric.Int64ObservableGauge
	transpileGates     metric.Int64ObservableGauge
	resultCount        metric.Int64ObservableGauge
	phaseDurationObsrv metric.Float64ObservableGauge
	usageSeconds       metric.Float64ObservableGauge
}

// RegisterCircuitMetrics declares the Circuit-side ObservableGauges
// and installs the per-scrape callback that observes them from the
// cache.  Mirrors RegisterQPUMetrics but reads CircuitList instead.
//
// Inventory:
//
//   - qcc_circuit_info        — identity, value=1, labels carry shots+qpu+mode+source_format
//   - qcc_circuit_transpile_depth
//   - qcc_circuit_transpile_gates  (label `kind` = single_qubit | two_qubit | total)
//   - qcc_circuit_result_count    (label `bitstring`; straddles L3/L4 of Kanazawa)
//
// Plus the event-driven counter + histogram from `events.go`
// (qcc_circuits_total, qcc_circuit_phase_duration_seconds).
//
// `qcc_circuit_result_count` straddles the L3 (task-level artifacts)
// to L4 (domain outcomes) boundary of Kanazawa's pyramid — per-
// bitstring outcome counts are the raw form of fidelity / TVD
// analytics.  Cardinality grows as `2^qubits × Circuits`; safe at
// thesis-circuit scale (≤ ~5 qubits, ~50 Circuits across thesis
// lifetime → ~1000 series budget).  Re-evaluate when VQE-scale
// workloads land.  See `QCC-Observability.md` §5.2 for the discussion.
func RegisterCircuitMetrics(c client.Client) error {
	meter := otel.Meter(meterName)

	inst, err := declareCircuitInstruments(meter)
	if err != nil {
		return err
	}

	_, err = meter.RegisterCallback(
		func(ctx context.Context, obs metric.Observer) error {
			return observeCircuits(ctx, c, obs, inst)
		},
		inst.info,
		inst.transpileDepth,
		inst.transpileGates,
		inst.resultCount,
		inst.phaseDurationObsrv,
		inst.usageSeconds,
	)
	if err != nil {
		return fmt.Errorf("register Circuit metrics callback: %w", err)
	}
	return nil
}

func declareCircuitInstruments(meter metric.Meter) (circuitInstruments, error) {
	var inst circuitInstruments
	var err error

	if inst.info, err = meter.Int64ObservableGauge(
		"qcc_circuit_info",
		metric.WithDescription(
			"Static identity of a Circuit (always 1; labels carry circuit, "+
				"namespace, uid, mode, source_format, shots, qpu, "+
				"provider_job_id for PromQL joins and cross-boundary "+
				"linkage from substrate-side logs back to QCC metrics).",
		),
	); err != nil {
		return inst, fmt.Errorf("declare qcc_circuit_info: %w", err)
	}

	if inst.transpileDepth, err = meter.Int64ObservableGauge(
		"qcc_circuit_transpile_depth",
		metric.WithDescription(
			"Post-transpile depth (longest chain of dependent gates) for a Circuit on the resolved QPU.",
		),
	); err != nil {
		return inst, fmt.Errorf("declare qcc_circuit_transpile_depth: %w", err)
	}

	if inst.transpileGates, err = meter.Int64ObservableGauge(
		"qcc_circuit_transpile_gates",
		metric.WithDescription(
			"Post-transpile gate counts for a Circuit on the resolved QPU.  "+
				"The `kind` label distinguishes single_qubit / two_qubit / total.",
		),
	); err != nil {
		return inst, fmt.Errorf("declare qcc_circuit_transpile_gates: %w", err)
	}

	if inst.resultCount, err = meter.Int64ObservableGauge(
		"qcc_circuit_result_count",
		metric.WithDescription(
			"Per-bitstring measurement-outcome counts from a Circuit's terminal results.  "+
				"Label `bitstring` carries the classical-bit register value (e.g. \"00\", \"11\").  "+
				"Cardinality grows as 2^qubits per Circuit; safe at thesis-circuit scale.",
		),
	); err != nil {
		return inst, fmt.Errorf("declare qcc_circuit_result_count: %w", err)
	}

	// qcc_circuit_phase_duration_seconds_observed is the
	// cache-derived companion to the synchronous histogram
	// `qcc_circuit_phase_duration_seconds` in events.go.  The
	// histogram captures phase transitions as they happen (good
	// for fleet-wide percentiles via histogram_quantile); this
	// gauge derives the same durations from
	// `status.conditions[].lastTransitionTime` deltas every
	// scrape, so per-Circuit drill-down panels survive controller
	// restarts and Prometheus's 5-minute staleness window.
	//
	// Granularity is at K8s-condition level, not 5-phase
	// granularity:
	//   - phase="Pending"    -> creationTimestamp → Accepted condition
	//   - phase="Selecting"  -> Accepted → Selected condition
	//   - phase="Submitting" -> Selected → Submitted condition
	//                          (covers Transpiling + Submitting,
	//                          which conditions can't separate)
	//   - phase="Running"    -> Submitted → Completed|Failed
	//
	// Conditions that fired in the same reconcile share a
	// timestamp; the corresponding phase duration is 0s.  That's
	// accurate, not broken.
	if inst.phaseDurationObsrv, err = meter.Float64ObservableGauge(
		"qcc_circuit_phase_duration_seconds_observed",
		metric.WithDescription(
			"Per-phase wall-clock duration (seconds) derived from "+
				"status.conditions[].lastTransitionTime deltas.  "+
				"Persistent companion to the synchronous histogram "+
				"qcc_circuit_phase_duration_seconds; survives "+
				"controller restarts and Prometheus staleness.  "+
				"Granularity is at K8s-condition level (4 phases: "+
				"Pending, Selecting, Submitting, Running).",
		),
		metric.WithUnit("s"),
	); err != nil {
		return inst, fmt.Errorf("declare qcc_circuit_phase_duration_seconds_observed: %w", err)
	}

	// qcc_circuit_usage_seconds is the substrate-reported billable
	// compute time for a Circuit (Qiskit Runtime `Job.usage()` on IBM
	// hardware).  Distinct from wall-clock duration: this measures
	// only on-QPU compute, not queue wait or controller transit.
	//
	// Emitted only when status.UsageSeconds > 0 — simulator paths and
	// substrates without a usage handle produce no series, so any
	// non-zero value in Prometheus reliably represents real-hardware
	// compute.  Pairs naturally with qcc_circuit_phase_duration_
	// seconds_observed{phase="Running"} for the "orchestration
	// overhead" ratio: usage_seconds / running_wall_clock.
	if inst.usageSeconds, err = meter.Float64ObservableGauge(
		"qcc_circuit_usage_seconds",
		metric.WithDescription(
			"Substrate-reported billable compute time for a Circuit "+
				"(Qiskit Runtime Job.usage() on IBM hardware).  Only "+
				"emitted when the substrate reports usage (real "+
				"hardware); simulator paths produce no series.  Pair "+
				"with qcc_circuit_phase_duration_seconds_observed"+
				"{phase=\"Running\"} for orchestration-overhead ratios.",
		),
		metric.WithUnit("s"),
	); err != nil {
		return inst, fmt.Errorf("declare qcc_circuit_usage_seconds: %w", err)
	}

	return inst, nil
}

// observeCircuits lists Circuits from the cache and emits one row per
// instrument per Circuit.  Skips per-Circuit observations that don't
// yet have post-transpile metrics (mode=draw, mode=select before
// transpile, or in-flight before the executor returned shape).
func observeCircuits(
	ctx context.Context,
	c client.Client,
	obs metric.Observer,
	inst circuitInstruments,
) error {
	var circuits qccv1alpha1.CircuitList
	if err := c.List(ctx, &circuits); err != nil {
		return fmt.Errorf("list Circuits for metric observation: %w", err)
	}

	for i := range circuits.Items {
		ci := &circuits.Items[i]
		base := circuitBaseAttrs(ci)

		// info — always emitted, carries identity for joins.  shots
		// and qpu live here as labels (set-once values; identity-
		// like) rather than as separate metrics.
		obs.ObserveInt64(inst.info, 1, metric.WithAttributes(circuitInfoAttrs(ci)...))

		// Transpile metrics — only meaningful once the executor has
		// returned post-transpile shape.  status.transpile is nil
		// until then; observing nil would emit zeros that look like
		// real data.  Skip the per-Circuit observation when nil.
		if tm := ci.Status.Transpile; tm != nil {
			obs.ObserveInt64(inst.transpileDepth, int64(tm.Depth),
				metric.WithAttributes(base...))
			obs.ObserveInt64(inst.transpileGates, int64(tm.TwoQubitGates),
				metric.WithAttributes(withKind(base, "two_qubit")...))
			obs.ObserveInt64(inst.transpileGates, int64(tm.TotalGates),
				metric.WithAttributes(withKind(base, "total")...))
			// single_qubit derived from total - two_qubit (we don't
			// store it directly on the CRD because the executor's
			// TranspileMetadata doesn't break it out; derivation is
			// honest enough for thesis-scope visualization).
			singleQ := int64(tm.TotalGates) - int64(tm.TwoQubitGates)
			if singleQ < 0 {
				singleQ = 0
			}
			obs.ObserveInt64(inst.transpileGates, singleQ,
				metric.WithAttributes(withKind(base, "single_qubit")...))
		}

		// result_count{bitstring} — one observation per measurement
		// outcome, value = how many shots landed on that bitstring.
		// Empty / nil results map cleanly: range over a nil map is a
		// no-op in Go.  Per-bitstring cardinality is bounded by
		// 2^qubits; safe for thesis-scale circuits, will need
		// re-evaluation at VQE scale (per-iteration Circuits add up).
		for bitstring, count := range ci.Status.Results {
			obs.ObserveInt64(inst.resultCount, count,
				metric.WithAttributes(withBitstring(base, bitstring)...))
		}

		// phase_duration_seconds_observed — derived from
		// status.conditions[].lastTransitionTime deltas.  Identity
		// labels include uid + provider_job_id so cross-boundary
		// joins work on this metric the same as on qcc_circuit_info.
		identityBase := append(base, //nolint:gocritic // local extension by design
			attribute.String("uid", string(ci.UID)),
			attribute.String("provider_job_id", ci.Status.ProviderJobID),
		)
		for phase, duration := range phaseDurationsFromConditions(ci) {
			obs.ObserveFloat64(inst.phaseDurationObsrv, duration,
				metric.WithAttributes(withPhase(identityBase, phase)...))
		}

		// usage_seconds — only emit when the substrate actually
		// reported it.  Aer/fake_* paths leave UsageSeconds at 0
		// and we skip emission so the metric reliably means
		// "real-hardware compute time" in Prometheus.
		if ci.Status.UsageSeconds > 0 {
			obs.ObserveFloat64(inst.usageSeconds, ci.Status.UsageSeconds,
				metric.WithAttributes(identityBase...))
		}
	}
	return nil
}

// phaseDurationsFromConditions returns the wall-clock duration spent
// in each lifecycle phase, derived from the LastTransitionTime
// deltas on `status.conditions`.  The four phases are mapped to
// condition boundaries:
//
//	Pending    : creationTimestamp -> first Accepted condition
//	Selecting  : Accepted          -> Selected
//	Submitting : Selected          -> Submitted (covers Transpiling
//	                                 because conditions don't have a
//	                                 dedicated "Transpiled" type)
//	Running    : Submitted         -> Completed | Failed
//
// Missing condition boundaries are simply skipped (no metric
// emission for that phase), so an in-flight Circuit emits only the
// durations that have actually completed.  Zero-duration cases
// (multiple conditions stamped in the same reconcile) are still
// emitted with value 0 — that's accurate, not noise.
func phaseDurationsFromConditions(ci *qccv1alpha1.Circuit) map[string]float64 {
	out := map[string]float64{}

	created := ci.CreationTimestamp.Time
	tAccepted := conditionTime(ci, qccv1alpha1.ConditionAccepted)
	tSelected := conditionTime(ci, qccv1alpha1.ConditionSelected)
	tSubmitted := conditionTime(ci, qccv1alpha1.ConditionSubmitted)
	tCompleted := conditionTime(ci, qccv1alpha1.ConditionCompleted)
	tFailed := conditionTime(ci, qccv1alpha1.ConditionFailed)

	// Terminal time = whichever of Completed/Failed fired (could be
	// neither, for an in-flight Circuit).
	var tTerminal time.Time
	switch {
	case !tCompleted.IsZero() && !tFailed.IsZero():
		if tCompleted.After(tFailed) {
			tTerminal = tCompleted
		} else {
			tTerminal = tFailed
		}
	case !tCompleted.IsZero():
		tTerminal = tCompleted
	case !tFailed.IsZero():
		tTerminal = tFailed
	}

	if !tAccepted.IsZero() && !created.IsZero() {
		out["Pending"] = nonNegativeSeconds(tAccepted.Sub(created))
	}
	if !tSelected.IsZero() && !tAccepted.IsZero() {
		out["Selecting"] = nonNegativeSeconds(tSelected.Sub(tAccepted))
	}
	if !tSubmitted.IsZero() && !tSelected.IsZero() {
		out["Submitting"] = nonNegativeSeconds(tSubmitted.Sub(tSelected))
	}
	if !tTerminal.IsZero() && !tSubmitted.IsZero() {
		out["Running"] = nonNegativeSeconds(tTerminal.Sub(tSubmitted))
	}

	return out
}

// conditionTime returns the LastTransitionTime for the named
// condition, or the zero time if absent (or not in True state).
// We require status=True so we don't pick up a stale failure-mode
// condition that the controller may have flipped off later.
func conditionTime(ci *qccv1alpha1.Circuit, conditionType string) time.Time {
	for i := range ci.Status.Conditions {
		c := &ci.Status.Conditions[i]
		if c.Type == conditionType && c.Status == metav1.ConditionTrue {
			return c.LastTransitionTime.Time
		}
	}
	return time.Time{}
}

// nonNegativeSeconds clamps a duration to seconds ≥ 0.  Negative
// deltas would only appear if condition timestamps were rewritten
// out of order — defensive guard.
func nonNegativeSeconds(d time.Duration) float64 {
	if d < 0 {
		return 0
	}
	return d.Seconds()
}

// circuitBaseAttrs returns the (circuit, namespace, qpu) label set
// used on the per-Circuit operational gauges.  The `qpu` value comes
// from status.selectedQPU once selection has happened; empty before
// then is fine — Prometheus drops empty-value series in queries that
// filter by qpu, which is the desired behavior.
func circuitBaseAttrs(c *qccv1alpha1.Circuit) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("circuit", c.Name),
		attribute.String("namespace", c.Namespace),
		attribute.String("qpu", c.Status.SelectedQPU),
	}
}

// circuitInfoAttrs returns the identity attribute set for
// qcc_circuit_info — superset of base + mode + source_format + uid +
// shots + provider_job_id + algorithm-grouping labels.  `shots` is
// the configured shot count from Spec; carrying it on info-as-label
// (rather than as a separate gauge) is a thesis-scope simplification
// because shots is set-once and never changes per Circuit.
//
// `provider_job_id` carries `status.providerJobId` — the external
// execution handle returned by the substrate (an IBM Runtime job id
// like `d8463bg0bvlc73d46tqg` on real hardware, or `aer-<uuid>` on
// simulator paths).  Naming it `provider_job_id` rather than
// `job_id` avoids collision with Prometheus's reserved `job` label
// (the scrape-target identifier).  Cardinality cost is zero — this
// label is 1-to-1 with the existing per-Circuit info series, not a
// new series multiplier.  Empty until the executor returns; once
// set, immutable for the lifetime of the resource.  Trace id is not
// plumbed through yet — will land when controller spans propagate.
//
// `algorithm` / `algorithm_version` / `experiment` / `run_index` are
// promoted from the user-authored `qcc.io/*` labels (see
// QCC-API.md §5.4).  Promotion is via an explicit allowlist — we
// don't blindly forward `metadata.labels` to metric labels because
// that's a cardinality landmine (any user-added label would become
// a new dimension).  All four are 1-to-1 with the Circuit and
// add no series multiplier.
//
// `source_sha256` is the controller-stamped truth-anchor for the
// source body — also 1-to-1, also cardinality-neutral, useful for
// "did v2 actually differ from v1" diagnostics.
//
// Numeric-as-label trade-off: filtering by shots requires regex
// (`{shots=~"1[0-9]{3}"}` for ≥1000), and arithmetic isn't possible
// on label values.  Acceptable because (a) thesis runs use a small
// set of distinct shot counts (~5 values), and (b) the "total
// shots per QPU" aggregation isn't a Ch7-load-bearing query.
func circuitInfoAttrs(c *qccv1alpha1.Circuit) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("circuit", c.Name),
		attribute.String("namespace", c.Namespace),
		attribute.String("uid", string(c.UID)),
		attribute.String("mode", string(c.Spec.Mode)),
		attribute.String("source_format", string(c.Spec.Source.Format)),
		attribute.String("shots", strconv.FormatInt(int64(c.Spec.Shots), 10)),
		attribute.String("qpu", c.Status.SelectedQPU),
		attribute.String("provider_job_id", c.Status.ProviderJobID),
		attribute.String("algorithm", c.Labels[qccv1alpha1.LabelAlgorithm]),
		attribute.String("algorithm_version", c.Labels[qccv1alpha1.LabelAlgorithmVersion]),
		attribute.String("experiment", c.Labels[qccv1alpha1.LabelExperiment]),
		attribute.String("run_index", c.Labels[qccv1alpha1.LabelRunIndex]),
		attribute.String("source_sha256", c.Labels[qccv1alpha1.LabelSourceSHA256]),
	}
}

// withKind clones base and appends a `kind` label.  Used by
// qcc_circuit_transpile_gates to distinguish single_qubit /
// two_qubit / total observations.
func withKind(base []attribute.KeyValue, kind string) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(base)+1)
	out = append(out, base...)
	out = append(out, attribute.String("kind", kind))
	return out
}

// withBitstring clones base and appends a `bitstring` label.  Used
// by qcc_circuit_result_count to label each measurement-outcome row.
func withBitstring(base []attribute.KeyValue, bitstring string) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(base)+1)
	out = append(out, base...)
	out = append(out, attribute.String("bitstring", bitstring))
	return out
}

// withPhase clones base and appends a `phase` label.  Used by
// qcc_circuit_phase_duration_seconds_observed.
func withPhase(base []attribute.KeyValue, phase string) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(base)+1)
	out = append(out, base...)
	out = append(out, attribute.String("phase", phase))
	return out
}
