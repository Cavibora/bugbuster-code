package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"bugbuster-code/pkg/i18n"
)

// BrowseTool is a universal search & content tool with headless browser support.
// It can search the web, render JavaScript-heavy pages, and extract clean text.
// No external APIs or provider search tools needed.
type BrowseTool struct {
	AllowNetwork bool
	HTTPClient   *http.Client
}

// NewBrowseTool creates a new browse tool
func NewBrowseTool() *BrowseTool {
	return &BrowseTool{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (t *BrowseTool) Name() string { return "browse" }

func (t *BrowseTool) Description() string {
	return i18n.T("tools.browse.description")
}

func (t *BrowseTool) Execute(params map[string]string) ToolResult {
	if !t.AllowNetwork {
		return Error("tools.browse.network_disabled")
	}

	action := params["action"]
	switch action {
	case "search", "find":
		return t.search(params)
	case "fetch", "render", "open":
		return t.fetch(params)
	case "extract", "readability":
		return t.extract(params)
	default:
		return Error("tools.browse.unknown_action", action)
	}
}

func (t *BrowseTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"search", "fetch", "extract"},
				"description": i18n.T("tools.browse.param_action_desc"),
			},
			"query": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.browse.param_query_desc"),
			},
			"url": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.browse.param_url_desc"),
			},
			"selector": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.browse.param_selector_desc"),
			},
			"max_results": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.browse.param_max_results_desc"),
			},
		},
		"required": []string{"action"},
	}
}

// searchResult represents a single search result
type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

// search performs a web search using DuckDuckGo HTML (no API key needed)
func (t *BrowseTool) search(params map[string]string) ToolResult {
	query := strings.TrimSpace(params["query"])
	if query == "" {
		return Error("tools.browse.param_query")
	}

	maxResults := 10
	if mr := params["max_results"]; mr != "" {
		fmt.Sscanf(mr, "%d", &maxResults)
	}
	if maxResults <= 0 {
		maxResults = 10
	}
	if maxResults > 20 {
		maxResults = 20
	}

	// Use DuckDuckGo HTML search (no JS, no API key)
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(context.Background(), "GET", searchURL, nil)
	if err != nil {
		return Error("tools.browse.search_error", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return Error("tools.browse.search_error", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return Error("tools.browse.search_error", fmt.Sprintf("HTTP %d", resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Error("tools.browse.search_error", err)
	}

	results := parseDuckDuckGoHTML(string(body), maxResults)
	if len(results) == 0 {
		return Success("tools.browse.no_results", query)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for \"%s\" (%d found):\n\n", query, len(results)))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet))
	}

	return ToolResult{Output: sb.String()}
}

// fetch renders a page using headless Chrome (handles JS, AJAX)
func (t *BrowseTool) fetch(params map[string]string) ToolResult {
	pageURL := strings.TrimSpace(params["url"])
	if pageURL == "" {
		return Error("tools.browse.param_url")
	}

	// Validate URL
	if !strings.HasPrefix(pageURL, "http://") && !strings.HasPrefix(pageURL, "https://") {
		pageURL = "https://" + pageURL
	}

	selector := params["selector"]

	// Try headless Chrome first
	content, err := t.fetchWithChrome(pageURL, selector)
	if err != nil {
		// Fallback to simple HTTP fetch
		content, err = t.fetchWithHTTP(pageURL)
		if err != nil {
			return Error("tools.browse.fetch_error", pageURL, err)
		}
	}

	// Truncate if too large
	if len(content) > 50000 {
		content = content[:50000] + "\n\n... (truncated, use selector to get specific parts)"
	}

	return ToolResult{Output: fmt.Sprintf("Content from %s:\n\n%s", pageURL, content)}
}

// extract fetches a page and extracts clean text (no HTML, no JS, no navigation)
func (t *BrowseTool) extract(params map[string]string) ToolResult {
	pageURL := strings.TrimSpace(params["url"])
	if pageURL == "" {
		return Error("tools.browse.param_url")
	}

	if !strings.HasPrefix(pageURL, "http://") && !strings.HasPrefix(pageURL, "https://") {
		pageURL = "https://" + pageURL
	}

	// Fetch with Chrome for full JS rendering
	content, err := t.fetchWithChrome(pageURL, "article, main, .content, #content, .post, .entry, .article")
	if err != nil {
		// Fallback to HTTP
		content, err = t.fetchWithHTTP(pageURL)
		if err != nil {
			return Error("tools.browse.fetch_error", pageURL, err)
		}
	}

	// Clean HTML to text
	clean := htmlToCleanText(content)

	// Truncate
	if len(clean) > 50000 {
		clean = clean[:50000] + "\n\n... (truncated)"
	}

	return ToolResult{Output: fmt.Sprintf("Extracted text from %s:\n\n%s", pageURL, clean)}
}

// fetchWithHTTP does a simple HTTP GET (no JS rendering)
func (t *BrowseTool) fetchWithHTTP(pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// fetchWithChrome uses chromedp to render a page with full JS support
func (t *BrowseTool) fetchWithChrome(pageURL, selector string) (string, error) {
	// Try to import chromedp dynamically — if not available, return error
	// This is handled by the build tag system
	return fetchWithChromeImpl(pageURL, selector)
}

// parseDuckDuckGoHTML parses DuckDuckGo HTML search results
func parseDuckDuckGoHTML(html string, maxResults int) []searchResult {
	var results []searchResult

	// DuckDuckGo HTML uses:
	// <a rel="nofollow" class="result__a" href="...">Title</a>
	// <a class="result__snippet" href="...">Snippet</a>

	// Extract result blocks — more flexible regex
	resultRe := regexp.MustCompile(`(?s)class="result[^"]*".*?class="result__body"`)
	titleRe := regexp.MustCompile(`(?s)class="result__a"[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	snippetRe := regexp.MustCompile(`(?s)class="result__snippet"[^>]*>(.*?)</a>`)

	// Find all title matches directly
	titleMatches := titleRe.FindAllStringSubmatch(html, -1)
	snippetMatches := snippetRe.FindAllStringSubmatch(html, -1)

	for i, tm := range titleMatches {
		if len(results) >= maxResults {
			break
		}
		if len(tm) < 3 {
			continue
		}
		resultURL := tm[1]
		title := cleanHTMLTags(tm[2])

		snippet := ""
		if i < len(snippetMatches) && len(snippetMatches[i]) >= 2 {
			snippet = cleanHTMLTags(snippetMatches[i][1])
		}

		if title != "" && resultURL != "" {
			results = append(results, searchResult{
				Title:   title,
				URL:     resultURL,
				Snippet: snippet,
			})
		}
	}

	_ = resultRe // unused but kept for reference

	return results
}

// cleanHTMLTags removes HTML tags from text
func cleanHTMLTags(s string) string {
	// Remove HTML tags
	re := regexp.MustCompile(`<[^>]*>`)
	s = re.ReplaceAllString(s, "")

	// Decode HTML entities
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")

	// Clean whitespace
	s = strings.TrimSpace(s)
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")

	return s
}

// htmlToCleanText converts HTML to clean readable text
func htmlToCleanText(html string) string {
	// Remove script and style tags with content
	re := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	html = re.ReplaceAllString(html, "")
	re = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	html = re.ReplaceAllString(html, "")

	// Remove nav, header, footer, sidebar
	for _, tag := range []string{"nav", "header", "footer", "aside", "noscript", "iframe"} {
		re = regexp.MustCompile(`(?is)<` + tag + `[^>]*>.*?</` + tag + `>`)
		html = re.ReplaceAllString(html, "")
	}

	// Convert block elements to newlines
	for _, tag := range []string{"p", "div", "br", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tr"} {
		re = regexp.MustCompile(`(?is)<` + tag + `[^>]*>`)
		html = re.ReplaceAllString(html, "\n")
		re = regexp.MustCompile(`(?is)</` + tag + `>`)
		html = re.ReplaceAllString(html, "\n")
	}

	// Remove all remaining HTML tags
	re = regexp.MustCompile(`<[^>]*>`)
	html = re.ReplaceAllString(html, "")

	// Decode HTML entities
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&nbsp;", " ")

	// Clean up whitespace
	lines := strings.Split(html, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	result := strings.Join(cleanLines, "\n")

	// Limit consecutive newlines to 2
	re = regexp.MustCompile(`\n{3,}`)
	result = re.ReplaceAllString(result, "\n\n")

	return strings.TrimSpace(result)
}
