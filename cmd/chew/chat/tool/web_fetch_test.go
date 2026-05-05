package tool

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebFetch_StripsScriptAndStyle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head>
<style>body { color: red; }</style>
<script>alert("nope");</script>
</head><body>
<h1>Hello</h1>
<p>World</p>
<script>alert("also nope");</script>
</body></html>`))
	}))
	defer srv.Close()

	tool := &WebFetch{}
	// Override scheme guard for httptest URL (it's http://).
	tool.AllowHTTP = true
	res, err := tool.Execute(map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "Hello") {
		t.Errorf("expected Hello in output, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "World") {
		t.Errorf("expected World in output, got: %s", res.Output)
	}
	if strings.Contains(res.Output, "alert") {
		t.Errorf("script content leaked into output:\n%s", res.Output)
	}
	if strings.Contains(res.Output, "color: red") {
		t.Errorf("style content leaked into output:\n%s", res.Output)
	}
}

func TestWebFetch_RejectsHTTPByDefault(t *testing.T) {
	tool := &WebFetch{}
	_, err := tool.Execute(map[string]any{"url": "http://example.com/"})
	if err == nil {
		t.Errorf("plain http should be rejected by default")
	}
	if !strings.Contains(err.Error(), "http://") {
		t.Errorf("error should mention http://, got: %v", err)
	}
}

func TestWebFetch_RejectsBadSchemes(t *testing.T) {
	tool := &WebFetch{}
	cases := []string{
		"ftp://example.com/",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"data:text/html,<h1>hi</h1>",
	}
	for _, raw := range cases {
		_, err := tool.Execute(map[string]any{"url": raw})
		if err == nil {
			t.Errorf("%q should be rejected", raw)
		}
	}
}

func TestWebFetch_TruncatesLargeBodies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		// Write 200 KB of text.
		_, _ = w.Write([]byte(strings.Repeat("A", 200*1024)))
	}))
	defer srv.Close()

	tool := &WebFetch{
		AllowHTTP: true,
		MaxBytes:  1024, // 1 KB cap forces truncation
	}
	res, err := tool.Execute(map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "page truncated") {
		t.Errorf("expected truncation note, got: %s", res.Output[:200])
	}
}

func TestWebFetch_PreservesNonHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hello":"world"}`))
	}))
	defer srv.Close()

	tool := &WebFetch{AllowHTTP: true}
	res, err := tool.Execute(map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, `{"hello":"world"}`) {
		t.Errorf("JSON body should pass through verbatim, got: %s", res.Output)
	}
}

func TestWebFetch_SurfacesHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	tool := &WebFetch{AllowHTTP: true}
	_, err := tool.Execute(map[string]any{"url": srv.URL})
	if err == nil {
		t.Errorf("404 should surface as error")
	}
}

func TestWebFetch_EmptyURLRejected(t *testing.T) {
	tool := &WebFetch{}
	_, err := tool.Execute(map[string]any{"url": "   "})
	if err == nil {
		t.Errorf("empty URL should be rejected")
	}
}

func TestWebFetch_OutputIncludesURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>ok</body></html>`))
	}))
	defer srv.Close()

	tool := &WebFetch{AllowHTTP: true}
	res, err := tool.Execute(map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Output, "Fetched "+srv.URL) {
		t.Errorf("output should announce the URL, got: %s", res.Output)
	}
}
