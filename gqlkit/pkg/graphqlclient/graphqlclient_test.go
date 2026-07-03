package graphqlclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capturedRequest records what the fake GraphQL server received so tests can
// assert on request-side correctness (headers, body, query, variables).
type capturedRequest struct {
	method      string
	contentType string
	accept      string
	authz       string
	customHdr   string
	body        graphQLRequest
	rawBody     string
}

// newServer stands up an httptest server that records the incoming request into
// capture and responds with the given HTTP status + raw body string.
func newServer(t *testing.T, status int, respBody string, capture *capturedRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			capture.method = r.Method
			capture.contentType = r.Header.Get("Content-Type")
			capture.accept = r.Header.Get("Accept")
			capture.authz = r.Header.Get("Authorization")
			capture.customHdr = r.Header.Get("X-Custom")
			raw, _ := io.ReadAll(r.Body)
			capture.rawBody = string(raw)
			_ = json.Unmarshal(raw, &capture.body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// -----------------------------------------------------------------------------
// Constructor + options
// -----------------------------------------------------------------------------

func TestNewClientDefaults(t *testing.T) {
	c := NewClient("http://example.test/graphql")
	if c.endpoint != "http://example.test/graphql" {
		t.Fatalf("endpoint = %q, want %q", c.endpoint, "http://example.test/graphql")
	}
	if c.httpClient != http.DefaultClient {
		t.Fatalf("httpClient should default to http.DefaultClient")
	}
	if c.headers == nil {
		t.Fatalf("headers map should be initialised")
	}
	if len(c.headers) != 0 {
		t.Fatalf("headers should start empty, got %v", c.headers)
	}
}

func TestWithHTTPClient(t *testing.T) {
	custom := &http.Client{}
	c := NewClient("http://x", WithHTTPClient(custom))
	if c.httpClient != custom {
		t.Fatalf("WithHTTPClient did not set the custom client")
	}
}

func TestWithHeader(t *testing.T) {
	c := NewClient("http://x", WithHeader("X-Custom", "abc"))
	if c.headers["X-Custom"] != "abc" {
		t.Fatalf("WithHeader did not set header, got %v", c.headers)
	}
}

func TestWithHeaders(t *testing.T) {
	c := NewClient("http://x",
		WithHeader("X-Custom", "keep"),
		WithHeaders(map[string]string{"A": "1", "B": "2"}),
	)
	if c.headers["A"] != "1" || c.headers["B"] != "2" || c.headers["X-Custom"] != "keep" {
		t.Fatalf("WithHeaders merge failed, got %v", c.headers)
	}
}

func TestWithAuthToken(t *testing.T) {
	c := NewClient("http://x", WithAuthToken("tok123"))
	if c.headers["Authorization"] != "Bearer tok123" {
		t.Fatalf("WithAuthToken = %q, want %q", c.headers["Authorization"], "Bearer tok123")
	}
}

// -----------------------------------------------------------------------------
// GraphQLError / GraphQLErrors error interface
// -----------------------------------------------------------------------------

func TestGraphQLErrorError(t *testing.T) {
	e := GraphQLError{Message: "boom"}
	if e.Error() != "boom" {
		t.Fatalf("GraphQLError.Error() = %q", e.Error())
	}
}

func TestGraphQLErrorsError(t *testing.T) {
	cases := []struct {
		name string
		errs GraphQLErrors
		want string
	}{
		{"empty", GraphQLErrors{}, ""},
		{"single", GraphQLErrors{{Message: "one"}}, "one"},
		{"multiple", GraphQLErrors{{Message: "one"}, {Message: "two"}}, "multiple errors: [one two]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.errs.Error(); got != tc.want {
				t.Fatalf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Execute — happy path + request correctness
// -----------------------------------------------------------------------------

func TestExecuteSuccess(t *testing.T) {
	var cap capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"user":{"id":"7","name":"Ada"}}}`, &cap)

	c := NewClient(srv.URL,
		WithAuthToken("secret"),
		WithHeader("X-Custom", "yes"),
	)

	var result struct {
		User struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"user"`
	}
	query := "query($id: ID!){ user(id:$id){ id name } }"
	vars := map[string]interface{}{"id": "7"}
	if err := c.Execute(context.Background(), query, vars, &result); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Response decoded correctly.
	if result.User.ID != "7" || result.User.Name != "Ada" {
		t.Fatalf("decoded result = %+v", result)
	}

	// Request-side assertions.
	if cap.method != http.MethodPost {
		t.Errorf("method = %q, want POST", cap.method)
	}
	if cap.contentType != "application/json" {
		t.Errorf("Content-Type = %q", cap.contentType)
	}
	if cap.accept != "application/json" {
		t.Errorf("Accept = %q", cap.accept)
	}
	if cap.authz != "Bearer secret" {
		t.Errorf("Authorization = %q", cap.authz)
	}
	if cap.customHdr != "yes" {
		t.Errorf("X-Custom = %q", cap.customHdr)
	}
	if cap.body.Query != query {
		t.Errorf("server query = %q, want %q", cap.body.Query, query)
	}
	if cap.body.Variables["id"] != "7" {
		t.Errorf("server variables = %v", cap.body.Variables)
	}
}

// When variables is nil, the omitempty on Variables should drop the field from
// the JSON body entirely.
func TestExecuteOmitsEmptyVariables(t *testing.T) {
	var cap capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{}}`, &cap)
	c := NewClient(srv.URL)

	if err := c.Execute(context.Background(), "{ ping }", nil, nil); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if strings.Contains(cap.rawBody, "variables") {
		t.Fatalf("expected variables to be omitted, body = %q", cap.rawBody)
	}
}

// result == nil must be tolerated (no decode attempted).
func TestExecuteNilResult(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{"data":{"anything":1}}`, nil)
	c := NewClient(srv.URL)
	if err := c.Execute(context.Background(), "{ x }", nil, nil); err != nil {
		t.Fatalf("Execute with nil result: %v", err)
	}
}

// data == null in the response must not attempt a decode into result.
func TestExecuteNullData(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{"data":null}`, nil)
	c := NewClient(srv.URL)
	var result struct {
		X int `json:"x"`
	}
	if err := c.Execute(context.Background(), "{ x }", nil, &result); err != nil {
		t.Fatalf("Execute with null data: %v", err)
	}
	if result.X != 0 {
		t.Fatalf("result should be zero-value, got %+v", result)
	}
}

// -----------------------------------------------------------------------------
// Execute — error paths
// -----------------------------------------------------------------------------

func TestExecuteGraphQLErrors(t *testing.T) {
	srv := newServer(t, http.StatusOK,
		`{"errors":[{"message":"field missing","locations":[{"line":1,"column":2}],"path":["user"]}]}`, nil)
	c := NewClient(srv.URL)

	var result map[string]interface{}
	err := c.Execute(context.Background(), "{ user }", nil, &result)
	if err == nil {
		t.Fatalf("expected GraphQL error")
	}
	var gqlErrs GraphQLErrors
	if !errors.As(err, &gqlErrs) {
		t.Fatalf("error is not GraphQLErrors: %T %v", err, err)
	}
	if len(gqlErrs) != 1 || gqlErrs[0].Message != "field missing" {
		t.Fatalf("unexpected errors: %v", gqlErrs)
	}
	if len(gqlErrs[0].Locations) != 1 || gqlErrs[0].Locations[0].Line != 1 || gqlErrs[0].Locations[0].Column != 2 {
		t.Fatalf("locations not decoded: %+v", gqlErrs[0].Locations)
	}
}

// data + errors both present: Execute short-circuits and returns the errors,
// leaving result undecoded.
func TestExecuteDataAndErrorsShortCircuits(t *testing.T) {
	srv := newServer(t, http.StatusOK,
		`{"data":{"user":{"id":"1"}},"errors":[{"message":"partial"}]}`, nil)
	c := NewClient(srv.URL)

	var result struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	err := c.Execute(context.Background(), "{ user }", nil, &result)
	if err == nil {
		t.Fatalf("expected error when errors array present")
	}
	if result.User.ID != "" {
		t.Fatalf("Execute should not decode data when errors present; got %+v", result)
	}
}

func TestExecuteNon200(t *testing.T) {
	srv := newServer(t, http.StatusInternalServerError, `internal boom`, nil)
	c := NewClient(srv.URL)
	err := c.Execute(context.Background(), "{ x }", nil, nil)
	if err == nil {
		t.Fatalf("expected error on non-200")
	}
	if !strings.Contains(err.Error(), "unexpected status code: 500") || !strings.Contains(err.Error(), "internal boom") {
		t.Fatalf("error text = %q", err.Error())
	}
}

func TestExecuteMalformedJSON(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{not json`, nil)
	c := NewClient(srv.URL)
	err := c.Execute(context.Background(), "{ x }", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to unmarshal response") {
		t.Fatalf("expected unmarshal error, got %v", err)
	}
}

// data present but shape does not fit result -> decode error.
func TestExecuteDataDecodeError(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{"data":{"count":"not-a-number"}}`, nil)
	c := NewClient(srv.URL)
	var result struct {
		Count int `json:"count"`
	}
	err := c.Execute(context.Background(), "{ count }", nil, &result)
	if err == nil || !strings.Contains(err.Error(), "failed to unmarshal data") {
		t.Fatalf("expected data unmarshal error, got %v", err)
	}
}

// Transport failure: point the client at a closed server.
func TestExecuteTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // now nothing is listening
	c := NewClient(url)
	err := c.Execute(context.Background(), "{ x }", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to execute request") {
		t.Fatalf("expected transport error, got %v", err)
	}
}

// Unmarshalable variable value (a channel) -> json.Marshal fails.
func TestExecuteMarshalError(t *testing.T) {
	c := NewClient("http://example.test")
	vars := map[string]interface{}{"bad": make(chan int)}
	err := c.Execute(context.Background(), "{ x }", vars, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to marshal request") {
		t.Fatalf("expected marshal error, got %v", err)
	}
}

// Bad endpoint URL -> NewRequestWithContext fails.
func TestExecuteBadRequestURL(t *testing.T) {
	c := NewClient("http://\x7f-bad-url")
	err := c.Execute(context.Background(), "{ x }", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to create request") {
		t.Fatalf("expected create-request error, got %v", err)
	}
}

// Cancelled context should surface as an execute error.
func TestExecuteContextCancelled(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{"data":{}}`, nil)
	c := NewClient(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := c.Execute(ctx, "{ x }", nil, nil)
	if err == nil {
		t.Fatalf("expected error from cancelled context")
	}
}

// -----------------------------------------------------------------------------
// RawQuery
// -----------------------------------------------------------------------------

func TestRawQuerySuccess(t *testing.T) {
	// Note: RawQuery decodes into a struct with a nested "data" field, so the
	// server's data payload must itself contain a "data" key to be non-nil.
	srv := newServer(t, http.StatusOK, `{"data":{"data":{"hello":"world"}}}`, nil)
	c := NewClient(srv.URL)
	raw, err := c.RawQuery(context.Background(), "{ hello }", nil)
	if err != nil {
		t.Fatalf("RawQuery error: %v", err)
	}
	if string(raw) != `{"hello":"world"}` {
		t.Fatalf("RawQuery raw = %s", string(raw))
	}
}

func TestRawQueryError(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{"errors":[{"message":"nope"}]}`, nil)
	c := NewClient(srv.URL)
	_, err := c.RawQuery(context.Background(), "{ x }", nil)
	if err == nil || err.Error() != "nope" {
		t.Fatalf("expected 'nope' error, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// ExecuteWithPartialData
// -----------------------------------------------------------------------------

func TestExecuteWithPartialDataSuccess(t *testing.T) {
	var cap capturedRequest
	srv := newServer(t, http.StatusOK, `{"data":{"a":1}}`, &cap)
	c := NewClient(srv.URL, WithAuthToken("t"))
	data, gqlErrs, err := c.ExecuteWithPartialData(context.Background(), "{ a }", map[string]interface{}{"k": "v"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if len(gqlErrs) != 0 {
		t.Fatalf("expected no gql errors, got %v", gqlErrs)
	}
	if string(data) != `{"a":1}` {
		t.Fatalf("data = %s", string(data))
	}
	if cap.authz != "Bearer t" || cap.contentType != "application/json" || cap.accept != "application/json" {
		t.Fatalf("headers not set: %+v", cap)
	}
	if cap.body.Variables["k"] != "v" {
		t.Fatalf("variables not sent: %v", cap.body.Variables)
	}
}

// The whole point of ExecuteWithPartialData: data AND errors coexist.
func TestExecuteWithPartialDataBoth(t *testing.T) {
	srv := newServer(t, http.StatusOK,
		`{"data":{"ok":1},"errors":[{"message":"aliased field failed"}]}`, nil)
	c := NewClient(srv.URL)
	data, gqlErrs, err := c.ExecuteWithPartialData(context.Background(), "{ ok bad }", nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if string(data) != `{"ok":1}` {
		t.Fatalf("data should still be populated, got %s", string(data))
	}
	if len(gqlErrs) != 1 || gqlErrs[0].Message != "aliased field failed" {
		t.Fatalf("errors not returned: %v", gqlErrs)
	}
}

func TestExecuteWithPartialDataNon200(t *testing.T) {
	srv := newServer(t, http.StatusBadGateway, `gateway down`, nil)
	c := NewClient(srv.URL)
	data, gqlErrs, err := c.ExecuteWithPartialData(context.Background(), "{ x }", nil)
	if err == nil || !strings.Contains(err.Error(), "unexpected status code: 502") {
		t.Fatalf("expected 502 error, got %v", err)
	}
	if data != nil || gqlErrs != nil {
		t.Fatalf("data/errors should be nil on transport failure")
	}
}

func TestExecuteWithPartialDataMalformedJSON(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{bad`, nil)
	c := NewClient(srv.URL)
	_, _, err := c.ExecuteWithPartialData(context.Background(), "{ x }", nil)
	if err == nil || !strings.Contains(err.Error(), "failed to unmarshal response") {
		t.Fatalf("expected unmarshal error, got %v", err)
	}
}

func TestExecuteWithPartialDataTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	c := NewClient(url)
	_, _, err := c.ExecuteWithPartialData(context.Background(), "{ x }", nil)
	if err == nil || !strings.Contains(err.Error(), "failed to execute request") {
		t.Fatalf("expected transport error, got %v", err)
	}
}

func TestExecuteWithPartialDataMarshalError(t *testing.T) {
	c := NewClient("http://example.test")
	vars := map[string]interface{}{"bad": make(chan int)}
	_, _, err := c.ExecuteWithPartialData(context.Background(), "{ x }", vars)
	if err == nil || !strings.Contains(err.Error(), "failed to marshal request") {
		t.Fatalf("expected marshal error, got %v", err)
	}
}

func TestExecuteWithPartialDataBadRequestURL(t *testing.T) {
	c := NewClient("http://\x7f-bad-url")
	_, _, err := c.ExecuteWithPartialData(context.Background(), "{ x }", nil)
	if err == nil || !strings.Contains(err.Error(), "failed to create request") {
		t.Fatalf("expected create-request error, got %v", err)
	}
}
