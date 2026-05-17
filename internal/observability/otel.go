/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package observability

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/ioaiaaii/quantum-circuit-controller/internal/observability/metrics"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/observability/traces"
)

// Setup initialises QCC's OTel SDK: resource, propagator, meter
// provider, tracer provider (skeleton).  Returns a single shutdown
// closure that the caller defers in `main` with a bounded timeout —
// this mirrors the pattern from ioaiaaii.net's
// `SetupOTelSDK(ctx, cfg)`.
//
// The orchestrator owns provider construction order:
//
//  1. Resource — built once, shared across all providers so every
//     signal carries the same identity.
//  2. Propagator — set globally before any provider that might consume
//     trace context.  Today the propagator is configured even though
//     no spans are emitted, so when tracing flips on the propagation
//     layer is already in place.
//  3. Meter provider — pushes OTLP metrics to the Collector.
//  4. Tracer provider — skeleton with no-op exporter; ready for the
//     future "flip the exporter" upgrade.
//
// When cfg.Enabled is false, Setup short-circuits to a no-op shutdown
// closure.  Useful for envtest runs that don't have a Collector and
// don't want stdout polluted with export errors.
//
// Errors from any provider init bubble up; the caller in main.go
// should treat them as fatal.
func Setup(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error) {
	if !cfg.Enabled {
		return noopShutdown, nil
	}

	// Compose multiple Shutdown funcs in reverse order of initialisation
	// (LIFO — flush exporters before tearing down providers).  Each
	// provider's Shutdown drains its in-flight exports up to the
	// context's deadline.
	var shutdownFns []func(context.Context) error
	shutdown = func(ctx context.Context) error {
		var errs []error
		// Iterate in reverse so flush ordering mirrors construction.
		for i := len(shutdownFns) - 1; i >= 0; i-- {
			if e := shutdownFns[i](ctx); e != nil {
				errs = append(errs, e)
			}
		}
		shutdownFns = nil
		return errors.Join(errs...)
	}

	// 1. Resource — service identity + K8s downward-API attributes.
	res, err := buildResource(ctx, cfg)
	if err != nil {
		return shutdown, fmt.Errorf("build OTel resource: %w", err)
	}

	// 2. Propagator — set globally even though we don't emit traces
	// yet.  Composite of W3C TraceContext + Baggage so future tracing
	// (and any incoming traces from clients that already propagate)
	// works without re-wiring.  Cost: zero runtime overhead when no
	// spans exist.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// 3. Meter provider — OTLP-gRPC push to the Collector.  Sets
	// itself as the global so `otel.Meter(...)` calls anywhere in
	// the codebase pick it up.
	mp, err := metrics.NewMeterProvider(ctx, res, cfg.OTLPEndpoint, cfg.MetricsInterval, cfg.OTLPInsecure)
	if err != nil {
		return shutdown, fmt.Errorf("init OTel meter provider: %w", err)
	}
	otel.SetMeterProvider(mp)
	shutdownFns = append(shutdownFns, mp.Shutdown)

	// 4. Tracer provider — skeleton with no-op exporter.  Set
	// globally so `otel.Tracer(...)` returns the real provider (not
	// the SDK's default noop).  When we wire a real exporter later,
	// existing tracer.Start(ctx, ...) calls light up automatically.
	tp, err := traces.NewTracerProvider(ctx, res)
	if err != nil {
		return shutdown, fmt.Errorf("init OTel tracer provider: %w", err)
	}
	otel.SetTracerProvider(tp)
	shutdownFns = append(shutdownFns, tp.Shutdown)

	return shutdown, nil
}

// noopShutdown is returned by Setup when cfg.Enabled is false.
// Provides a callable for the caller's deferred shutdown without
// requiring nil-checks.
func noopShutdown(context.Context) error { return nil }
