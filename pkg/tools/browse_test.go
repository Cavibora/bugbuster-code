package tools

import (
	"strings"
	"testing"
)

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
	if _, ok := props["action"]; !ok {
		t.Fatal("expected action parameter")
	}
	if _, ok := props["query"]; !ok {
		t.Fatal("expected query parameter")
	}
	if _, ok := props["url"]; !ok {
		t.Fatal("expected url parameter")
	}
}
