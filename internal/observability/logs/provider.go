/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package logs is a placeholder for the future OTel logs bridge.
//
// Today, QCC logging works as follows:
//
//   - The controller's main.go installs a slog JSON handler writing
//     to stdout.
//   - controller-runtime's logr is bridged to slog via
//     `logr.FromSlogHandler(...)`, so every `log.FromContext(ctx)`
//     call (the controller-runtime idiom) goes through the same
//     handler.
//   - Logs are picked up by the kubelet → visible via
//     `kubectl logs deploy/qcc-controller-manager`.
//
// What's NOT here: an OTel logs exporter.  The OTel logs API
// (`go.opentelemetry.io/otel/log`) is pre-v1 in mid-2026 — adopting
// it now risks API breaks.  When it goes v1, the upgrade is a
// three-line change in `cmd/qcc-controller/main.go`:
//
//	loggerProvider := sdklog.NewLoggerProvider(...)
//	slog.SetDefault(otelslog.NewLogger("qcc-controller", otelslog.WithLoggerProvider(loggerProvider)))
//
// At that point logs flow through the same Collector → Loki (or
// wherever) — the same OTLP path metrics use today.  No application
// call-sites change.
//
// This file exists so the upgrade location is unambiguous when the
// time comes, not so it does anything yet.
package logs
