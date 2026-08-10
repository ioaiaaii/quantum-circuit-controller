/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package render

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const histBarWidth = 24

var (
	histBarStyle  = lipgloss.NewStyle().Foreground(phase60)
	histRestStyle = lipgloss.NewStyle().Faint(true)
)

// Table renders a kubectl-style header+rows table.  Header cells are
// rendered in the faint style (subordinate), body cells in the emphasis
// style.  All columns auto-size to fit their widest entry.  No borders,
// no separators — just whitespace alignment, designed to be at home in
// a terminal next to `kubectl get`.
//
// Use this for list views — `qcc get circuits`, `qcc get qpus`.
// For single-resource detail views use KV inside a Card.
func Table(header []string, rows [][]string) string {
	if len(header) == 0 && len(rows) == 0 {
		return ""
	}
	cols := len(header)
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths := make([]int, cols)
	for i, h := range header {
		if len(h) > widths[i] {
			widths[i] = len(h)
		}
	}
	for _, r := range rows {
		for i, c := range r {
			if i < cols && len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}

	var b strings.Builder
	// Header row.
	for i, h := range header {
		pad := widths[i] - len(h) + 2 // 2-space gutter
		b.WriteString(detailStyle.Render(h))
		b.WriteString(strings.Repeat(" ", pad))
	}
	b.WriteByte('\n')
	// Body rows.
	for _, r := range rows {
		for i, c := range r {
			if i >= cols {
				continue
			}
			pad := widths[i] - len(c) + 2
			b.WriteString(emphasisStyle.Render(c))
			b.WriteString(strings.Repeat(" ", pad))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// KV renders a list of (label, value) rows aligned in two dim/bright columns.
// Designed for embedding inside Card.
func KV(rows [][2]string) string {
	if len(rows) == 0 {
		return ""
	}
	keyWidth := 0
	for _, r := range rows {
		if len(r[0]) > keyWidth {
			keyWidth = len(r[0])
		}
	}
	var b strings.Builder
	for _, r := range rows {
		key := lipgloss.NewStyle().Faint(true).Width(keyWidth + 2).Render(r[0])
		b.WriteString(key)
		b.WriteString(emphasisStyle.Render(r[1]))
		b.WriteByte('\n')
	}
	return b.String()
}

// Histogram renders measurement counts as horizontal bars sorted by outcome.
// Each row: outcome  ████░░░░  count.
func Histogram(counts map[string]int64) string {
	if len(counts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(counts))
	var maxCount int64
	for k, v := range counts {
		keys = append(keys, k)
		if v > maxCount {
			maxCount = v
		}
	}
	slices.Sort(keys)

	// Pad outcome labels to a consistent width.
	keyWidth := 0
	for _, k := range keys {
		if len(k) > keyWidth {
			keyWidth = len(k)
		}
	}

	var b strings.Builder
	for _, k := range keys {
		v := counts[k]
		var filled int
		if maxCount > 0 {
			filled = int(float64(v) / float64(maxCount) * float64(histBarWidth))
		}
		if filled > histBarWidth {
			filled = histBarWidth
		}
		bar := histBarStyle.Render(strings.Repeat("█", filled)) +
			histRestStyle.Render(strings.Repeat("░", histBarWidth-filled))

		label := emphasisStyle.Render(padRight(k, keyWidth))
		count := histRestStyle.Render(fmt.Sprintf("%d", v))
		fmt.Fprintf(&b, "%s  %s  %s\n", label, bar, count)
	}
	return b.String()
}

// Section renders titled content as a borderless block: title on its own
// line (rendered in the section style), a blank line, then the body
// indented two spaces, then a trailing blank line.  No box, no border —
// just whitespace.  This replaced the earlier Card wrapper because
// rounded borders ate terminal width and Lipgloss's width math gets
// confused by box-drawing Unicode in nested content (Qiskit text drawer,
// mostly), causing bisected frames.  See QCC-Design-State.md decision
// log for the trade-off rationale.
func Section(title, body string) string {
	var b strings.Builder
	if title != "" {
		b.WriteString(sectionStyle.Render(title))
		b.WriteString("\n\n")
	}
	for line := range strings.SplitSeq(strings.TrimRight(body, "\n"), "\n") {
		if line == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
