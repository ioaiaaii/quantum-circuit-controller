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
// Version identity comes from internal/version, shared with the controller.
package main

import (
	"os"

	"github.com/ioaiaaii/quantum-circuit-controller/cmd/qcc/commands"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/version"
)

func main() {
	cmd := commands.NewRootCmd(version.Version())
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
