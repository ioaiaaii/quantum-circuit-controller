/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package render

import (
	"bytes"
	"strings"
	"testing"
)

// A bytes.Buffer is not a *os.File so isTerminal returns false; this exercises
// the non-TTY fallback path, which is also the path used in CI.
func TestSpinnerNonTTYStartPrintsStepLine(t *testing.T) {
	var buf bytes.Buffer
	sp := NewSpinner(&buf)
	sp.Start("waiting for selection")
	if !strings.Contains(buf.String(), "▸") {
		t.Errorf("non-TTY Start should emit ▸ step prefix; got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "waiting for selection") {
		t.Errorf("non-TTY Start should emit message; got %q", buf.String())
	}
}

func TestSpinnerNonTTYFinishOK(t *testing.T) {
	var buf bytes.Buffer
	sp := NewSpinner(&buf)
	sp.Start("submitting")
	sp.FinishOK("queued")
	out := buf.String()
	if !strings.Contains(out, "✓") {
		t.Errorf("FinishOK should emit ✓; got %q", out)
	}
	if !strings.Contains(out, "queued") {
		t.Errorf("FinishOK should emit final text; got %q", out)
	}
}

func TestSpinnerNonTTYFinishFail(t *testing.T) {
	var buf bytes.Buffer
	sp := NewSpinner(&buf)
	sp.Start("running")
	sp.FinishFail("timeout")
	if !strings.Contains(buf.String(), "✗") {
		t.Errorf("FinishFail should emit ✗; got %q", buf.String())
	}
}

func TestSpinnerNonTTYUpdateNoop(t *testing.T) {
	var buf bytes.Buffer
	sp := NewSpinner(&buf)
	sp.Start("a")
	before := buf.Len()
	sp.Update("b") // non-TTY: should be silent
	if buf.Len() != before {
		t.Errorf("non-TTY Update should be silent; before=%d after=%d", before, buf.Len())
	}
}
