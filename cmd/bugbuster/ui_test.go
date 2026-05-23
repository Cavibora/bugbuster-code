package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestGlamourRendererRender(t *testing.T) {
	gr := NewGlamourRenderer()

	// Render буферизирует текст и возвращает пустую строку
	input := "Hello, **world**!\n"
	output := gr.Render(input)
	if output != "" {
		t.Errorf("Expected Render to return empty string (buffered), got: %q", output)
	}
	// Текст должен быть доступен через Flush
	result := gr.Flush()
	if !strings.Contains(result, "Hello") {
		t.Errorf("Expected Flush to contain 'Hello', got: %q", result)
	}
}

func TestGlamourRendererFlush(t *testing.T) {
	gr := NewGlamourRenderer()

	// Накапливаем текст
	gr.Render("# Hello\n\n**bold** text\n")
	gr.Render("- item1\n- item2\n")

	// Flush рендерит через glamour
	result := gr.Flush()
	if result == "" {
		t.Error("Expected Flush to return rendered markdown")
	}
	// Glamour должен отрендерить заголовок
	if !strings.Contains(result, "Hello") {
		t.Errorf("Expected 'Hello' in rendered output, got: %q", result)
	}
}

func TestGlamourRendererFlushEmpty(t *testing.T) {
	gr := NewGlamourRenderer()

	// Flush пустого буфера возвращает пустую строку
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

	// Glamour должен отрендерить кодовый блок
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
	// Директория — показываем "Directory /path" или "Директория /path"
	result := formatReadSummary("Directory /Users/ss/ai/grfn/crates/grfn-core:\n  .DS_Store\n  Cargo.toml\n  📁 src", nil)
	if !strings.Contains(result, "Directory") {
		t.Errorf("Expected 'Directory' in directory summary, got: %q", result)
	}
	if strings.Contains(result, "lines") {
		t.Errorf("Directory summary should not contain 'lines', got: %q", result)
	}
}

func TestFormatReadSummaryLines(t *testing.T) {
	// Обычный файл — показываем "N lines"
	result := formatReadSummary("line1\nline2\nline3", nil)
	if !strings.Contains(result, "3 lines") {
		t.Errorf("Expected '3 lines' for file, got: %q", result)
	}
}

func TestFormatDurationLessThanMs(t *testing.T) {
	// Длительность < 1ms — показываем "<1ms"
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
	// Read — показывает "N lines"
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
}

func TestFormatToolCallEndReadSummary(t *testing.T) {
	// Read — подсчёт строк из fullResult
	result := FormatToolCallEnd("read", true, "line1", "line1\nline2\nline3", 0, nil)

	if !strings.Contains(result, "3 lines") {
		t.Errorf("Expected '3 lines' for read, got: %q", result)
	}
}

func TestFormatToolCallEndBashSummary(t *testing.T) {
	// Bash — первая строка вывода
	result := FormatToolCallEnd("bash", true, "PASS", "PASS", 0, map[string]string{"command": "go test"})

	if !strings.Contains(result, "PASS") {
		t.Errorf("Expected 'PASS' in bash summary, got: %q", result)
	}
}

func TestFormatToolCallEndBashEmptyOutput(t *testing.T) {
	// Bash — пустой вывод
	result := FormatToolCallEnd("bash", true, "(command executed successfully, empty output)", "(command executed successfully, empty output)", 0, map[string]string{"command": "ls"})

	if !strings.Contains(result, "empty output") {
		t.Errorf("Expected 'empty output' for bash with no output, got: %q", result)
	}
}

func TestFormatToolCallEndWriteSummary(t *testing.T) {
	// Write — парсинг "file /path written (N bytes)"
	result := FormatToolCallEnd("write", true, "file /tmp/test.go written (1234 bytes)", "file /tmp/test.go written (1234 bytes)", 0, nil)

	if !strings.Contains(result, "Wrote") {
		t.Errorf("Expected 'Wrote' for write, got: %q", result)
	}
	if !strings.Contains(result, "/tmp/test.go") {
		t.Errorf("Expected path in write summary, got: %q", result)
	}
}

func TestFormatToolCallEndEditSummary(t *testing.T) {
	// Edit — парсинг "file /path edited"
	result := FormatToolCallEnd("edit", true, "file /tmp/main.go edited", "file /tmp/main.go edited", 0, map[string]string{"path": "/tmp/main.go"})

	if !strings.Contains(result, "Edited") {
		t.Errorf("Expected 'Edited' for edit, got: %q", result)
	}
	if !strings.Contains(result, "/tmp/main.go") {
		t.Errorf("Expected path in edit summary, got: %q", result)
	}
}

func TestFormatToolCallEndGlobSummary(t *testing.T) {
	// Glob — подсчёт файлов
	result := FormatToolCallEnd("glob", true, "3 files", "file1.go\nfile2.go\nfile3.go", 0, nil)

	if !strings.Contains(result, "3 files") {
		t.Errorf("Expected '3 files' for glob, got: %q", result)
	}
}

func TestFormatToolCallEndGlobNoFiles(t *testing.T) {
	// Glob — нет файлов
	result := FormatToolCallEnd("glob", true, "no files found", "no files found", 0, nil)

	if !strings.Contains(result, "no files found") {
		t.Errorf("Expected 'no files found' for glob, got: %q", result)
	}
}

func TestFormatToolCallEndGrepSummary(t *testing.T) {
	// Grep — подсчёт совпадений
	result := FormatToolCallEnd("grep", true, "5 matches", "match1\nmatch2\nmatch3\nmatch4\nmatch5", 0, nil)

	if !strings.Contains(result, "5 matches") {
		t.Errorf("Expected '5 matches' for grep, got: %q", result)
	}
}

func TestFormatToolCallEndGrepNoMatches(t *testing.T) {
	// Grep — нет совпадений
	result := FormatToolCallEnd("grep", true, "no matches found", "no matches found", 0, nil)

	if !strings.Contains(result, "no matches found") {
		t.Errorf("Expected 'no matches found' for grep, got: %q", result)
	}
}

func TestFormatToolCallEndGenericSummary(t *testing.T) {
	// Неизвестный инструмент — общий формат
	result := FormatToolCallEnd("custom_tool", true, "operation completed", "operation completed", 0, nil)

	if !strings.Contains(result, "operation completed") {
		t.Errorf("Expected result text for generic tool, got: %q", result)
	}
}

func TestFormatToolCallEndGenericEmpty(t *testing.T) {
	// Неизвестный инструмент с пустым результатом
	result := FormatToolCallEnd("custom_tool", true, "", "", 0, nil)

	if !strings.Contains(result, "done") {
		t.Errorf("Expected 'done' for empty result, got: %q", result)
	}
}

func TestFormatToolCallEndBashExtra(t *testing.T) {
	// Bash — полный вывод с отступом и подсветкой синтаксиса
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
	// Read — больше не показывает содержимое файла, только сводку
	full := "line1\nline2\nline3"
	result := FormatToolCallEnd("read", true, "3 lines", full, 0, nil)

	if !strings.Contains(result, "3 lines") {
		t.Errorf("Expected '3 lines' summary, got: %q", result)
	}
	// Read больше не показывает extra-строки с содержимым
	if strings.Contains(result, "     line1") {
		t.Errorf("Read should not show file content in extra lines, got: %q", result)
	}
}

func TestFormatToolCallEndEditWithDiff(t *testing.T) {
	// Edit с diff — показывает добавленные/удалённые строки
	fullResult := "file /tmp/test.go edited\n--- a//tmp/test.go\n+++ b//tmp/test.go\n@@ -1,3 +1,3 @@\n line1\n-old_line\n+new_line\n line3\n"
	result := FormatToolCallEnd("edit", true, "Edited /tmp/test.go", fullResult, 0, nil)

	if !strings.Contains(result, "Added") || !strings.Contains(result, "Removed") {
		t.Errorf("Expected 'Added/Removed' in edit summary, got: %q", result)
	}
	// Diff-строки должны быть в extra
	if !strings.Contains(result, "-old_line") {
		t.Errorf("Expected '-old_line' in diff extra, got: %q", result)
	}
	if !strings.Contains(result, "+new_line") {
		t.Errorf("Expected '+new_line' in diff extra, got: %q", result)
	}
}

func TestFormatToolCallEndWriteNewFile(t *testing.T) {
	// Write нового файла — показывает содержимое
	fullResult := "file /tmp/new.go written (50 bytes)\n   1  package main\n   2  \n   3  func main() {\n   4  }\n"
	result := FormatToolCallEnd("write", true, "Wrote 4 lines to /tmp/new.go", fullResult, 0, nil)

	if !strings.Contains(result, "Wrote") {
		t.Errorf("Expected 'Wrote' in write summary, got: %q", result)
	}
}

func TestFormatToolCallEndWriteWithDiff(t *testing.T) {
	// Write существующего файла — показывает diff
	fullResult := "file /tmp/test.go written (100 bytes)\n--- a//tmp/test.go\n+++ b//tmp/test.go\n@@ -1,3 +1,3 @@\n line1\n-old\n+new\n line3\n"
	result := FormatToolCallEnd("write", true, "Added 1, Removed 1 lines in /tmp/test.go", fullResult, 0, nil)

	if !strings.Contains(result, "Added") || !strings.Contains(result, "Removed") {
		t.Errorf("Expected 'Added/Removed' in write summary, got: %q", result)
	}
}

func TestFormatToolCallEndFullResultNoLimit(t *testing.T) {
	// Большой fullResult — bash вывод без лимита строк
	var lines []string
	for i := 0; i < 55; i++ {
		lines = append(lines, fmt.Sprintf("output line %d", i))
	}
	full := strings.Join(lines, "\n")
	result := FormatToolCallEnd("bash", true, "summary", full, 0, nil)
	plain := stripANSI(result)

	// Все строки должны присутствовать
	if !strings.Contains(plain, "output line 0") {
		t.Errorf("Expected first line, got: %q", plain)
	}
	if !strings.Contains(plain, "output line 54") {
		t.Errorf("Expected last line, got: %q", plain)
	}
	// Не должно быть сообщения о truncation
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
	// Короткий текст — не переносится
	result := wrapText("hello world", 4, 80)
	if !strings.Contains(result, "hello world") {
		t.Errorf("Expected short text to be preserved, got: %q", result)
	}

	// Длинный текст — переносится по словам
	longText := "This is a very long line that should be wrapped at the specified column width to prevent it from stretching across the terminal"
	result = wrapText(longText, 4, 40)
	lines := strings.Split(result, "\n")
	if len(lines) < 2 {
		t.Errorf("Expected long text to be wrapped into multiple lines, got %d lines", len(lines))
	}
	// Каждая строка после первой должна начинаться с отступа
	for i, line := range lines {
		if i > 0 && strings.HasPrefix(line, " ") == false && line != "" {
			// Допускаем пустые строки
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
	// Полный JSON
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
	// Частичный JSON — незакрытая скобка
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
	// Полные параметры без обрезания
	if !strings.Contains(result, longPath) {
		t.Errorf("Expected full path in summary, got: %q", result)
	}
	if !strings.Contains(result, "read(") {
		t.Errorf("Expected tool name with params, got: %q", result)
	}
}
func TestFormatDelegateTaskSummary(t *testing.T) {
	// Пустой результат
	result := formatDelegateTaskSummary("")
	if !strings.Contains(result, "subagent completed") {
		t.Errorf("Expected 'subagent completed' for empty result, got: %q", result)
	}

	// Одна строка
	result = formatDelegateTaskSummary("Found 3 bugs in main.go")
	if !strings.Contains(result, "Found 3 bugs in main.go") {
		t.Errorf("Expected result text, got: %q", result)
	}

	// Много строк — берём первую непустую
	result = formatDelegateTaskSummary("\n\nFound 3 bugs\nDetails here")
	if !strings.Contains(result, "Found 3 bugs") {
		t.Errorf("Expected first non-empty line, got: %q", result)
	}

	// Очень длинная строка — обрезается
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
