package main

import (
	"runtime/debug"
	"strings"
)

// version is set on release builds: -ldflags "-X main.version=<tag>".
var version = "dev"

// versionString reports the release version. A `go install ...@v1.2.3`
// build has no ldflags but records the module version in its build info.
func versionString() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return strings.TrimPrefix(bi.Main.Version, "v")
	}
	return version
}
