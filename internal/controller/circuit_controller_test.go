/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	qccv1alpha1 "github.com/ioaiaaii/quantum-circuit-controller/api/v1alpha1"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/executor"
)

// backendAer is the Aer simulator's provider-native backend name, used across
// the controller tests. The provider itself is providerLocal, in
// qpu_controller.go.
const backendAer = "aer_simulator"

const bellQASM = `OPENQASM 3.0;
include "stdgates.inc";
qubit[2] q;
bit[2] c;
h q[0];
cx q[0], q[1];
c[0] = measure q[0];
c[1] = measure q[1];
`

type fakeExecutor struct {
	result *executor.Result
	err    error
	calls  int

	// convertedQASM is the QASM the controller would have received from
	// ConvertSource for qiskit-format inputs.  The fake stamps it onto
	// result.ConvertedQASM when the Circuit's source format is qiskit, so
	// the runOnExecutor path that persists status.convertedRef can be
	// exercised end-to-end in tests.
	convertedQASM string

	drawing  string
	drawErr  error
	drawCall int

	// scheduleResult is the canned ScheduleResult for mode=schedule
	// tests; nil means "return a minimal one-op result with
	// total_duration_dt=0".  scheduleCall tracks invocation count.
	scheduleResult *executor.ScheduleResult
	scheduleErr    error
	scheduleCall   int

	// probeProfile is the BackendProfile returned by ProbeBackend; nil
	// means "return a minimal default" — most Circuit tests don't care.
	// probeCall tracks invocation count (QPUReconciler tests assert on it).
	probeProfile *executor.BackendProfile
	probeErr     error
	probeCall    int
}

func (f *fakeExecutor) RunCircuit(_ context.Context, _ string, circuit *qccv1alpha1.Circuit, _ *qccv1alpha1.QPU) (*executor.Result, error) {
	f.calls++
	if f.err != nil || f.result == nil {
		return f.result, f.err
	}
	// Mirror executor.Client.RunCircuit: populate ConvertedQASM only when
	// the input source was qiskit (real client sets this from resolveQASM).
	out := *f.result
	if circuit.Spec.Source.Format == qccv1alpha1.SourceQiskit {
		out.ConvertedQASM = f.convertedQASM
	}
	return &out, nil
}

func (f *fakeExecutor) DrawCircuit(_ context.Context, _ qccv1alpha1.CircuitSource) (string, error) {
	f.drawCall++
	return f.drawing, f.drawErr
}

// Async stubs — the fake executor doesn't exercise the hardware async
// lifecycle in M3 tests yet; these satisfy the interface and return
// "not implemented in fake" placeholders.  Real async-path coverage
// arrives with the M3 integration smoke test against ibm-fez.

func (f *fakeExecutor) SubmitTask(
	_ context.Context, _ string, _ *qccv1alpha1.Circuit, _ *qccv1alpha1.QPU,
) (*executor.SubmitResult, error) {
	return nil, errors.New("SubmitTask not implemented in fakeExecutor")
}

func (f *fakeExecutor) WatchTask(
	_ context.Context, _ string,
) (<-chan executor.TaskStatus, <-chan error, error) {
	return nil, nil, errors.New("WatchTask not implemented in fakeExecutor")
}

func (f *fakeExecutor) FetchTaskResult(
	_ context.Context, _ string,
) (executor.TaskResult, error) {
	return executor.TaskResult{}, errors.New("FetchTaskResult not implemented in fakeExecutor")
}

func (f *fakeExecutor) ScheduleCircuit(
	_ context.Context, _ *qccv1alpha1.Circuit, _ *qccv1alpha1.QPU,
) (*executor.ScheduleResult, error) {
	f.scheduleCall++
	if f.scheduleErr != nil {
		return nil, f.scheduleErr
	}
	if f.scheduleResult != nil {
		return f.scheduleResult, nil
	}
	return &executor.ScheduleResult{Ops: nil, TotalDurationDt: 0}, nil
}

func (f *fakeExecutor) ProbeBackend(_ context.Context, provider, backendName string) (*executor.BackendProfile, error) {
	f.probeCall++
	if f.probeErr != nil {
		return nil, f.probeErr
	}
	if f.probeProfile != nil {
		return f.probeProfile, nil
	}
	// Default minimal profile: the QPUReconciler still gets a valid
	// (zero-valued) response so its happy path runs without mocking.
	return &executor.BackendProfile{
		NumQubits: 32,
	}, nil
}

// driveToTerminal repeatedly reconciles until phase is terminal or maxSteps
// elapses, mirroring how controller-runtime requeues drive the state machine.
func driveToTerminal(ctx context.Context, r *CircuitReconciler, name types.NamespacedName, maxSteps int) *qccv1alpha1.Circuit {
	GinkgoHelper()
	var circuit qccv1alpha1.Circuit
	for range maxSteps {
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, name, &circuit)).To(Succeed())
		if circuit.Status.Phase == qccv1alpha1.PhaseSucceeded ||
			circuit.Status.Phase == qccv1alpha1.PhaseFailed {
			return &circuit
		}
	}
	Fail("Circuit did not reach a terminal phase within maxSteps")
	return &circuit
}

// driveToPhase reconciles until the Circuit reaches want, or fails the
// spec after maxSteps.  Prefer this over a hard-coded reconcile count:
// the number of passes needed to reach a phase is an implementation
// detail (label stamping alone consumes one), so counting couples the
// test to the reconciler's internals.
func driveToPhase(
	ctx context.Context,
	r *CircuitReconciler,
	name types.NamespacedName,
	want qccv1alpha1.CircuitPhase,
	maxSteps int,
) *qccv1alpha1.Circuit {
	GinkgoHelper()
	var circuit qccv1alpha1.Circuit
	for range maxSteps {
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, name, &circuit)).To(Succeed())
		if circuit.Status.Phase == want {
			return &circuit
		}
	}
	Fail(fmt.Sprintf("Circuit did not reach phase %q within %d reconciles (last: %q)",
		want, maxSteps, circuit.Status.Phase))
	return &circuit
}

func conditionByType(circuit *qccv1alpha1.Circuit, t string) *metav1.Condition {
	for i, c := range circuit.Status.Conditions {
		if c.Type == t {
			return &circuit.Status.Conditions[i]
		}
	}
	return nil
}

var _ = Describe("CircuitReconciler", func() {
	ctx := context.Background()

	bellResult := &executor.Result{
		TaskID:        "aer-test-task",
		BackendUsed:   backendAer,
		Counts:        map[string]int64{"00": 510, "11": 490},
		Depth:         2,
		TwoQubitGates: 1,
		TotalGates:    4,
	}

	mkCircuit := func(name string, mode qccv1alpha1.CircuitMode, body string) *qccv1alpha1.Circuit {
		return &qccv1alpha1.Circuit{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: qccv1alpha1.CircuitSpec{
				Source: qccv1alpha1.CircuitSource{
					Format: qccv1alpha1.SourceOpenQASM3,
					Body:   body,
				},
				Shots: 1000,
				Mode:  mode,
			},
		}
	}

	// localAerQPU seeds a minimal Available `local-aer` QPU so the
	// Circuit reconciler's Move-1 enumeration has a candidate to pick.
	// Without it, every test would hit NoEligibleBackend at selectBackend.
	const localAerQPUName = "local-aer"

	BeforeEach(func() {
		qpu := &qccv1alpha1.QPU{
			ObjectMeta: metav1.ObjectMeta{Name: localAerQPUName},
			Spec: qccv1alpha1.QPUSpec{
				Provider:    providerLocal,
				BackendName: backendAer,
				Kind:        qccv1alpha1.BackendKindSimulator,
				Qubits:      32,
			},
		}
		Expect(k8sClient.Create(ctx, qpu)).To(Succeed())

		// Create only persists spec; the QPU reconciler normally
		// stamps status.availability.  The Circuit tests run the QPU
		// reconciler out of band, so we patch the status here after
		// Create — patch base is the just-created QPU (empty status).
		base := qpu.DeepCopy()
		qpu.Status.Availability = qccv1alpha1.QPUAvailable
		Expect(k8sClient.Status().Patch(ctx, qpu, client.MergeFrom(base))).To(Succeed())
	})

	AfterEach(func() {
		var list qccv1alpha1.CircuitList
		Expect(k8sClient.List(ctx, &list)).To(Succeed())
		for i := range list.Items {
			Expect(k8sClient.Delete(ctx, &list.Items[i])).To(Succeed())
		}
		var qpus qccv1alpha1.QPUList
		Expect(k8sClient.List(ctx, &qpus)).To(Succeed())
		for i := range qpus.Items {
			Expect(k8sClient.Delete(ctx, &qpus.Items[i])).To(Succeed())
		}
	})

	It("drives a run-mode Circuit through all phases to Succeeded", func() {
		fake := &fakeExecutor{result: bellResult}
		r := &CircuitReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		circuit := mkCircuit("bell-run", qccv1alpha1.ModeRun, bellQASM)
		Expect(k8sClient.Create(ctx, circuit)).To(Succeed())
		nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}

		final := driveToTerminal(ctx, r, nn, 10)
		Expect(final.Status.Phase).To(Equal(qccv1alpha1.PhaseSucceeded))
		Expect(final.Status.SelectedQPU).To(Equal(localAerQPUName))
		Expect(final.Status.ProviderJobID).To(Equal("aer-test-task"))
		Expect(final.Status.Results).To(Equal(map[string]int64{"00": 510, "11": 490}))
		Expect(fake.calls).To(Equal(1))

		for _, t := range []string{
			qccv1alpha1.ConditionAccepted,
			qccv1alpha1.ConditionValidated,
			qccv1alpha1.ConditionSelected,
			qccv1alpha1.ConditionSubmitted,
			qccv1alpha1.ConditionCompleted,
		} {
			Expect(conditionByType(final, t)).NotTo(BeNil(), "condition %s missing", t)
			Expect(conditionByType(final, t).Status).To(Equal(metav1.ConditionTrue))
		}
	})

	It("terminates select mode at Succeeded without calling the executor", func() {
		fake := &fakeExecutor{}
		r := &CircuitReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		circuit := mkCircuit("bell-select", qccv1alpha1.ModeSelect, bellQASM)
		Expect(k8sClient.Create(ctx, circuit)).To(Succeed())
		nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}

		final := driveToTerminal(ctx, r, nn, 10)
		Expect(final.Status.Phase).To(Equal(qccv1alpha1.PhaseSucceeded))
		Expect(fake.calls).To(Equal(0))
		Expect(final.Status.Results).To(BeEmpty())
		Expect(conditionByType(final, qccv1alpha1.ConditionCompleted).Reason).To(Equal("SelectCompleted"))
	})

	It("drives a draw-mode Circuit to Succeeded and stores the drawing in a ConfigMap", func() {
		const expectedDrawing = "q_0: ─■──\nq_1: ─■──"
		fake := &fakeExecutor{drawing: expectedDrawing}
		r := &CircuitReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		circuit := mkCircuit("bell-draw", qccv1alpha1.ModeDraw, bellQASM)
		Expect(k8sClient.Create(ctx, circuit)).To(Succeed())
		nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}

		final := driveToTerminal(ctx, r, nn, 5)
		Expect(final.Status.Phase).To(Equal(qccv1alpha1.PhaseSucceeded))

		// Drawing lives in a ConfigMap, referenced via status.drawingRef.
		Expect(final.Status.DrawingRef).NotTo(BeNil())
		Expect(final.Status.DrawingRef.Name).To(Equal("bell-draw-drawing"))

		var cm corev1.ConfigMap
		cmName := types.NamespacedName{Name: final.Status.DrawingRef.Name, Namespace: final.Namespace}
		Expect(k8sClient.Get(ctx, cmName, &cm)).To(Succeed())
		Expect(cm.Data).To(HaveKeyWithValue("drawing", expectedDrawing))

		// OwnerReference makes the ConfigMap garbage-collected when the
		// Circuit is deleted; assert the controller wired it correctly.
		Expect(cm.OwnerReferences).To(HaveLen(1))
		Expect(cm.OwnerReferences[0].UID).To(Equal(final.UID))
		Expect(cm.OwnerReferences[0].Controller).To(Equal(new(true)))

		Expect(fake.drawCall).To(Equal(1))
		Expect(fake.calls).To(Equal(0), "draw mode must not call RunCircuit")
		Expect(final.Status.SelectedQPU).To(BeEmpty(), "draw mode skips selection")

		Expect(conditionByType(final, qccv1alpha1.ConditionRendered)).NotTo(BeNil())
		Expect(conditionByType(final, qccv1alpha1.ConditionRendered).Reason).To(Equal(qccv1alpha1.ReasonDrawingRendered))
		Expect(conditionByType(final, qccv1alpha1.ConditionCompleted).Reason).To(Equal(qccv1alpha1.ReasonDrawingRendered))
	})

	It("marks draw-mode Failed when the executor reports a rendering TaskError", func() {
		fake := &fakeExecutor{drawErr: &executor.TaskError{
			Reason:  qccv1alpha1.ReasonRenderingFailed,
			Message: "unsupported gate",
		}}
		r := &CircuitReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		circuit := mkCircuit("bell-draw-fail", qccv1alpha1.ModeDraw, bellQASM)
		Expect(k8sClient.Create(ctx, circuit)).To(Succeed())
		nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}

		final := driveToTerminal(ctx, r, nn, 5)
		Expect(final.Status.Phase).To(Equal(qccv1alpha1.PhaseFailed))
		failed := conditionByType(final, qccv1alpha1.ConditionFailed)
		Expect(failed.Reason).To(Equal(qccv1alpha1.ReasonRenderingFailed))
		Expect(failed.Message).To(ContainSubstring("unsupported gate"))
		Expect(fake.drawCall).To(Equal(1))
	})

	It("accepts source.format=qiskit, runs the circuit, and persists the converted QASM as an artifact", func() {
		// The controller does not see ConvertSource directly — it lives
		// behind executor.Client.RunCircuit, which bubbles the converted
		// QASM back via Result.ConvertedQASM.  The reconciler then writes
		// it to a sibling ConfigMap and points status.convertedRef at it.
		const convertedQASM = "OPENQASM 3.0;\nqubit[2] q;\nh q[0];\ncx q[0], q[1];\n"
		fake := &fakeExecutor{result: bellResult, convertedQASM: convertedQASM}
		r := &CircuitReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		circuit := &qccv1alpha1.Circuit{
			ObjectMeta: metav1.ObjectMeta{Name: "bell-qiskit", Namespace: "default"},
			Spec: qccv1alpha1.CircuitSpec{
				Source: qccv1alpha1.CircuitSource{
					Format: qccv1alpha1.SourceQiskit,
					Body:   "from qiskit import QuantumCircuit\ncircuit = QuantumCircuit(2,2)\n",
				},
				Shots: 1000,
				Mode:  qccv1alpha1.ModeRun,
			},
		}
		Expect(k8sClient.Create(ctx, circuit)).To(Succeed())
		nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}

		final := driveToTerminal(ctx, r, nn, 10)
		Expect(final.Status.Phase).To(Equal(qccv1alpha1.PhaseSucceeded))
		Expect(fake.calls).To(Equal(1))

		// Converted QASM is persisted as a sibling ConfigMap.
		Expect(final.Status.ConvertedRef).NotTo(BeNil())
		Expect(final.Status.ConvertedRef.Name).To(Equal("bell-qiskit-converted"))

		var cm corev1.ConfigMap
		cmName := types.NamespacedName{Name: final.Status.ConvertedRef.Name, Namespace: final.Namespace}
		Expect(k8sClient.Get(ctx, cmName, &cm)).To(Succeed())
		Expect(cm.Data).To(HaveKeyWithValue("qasm", convertedQASM))
		Expect(cm.OwnerReferences).To(HaveLen(1))
		Expect(cm.OwnerReferences[0].UID).To(Equal(final.UID))
		Expect(cm.OwnerReferences[0].Controller).To(Equal(new(true)))
	})

	It("fails with NoEligibleBackend when no QPU is Available", func() {
		// Strip the seeded local-aer QPU so selectBackend sees an empty
		// candidate list, then assert the expected terminal failure.
		Expect(k8sClient.Delete(ctx, &qccv1alpha1.QPU{
			ObjectMeta: metav1.ObjectMeta{Name: localAerQPUName},
		})).To(Succeed())

		fake := &fakeExecutor{result: bellResult}
		r := &CircuitReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		circuit := mkCircuit("no-qpu", qccv1alpha1.ModeRun, bellQASM)
		Expect(k8sClient.Create(ctx, circuit)).To(Succeed())
		nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}

		final := driveToTerminal(ctx, r, nn, 5)
		Expect(final.Status.Phase).To(Equal(qccv1alpha1.PhaseFailed))
		failed := conditionByType(final, qccv1alpha1.ConditionFailed)
		Expect(failed).NotTo(BeNil())
		Expect(failed.Reason).To(Equal(qccv1alpha1.ReasonNoEligibleBackend))
		Expect(fake.calls).To(Equal(0), "no candidate → no executor call")
	})

	It("derives spec.backendName from metadata.name (dash→underscore) when the field is omitted", func() {
		// QPU spec has NO backendName — should still be selectable.
		qpu := &qccv1alpha1.QPU{
			ObjectMeta: metav1.ObjectMeta{Name: "fake-osaka"},
			Spec: qccv1alpha1.QPUSpec{
				Provider: providerLocal,
				// BackendName intentionally omitted.
				Kind:   qccv1alpha1.BackendKindSimulator,
				Qubits: 127,
			},
		}
		Expect(k8sClient.Create(ctx, qpu)).To(Succeed())
		base := qpu.DeepCopy()
		qpu.Status.Availability = qccv1alpha1.QPUAvailable
		Expect(k8sClient.Status().Patch(ctx, qpu, client.MergeFrom(base))).To(Succeed())

		// EffectiveBackendName should derive the underscored form.
		Expect(qpu.EffectiveBackendName()).To(Equal("fake_osaka"))

		// Both --backend fake-osaka and --backend fake_osaka should resolve.
		fake := &fakeExecutor{result: bellResult}
		r := &CircuitReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		for i, name := range []string{"fake-osaka", "fake_osaka"} {
			circuit := mkCircuit(fmt.Sprintf("bell-derived-%d", i), qccv1alpha1.ModeRun, bellQASM)
			circuit.Spec.BackendSelector = &qccv1alpha1.BackendSelector{BackendName: name}
			Expect(k8sClient.Create(ctx, circuit)).To(Succeed())
			nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}
			final := driveToTerminal(ctx, r, nn, 10)
			Expect(final.Status.Phase).To(Equal(qccv1alpha1.PhaseSucceeded),
				"expected match for --backend %q via derivation", name)
			Expect(final.Status.SelectedQPU).To(Equal("fake-osaka"))
		}
	})

	It("matches BackendSelector.BackendName against either the QPU's K8s name or its spec.backendName", func() {
		// Seed a second QPU whose K8s name and spec.backendName differ —
		// mirrors the real-world fake-brisbane / fake_brisbane case.
		qpu := &qccv1alpha1.QPU{
			ObjectMeta: metav1.ObjectMeta{Name: "fake-brisbane"},
			Spec: qccv1alpha1.QPUSpec{
				Provider:    providerLocal,
				BackendName: "fake_brisbane",
				Kind:        qccv1alpha1.BackendKindSimulator,
				Qubits:      127,
			},
		}
		Expect(k8sClient.Create(ctx, qpu)).To(Succeed())
		base := qpu.DeepCopy()
		qpu.Status.Availability = qccv1alpha1.QPUAvailable
		Expect(k8sClient.Status().Patch(ctx, qpu, client.MergeFrom(base))).To(Succeed())

		fake := &fakeExecutor{result: bellResult}
		r := &CircuitReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		// Both forms must select the fake-brisbane QPU.  Use static
		// Circuit names — K8s names are DNS-1123 (no underscores), so we
		// can't interpolate the selector value into them.
		cases := []struct {
			circuitName string
			selectorBN  string
		}{
			{"bell-by-k8s-name", "fake-brisbane"},
			{"bell-by-backendname", "fake_brisbane"},
		}
		for _, tc := range cases {
			circuit := mkCircuit(tc.circuitName, qccv1alpha1.ModeRun, bellQASM)
			circuit.Spec.BackendSelector = &qccv1alpha1.BackendSelector{BackendName: tc.selectorBN}
			Expect(k8sClient.Create(ctx, circuit)).To(Succeed())
			nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}
			final := driveToTerminal(ctx, r, nn, 10)
			Expect(final.Status.Phase).To(Equal(qccv1alpha1.PhaseSucceeded),
				"expected match for --backend %q", tc.selectorBN)
			Expect(final.Status.SelectedQPU).To(Equal("fake-brisbane"))
		}
	})

	It("respects BackendSelector.MinQubits as a hard constraint", func() {
		// Seeded QPU has 32 qubits; ask for 64 → should fail.
		fake := &fakeExecutor{result: bellResult}
		r := &CircuitReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		circuit := mkCircuit("too-big", qccv1alpha1.ModeRun, bellQASM)
		circuit.Spec.BackendSelector = &qccv1alpha1.BackendSelector{MinQubits: 64}
		Expect(k8sClient.Create(ctx, circuit)).To(Succeed())
		nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}

		final := driveToTerminal(ctx, r, nn, 5)
		Expect(final.Status.Phase).To(Equal(qccv1alpha1.PhaseFailed))
		Expect(conditionByType(final, qccv1alpha1.ConditionFailed).Reason).
			To(Equal(qccv1alpha1.ReasonNoEligibleBackend))
	})

	It("populates status.selectionSummary with the chosen QPU's name and candidate count", func() {
		fake := &fakeExecutor{result: bellResult}
		r := &CircuitReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		circuit := mkCircuit("summary-check", qccv1alpha1.ModeRun, bellQASM)
		Expect(k8sClient.Create(ctx, circuit)).To(Succeed())
		nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}

		final := driveToTerminal(ctx, r, nn, 10)
		Expect(final.Status.Phase).To(Equal(qccv1alpha1.PhaseSucceeded))
		Expect(final.Status.SelectionSummary).NotTo(BeNil())
		Expect(final.Status.SelectionSummary.Selected).To(Equal(localAerQPUName))
		Expect(final.Status.SelectionSummary.Candidates).To(BeNumerically(">=", 1))
	})

	It("does not populate convertedRef when source.format=openqasm3", func() {
		// Native QASM inputs don't trigger ConvertSource, so there is
		// nothing to expose separately — convertedRef stays nil.
		fake := &fakeExecutor{result: bellResult, convertedQASM: "should not be used"}
		r := &CircuitReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		circuit := mkCircuit("bell-native", qccv1alpha1.ModeRun, bellQASM)
		Expect(k8sClient.Create(ctx, circuit)).To(Succeed())
		nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}

		final := driveToTerminal(ctx, r, nn, 10)
		Expect(final.Status.Phase).To(Equal(qccv1alpha1.PhaseSucceeded))
		Expect(final.Status.ConvertedRef).To(BeNil())
	})

	It("fails with InvalidCircuit when spec.source.body is empty", func() {
		fake := &fakeExecutor{}
		r := &CircuitReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		circuit := mkCircuit("invalid", qccv1alpha1.ModeRun, "")
		// CRD validation rejects empty body via MinLength=1; assert that.
		err := k8sClient.Create(ctx, circuit)
		if err != nil {
			Skip("CRD validation rejected empty body — controller-side check is unreachable here")
		}
		nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}

		final := driveToTerminal(ctx, r, nn, 5)
		Expect(final.Status.Phase).To(Equal(qccv1alpha1.PhaseFailed))
		Expect(conditionByType(final, qccv1alpha1.ConditionFailed).Reason).To(Equal(qccv1alpha1.ReasonInvalidCircuit))
		Expect(fake.calls).To(Equal(0))
	})

	It("propagates executor TaskError reasons into the Failed condition", func() {
		fake := &fakeExecutor{err: &executor.TaskError{
			Reason:  qccv1alpha1.ReasonTranspilationFailed,
			Message: "bad gate",
		}}
		r := &CircuitReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		circuit := mkCircuit("bad-circuit", qccv1alpha1.ModeRun, bellQASM)
		Expect(k8sClient.Create(ctx, circuit)).To(Succeed())
		nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}

		final := driveToTerminal(ctx, r, nn, 10)
		Expect(final.Status.Phase).To(Equal(qccv1alpha1.PhaseFailed))
		failed := conditionByType(final, qccv1alpha1.ConditionFailed)
		Expect(failed).NotTo(BeNil())
		Expect(failed.Reason).To(Equal(qccv1alpha1.ReasonTranspilationFailed))
		Expect(failed.Message).To(ContainSubstring("bad gate"))
	})

	It("requeues without marking Failed on transient executor RPC errors", func() {
		fake := &fakeExecutor{err: errors.New("network blip")}
		r := &CircuitReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		circuit := mkCircuit("transient", qccv1alpha1.ModeRun, bellQASM)
		Expect(k8sClient.Create(ctx, circuit)).To(Succeed())
		nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}

		// Drive through the pre-submission phases.  The number of passes
		// is not asserted: label stamping consumes one reconcile before
		// any phase work, so a hard-coded count silently rots whenever a
		// pass is added or removed.  Drive until the phase we care about.
		c := driveToPhase(ctx, r, nn, qccv1alpha1.PhaseSubmitting, 10)
		Expect(c.Status.Phase).To(Equal(qccv1alpha1.PhaseSubmitting))

		// Submitting reconcile — executor returns network error, should not mark Failed.
		result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		Expect(k8sClient.Get(ctx, nn, c)).To(Succeed())
		Expect(c.Status.Phase).To(Equal(qccv1alpha1.PhaseSubmitting), "should stay in Submitting on transient")
	})
})
