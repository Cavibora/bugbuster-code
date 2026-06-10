package tools

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ===================== Browse search engine tests =====================

func TestBrowseTool_SearchDuckDuckGo(t *testing.T) {
	html := `
	<div class="result__body">
		<a rel="nofollow" class="result__a" href="https://example.com">Example Result</a>
		<a class="result__snippet" href="https://example.com">This is a snippet</a>
	</div>
	<div class="result__body">
		<a rel="nofollow" class="result__a" href="https://test.org">Test Org</a>
		<a class="result__snippet" href="https://test.org">Another snippet</a>
	</div>
	`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, html)
	}))
	defer server.Close()

	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.Engine = "http"
	tool.HTTPClient = server.Client()

	// Test the parser directly
	results := parseDuckDuckGoHTML(html, 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Example Result" {
		t.Errorf("expected 'Example Result', got '%s'", results[0].Title)
	}
}

func TestBrowseTool_SearchWithHTTPEngine(t *testing.T) {
	// Create a mock server that returns DuckDuckGo-like HTML
	html := `
	<div class="result__body">
		<a rel="nofollow" class="result__a" href="https://golang.org">The Go Programming Language</a>
		<a class="result__snippet" href="https://golang.org">Go is an open source programming language</a>
	</div>
	`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, html)
	}))
	defer server.Close()

	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.Engine = "http"

	// Test httpGet via the server
	body, err := tool.httpGet(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(body, "Go Programming Language") {
		t.Errorf("expected 'Go Programming Language' in body")
	}
}

func TestBrowseTool_SearchGoogleParser(t *testing.T) {
	html := `
	<div class="g">
		<a href="/url?q=https://golang.org/pkg/&amp;sa=U">
			<h3 class="LC20lb">Go Packages</h3>
		</a>
		<span class="st">Package documentation for Go</span>
	</div>
	`
	results := parseGoogleHTML(html, 10)
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
	if results[0].Title != "Go Packages" {
		t.Errorf("expected 'Go Packages', got '%s'", results[0].Title)
	}
}

func TestBrowseTool_SearchYandexParser(t *testing.T) {
	html := `
	<div class="serp-item">
		<h2><a href="https://go.dev">Go Dev</a></h2>
		<span class="ExtendedText__text">Go development environment</span>
	</div>
	`
	results := parseYandexHTML(html, 10)
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
	if results[0].Title != "Go Dev" {
		t.Errorf("expected 'Go Dev', got '%s'", results[0].Title)
	}
}

func TestBrowseTool_SearchBingParser(t *testing.T) {
	html := `
	<li class="b_algo">
		<h2><a href="https://rust-lang.org">Rust Programming Language</a></h2>
		<p class="b_caption">A language empowering everyone</p>
	</li>
	`
	results := parseBingHTML(html, 10)
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
	if results[0].Title != "Rust Programming Language" {
		t.Errorf("expected 'Rust Programming Language', got '%s'", results[0].Title)
	}
}

func TestBrowseTool_SearchEngineOverride(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.SearchEngine = "duckduckgo"

	// Override to google
	result := tool.Execute(map[string]string{
		"action": "search",
		"query":  "test",
		"engine": "google",
	})
	// Will fail at network level, but should not fail with "unknown action"
	if result.Error != "" && strings.Contains(result.Error, "unknown") {
		t.Error("should not fail with unknown action for engine override")
	}
}

func TestBrowseTool_SearchMaxResultsParam(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.Engine = "http"

	// Test with max_results parameter
	result := tool.Execute(map[string]string{
		"action":      "search",
		"query":      "test",
		"max_results": "5",
	})
	// Will fail at network level, but should not crash
	_ = result
}

func TestBrowseTool_SearchEmptyQuery(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{
		"action": "search",
		"query":  "",
	})
	if result.Error == "" {
		t.Error("expected error for empty query")
	}
}

func TestBrowseTool_SearchNoQuery(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{
		"action": "search",
	})
	if result.Error == "" {
		t.Error("expected error for missing query")
	}
}

func TestBrowseTool_SearchNoResults(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.Engine = "http"

	// Use a server that returns no results
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "<html><body>No results here</body></html>")
	}))
	defer server.Close()

	results := parseDuckDuckGoHTML("<html><body>No results</body></html>", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty HTML, got %d", len(results))
	}

	// Test the search result formatting when no results
	result := tool.search(map[string]string{
		"query": "nonexistent_xyz_12345",
	})
	// searchDuckDuckGo will try to make a real HTTP request which will fail
	// That's fine — we just want to make sure it doesn't crash
	_ = result
}

func TestBrowseTool_FetchWithHTTPEngine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body><h1>Hello Fetch</h1></body></html>")
	}))
	defer server.Close()

	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.Engine = "http"
	tool.HTTPClient = server.Client()

	// Test fetch with HTTP engine
	content, err := tool.fetchWithHTTP(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "Hello Fetch") {
		t.Errorf("expected 'Hello Fetch' in content")
	}
}

func TestBrowseTool_ExtractWithURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><article><h1>Article Title</h1><p>Article content here.</p></article></body></html>`)
	}))
	defer server.Close()

	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.Engine = "http"
	tool.HTTPClient = server.Client()

	// Test extract with HTTP engine (will try Chrome first, then fall back)
	// Since Chrome is not available in test, it will fall back to HTTP
	result := tool.Execute(map[string]string{
		"action": "extract",
		"url":    server.URL,
	})
	// The extract function tries Chrome first, then falls back to HTTP
	// In test environment, Chrome is not available, so it should use HTTP
	_ = result
}

func TestBrowseTool_FetchURLNormalization(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.Engine = "http"

	// Test URL without protocol — should be normalized to https://
	result := tool.Execute(map[string]string{
		"action": "fetch",
		"url":    "example.invalid",
	})
	// Should fail because domain doesn't exist, but URL should be normalized
	if result.Error != "" && !strings.Contains(result.Error, "https://example.invalid") {
		t.Errorf("expected URL normalization, got error: %s", result.Error)
	}
}

func TestBrowseTool_FetchMissingURL(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{
		"action": "fetch",
	})
	if result.Error == "" {
		t.Error("expected error for missing URL")
	}
}

func TestBrowseTool_ExtractMissingURL(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{
		"action": "extract",
	})
	if result.Error == "" {
		t.Error("expected error for missing URL in extract")
	}
}

func TestBrowseTool_SearchHTTPEngine(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.Engine = "http"

	// With HTTP engine, search should use httpGet directly
	result := tool.Execute(map[string]string{
		"action": "search",
		"query":  "test",
	})
	// Will fail at network level (can't reach DuckDuckGo), but should not crash
	_ = result
}

func TestBrowseTool_httpGet_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Hello, World!")
	}))
	defer server.Close()

	tool := NewBrowseTool()
	tool.HTTPClient = server.Client()

	body, err := tool.httpGet(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got '%s'", body)
	}
}

func TestBrowseTool_httpGet_Error(t *testing.T) {
	tool := NewBrowseTool()

	// Try to connect to a non-existent server
	_, err := tool.httpGet("http://127.0.0.1:1")
	if err == nil {
		t.Error("expected error for connection refused")
	}
}

func TestBrowseTool_httpGet_UserAgent(t *testing.T) {
	var receivedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	tool := NewBrowseTool()
	tool.HTTPClient = server.Client()

	_, err := tool.httpGet(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedUA != tool.Config.UserAgent {
		t.Errorf("expected User-Agent '%s', got '%s'", tool.Config.UserAgent, receivedUA)
	}
}

func TestBrowseTool_httpGet_InvalidURL(t *testing.T) {
	tool := NewBrowseTool()

	_, err := tool.httpGet("not-a-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}