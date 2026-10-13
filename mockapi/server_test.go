package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"mockapi/graph"
)

// gqlResponse is the GraphQL-over-HTTP envelope; Data stays raw so each test
// decodes only the shape it asserts on.
type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message    string   `json:"message"`
		Path       []string `json:"path"`
		Extensions struct {
			Code string `json:"code"`
		} `json:"extensions"`
	} `json:"errors"`
}

// postErr sends one GraphQL request to the handler under test. It returns an
// error instead of failing the test so it is safe to call from goroutines.
func postErr(srv *httptest.Server, query string) (gqlResponse, error) {
	var out gqlResponse
	body, err := json.Marshal(struct {
		Query string `json:"query"`
	}{query})
	if err != nil {
		return out, err
	}
	resp, err := http.Post(srv.URL+queryPath, "application/json", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("status %d for %s", resp.StatusCode, query)
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	return out, err
}

// post is postErr for the test goroutine: any transport failure is fatal.
func post(t *testing.T, srv *httptest.Server, query string) gqlResponse {
	t.Helper()
	out, err := postErr(srv, query)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// firstProblem reduces a goroutine's result to one reportable string, or "".
func firstProblem(res gqlResponse, err error) string {
	if err != nil {
		return err.Error()
	}
	if len(res.Errors) > 0 {
		return res.Errors[0].Message
	}
	return ""
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(newHandler(&graph.Resolver{}))
	t.Cleanup(srv.Close)
	return srv
}

// TestIntrospectionEnabled: gqlkit-sdl's fetch task depends on the mock
// answering introspection with a query and mutation root.
func TestIntrospectionEnabled(t *testing.T) {
	srv := newTestServer(t)
	res := post(t, srv, `{ __schema { queryType { name } mutationType { name } } }`)
	if len(res.Errors) > 0 {
		t.Fatalf("errors: %+v", res.Errors)
	}
	if !strings.Contains(string(res.Data), `"Query"`) || !strings.Contains(string(res.Data), `"Mutation"`) {
		t.Errorf("unexpected introspection data: %s", res.Data)
	}
}

// TestRoutes: the playground answers at "/" and the endpoint rejects a
// malformed body with a GraphQL error rather than crashing.
func TestRoutes(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(srv.URL + playgroundPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("playground status %d", resp.StatusCode)
	}

	resp, err = http.Post(srv.URL+queryPath, "application/json", strings.NewReader("{not json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusOK || resp.StatusCode >= http.StatusInternalServerError {
		t.Errorf("malformed body status %d, want a 4xx", resp.StatusCode)
	}
}

// TestBatchedRootFields mirrors the example SDKs' batch samples: several
// aliased root fields in one document, resolved in parallel by gqlgen.
func TestBatchedRootFields(t *testing.T) {
	srv := newTestServer(t)
	res := post(t, srv, `{
		a: todos { id }
		b: todosConnection(pagination: {limit: 1}) { totalCount pageInfo { hasNextPage } }
		c: users { id }
		d: search(term: "") { __typename }
		e: ping
	}`)
	if len(res.Errors) > 0 {
		t.Fatalf("errors: %+v", res.Errors)
	}
	var data struct {
		B struct {
			TotalCount int `json:"totalCount"`
			PageInfo   struct {
				HasNextPage bool `json:"hasNextPage"`
			} `json:"pageInfo"`
		} `json:"b"`
	}
	if err := json.Unmarshal(res.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.B.TotalCount != 2 || !data.B.PageInfo.HasNextPage {
		t.Errorf("connection page info: %+v", data.B)
	}
}

// TestConcurrentReadsAndWrites hammers the server with interleaved mutations
// and batched reads. Run with -race: before the resolver lock, the lazy seed
// and every slice append raced.
func TestConcurrentReadsAndWrites(t *testing.T) {
	const workers = 16
	srv := newTestServer(t)
	var wg sync.WaitGroup
	errs := make(chan string, workers*2)
	for i := range workers {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if p := firstProblem(postErr(srv, fmt.Sprintf(`mutation { createTodo(input: {text: "t%d", userId: "1"}) { id } completeAllTodos }`, i))); p != "" {
				errs <- p
			}
		}()
		go func() {
			defer wg.Done()
			if p := firstProblem(postErr(srv, `{ todos { id text done tags user { name } } users { id } search(term: "t") { __typename } }`)); p != "" {
				errs <- p
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}

	var data struct {
		Todos []struct {
			ID string `json:"id"`
		} `json:"todos"`
	}
	if err := json.Unmarshal(post(t, srv, `{ todos { id } }`).Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Todos) != 2+workers {
		t.Errorf("todos = %d, want %d", len(data.Todos), 2+workers)
	}
	seen := map[string]bool{}
	for _, td := range data.Todos {
		if seen[td.ID] {
			t.Errorf("duplicate id %s under concurrency", td.ID)
		}
		seen[td.ID] = true
	}
}

// TestDocsExampleFields pins the fields added for the docs examples, over
// HTTP so header plumbing and error shaping are what a client sees.
func TestDocsExampleFields(t *testing.T) {
	srv := newTestServer(t)

	t.Run("fail gives partial data and coded errors", func(t *testing.T) {
		res := post(t, srv, `{ ok: ping bad: fail(message: "nope", code: "FORBIDDEN") dflt: fail }`)
		if !strings.Contains(string(res.Data), `"ok":"pong from gqlgenapi"`) || !strings.Contains(string(res.Data), `"bad":null`) {
			t.Errorf("data = %s", res.Data)
		}
		got := map[string]string{}
		for _, e := range res.Errors {
			if len(e.Path) != 1 {
				t.Fatalf("error without a one-element path: %+v", e)
			}
			got[e.Path[0]] = e.Message + "|" + e.Extensions.Code
		}
		if got["bad"] != "nope|FORBIDDEN" || got["dflt"] != "fail called|MOCK_FAILURE" {
			t.Errorf("errors = %v", got)
		}
	})

	t.Run("requestHeader echoes case-insensitively", func(t *testing.T) {
		body := strings.NewReader(`{"query":"{ a: requestHeader(name: \"authorization\") m: requestHeader(name: \"X-None\") }"}`)
		req, err := http.NewRequest(http.MethodPost, srv.URL+queryPath, body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer demo-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		var res gqlResponse
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			t.Fatal(err)
		}
		if string(res.Data) != `{"a":"Bearer demo-token","m":null}` {
			t.Errorf("data = %s errors = %+v", res.Data, res.Errors)
		}
	})

	t.Run("node resolves typed ids only", func(t *testing.T) {
		res := post(t, srv, `{
			a: node(id: "Todo:1") { __typename id }
			b: node(id: "User:2") { __typename id }
			c: node(id: "1") { id }
			d: node(id: "Todo:999") { id }
			e: node(id: "Nope:1") { id }
		}`)
		want := `{"a":{"__typename":"Todo","id":"1"},"b":{"__typename":"User","id":"2"},"c":null,"d":null,"e":null}`
		if len(res.Errors) > 0 || string(res.Data) != want {
			t.Errorf("data = %s errors = %+v", res.Data, res.Errors)
		}
	})

	t.Run("parent self-reference", func(t *testing.T) {
		res := post(t, srv, `{ c: todo(id: "2") { parent { id parent { id } } } top: todo(id: "1") { parent { id } } }`)
		want := `{"c":{"parent":{"id":"1","parent":null}},"top":{"parent":null}}`
		if len(res.Errors) > 0 || string(res.Data) != want {
			t.Errorf("data = %s errors = %+v", res.Data, res.Errors)
		}
	})
}
