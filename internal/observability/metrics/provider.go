/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package metrics owns QCC's OTel MeterProvider and the per-family
// metric collectors (qpu.go, circuit.go, events.go).  The provider
// pushes OTLP-encoded metrics to the helm-deployed Collector in the
// `monitoring` namespace; the Collector translates them to Prometheus
// exposition on :8889 and is scraped by kube-prometheus-stack.
//
// See `docs/systems-design/QCC-Observability.md` §12 for the full
// architecture and §5 for the locked metric inventory.
package metrics

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

// NewMeterProvider constructs the OTel MeterProvider that QCC uses for
// all `qcc_*` metrics.  Pushes to the Collector via OTLP-gRPC on a
// periodic interval.
//
// The PeriodicReader's interval matters: the Collector translates OTLP
// metrics to a Prometheus exposition endpoint that Prometheus scrapes
// at its own cadence (typically 30s).  Pushing more often than the
// downstream scrape just wastes Collector cycles, so we keep them
// aligned.
//
// `insecureTLS` flag controls TLS via the OTLP exporter's canonical
// `WithInsecure()` option — true for in-cluster pod-to-pod traffic over
// the cluster network, false for crossing a mesh boundary or external
// endpoint.  When false, the exporter uses the system trust store via
// gRPC's default credentials.
func NewMeterProvider(
	ctx context.Context,
	res *resource.Resource,
	endpoint string,
	interval time.Duration,
	insecureTLS bool,
) (*sdkmetric.MeterProvider, error) {
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(endpoint),
	}
	if insecureTLS {
		// Canonical insecure path for the OTLP metric exporter.
		// Equivalent to gRPC's WithTransportCredentials(insecure...)
		// but goes through the exporter's option surface so the SDK
		// doesn't override our credentials.
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP metric exporter for %s: %w", endpoint, err)
	}

	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(exporter,
				sdkmetric.WithInterval(interval),
			),
		),
	), nil
}
