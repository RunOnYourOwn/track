// Package version exposes the build version, surfaced identically by the CLI
// (`track --version`), the MCP server (initialize serverInfo), and the web UI
// (GET /api/meta) — one source of truth, no per-surface hard-coding.
package version

import "runtime/debug"

// Version is injected at build/release time via:
//
//	-ldflags "-X github.com/RunOnYourOwn/track/internal/version.Version=<git describe>"
//
// (the Makefile and the release workflow set it). Left empty for a bare
// `go build`, in which case String() falls back to the module version.
var Version = ""

// String returns the build version: the ldflag value if set, else the module
// version the Go proxy stamps for `go install …@vX.Y.Z`, else "dev".
func String() string {
	if Version != "" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
