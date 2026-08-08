/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// CircuitMode names the verb the controller should apply to the circuit
// source.  Each mode drives a different abbreviated phase machine.
// +kubebuilder:validation:Enum=run;select;draw;schedule
type CircuitMode string

const (
	// ModeRun submits the circuit through the executor and waits for results.
	ModeRun CircuitMode = "run"
	// ModeSelect evaluates backend selection and stops before submission.
	ModeSelect CircuitMode = "select"
	// ModeDraw renders the circuit as ASCII via the executor's DrawCircuit RPC.
	ModeDraw CircuitMode = "draw"
	// ModeSchedule transpiles + schedules the circuit for the selected
	// backend and persists the per-instruction timeline (start times in
	// dt cycles, durations, qubits).  No shots consumed.  Used by
	// `qcc schedule <source>` / `qcc get circuit X --schedule`; gives
	// the reader the µs-scale envelope of the run without executing.
	ModeSchedule CircuitMode = "schedule"
)

// SourceFormat names how to interpret CircuitSource.body.
// +kubebuilder:validation:Enum=openqasm3;qiskit
type SourceFormat string

const (
	SourceOpenQASM3 SourceFormat = "openqasm3"
	SourceQiskit    SourceFormat = "qiskit"
)

// CircuitSource carries the circuit body in one of the supported formats.
// `qiskit` bodies are converted to OpenQASM 3 server-side by the executor
// before execution; the CLI does not import Qiskit.
type CircuitSource struct {
	// +kubebuilder:validation:Required
	Format SourceFormat `json:"format"`

	// body is the raw source text in the chosen format.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Body string `json:"body"`
}

// BackendKind discriminates QPU profile types.
// +kubebuilder:validation:Enum=hardware;simulator
type BackendKind string

const (
	BackendKindHardware  BackendKind = "hardware"
	BackendKindSimulator BackendKind = "simulator"
)

// CircuitPhase is the user-facing lifecycle phase. See QCC-API.md §5.1.
// +kubebuilder:validation:Enum=Pending;Selecting;Transpiling;Submitting;Running;Rendering;Scheduling;Succeeded;Failed
type CircuitPhase string

const (
	PhasePending     CircuitPhase = "Pending"
	PhaseSelecting   CircuitPhase = "Selecting"
	PhaseTranspiling CircuitPhase = "Transpiling"
	PhaseSubmitting  CircuitPhase = "Submitting"
	PhaseRunning     CircuitPhase = "Running"
	// PhaseRendering applies only to mode=draw; the controller is calling
	// the executor's DrawCircuit RPC.
	PhaseRendering CircuitPhase = "Rendering"
	// PhaseScheduling applies only to mode=schedule; the controller is
	// calling the executor's ScheduleCircuit RPC to produce the
	// per-instruction timeline artifact.  Distinct from Transpiling so
	// `kubectl get circuit` makes the mode-specific work visible.
	PhaseScheduling CircuitPhase = "Scheduling"
	PhaseSucceeded  CircuitPhase = "Succeeded"
	PhaseFailed     CircuitPhase = "Failed"
)

// Reserved Circuit labels.  All under the `qcc.io/` prefix so they
// don't collide with user-chosen labels.  Two groups:
//
//   - User-authored (set at submission time, immutable thereafter):
//     algorithm / algorithm-version / experiment.  Identify which
//     algorithm a Circuit is a run of, and optionally which
//     experiment campaign it belongs to.  Driven either by the
//     `qcc run --algorithm/--version/--experiment` flags or by
//     authoring them directly in `metadata.labels` of a Circuit YAML.
//
//   - Controller-authored (auto-filled on first reconcile):
//     run-index (set only when `algorithm` is present; ordinal among
//     siblings sharing the same algorithm + experiment) and
//     source-sha256 (always; SHA-256 of `spec.source.body`, the
//     truth-anchor for "did the source actually change?").
//
// See QCC-API.md §5.4 for the convention and PromQL examples.
const (
	LabelAlgorithm        = "qcc.io/algorithm"
	LabelAlgorithmVersion = "qcc.io/algorithm-version"
	LabelExperiment       = "qcc.io/experiment"
	LabelRunIndex         = "qcc.io/run-index"
	LabelSourceSHA256     = "qcc.io/source-sha256"
)

// Condition types reported on Circuit.status.conditions. See QCC-API.md §5.2.
const (
	ConditionAccepted  = "Accepted"
	ConditionValidated = "Validated"
	ConditionSelected  = "Selected"
	ConditionSubmitted = "Submitted"
	ConditionRendered  = "Rendered"
	// ConditionScheduled marks mode=schedule completion — the
	// per-instruction timeline artifact is in the ConfigMap pointed at
	// by status.scheduleRef.
	ConditionScheduled = "Scheduled"
	ConditionCompleted = "Completed"
	ConditionFailed    = "Failed"
)

// Condition reasons. See QCC-API.md §5.2.
const (
	ReasonInvalidCircuit           = "InvalidCircuit"
	ReasonNoEligibleBackend        = "NoEligibleBackend"
	ReasonBackendSelected          = "BackendSelected"
	ReasonTranspilationFailed      = "TranspilationFailed"
	ReasonProviderSubmissionFailed = "ProviderSubmissionFailed"
	ReasonProviderJobTimedOut      = "ProviderJobTimedOut"
	ReasonExecutionCompleted       = "ExecutionCompleted"
	ReasonSourceConversionFailed   = "SourceConversionFailed"
	ReasonDrawingRendered          = "DrawingRendered"
	ReasonRenderingFailed          = "RenderingFailed"
	ReasonScheduleProduced         = "ScheduleProduced"
	ReasonSchedulingFailed         = "SchedulingFailed"
	ReasonSchedulingUnsupported    = "SchedulingUnsupported"
)

// BackendSelector expresses constraints and preferences for QPU selection.
type BackendSelector struct {
	// provider filters candidate QPUs by spec.provider (e.g. "local",
	// "ibm"); see QCC-API.md §4.4 for the dispatch table.  Alternative
	// substrates (QRMI for Pasqal/multi-vendor, CUDA-Q for NVIDIA) are
	// Ch9 future-work — no adapter wired today.
	// +optional
	Provider string `json:"provider,omitempty"`

	// backendName names a specific backend.  Matches against either the
	// QPU's metadata.name (DNS-1123, kubectl-friendly, e.g. "fake-brisbane")
	// *or* its spec.backendName (provider-native, e.g. "fake_brisbane").
	// The dual match exists because Kubernetes names cannot contain
	// underscores but Qiskit identifiers often do, so users naturally
	// refer to QPUs by whichever form they last saw.
	// +optional
	BackendName string `json:"backendName,omitempty"`

	// +optional
	Kind BackendKind `json:"kind,omitempty"`

	// +optional
	// +kubebuilder:validation:Minimum=1
	MinQubits int32 `json:"minQubits,omitempty"`

	// +optional
	// +listType=set
	AllowedQPURefs []string `json:"allowedQPURefs,omitempty"`

	// +optional
	Region string `json:"region,omitempty"`
}

// CircuitSpec is the desired state of a Circuit.
type CircuitSpec struct {
	// source carries the circuit body in one of the supported formats.
	// +kubebuilder:validation:Required
	Source CircuitSource `json:"source"`

	// shots is the number of circuit executions.  Required for mode=run;
	// ignored for mode=draw and mode=select.
	// +optional
	// +kubebuilder:validation:Minimum=1
	Shots int32 `json:"shots,omitempty"`

	// mode names the verb to apply to source: run | select | draw.
	// +kubebuilder:validation:Required
	// +kubebuilder:default=run
	Mode CircuitMode `json:"mode"`

	// +optional
	BackendSelector *BackendSelector `json:"backendSelector,omitempty"`

	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=3
	OptimizationLevel *int32 `json:"optimizationLevel,omitempty"`

	// +optional
	// +kubebuilder:validation:Minimum=1
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// transpile and execute carry per-stage *passthrough* configuration
	// (Composition Principle Tier 2 — QCC-Design-State.md §7a).  Each is
	// an opaque dict whose keys/values flow through the controller and
	// executor untouched and land verbatim as keyword arguments on the
	// upstream Qiskit call site: `transpile` → qiskit.compiler.transpile(),
	// `execute` → the adapter's submit path (SamplerV2.run for IBM,
	// AerSimulator.run for Aer).
	//
	// Keys MUST use snake_case to match Qiskit's parameter names
	// directly (e.g. seed_transpiler, layout_method, routing_method for
	// transpile; seed_simulator, memory for execute).  No translation
	// happens — unknown keys propagate to Qiskit's own validation and
	// surface as Circuit FAILED with the adapter-supplied error_reason.
	//
	// Tier-1 fields (shots, optimizationLevel) take precedence; setting
	// them via passthrough is a configuration smell.  The CRD uses
	// `x-kubernetes-preserve-unknown-fields: true` (set via the
	// `+kubebuilder:pruning:PreserveUnknownFields` marker) so arbitrary
	// keys survive a kubectl apply round-trip.
	//
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Transpile *apiextensionsv1.JSON `json:"transpile,omitempty"`

	// execute — see Transpile above.  Forwarded to the adapter's submit
	// stage (e.g. SamplerV2.run / AerSimulator.run).  shots is a Tier-1
	// field and must NOT appear here.
	//
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Execute *apiextensionsv1.JSON `json:"execute,omitempty"`
}

// SelectionSummary is a compact explanation of the executor's selection decision.
type SelectionSummary struct {
	// +optional
	Candidates int32 `json:"candidates,omitempty"`

	// +optional
	Selected string `json:"selected,omitempty"`

	// +optional
	Score string `json:"score,omitempty"`
}

// ArtifactRef points at a ConfigMap that holds a generated artifact (ASCII
// drawing, converted OpenQASM 3, future per-backend transpiled circuits)
// produced during reconciliation.  Artifacts live in ConfigMaps rather than
// inline on the Circuit because etcd values are bounded (~1.5 MiB hard,
// ≤256 KiB recommended) and large outputs would otherwise corrupt or refuse
// writes on every reconcile.  Each ConfigMap is owned by the Circuit via
// `controllerutil.SetControllerReference`, so it is garbage-collected when
// the Circuit is deleted.
//
// See docs/systems-design/QCC-API.md §3.7 for the full convention
// (predictable naming, standard labels, same-namespace constraint).
type ArtifactRef struct {
	// name of the ConfigMap.  Always lives in the same namespace as the
	// Circuit.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// Artifact ConfigMap conventions (part of the public CRD contract, see
// QCC-API.md §3.7).  These constants are exported so the controller and
// CLI can agree on the wire shape without either side hard-coding strings.
//
//   - Suffix appears at the end of the ConfigMap's name
//     (`<circuit-name>-<suffix>`) and is also the value of the
//     `qcc.io/artifact` label.
//   - DataKey is the single key inside the ConfigMap's `data` map under
//     which the artifact payload lives.
const (
	ArtifactSuffixDrawing   = "drawing"
	ArtifactSuffixConverted = "converted"
	ArtifactSuffixSchedule  = "schedule"

	ArtifactDataKeyDrawing  = "drawing"       // data["drawing"] holds the ASCII text
	ArtifactDataKeyQASM     = "qasm"          // data["qasm"] holds the OpenQASM 3 text
	ArtifactDataKeySchedule = "schedule.json" // data["schedule.json"] holds the JSON timeline
)

// CircuitTranspileMetrics summarises the transpiled-circuit shape returned
// by the executor after Move 3 (transpile per backend) of the selection
// chain.  Populated by mode=run after submission; useful in `qcc get` for
// explaining the histogram (deeper transpile → more gate-error exposure)
// and for thesis-side reproducibility (Ch7 cites these alongside the
// QPU's error medians).
type CircuitTranspileMetrics struct {
	// depth is the longest path of dependent gates in the transpiled
	// circuit — the coherence-time exposure proxy.
	// +optional
	Depth uint32 `json:"depth,omitempty"`

	// twoQubitGates counts ECR / CZ / CX-equivalent two-qubit gates
	// after transpile.  The single most predictive number for total
	// circuit error on real hardware.
	// +optional
	TwoQubitGates uint32 `json:"twoQubitGates,omitempty"`

	// totalGates is the post-transpile gate count across all arities
	// (single + two-qubit + measurement + reset etc.).
	// +optional
	TotalGates uint32 `json:"totalGates,omitempty"`
}

// CircuitStatus is the observed state of a Circuit.
type CircuitStatus struct {
	// +optional
	Phase CircuitPhase `json:"phase,omitempty"`

	// +optional
	SelectedQPU string `json:"selectedQPU,omitempty"`

	// +optional
	ProviderJobID string `json:"providerJobId,omitempty"`

	// +optional
	TraceID string `json:"traceId,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	SelectionSummary *SelectionSummary `json:"selectionSummary,omitempty"`

	// results holds measurement counts when the Circuit is run inline (mode=run).
	// Keys are classical-bitstring outcomes; values are observed counts.
	// +optional
	Results map[string]int64 `json:"results,omitempty"`

	// usageSeconds is the substrate-reported billable compute time for
	// the Circuit's execution (Qiskit Runtime `Job.usage()` for IBM
	// hardware), in seconds.  Distinct from wall-clock duration:
	// usageSeconds measures only on-QPU compute, not queue wait or
	// transit.  Zero or omitted for simulator paths (Aer / fake_*) and
	// for runs where the substrate didn't expose a usage handle —
	// observability code emits the corresponding metric only when this
	// value is > 0.  See QCC-Observability.md §5.2.
	// +optional
	UsageSeconds float64 `json:"usageSeconds,omitempty"`

	// transpile carries the post-transpilation gate-shape metrics
	// reported by the executor (depth, two-qubit gate count, total
	// gate count).  Populated by mode=run; null for mode=draw and
	// mode=select (no submission, no transpile metrics).
	// +optional
	Transpile *CircuitTranspileMetrics `json:"transpile,omitempty"`

	// drawingRef points at a ConfigMap holding the ASCII rendering produced
	// for mode=draw, under data["drawing"].
	// +optional
	DrawingRef *ArtifactRef `json:"drawingRef,omitempty"`

	// scheduleRef points at a ConfigMap holding the JSON-encoded
	// per-instruction timeline for mode=schedule, under
	// data["schedule.json"].  Times are in dt cycles; the ConfigMap
	// also carries dt_seconds, total_duration_dt, num_qubits, and
	// backend_used so the renderer can convert without an additional
	// QPU lookup.
	// +optional
	ScheduleRef *ArtifactRef `json:"scheduleRef,omitempty"`

	// convertedRef points at a ConfigMap holding the OpenQASM 3 form of a
	// qiskit-format source, under data["qasm"].  Populated by mode=run with
	// source.format=qiskit as a byproduct of the controller calling
	// Executor.ConvertSource before submission.  Null for OpenQASM 3
	// sources (nothing to convert) and for modes that do not invoke
	// ConvertSource.
	// +optional
	ConvertedRef *ArtifactRef `json:"convertedRef,omitempty"`

	// Note: execution results live inline on .results above as a
	// counts map[string]int64.  No out-of-band ResultRef indirection
	// today — thesis-scale circuits produce results well below etcd's
	// inline-value limit.  See QCC-Design-State.md §7a (Composition
	// Principle, "What this principle is not") and the 2026-05-16
	// decision-log entry for the inline-vs-ref tradeoff rationale.
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="QPU",type=string,JSONPath=`.status.selectedQPU`
// +kubebuilder:printcolumn:name="Format",type=string,JSONPath=`.spec.source.format`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Circuit is the Schema for the circuits API.
type Circuit struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec CircuitSpec `json:"spec"`

	// +optional
	Status CircuitStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// CircuitList contains a list of Circuit.
type CircuitList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Circuit `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &Circuit{}, &CircuitList{})
		return nil
	})
}
