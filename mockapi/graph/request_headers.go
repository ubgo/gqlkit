package graph

import (
	"context"
	"net/http"
)

// requestHeadersKey is the context key for the incoming HTTP request headers.
// An unexported struct type cannot collide with any other package's key.
type requestHeadersKey struct{}

// WithRequestHeaders returns ctx carrying the request's headers, so resolvers
// such as requestHeader can read what the client sent. The HTTP layer
// (server.go's newHandler) calls it once per request; gqlgen itself does not
// expose headers to resolvers.
func WithRequestHeaders(ctx context.Context, h http.Header) context.Context {
	return context.WithValue(ctx, requestHeadersKey{}, h)
}

// requestHeadersFrom returns the headers stored by WithRequestHeaders, or nil
// when the resolver runs outside an HTTP request (unit tests); http.Header's
// Get is nil-safe, so callers need no check.
func requestHeadersFrom(ctx context.Context) http.Header {
	h, _ := ctx.Value(requestHeadersKey{}).(http.Header)
	return h
}
