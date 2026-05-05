// web_search.go — search the web via DuckDuckGo's HTML endpoint.
//
// We hit https://html.duckduckgo.com/html/?q=<query>. That endpoint
// renders a static HTML results page (no JavaScript), which is exactly
// what we want for parsing. No API key, no account, no telemetry.
//
// Result shape: top N hits, each with title + URL + snippet. Returned as
// a numbered list the user can read or hand to the LLM as context.
//
// The HTTP client + base URL are fields on WebSearch so tests can swap in
// a fixture server without touching the network.

package tool

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// WebSearch performs a DuckDuckGo HTML search.
type WebSearch struct {
	// HTTPClient lets tests inject a fake transport. nil = http.DefaultClient
	// with a 15-second timeout.
	HTTPClient *http.Client
	// BaseURL overrides the DuckDuckGo endpoint. nil = the real one.
	BaseURL string
	// MaxResults caps the returned hits. 0 = 5.
	MaxResults int
	// UserAgent sent in the request. Empty = a sane default.
	UserAgent string
}

// Name implements Tool.
func (w *WebSearch) Name() string { return "web_search" }

// Execute implements Tool. Required param: "query" (string).
func (w *WebSearch) Execute(params map[string]any) (Result, error) {
	query := stringParam(params, "query")
	if query == "" {
		return Result{}, errors.New("web_search requires a non-empty 'query' param")
	}

	hits, err := w.search(query)
	if err != nil {
		return Result{}, err
	}
	if len(hits) == 0 {
		return Result{
			Output: "No results.",
			Mascot: "ghost",
		}, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Top %d for %q:\n\n", len(hits), query)
	for i, h := range hits {
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, h.Title, h.URL)
		if h.Snippet != "" {
			fmt.Fprintf(&b, "   %s\n", h.Snippet)
		}
		b.WriteString("\n")
	}
	return Result{Output: b.String(), Mascot: "idle"}, nil
}

// SearchHit is one row from the DuckDuckGo results page.
type SearchHit struct {
	Title   string
	URL     string
	Snippet string
}

func (w *WebSearch) baseURL() string {
	if w.BaseURL != "" {
		return w.BaseURL
	}
	return "https://html.duckduckgo.com/html/"
}

func (w *WebSearch) httpClient() *http.Client {
	if w.HTTPClient != nil {
		return w.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (w *WebSearch) maxResults() int {
	if w.MaxResults > 0 {
		return w.MaxResults
	}
	return 5
}

func (w *WebSearch) userAgent() string {
	if w.UserAgent != "" {
		return w.UserAgent
	}
	return "CHEWAgent/0.1 (+https://github.com/chewgumlabs/CHEWAgent)"
}

// search hits DuckDuckGo, parses the HTML, returns up to MaxResults hits.
func (w *WebSearch) search(query string) ([]SearchHit, error) {
	form := url.Values{}
	form.Set("q", query)

	req, err := http.NewRequest(http.MethodPost, w.baseURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", w.userAgent())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := w.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return parseDDGHTML(body, w.maxResults())
}

// parseDDGHTML walks the DuckDuckGo HTML results and pulls out the top N
// hits. Each result lives inside <div class="result"> with a
// <a class="result__a" href="..."> for the title/url and a
// <a class="result__snippet"> for the snippet. We unwrap the redirect
// URL DuckDuckGo wraps results in (uddg= query param).
func parseDDGHTML(body []byte, max int) ([]SearchHit, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	var hits []SearchHit
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if len(hits) >= max {
			return
		}
		if n.Type == html.ElementNode && n.Data == "div" && hasClass(n, "result") {
			h := extractHit(n)
			if h.URL != "" && h.Title != "" {
				hits = append(hits, h)
			}
			return // results don't nest
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return hits, nil
}

func extractHit(n *html.Node) SearchHit {
	var h SearchHit
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			if hasClass(n, "result__a") && h.Title == "" {
				h.Title = strings.TrimSpace(textContent(n))
				href := attr(n, "href")
				h.URL = unwrapDDGRedirect(href)
			}
			if hasClass(n, "result__snippet") && h.Snippet == "" {
				h.Snippet = strings.TrimSpace(textContent(n))
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return h
}

// unwrapDDGRedirect undoes DuckDuckGo's /l/?uddg=<encoded>&... wrapper so
// the user sees the actual destination URL.
func unwrapDDGRedirect(href string) string {
	if href == "" {
		return ""
	}
	// DuckDuckGo HTML often returns hrefs starting with "//duckduckgo.com/l/"
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	if u.Host == "duckduckgo.com" && strings.HasPrefix(u.Path, "/l/") {
		if uddg := u.Query().Get("uddg"); uddg != "" {
			if decoded, err := url.QueryUnescape(uddg); err == nil {
				return decoded
			}
		}
	}
	return href
}

// ---- tiny html helpers ----

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	classes := attr(n, "class")
	if classes == "" {
		return false
	}
	for _, c := range strings.Fields(classes) {
		if c == class {
			return true
		}
	}
	return false
}

func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(textContent(c))
	}
	return b.String()
}
