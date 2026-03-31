package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/khanakia/gqlkit/gqlkit-sdl/schema"

	"github.com/spf13/cobra"
)

var version = "dev"

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
			fmt.Println("gqlkit-sdl", version)
		},
	}
}

func fetchCmd() *cobra.Command {
	var (
		url              string
		output           string
		headers          []string
		debug            bool
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
			opts := &schema.FetchOptions{
				Headers: make(map[string]string),
				Debug:   debug,
			}

			for _, h := range headers {
				key, value, found := strings.Cut(h, ":")
				if !found {
					return fmt.Errorf("invalid header format %q, expected Key:Value", h)
				}
				opts.Headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}

			if !debug {
				fmt.Printf("Fetching schema from %s...\n", url)
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
				fmt.Println("Applying filters...")
				introspectionSchema = schema.FilterSchema(introspectionSchema, filterOpts)
			}

			switch format {
			case "json":
				fmt.Printf("Saving JSON to %s...\n", output)
				if err := schema.SaveAsJSON(introspectionSchema, output); err != nil {
					return fmt.Errorf("saving JSON file: %w", err)
				}
			default:
				fmt.Println("Converting to SDL format...")
				sdl := schema.ConvertToSDL(introspectionSchema)
				fmt.Printf("Saving to %s...\n", output)
				if err := schema.SaveToFile(sdl, output); err != nil {
					return fmt.Errorf("saving file: %w", err)
				}
			}

			fmt.Println("Done! Schema saved successfully.")
			return nil
		},
	}

	cmd.Flags().StringVar(&url, "url", "", "GraphQL endpoint URL (required)")
	cmd.Flags().StringVarP(&output, "output", "o", "schema.graphql", "Output file path")
	cmd.Flags().StringArrayVarP(&headers, "header", "H", nil, `HTTP header in "Key:Value" format (repeatable)`)
	cmd.Flags().BoolVar(&debug, "debug", false, "Print the curl command for debugging")
	cmd.Flags().StringVarP(&format, "format", "f", "graphql", `Output format: "graphql" (SDL) or "json"`)
	cmd.Flags().StringSliceVar(&onlyQueries, "only-queries", nil, "Keep only these query fields (comma-separated)")
	cmd.Flags().StringSliceVar(&onlyMutations, "only-mutations", nil, "Keep only these mutation fields (comma-separated)")
	cmd.Flags().StringSliceVar(&excludeQueries, "exclude-queries", nil, "Remove these query fields (comma-separated)")
	cmd.Flags().StringSliceVar(&excludeMutations, "exclude-mutations", nil, "Remove these mutation fields (comma-separated)")
	cmd.Flags().BoolVar(&removeUnused, "remove-unused", false, "Remove types/inputs not referenced by remaining operations")
	cmd.MarkFlagRequired("url")

	return cmd
}
