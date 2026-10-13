package wiretap

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTransportRecordsAndForwards pins the two promises the examples rely
// on: the recorded request matches what was sent, and the server still
// receives the full, unmodified body.
func TestTransportRecordsAndForwards(t *testing.T) {
	const body = `{"query":"{ ping }","variables":{"a":1}}`
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()

	tap := &Transport{}
	if _, ok := tap.Last(); ok {
		t.Fatal("Last reported a request before any was sent")
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Test", "yes")
	resp, err := (&http.Client{Transport: tap}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if gotBody != body {
		t.Errorf("server got body %q, want %q", gotBody, body)
	}
	last, ok := tap.Last()
	if !ok || last.Query != "{ ping }" || last.Variables["a"] != float64(1) || last.Header.Get("X-Test") != "yes" {
		t.Errorf("recorded %+v (ok=%t)", last, ok)
	}
	if tap.Count() != 1 {
		t.Errorf("Count = %d, want 1", tap.Count())
	}
}

// TestTransportRejectsNonJSONBody: a body that is not a GraphQL JSON request
// is reported, not silently recorded as empty.
func TestTransportRejectsNonJSONBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:1", strings.NewReader("not json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&Transport{}).RoundTrip(req); err == nil || !strings.Contains(err.Error(), "decode GraphQL body") {
		t.Fatalf("err = %v, want a decode error", err)
	}
}
