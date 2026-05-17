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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	qccv1alpha1 "github.com/ioaiaaii/quantum-circuit-controller/api/v1alpha1"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/cli/kubeclient"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/cli/render"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/executor"
)

type scheduleOpts struct {
	namespace    string
	provider     string
	backendName  string
	keep         bool
	timeout      time.Duration
	pollInterval time.Duration
	kubeconfig   string
}

func newScheduleCmd(version string) *cobra.Command {
	o := &scheduleOpts{}
	cmd := &cobra.Command{
		Use:   "schedule <file>",
		Short: "Compute and render the scheduled timeline for a circuit",
		Long: "Creates a Circuit with mode=schedule, waits for the executor to transpile + " +
			"schedule it against the chosen backend (Move 1 picks one if --backend is omitted), " +
			"and prints a per-qubit ASCII timeline.  Times come from the backend's Target — same " +
			"data as Qiskit's timeline_drawer, rendered for the terminal.  The Circuit is deleted " +
			"afterward unless --keep.",
		Example: `  qcc schedule bell-state.qasm --backend fake-brisbane
  qcc schedule shor.py --backend fake-sherbrooke --keep
  qcc schedule bell-state.qasm --provider local --timeout 60s`,
		Args: argsWithHelp(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			return scheduleCircuit(cmd.Context(), version, args[0], o)
		},
	}
	cmd.Flags().StringVarP(&o.namespace, "namespace", "n", "default", "Namespace to create the Circuit in")
	cmd.Flags().StringVar(&o.provider, "provider", "", "Optional provider constraint (e.g. local, ibm)")
	cmd.Flags().StringVar(&o.backendName, "backend", "",
		"Name of a registered QPU (e.g. fake-brisbane); matches either the QPU's metadata.name "+
			"or its spec.backendName")
	cmd.Flags().BoolVar(&o.keep, "keep", false, "Retain the Circuit resource after scheduling (deleted by default)")
	cmd.Flags().DurationVar(&o.timeout, "timeout", 120*time.Second, "Max wall-clock time to wait for the schedule")
	cmd.Flags().DurationVar(&o.pollInterval, "poll", 250*time.Millisecond, "Status poll interval")
	cmd.Flags().StringVar(&o.kubeconfig, "kubeconfig", "", "Path to kubeconfig (defaults to KUBECONFIG / ~/.kube/config)")
	return cmd
}

// scheduleCircuit drives the mode=schedule flow: load source, create
// Circuit, watch, render.  Parallel to drawCircuit in draw.go — the
// shared skeleton is kept duplicated rather than factored because the
// per-mode differences (different watch text, different artifact
// reader, different renderer) are exactly what makes each command
// readable.  Future cleanup can revisit if a third "artifact-mode"
// command lands.
//
//nolint:dupl // intentional symmetry with drawCircuit; see comment above.
func scheduleCircuit(ctx context.Context, version, file string, o *scheduleOpts) error {
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

	circuit := buildScheduleCircuit(file, format, body, o)
	if err := cli.Create(ctx, circuit); err != nil {
		fmt.Print(render.Fail("create circuit", err.Error()))
		return err
	}

	final, watchErr := watchForSchedule(ctx, cli, circuit, o)

	// Cleanup mirrors qcc draw: delete by default unless --keep.
	if !o.keep && final != nil {
		if delErr := cli.Delete(ctx, final); delErr != nil && !errors.Is(delErr, context.Canceled) {
			fmt.Print(render.Step("cleanup", "failed: "+delErr.Error()))
		}
	} else if o.keep && final != nil {
		fmt.Print(render.Step("keeping", fmt.Sprintf("%s/%s", final.Namespace, final.Name)))
	}

	return watchErr
}

func buildScheduleCircuit(
	file string, format qccv1alpha1.SourceFormat, body string, o *scheduleOpts,
) *qccv1alpha1.Circuit {
	c := &qccv1alpha1.Circuit{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateName(file),
			Namespace: o.namespace,
		},
		Spec: qccv1alpha1.CircuitSpec{
			Mode: qccv1alpha1.ModeSchedule,
			Source: qccv1alpha1.CircuitSource{
				Format: format,
				Body:   body,
			},
		},
	}
	if o.provider != "" || o.backendName != "" {
		c.Spec.BackendSelector = &qccv1alpha1.BackendSelector{
			Provider:    o.provider,
			BackendName: o.backendName,
		}
	}
	return c
}

// watchForSchedule mirrors watchForDrawing: polls Circuit status until
// Succeeded or Failed, then reads the artifact and renders.  The Render
// step is the only material difference — schedule artifacts are JSON.
func watchForSchedule(
	ctx context.Context,
	cli client.Client,
	circuit *qccv1alpha1.Circuit,
	o *scheduleOpts,
) (*qccv1alpha1.Circuit, error) {
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}
	start := time.Now()

	sp := render.NewSpinner(os.Stdout)
	sp.Start("scheduling")
	defer sp.Stop()

	for {
		select {
		case <-ctx.Done():
			sp.FinishFail(fmt.Sprintf("timeout · %s did not schedule in %s", circuit.Name, o.timeout))
			return circuit, errors.New("timeout")
		case <-time.After(o.pollInterval):
		}

		var c qccv1alpha1.Circuit
		if err := cli.Get(ctx, nn, &c); err != nil {
			sp.FinishFail("poll failed · " + err.Error())
			return circuit, err
		}

		switch c.Status.Phase {
		case qccv1alpha1.PhaseSucceeded:
			elapsed := time.Since(start).Round(time.Millisecond)
			sp.FinishOK("scheduled · " + elapsed.String())
			result, err := readSchedule(ctx, cli, &c)
			if err != nil {
				return &c, err
			}
			fmt.Println()
			fmt.Println(renderSchedule(&c, result))
			return &c, nil
		case qccv1alpha1.PhaseFailed:
			reason, msg := failureReason(&c)
			sp.FinishFail(fmt.Sprintf("%s · %s", reason, msg))
			return &c, fmt.Errorf("circuit %s failed: %s", c.Name, reason)
		}
	}
}

// readSchedule follows status.scheduleRef to its ConfigMap and returns
// the decoded ScheduleResult.  Same out-of-band-artifact pattern as
// drawings (QCC-API.md §3.7); JSON encoding rather than raw text is the
// only difference.
func readSchedule(ctx context.Context, cli client.Client, c *qccv1alpha1.Circuit) (*executor.ScheduleResult, error) {
	if c.Status.ScheduleRef == nil || c.Status.ScheduleRef.Name == "" {
		return nil, fmt.Errorf("circuit %s reached Succeeded without status.scheduleRef set", c.Name)
	}
	raw, err := readArtifact(ctx, cli, c.Namespace, c.Status.ScheduleRef.Name, qccv1alpha1.ArtifactDataKeySchedule)
	if err != nil {
		return nil, err
	}
	var out executor.ScheduleResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("decode schedule artifact: %w", err)
	}
	return &out, nil
}

// --- Schedule rendering --------------------------------------------------
//
// The renderer's job is to make the µs-scale envelope legible at the
// terminal.  Two views:
//
//   1. A headline summary — total duration in wall-clock units, number
//      of unique qubits touched, longest gate.
//   2. A per-qubit timeline — for each qubit that *does* something, a
//      compact "<op> @ <µs> (<duration>)" list, ordered by start time.
//
// Idle qubits (only `delay` instructions) are dropped — they tell the
// reader nothing past what the headline already gave them.  Deep
// circuits with thousands of gates get summarised rather than dumped;
// the threshold is conservative (12 events per qubit) so the typical
// thesis circuit shows everything and Shor's gets the "first/last few +
// gate-mix" treatment.

const (
	perQubitEventCap = 12
	// opDelay is the scheduled-circuit op name for idle padding.
	// Scheduling emits a delay on every qubit-cycle that isn't
	// otherwise occupied, so they dominate the op count but tell the
	// reader nothing — the renderer skips them.
	opDelay = "delay"
)

// renderSchedule converts a ScheduleResult into the ASCII timeline view
// the user sees after `qcc schedule …` or `qcc get circuit X --schedule`.
// Pure function over the artifact data + Circuit name so it's easy to
// unit-test once the harness is set up (M2).
func renderSchedule(c *qccv1alpha1.Circuit, s *executor.ScheduleResult) string {
	var b strings.Builder
	b.WriteString(scheduleHeadline(c, s))
	b.WriteString("\n\n")
	b.WriteString(section("summary", scheduleSummaryRows(s)))

	timeline := scheduleTimelineLines(s)
	if len(timeline) > 0 {
		b.WriteString("\n")
		b.WriteString("  timeline\n")
		for _, line := range timeline {
			b.WriteString("    ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func scheduleHeadline(c *qccv1alpha1.Circuit, s *executor.ScheduleResult) string {
	wall := formatExecTime(float64(s.TotalDurationDt) * s.DtSeconds)
	return fmt.Sprintf("▸ %s · schedule · on %s · %s scheduled envelope",
		c.Name, s.BackendUsed, wall)
}

func scheduleSummaryRows(s *executor.ScheduleResult) [][2]string {
	wall := float64(s.TotalDurationDt) * s.DtSeconds
	rows := [][2]string{
		{"duration", fmt.Sprintf("%d dt  (%s)", s.TotalDurationDt, formatExecTime(wall))},
		{"cycle time", fmt.Sprintf("dt = %s", formatDt(s.DtSeconds))},
		{"ops", fmt.Sprintf("%d total · %d non-idle", len(s.Ops), countNonIdle(s.Ops))},
	}
	if active := uniqueActiveQubits(s.Ops); len(active) > 0 {
		rows = append(rows, [2]string{"active qubits", formatQubitSet(active, int(s.NumQubits))})
	}
	if longest := longestGate(s.Ops); longest != nil {
		rows = append(rows, [2]string{
			"longest gate",
			fmt.Sprintf("%s on q%v · %s",
				longest.Name, sliceFromUint32(longest.Qubits),
				formatExecTime(float64(longest.DurationDt)*s.DtSeconds)),
		})
	}
	return rows
}

// scheduleTimelineLines builds one line per active qubit, listing the
// non-idle instructions on that qubit in start-time order.  Lines that
// would exceed perQubitEventCap collapse to "first N · … · last M" so
// Shor's still fits in a screen.
func scheduleTimelineLines(s *executor.ScheduleResult) []string {
	// Group ops by primary-qubit so each qubit has its own row.  Multi-
	// qubit ops are attached to *every* qubit they touch — readers
	// expect to see the ECR on both q0 and q1.
	byQubit := map[uint32][]executor.ScheduledOp{}
	for _, op := range s.Ops {
		if op.Name == opDelay {
			continue
		}
		for _, q := range op.Qubits {
			byQubit[q] = append(byQubit[q], op)
		}
	}
	if len(byQubit) == 0 {
		return nil
	}
	// Stable display order: ascending qubit index.
	qubits := make([]uint32, 0, len(byQubit))
	for q := range byQubit {
		qubits = append(qubits, q)
	}
	slices.Sort(qubits)

	// Pre-compute the label width so the time columns align.
	labelWidth := 0
	for _, q := range qubits {
		l := len(fmt.Sprintf("q%d", q))
		if l > labelWidth {
			labelWidth = l
		}
	}

	lines := make([]string, 0, len(qubits))
	for _, q := range qubits {
		ops := byQubit[q]
		sort.SliceStable(ops, func(i, j int) bool { return ops[i].StartDt < ops[j].StartDt })
		events := make([]string, 0, len(ops))
		for _, op := range ops {
			events = append(events, formatTimelineEvent(op, s.DtSeconds))
		}
		if len(events) > perQubitEventCap {
			head := perQubitEventCap / 2
			tail := perQubitEventCap - head
			events = append(
				append([]string{}, events[:head]...),
				append([]string{fmt.Sprintf("… (%d more) …", len(events)-perQubitEventCap)},
					events[len(events)-tail:]...)...,
			)
		}
		label := fmt.Sprintf("q%d", q)
		lines = append(lines, fmt.Sprintf("%-*s  %s",
			labelWidth, label, strings.Join(events, "  ·  ")))
	}
	return lines
}

func formatTimelineEvent(op executor.ScheduledOp, dt float64) string {
	startWall := float64(op.StartDt) * dt
	if op.DurationDt == 0 {
		return fmt.Sprintf("%s @ %s", op.Name, formatExecTime(startWall))
	}
	durWall := float64(op.DurationDt) * dt
	return fmt.Sprintf("%s @ %s (%s)", op.Name, formatExecTime(startWall), formatExecTime(durWall))
}

func countNonIdle(ops []executor.ScheduledOp) int {
	n := 0
	for _, op := range ops {
		if op.Name != opDelay {
			n++
		}
	}
	return n
}

func uniqueActiveQubits(ops []executor.ScheduledOp) []uint32 {
	seen := map[uint32]struct{}{}
	for _, op := range ops {
		if op.Name == opDelay {
			continue
		}
		for _, q := range op.Qubits {
			seen[q] = struct{}{}
		}
	}
	out := make([]uint32, 0, len(seen))
	for q := range seen {
		out = append(out, q)
	}
	slices.Sort(out)
	return out
}

// formatQubitSet condenses a sorted qubit list into a compact display:
// few qubits → spell them all out; many → "<count> of <total> qubits".
// The threshold (5) is the point where listing stops being useful.
func formatQubitSet(qubits []uint32, total int) string {
	if len(qubits) <= 5 {
		labels := make([]string, len(qubits))
		for i, q := range qubits {
			labels[i] = fmt.Sprintf("q%d", q)
		}
		return strings.Join(labels, ", ")
	}
	if total > 0 {
		return fmt.Sprintf("%d of %d qubits", len(qubits), total)
	}
	return fmt.Sprintf("%d qubits", len(qubits))
}

// longestGate returns a pointer to the longest non-idle op (by
// duration_dt).  Nil when every op is zero-duration (degenerate
// schedule).  Ties are broken by start-time order.
func longestGate(ops []executor.ScheduledOp) *executor.ScheduledOp {
	var best *executor.ScheduledOp
	for i := range ops {
		op := &ops[i]
		if op.Name == opDelay || op.DurationDt == 0 {
			continue
		}
		if best == nil || op.DurationDt > best.DurationDt {
			best = op
		}
	}
	return best
}

func sliceFromUint32(in []uint32) []int {
	out := make([]int, len(in))
	for i, v := range in {
		out[i] = int(v)
	}
	return out
}
