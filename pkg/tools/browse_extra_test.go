package tools

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bugbuster-code/pkg/i18n"
)

func init() {
	i18n.Init("en")
}

// --- Helper: create a BrowseTool with HTTP client pointing to a test server ---

func newBrowseToolWithClient(client *http.Client) *BrowseTool {
	return &BrowseTool{
		AllowNetwork: true,
		HTTPClient:   client,
		Config:       DefaultBrowseToolConfig(),
	}
}

// --- search() method tests ---

func TestSearchDuckDuckGoWithMockServer(t *testing.T) {
	html := `
	<div class="result__body">
		<a rel="nofollow" class="result__a" href="https://example.com/ddg1">DuckDuckGo Result 1</a>
		<a class="result__snippet" href="https://example.com/ddg1">Snippet for DDG result 1</a>
	</div>
	<div class="result__body">
		<a rel="nofollow" class="result__a" href="https://example.com/ddg2">DuckDuckGo Result 2</a>
		<a class="result__snippet" href="https://example.com/ddg2">Snippet for DDG result 2</a>
	</div>
	`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	body, err := tool.httpGet(ts.URL + "/?q=test")
	if err != nil {
		t.Fatalf("httpGet failed: %v", err)
	}
	results := parseDuckDuckGoHTML(body, 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "DuckDuckGo Result 1" {
		t.Fatalf("expected 'DuckDuckGo Result 1', got '%s'", results[0].Title)
	}
	if results[1].Title != "DuckDuckGo Result 2" {
		t.Fatalf("expected 'DuckDuckGo Result 2', got '%s'", results[1].Title)
	}
}

func TestSearchGoogleWithMockServer(t *testing.T) {
	html := `
	<div class="g">
		<a href="/url?q=https://google-example.com/page1&amp;sa=U">
			<h3 class="LC20lb">Google Result 1</h3>
		</a>
		<div class="VwiC3b">Google snippet 1</div>
	</div>
	<div class="g">
		<a href="/url?q=https://google-example.com/page2&amp;sa=U">
			<h3 class="LC20lb">Google Result 2</h3>
		</a>
		<div class="VwiC3b">Google snippet 2</div>
	</div>
	`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	body, err := tool.httpGet(ts.URL + "/search?q=test")
	if err != nil {
		t.Fatalf("httpGet failed: %v", err)
	}
	results := parseGoogleHTML(body, 10)
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
	if results[0].Title != "Google Result 1" {
		t.Fatalf("expected 'Google Result 1', got '%s'", results[0].Title)
	}
}

func TestSearchYandexWithMockServer(t *testing.T) {
	html := `
	<div class="serp-item">
		<h2 class="organic__title"><a href="https://yandex-example.com/1">Yandex Result 1</a></h2>
		<span class="ExtendedText__text">Yandex snippet 1</span>
	</div>
	<div class="serp-item">
		<h2 class="organic__title"><a href="https://yandex-example.com/2">Yandex Result 2</a></h2>
		<span class="ExtendedText__text">Yandex snippet 2</span>
	</div>
	`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	body, err := tool.httpGet(ts.URL + "/search/?text=test")
	if err != nil {
		t.Fatalf("httpGet failed: %v", err)
	}
	results := parseYandexHTML(body, 10)
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
	if results[0].Title != "Yandex Result 1" {
		t.Fatalf("expected 'Yandex Result 1', got '%s'", results[0].Title)
	}
}

func TestSearchBingWithMockServer(t *testing.T) {
	html := `
	<li class="b_algo">
		<h2><a href="https://bing-example.com/1">Bing Result 1</a></h2>
		<div class="b_caption"><p>Bing snippet 1</p></div>
	</li>
	<li class="b_algo">
		<h2><a href="https://bing-example.com/2">Bing Result 2</a></h2>
		<div class="b_caption"><p>Bing snippet 2</p></div>
	</li>
	`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	body, err := tool.httpGet(ts.URL + "/search?q=test")
	if err != nil {
		t.Fatalf("httpGet failed: %v", err)
	}
	results := parseBingHTML(body, 10)
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
	if results[0].Title != "Bing Result 1" {
		t.Fatalf("expected 'Bing Result 1', got '%s'", results[0].Title)
	}
}

func TestSearchWithEngineOverride(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.SearchEngine = "duckduckgo"

	// Per-query engine override via params["engine"]
	result := tool.search(map[string]string{
		"query":  "test",
		"engine": "duckduckgo",
	})
	// Will fail at network level, but should not produce "unknown engine" error
	if result.Error != "" && strings.Contains(result.Error, "unknown") {
		t.Fatalf("engine override should not produce unknown engine error, got: %s", result.Error)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.search(map[string]string{"query": ""})
	if result.Error == "" {
		t.Fatal("expected error for empty query")
	}
}

func TestSearchWhitespaceQuery(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.search(map[string]string{"query": "   "})
	if result.Error == "" {
		t.Fatal("expected error for whitespace-only query")
	}
}

func TestSearchMaxResultsFromParams(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<a class="result__a" href="https://example.com">R</a><a class="result__snippet" href="https://example.com">S</a>`))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())

	// Test that max_results param is parsed without crash
	_ = tool.search(map[string]string{
		"query":       "test",
		"max_results": "3",
	})
}

func TestSearchNoResults(t *testing.T) {
	results := parseDuckDuckGoHTML(`<html><body>No results here</body></html>`, 10)
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

// --- fetch() method tests ---

func TestFetchWithHTTPEngine(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><h1>Hello World</h1><p>Content here</p></body></html>`))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	result := tool.fetch(map[string]string{
		"url": ts.URL,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Hello World") {
		t.Fatalf("expected output to contain 'Hello World', got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Content from") {
		t.Fatalf("expected output to contain 'Content from', got: %s", result.Output)
	}
}

func TestFetchWithURLNormalization(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>Normalized</body></html>`))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	result := tool.fetch(map[string]string{
		"url": ts.URL,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Normalized") {
		t.Fatalf("expected output to contain 'Normalized', got: %s", result.Output)
	}
}

func TestFetchURLWithoutProtocol(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.Engine = "http"

	// URL without protocol should be normalized to https://
	result := tool.fetch(map[string]string{
		"url": "example.invalid",
	})
	if result.Error == "" {
		return // somehow succeeded
	}
	if !strings.Contains(result.Error, "https://example.invalid") {
		t.Fatalf("URL should be normalized to https://, got: %s", result.Error)
	}
}

func TestFetchEmptyURL(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.fetch(map[string]string{"url": ""})
	if result.Error == "" {
		t.Fatal("expected error for empty URL")
	}
}

func TestFetchTruncation(t *testing.T) {
	largeContent := strings.Repeat("A", 60000)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(largeContent))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	result := tool.fetch(map[string]string{
		"url": ts.URL,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "truncated") {
		t.Fatalf("expected output to be truncated, got length: %d", len(result.Output))
	}
}

func TestFetchDefaultEngineFallsBackToHTTP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>Fallback content</body></html>`))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	// Default engine is "chromedp" — it will try Chrome first, fail, then fall back to HTTP
	tool.Config.Engine = "chromedp"

	result := tool.fetch(map[string]string{
		"url": ts.URL,
	})
	// Should succeed via HTTP fallback even though Chrome is not available
	if result.Error != "" {
		t.Fatalf("expected fallback to HTTP to succeed, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Fallback content") {
		t.Fatalf("expected output to contain 'Fallback content', got: %s", result.Output)
	}
}

// --- extract() method tests ---

func TestExtractWithArticleTag(t *testing.T) {
	html := `<html><body>
		<nav>Navigation</nav>
		<article>
			<h1>Article Title</h1>
			<p>Article content paragraph one.</p>
			<p>Article content paragraph two.</p>
		</article>
		<footer>Footer</footer>
	</body></html>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	result := tool.extract(map[string]string{
		"url": ts.URL,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Article Title") {
		t.Fatalf("expected output to contain 'Article Title', got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Article content paragraph one") {
		t.Fatalf("expected output to contain 'Article content paragraph one', got: %s", result.Output)
	}
}

func TestExtractWithMainTag(t *testing.T) {
	html := `<html><body>
		<header>Header</header>
		<main>
			<h1>Main Content</h1>
			<p>This is the main content area.</p>
		</main>
		<aside>Sidebar</aside>
	</body></html>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	result := tool.extract(map[string]string{
		"url": ts.URL,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Main Content") {
		t.Fatalf("expected output to contain 'Main Content', got: %s", result.Output)
	}
}

func TestExtractWithContentClass(t *testing.T) {
	html := `<html><body>
		<div class="content">
			<h1>Content Title</h1>
			<p>Content inside .content div.</p>
		</div>
	</body></html>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	result := tool.extract(map[string]string{
		"url": ts.URL,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Content Title") {
		t.Fatalf("expected output to contain 'Content Title', got: %s", result.Output)
	}
}

func TestExtractStripsScriptAndStyle(t *testing.T) {
	html := `<html><body>
		<script>var x = 1;</script>
		<style>body { color: red; }</style>
		<main>
			<h1>Clean Title</h1>
			<p>Clean paragraph.</p>
		</main>
	</body></html>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	result := tool.extract(map[string]string{
		"url": ts.URL,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if strings.Contains(result.Output, "var x") {
		t.Fatal("output should not contain script content")
	}
	if strings.Contains(result.Output, "color: red") {
		t.Fatal("output should not contain style content")
	}
	if !strings.Contains(result.Output, "Clean Title") {
		t.Fatalf("expected output to contain 'Clean Title', got: %s", result.Output)
	}
}

func TestExtractEmptyURL(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.extract(map[string]string{"url": ""})
	if result.Error == "" {
		t.Fatal("expected error for empty URL")
	}
}

func TestExtractURLNormalization(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.Engine = "http"

	result := tool.extract(map[string]string{"url": "example.invalid"})
	if result.Error == "" {
		return // somehow succeeded
	}
	if !strings.Contains(result.Error, "https://example.invalid") {
		t.Fatalf("URL should be normalized to https://, got: %s", result.Error)
	}
}

func TestExtractTruncation(t *testing.T) {
	largeContent := `<html><body><main>` + strings.Repeat("A", 60000) + `</main></body></html>`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(largeContent))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	result := tool.extract(map[string]string{
		"url": ts.URL,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "truncated") {
		t.Fatal("expected output to be truncated")
	}
}

// --- httpGet() method tests ---

func TestHTTPGetSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello from server"))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())

	body, err := tool.httpGet(ts.URL + "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != "Hello from server" {
		t.Fatalf("expected 'Hello from server', got '%s'", body)
	}
}

func TestHTTPGetUserAgent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("User-Agent: " + r.Header.Get("User-Agent")))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())

	body, err := tool.httpGet(ts.URL + "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(body, tool.Config.UserAgent) {
		t.Fatalf("expected User-Agent header '%s' to be sent, got: %s", tool.Config.UserAgent, body)
	}
}

func TestHTTPGetCustomUserAgent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("User-Agent: " + r.Header.Get("User-Agent")))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.UserAgent = "MyCustomBot/1.0"

	body, err := tool.httpGet(ts.URL + "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(body, "MyCustomBot/1.0") {
		t.Fatalf("expected custom User-Agent, got: %s", body)
	}
}

func TestHTTPGet404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())

	body, err := tool.httpGet(ts.URL + "/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error for 404: %v", err)
	}
	// httpGet returns the body regardless of status code
	if body != "Not Found" {
		t.Fatalf("expected 'Not Found', got '%s'", body)
	}
}

func TestHTTPGet500(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())

	body, err := tool.httpGet(ts.URL + "/error")
	if err != nil {
		t.Fatalf("unexpected error for 500: %v", err)
	}
	if body != "Internal Server Error" {
		t.Fatalf("expected 'Internal Server Error', got '%s'", body)
	}
}

func TestHTTPGetConnectionError(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	_, err := tool.httpGet("http://127.0.0.1:1/impossible")
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

func TestHTTPGetInvalidURL(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	_, err := tool.httpGet("://invalid-url")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestHTTPGetLargeResponse(t *testing.T) {
	largeBody := strings.Repeat("X", 100000)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(largeBody))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())

	body, err := tool.httpGet(ts.URL + "/large")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(body) != 100000 {
		t.Fatalf("expected 100000 bytes, got %d", len(body))
	}
}

func TestHTTPGetContentType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html><body>OK</body></html>"))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())

	body, err := tool.httpGet(ts.URL + "/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(body, "OK") {
		t.Fatalf("expected 'OK' in body, got '%s'", body)
	}
}

// --- fetchWithChrome() tests ---

func TestFetchWithChromeNotAvailable(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	_, err := tool.fetchWithChrome("https://example.com", "")
	if err == nil {
		t.Skip("Chrome is available in this environment, skipping test for Chrome unavailability")
	}
}

func TestFetchWithChromeEmptyURL(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	_, err := tool.fetchWithChrome("", "")
	if err == nil {
		t.Fatal("expected error for empty URL with Chrome")
	}
}

// --- Description() method tests ---

func TestBrowseToolDescription(t *testing.T) {
	tool := NewBrowseTool()
	desc := tool.Description()
	if desc == "" {
		t.Fatal("expected non-empty description")
	}
}

func TestBrowseToolDescriptionNonEmpty(t *testing.T) {
	tool := NewBrowseTool()
	desc := tool.Description()
	if len(desc) == 0 {
		t.Fatal("Description() should return a non-empty string")
	}
	// The description should come from i18n.T, so it should be a translated string
	// It should not be the raw key
	if desc == "tools.browse.description" {
		t.Fatal("Description() should return a translated string, not the raw key")
	}
}

// --- fetchWithHTTP() tests ---

func TestFetchWithHTTP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("HTTP fetch content"))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())

	content, err := tool.fetchWithHTTP(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "HTTP fetch content" {
		t.Fatalf("expected 'HTTP fetch content', got '%s'", content)
	}
}

func TestFetchWithHTTPErrors(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	_, err := tool.fetchWithHTTP("http://127.0.0.1:1/impossible")
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

// --- Execute() integration tests ---

func TestExecuteFetchWithMockServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><h1>Fetched Page</h1></body></html>`))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	result := tool.Execute(map[string]string{
		"action": "fetch",
		"url":    ts.URL,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Fetched Page") {
		t.Fatalf("expected output to contain 'Fetched Page', got: %s", result.Output)
	}
}

func TestExecuteExtractWithMockServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><article><h1>Extracted Article</h1><p>Article text.</p></article></body></html>`))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	result := tool.Execute(map[string]string{
		"action": "extract",
		"url":    ts.URL,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Extracted Article") {
		t.Fatalf("expected output to contain 'Extracted Article', got: %s", result.Output)
	}
}

func TestExecuteSearchAction(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{
		"action": "search",
		"query":  "test",
	})
	// Will fail at network level, but should not be "unknown action"
	if result.Error != "" && strings.Contains(result.Error, "unknown") {
		t.Fatalf("search action should not be unknown, got: %s", result.Error)
	}
}

func TestExecuteFindAction(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{
		"action": "find",
		"query":  "test",
	})
	if result.Error != "" && strings.Contains(result.Error, "unknown") {
		t.Fatalf("find action should not be unknown, got: %s", result.Error)
	}
}

func TestExecuteRenderAction(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>Rendered content</body></html>`))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	result := tool.Execute(map[string]string{
		"action": "render",
		"url":    ts.URL,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Rendered content") {
		t.Fatalf("expected output to contain 'Rendered content', got: %s", result.Output)
	}
}

func TestExecuteOpenAction(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>Opened page</body></html>`))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	result := tool.Execute(map[string]string{
		"action": "open",
		"url":    ts.URL,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Opened page") {
		t.Fatalf("expected output to contain 'Opened page', got: %s", result.Output)
	}
}

func TestExecuteReadabilityAction(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><article><h1>Readable</h1><p>Content</p></article></body></html>`))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	result := tool.Execute(map[string]string{
		"action": "readability",
		"url":    ts.URL,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Readable") {
		t.Fatalf("expected output to contain 'Readable', got: %s", result.Output)
	}
}

// --- Parser edge case tests ---

func TestParseDuckDuckGoHTMLWithSnippetHTML(t *testing.T) {
	html := `<a class="result__a" href="https://example.com">Title with <b>bold</b></a>
	         <a class="result__snippet" href="https://example.com">Snippet with <em>emphasis</em></a>`
	results := parseDuckDuckGoHTML(html, 10)
	if len(results) < 1 {
		t.Fatal("expected at least 1 result")
	}
	if !strings.Contains(results[0].Title, "Title with bold") {
		t.Fatalf("expected HTML tags to be stripped from title, got '%s'", results[0].Title)
	}
	if !strings.Contains(results[0].Snippet, "emphasis") {
		t.Fatalf("expected HTML tags to be stripped from snippet, got '%s'", results[0].Snippet)
	}
}

func TestParseGoogleHTMLPattern2(t *testing.T) {
	html := `<a href="/url?q=https://mobile-example.com/page&amp;sa=U"><h3>Mobile Result</h3></a>`
	results := parseGoogleHTML(html, 10)
	if len(results) < 1 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Title != "Mobile Result" {
		t.Fatalf("expected 'Mobile Result', got '%s'", results[0].Title)
	}
}

func TestParseYandexHTMLPattern2(t *testing.T) {
	html := `<a data-cy="organic-title" href="https://yandex-example.com/page">Yandex DataCy Result</a>`
	results := parseYandexHTML(html, 10)
	if len(results) < 1 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Title != "Yandex DataCy Result" {
		t.Fatalf("expected 'Yandex DataCy Result', got '%s'", results[0].Title)
	}
}

func TestParseYandexHTMLSkipsInternalLinks(t *testing.T) {
	html := `<h2><a href="/internal/page">Internal Link</a></h2>
	         <h2><a href="https://external.com/page">External Link</a></h2>`
	results := parseYandexHTML(html, 10)
	for _, r := range results {
		if !strings.HasPrefix(r.URL, "http") {
			t.Fatalf("should not include non-http URL: %s", r.URL)
		}
	}
}

func TestParseBingHTMLSkipsInternalLinks(t *testing.T) {
	html := `<li class="b_algo"><h2><a href="https://bing.com/search">Bing Internal</a></h2></li>
	         <li class="b_algo"><h2><a href="https://microsoft.com/page">Microsoft Internal</a></h2></li>
	         <li class="b_algo"><h2><a href="https://external.com/page">External Result</a></h2></li>`
	results := parseBingHTML(html, 10)
	for _, r := range results {
		if strings.Contains(r.URL, "bing.com") || strings.Contains(r.URL, "microsoft.com") {
			t.Fatalf("should not include Bing/Microsoft internal URL: %s", r.URL)
		}
	}
}

func TestParseBingHTMLPattern2(t *testing.T) {
	html := `<h2><a h="https://bing-alt.com/page" href="https://bing-alt.com/page">Bing Alt Result</a></h2>`
	results := parseBingHTML(html, 10)
	if len(results) < 1 {
		t.Fatal("expected at least 1 result")
	}
}

// --- htmlToCleanText edge cases ---

func TestHTMLToCleanTextWithNavHeaderFooter(t *testing.T) {
	html := `<html><body>
		<nav><a href="/home">Home</a><a href="/about">About</a></nav>
		<header>Site Header</header>
		<main>
			<h1>Main Title</h1>
			<p>Main content here.</p>
		</main>
		<footer>Copyright 2024</footer>
	</body></html>`

	result := htmlToCleanText(html)
	if strings.Contains(result, "Home") {
		t.Error("should not contain nav content")
	}
	if strings.Contains(result, "Site Header") {
		t.Error("should not contain header content")
	}
	if strings.Contains(result, "Copyright") {
		t.Error("should not contain footer content")
	}
	if !strings.Contains(result, "Main Title") {
		t.Error("should contain main title")
	}
	if !strings.Contains(result, "Main content here") {
		t.Error("should contain main content")
	}
}

func TestHTMLToCleanTextWithAside(t *testing.T) {
	html := `<html><body>
		<aside>Sidebar content</aside>
		<main><p>Real content</p></main>
	</body></html>`

	result := htmlToCleanText(html)
	if strings.Contains(result, "Sidebar") {
		t.Error("should not contain aside content")
	}
	if !strings.Contains(result, "Real content") {
		t.Error("should contain main content")
	}
}

func TestHTMLToCleanTextWithIframe(t *testing.T) {
	html := `<html><body>
		<iframe src="https://ads.example.com"></iframe>
		<main><p>Content</p></main>
	</body></html>`

	result := htmlToCleanText(html)
	if !strings.Contains(result, "Content") {
		t.Error("should contain main content")
	}
}

func TestHTMLToCleanTextWithNoscript(t *testing.T) {
	html := `<html><body>
		<noscript>Enable JavaScript</noscript>
		<main><p>Visible content</p></main>
	</body></html>`

	result := htmlToCleanText(html)
	if strings.Contains(result, "Enable JavaScript") {
		t.Error("should not contain noscript content")
	}
	if !strings.Contains(result, "Visible content") {
		t.Error("should contain visible content")
	}
}

// --- searchDuckDuckGo/searchGoogle/searchYandex/searchBing with mock server ---

func TestSearchDuckDuckGoWithHTTPClient(t *testing.T) {
	html := `
	<a class="result__a" href="https://ddg-test.com/1">DDG Test 1</a>
	<a class="result__snippet" href="https://ddg-test.com/1">DDG snippet 1</a>
	<a class="result__a" href="https://ddg-test.com/2">DDG Test 2</a>
	<a class="result__snippet" href="https://ddg-test.com/2">DDG snippet 2</a>
	`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())

	body, err := tool.httpGet(ts.URL)
	if err != nil {
		t.Fatalf("httpGet failed: %v", err)
	}
	results := parseDuckDuckGoHTML(body, 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestSearchGoogleWithHTTPClient(t *testing.T) {
	html := `
	<div class="g">
		<a href="/url?q=https://g-test.com/1&amp;sa=U"><h3 class="LC20lb">G Test 1</h3></a>
		<div class="VwiC3b">G snippet 1</div>
	</div>
	`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	body, err := tool.httpGet(ts.URL)
	if err != nil {
		t.Fatalf("httpGet failed: %v", err)
	}
	results := parseGoogleHTML(body, 10)
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
}

func TestSearchYandexWithHTTPClient(t *testing.T) {
	html := `
	<h2><a href="https://ya-test.com/1">Ya Test 1</a></h2>
	<span class="ExtendedText__text">Ya snippet 1</span>
	`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	body, err := tool.httpGet(ts.URL)
	if err != nil {
		t.Fatalf("httpGet failed: %v", err)
	}
	results := parseYandexHTML(body, 10)
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
}

func TestSearchBingWithHTTPClient(t *testing.T) {
	html := `
	<li class="b_algo"><h2><a href="https://bing-test.com/1">Bing Test 1</a></h2>
	<div class="b_caption"><p>Bing snippet 1</p></div></li>
	`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	}))
	defer ts.Close()

	tool := newBrowseToolWithClient(ts.Client())
	tool.Config.Engine = "http"

	body, err := tool.httpGet(ts.URL)
	if err != nil {
		t.Fatalf("httpGet failed: %v", err)
	}
	results := parseBingHTML(body, 10)
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
}

// --- search() with max_results parameter ---

func TestSearchMaxResultsZero(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	// max_results=0 should be treated as default (10)
	_ = tool.search(map[string]string{
		"query":       "test",
		"max_results": "0",
	})
}

func TestSearchMaxResultsNegative(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	_ = tool.search(map[string]string{
		"query":       "test",
		"max_results": "-5",
	})
}

// --- cleanHTMLTags edge cases ---

func TestCleanHTMLTagsEmpty(t *testing.T) {
	result := cleanHTMLTags("")
	if result != "" {
		t.Fatalf("expected empty string, got '%s'", result)
	}
}

func TestCleanHTMLTagsNoTags(t *testing.T) {
	result := cleanHTMLTags("plain text")
	if result != "plain text" {
		t.Fatalf("expected 'plain text', got '%s'", result)
	}
}

func TestCleanHTMLTagsNested(t *testing.T) {
	result := cleanHTMLTags("<div><p><b>Bold</b> text</p></div>")
	if result != "Bold text" {
		t.Fatalf("expected 'Bold text', got '%s'", result)
	}
}

func TestCleanHTMLTagsEntities(t *testing.T) {
	result := cleanHTMLTags("&amp; &lt; &gt; &quot; &#39;")
	expected := `& < > " '`
	if result != expected {
		t.Fatalf("expected '%s', got '%s'", expected, result)
	}
}

func TestCleanHTMLTagsNbsp(t *testing.T) {
	result := cleanHTMLTags("hello&nbsp;world")
	if result != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", result)
	}
}

func TestCleanHTMLTagsWhitespace(t *testing.T) {
	result := cleanHTMLTags("  hello   world  ")
	if result != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", result)
	}
}