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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"sigs.k8s.io/controller-runtime/pkg/client"

	qccv1alpha1 "github.com/ioaiaaii/quantum-circuit-controller/api/v1alpha1"
)

// meterName identifies the meter / instrumentation scope that QPU
// metrics emit under.  OTel uses this to group instruments per
// package; it appears as an `otel_scope_name` attribute on the
// Prometheus side.
const meterName = "qcc.io/observability/metrics"

// qpuInstruments bundles all the QPU-side metric handles registered
// in one go.  Holding them in a struct keeps RegisterQPUMetrics
// readable as the inventory grows and avoids a long
// variadic list at the RegisterCallback site.
type qpuInstruments struct {
	info              metric.Int64ObservableGauge
	operationError    metric.Float64ObservableGauge
	operationDuration metric.Float64ObservableGauge
	coherence         metric.Float64ObservableGauge
	calibrationStamp  metric.Float64ObservableGauge
	condition         metric.Int64ObservableGauge
}

// RegisterQPUMetrics declares the QPU-side ObservableGauges and
// installs a single callback that observes them from the
// controller-runtime cache.
//
// Rationale for the single-callback / multi-instrument pattern (see
// `QCC-Observability.md` §4.4.1):
//
//   - Calling `cache.List(&qpus)` once per scrape, then observing all
//     per-QPU gauges in one pass, avoids repeated cache iteration.
//   - The callback fires on Prometheus scrape (~30s cadence in the
//     deployed stack), not on every reconcile.  This decouples scrape
//     cost from reconcile rate.
//   - The cache read path (`c.List(ctx, &list)` on a manager-built
//     client) returns from the in-memory informer — no apiserver
//     round-trip on the scrape path.  Critical: scrape paths must
//     never block on the apiserver.
//
// Inventory below matches the locked design in
// `QCC-Observability.md` §5.1.  Six metrics total; condition is the
// KSM-canonical Conditions matrix gauge (one row per
// `(qpu, condition, status)` triple).
func RegisterQPUMetrics(c client.Client) error {
	meter := otel.Meter(meterName)

	inst, err := declareQPUInstruments(meter)
	if err != nil {
		return err
	}

	_, err = meter.RegisterCallback(
		func(ctx context.Context, obs metric.Observer) error {
			return observeQPUs(ctx, c, obs, inst)
		},
		inst.info,
		inst.operationError,
		inst.operationDuration,
		inst.coherence,
		inst.calibrationStamp,
		inst.condition,
	)
	if err != nil {
		return fmt.Errorf("register QPU metrics callback: %w", err)
	}

	return nil
}

// declareQPUInstruments allocates the six QPU instruments with their
// descriptions and units.  Split from RegisterQPUMetrics so the
// inventory is readable in one block.
func declareQPUInstruments(meter metric.Meter) (qpuInstruments, error) {
	var inst qpuInstruments
	var err error

	if inst.info, err = meter.Int64ObservableGauge(
		"qcc_qpu_info",
		metric.WithDescription(
			"Static identity of a QPU resource (always 1; labels carry identity for PromQL joins).",
		),
	); err != nil {
		return inst, fmt.Errorf("declare qcc_qpu_info: %w", err)
	}

	if inst.operationError, err = meter.Float64ObservableGauge(
		"qcc_qpu_operation_error_median",
		metric.WithDescription(
			"Median error rate per quantum operation class (gate_1q, gate_2q, readout), in [0,1].",
		),
	); err != nil {
		return inst, fmt.Errorf("declare qcc_qpu_operation_error_median: %w", err)
	}

	if inst.operationDuration, err = meter.Float64ObservableGauge(
		"qcc_qpu_operation_duration_median_seconds",
		metric.WithDescription(
			"Median duration per quantum operation class (gate_1q, gate_2q), in seconds.",
		),
		metric.WithUnit("s"),
	); err != nil {
		return inst, fmt.Errorf("declare qcc_qpu_operation_duration_median_seconds: %w", err)
	}

	if inst.coherence, err = meter.Float64ObservableGauge(
		"qcc_qpu_coherence_seconds",
		metric.WithDescription(
			"Median qubit coherence times (t1, t2) in seconds.",
		),
		metric.WithUnit("s"),
	); err != nil {
		return inst, fmt.Errorf("declare qcc_qpu_coherence_seconds: %w", err)
	}

	if inst.calibrationStamp, err = meter.Float64ObservableGauge(
		"qcc_qpu_last_calibration_timestamp_seconds",
		metric.WithDescription(
			"Unix epoch seconds of the QPU's most recent calibration refresh.",
		),
		metric.WithUnit("s"),
	); err != nil {
		return inst, fmt.Errorf("declare qcc_qpu_last_calibration_timestamp_seconds: %w", err)
	}

	if inst.condition, err = meter.Int64ObservableGauge(
		"qcc_qpu_condition",
		metric.WithDescription(
			"KSM-style Conditions matrix: one row per (qpu, condition, status).  "+
				"Value is 1 when the named condition's status matches, 0 otherwise.",
		),
	); err != nil {
		return inst, fmt.Errorf("declare qcc_qpu_condition: %w", err)
	}

	return inst, nil
}

// observeQPUs is the body of the QPU metrics callback.  Split out
// from RegisterQPUMetrics so the instrument closures stay narrow and
// the observation logic is testable.
//
// The function reads all QPUs from the controller-runtime cache once
// per invocation and emits per-QPU observations for each instrument.
// Errors from the cache list are returned to the SDK, which logs
// them; we don't return them to a partial observation set.
func observeQPUs(
	ctx context.Context,
	c client.Client,
	obs metric.Observer,
	inst qpuInstruments,
) error {
	var qpus qccv1alpha1.QPUList
	if err := c.List(ctx, &qpus); err != nil {
		return fmt.Errorf("list QPUs for metric observation: %w", err)
	}

	for i := range qpus.Items {
		qpu := &qpus.Items[i]
		baseAttrs := []attribute.KeyValue{
			attribute.String("qpu", qpu.Name),
		}

		// info — identity carrier, value=1
		obs.ObserveInt64(inst.info, 1, metric.WithAttributes(qpuInfoAttrs(qpu)...))

		// operation_error_median{operation=gate_1q|gate_2q|readout}
		if em := qpu.Status.ErrorMedians; em != nil {
			obs.ObserveFloat64(inst.operationError, em.SingleQubit,
				metric.WithAttributes(withOp(baseAttrs, "gate_1q")...))
			obs.ObserveFloat64(inst.operationError, em.TwoQubit,
				metric.WithAttributes(withOp(baseAttrs, "gate_2q")...))
			obs.ObserveFloat64(inst.operationError, em.Readout,
				metric.WithAttributes(withOp(baseAttrs, "readout")...))
		}

		// operation_duration_median_seconds{operation=gate_1q|gate_2q}
		// Readout duration is NOT reported by IBM Target — the
		// operation label value is absent (not zero); see
		// QCC-Observability.md §5.1 row 3 + design-state §4 type
		// discipline notes.
		if dm := qpu.Status.InstructionDurationMedians; dm != nil {
			obs.ObserveFloat64(inst.operationDuration, dm.SingleQubitSeconds,
				metric.WithAttributes(withOp(baseAttrs, "gate_1q")...))
			obs.ObserveFloat64(inst.operationDuration, dm.TwoQubitSeconds,
				metric.WithAttributes(withOp(baseAttrs, "gate_2q")...))
		}

		// coherence_seconds{type=t1|t2}
		// CRD stores microseconds (the unit IBM publishes); convert
		// to seconds here so the metric name's `_seconds` suffix is
		// honest.  Dashboards format as µs for display.
		if cm := qpu.Status.CoherenceMedians; cm != nil {
			obs.ObserveFloat64(inst.coherence, cm.T1Micros/1e6,
				metric.WithAttributes(withType(baseAttrs, "t1")...))
			obs.ObserveFloat64(inst.coherence, cm.T2Micros/1e6,
				metric.WithAttributes(withType(baseAttrs, "t2")...))
		}

		// last_calibration_timestamp_seconds{qpu}
		if t := qpu.Status.LastCalibrationTime; t != nil && !t.IsZero() {
			obs.ObserveFloat64(inst.calibrationStamp, float64(t.Unix()),
				metric.WithAttributes(baseAttrs...))
		}

		// condition{condition, status} — emit (condition × status)
		// matrix per the KSM Conditions pattern; exactly one
		// `status` row is 1 per (qpu, condition) at any moment.
		for _, cond := range qpu.Status.Conditions {
			emitConditionRows(obs, inst.condition, baseAttrs, string(cond.Type), string(cond.Status))
		}
	}
	return nil
}

// withOp returns base attributes plus an `operation` label.  Allocates
// a new slice so callers don't mutate each other's attribute sets.
func withOp(base []attribute.KeyValue, op string) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(base)+1)
	out = append(out, base...)
	out = append(out, attribute.String("operation", op))
	return out
}

// withType returns base attributes plus a `type` label (for
// coherence's t1/t2).
func withType(base []attribute.KeyValue, t string) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(base)+1)
	out = append(out, base...)
	out = append(out, attribute.String("type", t))
	return out
}

// emitConditionRows emits the KSM-canonical 1-of-N pattern for one
// Condition: three rows (`status=true|false|unknown`) per condition
// type, exactly one of which has value 1.  Mirrors what
// kube-state-metrics does for `kube_pod_status_condition`.
func emitConditionRows(
	obs metric.Observer,
	inst metric.Int64ObservableGauge,
	base []attribute.KeyValue,
	conditionType, actualStatus string,
) {
	// Lowercase the actual status to match the KSM `status` label
	// vocabulary ("true"/"false"/"unknown").  metav1.ConditionStatus
	// is "True"/"False"/"Unknown" (title case).
	actual := lowerASCII(actualStatus)
	for _, candidate := range []string{"true", "false", "unknown"} {
		val := int64(0)
		if candidate == actual {
			val = 1
		}
		attrs := make([]attribute.KeyValue, 0, len(base)+2)
		attrs = append(attrs, base...)
		attrs = append(attrs,
			attribute.String("condition", conditionType),
			attribute.String("status", candidate),
		)
		obs.ObserveInt64(inst, val, metric.WithAttributes(attrs...))
	}
}

// lowerASCII is a tiny inlined ToLower for ASCII-only condition
// statuses ("True"/"False"/"Unknown" → "true"/"false"/"unknown").
// Avoids pulling in strings just for one call site; condition
// statuses are well-defined ASCII per K8s API.
func lowerASCII(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// qpuInfoAttrs builds the attribute set for one QPU's info row.
// Centralised so the attribute schema lives in one place (matters
// when we add more QPU metrics — they all share these identity
// attributes for PromQL joins).
//
// Labels emitted (per `QCC-Observability.md` §5.1):
//
//	qpu, uid, provider, kind, processor_family, processor_revision
//
// Missing optional values (processor metadata absent on generic Aer)
// emit empty strings rather than being dropped — this keeps the label
// set uniform across series so PromQL doesn't have to special-case.
func qpuInfoAttrs(qpu *qccv1alpha1.QPU) []attribute.KeyValue {
	family, revision := "", ""
	if qpu.Status.Processor != nil {
		family = qpu.Status.Processor.Family
		revision = qpu.Status.Processor.Revision
	}
	return []attribute.KeyValue{
		attribute.String("qpu", qpu.Name),
		attribute.String("uid", string(qpu.UID)),
		attribute.String("provider", qpu.Spec.Provider),
		attribute.String("kind", string(qpu.Spec.Kind)),
		attribute.String("processor_family", family),
		attribute.String("processor_revision", revision),
	}
}
