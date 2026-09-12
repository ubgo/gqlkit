package main

import (
	"runtime/debug"
	"testing"
)

func buildInfo(v string) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: v}}, true
	}
}

func noBuildInfo() (*debug.BuildInfo, bool) { return nil, false }

// Pins every arm of resolveVersion. The function exists because a release
// installed with `go install` reported itself as "dev", so each arm is a
// distinct way of getting the reported version wrong.
func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name    string
		ldflags string
		read    func() (*debug.BuildInfo, bool)
		want    string
	}{
		{
			// The release path: goreleaser stamps the tag, and it must win
			// even when build info also knows a module version, because the
			// two can legitimately differ (release candidates, hand builds).
			name:    "ldflags value wins over build info",
			ldflags: "v1.2.3",
			read:    buildInfo("v0.11.0"),
			want:    "v1.2.3",
		},
		{
			// The `go install` path this function was written for.
			name:    "falls back to the module version",
			ldflags: "dev",
			read:    buildInfo("v0.11.0"),
			want:    "v0.11.0",
		},
		{
			// An empty ldflags var is as unset as the "dev" default.
			name:    "empty ldflags also falls back",
			ldflags: "",
			read:    buildInfo("v0.11.0"),
			want:    "v0.11.0",
		},
		{
			// `go build` in a working tree: the toolchain records "(devel)",
			// which is no more useful than "dev" and less recognisable.
			name:    "devel module version reports dev",
			ldflags: "dev",
			read:    buildInfo("(devel)"),
			want:    "dev",
		},
		{
			name:    "empty module version reports dev",
			ldflags: "dev",
			read:    buildInfo(""),
			want:    "dev",
		},
		{
			// Must not panic when build info is unavailable — that is the
			// documented failure mode of debug.ReadBuildInfo.
			name:    "missing build info reports dev",
			ldflags: "dev",
			read:    noBuildInfo,
			want:    "dev",
		},
		{
			name:    "nil build info reports dev",
			ldflags: "dev",
			read:    func() (*debug.BuildInfo, bool) { return nil, true },
			want:    "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.ldflags, tt.read); got != tt.want {
				t.Fatalf("resolveVersion(%q) = %q, want %q", tt.ldflags, got, tt.want)
			}
		})
	}
}
