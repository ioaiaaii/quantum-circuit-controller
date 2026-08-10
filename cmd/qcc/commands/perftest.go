/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package commands

// `qcc run --performance-test` orchestration.
//
// Submits one Circuit per discovered candidate QPU under a shared
// `qcc.io/experiment` label, waits for them all to terminal, then
// prints a comparison table and a deep link to the Circuit Grafana
// dashboard with the experiment filter pre-applied.
//
// This is the platform's *empirical* cross-substrate evaluation
// primitive — no analytical scorer, no per-candidate prediction
// formula.  Same source body, every available simulator (and
// optionally real hardware via --include-hardware), table at the end.
//
// Rationale in `docs/systems-design/QCC-Design-State.md` decision-log
// entry 2026-05-17 (evening, third pass): the thesis claim is
// "orchestration platform with empirical cross-substrate evaluation",
// not "platform with predictive backend selection".  Predictive
// scoring (Move 5 formula, fake-twin variant, full Moves 2–4 +
// mapomatic) are collected under Ch9 "selection-chain extensions".

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	qccv1alpha1 "github.com/ioaiaaii/quantum-circuit-controller/api/v1alpha1"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/cli/kubeclient"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/cli/render"
)

// runPerformanceTest is the entry point for `qcc run --performance-test`.
// Pulls every QPU passing the candidate filter (simulators by default,
// + hardware when --include-hardware is set), submits one Circuit per
// QPU under a shared `qcc.io/experiment` label, blocks until they all
// reach a terminal phase (or the timeout fires), and prints a
// comparison table + a Grafana deep link.
//
// Returns an error only on infrastructure failures (kubeconfig, list,
// no candidates, all submissions failed); a perf-test where some
// individual Circuits fail is still considered "completed" — the
// failures show up in the comparison table for the user to see.
func runPerformanceTest(ctx context.Context, version, file string, o *runOpts) error {
	fmt.Print(render.Banner(version))

	if err := validatePerfTestOpts(o); err != nil {
		fmt.Print(render.Fail("performance-test", err.Error()))
		return err
	}

	// Load the source body once — re-used across every Circuit.
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

	// Discover the candidate QPU set.  Filters: status.Availability ==
	// Available, plus spec.Kind matches the include-hardware flag.
	candidates, err := listPerfTestQPUs(ctx, cli, o.perfTestIncludeHW)
	if err != nil {
		fmt.Print(render.Fail("list QPUs", err.Error()))
		return err
	}
	if len(candidates) == 0 {
		msg := "no Available simulator QPUs in the cluster"
		if o.perfTestIncludeHW {
			msg = "no Available QPUs in the cluster"
		}
		fmt.Print(render.Fail("performance-test", msg))
		return errors.New(msg)
	}

	// Default algorithm label to the file basename so the Grafana
	// `$algorithm` filter on the Circuit dashboard always has
	// something to group by.  Users can still override with --algorithm.
	if o.algorithm == "" {
		o.algorithm = defaultAlgorithmFromFile(file)
	}
	// Generate experiment id if the user didn't pass one.  Format is
	// chosen for sortability in a kubectl list view and for safe use
	// as a label value.
	if o.experiment == "" {
		o.experiment = fmt.Sprintf("perf-test-%s", time.Now().UTC().Format("20060102-150405"))
	}

	fmt.Print(render.Step(
		fmt.Sprintf("performance test · %d candidates", len(candidates)),
		fmt.Sprintf("algorithm=%s · experiment=%s", o.algorithm, o.experiment),
	))

	// Submit one Circuit per candidate in sequence — submission is
	// cheap (a single Create call), there's no benefit to parallelism
	// here, and serial output is easier to read in the CLI.
	submitted := make([]*perfTestRun, 0, len(candidates))
	for _, qpu := range candidates {
		co := *o
		// Point the BackendSelector at this specific QPU.  Using the
		// QPU's K8s name is unambiguous; the controller resolves it
		// to the provider-native backend name via QPU.spec.backendName.
		co.backendName = qpu.Name
		co.provider = qpu.Spec.Provider
		co.performanceTest = false // sub-runs are normal Circuits

		c, err := buildCircuit(file, format, body, &co)
		if err != nil {
			fmt.Print(render.Fail(
				fmt.Sprintf("build circuit for %s", qpu.Name), err.Error()))
			continue
		}
		if err := cli.Create(ctx, c); err != nil {
			fmt.Print(render.Fail(
				fmt.Sprintf("submit to %s", qpu.Name), err.Error()))
			continue
		}
		submitted = append(submitted, &perfTestRun{
			qpu:       qpu,
			circuit:   c,
			submitted: time.Now(),
		})
		fmt.Print(render.Step("submitted",
			fmt.Sprintf("%s/%s → %s", c.Namespace, c.Name, qpu.Name)))
	}
	if len(submitted) == 0 {
		fmt.Print(render.Fail("performance-test", "all submissions failed; see errors above"))
		return errors.New("no circuits submitted")
	}

	// Block on all of them reaching terminal.  One polling goroutine
	// per Circuit because the wait windows are independent (Aer is
	// near-instant, fake-* can take a few seconds, real hardware
	// queues for minutes).  A shared context limits the total wait.
	waitAllTerminal(ctx, cli, submitted, o)

	// Render the comparison table and the Grafana deep link.
	fmt.Println()
	fmt.Print(render.Section(
		fmt.Sprintf("results · %s", o.experiment),
		renderPerfTestTable(submitted),
	))
	printGrafanaLink(o.algorithm, o.experiment)
	return nil
}

// perfTestRun bundles the per-candidate state the orchestrator
// tracks: the candidate QPU, the submitted Circuit (mutated as the
// goroutine refreshes it from the API), and timing.  A separate
// struct rather than parallel slices keeps the comparison-table
// renderer trivial.
type perfTestRun struct {
	qpu       qccv1alpha1.QPU
	circuit   *qccv1alpha1.Circuit
	submitted time.Time
	completed time.Time
	terminal  qccv1alpha1.CircuitPhase // PhaseSucceeded | PhaseFailed | empty (timeout)
	failure   string                   // condition reason on failure; empty on success
}

// validatePerfTestOpts rejects flag combinations that don't make
// sense under --performance-test.  Surfaces the error before any
// network call so the user sees the problem immediately.
func validatePerfTestOpts(o *runOpts) error {
	var conflicts []string
	if o.backendName != "" {
		conflicts = append(conflicts, "--backend")
	}
	if o.provider != "" {
		conflicts = append(conflicts, "--provider")
	}
	if o.detach {
		conflicts = append(conflicts, "--detach")
	}
	if o.selectOnly {
		conflicts = append(conflicts, "--select-only")
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("--performance-test is mutually exclusive with: %s",
			strings.Join(conflicts, ", "))
	}
	return nil
}

// listPerfTestQPUs returns the candidate QPU set.  Filters: only
// QPUs whose `status.availability` is `Available` (otherwise we'd
// queue Circuits against a backend the controller will immediately
// fail), and whose kind matches the include-hardware flag.
//
// Sorted alphabetically by Name so the submission order, table
// rows, and Grafana visual order all line up — useful for thesis
// reproducibility.
func listPerfTestQPUs(ctx context.Context, cli client.Client, includeHardware bool) ([]qccv1alpha1.QPU, error) {
	var list qccv1alpha1.QPUList
	if err := cli.List(ctx, &list); err != nil {
		return nil, err
	}
	out := make([]qccv1alpha1.QPU, 0, len(list.Items))
	for _, q := range list.Items {
		if q.Status.Availability != qccv1alpha1.QPUAvailable {
			continue
		}
		isSim := q.Spec.Kind == qccv1alpha1.BackendKindSimulator
		if !isSim && !includeHardware {
			continue
		}
		out = append(out, q)
	}
	slices.SortFunc(out, func(a, b qccv1alpha1.QPU) int { return cmp.Compare(a.Name, b.Name) })
	return out, nil
}

// waitAllTerminal blocks until every submitted Circuit reaches a
// terminal phase (Succeeded / Failed) or the user-specified
// --timeout elapses for that Circuit.  One goroutine per Circuit;
// they're independent so there's no benefit to a single shared
// polling loop, and the per-Circuit timeout semantics match what
// `qcc run` without --performance-test already does.
//
// Side effect: mutates each `*perfTestRun` in place with the final
// `circuit` snapshot, `completed` timestamp, `terminal` phase, and
// failure reason.  The caller reads these for the comparison table.
func waitAllTerminal(ctx context.Context, cli client.Client, runs []*perfTestRun, o *runOpts) {
	var wg sync.WaitGroup
	for _, r := range runs {
		wg.Add(1)
		go func(r *perfTestRun) {
			defer wg.Done()
			waitOneTerminal(ctx, cli, r, o)
		}(r)
	}
	wg.Wait()
}

// waitOneTerminal is the per-Circuit poll loop.  Independent of the
// `watchToTerminal` helper because we want non-interactive output
// (no spinner, no large result-card render) and per-Circuit completion
// to stream into the parent's progress lines as they arrive.
func waitOneTerminal(ctx context.Context, cli client.Client, r *perfTestRun, o *runOpts) {
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	nn := types.NamespacedName{Name: r.circuit.Name, Namespace: r.circuit.Namespace}
	for {
		select {
		case <-ctx.Done():
			// Timeout — leave terminal phase empty so the renderer
			// shows a clear "TIMEOUT" cell rather than confusing the
			// reader with a stale snapshot.
			return
		case <-time.After(o.pollInterval):
		}

		var c qccv1alpha1.Circuit
		if err := cli.Get(ctx, nn, &c); err != nil {
			// Transient API error — keep polling until the parent
			// context cancels.  Per-Circuit goroutines don't fail
			// the whole perf-test; they just report what they saw.
			continue
		}
		r.circuit = &c
		switch c.Status.Phase {
		case qccv1alpha1.PhaseSucceeded:
			r.completed = time.Now()
			r.terminal = qccv1alpha1.PhaseSucceeded
			fmt.Print(render.Step("completed",
				fmt.Sprintf("%s on %s · %s",
					r.circuit.Name, r.qpu.Name,
					r.completed.Sub(r.submitted).Round(time.Millisecond))))
			return
		case qccv1alpha1.PhaseFailed:
			r.completed = time.Now()
			r.terminal = qccv1alpha1.PhaseFailed
			reason, _ := failureReason(&c)
			r.failure = reason
			fmt.Print(render.Fail(
				fmt.Sprintf("failed · %s on %s", r.circuit.Name, r.qpu.Name),
				reason))
			return
		}
	}
}

// renderPerfTestTable produces the comparison block.  One row per
// candidate, sorted by alphabetical QPU name (the same order they
// were submitted in).  Per-row fields:
//
//   - BACKEND                : QPU's K8s name
//   - PHASE                  : SUCCEEDED / FAILED / TIMEOUT
//   - DEPTH / 1Q / 2Q / TOTAL: post-transpile shape from
//     status.transpile (empty on failure/timeout)
//   - TOP OUTCOMES           : highest-count bitstrings from
//     status.results (top 3 by count)
//   - TIME                   : submitted-to-terminal wall-clock
//   - JOB                    : provider job id when available
//
// Designed for terminal width — the BACKEND and TOP OUTCOMES columns
// are the ones that can grow; everything else is fixed-width.  Long
// bitstrings (e.g. 4-qubit Shor) are kept as-is rather than
// truncated, because that's the data the user wants to see.
func renderPerfTestTable(runs []*perfTestRun) string {
	header := []string{"BACKEND", "PHASE", "DEPTH", "1Q", "2Q", "TOTAL", "TOP OUTCOMES", "TIME", "JOB"}
	rows := make([][]string, 0, len(runs))
	for _, r := range runs {
		phase := strOr(string(r.terminal), "TIMEOUT")
		if r.terminal == qccv1alpha1.PhaseFailed && r.failure != "" {
			phase = "FAILED · " + r.failure
		}

		depth, oneQ, twoQ, total := emDash, emDash, emDash, emDash
		if tm := r.circuit.Status.Transpile; tm != nil {
			depth = fmt.Sprintf("%d", tm.Depth)
			twoQ = fmt.Sprintf("%d", tm.TwoQubitGates)
			total = fmt.Sprintf("%d", tm.TotalGates)
			if oneQv := int64(tm.TotalGates) - int64(tm.TwoQubitGates); oneQv >= 0 {
				oneQ = fmt.Sprintf("%d", oneQv)
			}
		}

		wall := emDash
		if !r.completed.IsZero() {
			wall = r.completed.Sub(r.submitted).Round(time.Millisecond).String()
		}

		rows = append(rows, []string{
			r.qpu.Name,
			phase,
			depth, oneQ, twoQ, total,
			topOutcomes(r.circuit.Status.Results, 3),
			wall,
			strOr(r.circuit.Status.ProviderJobID, emDash),
		})
	}
	return render.Table(header, rows)
}

// topOutcomes returns a compact rendering of the n highest-count
// bitstrings from a measurement-results map, e.g.
// "00:516 11:508 01:7".  Empty string when results is nil (failure /
// timeout / pending) — the caller's table cell shows an emdash
// instead.
func topOutcomes(results map[string]int64, n int) string {
	if len(results) == 0 {
		return emDash
	}
	type kv struct {
		bitstring string
		count     int64
	}
	pairs := make([]kv, 0, len(results))
	for b, c := range results {
		pairs = append(pairs, kv{b, c})
	}
	slices.SortFunc(pairs, func(a, b kv) int {
		// Descending by count: b before a.
		if c := cmp.Compare(b.count, a.count); c != 0 {
			return c
		}
		// Stable secondary sort by bitstring so identical-count
		// outcomes don't shuffle between table renders.
		return cmp.Compare(a.bitstring, b.bitstring)
	})
	if len(pairs) > n {
		pairs = pairs[:n]
	}
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, fmt.Sprintf("%s:%d", p.bitstring, p.count))
	}
	return strings.Join(parts, " ")
}

// printGrafanaLink emits a deep link to the Circuit dashboard with
// the experiment + algorithm filters pre-applied.  Path-only (no
// host) because the user may be running against any cluster — the
// host is whatever Grafana is reachable at locally; the path resolves
// once they paste it after their port-forward / ingress prefix.
func printGrafanaLink(algorithm, experiment string) {
	url := fmt.Sprintf("/d/qcc-circuit?var-algorithm=%s&var-experiment=%s",
		algorithm, experiment)
	fmt.Print(render.Step("dashboard", url))
}

// defaultAlgorithmFromFile derives a sensible `qcc.io/algorithm`
// label value from the source filename when the user didn't pass
// --algorithm explicitly.  Strips extension, lowercases, replaces
// underscores with hyphens — matching the convention used in
// `generateName` so the algorithm label looks like the
// circuit-name prefix.
func defaultAlgorithmFromFile(file string) string {
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	base = strings.ToLower(strings.ReplaceAll(base, "_", "-"))
	if base == "" {
		return "circuit"
	}
	return base
}
