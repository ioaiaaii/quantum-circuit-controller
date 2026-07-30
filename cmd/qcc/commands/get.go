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
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	qccv1alpha1 "github.com/ioaiaaii/quantum-circuit-controller/api/v1alpha1"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/cli/kubeclient"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/cli/render"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/executor"
)

type getOpts struct {
	namespace  string
	qasm       bool
	draw       bool
	schedule   bool
	kubeconfig string
	// Algorithm-grouping filters for `qcc get circuits`.  Map to
	// label selectors on the K8s list call.  Empty = no filter.
	algorithm  string
	experiment string
	version    string
}

// emDash is the placeholder rendered in tables / KV rows when a value
// is absent.  Single shared constant to keep visual consistency.
const emDash = "—"

// kindKey is the canonical (singular) form of a resource kind on the CLI.
// kubectl accepts plural and singular for `kubectl get`; we follow suit.
type kindKey string

const (
	kindCircuit kindKey = "circuit"
	kindQPU     kindKey = "qpu"
)

// normalizeKind maps any user-typed form (`circuit`, `circuits`, `qpu`,
// `qpus`, `qp`) to the canonical key the dispatcher reads.  Returns
// empty string for unknown kinds — the caller emits a friendly error.
func normalizeKind(s string) kindKey {
	switch strings.ToLower(s) {
	case string(kindCircuit), "circuits", "c", "circ":
		return kindCircuit
	case "qpu", "qpus", "qp":
		return kindQPU
	}
	return ""
}

func newGetCmd(version string) *cobra.Command {
	o := &getOpts{}
	cmd := &cobra.Command{
		Use:   "get <kind> [name]",
		Short: "Show one or list QCC resources",
		Long: "Inspect Circuit or QPU resources, kubectl-style.\n\n" +
			"Supplying a name shows that resource in detail.  Omitting it lists\n" +
			"all resources of the kind in the namespace.  Plural and singular\n" +
			"forms are interchangeable: `qcc get qpu` and `qcc get qpus` both\n" +
			"list, `qcc get qpu fake-brisbane` shows a single QPU.",
		Example: `  qcc get circuit shor-7p4jp           # one Circuit
  qcc get circuits                     # list Circuits in namespace
  qcc get qpu fake-brisbane            # one QPU
  qcc get qpus                         # list all QPUs
  qcc get circuit shor-7p4jp --qasm    # raw converted QASM (pipe-friendly)
  qcc get circuit bell-draw-xy --draw  # raw ASCII drawing (pipe-friendly)
  qcc get circuit bell-sch-xy --schedule  # rendered ASCII timeline`,
		Args:      argsWithHelp(cobra.MinimumNArgs(1)),
		ValidArgs: []string{string(kindCircuit), "circuits", string(kindQPU), "qpus"},
		RunE: func(cmd *cobra.Command, args []string) error {
			selectors := 0
			if o.qasm {
				selectors++
			}
			if o.draw {
				selectors++
			}
			if o.schedule {
				selectors++
			}
			if selectors > 1 {
				return fmt.Errorf("--qasm, --draw, and --schedule are mutually exclusive")
			}
			kind := normalizeKind(args[0])
			if kind == "" {
				return fmt.Errorf("unknown resource kind %q; expected one of: circuit, circuits, qpu, qpus", args[0])
			}
			var name string
			if len(args) > 1 {
				name = args[1]
			}
			return runGet(cmd.Context(), version, kind, name, o)
		},
	}
	cmd.Flags().StringVarP(&o.namespace, "namespace", "n", "default",
		"Namespace to look in (Circuits only; QPUs are cluster-scoped)")
	cmd.Flags().BoolVar(&o.qasm, "qasm", false,
		"Circuits only — print the converted OpenQASM 3 (pipe-friendly, no banner)")
	cmd.Flags().BoolVar(&o.draw, "draw", false,
		"Circuits only — print the ASCII drawing (pipe-friendly, no banner)")
	cmd.Flags().BoolVar(&o.schedule, "schedule", false,
		"Circuits only — print the scheduled per-instruction timeline (mode=schedule artifact)")
	cmd.Flags().StringVar(&o.kubeconfig, "kubeconfig", "", "Path to kubeconfig (defaults to KUBECONFIG / ~/.kube/config)")
	// Algorithm-grouping filters (Circuit list only; no-op on a
	// single-Circuit get and on QPU operations).  Map directly to
	// K8s label selectors.
	cmd.Flags().StringVar(&o.algorithm, "algorithm", "",
		"Circuits only — filter by qcc.io/algorithm label (e.g. 'vqe-h2')")
	cmd.Flags().StringVar(&o.version, "version", "",
		"Circuits only — filter by qcc.io/algorithm-version label")
	cmd.Flags().StringVar(&o.experiment, "experiment", "",
		"Circuits only — filter by qcc.io/experiment label")
	return cmd
}

func runGet(ctx context.Context, version string, kind kindKey, name string, o *getOpts) error {
	cli, err := kubeclient.New(o.kubeconfig)
	if err != nil {
		fmt.Print(render.Fail("kubeconfig", err.Error()))
		return err
	}

	// Artifact selectors only apply to Circuits.  Reject them on QPUs
	// up-front so the error fires before the K8s round-trip.
	if kind == kindQPU && (o.qasm || o.draw || o.schedule) {
		return fmt.Errorf("--qasm / --draw / --schedule are only valid for circuit resources")
	}

	switch kind {
	case kindCircuit:
		if name == "" {
			return listCircuits(ctx, cli, o)
		}
		return getCircuit(ctx, cli, version, name, o)
	case kindQPU:
		if name == "" {
			return listQPUs(ctx, cli, version)
		}
		return getQPU(ctx, cli, version, name)
	}
	return fmt.Errorf("unhandled kind %q", kind)
}

// --- Circuit: single ----------------------------------------------------

func getCircuit(ctx context.Context, cli client.Client, version, name string, o *getOpts) error {
	nn := types.NamespacedName{Name: name, Namespace: o.namespace}
	var c qccv1alpha1.Circuit
	if err := cli.Get(ctx, nn, &c); err != nil {
		fmt.Print(render.Fail("get circuit", err.Error()))
		return err
	}

	// Pipe-friendly artifact selectors: raw stdout, no banner, no Card.
	if o.qasm {
		return printArtifact(ctx, cli, &c, c.Status.ConvertedRef, "converted QASM", qccv1alpha1.ArtifactDataKeyQASM)
	}
	if o.draw {
		return printArtifact(ctx, cli, &c, c.Status.DrawingRef, "drawing", qccv1alpha1.ArtifactDataKeyDrawing)
	}
	if o.schedule {
		// Schedules are structured (JSON on the wire) so we render the
		// ASCII timeline rather than dumping the artifact.  The renderer
		// lives in schedule.go alongside `qcc schedule`.
		return printScheduleArtifact(ctx, cli, &c)
	}

	// Default view: rich summary + inline drawing (when present).  The
	// drawing is *content* — show it; the converted QASM is pipe-grab
	// material and stays a hint until --qasm is asked.
	var resolvedQPU *qccv1alpha1.QPU
	if c.Status.SelectedQPU != "" {
		var fetched qccv1alpha1.QPU
		if err := cli.Get(ctx, types.NamespacedName{Name: c.Status.SelectedQPU}, &fetched); err == nil {
			resolvedQPU = &fetched
		}
	}

	fmt.Print(render.Banner(version))
	fmt.Print(render.Section(c.Name, buildCircuitSummary(&c, resolvedQPU)))
	if c.Status.DrawingRef != nil && c.Status.DrawingRef.Name != "" {
		drawing, err := readArtifact(ctx, cli, c.Namespace,
			c.Status.DrawingRef.Name, qccv1alpha1.ArtifactDataKeyDrawing)
		if err != nil {
			fmt.Print(render.Fail("drawing", err.Error()))
		} else {
			fmt.Println()
			fmt.Println(strings.TrimRight(drawing, "\n"))
			fmt.Println()
		}
	}
	printArtifactHints(&c)
	return nil
}

// --- Circuit: list -------------------------------------------------------

func listCircuits(ctx context.Context, cli client.Client, o *getOpts) error {
	listOpts := []client.ListOption{client.InNamespace(o.namespace)}
	if sel := circuitLabelSelector(o); len(sel) > 0 {
		listOpts = append(listOpts, sel)
	}

	var list qccv1alpha1.CircuitList
	if err := cli.List(ctx, &list, listOpts...); err != nil {
		fmt.Print(render.Fail("list circuits", err.Error()))
		return err
	}
	if len(list.Items) == 0 {
		_, err := fmt.Fprintln(os.Stdout, "No Circuits in namespace "+o.namespace+listFilterSuffix(o)+".")
		return err
	}

	// JOB column: the provider job ID stamped by the controller after
	// submission.  Surfacing it in the list view lets users correlate
	// Circuit CRs with provider-side dashboards (IBM Quantum console,
	// Braket job list, etc.) without drilling into `qcc get <name>`.
	// Consistent across local Aer (`aer-<uuid>`) and real hardware
	// (provider's native job ID like `cm8h9k3lq7ml6yz`) — both come from
	// the same status.providerJobID field.
	//
	// Algorithm columns (ALGORITHM, VER, RUN): only printed when at
	// least one Circuit in the list carries the corresponding label.
	// Single-shot / no-grouping submissions keep the lean three-column
	// view; algorithm-tagged fleets get the extra correlation columns
	// for free.  Each column is independent — having algorithm
	// without version is fine, no padding columns appear.
	showAlgorithm := anyHasLabel(list.Items, qccv1alpha1.LabelAlgorithm)
	showVersion := anyHasLabel(list.Items, qccv1alpha1.LabelAlgorithmVersion)
	showRunIndex := anyHasLabel(list.Items, qccv1alpha1.LabelRunIndex)

	header := []string{"NAME", "PHASE", "MODE", "QPU"}
	if showAlgorithm {
		header = append(header, "ALGORITHM")
	}
	if showVersion {
		header = append(header, "VER")
	}
	if showRunIndex {
		header = append(header, "RUN")
	}
	header = append(header, "JOB", "AGE")

	rows := make([][]string, 0, len(list.Items))
	for _, c := range list.Items {
		row := []string{
			c.Name,
			strOr(string(c.Status.Phase), emDash),
			string(c.Spec.Mode),
			strOr(c.Status.SelectedQPU, emDash),
		}
		if showAlgorithm {
			row = append(row, strOr(c.Labels[qccv1alpha1.LabelAlgorithm], emDash))
		}
		if showVersion {
			row = append(row, strOr(c.Labels[qccv1alpha1.LabelAlgorithmVersion], emDash))
		}
		if showRunIndex {
			row = append(row, strOr(c.Labels[qccv1alpha1.LabelRunIndex], emDash))
		}
		row = append(row,
			strOr(c.Status.ProviderJobID, emDash),
			humaniseAge(time.Since(c.CreationTimestamp.Time)),
		)
		rows = append(rows, row)
	}
	fmt.Print(render.Table(header, rows))
	return nil
}

// circuitLabelSelector translates the algorithm-grouping filter
// flags into a controller-runtime label-match selector.  Returns
// nil when no filters are set so callers can append unconditionally.
func circuitLabelSelector(o *getOpts) client.MatchingLabels {
	if o.algorithm == "" && o.version == "" && o.experiment == "" {
		return nil
	}
	sel := client.MatchingLabels{}
	if o.algorithm != "" {
		sel[qccv1alpha1.LabelAlgorithm] = o.algorithm
	}
	if o.version != "" {
		sel[qccv1alpha1.LabelAlgorithmVersion] = o.version
	}
	if o.experiment != "" {
		sel[qccv1alpha1.LabelExperiment] = o.experiment
	}
	return sel
}

// listFilterSuffix renders a human-readable suffix for the empty-
// list message when filters are active, so users see *why* their
// list is empty.  Returns "" when no filters are set.
func listFilterSuffix(o *getOpts) string {
	parts := []string{}
	if o.algorithm != "" {
		parts = append(parts, "algorithm="+o.algorithm)
	}
	if o.version != "" {
		parts = append(parts, "version="+o.version)
	}
	if o.experiment != "" {
		parts = append(parts, "experiment="+o.experiment)
	}
	if len(parts) == 0 {
		return ""
	}
	return " matching " + strings.Join(parts, ", ")
}

// anyHasLabel reports whether at least one Circuit in the list
// carries a non-empty value for the given label key.  Used to drive
// conditional rendering of algorithm columns — the table stays lean
// when nothing in the fleet is algorithm-tagged.
func anyHasLabel(items []qccv1alpha1.Circuit, key string) bool {
	for i := range items {
		if items[i].Labels[key] != "" {
			return true
		}
	}
	return false
}

// --- QPU: single ---------------------------------------------------------

func getQPU(ctx context.Context, cli client.Client, version, name string) error {
	var qpu qccv1alpha1.QPU
	if err := cli.Get(ctx, types.NamespacedName{Name: name}, &qpu); err != nil {
		fmt.Print(render.Fail("get qpu", err.Error()))
		return err
	}
	fmt.Print(render.Banner(version))
	fmt.Print(render.Section(qpu.Name, buildQPUSummary(&qpu)))
	return nil
}

// --- QPU: list -----------------------------------------------------------

func listQPUs(ctx context.Context, cli client.Client, _ string) error {
	var list qccv1alpha1.QPUList
	if err := cli.List(ctx, &list); err != nil {
		fmt.Print(render.Fail("list qpus", err.Error()))
		return err
	}
	if len(list.Items) == 0 {
		_, err := fmt.Fprintln(os.Stdout, "No QPUs registered.")
		return err
	}

	// The list view's job is *cross-backend comparison*.  The columns
	// were chosen by walking the fake-* set side-by-side and asking what
	// a thesis reader would compare:
	//
	//   - PROCESSOR makes the chip family explicit (Eagle r3 vs Heron r2)
	//     instead of forcing the reader to infer it from dt and basis
	//     gates.
	//   - 2Q ERR is the dominant gate-error contributor on real hardware
	//     and the cleanest single-number quality metric.
	//   - T1 is the coherence-budget headline.
	//   - DT grounds exec-time estimates in cycle-period units.
	//   - CALIBRATED replaces creation-AGE; freshness of the calibration
	//     snapshot is the relevant temporal axis, not when the CR was
	//     created.
	//
	// PROVIDER and KIND were dropped — in the thesis-scale fake-* set
	// they're degenerate (all "local" / "simulator") and a `qcc get qpu
	// <name>` detail still shows them.
	header := []string{"NAME", "AVAILABLE", "PROCESSOR", "QUBITS", "2Q ERR", "T1", "DT", "CALIBRATED"}
	rows := make([][]string, 0, len(list.Items))
	for _, q := range list.Items {
		rows = append(rows, []string{
			q.Name,
			strOr(string(q.Status.Availability), emDash),
			strOr(formatProcessor(q.Status.Processor), emDash),
			qpuListQubits(q),
			qpuList2QError(q),
			qpuListT1(q),
			qpuListDt(q),
			qpuListCalibrated(q),
		})
	}
	fmt.Print(render.Table(header, rows))
	return nil
}

// --- listQPUs column formatters ----------------------------------------
//
// Pulled out so each cell's "what does empty mean" rule lives next to
// the cell, not buried in the loop above.  Every helper returns the
// em-dash sentinel for missing data so the table reads as "this
// backend doesn't report X" rather than "this column is broken".

func qpuListQubits(q qccv1alpha1.QPU) string {
	if eff := q.EffectiveQubits(); eff > 0 {
		return fmt.Sprintf("%d", eff)
	}
	return emDash
}

func qpuList2QError(q qccv1alpha1.QPU) string {
	if em := q.Status.ErrorMedians; em != nil && em.TwoQubit > 0 {
		return formatError(em.TwoQubit)
	}
	return emDash
}

func qpuListT1(q qccv1alpha1.QPU) string {
	if c := q.Status.CoherenceMedians; c != nil && c.T1Micros > 0 {
		return formatMicros(c.T1Micros)
	}
	return emDash
}

func qpuListDt(q qccv1alpha1.QPU) string {
	if dt := q.Status.DtSeconds; dt > 0 {
		return formatDt(dt)
	}
	return emDash
}

func qpuListCalibrated(q qccv1alpha1.QPU) string {
	if q.Status.LastCalibrationTime == nil {
		return emDash
	}
	return q.Status.LastCalibrationTime.Format("2006-01-02")
}

// --- Artifact helpers ----------------------------------------------------

func printArtifact(
	ctx context.Context,
	cli client.Client,
	c *qccv1alpha1.Circuit,
	ref *qccv1alpha1.ArtifactRef,
	humanName, dataKey string,
) error {
	if ref == nil || ref.Name == "" {
		return fmt.Errorf("%s", noArtifactMessage(c, humanName))
	}
	payload, err := readArtifact(ctx, cli, c.Namespace, ref.Name, dataKey)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, strings.TrimRight(payload, "\n"))
	return err
}

func noArtifactMessage(c *qccv1alpha1.Circuit, humanName string) string {
	base := fmt.Sprintf("circuit %s/%s has no %s artifact", c.Namespace, c.Name, humanName)
	switch humanName {
	case "drawing":
		if c.Spec.Mode != qccv1alpha1.ModeDraw {
			return fmt.Sprintf(
				"%s — drawings are only produced by mode=draw, and this circuit's mode is %q. "+
					"Render it with `qcc draw <source>`.",
				base, c.Spec.Mode)
		}
	case "converted QASM":
		if c.Spec.Source.Format != qccv1alpha1.SourceQiskit {
			return fmt.Sprintf(
				"%s — converted QASM is only produced when source.format=qiskit, and this "+
					"circuit's format is %q. The original source is already QASM 3.",
				base, c.Spec.Source.Format)
		}
	case "schedule":
		if c.Spec.Mode != qccv1alpha1.ModeSchedule {
			return fmt.Sprintf(
				"%s — schedules are only produced by mode=schedule, and this circuit's "+
					"mode is %q. Render one with `qcc schedule <source> --backend <name>`.",
				base, c.Spec.Mode)
		}
	}
	return base
}

// printScheduleArtifact decodes the JSON schedule from the
// status.scheduleRef ConfigMap and prints the ASCII timeline via the
// same renderer `qcc schedule` uses.  Unlike printArtifact this doesn't
// dump raw bytes — schedules are structured, the renderer is the
// useful artifact.
func printScheduleArtifact(
	ctx context.Context,
	cli client.Client,
	c *qccv1alpha1.Circuit,
) error {
	if c.Status.ScheduleRef == nil || c.Status.ScheduleRef.Name == "" {
		return fmt.Errorf("%s", noArtifactMessage(c, "schedule"))
	}
	raw, err := readArtifact(ctx, cli, c.Namespace, c.Status.ScheduleRef.Name, qccv1alpha1.ArtifactDataKeySchedule)
	if err != nil {
		return err
	}
	var sched executor.ScheduleResult
	if err := json.Unmarshal([]byte(raw), &sched); err != nil {
		return fmt.Errorf("decode schedule artifact: %w", err)
	}
	fmt.Println(renderSchedule(c, &sched))
	return nil
}

func readArtifact(
	ctx context.Context,
	cli client.Client,
	namespace, name, dataKey string,
) (string, error) {
	var cm corev1.ConfigMap
	nn := types.NamespacedName{Name: name, Namespace: namespace}
	if err := cli.Get(ctx, nn, &cm); err != nil {
		return "", fmt.Errorf("fetch ConfigMap %s/%s: %w", namespace, name, err)
	}
	payload, ok := cm.Data[dataKey]
	if !ok {
		return "", fmt.Errorf("ConfigMap %s/%s missing data[%q]", namespace, name, dataKey)
	}
	return payload, nil
}

// --- Circuit summary rendering ------------------------------------------
//
// Single render path for Circuit detail views — used by both
// `qcc run` (post-completion summary) and `qcc get circuit <name>`.
// The output is organised as scientific-paper style: a one-line
// headline takeaway, followed by titled sections grouping related
// rows.  Section headers replace the old single-Card layout because
// thesis-reader scanability beats engineer-log density.
//
// The shape of each section maps to a Ch1 motivation finding:
//   - SETUP     → reproducibility surface (shots, source, provenance)
//   - BACKEND   → calibration provenance (Finding #4, calibration drift)
//   - CIRCUIT   → transpile invisibility (Finding #1) + budget signal
//   - RESULTS   → empirical evidence + wall time
//
// What's *not* here yet, M2 work flagged at the relevant call sites:
//   - layout fidelity / qubit assignment (Finding #4)
//   - wall-time breakdown queue/transpile/exec (Finding #2 — 6-OOM gap)

func buildCircuitSummary(c *qccv1alpha1.Circuit, qpu *qccv1alpha1.QPU) string {
	var b strings.Builder

	// Headline takeaway — one line that a thesis figure caption can
	// quote directly.  Built from the strongest signal we have:
	//   - phase + error-exposure regime when execution is done
	//   - phase only when mid-flight or failed
	b.WriteString(buildHeadline(c, qpu))
	b.WriteString("\n\n")

	// SETUP — what the user submitted (or what the controller resolved).
	setup := buildSetupRows(c)
	if len(setup) > 0 {
		b.WriteString(section("setup", setup))
	}

	// BACKEND — calibration provenance.  Only when a QPU was resolved
	// (post-selection) and at least one of its probed fields is present.
	if qpu != nil {
		backend := buildBackendRows(c, qpu)
		if len(backend) > 0 {
			b.WriteString("\n")
			b.WriteString(section("backend", backend))
		}
	}

	// CIRCUIT — transpile shape + the budget-signal rows.  Skipped when
	// nothing transpilation-related exists yet (mode=draw never
	// transpiles; failed circuits may not have reached it).
	circuit := buildCircuitRows(c, qpu)
	if len(circuit) > 0 {
		b.WriteString("\n")
		b.WriteString(section("circuit", circuit))
	}

	// RESULTS — the histogram + failure-explanation when applicable.
	if len(c.Status.Results) > 0 {
		b.WriteString("\nresults\n\n")
		b.WriteString(indent(render.Histogram(c.Status.Results)))
	} else if c.Status.Phase == qccv1alpha1.PhaseFailed {
		reason, msg := failureReason(c)
		failRows := [][2]string{{"reason", reason}}
		if msg != "" {
			failRows = append(failRows, [2]string{"message", msg})
		}
		b.WriteString("\n")
		b.WriteString(section("failure", failRows))
	}

	return b.String()
}

// buildHeadline produces the one-line takeaway shown above the sections.
// Wording is chosen so it stands alone as a figure caption fragment in
// the thesis manuscript: "Bell-state-X · run on fake-brisbane · signal
// preserved (error exposure 0.01)".
func buildHeadline(c *qccv1alpha1.Circuit, qpu *qccv1alpha1.QPU) string {
	phaseSymbol := phaseSymbolFor(c.Status.Phase)
	verb := fmt.Sprintf("%s · ", c.Spec.Mode)
	backendStr := ""
	if c.Status.SelectedQPU != "" {
		backendStr = "on " + c.Status.SelectedQPU + " · "
	}

	switch c.Status.Phase {
	case qccv1alpha1.PhaseSucceeded:
		head := fmt.Sprintf("%s %s · %s%s", phaseSymbol, c.Name, verb, backendStr)
		if exp, ok := errorExposure(c, qpu); ok {
			return head + verdictFrom(exp)
		}
		return strings.TrimRight(head, " ·") + " succeeded"
	case qccv1alpha1.PhaseFailed:
		reason, _ := failureReason(c)
		return fmt.Sprintf("%s %s · %s%sfailed (%s)", phaseSymbol, c.Name, verb, backendStr, reason)
	default:
		return fmt.Sprintf("%s %s · %s%sphase=%s",
			phaseSymbol, c.Name, verb, backendStr, c.Status.Phase)
	}
}

// verdictFrom turns the error-exposure number into a thesis-citable
// short verdict — the one phrase a figure caption can lead with.
func verdictFrom(exp float64) string {
	switch {
	case exp >= 5:
		return fmt.Sprintf("signal expected lost (error exposure ≈ %s)", formatExpectedError(exp))
	case exp >= 1:
		return fmt.Sprintf("degraded signal (error exposure ≈ %s)", formatExpectedError(exp))
	case exp >= 0.1:
		return fmt.Sprintf("noisy signal (error exposure ≈ %s)", formatExpectedError(exp))
	default:
		return fmt.Sprintf("signal preserved (error exposure ≈ %s)", formatExpectedError(exp))
	}
}

func phaseSymbolFor(p qccv1alpha1.CircuitPhase) string {
	switch p {
	case qccv1alpha1.PhaseSucceeded:
		return "✓"
	case qccv1alpha1.PhaseFailed:
		return "✗"
	default:
		return "▸"
	}
}

func buildSetupRows(c *qccv1alpha1.Circuit) [][2]string {
	rows := [][2]string{
		{"phase", strOr(string(c.Status.Phase), emDash)},
		{"source", string(c.Spec.Source.Format)},
	}
	if c.Spec.Shots > 0 && c.Spec.Mode == qccv1alpha1.ModeRun {
		rows = append(rows, [2]string{"shots", fmt.Sprintf("%d", c.Spec.Shots)})
	}
	// JOB row: the provider's job ID — `aer-<uuid>` for in-process Aer,
	// the vendor's native ID for real-hardware adapters.  Same field
	// across providers (status.providerJobID), same label here, so users
	// looking at fake- vs. ibm- circuits see consistent output.  The
	// circuit name itself is already in the section header so we don't
	// repeat it here.
	if c.Status.ProviderJobID != "" {
		rows = append(rows, [2]string{"job", c.Status.ProviderJobID})
	}
	return rows
}

func buildBackendRows(c *qccv1alpha1.Circuit, qpu *qccv1alpha1.QPU) [][2]string {
	rows := [][2]string{
		{"name", formatBackendCell(c.Status.SelectedQPU, qpu)},
	}
	// Processor identity sits right under the name because the family
	// label ("Eagle r3", "Heron r2") is what readers want to know about
	// the chip; the calibration / gate-error rows below describe what
	// that chip is doing today.
	if proc := formatProcessor(qpu.Status.Processor); proc != "" {
		rows = append(rows, [2]string{"processor", proc})
	}
	if qpu.Status.LastCalibrationTime != nil {
		rows = append(rows, [2]string{
			"calibrated",
			formatCalibrationTime(qpu.Status.LastCalibrationTime.Time),
		})
	}
	if em := qpu.Status.ErrorMedians; em != nil {
		rows = append(rows, [2]string{
			"gate errors",
			fmt.Sprintf("1Q %s · 2Q %s · readout %s",
				formatError(em.SingleQubit),
				formatError(em.TwoQubit),
				formatError(em.Readout)),
		})
	}
	if cm := qpu.Status.CoherenceMedians; cm != nil {
		rows = append(rows, [2]string{
			"coherence",
			fmt.Sprintf("T1 %s · T2 %s",
				formatMicros(cm.T1Micros), formatMicros(cm.T2Micros)),
		})
	}
	// Cycle time grounds the exec-time row in the `circuit` section: every
	// gate duration is an integer multiple of dt, so seeing 0.222 ns next
	// to a 1.89 µs exec-time estimate makes the math reconstructible.
	if dt := qpu.Status.DtSeconds; dt > 0 {
		rows = append(rows, [2]string{
			"cycle time",
			fmt.Sprintf("dt = %s", formatDt(dt)),
		})
	}
	return rows
}

func buildCircuitRows(c *qccv1alpha1.Circuit, qpu *qccv1alpha1.QPU) [][2]string {
	var rows [][2]string
	if t := c.Status.Transpile; t != nil {
		rows = append(rows, [2]string{
			"transpiled",
			fmt.Sprintf("depth %d · %d gates · %d 2Q", t.Depth, t.TotalGates, t.TwoQubitGates),
		})
	}
	if exec, ok := estimatedExecTime(c, qpu); ok {
		rows = append(rows, [2]string{
			"exec time",
			fmt.Sprintf("~%s (critical-path estimate)", formatExecTime(exec)),
		})
	}
	if exp, ok := errorExposure(c, qpu); ok {
		rows = append(rows, [2]string{
			"error exposure",
			fmt.Sprintf("≈ %s events/shot %s", formatExpectedError(exp), exposureRegime(exp)),
		})
		rows = append(rows, [2]string{
			"fidelity bound",
			fmt.Sprintf("P(no gate error) ≈ %s  (excludes readout & coherence)", formatSurvival(exp)),
		})
	}
	return rows
}

// section renders one labelled group: a dim title, then the KV rows
// indented two spaces.  Mirrors the borderless render.Section but
// scoped to one logical group so multiple sections can stack with
// a blank line between them.
func section(title string, rows [][2]string) string {
	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(indent(render.KV(rows)))
	return b.String()
}

// indent prepends two spaces to every non-empty line of s.  Keeps the
// section bodies visually subordinate to their headers.
func indent(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for line := range strings.SplitSeq(strings.TrimRight(s, "\n"), "\n") {
		if line == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// estimatedExecTime returns a critical-path estimate of the transpiled
// circuit's wall-time on the resolved backend, in seconds.  Formula:
//
//	exec_time ≈ depth × max(1Q_duration, 2Q_duration)
//
// This is a lower bound because:
//   - it assumes every "moment" (layer of the depth-N circuit) takes
//     as long as the *slowest* gate type — true when 2Q gates appear
//     in most moments (Shor's-style), too generous for 1Q-heavy moments
//   - it ignores measurement duration (typically 1-2 µs each) and the
//     synchronisation delays Ch1 cites as dominant in the schedule
//   - it ignores classical-control overhead (the 6-OOM-gap from Ch1)
//
// Compare exec_time to wall-clock duration (already shown via the
// spinner): the gap between them is the "where did the time go?"
// question Ch1 §motivation makes central.
func estimatedExecTime(c *qccv1alpha1.Circuit, qpu *qccv1alpha1.QPU) (float64, bool) {
	if c.Status.Transpile == nil || qpu == nil || qpu.Status.InstructionDurationMedians == nil {
		return 0, false
	}
	durs := qpu.Status.InstructionDurationMedians
	if durs.SingleQubitSeconds == 0 && durs.TwoQubitSeconds == 0 {
		return 0, false
	}
	perMoment := durs.SingleQubitSeconds
	if durs.TwoQubitSeconds > perMoment {
		perMoment = durs.TwoQubitSeconds
	}
	return float64(c.Status.Transpile.Depth) * perMoment, true
}

// formatExecTime renders a duration in the natural unit for its
// magnitude — nanoseconds, microseconds, milliseconds, seconds.
func formatExecTime(s float64) string {
	switch {
	case s >= 1:
		return fmt.Sprintf("%.2f s", s)
	case s >= 1e-3:
		return fmt.Sprintf("%.2f ms", s*1e3)
	case s >= 1e-6:
		return fmt.Sprintf("%.2f µs", s*1e6)
	default:
		return fmt.Sprintf("%.0f ns", s*1e9)
	}
}

// errorExposure estimates the *number of expected gate-error events
// per shot* from the executor-reported transpile metrics and the
// backend's per-instruction error medians.  Returns (value, true)
// when both inputs are present; (0, false) otherwise.
//
// Formula:  exposure ≈ 1Q_count × e_1Q  +  2Q_count × e_2Q
//
// What this *is*:  a first-order linear sum of expected error events.
// Useful as a regime indicator — ≪1 means the gate-error budget is
// safe, ≫1 means at least one error per shot is essentially certain.
//
// What this is *not*:
//   - NOT a probability.  For exposure > 1 the linear sum has lost its
//     meaning as a per-shot success rate; the Poisson approximation
//     P(no error) ≈ exp(-exposure) is what's reported alongside.
//   - NOT a fidelity.  Real fidelity needs per-qubit error data, the
//     specific physical-qubit assignment from layout, coherence
//     decay over the circuit's runtime, and readout error.  All of
//     those are partial today (M2 work).
//
// What's missing from the sum:
//   - Readout error (≈ readout_err × num_measurements).  For shallow
//     circuits this often dominates; for deep circuits it's <10%.
//   - Coherence decay.  When total exec time approaches T1 or T2,
//     decoherence becomes the dominant error source, not gate errors.
//   - The actual qubit assignment.  We use median errors; a bad layout
//     touching a noisy edge can be 2-5× worse than the median.
//
// Used by Move 5 (selection scoring) in M2 as a fast-reject signal
// — refuse matches whose exposure is severe before submission.  See
// QCC-API.md §4.5 for the full caveat list.  See Ch1 §motivation for
// the Brisbane-vs-Sherbrooke finding this metric makes predictable.
func errorExposure(c *qccv1alpha1.Circuit, qpu *qccv1alpha1.QPU) (float64, bool) {
	if c.Status.Transpile == nil || qpu == nil || qpu.Status.ErrorMedians == nil {
		return 0, false
	}
	t := c.Status.Transpile
	em := qpu.Status.ErrorMedians
	twoQContrib := float64(t.TwoQubitGates) * em.TwoQubit
	// 1Q gate count approximated as (total - 2Q).  Includes measurements;
	// for thesis-scale circuits the bias from that is <5%.
	oneQCount := max(int64(t.TotalGates)-int64(t.TwoQubitGates), 0)
	oneQContrib := float64(oneQCount) * em.SingleQubit
	return twoQContrib + oneQContrib, true
}

// formatExpectedError renders the exposure count with significant-figure
// precision that scales with magnitude — "0.05" for clean Bell, "16"
// for Shor's-on-Brisbane.
func formatExpectedError(e float64) string {
	switch {
	case e >= 10:
		return fmt.Sprintf("%.0f", e)
	case e >= 1:
		return fmt.Sprintf("%.1f", e)
	default:
		return fmt.Sprintf("%.2g", e)
	}
}

// exposureRegime annotates the error-exposure number with the regime
// it falls in.  Wording reflects that the metric covers only gate
// errors; readout and coherence are *not* in the budget the regime
// labels describe.
func exposureRegime(e float64) string {
	switch {
	case e >= 5:
		return "(severe — gate-error budget exceeded)"
	case e >= 1:
		return "(approaching gate-error budget)"
	case e >= 0.1:
		return "(noticeable gate-level noise)"
	default:
		return "(within gate-error budget)"
	}
}

// formatSurvival renders the Poisson-approximation survival probability
// P(no error) ≈ exp(-exposure).  Switches to scientific notation when
// the value would drown in trailing zeros (Shor's-on-Brisbane: 10⁻⁷),
// keeps a decimal form for Bell-on-Brisbane (~0.99).
func formatSurvival(exposure float64) string {
	p := math.Exp(-exposure)
	if p >= 0.01 {
		return fmt.Sprintf("%.2f", p)
	}
	return fmt.Sprintf("%.1e", p)
}

// formatProcessor renders the chip-generation label as "Eagle r3" or
// "Falcon r4 (T)".  Returns "" when the QPU has no processor metadata
// (generic Aer) so callers can skip the row entirely.
func formatProcessor(p *qccv1alpha1.QPUProcessor) string {
	if p == nil || p.Family == "" {
		return ""
	}
	label := p.Family
	if p.Revision != "" {
		label = fmt.Sprintf("%s r%s", label, p.Revision)
	}
	if p.Segment != "" {
		label = fmt.Sprintf("%s (%s)", label, p.Segment)
	}
	return label
}

// formatDt renders a control-electronics cycle period (seconds) in the
// natural unit: ps for sub-ns dt (Sherbrooke: 0.222 ns reads as
// "222 ps"), ns for everything else.  Two significant figures across
// the range we care about (10 ps – 10 ns).
func formatDt(s float64) string {
	switch {
	case s < 1e-9:
		return fmt.Sprintf("%.0f ps", s*1e12)
	case s < 10e-9:
		return fmt.Sprintf("%.2g ns", s*1e9)
	default:
		return fmt.Sprintf("%.1f ns", s*1e9)
	}
}

func formatBackendCell(name string, qpu *qccv1alpha1.QPU) string {
	if qpu == nil {
		return name
	}
	parts := []string{}
	if q := qpu.EffectiveQubits(); q > 0 {
		parts = append(parts, fmt.Sprintf("%dq", q))
	}
	if qpu.Status.CouplingEdges > 0 {
		parts = append(parts, fmt.Sprintf("%d edges", qpu.Status.CouplingEdges))
	}
	if n := len(qpu.Status.BasisGates); n > 0 {
		parts = append(parts, fmt.Sprintf("%d basis gates", n))
	}
	if len(parts) == 0 {
		return name
	}
	return fmt.Sprintf("%s  (%s)", name, strings.Join(parts, " · "))
}

func printArtifactHints(c *qccv1alpha1.Circuit) {
	if c.Status.ConvertedRef != nil && c.Status.ConvertedRef.Name != "" {
		fmt.Print(render.Step("qasm", fmt.Sprintf("qcc get circuit %s --qasm", c.Name)))
	}
	if c.Status.ScheduleRef != nil && c.Status.ScheduleRef.Name != "" {
		fmt.Print(render.Step("schedule", fmt.Sprintf("qcc get circuit %s --schedule", c.Name)))
	}
}

// --- QPU summary rendering ----------------------------------------------

// buildQPUSummary renders a QPU's full characterisation in the same
// sectioned shape as buildCircuitSummary: a one-line headline followed
// by identity / calibration / gate-errors / coherence / timing
// sections.  Every section is conditional — generic Aer (no processor
// type, no error medians, no coherence, no dt) reduces to a one-row
// identity block plus the headline, and nothing else.
func buildQPUSummary(q *qccv1alpha1.QPU) string {
	var b strings.Builder
	b.WriteString(buildQPUHeadline(q))
	b.WriteString("\n\n")

	identity := buildQPUIdentityRows(q)
	if len(identity) > 0 {
		b.WriteString(section("identity", identity))
	}

	if cal := buildQPUCalibrationRows(q); len(cal) > 0 {
		b.WriteString("\n")
		b.WriteString(section("calibration", cal))
	}

	if errs := buildQPUGateErrorRows(q); len(errs) > 0 {
		b.WriteString("\n")
		b.WriteString(section("gate errors", errs))
	}

	if coh := buildQPUCoherenceRows(q); len(coh) > 0 {
		b.WriteString("\n")
		b.WriteString(section("coherence", coh))
	}

	if timing := buildQPUTimingRows(q); len(timing) > 0 {
		b.WriteString("\n")
		b.WriteString(section("timing", timing))
	}

	return b.String()
}

// buildQPUHeadline produces a one-line takeaway: "✓ fake-brisbane ·
// 127q Eagle r3 · 144 edges · Available · 15 mo old".  Mirrors the
// circuit headline so the visual rhythm is the same across resource
// kinds.  Fields that the QPU doesn't report (no processor metadata,
// no calibration) drop out cleanly.
func buildQPUHeadline(q *qccv1alpha1.QPU) string {
	symbol := qpuAvailabilitySymbol(q.Status.Availability)
	parts := []string{}
	if eff := q.EffectiveQubits(); eff > 0 {
		if proc := formatProcessor(q.Status.Processor); proc != "" {
			parts = append(parts, fmt.Sprintf("%dq %s", eff, proc))
		} else {
			parts = append(parts, fmt.Sprintf("%dq", eff))
		}
	} else if proc := formatProcessor(q.Status.Processor); proc != "" {
		parts = append(parts, proc)
	}
	if q.Status.CouplingEdges > 0 {
		parts = append(parts, fmt.Sprintf("%d edges", q.Status.CouplingEdges))
	}
	if av := string(q.Status.Availability); av != "" {
		parts = append(parts, av)
	}
	if q.Status.LastCalibrationTime != nil {
		parts = append(parts, fmt.Sprintf("calibrated %s ago",
			humaniseAge(time.Since(q.Status.LastCalibrationTime.Time))))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%s %s", symbol, q.Name)
	}
	return fmt.Sprintf("%s %s · %s", symbol, q.Name, strings.Join(parts, " · "))
}

// qpuAvailabilitySymbol picks the headline glyph from QPUAvailability.
// Available → ✓, Unavailable → ✗, anything else (including Unknown
// or the zero value) → ▸ as a "in flight / no opinion" marker.
func qpuAvailabilitySymbol(a qccv1alpha1.QPUAvailability) string {
	switch a {
	case qccv1alpha1.QPUAvailable:
		return "✓"
	case qccv1alpha1.QPUUnavailable:
		return "✗"
	default:
		return "▸"
	}
}

// buildQPUIdentityRows assembles the "what this resource is" block —
// provider, backend, kind, region, and the qubit / coupling /
// basis-gate stats that describe the chip topologically.
func buildQPUIdentityRows(q *qccv1alpha1.QPU) [][2]string {
	rows := [][2]string{
		{"provider", q.Spec.Provider},
		{"backend", q.EffectiveBackendName()},
		{"kind", string(q.Spec.Kind)},
	}
	if eff := q.EffectiveQubits(); eff > 0 {
		// Surface a spec/status mismatch when both are set — the spec
		// number is user-authored, the status number came from the
		// probe.  Drift between them is worth seeing.
		if q.Status.Qubits > 0 && q.Spec.Qubits > 0 && q.Status.Qubits != q.Spec.Qubits {
			rows = append(rows, [2]string{
				"qubits",
				fmt.Sprintf("%d (probed) ≠ %d (spec)", q.Status.Qubits, q.Spec.Qubits),
			})
		} else {
			rows = append(rows, [2]string{"qubits", fmt.Sprintf("%d", eff)})
		}
	}
	if q.Status.CouplingEdges > 0 {
		rows = append(rows, [2]string{"coupling", fmt.Sprintf("%d edges", q.Status.CouplingEdges)})
	}
	if len(q.Status.BasisGates) > 0 {
		rows = append(rows, [2]string{"basis gates", strings.Join(q.Status.BasisGates, ", ")})
	}
	if q.Spec.Region != "" {
		rows = append(rows, [2]string{"region", q.Spec.Region})
	}
	return rows
}

// buildQPUCalibrationRows surfaces the "when was this measured" block.
// Currently one row (calibrated date + age) — kept as its own section
// so M2 can drop the calibration TTL / next-refresh hints here without
// reshuffling the layout.
func buildQPUCalibrationRows(q *qccv1alpha1.QPU) [][2]string {
	if q.Status.LastCalibrationTime == nil {
		return nil
	}
	return [][2]string{
		{"calibrated", formatCalibrationTime(q.Status.LastCalibrationTime.Time)},
	}
}

// buildQPUGateErrorRows splits the three gate-error medians onto their
// own rows so the magnitudes are easy to scan.  Empty when the backend
// has no error medians (generic Aer).
func buildQPUGateErrorRows(q *qccv1alpha1.QPU) [][2]string {
	em := q.Status.ErrorMedians
	if em == nil {
		return nil
	}
	return [][2]string{
		{"1Q", formatError(em.SingleQubit)},
		{"2Q", formatError(em.TwoQubit)},
		{"readout", formatError(em.Readout)},
	}
}

// buildQPUCoherenceRows surfaces T1 / T2 with parenthetical hints on
// what they mean (the audience is scientists, but the diagnostic
// reading is "is exec time approaching T1 or T2").  Empty when the
// backend reports no coherence data.
func buildQPUCoherenceRows(q *qccv1alpha1.QPU) [][2]string {
	c := q.Status.CoherenceMedians
	if c == nil {
		return nil
	}
	return [][2]string{
		{"T1", fmt.Sprintf("%s  (energy relaxation)", formatMicros(c.T1Micros))},
		{"T2", fmt.Sprintf("%s  (dephasing)", formatMicros(c.T2Micros))},
	}
}

// buildQPUTimingRows is the cycle-time + per-instruction-duration
// block — the data that grounds circuit exec-time estimates.  Empty
// when the backend reports neither dt nor instruction durations.
func buildQPUTimingRows(q *qccv1alpha1.QPU) [][2]string {
	var rows [][2]string
	if dt := q.Status.DtSeconds; dt > 0 {
		rows = append(rows, [2]string{
			"dt",
			fmt.Sprintf("%s  (control-electronics sample period)", formatDt(dt)),
		})
	}
	if d := q.Status.InstructionDurationMedians; d != nil {
		if d.SingleQubitSeconds > 0 {
			rows = append(rows, [2]string{
				"1Q duration",
				fmt.Sprintf("~%s  (median)", formatExecTime(d.SingleQubitSeconds)),
			})
		}
		if d.TwoQubitSeconds > 0 {
			rows = append(rows, [2]string{
				"2Q duration",
				fmt.Sprintf("~%s  (median)", formatExecTime(d.TwoQubitSeconds)),
			})
		}
	}
	return rows
}

// --- Format helpers ------------------------------------------------------

func formatCalibrationTime(t time.Time) string {
	age := time.Since(t)
	return fmt.Sprintf("%s (%s ago)", t.Format("2006-01-02"), humaniseAge(age))
}

func humaniseAge(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.0fh", d.Hours())
	}
	days := d.Hours() / 24
	if days < 60 {
		return fmt.Sprintf("%.0fd", days)
	}
	months := days / 30
	return fmt.Sprintf("%.0f mo", months)
}

func formatError(e float64) string {
	if e == 0 {
		return emDash
	}
	return fmt.Sprintf("%.2e", e)
}

func formatMicros(us float64) string {
	if us == 0 {
		return emDash
	}
	if us >= 1000 {
		return fmt.Sprintf("%.2f ms", us/1000.0)
	}
	return fmt.Sprintf("%.0f µs", us)
}
