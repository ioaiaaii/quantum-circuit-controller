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
	"sigs.k8s.io/controller-runtime/pkg/client"

	qccv1alpha1 "github.com/ioaiaaii/quantum-circuit-controller/api/v1alpha1"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/cli/kubeclient"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/cli/render"
)

type drawOpts struct {
	namespace    string
	keep         bool
	timeout      time.Duration
	pollInterval time.Duration
	kubeconfig   string
}

func newDrawCmd(version string) *cobra.Command {
	o := &drawOpts{}
	cmd := &cobra.Command{
		Use:   "draw <file>",
		Short: "Render a circuit as ASCII art via the executor",
		Long: "Creates a Circuit with mode=draw, waits for the executor to render it, " +
			"and prints the drawing.  Accepts OpenQASM 3 (.qasm) or Qiskit-Python (.py); " +
			"the executor picks the loader.  The Circuit is deleted afterward unless --keep.",
		Example: `  qcc draw bell.qasm
  qcc draw bell.py
  qcc draw bell.qasm --keep
  qcc draw bell.qasm -n my-ns --timeout 30s`,
		Args: argsWithHelp(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			return drawCircuit(cmd.Context(), version, args[0], o)
		},
	}
	cmd.Flags().StringVarP(&o.namespace, "namespace", "n", "default", "Namespace to create the Circuit in")
	cmd.Flags().BoolVar(&o.keep, "keep", false, "Retain the Circuit resource after rendering (deleted by default)")
	cmd.Flags().DurationVar(&o.timeout, "timeout", 60*time.Second, "Max wall-clock time to wait for the drawing")
	cmd.Flags().DurationVar(&o.pollInterval, "poll", 250*time.Millisecond, "Status poll interval")
	cmd.Flags().StringVar(&o.kubeconfig, "kubeconfig", "", "Path to kubeconfig (defaults to KUBECONFIG / ~/.kube/config)")
	return cmd
}

// drawCircuit drives the mode=draw flow.  Mirrored by scheduleCircuit
// in schedule.go; the dupl lint nolint there points back here.
//
//nolint:dupl // intentional symmetry with scheduleCircuit; see schedule.go.
func drawCircuit(ctx context.Context, version, file string, o *drawOpts) error {
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

	circuit := buildDrawCircuit(file, format, body, o)
	if err := cli.Create(ctx, circuit); err != nil {
		fmt.Print(render.Fail("create circuit", err.Error()))
		return err
	}

	final, watchErr := watchForDrawing(ctx, cli, circuit, o)

	// Cleanup: delete by default; --keep retains.  Done regardless of
	// success/failure so the cluster doesn't accumulate ephemeral draw CRs.
	if !o.keep && final != nil {
		if delErr := cli.Delete(ctx, final); delErr != nil && !errors.Is(delErr, context.Canceled) {
			fmt.Print(render.Step("cleanup", "failed: "+delErr.Error()))
		}
	} else if o.keep && final != nil {
		fmt.Print(render.Step("keeping", fmt.Sprintf("%s/%s", final.Namespace, final.Name)))
	}

	return watchErr
}

// loadSourceFile reads a circuit source and infers its format from the file
// extension.  Returns SourceOpenQASM3 for .qasm, SourceQiskit for .py.
// Everything else is rejected — the executor only knows those two loaders.
func loadSourceFile(file string) (qccv1alpha1.SourceFormat, string, error) {
	ext := strings.ToLower(filepath.Ext(file))
	var format qccv1alpha1.SourceFormat
	switch ext {
	case ".qasm":
		format = qccv1alpha1.SourceOpenQASM3
	case ".py":
		format = qccv1alpha1.SourceQiskit
	default:
		return "", "", fmt.Errorf("unsupported file type %q: qcc draw accepts .qasm (OpenQASM 3) or .py (Qiskit)", ext)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return "", "", err
	}
	body := string(b)
	if format == qccv1alpha1.SourceOpenQASM3 && !strings.Contains(body, "OPENQASM") {
		return "", "", fmt.Errorf("%s does not contain an OPENQASM declaration", filepath.Base(file))
	}
	return format, body, nil
}

func buildDrawCircuit(file string, format qccv1alpha1.SourceFormat, body string, o *drawOpts) *qccv1alpha1.Circuit {
	return &qccv1alpha1.Circuit{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateName(file),
			Namespace: o.namespace,
		},
		Spec: qccv1alpha1.CircuitSpec{
			Mode: qccv1alpha1.ModeDraw,
			Source: qccv1alpha1.CircuitSource{
				Format: format,
				Body:   body,
			},
		},
	}
}

// watchForDrawing polls until the Circuit reaches Succeeded or Failed, prints
// the result, and returns the final state (which the caller uses for cleanup).
func watchForDrawing(
	ctx context.Context,
	cli client.Client,
	circuit *qccv1alpha1.Circuit,
	o *drawOpts,
) (*qccv1alpha1.Circuit, error) {
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	nn := types.NamespacedName{Name: circuit.Name, Namespace: circuit.Namespace}
	start := time.Now()

	sp := render.NewSpinner(os.Stdout)
	sp.Start("rendering")
	defer sp.Stop()

	for {
		select {
		case <-ctx.Done():
			sp.FinishFail(fmt.Sprintf("timeout · %s did not render in %s", circuit.Name, o.timeout))
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
			sp.FinishOK("rendered · " + elapsed.String())
			// Drawings can exceed terminal width and use Unicode
			// box-drawing characters whose widths Lipgloss measures
			// incorrectly; render raw to avoid bisected borders.
			drawing, err := readDrawing(ctx, cli, &c)
			if err != nil {
				return &c, err
			}
			fmt.Println()
			fmt.Println(strings.TrimRight(drawing, "\n"))
			fmt.Println()
			return &c, nil
		case qccv1alpha1.PhaseFailed:
			reason, msg := failureReason(&c)
			sp.FinishFail(fmt.Sprintf("%s · %s", reason, msg))
			return &c, fmt.Errorf("circuit %s failed: %s", c.Name, reason)
		}
	}
}

// readDrawing follows status.drawingRef to its ConfigMap and returns the
// rendered ASCII drawing.  Drawings live out-of-band in a ConfigMap rather
// than inline on the Circuit so the Circuit object stays small regardless
// of circuit size (etcd values are bounded; large drawings would otherwise
// blow the per-object limit).  See QCC-API.md §3.7.
func readDrawing(ctx context.Context, cli client.Client, c *qccv1alpha1.Circuit) (string, error) {
	if c.Status.DrawingRef == nil || c.Status.DrawingRef.Name == "" {
		return "", fmt.Errorf("circuit %s reached Succeeded without status.drawingRef set", c.Name)
	}
	return readArtifact(ctx, cli, c.Namespace, c.Status.DrawingRef.Name, qccv1alpha1.ArtifactDataKeyDrawing)
}
