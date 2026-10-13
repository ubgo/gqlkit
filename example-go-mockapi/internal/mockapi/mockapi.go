// Package mockapi holds what every example program in this module shares
// about the mockapi test server: where it listens and how a test proves it
// is up. The test-only half (Require) lives in package mockapitest.
//
// Why a package and not a constant per example: the examples are linked
// from the Go guides, and a reader who runs mockapi on another port should
// change one environment variable, not five files.
package mockapi

import (
	"fmt"
	"os"
)

// DefaultEndpoint is the GraphQL endpoint `task mockapi:run` serves.
const DefaultEndpoint = "http://localhost:8081/query"

// EndpointEnv overrides DefaultEndpoint, e.g. MOCKAPI_URL=http://localhost:9000/query.
const EndpointEnv = "MOCKAPI_URL"

// Endpoint returns the endpoint from EndpointEnv, or DefaultEndpoint.
func Endpoint() string {
	if v := os.Getenv(EndpointEnv); v != "" {
		return v
	}
	return DefaultEndpoint
}

// Exit prints err and exits non-zero, so a failing example is visible to any
// script that runs it (`go run ./cmd/... && echo ok`).
func Exit(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

// Finish prints an example's collected output and exits non-zero when the
// example or the write failed. Examples collect output in a strings.Builder,
// whose writes cannot fail, and print it here in one write that is checked,
// so a closed stdout is reported instead of silently losing output.
func Finish(output string, err error) {
	if _, werr := os.Stdout.WriteString(output); werr != nil && err == nil {
		err = fmt.Errorf("write output: %w", werr)
	}
	if err != nil {
		Exit(err)
	}
}
