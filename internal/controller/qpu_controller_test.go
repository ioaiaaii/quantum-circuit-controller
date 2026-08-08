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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	qccv1alpha1 "github.com/ioaiaaii/quantum-circuit-controller/api/v1alpha1"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/executor"
)

func qpuConditionByType(qpu *qccv1alpha1.QPU, t string) *metav1.Condition {
	for i, c := range qpu.Status.Conditions {
		if c.Type == t {
			return &qpu.Status.Conditions[i]
		}
	}
	return nil
}

var _ = Describe("QPUReconciler", func() {
	ctx := context.Background()

	AfterEach(func() {
		var list qccv1alpha1.QPUList
		Expect(k8sClient.List(ctx, &list)).To(Succeed())
		for i := range list.Items {
			Expect(k8sClient.Delete(ctx, &list.Items[i])).To(Succeed())
		}
	})

	It("marks a local-provider QPU Available immediately, with Ready + MetadataFresh conditions", func() {
		fake := &fakeExecutor{}
		r := &QPUReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		qpu := &qccv1alpha1.QPU{
			ObjectMeta: metav1.ObjectMeta{Name: "qpu-test-local"},
			Spec: qccv1alpha1.QPUSpec{
				Provider:    providerLocal,
				BackendName: backendAer,
				Kind:        qccv1alpha1.BackendKindSimulator,
				Qubits:      32,
			},
		}
		Expect(k8sClient.Create(ctx, qpu)).To(Succeed())
		nn := types.NamespacedName{Name: qpu.Name}

		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		var got qccv1alpha1.QPU
		Expect(k8sClient.Get(ctx, nn, &got)).To(Succeed())
		Expect(got.Status.Availability).To(Equal(qccv1alpha1.QPUAvailable))
		Expect(got.Status.ObservedGeneration).To(Equal(got.Generation))

		ready := qpuConditionByType(&got, qccv1alpha1.ConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		Expect(ready.Reason).To(Equal(qccv1alpha1.ReasonProviderProbeOK))

		fresh := qpuConditionByType(&got, qccv1alpha1.ConditionMetadataFresh)
		Expect(fresh).NotTo(BeNil())
		Expect(fresh.Status).To(Equal(metav1.ConditionTrue))
		Expect(fresh.Reason).To(Equal(qccv1alpha1.ReasonCalibrationRefreshed))
	})

	It("marks an IBM-provider QPU as Available with Ready=True (probe-driven, M3)", func() {
		// Probe returns a minimal profile so applyBackendProfile has
		// something to stamp; the test asserts on the availability +
		// Ready condition, not the calibration enrichment itself
		// (covered by the "enriches status with probe data" spec).
		fake := &fakeExecutor{
			probeProfile: &executor.BackendProfile{
				NumQubits:           127,
				LastCalibrationTime: "2026-05-16T10:00:00Z",
			},
		}
		r := &QPUReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		qpu := &qccv1alpha1.QPU{
			ObjectMeta: metav1.ObjectMeta{Name: "qpu-test-hw"},
			Spec: qccv1alpha1.QPUSpec{
				Provider:    "ibm",
				BackendName: "ibm_brisbane",
				Kind:        qccv1alpha1.BackendKindHardware,
				Qubits:      127,
			},
		}
		Expect(k8sClient.Create(ctx, qpu)).To(Succeed())
		nn := types.NamespacedName{Name: qpu.Name}

		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		var got qccv1alpha1.QPU
		Expect(k8sClient.Get(ctx, nn, &got)).To(Succeed())

		// IBM provider is treated as optimistically Available in M3 (the
		// adapter is real; the probe path is real).  Failure to probe
		// (bad token, network) still leaves the QPU Available with
		// empty calibration — surfaced via the next reconcile retry.
		Expect(got.Status.Availability).To(Equal(qccv1alpha1.QPUAvailable))

		ready := qpuConditionByType(&got, qccv1alpha1.ConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		Expect(ready.Reason).To(Equal(qccv1alpha1.ReasonProviderProbeOK))

		// MetadataFresh is intentionally NOT asserted on hardware
		// providers — live calibration drifts, so freshness is
		// tracked via status.lastCalibrationTime, not a static
		// boolean condition (see desiredQPUStatus comment).
		Expect(qpuConditionByType(&got, qccv1alpha1.ConditionMetadataFresh)).To(BeNil())
	})

	It("leaves an unknown-provider QPU as Unknown (no adapter wired)", func() {
		fake := &fakeExecutor{}
		r := &QPUReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		qpu := &qccv1alpha1.QPU{
			ObjectMeta: metav1.ObjectMeta{Name: "qpu-test-unknown-provider"},
			Spec: qccv1alpha1.QPUSpec{
				Provider:    "cuda-q", // Ch9 future-work substrate, no adapter today
				BackendName: "nvidia-h100",
				Kind:        qccv1alpha1.BackendKindHardware,
			},
		}
		Expect(k8sClient.Create(ctx, qpu)).To(Succeed())
		nn := types.NamespacedName{Name: qpu.Name}

		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		var got qccv1alpha1.QPU
		Expect(k8sClient.Get(ctx, nn, &got)).To(Succeed())
		Expect(got.Status.Availability).To(Equal(qccv1alpha1.QPUUnknown))
		// No positive conditions for providers without a shipped adapter.
		Expect(qpuConditionByType(&got, qccv1alpha1.ConditionReady)).To(BeNil())
	})

	It("enriches status with probe data (qubits, basisGates, errorMedians, calibrationTime)", func() {
		fake := &fakeExecutor{
			probeProfile: &executor.BackendProfile{
				NumQubits:              127,
				BasisGates:             []string{"ecr", "id", "rz", "sx", "x"},
				CouplingEdges:          144,
				LastCalibrationTime:    "2025-02-26T14:33:06-05:00",
				SingleQubitErrorMedian: 0.0002,
				TwoQubitErrorMedian:    0.008,
				ReadoutErrorMedian:     0.02,
			},
		}
		r := &QPUReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		qpu := &qccv1alpha1.QPU{
			ObjectMeta: metav1.ObjectMeta{Name: "qpu-test-probe"},
			Spec: qccv1alpha1.QPUSpec{
				Provider: providerLocal,
				// backendName omitted on purpose; derived from K8s
				// name via dash→underscore → "qpu_test_probe".
				Kind: qccv1alpha1.BackendKindSimulator,
				// qubits also omitted; comes from probe.
			},
		}
		Expect(k8sClient.Create(ctx, qpu)).To(Succeed())
		nn := types.NamespacedName{Name: qpu.Name}

		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(fake.probeCall).To(Equal(1), "probe should run once on first reconcile")

		var got qccv1alpha1.QPU
		Expect(k8sClient.Get(ctx, nn, &got)).To(Succeed())

		Expect(got.Status.Qubits).To(Equal(int32(127)))
		Expect(got.Status.BasisGates).To(ConsistOf("ecr", "id", "rz", "sx", "x"))
		Expect(got.Status.CouplingEdges).To(Equal(int32(144)))
		Expect(got.Status.LastCalibrationTime).NotTo(BeNil())
		Expect(got.Status.ErrorMedians).NotTo(BeNil())
		Expect(got.Status.ErrorMedians.SingleQubit).To(BeNumerically("~", 0.0002, 1e-9))
		Expect(got.Status.ErrorMedians.TwoQubit).To(BeNumerically("~", 0.008, 1e-9))
		Expect(got.Status.ErrorMedians.Readout).To(BeNumerically("~", 0.02, 1e-9))

		// EffectiveQubits prefers status (probed) over spec (zero here).
		Expect(got.EffectiveQubits()).To(Equal(int32(127)))

		// Second reconcile must not re-probe — status.qubits is set.
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(fake.probeCall).To(Equal(1), "probe must not re-fire when status already populated")
	})

	It("still goes Available when ProbeBackend fails (probe is non-fatal)", func() {
		fake := &fakeExecutor{probeErr: errors.New("simulated probe transport failure")}
		r := &QPUReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		qpu := &qccv1alpha1.QPU{
			ObjectMeta: metav1.ObjectMeta{Name: "qpu-test-probe-fail"},
			Spec: qccv1alpha1.QPUSpec{
				Provider:    providerLocal,
				BackendName: backendAer,
				Kind:        qccv1alpha1.BackendKindSimulator,
				Qubits:      16, // user hint, falls through to EffectiveQubits
			},
		}
		Expect(k8sClient.Create(ctx, qpu)).To(Succeed())
		nn := types.NamespacedName{Name: qpu.Name}

		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		var got qccv1alpha1.QPU
		Expect(k8sClient.Get(ctx, nn, &got)).To(Succeed())
		// Availability still flipped to Available; probe enrichment
		// just missing.  Selection falls back to spec.qubits.
		Expect(got.Status.Availability).To(Equal(qccv1alpha1.QPUAvailable))
		Expect(got.Status.Qubits).To(Equal(int32(0)), "probe failed → status.qubits stays zero")
		Expect(got.EffectiveQubits()).To(Equal(int32(16)), "falls back to spec.qubits hint")
	})

	It("is a no-op when the QPU's status already matches the desired state", func() {
		fake := &fakeExecutor{}
		r := &QPUReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Executor: fake}

		qpu := &qccv1alpha1.QPU{
			ObjectMeta: metav1.ObjectMeta{Name: "qpu-test-idempotent"},
			Spec: qccv1alpha1.QPUSpec{
				Provider:    providerLocal,
				BackendName: backendAer,
				Kind:        qccv1alpha1.BackendKindSimulator,
				Qubits:      16,
			},
		}
		Expect(k8sClient.Create(ctx, qpu)).To(Succeed())
		nn := types.NamespacedName{Name: qpu.Name}

		// First reconcile populates status.
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		var afterFirst qccv1alpha1.QPU
		Expect(k8sClient.Get(ctx, nn, &afterFirst)).To(Succeed())
		firstReady := qpuConditionByType(&afterFirst, qccv1alpha1.ConditionReady)
		Expect(firstReady).NotTo(BeNil())
		firstStamp := firstReady.LastTransitionTime

		// Second reconcile must not churn the condition's transition
		// time — qpuStatusMatches should detect the no-op and skip.
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		var afterSecond qccv1alpha1.QPU
		Expect(k8sClient.Get(ctx, nn, &afterSecond)).To(Succeed())
		secondReady := qpuConditionByType(&afterSecond, qccv1alpha1.ConditionReady)
		Expect(secondReady).NotTo(BeNil())
		Expect(secondReady.LastTransitionTime).To(Equal(firstStamp),
			"transition time should be stable across no-op reconciles")
	})
})
