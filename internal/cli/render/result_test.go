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
)

// backendKey is the row label these render tests assert on.
const backendKey = "backend"

func TestKVAlignsAndOrdersRows(t *testing.T) {
	out := KV([][2]string{
		{backendKey, "aer_simulator"},
		{"shots", "1024"},
		{"duration", "1.34s"},
	})
	for _, want := range []string{backendKey, "aer_simulator", "shots", "1024", "duration", "1.34s"} {
		if !strings.Contains(out, want) {
			t.Errorf("KV missing %q\n--- got ---\n%s", want, out)
		}
	}
	// Order preserved.
	if i := strings.Index(out, backendKey); i > strings.Index(out, "shots") {
		t.Errorf("KV reordered rows; backend should appear before shots\n%s", out)
	}
}

func TestKVEmpty(t *testing.T) {
	if out := KV(nil); out != "" {
		t.Errorf("expected empty string for nil rows, got %q", out)
	}
}

func TestHistogramBarsAndCounts(t *testing.T) {
	out := Histogram(map[string]int64{"00": 510, "11": 514})
	for _, want := range []string{"00", "11", "510", "514", "█"} {
		if !strings.Contains(out, want) {
			t.Errorf("Histogram missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestHistogramEmpty(t *testing.T) {
	if out := Histogram(nil); out != "" {
		t.Errorf("expected empty string for nil counts, got %q", out)
	}
}

func TestSectionRendersTitleAndBodyWithoutBorder(t *testing.T) {
	out := Section("results", "backend  aer_simulator\nshots    1024")
	for _, want := range []string{"results", backendKey, "1024"} {
		if !strings.Contains(out, want) {
			t.Errorf("Section missing %q\n--- got ---\n%s", want, out)
		}
	}
	// No box-drawing glyphs — Section is the borderless replacement
	// for the old Card; if any rounded-border corner sneaks in, the
	// width-clipping bug Card had is back.
	if strings.ContainsAny(out, "╭╮╰╯│─") {
		t.Errorf("Section should be borderless\n--- got ---\n%s", out)
	}
}

func TestPadRight(t *testing.T) {
	if got := padRight("ab", 5); got != "ab   " {
		t.Errorf("padRight(\"ab\", 5) = %q; want %q", got, "ab   ")
	}
	if got := padRight("abcdef", 3); got != "abcdef" {
		t.Errorf("padRight should not truncate; got %q", got)
	}
}
