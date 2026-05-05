// web_fetch.go — fetch a URL and return its text content.
//
// HTTPS only by default (no plain http unless explicitly enabled). The
// response body is parsed as HTML, the <script>/<style>/<noscript> nodes
// are dropped, and the remaining text is returned. For non-HTML
// (text/plain, application/json, etc.) the body is returned verbatim.
//
// Size cap: 1 MB by default. Anything larger gets truncated and the user
// is told. The 1 MB cap exists because long pages eat the LLM's context
// window — and because we want web_fetch to feel snappy, not turn into a
// download manager.

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

// WebFetch implements the web_fetch verb.
type WebFetch struct {
	HTTPClient *http.Client
	UserAgent  string
	// MaxBytes caps the response body. 0 = 1 MB.
	MaxBytes int64
	// AllowHTTP permits plain http:// URLs. Default false (HTTPS only).
	AllowHTTP bool
}

// Name implements Tool.
func (w *WebFetch) Name() string { return "web_fetch" }

// Execute implements Tool. Required param: "url" (string).
func (w *WebFetch) Execute(params map[string]any) (Result, error) {
	raw := stringParam(params, "url")
	if raw == "" {
		return Result{}, errors.New("web_fetch requires a non-empty 'url' param")
	}
	u, err := validateURL(raw, w.AllowHTTP)
	if err != nil {
		return Result{}, err
	}

	body, contentType, truncated, err := w.fetch(u.String())
	if err != nil {
		return Result{}, err
	}

	text := extractText(body, contentType)
	if truncated {
		text += "\n\n[... page truncated; see the original URL for the full content ...]"
	}

	header := fmt.Sprintf("Fetched %s\n\n", u.String())
	return Result{
		Output: header + text,
		Mascot: "idle",
	}, nil
}

// validateURL parses + checks the URL. We refuse plain http:// unless the
// caller opted in, and we reject anything that looks like a non-public
// scheme.
func validateURL(raw string, allowHTTP bool) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	switch u.Scheme {
	case "https":
		// always allowed
	case "http":
		if !allowHTTP {
			return nil, errors.New("plain http:// URLs are blocked; use https:// (or set AllowHTTP)")
		}
	case "":
		return nil, errors.New("URL is missing a scheme; expected https://...")
	default:
		return nil, fmt.Errorf("unsupported URL scheme %q (only https:// is allowed)", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("URL is missing a host")
	}
	return u, nil
}

func (w *WebFetch) httpClient() *http.Client {
	if w.HTTPClient != nil {
		return w.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (w *WebFetch) userAgent() string {
	if w.UserAgent != "" {
		return w.UserAgent
	}
	return "CHEWAgent/0.1 (+https://github.com/chewgumlabs/CHEWAgent)"
}

func (w *WebFetch) maxBytes() int64 {
	if w.MaxBytes > 0 {
		return w.MaxBytes
	}
	return 1 << 20 // 1 MB
}

// fetch performs the GET, capped at MaxBytes. Returns body, content-type,
// truncated flag, error.
func (w *WebFetch) fetch(target string) ([]byte, string, bool, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, "", false, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", w.userAgent())
	req.Header.Set("Accept", "text/html,text/plain,application/json,*/*")

	resp, err := w.httpClient().Do(req)
	if err != nil {
		return nil, "", false, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", false, fmt.Errorf("http status %d", resp.StatusCode)
	}

	cap := w.maxBytes()
	limited := io.LimitReader(resp.Body, cap+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", false, fmt.Errorf("read body: %w", err)
	}
	truncated := false
	if int64(len(body)) > cap {
		body = body[:cap]
		truncated = true
	}
	return body, resp.Header.Get("Content-Type"), truncated, nil
}

// extractText returns the readable text of body. For HTML responses we
// drop script/style/noscript and collapse whitespace. For non-HTML we
// return the body as-is.
func extractText(body []byte, contentType string) string {
	if !strings.Contains(strings.ToLower(contentType), "html") {
		return string(body)
	}
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return string(body)
	}
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "iframe", "svg":
				return
			case "br", "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6":
				// add a newline boundary so tags don't collapse into one line
				b.WriteByte('\n')
			}
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return collapseWhitespace(b.String())
}

// collapseWhitespace reduces runs of whitespace to single spaces, but
// preserves paragraph breaks (double newlines).
func collapseWhitespace(s string) string {
	// Split on blank lines, trim each line, drop empties, rejoin.
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
