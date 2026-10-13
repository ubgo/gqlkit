package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// noPositionalArgs rejects stray positional arguments on the generate
// commands. The usual cause is an UNQUOTED glob: the shell expands
// `-s ./schema/*.graphql` into `-s a.graphql b.graphql c.graphql`, so -s
// silently received only the first file and the rest were ignored. -s accepts
// a doublestar glob itself, so the fix is to quote it.
func noPositionalArgs(_ *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf("unexpected argument(s): %s\nhint: if you passed a glob to -s, quote it so the shell does not expand it, e.g. -s './schema/*.graphql'",
		strings.Join(args, " "))
}

// requireConfigFile fails when an explicitly passed -c path does not exist.
// The generators treat a missing config file as "no bindings", which is right
// when no -c was given but turns a typo in -c into a silently different SDK
// (every custom scalar falls back to `any`).
func requireConfigFile(path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("config file %s: %w", path, err)
	}
	return nil
}
