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
	"os"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// buildResource composes the OTel Resource that every emitted signal
// (metric, log, future trace) carries.  Resource attributes identify
// *where* a signal came from (which controller pod, on which node, in
// which namespace) — distinct from instrument attributes which describe
// *what* the signal is about.
//
// The resource is built from three sources, in order of precedence:
//
//  1. Explicit Config-derived attributes (service.name, service.version)
//     — these come from the Deployment's env block, sourced from values
//     files or the build's -ldflags.
//  2. Downward-API K8s attributes (k8s.pod.name, k8s.pod.uid,
//     k8s.namespace.name, k8s.node.name) — populated by valueFrom:
//     fieldRef: blocks in config/manager/manager.yaml.
//  3. The SDK's resource.WithFromEnv() reads OTEL_RESOURCE_ATTRIBUTES
//     and resource.WithProcess() reads runtime/process info (pid,
//     executable, runtime version) — both standard OTel defaults.
//
// Why all three: a Grafana operator looking at a metric needs to know
// service identity (which controller), pod identity (which replica, on
// which node, in which namespace), and process identity (which version
// of which binary).  Without these, the same metric across pods is
// indistinguishable.
//
// See `QCC-Observability.md` §12.6 for the corresponding Deployment
// manifest env block.
func buildResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
	}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.ServiceVersion))
	}

	// K8s downward-API attributes from env vars set on the controller
	// Pod.  These map onto OTel semantic-conventions K8s namespace.
	// We use os.Getenv (not the SDK's resource.WithFromEnv) because
	// the SDK reader expects OTEL_RESOURCE_ATTRIBUTES as a single
	// comma-separated string; the downward-API more naturally gives
	// us individual env vars.
	if v := os.Getenv("K8S_POD_NAME"); v != "" {
		attrs = append(attrs, semconv.K8SPodName(v))
	}
	if v := os.Getenv("K8S_POD_UID"); v != "" {
		attrs = append(attrs, semconv.K8SPodUID(v))
	}
	if v := os.Getenv("K8S_NAMESPACE_NAME"); v != "" {
		attrs = append(attrs, semconv.K8SNamespaceName(v))
	}
	if v := os.Getenv("K8S_NODE_NAME"); v != "" {
		attrs = append(attrs, semconv.K8SNodeName(v))
	}

	return resource.New(ctx,
		// SchemaURL: lets backends correlate our attribute names with
		// the canonical semantic-conventions schema.  Required when
		// you mix WithAttributes and other detectors.
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(attrs...),
		// Process info: pid, executable name, runtime version.  Free
		// observability for "which binary is emitting this signal".
		resource.WithProcess(),
		// FromEnv: honours OTEL_RESOURCE_ATTRIBUTES if the operator
		// chooses to set additional attributes that way.  Last so it
		// can override defaults if needed.
		resource.WithFromEnv(),
	)
}
