/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	qccv1alpha1 "github.com/ioaiaaii/quantum-circuit-controller/api/v1alpha1"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/cli/kubeclient"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/cli/render"
)

type runOpts struct {
	namespace    string
	shots        int32
	selectOnly   bool
	provider     string
	backendName  string
	timeout      time.Duration
	pollInterval time.Duration
	detach       bool
	kubeconfig   string
	// Algorithm-grouping labels (see QCC-API.md §5.4).  Stamped on
	// the Circuit's metadata.labels under their canonical
	// `qcc.io/<key>` form so PromQL queries, dashboard variables,
	// and `qcc get --algorithm=X` filters all line up.  Optional —
	// omitting them produces a one-off Circuit identical to
	// pre-labels behaviour.
	algorithm  string
	version    string
	experiment string
	// extraLabels accepts `key=value` pairs (repeatable) for any
	// non-canonical label the user wants on the Circuit.  Provides
	// an escape hatch without committing to additional reserved
	// keys in the API package.
	extraLabels []string
	// performanceTest, when true, discovers every simulator QPU in
	// the cluster (or simulator + hardware with --include-hardware),
	// submits the same Circuit body across all of them under a
	// shared `qcc.io/experiment` label, then prints a comparison
	// table once they all reach a terminal phase.  This is the
	// platform's empirical cross-substrate evaluation primitive —
	// see `QCC-Design-State.md` decision-log 2026-05-17 (evening,
	// third pass) for the scope rationale.
	performanceTest   bool
	perfTestIncludeHW bool
}

func newRunCmd(version string) *cobra.Command {
	o := &runOpts{}
	cmd := &cobra.Command{
		Use:   "run <file>",
		Short: "Submit a circuit and watch it run",
		Long: "Creates a Circuit resource and streams progress until terminal.  " +
			"Accepts OpenQASM 3 (.qasm) or Qiskit-Python (.py); the executor " +
			"picks the loader and (for qiskit) converts to QASM 3 server-side.",
		Example: `  qcc run bell.qasm
  qcc run shor.py --shots 4096
  qcc run bell.qasm --select-only --provider local
  qcc run bell.qasm --backend ibm_sherbrooke --timeout 30m
  qcc run bell.qasm --backend ibm-fez --detach           # submit + exit; check with qcc get
  qcc run vqe.py --algorithm vqe-h2 --version v2         # group with other vqe-h2 runs
  qcc run vqe.py --algorithm vqe-h2 --experiment noise-survey --label team=hpc
  qcc run shor.py --performance-test --algorithm shor    # ladder across every simulator
  qcc run shor.py --performance-test --include-hardware  # add IBM real-hardware QPUs too`,
		Args: argsWithHelp(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCircuit(cmd.Context(), version, args[0], o)
		},
	}
	cmd.Flags().StringVarP(&o.namespace, "namespace", "n", "default", "Namespace to create the Circuit in")
	cmd.Flags().Int32Var(&o.shots, "shots", 1024, "Number of shots to execute")
	cmd.Flags().BoolVar(&o.selectOnly, "select-only", false, "Run in selectOnly mode (selection without execution)")
	cmd.Flags().StringVar(&o.provider, "provider", "", "Optional provider constraint (e.g. local, ibm)")
	cmd.Flags().StringVar(&o.backendName, "backend", "",
		"Backend name — matches the QPU's K8s name (e.g. fake-brisbane) "+
			"or its spec.backendName (e.g. fake_brisbane)")
	// 30m default accommodates real-hardware queues — IBM Open Plan jobs
	// can wait minutes (occasionally tens of minutes) for execution.
	// Simulator runs complete in seconds regardless; the higher ceiling
	// doesn't slow them down (it only triggers when nothing completes).
	// Override for short-circuit dev loops with --timeout 30s.
	cmd.Flags().DurationVar(&o.timeout, "timeout", 30*time.Minute, "Max wall-clock time to wait for completion")
	cmd.Flags().DurationVar(&o.pollInterval, "poll", 500*time.Millisecond, "Status poll interval")
	// --detach: submit and exit as soon as the controller has accepted
	// the Circuit and a provider job is queued.  Useful for real-hardware
	// runs where the queue wait can be minutes — the controller polls
	// in the background, and the user checks back later with
	// `qcc get circuit <name>`.  This is the K8s-native pattern of
	// "submit + walk away" applied to circuit execution.
	cmd.Flags().BoolVar(&o.detach, "detach", false,
		"Submit the Circuit and exit once a provider job is queued — "+
			"controller continues in the background; check status with `qcc get circuit`")
	cmd.Flags().StringVar(&o.kubeconfig, "kubeconfig", "", "Path to kubeconfig (defaults to KUBECONFIG / ~/.kube/config)")
	// Algorithm-grouping flags.  Stamp the canonical qcc.io/* labels
	// on the Circuit so cross-run correlation works in PromQL,
	// dashboards, and `qcc get --algorithm`.  All optional.
	cmd.Flags().StringVar(&o.algorithm, "algorithm", "",
		"Algorithm family this run belongs to (stamps qcc.io/algorithm). "+
			"Required for run-index auto-numbering and per-algorithm dashboards.")
	cmd.Flags().StringVar(&o.version, "version", "",
		"Algorithm version (stamps qcc.io/algorithm-version, e.g. 'v2'). "+
			"Used to compare iterations of the same algorithm.")
	cmd.Flags().StringVar(&o.experiment, "experiment", "",
		"Optional experiment/campaign identifier (stamps qcc.io/experiment). "+
			"Groups runs across multiple algorithms in the same study.")
	cmd.Flags().StringArrayVarP(&o.extraLabels, "label", "l", nil,
		"Additional label in key=value form (repeatable).  Escape hatch for "+
			"non-canonical labels; canonical algorithm grouping should use "+
			"--algorithm / --version / --experiment instead.")
	// Performance-test mode: cross-substrate ladder for one Circuit
	// body, sharing a `qcc.io/experiment` label so the Grafana
	// dashboard groups the runs automatically.  See the
	// QCC-Design-State decision log for the scope rationale.
	cmd.Flags().BoolVar(&o.performanceTest, "performance-test", false,
		"Submit the same Circuit across all simulator QPUs with a shared "+
			"`qcc.io/experiment` label; prints a comparison table and a "+
			"Grafana link.  Mutually exclusive with --backend / --provider "+
			"/ --detach / --select-only.")
	cmd.Flags().BoolVar(&o.perfTestIncludeHW, "include-hardware", false,
		"Performance-test mode only: include real-hardware QPUs in the "+
			"ladder.  Off by default because real-hardware credits are "+
			"non-trivial; turn on intentionally for cross-fidelity studies.")
	return cmd
}

func runCircuit(ctx context.Context, version, file string, o *runOpts) error {
	if o.performanceTest {
		return runPerformanceTest(ctx, version, file, o)
	}

	fmt.Print(render.Banner(version))

	loadStart := time.Now()
	format, body, err := loadSourceFile(file)
	if err != nil {
		fmt.Print(render.Fail("load failed", err.Error()))
		return err
	}
	fmt.Print(render.Step(
		fmt.Sprintf("loading %s", filepath.Base(file)),
		fmt.Sprintf("· %s · %d ms", format, time.Since(loadStart).Milliseconds()),
	))

	cli, err := kubeclient.New(o.kubeconfig)
	if err != nil {
		fmt.Print(render.Fail("kubeconfig", err.Error()))
		return err
	}

	circuit, err := buildCircuit(file, format, body, o)
	if err != nil {
		fmt.Print(render.Fail("build circuit", err.Error()))
		return err
	}
	if err := cli.Create(ctx, circuit); err != nil {
		fmt.Print(render.Fail("create circuit", err.Error()))
		return err
	}
	fmt.Print(render.Step("submitted",
		fmt.Sprintf("%s/%s", circuit.Namespace, circuit.Name)))

	if o.detach {
		return watchToQueued(ctx, cli, circuit, o)
	}
	return watchToTerminal(ctx, cli, circuit, o)
}

// watchToQueued is the --detach exit path: poll just until the
// controller has accepted the Circuit and a provider job is queued
// (status.providerJobID set), then print a hint and exit.  The
// controller keeps polling the vendor in the background; `qcc get
// circuit <name>` shows progress and final results.
//
// Bounded by a short timeout (60s) because submission itself is
// supposed to be fast — if the controller doesn't get to PhaseRunning
// in a minute, something's wrong and we exit with an error rather
// than walking away from a stuck Circuit.
func watchToQueued(ctx context.Context, cli client.Client, circuit *qccv1alpha1.Circuit, o *runOpts) error {
	// 60s ceiling for submission to reach a queued state.  Even busy
	// controllers should process reconciles within seconds; if not,
	// the user wants to know rather than detach into a stuck state.
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}

	sp := render.NewSpinner(os.Stdout)
	sp.Start("waiting for selection")
	defer sp.Stop()

	var seenBackend bool
	for {
		select {
		case <-ctx.Done():
			sp.FinishFail(fmt.Sprintf("timeout · %s did not get queued in 60s", circuit.Name))
			return errors.New("timeout waiting for circuit to be queued")
		case <-time.After(o.pollInterval):
		}

		var c qccv1alpha1.Circuit
		if err := cli.Get(ctx, nn, &c); err != nil {
			sp.FinishFail("poll failed · " + err.Error())
			return err
		}

		if !seenBackend && c.Status.SelectedQPU != "" {
			sp.FinishOK("targeting " + c.Status.SelectedQPU)
			sp.Start("submitting")
			seenBackend = true
		}

		// The detach exit point: as soon as the controller stamps
		// the provider job ID, the work is durably in the vendor's
		// queue (or in-process for Aer).  Safe to walk away.
		if c.Status.ProviderJobID != "" {
			sp.FinishOK("queued · job " + c.Status.ProviderJobID)
			fmt.Println()
			fmt.Print(render.Step("detached",
				fmt.Sprintf("check progress with: qcc get circuit %s", circuit.Name)))
			return nil
		}

		// Failure during submission (e.g. NoEligibleBackend) — surface
		// it instead of detaching, so the user sees the problem.
		if c.Status.Phase == qccv1alpha1.PhaseFailed {
			reason, msg := failureReason(&c)
			sp.FinishFail(fmt.Sprintf("%s · %s", reason, msg))
			return fmt.Errorf("circuit %s failed before queue: %s", c.Name, reason)
		}
	}
}

func buildCircuit(file string, format qccv1alpha1.SourceFormat, body string, o *runOpts) (*qccv1alpha1.Circuit, error) {
	mode := qccv1alpha1.ModeRun
	if o.selectOnly {
		mode = qccv1alpha1.ModeSelect
	}
	labels, err := buildCircuitLabels(o)
	if err != nil {
		return nil, err
	}
	c := &qccv1alpha1.Circuit{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateName(file),
			Namespace: o.namespace,
			Labels:    labels,
		},
		Spec: qccv1alpha1.CircuitSpec{
			Source: qccv1alpha1.CircuitSource{
				Format: format,
				Body:   body,
			},
			Shots: o.shots,
			Mode:  mode,
		},
	}
	if o.provider != "" || o.backendName != "" {
		c.Spec.BackendSelector = &qccv1alpha1.BackendSelector{
			Provider:    o.provider,
			BackendName: o.backendName,
		}
	}
	return c, nil
}

// buildCircuitLabels assembles metadata.labels from the algorithm-
// grouping flags and the --label key=value escape hatch.  Returns
// nil (not an empty map) when no labels are set so the resulting
// Circuit YAML stays clean for one-off submissions.
//
// Validation:
//   - --version and --experiment require --algorithm to be set
//     (a version label without an algorithm to anchor it is
//     meaningless for grouping; explicit error beats silent oddness).
//   - --label values must be key=value pairs.
func buildCircuitLabels(o *runOpts) (map[string]string, error) {
	if (o.version != "" || o.experiment != "") && o.algorithm == "" {
		return nil, errors.New("--version and --experiment require --algorithm to be set")
	}
	labels := map[string]string{}
	if o.algorithm != "" {
		labels[qccv1alpha1.LabelAlgorithm] = o.algorithm
	}
	if o.version != "" {
		labels[qccv1alpha1.LabelAlgorithmVersion] = o.version
	}
	if o.experiment != "" {
		labels[qccv1alpha1.LabelExperiment] = o.experiment
	}
	for _, kv := range o.extraLabels {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --label %q (expected key=value)", kv)
		}
		labels[k] = v
	}
	if len(labels) == 0 {
		return nil, nil
	}
	return labels, nil
}

func watchToTerminal(ctx context.Context, cli client.Client, circuit *qccv1alpha1.Circuit, o *runOpts) error {
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}
	start := time.Now()

	sp := render.NewSpinner(os.Stdout)
	sp.Start("waiting for selection")
	defer sp.Stop()

	var seenBackend, seenJob bool
	for {
		select {
		case <-ctx.Done():
			sp.FinishFail(fmt.Sprintf("timeout · %s did not reach terminal in %s", circuit.Name, o.timeout))
			return errors.New("timeout")
		case <-time.After(o.pollInterval):
		}

		var c qccv1alpha1.Circuit
		if err := cli.Get(ctx, nn, &c); err != nil {
			sp.FinishFail("poll failed · " + err.Error())
			return err
		}

		if !seenBackend && c.Status.SelectedQPU != "" {
			sp.FinishOK("targeting " + c.Status.SelectedQPU)
			sp.Start("transpiling for " + c.Status.SelectedQPU)
			seenBackend = true
		} else {
			updateSpinnerForPhase(sp, c.Status.Phase)
		}

		if !seenJob && c.Status.ProviderJobID != "" {
			sp.FinishOK("queued · job " + c.Status.ProviderJobID)
			sp.Start("executing")
			seenJob = true
		}

		switch c.Status.Phase {
		case qccv1alpha1.PhaseSucceeded:
			elapsed := time.Since(start).Round(time.Millisecond)
			sp.FinishOK("completed · " + elapsed.String())
			fmt.Println()
			// Resolve the selected QPU so the result section can render
			// gate-error medians, coherence times, calibration date,
			// and the expected-error estimate (Ch1-derived helpful
			// details).  Best-effort: if the QPU lookup fails, fall
			// back to the bare circuit summary.
			var resolvedQPU *qccv1alpha1.QPU
			if c.Status.SelectedQPU != "" {
				var fetched qccv1alpha1.QPU
				if err := cli.Get(ctx, types.NamespacedName{Name: c.Status.SelectedQPU}, &fetched); err == nil {
					resolvedQPU = &fetched
				}
			}
			fmt.Print(render.Section("results", buildCircuitSummary(&c, resolvedQPU)))
			// Footer hints when extra artifacts are available; mirrors
			// `qcc get`'s default view so users discover the same surface
			// regardless of how they arrived at it.
			printArtifactHints(&c)
			return nil
		case qccv1alpha1.PhaseFailed:
			reason, msg := failureReason(&c)
			sp.FinishFail(fmt.Sprintf("%s · %s", reason, msg))
			return fmt.Errorf("circuit %s failed: %s", c.Name, reason)
		}
	}
}

// updateSpinnerForPhase keeps the spinner suffix coherent with the controller's
// phase machine, so the user sees "transpiling" / "submitting" / "executing"
// even when no new milestone has been hit.
func updateSpinnerForPhase(sp *render.Spinner, phase qccv1alpha1.CircuitPhase) {
	switch phase {
	case qccv1alpha1.PhaseSelecting:
		sp.Update("selecting backend")
	case qccv1alpha1.PhaseTranspiling:
		sp.Update("transpiling")
	case qccv1alpha1.PhaseSubmitting:
		sp.Update("submitting")
	case qccv1alpha1.PhaseRunning:
		sp.Update("executing")
	}
}

// strOr returns s if non-empty, else the placeholder for absent values.
// Today every caller passes emDash; kept as a second parameter so future
// callers can override (e.g. "<unknown>" for diagnostic output).
func strOr(s, fallback string) string { //nolint:unparam // fallback intentionally parameterised
	if s == "" {
		return fallback
	}
	return s
}

func failureReason(c *qccv1alpha1.Circuit) (reason, message string) {
	for _, cond := range c.Status.Conditions {
		if cond.Type == qccv1alpha1.ConditionFailed {
			return cond.Reason, cond.Message
		}
	}
	return "Failed", "no reason reported"
}

func generateName(file string) string {
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	base = strings.ToLower(strings.ReplaceAll(base, "_", "-"))
	if base == "" {
		base = "circuit"
	}
	return fmt.Sprintf("%s-%s", base, rand.String(5))
}
