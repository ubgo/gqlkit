package main

import "runtime/debug"

// resolveVersion reports the version to print for `gqlkit-sdl version`.
//
// Why it exists: `version` is injected at build time via goreleaser
// ldflags, which only happens for the release binaries served by GitHub
// Releases and the install script. Anyone who installs with
// `go install github.com/ubgo/gqlkit/gqlkit-sdl@latest` gets
// the compile-time default instead, so a correctly-installed release
// reported itself as "dev" and users had no way to tell which version
// they were running when filing a bug.
//
// The module version IS recorded in the binary by the toolchain, so fall
// back to build info when ldflags did not set anything. Precedence is
// deliberate: an explicit ldflags value always wins, because that is the
// release identity and it may differ from the module version (release
// candidates, local builds tagged by hand).
//
// Returns "dev" when neither source knows — `go build` of a working tree
// has no module version, and claiming one there would be a lie.
func resolveVersion(ldflagsVersion string, read func() (*debug.BuildInfo, bool)) string {
	if ldflagsVersion != "" && ldflagsVersion != "dev" {
		return ldflagsVersion
	}
	info, ok := read()
	if !ok || info == nil {
		return "dev"
	}
	// "(devel)" is what the toolchain records for a build from a working
	// tree rather than a resolved module version — no more informative
	// than "dev", and less recognisable.
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return "dev"
}
