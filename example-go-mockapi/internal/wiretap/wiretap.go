// Package wiretap is an http.RoundTripper that records each outgoing request
// before passing it on. The examples plug it in with
// graphqlclient.WithHTTPClient to show exactly what the generated client
// sends (headers, the GraphQL document, the variables) without needing a
// server that echoes them back.
package wiretap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// Request is one recorded call: the headers as sent and the decoded
// GraphQL request body.
type Request struct {
	Header    http.Header
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

// Transport records every request, then delegates to Next (or
// http.DefaultTransport when Next is nil). Safe for concurrent use.
type Transport struct {
	Next http.RoundTripper

	mu   sync.Mutex
	reqs []Request
}

// RoundTrip implements http.RoundTripper. A RoundTripper must not modify
// the caller's request, so the body is read once and the real transport
// gets a clone carrying an identical copy of it.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := Request{Header: req.Header.Clone()}
	out := req
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("wiretap: read body: %w", err)
		}
		_ = req.Body.Close()
		out = req.Clone(req.Context())
		out.Body = io.NopCloser(bytes.NewReader(body))
		if err := json.Unmarshal(body, &rec); err != nil {
			return nil, fmt.Errorf("wiretap: decode GraphQL body: %w", err)
		}
	}
	t.mu.Lock()
	t.reqs = append(t.reqs, rec)
	t.mu.Unlock()

	next := t.Next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(out)
}

// Last returns the most recent request, or false when none was sent.
func (t *Transport) Last() (Request, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.reqs) == 0 {
		return Request{}, false
	}
	return t.reqs[len(t.reqs)-1], true
}

// Count returns how many requests went through the transport.
func (t *Transport) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.reqs)
}
