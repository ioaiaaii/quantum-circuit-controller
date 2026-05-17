/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package traces holds a TracerProvider skeleton.  Today the provider
// is configured with no SpanProcessor — so any spans started via
// `otel.Tracer(...)` are dropped silently — but the propagator is set
// globally (in `observability/otel.go`) and the provider exists as the
// global tracer source.  This means:
//
//   - When we flip tracing on (M3 follow-up or later), the change is
//     swapping in an OTLP span processor here — no call-site changes
//     anywhere else in the codebase.
//   - Exemplars on the existing histograms (qcc_circuit_phase_duration_seconds,
//     etc.) become automatic at that point — when a synchronous
//     instrument is recorded inside an active span, the OTel SDK
//     attaches the trace_id to the histogram observation.  This is
//     why the propagator is set today even though we don't emit:
//     the wiring is in place for the future flip.
//
// See `docs/systems-design/QCC-Observability.md` §14 for the rationale
// behind deferring trace emission.
package traces

import (
	"context"

	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// NewTracerProvider returns a TracerProvider that drops spans by
// design.  We set it as the global anyway so `otel.Tracer(...)` calls
// from elsewhere return a real (non-noop) tracer, and so attribute /
// span-name conventions can be authored today without waiting for a
// real exporter.
//
// When tracing flips on, add a span processor in this constructor:
//
//	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(...))
//	...
//	return sdktrace.NewTracerProvider(
//	    sdktrace.WithResource(res),
//	    sdktrace.WithBatcher(exp),
//	), nil
//
// That's the entire change — no callers need updating.
func NewTracerProvider(_ context.Context, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	return sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		// No span processor → spans are dropped.  Intentional today;
		// see package doc.
	), nil
}
