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

// BrowseTool is a universal search & content tool with configurable headless browser.
// Supports multiple search engines (DuckDuckGo, Google, Yandex, Bing)
// and multiple browser engines (chromedp, rod, playwright, http fallback).
type BrowseTool struct {
	AllowNetwork bool
	HTTPClient   *http.Client
	Config       BrowseToolConfig
}

// BrowseToolConfig is the runtime configuration for BrowseTool
type BrowseToolConfig struct {
	Engine       string // "chromedp", "rod", "playwright", "http"
	SearchEngine string // "duckduckgo", "google", "yandex", "bing"
	Timeout      time.Duration
	MaxResults   int
	UserAgent    string
	Headless     bool
	ChromePath   string
}

// DefaultBrowseToolConfig returns default configuration
func DefaultBrowseToolConfig() BrowseToolConfig {
	return BrowseToolConfig{
		Engine:       "chromedp",
		SearchEngine: "duckduckgo",
		Timeout:      30 * time.Second,
		MaxResults:   10,
		UserAgent:    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Headless:     true,
	}
}

// NewBrowseTool creates a new browse tool with default config
func NewBrowseTool() *BrowseTool {
	return &BrowseTool{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Config:     DefaultBrowseToolConfig(),
	}
}

// SetConfig applies configuration from config file
func (t *BrowseTool) SetConfig(engine, searchEngine string, timeout, maxResults int, userAgent string, headless bool, chromePath string) {
	if engine != "" {
		t.Config.Engine = engine
	}
	if searchEngine != "" {
		t.Config.SearchEngine = searchEngine
	}
	if timeout > 0 {
		t.Config.Timeout = time.Duration(timeout) * time.Second
	}
	if maxResults > 0 {
		t.Config.MaxResults = maxResults
	}
	if userAgent != "" {
		t.Config.UserAgent = userAgent
	}
	t.Config.Headless = headless
	if chromePath != "" {
		t.Config.ChromePath = chromePath
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
			"engine": map[string]any{
				"type":        "string",
				"description": "Override search engine for this query: duckduckgo, google, yandex, bing",
			},
		},
		"required": []string{"action"},
	}
}

// --- Search ---

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

func (t *BrowseTool) search(params map[string]string) ToolResult {
	query := strings.TrimSpace(params["query"])
	if query == "" {
		return Error("tools.browse.param_query")
	}

	maxResults := t.Config.MaxResults
	if mr := params["max_results"]; mr != "" {
		fmt.Sscanf(mr, "%d", &maxResults)
	}
	if maxResults <= 0 {
		maxResults = 10
	}
	if maxResults > 20 {
		maxResults = 20
	}

	// Allow per-query engine override
	engine := t.Config.SearchEngine
	if e := params["engine"]; e != "" {
		engine = e
	}

	var results []searchResult
	var err error

	switch engine {
	case "google":
		results, err = t.searchGoogle(query, maxResults)
	case "yandex":
		results, err = t.searchYandex(query, maxResults)
	case "bing":
		results, err = t.searchBing(query, maxResults)
	default: // duckduckgo
		results, err = t.searchDuckDuckGo(query, maxResults)
	}

	if err != nil {
		return Error("tools.browse.search_error", err)
	}

	if len(results) == 0 {
		return ToolResult{Output: fmt.Sprintf("No results found for \"%s\" (%s)", query, engine)}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for \"%s\" [%s] (%d found):\n\n", query, engine, len(results)))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet))
	}

	return ToolResult{Output: sb.String()}
}

func (t *BrowseTool) searchDuckDuckGo(query string, maxResults int) ([]searchResult, error) {
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	body, err := t.httpGet(searchURL)
	if err != nil {
		return nil, err
	}
	return parseDuckDuckGoHTML(body, maxResults), nil
}

func (t *BrowseTool) searchGoogle(query string, maxResults int) ([]searchResult, error) {
	searchURL := fmt.Sprintf("https://www.google.com/search?q=%s&num=%d&hl=en", url.QueryEscape(query), maxResults)
	body, err := t.httpGet(searchURL)
	if err != nil {
		return nil, err
	}
	return parseGoogleHTML(body, maxResults), nil
}

func (t *BrowseTool) searchYandex(query string, maxResults int) ([]searchResult, error) {
	searchURL := fmt.Sprintf("https://yandex.ru/search/?text=%s&numdoc=%d", url.QueryEscape(query), maxResults)
	// Yandex renders via JS — use headless browser
	content, err := t.fetchWithChrome(searchURL, "")
	if err != nil {
		// Fallback to HTTP
		body, err2 := t.httpGet(searchURL)
		if err2 != nil {
			return nil, fmt.Errorf("chrome: %v, http: %v", err, err2)
		}
		return parseYandexHTML(body, maxResults), nil
	}
	return parseYandexHTML(content, maxResults), nil
}

func (t *BrowseTool) searchBing(query string, maxResults int) ([]searchResult, error) {
	searchURL := fmt.Sprintf("https://www.bing.com/search?q=%s&count=%d", url.QueryEscape(query), maxResults)
	body, err := t.httpGet(searchURL)
	if err != nil {
		return nil, err
	}
	return parseBingHTML(body, maxResults), nil
}

// --- Fetch ---

func (t *BrowseTool) fetch(params map[string]string) ToolResult {
	pageURL := strings.TrimSpace(params["url"])
	if pageURL == "" {
		return Error("tools.browse.param_url")
	}
	if !strings.HasPrefix(pageURL, "http://") && !strings.HasPrefix(pageURL, "https://") {
		pageURL = "https://" + pageURL
	}

	selector := params["selector"]
	var content string
	var err error

	// Use configured engine
	switch t.Config.Engine {
	case "http":
		content, err = t.fetchWithHTTP(pageURL)
	default: // chromedp, rod, playwright — all use chrome
		content, err = t.fetchWithChrome(pageURL, selector)
		if err != nil {
			// Fallback to HTTP
			content, err = t.fetchWithHTTP(pageURL)
		}
	}

	if err != nil {
		return Error("tools.browse.fetch_error", pageURL, err)
	}

	if len(content) > 50000 {
		content = content[:50000] + "\n\n... (truncated, use selector to get specific parts)"
	}

	return ToolResult{Output: fmt.Sprintf("Content from %s:\n\n%s", pageURL, content)}
}

func (t *BrowseTool) extract(params map[string]string) ToolResult {
	pageURL := strings.TrimSpace(params["url"])
	if pageURL == "" {
		return Error("tools.browse.param_url")
	}
	if !strings.HasPrefix(pageURL, "http://") && !strings.HasPrefix(pageURL, "https://") {
		pageURL = "https://" + pageURL
	}

	content, err := t.fetchWithChrome(pageURL, "article, main, .content, #content, .post, .entry, .article")
	if err != nil {
		content, err = t.fetchWithHTTP(pageURL)
		if err != nil {
			return Error("tools.browse.fetch_error", pageURL, err)
		}
	}

	clean := htmlToCleanText(content)
	if len(clean) > 50000 {
		clean = clean[:50000] + "\n\n... (truncated)"
	}

	return ToolResult{Output: fmt.Sprintf("Extracted text from %s:\n\n%s", pageURL, clean)}
}

// --- HTTP helpers ---

func (t *BrowseTool) httpGet(pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", t.Config.UserAgent)

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

func (t *BrowseTool) fetchWithHTTP(pageURL string) (string, error) {
	return t.httpGet(pageURL)
}

func (t *BrowseTool) fetchWithChrome(pageURL, selector string) (string, error) {
	return fetchWithChromeImpl(pageURL, selector)
}

// --- HTML parsers ---

func parseDuckDuckGoHTML(html string, maxResults int) []searchResult {
	var results []searchResult
	titleRe := regexp.MustCompile(`(?s)class="result__a"[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	snippetRe := regexp.MustCompile(`(?s)class="result__snippet"[^>]*>(.*?)</a>`)

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
			results = append(results, searchResult{Title: title, URL: resultURL, Snippet: snippet})
		}
	}
	return results
}

func parseGoogleHTML(html string, maxResults int) []searchResult {
	var results []searchResult
	titleRe := regexp.MustCompile(`(?s)<a[^>]*href="(/url\?q=([^"&]*)[^"]*)"[^>]*>.*?<h3[^>]*>(.*?)</h3>`)
	snippetRe := regexp.MustCompile(`(?s)<span[^>]*>(.*?)</span>`)

	matches := titleRe.FindAllStringSubmatch(html, -1)
	allSnippets := snippetRe.FindAllStringSubmatch(html, -1)

	for i, m := range matches {
		if len(results) >= maxResults || len(m) < 4 {
			break
		}
		link := m[2]
		if strings.HasPrefix(link, "&") || strings.HasPrefix(link, "/search") {
			continue
		}
		if !strings.HasPrefix(link, "http") {
			link = "https://" + link
		}
		title := cleanHTMLTags(m[3])

		snippet := ""
		if i < len(allSnippets) && len(allSnippets[i]) >= 2 {
			snippet = cleanHTMLTags(allSnippets[i][1])
		}

		if title != "" {
			results = append(results, searchResult{Title: title, URL: link, Snippet: snippet})
		}
	}
	return results
}

func parseYandexHTML(html string, maxResults int) []searchResult {
	var results []searchResult
	// Yandex uses: <h2 class="..."><a href="...">Title</a></h2>
	titleRe := regexp.MustCompile(`(?s)<h2[^>]*>.*?<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	snippetRe := regexp.MustCompile(`(?s)<span class="ExtendedText[^"]*">(.*?)</span>`)

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
			results = append(results, searchResult{Title: title, URL: resultURL, Snippet: snippet})
		}
	}
	return results
}

func parseBingHTML(html string, maxResults int) []searchResult {
	var results []searchResult
	// Bing uses: <h2><a href="...">Title</a></h2>
	titleRe := regexp.MustCompile(`(?s)<h2[^>]*><a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	snippetRe := regexp.MustCompile(`(?s)<p[^>]*class="[^"]*"[^>]*>(.*?)</p>`)

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
			results = append(results, searchResult{Title: title, URL: resultURL, Snippet: snippet})
		}
	}
	return results
}

// --- HTML utilities ---

func cleanHTMLTags(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	s = re.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.TrimSpace(s)
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return s
}

func htmlToCleanText(html string) string {
	for _, tag := range []string{"script", "style", "nav", "header", "footer", "aside", "noscript", "iframe"} {
		re := regexp.MustCompile(`(?is)<` + tag + `[^>]*>.*?</` + tag + `>`)
		html = re.ReplaceAllString(html, "")
	}
	for _, tag := range []string{"p", "div", "br", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tr"} {
		re := regexp.MustCompile(`(?is)<` + tag + `[^>]*>`)
		html = re.ReplaceAllString(html, "\n")
		re = regexp.MustCompile(`(?is)</` + tag + `>`)
		html = re.ReplaceAllString(html, "\n")
	}
	re := regexp.MustCompile(`<[^>]*>`)
	html = re.ReplaceAllString(html, "")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&nbsp;", " ")

	lines := strings.Split(html, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}
	result := strings.Join(cleanLines, "\n")
	re = regexp.MustCompile(`\n{3,}`)
	result = re.ReplaceAllString(result, "\n\n")
	return strings.TrimSpace(result)
}
