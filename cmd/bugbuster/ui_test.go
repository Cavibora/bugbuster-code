package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestGlamourRendererRender(t *testing.T) {
	gr := NewGlamourRenderer()

	// Render buffers text and returns an empty string
	input := "Hello, **world**!\n"
	output := gr.Render(input)
	if output != "" {
		t.Errorf("Expected Render to return empty string (buffered), got: %q", output)
	}
	// Text should be available via Flush
	result := gr.Flush()
	if !strings.Contains(result, "Hello") {
		t.Errorf("Expected Flush to contain 'Hello', got: %q", result)
	}
}

func TestGlamourRendererFlush(t *testing.T) {
	gr := NewGlamourRenderer()

	// Accumulate text
	gr.Render("# Hello\n\n**bold** text\n")
	gr.Render("- item1\n- item2\n")

	// Flush renders via glamour
	result := gr.Flush()
	if result == "" {
		t.Error("Expected Flush to return rendered markdown")
	}
	// Glamour should render a heading
	if !strings.Contains(result, "Hello") {
		t.Errorf("Expected 'Hello' in rendered output, got: %q", result)
	}
}

func TestGlamourRendererFlushEmpty(t *testing.T) {
	gr := NewGlamourRenderer()

	// Flushing an empty buffer returns an empty string
	result := gr.Flush()
	if result != "" {
		t.Errorf("Expected empty Flush to return '', got: %q", result)
	}
}

func TestGlamourRendererCodeBlock(t *testing.T) {
	gr := NewGlamourRenderer()

	input := "```go\nfmt.Println(\"hello\")\n```\n"
	gr.Render(input)
	result := gr.Flush()

	// Glamour should render a code block
	if !strings.Contains(result, "hello") {
		t.Errorf("Expected 'hello' in rendered code block, got: %q", result)
	}
}

func TestRenderMarkdownGlamour(t *testing.T) {
	input := "# Title\n\n**bold** text\n\n- item1\n- item2\n"
	result, err := RenderMarkdownGlamour(input)
	if err != nil {
		t.Fatalf("RenderMarkdownGlamour returned error: %v", err)
	}
	if !strings.Contains(result, "Title") {
		t.Errorf("Expected 'Title' in rendered output, got: %q", result)
	}
	if !strings.Contains(result, "bold") {
		t.Errorf("Expected 'bold' in rendered output, got: %q", result)
	}
}

func TestFormatContextBar(t *testing.T) {
	tests := []struct {
		used    int
		max     int
		hasBar  bool
		hasPct  bool
	}{
		{100, 1000, true, true},
		{0, 1000, true, true},
		{800, 1000, true, true},
		{500, 0, false, false},
	}

	for _, tt := range tests {
		result := FormatContextBar(tt.used, tt.max)
		if tt.max > 0 && !strings.Contains(result, "/") {
			t.Errorf("FormatContextBar(%d, %d) = %q, expected /", tt.used, tt.max, result)
		}
		if tt.max == 0 && strings.Contains(result, "/") {
			t.Errorf("FormatContextBar(%d, 0) = %q, expected no /", tt.used, result)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	result := FormatTokens(100, 200)
	if !strings.Contains(result, "100") {
		t.Errorf("FormatTokens should contain input tokens, got: %q", result)
	}
	if !strings.Contains(result, "200") {
		t.Errorf("FormatTokens should contain output tokens, got: %q", result)
	}
	if !strings.Contains(result, "300") {
		t.Errorf("FormatTokens should contain total tokens, got: %q", result)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "<1ms"},
		{500 * time.Microsecond, "<1ms"},
		{100 * time.Millisecond, "100ms"},
		{1500 * time.Millisecond, "1.5s"},
		{90 * time.Second, "1.5m"},
	}

	for _, tt := range tests {
		result := FormatDuration(tt.d)
		if !strings.Contains(result, tt.want) {
			t.Errorf("FormatDuration(%v) = %q, expected to contain %q", tt.d, result, tt.want)
		}
	}
}

func TestFormatIteration(t *testing.T) {
	result := FormatIteration(2)
	if !strings.Contains(result, "2") {
		t.Errorf("FormatIteration(2) = %q, expected to contain 2", result)
	}
	if strings.Contains(result, "/") {
		t.Errorf("FormatIteration should not contain '/', got: %q", result)
	}
}

func TestFormatToolCallStart(t *testing.T) {
	params := map[string]string{"path": "main.go"}
	result := FormatToolCallStart("read", params)

	if !strings.Contains(result, "read") {
		t.Errorf("Expected tool name 'read', got: %q", result)
	}
	if !strings.Contains(result, "main.go") {
		t.Errorf("Expected param value 'main.go', got: %q", result)
	}
	if !strings.Contains(result, "⏺") {
		t.Errorf("Expected ⏺ marker, got: %q", result)
	}
}

func TestFormatToolCallStartNoParams(t *testing.T) {
	result := FormatToolCallStart("bash", nil)

	if !strings.Contains(result, "bash") {
		t.Errorf("Expected tool name 'bash', got: %q", result)
	}
	if !strings.Contains(result, "⏺") {
		t.Errorf("Expected ⏺ marker, got: %q", result)
	}
}

func TestFormatToolCallStartWithLines(t *testing.T) {
	params := map[string]string{"path": "main.go", "lines": "1-40"}
	result := FormatToolCallStart("read", params)

	if !strings.Contains(result, "main.go") {
		t.Errorf("Expected path 'main.go', got: %q", result)
	}
	if !strings.Contains(result, "1-40") {
		t.Errorf("Expected lines '1-40', got: %q", result)
	}
	if !strings.Contains(result, "·") {
		t.Errorf("Expected separator '·', got: %q", result)
	}
}


func TestFormatReadSummaryDirectory(t *testing.T) {
	// Directory — show "Directory /path" or "Директория /path"
	result := formatReadSummary("Directory /Users/ss/ai/grfn/crates/grfn-core:\n  .DS_Store\n  Cargo.toml\n  📁 src", nil)
	if !strings.Contains(result, "Directory") {
		t.Errorf("Expected 'Directory' in directory summary, got: %q", result)
	}
	if strings.Contains(result, "lines") {
		t.Errorf("Directory summary should not contain 'lines', got: %q", result)
	}
}

func TestFormatReadSummaryLines(t *testing.T) {
	// Regular file — show "N lines"
	result := formatReadSummary("line1\nline2\nline3", nil)
	if !strings.Contains(result, "3 lines") {
		t.Errorf("Expected '3 lines' for file, got: %q", result)
	}
}

func TestFormatDurationLessThanMs(t *testing.T) {
	// Duration < 1ms — show "<1ms"
	result := FormatDuration(0)
	if result != "<1ms" {
		t.Errorf("Expected '<1ms' for 0 duration, got: %q", result)
	}
	result = FormatDuration(500 * time.Microsecond)
	if result != "<1ms" {
		t.Errorf("Expected '<1ms' for 500us, got: %q", result)
	}
}

func TestFormatToolCallEnd(t *testing.T) {
	// Read — shows "N lines"
	result := FormatToolCallEnd("read", true, "5 lines", "line1\nline2\nline3\nline4\nline5", 100*time.Millisecond, map[string]string{"path": "/tmp/test.go"})

	if !strings.Contains(result, "✓") {
		t.Errorf("Expected ✓ marker for success, got: %q", result)
	}
	if !strings.Contains(result, "5 lines") {
		t.Errorf("Expected '5 lines' for read, got: %q", result)
	}
	if !strings.Contains(result, "100ms") {
		t.Errorf("Expected duration, got: %q", result)
	}
}

func TestFormatToolCallEndError(t *testing.T) {
	result := FormatToolCallEnd("bash", false, "command failed: exit status 1", "command failed: exit status 1", 0, nil)

	if !strings.Contains(result, "✗") {
		t.Errorf("Expected ✗ marker for error, got: %q", result)
	}
	if !strings.Contains(result, appTheme.Error.ANSICode()) {
		t.Errorf("Expected red color for error, got: %q", result)
	}
	if !strings.Contains(result, "command failed") {
		t.Errorf("Expected error message, got: %q", result)
	}
	// Error results should NOT show extra output lines
	lines := strings.Split(result, "\n")
	if len(lines) > 1 {
		t.Errorf("Error result should not have extra lines, got %d lines: %s", len(lines), result)
	}
}

func TestFormatToolCallEndReadSummary(t *testing.T) {
	// Read — line count from fullResult
	result := FormatToolCallEnd("read", true, "line1", "line1\nline2\nline3", 0, nil)

	if !strings.Contains(result, "3 lines") {
		t.Errorf("Expected '3 lines' for read, got: %q", result)
	}
}

func TestFormatToolCallEndBashSummary(t *testing.T) {
	// Bash — first line of output
	result := FormatToolCallEnd("bash", true, "PASS", "PASS", 0, map[string]string{"command": "go test"})

	if !strings.Contains(result, "PASS") {
		t.Errorf("Expected 'PASS' in bash summary, got: %q", result)
	}
}

func TestFormatToolCallEndBashEmptyOutput(t *testing.T) {
	// Bash — empty output
	result := FormatToolCallEnd("bash", true, "(command executed successfully, empty output)", "(command executed successfully, empty output)", 0, map[string]string{"command": "ls"})

	if !strings.Contains(result, "empty output") {
		t.Errorf("Expected 'empty output' for bash with no output, got: %q", result)
	}
}

func TestFormatToolCallEndWriteSummary(t *testing.T) {
	// Write — parsing "file /path written (N bytes)"
	result := FormatToolCallEnd("write", true, "file /tmp/test.go written (1234 bytes)", "file /tmp/test.go written (1234 bytes)", 0, nil)

	if !strings.Contains(result, "Wrote") {
		t.Errorf("Expected 'Wrote' for write, got: %q", result)
	}
	if !strings.Contains(result, "/tmp/test.go") {
		t.Errorf("Expected path in write summary, got: %q", result)
	}
}

func TestFormatToolCallEndEditSummary(t *testing.T) {
	// Edit — parsing "file /path edited"
	result := FormatToolCallEnd("edit", true, "file /tmp/main.go edited", "file /tmp/main.go edited", 0, map[string]string{"path": "/tmp/main.go"})

	if !strings.Contains(result, "Edited") {
		t.Errorf("Expected 'Edited' for edit, got: %q", result)
	}
	if !strings.Contains(result, "/tmp/main.go") {
		t.Errorf("Expected path in edit summary, got: %q", result)
	}
}

func TestFormatToolCallEndGlobSummary(t *testing.T) {
	// Glob — file count
	result := FormatToolCallEnd("glob", true, "3 files", "file1.go\nfile2.go\nfile3.go", 0, nil)

	if !strings.Contains(result, "3 files") {
		t.Errorf("Expected '3 files' for glob, got: %q", result)
	}
}

func TestFormatToolCallEndGlobNoFiles(t *testing.T) {
	// Glob — no files
	result := FormatToolCallEnd("glob", true, "no files found", "no files found", 0, nil)

	if !strings.Contains(result, "no files found") {
		t.Errorf("Expected 'no files found' for glob, got: %q", result)
	}
}

func TestFormatToolCallEndGrepSummary(t *testing.T) {
	// Grep — match count
	result := FormatToolCallEnd("grep", true, "5 matches", "match1\nmatch2\nmatch3\nmatch4\nmatch5", 0, nil)

	if !strings.Contains(result, "5 matches") {
		t.Errorf("Expected '5 matches' for grep, got: %q", result)
	}
}

func TestFormatToolCallEndGrepNoMatches(t *testing.T) {
	// Grep — no matches
	result := FormatToolCallEnd("grep", true, "no matches found", "no matches found", 0, nil)

	if !strings.Contains(result, "no matches found") {
		t.Errorf("Expected 'no matches found' for grep, got: %q", result)
	}
}

func TestFormatToolCallEndGenericSummary(t *testing.T) {
	// Unknown tool — general format
	result := FormatToolCallEnd("custom_tool", true, "operation completed", "operation completed", 0, nil)

	if !strings.Contains(result, "operation completed") {
		t.Errorf("Expected result text for generic tool, got: %q", result)
	}
}

func TestFormatToolCallEndGenericEmpty(t *testing.T) {
	// Unknown tool with empty result
	result := FormatToolCallEnd("custom_tool", true, "", "", 0, nil)

	if !strings.Contains(result, "done") {
		t.Errorf("Expected 'done' for empty result, got: %q", result)
	}
}

func TestFormatToolCallEndBashExtra(t *testing.T) {
	// Bash — full output with indentation and syntax highlighting
	full := "line1\nline2\nline3"
	result := FormatToolCallEnd("bash", true, "line1", full, 0, nil)
	plain := stripANSI(result)

	if !strings.Contains(plain, "⎿") {
		t.Errorf("Expected ⎿ marker, got: %q", plain)
	}
	if !strings.Contains(plain, "line1") {
		t.Errorf("Expected line1 in output, got: %q", plain)
	}
	if !strings.Contains(plain, "line2") {
		t.Errorf("Expected line2 in output, got: %q", plain)
	}
}

func TestFormatToolCallEndReadExtra(t *testing.T) {
	// Read — no longer shows file content, only a summary
	full := "line1\nline2\nline3"
	result := FormatToolCallEnd("read", true, "3 lines", full, 0, nil)

	if !strings.Contains(result, "3 lines") {
		t.Errorf("Expected '3 lines' summary, got: %q", result)
	}
	// Read no longer shows extra lines with content
	if strings.Contains(result, "     line1") {
		t.Errorf("Read should not show file content in extra lines, got: %q", result)
	}
}

func TestFormatToolCallEndEditWithDiff(t *testing.T) {
	// Edit with diff — shows added/removed lines
	fullResult := "file /tmp/test.go edited\n--- a//tmp/test.go\n+++ b//tmp/test.go\n@@ -1,3 +1,3 @@\n line1\n-old_line\n+new_line\n line3\n"
	result := FormatToolCallEnd("edit", true, "Edited /tmp/test.go", fullResult, 0, nil)

	if !strings.Contains(result, "Added") || !strings.Contains(result, "Removed") {
		t.Errorf("Expected 'Added/Removed' in edit summary, got: %q", result)
	}
	// Diff lines should be in extra
	if !strings.Contains(result, "-old_line") {
		t.Errorf("Expected '-old_line' in diff extra, got: %q", result)
	}
	if !strings.Contains(result, "+new_line") {
		t.Errorf("Expected '+new_line' in diff extra, got: %q", result)
	}
}

func TestFormatToolCallEndWriteNewFile(t *testing.T) {
	// Write of a new file — shows content
	fullResult := "file /tmp/new.go written (50 bytes)\n   1  package main\n   2  \n   3  func main() {\n   4  }\n"
	result := FormatToolCallEnd("write", true, "Wrote 4 lines to /tmp/new.go", fullResult, 0, nil)

	if !strings.Contains(result, "Wrote") {
		t.Errorf("Expected 'Wrote' in write summary, got: %q", result)
	}
}

func TestFormatToolCallEndWriteWithDiff(t *testing.T) {
	// Write of an existing file — shows diff
	fullResult := "file /tmp/test.go written (100 bytes)\n--- a//tmp/test.go\n+++ b//tmp/test.go\n@@ -1,3 +1,3 @@\n line1\n-old\n+new\n line3\n"
	result := FormatToolCallEnd("write", true, "Added 1, Removed 1 lines in /tmp/test.go", fullResult, 0, nil)

	if !strings.Contains(result, "Added") || !strings.Contains(result, "Removed") {
		t.Errorf("Expected 'Added/Removed' in write summary, got: %q", result)
	}
}

func TestFormatToolCallEndFullResultNoLimit(t *testing.T) {
	// Large fullResult — bash output without line limit
	var lines []string
	for i := 0; i < 55; i++ {
		lines = append(lines, fmt.Sprintf("output line %d", i))
	}
	full := strings.Join(lines, "\n")
	result := FormatToolCallEnd("bash", true, "summary", full, 0, nil)
	plain := stripANSI(result)

	// All lines should be present
	if !strings.Contains(plain, "output line 0") {
		t.Errorf("Expected first line, got: %q", plain)
	}
	if !strings.Contains(plain, "output line 54") {
		t.Errorf("Expected last line, got: %q", plain)
	}
	// There should be no truncation message
	if strings.Contains(plain, "more lines") {
		t.Errorf("Expected no truncation message, got: %q", plain)
	}
}

func TestStripLineNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"   1  package main", "package main"},
		{"  42  func foo() {", "func foo() {"},
		{"   1  hello", "hello"},
		{"hello", "hello"},
		{"", ""},
	}
	for _, tt := range tests {
		result := stripLineNumber(tt.input)
		if result != tt.expected {
			t.Errorf("stripLineNumber(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestLanguageFromPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/tmp/main.go", "go"},
		{"/tmp/app.py", "python"},
		{"/tmp/index.js", "javascript"},
		{"/tmp/style.css", "css"},
		{"/tmp/config.yaml", "yaml"},
		{"/tmp/Dockerfile", "docker"},
		{"/tmp/unknown.txt", ""},
		{"/tmp/Makefile", "make"},
	}
	for _, tt := range tests {
		result := languageFromPath(tt.path)
		if result != tt.expected {
			t.Errorf("languageFromPath(%q) = %q, want %q", tt.path, result, tt.expected)
		}
	}
}

func TestFormatSeparator(t *testing.T) {
	result := FormatSeparator()
	if !strings.Contains(result, "──") {
		t.Errorf("Expected separator line, got: %q", result)
	}
	if !strings.Contains(result, appTheme.Separator.ANSICode()) {
		t.Errorf("Expected separator color in separator, got: %q", result)
	}
}

func TestFormatStatusLine(t *testing.T) {
	result := FormatStatusLine(100, 200, 5*time.Second, 1000, 8000, "zai", "glm-5.1")
	if !strings.Contains(result, "⏱") {
		t.Errorf("Expected ⏱ timer in status line, got: %q", result)
	}
	if !strings.Contains(result, "⬆") {
		t.Errorf("Expected input tokens arrow in status line, got: %q", result)
	}
	if !strings.Contains(result, "zai") {
		t.Errorf("Expected provider name in status line, got: %q", result)
	}
	if !strings.Contains(result, "glm-5.1") {
		t.Errorf("Expected model name in status line, got: %q", result)
	}
}

func TestFormatStatusLineEmpty(t *testing.T) {
	result := FormatStatusLine(0, 0, 0, 0, 0, "", "")
	if result != "" {
		t.Errorf("Expected empty status line for zero values, got: %q", result)
	}
}

func TestWrapText(t *testing.T) {
	// Short text — does not wrap
	result := wrapText("hello world", 4, 80)
	if !strings.Contains(result, "hello world") {
		t.Errorf("Expected short text to be preserved, got: %q", result)
	}

	// Long text — wraps by words
	longText := "This is a very long line that should be wrapped at the specified column width to prevent it from stretching across the terminal"
	result = wrapText(longText, 4, 40)
	lines := strings.Split(result, "\n")
	if len(lines) < 2 {
		t.Errorf("Expected long text to be wrapped into multiple lines, got %d lines", len(lines))
	}
	// Each line after the first should start with an indent
	for i, line := range lines {
		if i > 0 && strings.HasPrefix(line, " ") == false && line != "" {
			// Allow empty lines
		}
	}
}

func TestWrapTextMultiline(t *testing.T) {
	text := "Line one\nLine two\nLine three"
	result := wrapText(text, 4, 80)
	if !strings.Contains(result, "Line one") {
		t.Errorf("Expected 'Line one' in wrapped text, got: %q", result)
	}
	if !strings.Contains(result, "Line two") {
		t.Errorf("Expected 'Line two' in wrapped text, got: %q", result)
	}
}

func TestWrapTextEmpty(t *testing.T) {
	result := wrapText("", 4, 80)
	if result != "" {
		t.Errorf("Expected empty string for empty input, got: %q", result)
	}
}

func TestParsePartialToolInput(t *testing.T) {
	// Full JSON
	params := parsePartialToolInput(`{"path": "main.go", "lines": 10}`)
	if params["path"] != "main.go" {
		t.Errorf("Expected path=main.go, got %s", params["path"])
	}
	if params["lines"] != "10" {
		t.Errorf("Expected lines=10, got %s", params["lines"])
	}
}

func TestParsePartialToolInputEmpty(t *testing.T) {
	params := parsePartialToolInput("")
	if len(params) != 0 {
		t.Errorf("Expected empty map for empty input, got %d params", len(params))
	}
}

func TestParsePartialToolInputPartial(t *testing.T) {
	// Partial JSON — unclosed bracket
	params := parsePartialToolInput(`{"path": "main.go"`)
	if params["path"] != "main.go" {
		t.Errorf("Expected path=main.go from partial JSON, got %s", params["path"])
	}
}

func TestFormatToolSummary(t *testing.T) {
	params := map[string]string{"command": "ls -la /tmp"}
	result := formatToolSummary("bash", params)
	if !strings.Contains(result, "bash") {
		t.Errorf("Expected tool name 'bash', got: %q", result)
	}
	if !strings.Contains(result, "ls -la /tmp") {
		t.Errorf("Expected command value, got: %q", result)
	}
	if !strings.Contains(result, "(") {
		t.Errorf("Expected parentheses with params, got: %q", result)
	}
}

func TestFormatToolSummaryNoParams(t *testing.T) {
	result := formatToolSummary("bash", nil)
	if result != "bash" {
		t.Errorf("Expected just tool name without params, got: %q", result)
	}
}

func TestFormatToolSummaryLongValue(t *testing.T) {
	longPath := strings.Repeat("x", 100)
	params := map[string]string{"path": longPath}
	result := formatToolSummary("read", params)
	// Full parameters without truncation
	if !strings.Contains(result, longPath) {
		t.Errorf("Expected full path in summary, got: %q", result)
	}
	if !strings.Contains(result, "read(") {
		t.Errorf("Expected tool name with params, got: %q", result)
	}
}
func TestFormatDelegateTaskSummary(t *testing.T) {
	// Empty result
	result := formatDelegateTaskSummary("")
	if !strings.Contains(result, "subagent completed") {
		t.Errorf("Expected 'subagent completed' for empty result, got: %q", result)
	}

	// One line
	result = formatDelegateTaskSummary("Found 3 bugs in main.go")
	if !strings.Contains(result, "Found 3 bugs in main.go") {
		t.Errorf("Expected result text, got: %q", result)
	}

	// Many lines — take the first non-empty one
	result = formatDelegateTaskSummary("\n\nFound 3 bugs\nDetails here")
	if !strings.Contains(result, "Found 3 bugs") {
		t.Errorf("Expected first non-empty line, got: %q", result)
	}

	// Very long line — truncated
	longLine := strings.Repeat("x", 200)
	result = formatDelegateTaskSummary(longLine)
	if len(result) > 200 {
		t.Errorf("Expected truncated result, got %d chars", len(result))
	}
}

// --- Terminal width and truncation tests ---


func TestTruncateMessage(t *testing.T) {
	tests := []struct {
		name   string
		msg    string
		width  int
	}{
		{"short message", "hello", 80},
		{"exact fit", "hello", 40},
		{"too long", "this is a very long message that should be truncated because it exceeds the terminal width", 50},
		{"very narrow terminal", "hello world", 40},
		{"unicode message", "Привет мир это длинное сообщение", 50},
		{"empty message", "", 80},
		{"single char", "a", 80},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateMessage(tt.msg, tt.width)
			// Check that result fits in terminal width
			if len(result) > tt.width {
				t.Errorf("result length %d exceeds terminal width %d", len(result), tt.width)
			}
			// Check that result is not empty for non-empty input
			if tt.msg != "" && result == "" {
				t.Errorf("truncateMessage(%q, %d) returned empty string", tt.msg, tt.width)
			}
			// Check that truncation adds "..." for long messages
			if len(tt.msg) > tt.width-35 && tt.msg != "" {
				// Message should be truncated
				if len(result) > len(tt.msg) {
					t.Errorf("result longer than input: %d > %d", len(result), len(tt.msg))
				}
			}
		})
	}
}

func TestTerminalWidth(t *testing.T) {
	width := terminalWidth()
	if width < 20 {
		t.Errorf("terminalWidth() = %d, expected at least 20", width)
	}
}

// --- Spinner frame line-count tests (regression for CLI text being erased) ---

func TestVisibleWidth_MultiByteGlyphs(t *testing.T) {
	// The status line is full of multi-byte glyphs. len() counts BYTES (over-
	// states width); visibleWidth must count COLUMNS.
	s := "· ⚡ ⬆ ⬇ Σ ⏱"
	if visibleWidth(s) >= len(s) {
		t.Errorf("visibleWidth(%d) should be < byte len(%d) for multi-byte glyphs", visibleWidth(s), len(s))
	}
	// CJK counts as 2 columns per rune.
	if got := visibleWidth("你好"); got != 4 {
		t.Errorf("visibleWidth(CJK) = %d, want 4", got)
	}
	// ANSI escapes cost zero columns.
	if got := visibleWidth(ansiBold + "abc" + ansiReset); got != 3 {
		t.Errorf("visibleWidth(colored abc) = %d, want 3", got)
	}
}

func TestSpinnerFrameLines_SingleLineStatus(t *testing.T) {
	// A realistic, glyph-heavy status line on an 80-col terminal must occupy
	// exactly ONE row. The old byte-length formula returned >1 here, which made
	// the next tick erase the agent text above the spinner.
	display := "⬆ 12345 ⬇ 6789 Σ 80234 · ⚡ 123tok/s · local · claude-sonnet"
	if got := spinnerFrameLines(display, 80); got != 1 {
		t.Errorf("spinnerFrameLines = %d, want 1 (must not climb into agent text)", got)
	}
	// Contrast: the buggy byte-based count would have over-estimated.
	if byteLines := len(display)/80 + 1; byteLines < spinnerFrameLines(display, 80) {
		t.Error("sanity: byte count should not exceed column count")
	}
}

func TestSpinnerFrameLines_PrefersOneRowWhenItFits(t *testing.T) {
	// Short status → 1 line.
	if got := spinnerFrameLines("hello", 80); got != 1 {
		t.Errorf("spinnerFrameLines(hello) = %d, want 1", got)
	}
	// A status that genuinely overflows one row must report the true row count
	// (so the clear still covers the whole wrapped block).
	long := "x 12345 ⬆ 6789 Σ 80234 ⚡ 123tok/s local claude-sonnet-4-5-20250901 extra extra extra"
	got := spinnerFrameLines(long, 40)
	if got < 2 {
		t.Errorf("spinnerFrameLines(long, 40) = %d, want >=2", got)
	}
}

func TestTruncateToWidth_FitsAndPreservesANSI(t *testing.T) {
	s := ansiBold + "this is a very long status line with · ⚡ ⬆ ⬇ glyphs" + ansiReset
	out := truncateToWidth(s, 20)
	if visibleWidth(out) > 20 {
		t.Errorf("truncated width %d exceeds max 20", visibleWidth(out))
	}
	if !strings.Contains(out, ansiBold) {
		t.Error("truncateToWidth must preserve ANSI escape codes")
	}
	if !strings.HasSuffix(out, "…") {
		t.Error("truncated string should end with ellipsis")
	}
	// No-op when it already fits.
	if truncateToWidth("short", 80) != "short" {
		t.Error("truncateToWidth must not alter a string that fits")
	}
}

func TestSingleLine_CollapsesNewlines(t *testing.T) {
	in := "line1\nline2\r\nline3\rline4"
	out := singleLine(in)
	if strings.ContainsAny(out, "\n\r") {
		t.Errorf("singleLine must remove all line breaks, got %q", out)
	}
	if !strings.Contains(out, "⏎") {
		t.Errorf("singleLine should mark breaks with ⏎, got %q", out)
	}
	// No-op fast path for clean strings.
	if singleLine("clean") != "clean" {
		t.Error("singleLine must not alter a line-break-free string")
	}
}

func TestFormatToolSummary_WriteContentIsSingleLine(t *testing.T) {
	// Regression: streaming a write() call feeds the spinner a preview built
	// from partial JSON. The "content" param (not in displayKeys) used to be
	// inserted RAW with its newlines, making the frame multi-row while
	// lastLines said 1 — every tick appended a new frame (wall of repeated
	// "⏺ write(content=..." lines).
	multiline := "const fs = require('fs');\nconst vm = require('vm');\n\nfunction hello() {\n  return 1;\n}"
	summary := formatToolSummary("write", map[string]string{
		"path":    "/tmp/app.js",
		"content": multiline,
	})
	if strings.ContainsAny(summary, "\n\r") {
		t.Errorf("formatToolSummary must never return a multi-line string, got %q", summary)
	}
	if visibleWidth(summary) > 120 {
		t.Errorf("summary too wide for the spinner: %d cols", visibleWidth(summary))
	}
	// Path (a displayKeys entry) should still be present.
	if !strings.Contains(summary, "/tmp/app.js") {
		t.Errorf("summary should include the path, got %q", summary)
	}
}

func TestFormatToolSummary_TailParamNoBrokenUTF8(t *testing.T) {
	// The old byte-slice truncation could split multi-byte runes.
	long := strings.Repeat("日本語テキスト", 20)
	summary := formatToolSummary("write", map[string]string{"content": long})
	if !utf8.ValidString(summary) {
		t.Errorf("summary contains broken UTF-8 after truncation: %q", summary)
	}
	if strings.ContainsAny(summary, "\n") {
		t.Error("summary must be single-line")
	}
}
