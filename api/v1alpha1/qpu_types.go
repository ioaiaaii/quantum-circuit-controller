/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package v1alpha1

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// QPUAvailability is the user-facing health state of a QPU candidate.
// Backend-selection in the Circuit reconciler filters on this field, so the
// vocabulary is deliberately small.  See QCC-System-Design.md §9 and §12.
// +kubebuilder:validation:Enum=Available;Unavailable;Unknown
type QPUAvailability string

const (
	// QPUAvailable means the executor can dispatch to this backend right
	// now.  For local (Aer) QPUs this is set on registration; for hardware
	// QPUs (M2) it is set by the QPU reconciler after a successful
	// calibration/queue probe.
	QPUAvailable QPUAvailability = "Available"
	// QPUUnavailable means the backend is reachable but rejecting jobs
	// (provider outage, queue closed, account quota, etc.).  Distinct from
	// Unknown because the reconciler has positive evidence.
	QPUUnavailable QPUAvailability = "Unavailable"
	// QPUUnknown means the reconciler has not yet probed (M1 default for
	// hardware QPUs) or the last probe failed in a way that doesn't prove
	// availability either way.  Selection treats Unknown as "not eligible
	// for now."
	QPUUnknown QPUAvailability = "Unknown"
)

// QPU condition types reported on QPU.status.conditions.
const (
	// ConditionMetadataFresh is True when the QPU reconciler has refreshed
	// calibration/queue metadata within the freshness budget (M2 wires the
	// freshness check; M1 leaves this condition unset for hardware QPUs
	// and True for local QPUs).
	ConditionMetadataFresh = "MetadataFresh"
	// ConditionReady is True when the QPU is registered and ready for
	// selection (i.e. status.availability == Available).
	ConditionReady = "Ready"
)

// QPU condition reasons.
const (
	// ReasonCalibrationRefreshed indicates the QPU reconciler successfully
	// refreshed calibration metadata from the provider.
	ReasonCalibrationRefreshed = "CalibrationRefreshed"
	// ReasonProviderProbeOK is set for local QPUs (no real provider) or
	// when a hardware QPU's reachability probe succeeded.
	ReasonProviderProbeOK = "ProviderProbeOK"
	// ReasonProviderProbeFailed signals a probe failure.  The accompanying
	// message and status.lastError carry the details.
	ReasonProviderProbeFailed = "ProviderProbeFailed"
)

// QPUAccess scopes the (small) credential surface attached to a QPU.  The
// Secret is dereferenced by the executor when an adapter that needs
// credentials is dispatched (today: IBMAdapter; future Ch9 substrates
// will reuse the same surface).  The controller treats the Secret as
// opaque — it never reads its contents.
type QPUAccess struct {
	// credentialSecretRef points at the Kubernetes Secret holding provider
	// credentials (e.g. IBM Quantum API token).  Optional and omitted for
	// provider=local (Aer needs no credentials).
	// +optional
	CredentialSecretRef *corev1.SecretReference `json:"credentialSecretRef,omitempty"`
}

// QPUCapabilities captures static or semi-static backend capabilities that
// affect selection eligibility but are not naturally expressed as integer
// hard constraints.  The shape is deliberately small; we add fields as new
// vendors expose new dimensions, rather than pre-paving an open-ended map.
//
// Per the Composition Principle (QCC-Design-State.md §7a), capabilities is
// the *declarative contract* surface — what a backend promises — distinct
// from the per-stage `options` blocks (imperative configuration forwarded
// to upstream functions).  Capabilities answers what's possible; options
// provides values.  Don't conflate.
type QPUCapabilities struct {
	// maxShots caps the per-job shot count the backend accepts.  Selection
	// rejects Circuits whose spec.shots exceeds this when set.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxShots *int32 `json:"maxShots,omitempty"`
}

// QPUSpec is the desired state of a QPU backend profile.
type QPUSpec struct {
	// provider names the adapter that handles this QPU on the executor
	// side (see QCC-API.md §4.4 for the full mapping):
	//   - "local"  → AerAdapter (in-process simulator, no credentials)
	//   - "ibm"    → IBMAdapter (qiskit-ibm-runtime; shipped M3)
	// Alternative substrates (QRMI, CUDA-Q) are Ch9 future-work; see
	// QCC-Design-State.md §7d (QEI direction).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Provider string `json:"provider"`

	// backendName is the provider-native identifier (e.g. "aer_simulator",
	// "ibm_sherbrooke") that the executor passes verbatim to the adapter.
	//
	// Optional: when omitted, the controller derives it from
	// metadata.name by replacing dashes with underscores.  This means the
	// common case where a QPU's K8s name and its vendor wire-name are the
	// same string in two character sets (e.g. K8s `fake-brisbane` ↔
	// Qiskit `fake_brisbane`) needs no manual mirroring.  Set this field
	// explicitly when the K8s name diverges from the vendor identifier —
	// for example multi-tenant deployments naming a resource
	// `prod-ibm-sherbrooke` that still targets `ibm_sherbrooke`.
	// Use `QPU.EffectiveBackendName()` to read the resolved value.
	// +optional
	BackendName string `json:"backendName,omitempty"`

	// kind discriminates real-hardware backends from simulators.  Used by
	// BackendSelector to filter and by observability to label spans.
	// +kubebuilder:validation:Required
	Kind BackendKind `json:"kind"`

	// qubits is the user-asserted qubit count, treated as a *hint* rather
	// than authoritative.  The QPUReconciler probes the backend (via the
	// Executor's ProbeBackend RPC) and stamps the observed value onto
	// status.qubits — selection reads status.qubits when present and
	// falls back to spec.qubits only when the probe hasn't run.  Use
	// QPU.EffectiveQubits() to read the resolved value.
	// +optional
	// +kubebuilder:validation:Minimum=1
	Qubits int32 `json:"qubits,omitempty"`

	// access carries the credential reference for adapters that need one.
	// Omit for provider=local.
	// +optional
	Access *QPUAccess `json:"access,omitempty"`

	// capabilities expresses static-ish backend properties that affect
	// selection eligibility beyond integer hard constraints.
	// +optional
	Capabilities *QPUCapabilities `json:"capabilities,omitempty"`

	// region is an optional locality / provider-region hint forwarded to
	// the adapter (e.g. for IBM Cloud regional routing).  Selection does
	// not filter on region in M1.
	// +optional
	Region string `json:"region,omitempty"`
}

// QPUStatus is the observed state of a QPU.
type QPUStatus struct {
	// availability is the rolling health verdict the selection chain
	// reads.  For local QPUs the QPU reconciler stamps Available on
	// registration; for hardware QPUs (M2) it transitions Unknown →
	// Available/Unavailable based on provider probes.
	// +optional
	Availability QPUAvailability `json:"availability,omitempty"`

	// qubits is the authoritative qubit count discovered by the
	// Executor's ProbeBackend RPC.  When present, the Circuit
	// reconciler's Move-1 filter reads this instead of spec.qubits.
	// Zero when the probe has not yet run; selection falls back to
	// spec.qubits then.
	// +optional
	// +kubebuilder:validation:Minimum=0
	Qubits int32 `json:"qubits,omitempty"`

	// basisGates is the native gate set the backend supports, as
	// reported by its Target.  Read by Move-3 (transpile per backend)
	// in M2; surfaced by `qcc get qpu` in M1.5b for visibility.
	// +optional
	// +listType=set
	BasisGates []string `json:"basisGates,omitempty"`

	// couplingEdges is the number of physical 2-qubit edges in the
	// backend's coupling map.  Zero means all-to-all (typical for
	// generic Aer); a positive value means the backend has explicit
	// topology constraints that Move-4 (layout) will respect.
	// +optional
	// +kubebuilder:validation:Minimum=0
	CouplingEdges int32 `json:"couplingEdges,omitempty"`

	// lastCalibrationTime is the wall-clock instant at which the most
	// recent calibration snapshot was taken from the provider.  For
	// fake_* simulators this is the frozen snapshot date (typically
	// months old); for live hardware it's the most recent vendor refresh.
	// Empty for plain Aer (no calibration concept).  Used by Move 5
	// (scoring) as the freshness weight.
	// +optional
	LastCalibrationTime *metav1.Time `json:"lastCalibrationTime,omitempty"`

	// errorMedians summarises gate/readout error rates from the
	// backend's Target.  Each value is a population median in [0, 1];
	// zero means "not reported by this backend" — scoring (M2) treats
	// absence as skip, never as perfect.
	// +optional
	ErrorMedians *QPUErrorMedians `json:"errorMedians,omitempty"`

	// coherenceMedians carries median T1 (energy-relaxation) and T2
	// (dephasing) lifetimes in microseconds.  Sets the coherence
	// budget the executor's transpilation must live within and feeds
	// Move 5's freshness/quality weight in M2 (T1 / gate_duration =
	// circuits-worth-of-time).  Zero values mean the backend doesn't
	// report coherence data (generic Aer).
	// +optional
	CoherenceMedians *QPUCoherenceMedians `json:"coherenceMedians,omitempty"`

	// dtSeconds is the backend's control-electronics cycle period —
	// the smallest time quantum the AWG can address.  All gate
	// durations are integer multiples of dt.  Typical IBM: ~2.22e-10 s.
	// Zero when the backend doesn't expose dt (generic Aer).  Paired
	// with instructionDurationMedians for circuit-time estimation.
	// +optional
	DtSeconds float64 `json:"dtSeconds,omitempty"`

	// instructionDurationMedians carries median per-instruction
	// durations in seconds (single-qubit gates excluding measure;
	// two-qubit gates).  Together with circuit depth they estimate
	// total execution time — depth × max(1Q, 2Q duration) is a
	// critical-path lower bound that Move 5 will compare to T1/T2
	// for coherence-budget scoring in M2.
	// +optional
	InstructionDurationMedians *QPUInstructionDurationMedians `json:"instructionDurationMedians,omitempty"`

	// processor identifies the chip generation behind the backend
	// (e.g. {Family: "Eagle", Revision: "3"} for fake_brisbane).
	// Populated from Qiskit's backend.processor_type at probe time.
	// Nil when the backend has no processor_type metadata (generic
	// Aer).  Surfaced so the CLI can label runs by hardware family
	// and the thesis can cite Eagle/Heron/Falcon from system data.
	// +optional
	Processor *QPUProcessor `json:"processor,omitempty"`

	// queueDepth is the provider-reported number of jobs ahead in the
	// backend's queue, when available.  Populated by M2 probes.
	// +optional
	// +kubebuilder:validation:Minimum=0
	QueueDepth *int32 `json:"queueDepth,omitempty"`

	// observedGeneration tracks reconciliation correctness.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions communicates fine-grained state.  Standard list-map
	// shape so kubectl describe renders them well.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// lastError carries the most recent reconciliation/probe failure for
	// debugging.  Populated by M2 hardware probes; M1 leaves it nil.
	// +optional
	LastError *QPULastError `json:"lastError,omitempty"`
}

// QPULastError records a recent failure with enough detail for the user to
// act, without forcing them to grovel through controller logs.
type QPULastError struct {
	// time the error was last observed.
	Time metav1.Time `json:"time"`
	// reason is a short machine-readable token (see Reason* constants).
	Reason string `json:"reason"`
	// message is a human-readable explanation.
	Message string `json:"message"`
}

// QPUErrorMedians carries population medians for gate/readout errors
// reported by the backend's Target.  Stored as +-resource.Quantity-style
// fixed-precision strings would be more etcd-correct, but for thesis-scale
// observability raw float64 is plenty and renders cleanly in `qcc get qpu`.
type QPUErrorMedians struct {
	// singleQubit is the median single-qubit gate error rate (excluding
	// measure), in [0, 1].
	SingleQubit float64 `json:"singleQubit"`
	// twoQubit is the median two-qubit gate error rate, in [0, 1].
	TwoQubit float64 `json:"twoQubit"`
	// readout is the median readout (measure-instruction) error, in [0, 1].
	Readout float64 `json:"readout"`
}

// QPUInstructionDurationMedians carries per-instruction duration
// medians in seconds.  The 1Q value typically lands in the tens of
// nanoseconds; the 2Q value in the hundreds (ECR/CZ ~600 ns on Eagle/
// Heron).  Zero values mean the backend Target lacks duration data.
type QPUInstructionDurationMedians struct {
	// singleQubitSeconds is the median duration of single-qubit gates
	// (excluding measurement).  In seconds.
	SingleQubitSeconds float64 `json:"singleQubitSeconds"`
	// twoQubitSeconds is the median duration of two-qubit gates.
	// In seconds.  Typically the largest gate-duration contributor.
	TwoQubitSeconds float64 `json:"twoQubitSeconds"`
}

// QPUProcessor identifies the chip generation behind a backend.
// Populated from Qiskit's backend.processor_type, which exposes a
// {family, revision, segment?} dict — e.g. {"Eagle", 3}, {"Heron", 1},
// {"Falcon", 4, "T"}.  All three are stored as strings (revision in
// particular arrives as a mix of int and string across families and
// providers; normalising at the wire boundary lets the CRD type stay
// simple).  Used by the CLI for hardware-family labelling and by M2
// selection to prefer one family over another.
type QPUProcessor struct {
	// family names the chip generation, e.g. "Eagle", "Heron",
	// "Falcon".  Always present when the parent QPUProcessor is set.
	Family string `json:"family"`
	// revision is the within-family iteration as a string, e.g. "3"
	// for Eagle r3.  Empty when the backend reports no revision.
	// +optional
	Revision string `json:"revision,omitempty"`
	// segment is a sub-divider used by some families (Falcon's "T"
	// segment, for example).  Empty when not applicable.
	// +optional
	Segment string `json:"segment,omitempty"`
}

// QPUCoherenceMedians carries median qubit-coherence times reported by
// the backend's Target.  Values are in microseconds — the unit IBM
// publishes and that keeps `qcc qpu` rendering compact ("230 µs" vs
// "0.00023 s").  Zero means the backend reports no coherence data.
type QPUCoherenceMedians struct {
	// t1Micros is the median T1 (energy-relaxation) lifetime across
	// qubits, in microseconds.  Upper bound on how long a circuit can
	// run before the |1⟩ population decays to |0⟩.
	T1Micros float64 `json:"t1Micros"`
	// t2Micros is the median T2 (dephasing) lifetime across qubits,
	// in microseconds.  Always ≤ T1 by physics.
	T2Micros float64 `json:"t2Micros"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.provider`
// +kubebuilder:printcolumn:name="Kind",type=string,JSONPath=`.spec.kind`
// +kubebuilder:printcolumn:name="Qubits",type=integer,JSONPath=`.status.qubits`
// +kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.status.availability`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// QPU is the Schema for the qpus API — a declarative backend profile that
// the controller's selection chain treats as a candidate.  Cluster-scoped
// because QPUs are infrastructure: one QPU resource per registered
// backend, shared across all namespaces that submit Circuits.  See
// QCC-API.md §4 for the contract and QCC-System-Design.md §6 for the
// execution-architecture context.
type QPU struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec QPUSpec `json:"spec"`

	// +optional
	Status QPUStatus `json:"status,omitzero"`
}

// EffectiveBackendName returns the provider-native backend identifier the
// executor should pass to its adapter — either the explicit
// `spec.backendName` when set, or the metadata.name with dashes converted
// to underscores otherwise.  Every controller-side read of "the backend
// name" should go through this method rather than touching
// `Spec.BackendName` directly, so the optional+derived contract is
// consistently enforced.
//
// The dash→underscore rule mirrors the common case where a QPU's K8s
// name and its vendor wire-name differ only in character set (K8s is
// DNS-1123 so no underscores allowed; Qiskit identifiers usually have
// them).  See QCC-API.md §4.3.
func (q *QPU) EffectiveBackendName() string {
	if q.Spec.BackendName != "" {
		return q.Spec.BackendName
	}
	return strings.ReplaceAll(q.Name, "-", "_")
}

// EffectiveQubits returns the authoritative qubit count for the QPU.
// Prefers `status.qubits` (probed from the backend's Target) over
// `spec.qubits` (user hint) — the Circuit reconciler's Move-1 filter
// must use this so registered-but-not-yet-probed QPUs degrade gracefully
// (they fall back to the user-supplied hint) and probed QPUs ignore a
// stale or wrong user value.
//
// Returns 0 when neither status nor spec carry a value; selection treats
// 0 as "unknown, skip the qubit-count constraint" rather than blocking.
func (q *QPU) EffectiveQubits() int32 {
	if q.Status.Qubits > 0 {
		return q.Status.Qubits
	}
	return q.Spec.Qubits
}

// +kubebuilder:object:root=true

// QPUList contains a list of QPU.
type QPUList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []QPU `json:"items"`
}

func init() {
	SchemeBuilder.Register(&QPU{}, &QPUList{})
}
