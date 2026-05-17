/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package render

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBannerIncludesTitleAndVersion(t *testing.T) {
	out := Banner("v9.9.9")
	for _, want := range []string{"Q", "•", "C", "v9.9.9"} {
		if !strings.Contains(out, want) {
			t.Errorf("Banner output missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestStepRendersArrowAndMessage(t *testing.T) {
	out := Step("compiling bell.qasm", "… 14 ms")
	if !strings.Contains(out, "▸") {
		t.Errorf("Step missing ▸ icon: %q", out)
	}
	if !strings.Contains(out, "compiling bell.qasm") {
		t.Errorf("Step missing message: %q", out)
	}
	if !strings.Contains(out, "14 ms") {
		t.Errorf("Step missing detail: %q", out)
	}
}

func TestOKAndFailIcons(t *testing.T) {
	if !strings.Contains(OK("done", ""), "✓") {
		t.Error("OK missing ✓ icon")
	}
	if !strings.Contains(Fail("nope", ""), "✗") {
		t.Error("Fail missing ✗ icon")
	}
}

func TestHelpForRootCommand(t *testing.T) {
	noop := func(*cobra.Command, []string) {}
	root := &cobra.Command{
		Use:   "qcc",
		Short: "QCC CLI",
		Long:  "Long description here.",
		Run:   noop,
	}
	root.AddCommand(&cobra.Command{Use: "run <file>", Short: "Run a circuit", Run: noop})
	root.AddCommand(&cobra.Command{Use: "version", Short: "Print version", Run: noop})

	out := Help(root, "v1.0")
	for _, want := range []string{
		"v1.0", "USAGE", "COMMANDS", "run", "version", "Long description",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Help missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestHelpForLeafCommandShowsFlagsAndExamples(t *testing.T) {
	cmd := &cobra.Command{
		Use:     "run <file.qasm>",
		Short:   "Submit a circuit",
		Example: "  qcc run bell.qasm\n  qcc run bell.qasm --shots 4096",
		Run:     func(*cobra.Command, []string) {},
	}
	cmd.Flags().Int("shots", 1024, "shots per submission")
	cmd.Flags().Bool("select-only", false, "selectOnly mode")

	out := Help(cmd, "v1.0")
	for _, want := range []string{"FLAGS", "EXAMPLES", "--shots", "--select-only", "qcc run bell.qasm"} {
		if !strings.Contains(out, want) {
			t.Errorf("leaf Help missing %q\n--- got ---\n%s", want, out)
		}
	}
}
