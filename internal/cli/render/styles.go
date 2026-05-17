/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package render contains lipgloss styles and helpers for the qcc CLI.
// All terminal output passes through these helpers so the visual language
// (Q•C•C banner, ▸ step, ✓ ok, ✗ fail) stays consistent across subcommands.
package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	dividerWidth = 60
	flagColWidth = 28
)

// QCC color system — "three inks, three dots".
//
//	phase/60   #0f62fe   primary accent (brand blue)
//	phase/30   #00f790ff   secondary accent (soft blue)
//	ink        #161616   ink on paper
//	night      #0a0a16   deep ink
//	paper      #ffffff   white
//	paper/dim  #f4f4f4   off-white
//
// In a terminal we don't control the background, so:
//   - phase/60 + phase/30 ride as foreground accents on either light or dark.
//   - "ink ↔ paper" is rendered as `AdaptiveColor` so emphasis flips with
//     the user's terminal background.
//   - "dim" is the ANSI Faint attribute rather than a fixed hex, so it
//     reads on every theme.
//
// Warn and fail intentionally stay outside the palette (amber / red) so
// status semantics survive even in monochrome or accessibility modes.
var (
	phase60 = lipgloss.Color("#0f62fe")
	phase30 = lipgloss.Color("#b700ffff")
	emphFG  = lipgloss.AdaptiveColor{Light: "#0a0a16", Dark: "#0f62fe"}
	warnFG  = lipgloss.Color("214")
	failFG  = lipgloss.Color("203")

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(phase60)
	versionStyle  = lipgloss.NewStyle().Faint(true)
	dividerStyle  = lipgloss.NewStyle().Faint(true)
	stepStyle     = lipgloss.NewStyle().Foreground(phase30)
	okStyle       = lipgloss.NewStyle().Foreground(phase60)
	warnStyle     = lipgloss.NewStyle().Foreground(warnFG)
	failStyle     = lipgloss.NewStyle().Foreground(failFG)
	detailStyle   = lipgloss.NewStyle().Faint(false)
	emphasisStyle = lipgloss.NewStyle().Bold(false).Foreground(emphFG)
	sectionStyle  = lipgloss.NewStyle().Bold(false).Foreground(phase60)
	cmdNameStyle  = lipgloss.NewStyle().Foreground(phase60)
	flagNameStyle = lipgloss.NewStyle().Foreground(phase30)
)

// Banner renders the Q•C•C header with the given version.
func Banner(version string) string {
	title := titleStyle.Render("Q•C•C")
	ver := versionStyle.Render(version)
	line := dividerStyle.Render(strings.Repeat("─", dividerWidth))
	return fmt.Sprintf("%s\n%s  %s\n%s\n", line, title, ver, line)
}

// Divider draws a thin separator line.
func Divider() string {
	return dividerStyle.Render(strings.Repeat("─", dividerWidth)) + "\n"
}

// Step renders an in-progress line prefixed by ▸.
func Step(msg, detail string) string {
	return prefixed(stepStyle.Render("▸"), msg, detail)
}

// OK renders a success line prefixed by ✓.
func OK(msg, detail string) string {
	return prefixed(okStyle.Render("✓"), msg, detail)
}

// Warn renders a warning line prefixed by ⚠.
func Warn(msg, detail string) string {
	return prefixed(warnStyle.Render("⚠"), msg, detail)
}

// Fail renders a failure line prefixed by ✗.
func Fail(msg, detail string) string {
	return prefixed(failStyle.Render("✗"), msg, detail)
}

// Emphasis renders msg in the bright foreground style (counts, ids).
func Emphasis(msg string) string {
	return emphasisStyle.Render(msg)
}

// Detail renders msg in the dimmed secondary style.
func Detail(msg string) string {
	return detailStyle.Render(msg)
}

func prefixed(icon, msg, detail string) string {
	if detail == "" {
		return fmt.Sprintf("%s %s\n", icon, msg)
	}
	return fmt.Sprintf("%s %s %s\n", icon, msg, detailStyle.Render(detail))
}

// Help is a drop-in replacement for cobra's default help/usage output that
// uses the same visual language as the rest of the CLI.  Pass it as both
// SetHelpFunc and SetUsageFunc on the root command; cobra propagates to
// subcommands automatically.
func Help(cmd *cobra.Command, version string) string {
	var b strings.Builder
	b.WriteString(Banner(version))

	if cmd.Short != "" {
		b.WriteString("  " + cmd.Short + "\n")
	}
	if cmd.Long != "" && cmd.Long != cmd.Short {
		for line := range strings.SplitSeq(strings.TrimRight(cmd.Long, "\n"), "\n") {
			b.WriteString("  " + detailStyle.Render(line) + "\n")
		}
	}
	b.WriteString("\n")

	b.WriteString(section("USAGE"))
	b.WriteString("    " + cmd.UseLine() + "\n\n")

	if cmd.Example != "" {
		b.WriteString(section("EXAMPLES"))
		for line := range strings.SplitSeq(strings.TrimRight(cmd.Example, "\n"), "\n") {
			b.WriteString("    " + detailStyle.Render(strings.TrimSpace(line)) + "\n")
		}
		b.WriteString("\n")
	}

	if cmd.HasAvailableSubCommands() {
		b.WriteString(section("COMMANDS"))
		for _, sub := range cmd.Commands() {
			if sub.Hidden || !sub.IsAvailableCommand() {
				continue
			}
			b.WriteString(commandRow(sub.Name(), sub.Short))
		}
		b.WriteString("\n")
	}

	if cmd.HasAvailableLocalFlags() {
		b.WriteString(section("FLAGS"))
		cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			b.WriteString(flagRow(f))
		})
		b.WriteString("\n")
	}

	if cmd.HasAvailableSubCommands() {
		hint := fmt.Sprintf("Run \"%s <command> --help\" for command-specific help.", cmd.CommandPath())
		b.WriteString("  " + detailStyle.Render(hint) + "\n")
	}
	return b.String()
}

func section(name string) string {
	return "  " + sectionStyle.Render(name) + "\n"
}

func commandRow(name, desc string) string {
	col := lipgloss.NewStyle().Width(flagColWidth).Render(cmdNameStyle.Render(name))
	return "    " + col + detailStyle.Render(desc) + "\n"
}

func flagRow(f *pflag.Flag) string {
	var head strings.Builder
	if f.Shorthand != "" {
		head.WriteString(flagNameStyle.Render("-" + f.Shorthand + ","))
	} else {
		head.WriteString("   ")
	}
	head.WriteString(" " + flagNameStyle.Render("--"+f.Name))
	if t := f.Value.Type(); t != "bool" {
		head.WriteString(" " + detailStyle.Render(t))
	}

	col := lipgloss.NewStyle().Width(flagColWidth).Render(head.String())

	desc := detailStyle.Render(f.Usage)
	if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "[]" {
		desc += detailStyle.Render(fmt.Sprintf(" (default %q)", f.DefValue))
	}
	return "    " + col + desc + "\n"
}
