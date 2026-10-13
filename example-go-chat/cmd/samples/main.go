package main

import (
	"context"
	"encoding/json"
	"errors"
	"example-go-chat/sdk/graphqlclient"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"example-go-chat/sdk/fields"
	"example-go-chat/sdk/inputs"
	"example-go-chat/sdk/mutations"
	"example-go-chat/sdk/queries"
)

// Environment variables that configure the private chat API. The server,
// workspace and token belong to a private project, so nothing is hard-coded:
// without them the client still builds, and the server rejects the call.
const (
	envURL         = "CHAT_API_URL"
	envWorkspaceID = "CHAT_WORKSPACE_ID"
	envToken       = "CHAT_TOKEN"

	// defaultURL is where the private chat API listens in its own dev setup.
	defaultURL = "http://localhost:2310/api/sa/query"
)

// defaultSamples run when no sample names are given on the command line;
// they are the two the program has always run.
var defaultSamples = []string{"ping", "chatbots"}

// sample is one runnable demo. Each receives both roots and uses the one it needs.
type sample func(ctx context.Context, qr *queries.QueryRoot, mr *mutations.MutationRoot) error

// samples maps the names accepted on the command line to their demos.
// Order of execution follows the command-line order, not this map.
var samples = map[string]sample{
	"ping": func(ctx context.Context, qr *queries.QueryRoot, _ *mutations.MutationRoot) error {
		return runPing(ctx, qr)
	},
	"chatbots": func(ctx context.Context, qr *queries.QueryRoot, _ *mutations.MutationRoot) error {
		return runChatbots(ctx, qr)
	},
	"credentials": func(ctx context.Context, qr *queries.QueryRoot, _ *mutations.MutationRoot) error {
		return runCredentials(ctx, qr)
	},
	"users": func(ctx context.Context, qr *queries.QueryRoot, _ *mutations.MutationRoot) error {
		return runUsers(ctx, qr)
	},
	"folders": func(ctx context.Context, qr *queries.QueryRoot, _ *mutations.MutationRoot) error {
		return runFolders(ctx, qr)
	},
	"chatbots-nested": func(ctx context.Context, qr *queries.QueryRoot, _ *mutations.MutationRoot) error {
		return runChatbotsNested(ctx, qr)
	},
	"folders-nested": func(ctx context.Context, qr *queries.QueryRoot, _ *mutations.MutationRoot) error {
		return runFoldersNested(ctx, qr)
	},
	"channels-nested": func(ctx context.Context, qr *queries.QueryRoot, _ *mutations.MutationRoot) error {
		return runChannelsNested(ctx, qr)
	},
	"html-to-markdown": func(ctx context.Context, _ *queries.QueryRoot, mr *mutations.MutationRoot) error {
		return runHTMLToMarkdown(ctx, mr)
	},
}

// sampleNames returns every registered sample name, sorted, for usage text.
func sampleNames() []string {
	names := make([]string, 0, len(samples))
	for name := range samples {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Usage: go run ./cmd/samples [name ...]   (or "all"; default: ping chatbots)
func main() {
	names := os.Args[1:]
	switch {
	case len(names) == 0:
		names = defaultSamples
	case len(names) == 1 && names[0] == "all":
		names = sampleNames()
	}
	for _, name := range names {
		if _, ok := samples[name]; !ok {
			log.Fatalf("unknown sample %q; choose from: all %s", name, strings.Join(sampleNames(), " "))
		}
	}

	url := os.Getenv(envURL)
	if url == "" {
		url = defaultURL
	}
	headers := map[string]string{}
	if v := os.Getenv(envWorkspaceID); v != "" {
		headers["x-workspace-id"] = v
	}
	if v := os.Getenv(envToken); v != "" {
		headers["token"] = v
	}
	client := graphqlclient.NewClient(url, graphqlclient.WithHeaders(headers))

	ctx := context.Background()
	qr := queries.NewQueryRoot(client)
	mr := mutations.NewMutationRoot(client)

	for _, name := range names {
		if err := samples[name](ctx, qr, mr); err != nil {
			debugPrintError(err)
			log.Fatalf("%s: %v", name, err)
		}
	}
}

func debugPrintError(err error) {
	var gqlErrs graphqlclient.GraphQLErrors
	if errors.As(err, &gqlErrs) {
		data, mErr := json.MarshalIndent(gqlErrs, "", "  ")
		if mErr != nil {
			log.Printf("GraphQL error (marshal failed: %v): %v", mErr, err)
			return
		}
		log.Printf("GraphQL errors (raw):\n%s", string(data))
		return
	}

	log.Printf("error: %+v", err)
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}

func runPing(ctx context.Context, qr *queries.QueryRoot) error {
	pingResult, err := qr.Ping().Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Ping result: %v\n", pingResult)

	// full data JSON
	rawData, err := qr.Ping().ExecuteRaw(ctx)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(rawData, "", "  ")
	fmt.Printf("Ping raw data:\n%s\n", b)

	return nil
}

func runChatbots(ctx context.Context, qr *queries.QueryRoot) error {
	chatbotsResult, err := qr.Chatbots().
		First(intPtr(10)).
		Select(func(conn *fields.ChatbotConnectionFields) {
			conn.TotalCount()
			conn.Edges(func(e *fields.ChatbotEdgeFields) {
				e.Cursor()
				e.Node(func(c *fields.ChatbotFields) {
					c.ID().
						Name().
						CreatedAt().
						UpdatedAt()
				})
			})
			conn.PageInfo(func(p *fields.PageInfoFields) {
				p.HasNextPage().
					HasPreviousPage().
					StartCursor().
					EndCursor()
			})
		}).
		Execute(ctx)
	if err != nil {
		return err
	}

	// goutil.PrintToJSON(chatbotsResult)

	fmt.Printf("Chatbots: %+v\n", chatbotsResult)
	return nil
}

func runCredentials(ctx context.Context, qr *queries.QueryRoot) error {
	credentialsResult, err := qr.Credentials().
		First(intPtr(5)).
		Select(func(conn *fields.CredentialConnectionFields) {
			conn.TotalCount()
			conn.Edges(func(e *fields.CredentialEdgeFields) {
				e.Cursor()
				e.Node(func(c *fields.CredentialFields) {
					c.ID().
						Name().
						CreatedAt()
				})
			})
		}).
		Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Credentials: %+v\n", credentialsResult)
	return nil
}

func runUsers(ctx context.Context, qr *queries.QueryRoot) error {
	usersResult, err := qr.Users().
		First(intPtr(10)).
		Select(func(conn *fields.UserConnectionFields) {
			conn.TotalCount()
			conn.Edges(func(e *fields.UserEdgeFields) {
				e.Cursor()
				e.Node(func(u *fields.UserFields) {
					u.ID().
						Email().
						FirstName().
						LastName().
						CreatedAt()
				})
			})
		}).
		Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Users: %+v\n", usersResult)
	return nil
}

func runFolders(ctx context.Context, qr *queries.QueryRoot) error {
	foldersResult, err := qr.Folders().
		First(intPtr(20)).
		Select(func(conn *fields.FolderConnectionFields) {
			conn.TotalCount()
			conn.Edges(func(e *fields.FolderEdgeFields) {
				e.Cursor()
				e.Node(func(f *fields.FolderFields) {
					f.ID().
						Name().
						Position().
						CreatedAt()
				})
			})
		}).
		Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Folders: %+v\n", foldersResult)
	return nil
}

func runChatbotsNested(ctx context.Context, qr *queries.QueryRoot) error {
	chatbotsNestedResult, err := qr.Chatbots().
		First(intPtr(5)).
		Select(func(conn *fields.ChatbotConnectionFields) {
			conn.TotalCount()
			conn.Edges(func(e *fields.ChatbotEdgeFields) {
				e.Node(func(c *fields.ChatbotFields) {
					c.ID().
						Name().
						CreatedAt()
					c.AiModel(func(ai *fields.AiModelFields) {
						ai.ID().
							ModelID().
							ProviderName().
							Status()
					})
					c.Users(func(u *fields.UserFields) {
						u.ID().
							Email().
							FirstName().
							LastName()
					})
				})
			})
		}).
		Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Chatbots with nested: %+v\n", chatbotsNestedResult)
	return nil
}

func runFoldersNested(ctx context.Context, qr *queries.QueryRoot) error {
	foldersNestedResult, err := qr.Folders().
		First(intPtr(10)).
		Select(func(conn *fields.FolderConnectionFields) {
			conn.TotalCount()
			conn.Edges(func(e *fields.FolderEdgeFields) {
				e.Node(func(f *fields.FolderFields) {
					f.ID().
						Name().
						Position()
					f.Owner(func(u *fields.UserFields) {
						u.ID().
							Email().
							FirstName()
					})
					f.Chatbot(func(c *fields.ChatbotFields) {
						c.ID().
							Name()
					})
					f.Channels(func(ch *fields.ChannelFields) {
						ch.ID().
							Name().
							CreatedAt()
					})
				})
			})
		}).
		Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Folders with nested: %+v\n", foldersNestedResult)
	return nil
}

func runChannelsNested(ctx context.Context, qr *queries.QueryRoot) error {
	channelsResult, err := qr.Channels().
		First(intPtr(5)).
		Select(func(conn *fields.ChannelConnectionFields) {
			conn.TotalCount()
			conn.Edges(func(e *fields.ChannelEdgeFields) {
				e.Node(func(ch *fields.ChannelFields) {
					ch.ID().
						Name().
						CreatedAt()
					ch.Owner(func(u *fields.UserFields) {
						u.ID().
							Email()
					})
					ch.Chatbot(func(c *fields.ChatbotFields) {
						c.ID().
							Name()
						c.AiModel(func(ai *fields.AiModelFields) {
							ai.ID().
								ModelID().
								ProviderName()
						})
					})
					ch.Folder(func(f *fields.FolderFields) {
						f.ID().
							Name().
							Position()
					})
				})
			})
		}).
		Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Channels with deep nesting: %+v\n", channelsResult)
	return nil
}

func runHTMLToMarkdown(ctx context.Context, mr *mutations.MutationRoot) error {
	markdownResult, err := mr.HTMLToMarkdown().
		Input(inputs.HtmlToMarkdownInput{
			HTML: "<h1>Hello World</h1><p>This is a test</p>",
		}).
		Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Markdown: %+v\n", markdownResult)
	return nil
}
