/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package version resolves the build identity for QCC binaries:
// linker-stamped Build* variables first (VERSION_PKG in go.mk), then
// the module version from ReadBuildInfo, then "dev".
package version

import (
	"runtime/debug"
)

// Overwritten by the linker on stamped builds.
var (
	BuildVersion = ""
	BuildHash    = ""
	BuildTime    = ""
)

// Version returns the best available version identity for this binary.
func Version() string {
	if BuildVersion != "" {
		return BuildVersion
	}
	if v := fromBuildInfo(); v != "" {
		return v
	}
	return "dev"
}

func fromBuildInfo() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return ""
}
