package schema

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// cannedIntrospectionJSON returns a minimal but valid introspection response
// body describing a Query type with a single field.
func cannedIntrospectionJSON(t *testing.T) []byte {
	t.Helper()
	resp := IntrospectionResponse{
		Data: IntrospectionData{
			Schema: IntrospectionSchema{
				QueryType: &TypeRef{Name: "Query"},
				Types: []FullType{
					{Kind: "OBJECT", Name: "Query", Fields: []Field{
						{Name: "hello", Type: named("SCALAR", "String")},
					}},
				},
			},
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal canned response: %v", err)
	}
	return b
}

func TestFetchSchemaSuccess(t *testing.T) {
	body := cannedIntrospectionJSON(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request shape the fetcher produces.
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json content type, got %q", ct)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer tok" {
			t.Errorf("expected custom Authorization header to be forwarded, got %q", auth)
		}
		reqBody, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(reqBody), "IntrospectionQuery") {
			t.Errorf("request body should contain the introspection query, got %q", string(reqBody))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	got, err := FetchSchema(srv.URL, &FetchOptions{Headers: map[string]string{"Authorization": "Bearer tok"}})
	if err != nil {
		t.Fatalf("FetchSchema error: %v", err)
	}
	if got == nil || got.QueryType == nil || got.QueryType.Name != "Query" {
		t.Fatalf("unexpected parsed schema: %+v", got)
	}
	if len(got.Types) != 1 || got.Types[0].Name != "Query" {
		t.Errorf("unexpected types: %+v", got.Types)
	}
}

func TestFetchSchemaNilOpts(t *testing.T) {
	body := cannedIntrospectionJSON(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	got, err := FetchSchema(srv.URL, nil)
	if err != nil {
		t.Fatalf("FetchSchema with nil opts error: %v", err)
	}
	if got.QueryType.Name != "Query" {
		t.Errorf("unexpected schema: %+v", got)
	}
}

func TestFetchSchemaHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	}))
	defer srv.Close()

	_, err := FetchSchema(srv.URL, nil)
	if err == nil {
		t.Fatal("expected error on 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should include status code and body, got: %v", err)
	}
}

func TestFetchSchemaMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{ not valid json"))
	}))
	defer srv.Close()

	_, err := FetchSchema(srv.URL, nil)
	if err == nil {
		t.Fatal("expected unmarshal error, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("expected unmarshal error, got: %v", err)
	}
}

func TestFetchSchemaGraphQLErrors(t *testing.T) {
	resp := IntrospectionResponse{
		Errors: []GraphQLError{
			{Message: "introspection disabled"},
			{Message: "second error"},
		},
	}
	body, _ := json.Marshal(resp)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	_, err := FetchSchema(srv.URL, nil)
	if err == nil {
		t.Fatal("expected GraphQL errors to surface, got nil")
	}
	if !strings.Contains(err.Error(), "introspection disabled") ||
		!strings.Contains(err.Error(), "second error") {
		t.Errorf("expected aggregated GraphQL error messages, got: %v", err)
	}
}

func TestFetchSchemaRequestError(t *testing.T) {
	// A malformed URL causes http.NewRequest / client.Do to fail before any
	// network round-trip.
	_, err := FetchSchema("http://[::1]:namedport/graphql", nil)
	if err == nil {
		t.Fatal("expected error for malformed URL, got nil")
	}
}

func TestFetchSchemaConnectionRefused(t *testing.T) {
	// Point at a closed server to exercise the client.Do error path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	_, err := FetchSchema(url, nil)
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
	if !strings.Contains(err.Error(), "execute request") {
		t.Errorf("expected execute request error, got: %v", err)
	}
}

func TestFetchSchemaDebugReturnsNil(t *testing.T) {
	// Debug mode prints the curl command and returns (nil, nil) without
	// performing the request.
	schema, err := FetchSchema("http://example.com/graphql", &FetchOptions{Debug: true})
	if err != nil {
		t.Fatalf("debug mode should not error, got: %v", err)
	}
	if schema != nil {
		t.Errorf("debug mode should return nil schema, got: %+v", schema)
	}
}
