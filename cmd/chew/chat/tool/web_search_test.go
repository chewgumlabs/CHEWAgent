package tool

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fixtureHTML mimics a stripped-down DuckDuckGo HTML response with three
// results, so the parser has something realistic to chew on.
const fixtureHTML = `<!DOCTYPE html>
<html><body>
<div class="serp__results">
  <div class="result">
    <h2 class="result__title">
      <a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Ffirst&amp;rut=x">First result</a>
    </h2>
    <a class="result__snippet" href="...">First snippet text.</a>
  </div>
  <div class="result">
    <h2 class="result__title">
      <a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fsecond&amp;rut=x">Second result</a>
    </h2>
    <a class="result__snippet" href="...">Second snippet text.</a>
  </div>
  <div class="result">
    <h2 class="result__title">
      <a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fthird&amp;rut=x">Third result</a>
    </h2>
    <a class="result__snippet" href="...">Third snippet text.</a>
  </div>
</div>
</body></html>`

func TestWebSearch_ParsesHits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mimic DDG's POST contract.
		if r.Method != http.MethodPost {
			http.Error(w, "POST expected", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(fixtureHTML))
	}))
	defer srv.Close()

	tool := &WebSearch{BaseURL: srv.URL, MaxResults: 5}
	res, err := tool.Execute(map[string]any{"query": "anything"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{
		"Top 3 for \"anything\":",
		"1. First result",
		"https://example.com/first",
		"First snippet text.",
		"3. Third result",
		"https://example.com/third",
	} {
		if !strings.Contains(res.Output, want) {
			t.Errorf("expected %q in output, missing.\noutput:\n%s", want, res.Output)
		}
	}
	// DDG redirect URLs should be unwrapped — never leak to the user.
	if strings.Contains(res.Output, "duckduckgo.com/l/") {
		t.Errorf("output should not contain DDG redirect URLs:\n%s", res.Output)
	}
}

func TestWebSearch_RespectsMaxResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(fixtureHTML))
	}))
	defer srv.Close()

	tool := &WebSearch{BaseURL: srv.URL, MaxResults: 2}
	res, err := tool.Execute(map[string]any{"query": "x"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "Top 2 for \"x\":") {
		t.Errorf("expected exactly 2 hits, got:\n%s", res.Output)
	}
	if strings.Contains(res.Output, "Third result") {
		t.Errorf("output should not include the 3rd result when MaxResults=2")
	}
}

func TestWebSearch_EmptyQueryRejected(t *testing.T) {
	tool := &WebSearch{}
	_, err := tool.Execute(map[string]any{"query": "   "})
	if err == nil {
		t.Errorf("empty query should be rejected")
	}
}

func TestWebSearch_NoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><div class="no-results">nope</div></body></html>`))
	}))
	defer srv.Close()

	tool := &WebSearch{BaseURL: srv.URL}
	res, err := tool.Execute(map[string]any{"query": "obscure"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "No results.") {
		t.Errorf("expected 'No results.' message, got: %s", res.Output)
	}
	if res.Mascot != "ghost" {
		t.Errorf("no-results should set mascot=ghost, got %s", res.Mascot)
	}
}

func TestWebSearch_HTTPErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	tool := &WebSearch{BaseURL: srv.URL}
	_, err := tool.Execute(map[string]any{"query": "x"})
	if err == nil {
		t.Errorf("expected error on 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 in err, got: %v", err)
	}
}

func TestUnwrapDDGRedirect(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Ffoo&rut=x", "https://example.com/foo"},
		{"https://example.com/direct", "https://example.com/direct"},
		{"", ""},
		{"//duckduckgo.com/l/?nope=1", "https://duckduckgo.com/l/?nope=1"},
	}
	for _, c := range cases {
		got := unwrapDDGRedirect(c.in)
		if got != c.want {
			t.Errorf("unwrap(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
