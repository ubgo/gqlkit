package graphqlclient

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestResponseStatusMatrix crosses HTTP status × response body for both
// Execute and ExecuteWithPartialData. The rule under test (Client.do): a body
// that is a GraphQL response wins whatever the status (gqlgen answers
// validation failures with 422 + {"errors":[...]}); only a non-GraphQL body
// with a non-200 status becomes *HTTPError, keeping status and body.
func TestResponseStatusMatrix(t *testing.T) {
	bodies := map[string]string{
		"errors":      `{"errors":[{"message":"missing b"}]}`,
		"data":        `{"data":{"x":1}}`,
		"data+errors": `{"data":{"x":1},"errors":[{"message":"partial"}]}`,
		"null data":   `{"data":null}`,
		"html":        `<html>bad gateway</html>`,
		"empty":       ``,
		"json no gql": `{"message":"unauthorized"}`,
	}
	statuses := []int{http.StatusOK, http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusInternalServerError, http.StatusBadGateway}

	for _, status := range statuses {
		for name, body := range bodies {
			srv := newServer(t, status, body, nil)
			c := NewClient(srv.URL)
			isGQL := name == "errors" || name == "data" || name == "data+errors"

			var out struct{ X int }
			err := c.Execute(context.Background(), "{ x }", nil, &out)
			data, gqlErrs, perr := c.ExecuteWithPartialData(context.Background(), "{ x }", nil)

			var httpErr *HTTPError
			var execGQL GraphQLErrors
			switch {
			case isGQL:
				if strings.Contains(name, "errors") {
					if !errors.As(err, &execGQL) {
						t.Errorf("%d/%s Execute: err = %v, want GraphQLErrors", status, name, err)
					}
					if len(gqlErrs) == 0 || perr != nil {
						t.Errorf("%d/%s partial: errs=%v err=%v, want GraphQL errors", status, name, gqlErrs, perr)
					}
				} else {
					if err != nil || out.X != 1 {
						t.Errorf("%d/%s Execute: err=%v X=%d, want decoded data", status, name, err, out.X)
					}
				}
				if strings.HasPrefix(name, "data") && string(data) != `{"x":1}` {
					t.Errorf("%d/%s partial data = %s", status, name, data)
				}
			case status != http.StatusOK:
				if !errors.As(err, &httpErr) || httpErr.StatusCode != status || httpErr.Body != body {
					t.Errorf("%d/%s Execute: err = %#v, want *HTTPError with status+body", status, name, err)
				}
				if !errors.As(perr, &httpErr) || httpErr.StatusCode != status {
					t.Errorf("%d/%s partial: err = %v, want *HTTPError", status, name, perr)
				}
				if err != nil && !strings.Contains(err.Error(), "unexpected status code") {
					t.Errorf("%d/%s message changed: %v", status, name, err)
				}
			default: // 200 with a non-GraphQL body
				if name == "html" || name == "empty" {
					if err == nil || perr == nil {
						t.Errorf("200/%s: want decode errors, got %v / %v", name, err, perr)
					}
				} else if err != nil || perr != nil {
					// {"data":null} and a JSON object without data/errors
					// decode cleanly to "nothing".
					t.Errorf("200/%s: unexpected errors %v / %v", name, err, perr)
				}
			}
		}
	}
}

// TestRawQueryReturnsDataObject pins RawQuery against the response shape a
// real server sends.
func TestRawQueryReturnsDataObject(t *testing.T) {
	srv := newServer(t, http.StatusOK, `{"data":{"ping":"pong from gqlgenapi"}}`, nil)
	raw, err := NewClient(srv.URL).RawQuery(context.Background(), "{ ping }", nil)
	if err != nil || string(raw) != `{"ping":"pong from gqlgenapi"}` {
		t.Fatalf("RawQuery = %q, %v", raw, err)
	}
}

// TestGraphQLClientTemplateMirrorsPackage enforces the "two faces" rule: the
// generated SDK's graphqlclient (graphqlclient.tmpl) must carry the same
// request/decode logic as this package, from HTTPError to the end.
func TestGraphQLClientTemplateMirrorsPackage(t *testing.T) {
	pkgSrc, err := os.ReadFile("graphqlclient.go")
	if err != nil {
		t.Fatal(err)
	}
	tmplSrc, err := os.ReadFile("../templater/template/graphqlclient.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	const marker = "// HTTPError is returned"
	tail := func(name, s string) string {
		i := strings.Index(s, marker)
		if i < 0 {
			t.Fatalf("%s: marker not found", name)
		}
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s[i:]), "{{ end }}"))
	}
	if tail("go", string(pkgSrc)) != tail("tmpl", string(tmplSrc)) {
		t.Error("graphqlclient.tmpl has drifted from pkg/graphqlclient/graphqlclient.go; mirror the change")
	}
}
