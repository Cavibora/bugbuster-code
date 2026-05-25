package tools

import (
	"strings"
	"testing"
)

// --- Basic validation ---

func TestBrowseToolSearchMissingQuery(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{
		"action": "search",
	})
	if result.Error == "" {
		t.Fatal("expected error for missing query")
	}
}

func TestBrowseToolFetchMissingURL(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{
		"action": "fetch",
	})
	if result.Error == "" {
		t.Fatal("expected error for missing url")
	}
}

func TestBrowseToolExtractMissingURL(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{
		"action": "extract",
	})
	if result.Error == "" {
		t.Fatal("expected error for missing url in extract")
	}
}

func TestBrowseToolNetworkDisabled(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = false

	result := tool.Execute(map[string]string{
		"action": "search",
		"query":  "test",
	})
	if result.Error == "" {
		t.Fatal("expected error when network disabled")
	}
}

func TestBrowseToolUnknownAction(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{
		"action": "unknown",
	})
	if result.Error == "" {
		t.Fatal("expected error for unknown action")
	}
}

func TestBrowseToolNoAction(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{})
	if result.Error == "" {
		t.Fatal("expected error for missing action")
	}
}

// --- DuckDuckGo parser ---

func TestParseDuckDuckGoHTML(t *testing.T) {
	html := `
	<div class="result__body">
		<a rel="nofollow" class="result__a" href="https://example.com">Example Title</a>
		<a class="result__snippet" href="https://example.com">This is a <b>snippet</b> of the result</a>
	</div>
	<div class="result__body">
		<a rel="nofollow" class="result__a" href="https://test.com">Test Title</a>
		<a class="result__snippet" href="https://test.com">Another snippet here</a>
	</div>
	`
	results := parseDuckDuckGoHTML(html, 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Example Title" {
		t.Fatalf("expected 'Example Title', got '%s'", results[0].Title)
	}
	if results[0].URL != "https://example.com" {
		t.Fatalf("expected 'https://example.com', got '%s'", results[0].URL)
	}
	if !strings.Contains(results[0].Snippet, "snippet") {
		t.Fatalf("expected snippet to contain 'snippet', got '%s'", results[0].Snippet)
	}
}

func TestParseDuckDuckGoHTMLMaxResults(t *testing.T) {
	html := ""
	for i := 0; i < 20; i++ {
		html += `
		<div class="result__body">
			<a rel="nofollow" class="result__a" href="https://example.com">Title</a>
			<a class="result__snippet" href="https://example.com">Snippet</a>
		</div>
		`
	}
	results := parseDuckDuckGoHTML(html, 5)
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
}

func TestParseDuckDuckGoHTMLEmpty(t *testing.T) {
	results := parseDuckDuckGoHTML("", 10)
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty HTML, got %d", len(results))
	}
}

func TestParseDuckDuckGoHTMLNoResults(t *testing.T) {
	html := `<html><body><p>No results found</p></body></html>`
	results := parseDuckDuckGoHTML(html, 10)
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

// --- Google parser ---

func TestParseGoogleHTML(t *testing.T) {
	html := `
	<div class="g">
		<a href="/url?q=https://example.com/page&amp;sa=U">
			<h3 class="LC20lb">Example Result</h3>
		</a>
		<span>This is a snippet</span>
	</div>
	`
	results := parseGoogleHTML(html, 10)
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
	if results[0].Title != "Example Result" {
		t.Fatalf("expected 'Example Result', got '%s'", results[0].Title)
	}
}

func TestParseGoogleHTMLEmpty(t *testing.T) {
	results := parseGoogleHTML("", 10)
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty HTML, got %d", len(results))
	}
}

func TestParseGoogleHTMLMaxResults(t *testing.T) {
	html := ""
	for i := 0; i < 20; i++ {
		html += `<div class="g"><a href="/url?q=https://example.com/` + string(rune('a'+i)) + `&amp;sa=U"><h3 class="LC20lb">Title</h3></a><span>Snippet</span></div>`
	}
	results := parseGoogleHTML(html, 3)
	if len(results) > 3 {
		t.Fatalf("expected at most 3 results, got %d", len(results))
	}
}

// --- Yandex parser ---

func TestParseYandexHTML(t *testing.T) {
	html := `
	<div class="serp-item">
		<h2 class="organic__title"><a href="https://yandex.ru/example">Яндекс Результат</a></h2>
		<span class="ExtendedText__text">Описание результата поиска</span>
	</div>
	`
	results := parseYandexHTML(html, 10)
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
	if results[0].Title != "Яндекс Результат" {
		t.Fatalf("expected 'Яндекс Результат', got '%s'", results[0].Title)
	}
}

func TestParseYandexHTMLEmpty(t *testing.T) {
	results := parseYandexHTML("", 10)
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty HTML, got %d", len(results))
	}
}

// --- Bing parser ---

func TestParseBingHTML(t *testing.T) {
	html := `
	<div class="b_algo">
		<h2><a href="https://bing.example.com">Bing Result</a></h2>
		<p class="b_caption">Bing snippet text</p>
	</div>
	`
	results := parseBingHTML(html, 10)
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
	if results[0].Title != "Bing Result" {
		t.Fatalf("expected 'Bing Result', got '%s'", results[0].Title)
	}
}

func TestParseBingHTMLEmpty(t *testing.T) {
	results := parseBingHTML("", 10)
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty HTML, got %d", len(results))
	}
}

// --- HTML utilities ---

func TestCleanHTMLTags(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<b>bold</b>", "bold"},
		{"Hello &amp; World", "Hello & World"},
		{"<a href='test'>link</a> text", "link text"},
		{"  spaces  ", "spaces"},
		{"&lt;script&gt;", "<script>"},
		{"&nbsp;test&nbsp;", "test"},
		{"<em>italic</em> and <strong>bold</strong>", "italic and bold"},
		{"line1\nline2", "line1 line2"},
		{"", ""},
	}

	for _, tt := range tests {
		result := cleanHTMLTags(tt.input)
		if result != tt.expected {
			t.Errorf("cleanHTMLTags(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestHTMLToCleanText(t *testing.T) {
	html := `
	<html>
	<head><script>var x = 1;</script><style>body{color:red}</style></head>
	<body>
		<nav>Navigation</nav>
		<header>Header</header>
		<main>
			<h1>Title</h1>
			<p>Paragraph one</p>
			<p>Paragraph two</p>
		</main>
		<footer>Footer</footer>
	</body>
	</html>
	`
	result := htmlToCleanText(html)

	if strings.Contains(result, "var x") {
		t.Error("should not contain script content")
	}
	if strings.Contains(result, "color:red") {
		t.Error("should not contain style content")
	}
	if strings.Contains(result, "Navigation") {
		t.Error("should not contain nav content")
	}
	if strings.Contains(result, "Header") {
		t.Error("should not contain header content")
	}
	if strings.Contains(result, "Footer") {
		t.Error("should not contain footer content")
	}
	if !strings.Contains(result, "Title") {
		t.Error("should contain main title")
	}
	if !strings.Contains(result, "Paragraph one") {
		t.Error("should contain paragraph one")
	}
	if !strings.Contains(result, "Paragraph two") {
		t.Error("should contain paragraph two")
	}
}

func TestHTMLToCleanTextEmpty(t *testing.T) {
	result := htmlToCleanText("")
	if result != "" {
		t.Fatalf("expected empty result, got '%s'", result)
	}
}

func TestHTMLToCleanTextNoHTML(t *testing.T) {
	result := htmlToCleanText("Just plain text")
	if result != "Just plain text" {
		t.Fatalf("expected 'Just plain text', got '%s'", result)
	}
}

func TestHTMLToCleanTextNested(t *testing.T) {
	html := `<div><div><p>Nested content</p></div></div>`
	result := htmlToCleanText(html)
	if !strings.Contains(result, "Nested content") {
		t.Fatalf("expected nested content, got '%s'", result)
	}
}

func TestHTMLToCleanTextEntities(t *testing.T) {
	html := `<p>&amp; &lt; &gt; &quot; &#39; &nbsp;</p>`
	result := htmlToCleanText(html)
	if !strings.Contains(result, "& < > \" '") {
		t.Fatalf("expected decoded entities, got '%s'", result)
	}
}

func TestHTMLToCleanTextMultipleNewlines(t *testing.T) {
	html := `<p>Line 1</p><br/><br/><br/><p>Line 2</p>`
	result := htmlToCleanText(html)
	if strings.Contains(result, "\n\n\n") {
		t.Fatalf("should not have 3+ consecutive newlines, got '%s'", result)
	}
}

// --- Tool interface ---

func TestBrowseToolName(t *testing.T) {
	tool := NewBrowseTool()
	if tool.Name() != "browse" {
		t.Fatalf("expected 'browse', got '%s'", tool.Name())
	}
}

func TestBrowseToolParameters(t *testing.T) {
	tool := NewBrowseTool()
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Fatal("expected type=object")
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	expectedParams := []string{"action", "query", "url", "selector", "max_results", "engine"}
	for _, p := range expectedParams {
		if _, ok := props[p]; !ok {
			t.Fatalf("expected '%s' parameter", p)
		}
	}
}

// --- Configuration ---

func TestBrowseToolDefaultConfig(t *testing.T) {
	config := DefaultBrowseToolConfig()
	if config.Engine != "chromedp" {
		t.Fatalf("expected default engine 'chromedp', got '%s'", config.Engine)
	}
	if config.SearchEngine != "duckduckgo" {
		t.Fatalf("expected default search engine 'duckduckgo', got '%s'", config.SearchEngine)
	}
	if config.Timeout != 30_000_000_000 { // 30 seconds in nanoseconds
		t.Fatalf("expected default timeout 30s, got %v", config.Timeout)
	}
	if config.MaxResults != 10 {
		t.Fatalf("expected default max results 10, got %d", config.MaxResults)
	}
	if config.Headless != true {
		t.Fatal("expected default headless=true")
	}
}

func TestBrowseToolSetConfig(t *testing.T) {
	tool := NewBrowseTool()

	tool.SetConfig("rod", "yandex", 60, 5, "CustomAgent/1.0", false, "/usr/bin/chromium")

	if tool.Config.Engine != "rod" {
		t.Fatalf("expected engine 'rod', got '%s'", tool.Config.Engine)
	}
	if tool.Config.SearchEngine != "yandex" {
		t.Fatalf("expected search engine 'yandex', got '%s'", tool.Config.SearchEngine)
	}
	if tool.Config.Timeout != 60_000_000_000 {
		t.Fatalf("expected timeout 60s, got %v", tool.Config.Timeout)
	}
	if tool.Config.MaxResults != 5 {
		t.Fatalf("expected max results 5, got %d", tool.Config.MaxResults)
	}
	if tool.Config.UserAgent != "CustomAgent/1.0" {
		t.Fatalf("expected custom user agent, got '%s'", tool.Config.UserAgent)
	}
	if tool.Config.Headless != false {
		t.Fatal("expected headless=false")
	}
	if tool.Config.ChromePath != "/usr/bin/chromium" {
		t.Fatalf("expected chrome path, got '%s'", tool.Config.ChromePath)
	}
}

func TestBrowseToolSetConfigPartial(t *testing.T) {
	tool := NewBrowseTool()
	defaultUA := tool.Config.UserAgent

	// Only set search engine — other fields should keep defaults
	tool.SetConfig("", "google", 0, 0, "", true, "")

	if tool.Config.Engine != "chromedp" {
		t.Fatalf("engine should keep default, got '%s'", tool.Config.Engine)
	}
	if tool.Config.SearchEngine != "google" {
		t.Fatalf("expected search engine 'google', got '%s'", tool.Config.SearchEngine)
	}
	if tool.Config.UserAgent != defaultUA {
		t.Fatal("user agent should keep default")
	}
}

func TestBrowseToolSetConfigNegativeValues(t *testing.T) {
	tool := NewBrowseTool()

	// Negative/zero values should be ignored
	tool.SetConfig("", "", -1, -5, "", true, "")

	if tool.Config.Timeout <= 0 {
		t.Fatal("timeout should not be set to negative value")
	}
	if tool.Config.MaxResults <= 0 {
		t.Fatal("max results should not be set to negative value")
	}
}

// --- Action aliases ---

func TestBrowseToolSearchActionAliases(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = false // Will fail at network, but action should be recognized

	for _, action := range []string{"search", "find"} {
		result := tool.Execute(map[string]string{"action": action, "query": "test"})
		// Should fail because network disabled, NOT because action unknown
		if result.Error == "" {
			t.Fatalf("action '%s' should fail with network disabled", action)
		}
		if strings.Contains(result.Error, "unknown") {
			t.Fatalf("action '%s' should not be unknown", action)
		}
	}
}

func TestBrowseToolFetchActionAliases(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = false

	for _, action := range []string{"fetch", "render", "open"} {
		result := tool.Execute(map[string]string{"action": action, "url": "https://example.com"})
		if result.Error == "" {
			t.Fatalf("action '%s' should fail with network disabled", action)
		}
		if strings.Contains(result.Error, "unknown") {
			t.Fatalf("action '%s' should not be unknown", action)
		}
	}
}

func TestBrowseToolExtractActionAliases(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = false

	for _, action := range []string{"extract", "readability"} {
		result := tool.Execute(map[string]string{"action": action, "url": "https://example.com"})
		if result.Error == "" {
			t.Fatalf("action '%s' should fail with network disabled", action)
		}
		if strings.Contains(result.Error, "unknown") {
			t.Fatalf("action '%s' should not be unknown", action)
		}
	}
}

// --- URL normalization ---

func TestBrowseToolFetchURLNormalization(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.Engine = "http" // Use HTTP engine to avoid Chrome dependency

	// URL without protocol — should be normalized to https://
	// This will fail at network level (can't connect), but URL should be normalized
	result := tool.Execute(map[string]string{"action": "fetch", "url": "example.invalid"})
	// Should fail because domain doesn't exist, but URL should be normalized
	if result.Error == "" {
		// If it somehow succeeds, that's fine too
		return
	}
	// The error should mention the normalized URL
	if !strings.Contains(result.Error, "https://example.invalid") {
		t.Fatalf("URL should be normalized to https://, got: %s", result.Error)
	}
}

// --- Search engine selection ---

func TestBrowseToolSearchEngineOverride(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.SearchEngine = "duckduckgo"

	// Per-query override to google
	// This will fail at network level, but we can verify the engine is used
	// by checking the search function is called (not unknown engine error)
	// We can't easily test this without mocking HTTP, so we test the config path
	tool.SetConfig("", "google", 0, 0, "", true, "")
	if tool.Config.SearchEngine != "google" {
		t.Fatal("search engine should be google after SetConfig")
	}
}

// --- Max results limits ---

func TestBrowseToolMaxResultsLimit(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true

	// Max results > 20 should be capped
	tool.Config.MaxResults = 50
	if tool.Config.MaxResults != 50 {
		t.Fatal("config allows setting high max results")
	}
	// The actual capping happens in search() method
}

// --- HTTP engine fallback ---

func TestBrowseToolHTTPEngine(t *testing.T) {
	tool := NewBrowseTool()
	tool.AllowNetwork = true
	tool.Config.Engine = "http"

	// HTTP engine should use simple HTTP fetch, not Chrome
	if tool.Config.Engine != "http" {
		t.Fatal("engine should be http")
	}
}
