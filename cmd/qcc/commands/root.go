/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package commands contains the cobra command tree for the qcc CLI.
package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ioaiaaii/quantum-circuit-controller/internal/cli/render"
)

// NewRootCmd builds the qcc root command with all subcommands attached and
// every level styled by the render package.
//
// Error-handling shape:
//
//   - `SilenceUsage: true` — we don't dump the full usage block on every
//     error (it's noisy when business logic fails for transient reasons).
//   - `SilenceErrors: false` — Cobra prints "Error: <msg>" itself for
//     flag-parse / arg-validation failures.  Business errors that flow
//     through RunE have already been styled by render.Fail, so they'll
//     double-print; that's accepted as a minor blemish and the price for
//     getting flag/arg errors visible.
//   - `SetFlagErrorFunc` rewrites unknown-flag errors to also print the
//     command's help block, since "what did I type wrong?" is the
//     single most useful answer when a flag fails.
func NewRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "qcc <command>",
		Short:         "Quantum Circuit Controller CLI",
		Long:          "Submit and observe quantum circuits managed by the Quantum Circuit Controller operator.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	cmd.SetVersionTemplate("{{.Version}}\n")
	// Keep cobra's built-in `completion` subcommand available but unlisted
	// in the COMMANDS table.  `qcc completion bash|zsh|fish|powershell`
	// still works; ValidArgsFunction declarations on subcommands feed it.
	cmd.CompletionOptions.HiddenDefaultCmd = true

	help := func(c *cobra.Command, _ []string) {
		_, _ = fmt.Fprint(c.OutOrStdout(), render.Help(c, version))
	}
	usage := func(c *cobra.Command) error {
		_, _ = fmt.Fprint(c.OutOrStderr(), render.Help(c, version))
		return nil
	}
	cmd.SetHelpFunc(help)
	cmd.SetUsageFunc(usage)

	// When a flag is wrong/unknown, also print the help block — that's
	// the answer the user usually wants ("what flags does this take?").
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		_, _ = fmt.Fprint(c.OutOrStderr(), render.Fail("flag error", err.Error()))
		_, _ = fmt.Fprintln(c.OutOrStderr())
		_, _ = fmt.Fprint(c.OutOrStderr(), render.Help(c, version))
		return err
	})

	cmd.AddCommand(newVersionCmd(version))
	cmd.AddCommand(newRunCmd(version))
	cmd.AddCommand(newDrawCmd(version))
	cmd.AddCommand(newScheduleCmd(version))
	cmd.AddCommand(newGetCmd(version))
	return cmd
}

// argsWithHelp wraps a Cobra args-validator so a wrong-count error
// emits the command's help block before failing.  Used by all commands
// that take positional args — `qcc run`, `qcc draw`, `qcc get` —
// because the answer to "I forgot the file argument" is almost always
// "here's what this command takes."
func argsWithHelp(validator cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validator(cmd, args); err != nil {
			_, _ = fmt.Fprint(cmd.OutOrStderr(), render.Fail("usage", err.Error()))
			_, _ = fmt.Fprintln(cmd.OutOrStderr())
			_, _ = fmt.Fprint(cmd.OutOrStderr(), render.Help(cmd, cmd.Root().Version))
			return err
		}
		return nil
	}
}
