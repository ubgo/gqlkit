// Package main is the entry point for the mock GraphQL API server.
// It sets up a gqlgen-based HTTP server with an in-memory data store,
// providing a todo/user CRUD API for testing GraphQL client libraries.
// The server exposes a GraphQL playground at "/" and the query endpoint at "/query".
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"mockapi/graph"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/vektah/gqlparser/v2/ast"
)

// defaultPort is the fallback HTTP port when the PORT env var is not set.
// The example SDKs and Taskfile point at http://localhost:8081/query.
const defaultPort = "8081"

// Routes served by the mock API. queryPath is what every example SDK targets.
const (
	playgroundPath = "/"
	queryPath      = "/query"
)

const (
	// queryCacheSize bounds the LRU of parsed query documents, so identical
	// queries are parsed once.
	queryCacheSize = 1000
	// apqCacheSize bounds the Automatic Persisted Query hash -> query cache.
	apqCacheSize = 100
	// readHeaderTimeout stops a client that never finishes its headers from
	// holding a connection open forever (http.ListenAndServe has no timeout).
	readHeaderTimeout = 10 * time.Second
)

// newHandler builds the full HTTP surface — playground at playgroundPath and
// the GraphQL endpoint at queryPath — around the given resolver. It is split
// from main so tests drive exactly what the server serves.
func newHandler(resolver *graph.Resolver) http.Handler {
	srv := handler.New(graph.NewExecutableSchema(graph.Config{Resolvers: resolver}))

	// Register supported HTTP transports: OPTIONS (CORS preflight), GET, and POST.
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](queryCacheSize))

	// Enable schema introspection so clients (and gqlkit-sdl) can discover the API.
	srv.Use(extension.Introspection{})
	// Enable Automatic Persisted Queries (APQ), allowing clients to send a
	// query hash instead of the full query string.
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](apqCacheSize),
	})

	mux := http.NewServeMux()
	mux.Handle(playgroundPath, playground.Handler("GraphQL playground", queryPath))
	// Stash the request headers in the context for the requestHeader query.
	mux.Handle(queryPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.ServeHTTP(w, r.WithContext(graph.WithRequestHeaders(r.Context(), r.Header)))
	}))
	return mux
}

func main() {
	// Allow port override via environment variable for flexible deployment.
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           newHandler(&graph.Resolver{}),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	log.Printf("connect to http://localhost:%s%s for GraphQL playground", port, playgroundPath)
	log.Fatal(server.ListenAndServe())
}
