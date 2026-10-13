package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/ubgo/gqlkit/gqlkit-sdl/schema"

	"github.com/spf13/cobra"
)

// version is the release identity, injected by goreleaser via
// `-ldflags "-X main.version=..."`. "dev" means not injected; resolveVersion
// then falls back to the module version recorded in build info.
var version = "dev"

// Output formats accepted by `fetch --format`. formatGraphQL is the default.
const (
	formatGraphQL = "graphql"
	formatJSON    = "json"
)

// outputFormats is the canonical list of --format values, used to validate
// the flag and to name the choices in its error.
var outputFormats = []string{formatGraphQL, formatJSON}

// defaultOutputPath is where `fetch` writes when --output is not given.
const defaultOutputPath = "schema.graphql"

func main() {
	rootCmd := &cobra.Command{
		Use:   "gqlkit-sdl",
		Short: "Fetch a GraphQL schema and convert it to SDL format",
	}

	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(fetchCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and exit",
		Run: func(cmd *cobra.Command, args []string) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "gqlkit-sdl", resolveVersion(version, debug.ReadBuildInfo))
		},
	}
}

func fetchCmd() *cobra.Command {
	var (
		url              string
		output           string
		headers          []string
		debugCurl        bool
		format           string
		onlyQueries      []string
		onlyMutations    []string
		excludeQueries   []string
		excludeMutations []string
		removeUnused     bool
	)

	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Fetch GraphQL schema and save as SDL",
		Long: `Fetch a GraphQL schema via introspection and save it as SDL.

Filter flags accept exact names or regex patterns (comma-separated).
Any value with regex metacharacters (. * + ? etc.) is treated as a regex.

Examples:
  gqlkit-sdl fetch --url https://graphql.anilist.co
  gqlkit-sdl fetch --url https://graphql.anilist.co -o my-schema.graphql
  gqlkit-sdl fetch --url https://graphql.anilist.co -H "Authorization: Bearer token"
  gqlkit-sdl fetch --url https://graphql.anilist.co -f json -o schema.json

  # Keep only specific queries
  gqlkit-sdl fetch --url https://example.com/graphql --only-queries users,posts

  # Exclude mutations by regex pattern
  gqlkit-sdl fetch --url https://example.com/graphql --exclude-mutations "task.*,space.*"

  # Combine filters and prune orphaned types
  gqlkit-sdl fetch --url https://example.com/graphql \
    --exclude-queries "task.*,space.*" \
    --exclude-mutations "task.*,space.*" \
    --remove-unused`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Progress lines go to stdout exactly as before; routing them
			// through cmd lets tests capture them. A failed progress write
			// cannot affect the schema file, so its error is not checked.
			out := cmd.OutOrStdout()

			// Validate before any network call: an unknown value used to fall
			// back to SDL silently, so `-f yaml` wrote GraphQL to schema.yaml.
			if !slices.Contains(outputFormats, format) {
				return fmt.Errorf("unknown --format %q (valid: %s)", format, strings.Join(outputFormats, ", "))
			}

			opts := &schema.FetchOptions{
				Headers:     make(map[string]string),
				Debug:       debugCurl,
				DebugOutput: out,
			}

			for _, h := range headers {
				key, value, found := strings.Cut(h, ":")
				if !found {
					return fmt.Errorf("invalid header format %q, expected Key:Value", h)
				}
				opts.Headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}

			if !debugCurl {
				_, _ = fmt.Fprintf(out, "Fetching schema from %s...\n", url)
			}
			introspectionSchema, err := schema.FetchSchema(url, opts)
			if err != nil {
				return fmt.Errorf("fetching schema: %w", err)
			}
			if introspectionSchema == nil {
				return nil
			}

			filterOpts := &schema.FilterOptions{
				OnlyQueries:      onlyQueries,
				OnlyMutations:    onlyMutations,
				ExcludeQueries:   excludeQueries,
				ExcludeMutations: excludeMutations,
				RemoveUnused:     removeUnused,
			}
			if filterOpts.HasFilters() || filterOpts.RemoveUnused {
				_, _ = fmt.Fprintln(out, "Applying filters...")
				filtered, err := schema.FilterSchema(introspectionSchema, filterOpts)
				if err != nil {
					return fmt.Errorf("filtering schema: %w", err)
				}
				// FilterSchema drops a root left with no fields (empty types
				// are invalid SDL); say so, since the user did not ask for it.
				warnDroppedRoot(cmd.ErrOrStderr(), "mutation", introspectionSchema.MutationType, filtered.MutationType)
				warnDroppedRoot(cmd.ErrOrStderr(), "subscription", introspectionSchema.SubscriptionType, filtered.SubscriptionType)
				introspectionSchema = filtered
			}

			switch format {
			case formatJSON:
				_, _ = fmt.Fprintf(out, "Saving JSON to %s...\n", output)
				if err := schema.SaveAsJSON(introspectionSchema, output); err != nil {
					return fmt.Errorf("saving JSON file: %w", err)
				}
			case formatGraphQL:
				_, _ = fmt.Fprintln(out, "Converting to SDL format...")
				sdl := schema.ConvertToSDL(introspectionSchema)
				_, _ = fmt.Fprintf(out, "Saving to %s...\n", output)
				if err := schema.SaveToFile(sdl, output); err != nil {
					return fmt.Errorf("saving file: %w", err)
				}
			}

			_, _ = fmt.Fprintln(out, "Done! Schema saved successfully.")
			return nil
		},
	}

	cmd.Flags().StringVar(&url, "url", "", "GraphQL endpoint URL (required)")
	cmd.Flags().StringVarP(&output, "output", "o", defaultOutputPath, "Output file path")
	cmd.Flags().StringArrayVarP(&headers, "header", "H", nil, `HTTP header in "Key:Value" format (repeatable)`)
	cmd.Flags().BoolVar(&debugCurl, "debug", false, "Print the curl command for debugging")
	cmd.Flags().StringVarP(&format, "format", "f", formatGraphQL, `Output format: "graphql" (SDL) or "json"`)
	cmd.Flags().StringSliceVar(&onlyQueries, "only-queries", nil, "Keep only these query fields (comma-separated)")
	cmd.Flags().StringSliceVar(&onlyMutations, "only-mutations", nil, "Keep only these mutation fields (comma-separated)")
	cmd.Flags().StringSliceVar(&excludeQueries, "exclude-queries", nil, "Remove these query fields (comma-separated)")
	cmd.Flags().StringSliceVar(&excludeMutations, "exclude-mutations", nil, "Remove these mutation fields (comma-separated)")
	cmd.Flags().BoolVar(&removeUnused, "remove-unused", false, "Remove types/inputs not referenced by remaining operations")
	// Can only fail if --url was renamed without updating this line.
	// Ignoring it would silently make the flag optional.
	if err := cmd.MarkFlagRequired("url"); err != nil {
		panic(err)
	}

	return cmd
}

// warnDroppedRoot prints a note when filtering removed a root operation type
// that the fetched schema had. A failed write only loses the note.
func warnDroppedRoot(w io.Writer, kind string, before, after *schema.TypeRef) {
	if before != nil && after == nil {
		_, _ = fmt.Fprintf(w, "Note: filters removed every %s field; dropped the empty %s type from the output.\n", kind, before.Name)
	}
}
