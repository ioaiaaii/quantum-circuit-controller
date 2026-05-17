/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package observability owns QCC's OpenTelemetry SDK setup, metric/tracer
// provider lifecycle, and resource attribution.  The package layout mirrors
// the pattern from ioaiaaii.net (single orchestrator + per-signal
// subpackages), adapted for the K8s controller context where:
//
//   - Metrics ship via OTLP-gRPC push to the in-cluster OpenTelemetry
//     Collector (Helm-deployed in the `monitoring` namespace; see
//     `deploy/platform/otelcol-values.yaml`).
//   - The Collector translates OTLP metrics to Prometheus exposition on
//     :8889, scraped by kube-prometheus-stack's Prometheus.
//   - Traces share the same Collector endpoint; today the QCC tracer
//     provider is a skeleton with a no-op exporter (`traces/provider.go`)
//     so future tracing is a config flip, not a code change.
//   - Logs stay on slog → stdout for now; controller-runtime's `logr` is
//     bridged to slog via `logr.FromSlogHandler` in
//     `cmd/qcc-controller/main.go`.  When `go.opentelemetry.io/otel/log`
//     reaches v1, an `otelslog` bridge slots into `logs/provider.go`.
//
// See `docs/systems-design/QCC-Observability.md` for the architectural
// rationale (§3 stack overview, §4 idiomatic principles, §12 wiring).
package observability

import (
	"os"
	"time"
)

// Default values mirror the in-cluster service DNS for the
// helm-deployed OTel Collector.  They're overridable via env vars so
// the controller can target a different collector (or be disabled
// entirely) without a rebuild.
const (
	// DefaultOTLPEndpoint targets the helm-deployed Collector in the
	// `monitoring` namespace.  The Collector's OTLP-gRPC receiver
	// listens on port 4317.  Override via OTEL_EXPORTER_OTLP_ENDPOINT.
	DefaultOTLPEndpoint = "otelcol-opentelemetry-collector.monitoring.svc.cluster.local:4317"

	// DefaultServiceName is the OTel resource attribute that
	// distinguishes QCC's metrics/traces from anything else flowing
	// through the same Collector.  Override via OTEL_SERVICE_NAME.
	DefaultServiceName = "qcc-controller"

	// DefaultMetricsInterval is how often the SDK's PeriodicReader
	// pushes accumulated metrics to the Collector.  30s mirrors
	// Prometheus's typical scrape interval; tighter pushes don't
	// produce more resolution because Prometheus then scrapes the
	// Collector at 30s anyway.
	DefaultMetricsInterval = 30 * time.Second

	// DefaultShutdownTimeout caps how long Setup's returned shutdown
	// closure will wait for in-flight exports to drain.  Bounded so
	// main()'s `defer` can't hang on a wedged Collector.
	DefaultShutdownTimeout = 10 * time.Second
)

// Config controls the OTel SDK lifecycle.  Zero-value Config is usable —
// it loads from env with sensible defaults — but callers typically
// override Enabled in tests to skip the SDK entirely (no-op shutdown,
// no exporter dial-attempts).
type Config struct {
	// Enabled gates all OTel SDK setup.  When false, Setup() returns
	// a no-op shutdown closure and never dials the Collector — useful
	// for `go test` / `envtest` runs that don't have a Collector and
	// don't want their stdout polluted with export errors.
	Enabled bool

	// ServiceName / ServiceVersion populate the OTel resource's
	// service.name and service.version attributes.  ServiceName has
	// a default (qcc-controller); ServiceVersion is empty unless
	// explicitly set, typically via a -ldflags build-time value.
	ServiceName    string
	ServiceVersion string

	// OTLPEndpoint is the gRPC target the OTel exporters dial.
	// `host:port` (no scheme).  Defaults to the in-cluster Collector
	// service DNS.
	OTLPEndpoint string

	// OTLPInsecure controls whether the gRPC exporter uses TLS.
	// True (insecure) is correct for in-cluster Collector traffic
	// over the pod network; false would be appropriate when crossing
	// a mesh boundary or external endpoint.
	OTLPInsecure bool

	// MetricsInterval is the PeriodicReader push cadence to the
	// Collector.  See DefaultMetricsInterval for the rationale.
	MetricsInterval time.Duration

	// ShutdownTimeout caps the deferred shutdown closure's wait for
	// in-flight exports to drain.
	ShutdownTimeout time.Duration
}

// ConfigFromEnv constructs a Config from the standard OTEL_* environment
// variables that the K8s Deployment manifest sets via downward-API and
// values blocks.  Callers can override the result before passing it to
// Setup.
//
// Recognised env vars:
//
//	OTEL_SDK_DISABLED        = "true" turns Enabled off (no SDK setup at all)
//	OTEL_EXPORTER_OTLP_ENDPOINT = host:port for the Collector
//	OTEL_SERVICE_NAME        = service.name resource attribute
//	OTEL_SERVICE_VERSION     = service.version resource attribute
//
// Other OTEL_* env vars (OTEL_RESOURCE_ATTRIBUTES,
// OTEL_EXPORTER_OTLP_INSECURE, …) are recognised by the SDK itself via
// `resource.WithFromEnv()` and the exporter constructors — we don't
// re-read them here.
func ConfigFromEnv() Config {
	cfg := Config{
		Enabled:         os.Getenv("OTEL_SDK_DISABLED") != "true",
		ServiceName:     envOr("OTEL_SERVICE_NAME", DefaultServiceName),
		ServiceVersion:  os.Getenv("OTEL_SERVICE_VERSION"),
		OTLPEndpoint:    envOr("OTEL_EXPORTER_OTLP_ENDPOINT", DefaultOTLPEndpoint),
		OTLPInsecure:    true, // in-cluster default; override-via-env handled by SDK natively
		MetricsInterval: DefaultMetricsInterval,
		ShutdownTimeout: DefaultShutdownTimeout,
	}
	return cfg
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
