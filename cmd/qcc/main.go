/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// qcc is the user-facing CLI for the Quantum Circuit Controller.
// It submits OpenQASM 3 circuits to a cluster running the QCC operator and
// streams progress back to the terminal.
//
// Version resolution order:
//  1. linker-injected `version` (release builds; see Makefile qcc-build).
//  2. VCS info embedded by `go build` via debug.ReadBuildInfo.
//  3. `git describe --tags --always` shelled out at runtime (works for
//     `go run` inside a git checkout, where Go does not embed VCS info).
//  4. literal `dev`.
package main

import (
	"os"
	"os/exec"
	"runtime/debug"
	"strings"

	"github.com/ioaiaaii/quantum-circuit-controller/cmd/qcc/commands"
)

// devVersion marks a build with no explicit or VCS-derived version.
const devVersion = "dev"

// version is overridden at build time with:
//
//	-ldflags "-X main.version=v1.2.3"
var version = devVersion

func main() {
	cmd := commands.NewRootCmd(resolveVersion())
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func resolveVersion() string {
	if version != devVersion {
		return version
	}
	if v := versionFromBuildInfo(); v != "" {
		return v
	}
	if v := versionFromGit(); v != "" {
		return v
	}
	return devVersion
}

func versionFromBuildInfo() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev, suffix string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
			if len(rev) > 7 {
				rev = rev[:7]
			}
		case "vcs.modified":
			if s.Value == "true" {
				suffix = "-dirty"
			}
		}
	}
	if rev == "" {
		return ""
	}
	return rev + suffix
}

func versionFromGit() string {
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
