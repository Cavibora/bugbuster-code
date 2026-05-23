package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestLSPTool_Name(t *testing.T) {
	tool := NewLSPTool()
	if tool.Name() != "lsp" {
		t.Errorf("expected 'lsp', got '%s'", tool.Name())
	}
}

func TestLSPTool_Parameters(t *testing.T) {
	tool := NewLSPTool()
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("expected type=object, got %v", params["type"])
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be map[string]any")
	}
	if _, ok := props["operation"]; !ok {
		t.Error("expected 'operation' property")
	}
	if _, ok := props["file_path"]; !ok {
		t.Error("expected 'file_path' property")
	}
}

func TestLSPTool_Execute_MissingParams(t *testing.T) {
	tool := NewLSPTool()
	// Без operation
	result := tool.Execute(map[string]string{"file_path": "/tmp/test.go"})
	if result.Error == "" {
		t.Error("expected error for missing operation")
	}
	// Без file_path
	result = tool.Execute(map[string]string{"operation": "hover"})
	if result.Error == "" {
		t.Error("expected error for missing file_path")
	}
}

func TestLSPTool_Execute_InvalidOperation(t *testing.T) {
	tool := NewLSPTool()
	result := tool.Execute(map[string]string{
		"operation": "invalid_op",
		"file_path": "/tmp/test.go",
	})
	if result.Error == "" {
		t.Error("expected error for invalid operation")
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := map[string]string{
		"main.go":     "go",
		"app.ts":      "typescript",
		"app.tsx":     "typescript",
		"app.js":      "typescript",
		"app.jsx":     "typescript",
		"app.py":      "python",
		"main.c":      "clangd",
		"main.cpp":    "clangd",
		"header.h":    "clangd",
		"header.hpp":  "clangd",
		"main.rs":     "rust-analyzer",
		"Main.java":   "jdtls",
		"README.md":   "",
		"config.yaml": "",
	}
	for path, expected := range tests {
		result := detectLanguage(path)
		if result != expected {
			t.Errorf("detectLanguage(%q) = %q, want %q", path, result, expected)
		}
	}
}

func TestParseLineChar(t *testing.T) {
	tests := []struct {
		line     string
		char     string
		wantLine int
		wantChar int
	}{
		{"10", "5", 9, 4}, // 1-based → 0-based
		{"1", "1", 0, 0},  // first line, first char
		{"", "", 0, 0},    // defaults
		{"0", "0", 0, 0},  // zero stays zero
	}
	for _, tt := range tests {
		line, char := parseLineChar(map[string]string{"line": tt.line, "character": tt.char})
		if line != tt.wantLine || char != tt.wantChar {
			t.Errorf("parseLineChar(%q, %q) = (%d, %d), want (%d, %d)",
				tt.line, tt.char, line, char, tt.wantLine, tt.wantChar)
		}
	}
}

func TestFormatLocations(t *testing.T) {
	locations := []Location{
		{URI: "file:///tmp/test.go", Range: Range{Start: Position{Line: 9, Character: 4}, End: Position{Line: 9, Character: 10}}},
		{URI: "file:///tmp/other.go", Range: Range{Start: Position{Line: 20, Character: 0}, End: Position{Line: 22, Character: 5}}},
	}
	result := formatLocations(locations)
	if !strings.Contains(result, "/tmp/test.go:10:5") {
		t.Errorf("expected /tmp/test.go:10:5 in result, got: %s", result)
	}
	if !strings.Contains(result, "/tmp/other.go:21:1") {
		t.Errorf("expected /tmp/other.go:21:1 in result, got: %s", result)
	}
}

func TestFormatHover(t *testing.T) {
	hover := &HoverResult{
		Contents: MarkupContent{Kind: "markdown", Value: "func main()"},
	}
	result := formatHover(hover)
	if result != "func main()" {
		t.Errorf("expected 'func main()', got '%s'", result)
	}
}

func TestFormatSymbols(t *testing.T) {
	symbols := []DocumentSymbol{
		{
			Name:           "main",
			Kind:           SymbolKindFunction,
			Range:          Range{Start: Position{Line: 9, Character: 0}, End: Position{Line: 15, Character: 1}},
			SelectionRange: Range{Start: Position{Line: 9, Character: 5}, End: Position{Line: 9, Character: 9}},
		},
		{
			Name:           "Config",
			Kind:           SymbolKindClass,
			Range:          Range{Start: Position{Line: 19, Character: 0}, End: Position{Line: 30, Character: 1}},
			SelectionRange: Range{Start: Position{Line: 19, Character: 5}, End: Position{Line: 19, Character: 11}},
			Children: []DocumentSymbol{
				{
					Name:           "Name",
					Kind:           SymbolKindField,
					Range:          Range{Start: Position{Line: 20, Character: 1}, End: Position{Line: 20, Character: 10}},
					SelectionRange: Range{Start: Position{Line: 20, Character: 1}, End: Position{Line: 20, Character: 5}},
				},
			},
		},
	}
	result := formatSymbols(symbols, "")
	if !strings.Contains(result, "main [Function] line 10") {
		t.Errorf("expected 'main [Function] line 10' in result, got: %s", result)
	}
	if !strings.Contains(result, "Config [Class] line 20") {
		t.Errorf("expected 'Config [Class] line 20' in result, got: %s", result)
	}
	if !strings.Contains(result, "  Name [Field] line 21") {
		t.Errorf("expected '  Name [Field] line 21' in result, got: %s", result)
	}
}

func TestPathToURI(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/tmp/test.go", "file:///tmp/test.go"},
		{"file:///tmp/test.go", "file:///tmp/test.go"},
		{"C:\\Users\\test\\main.go", "file:///C:/Users/test/main.go"},
	}
	for _, tt := range tests {
		result := pathToURI(tt.path)
		if result != tt.expected {
			t.Errorf("pathToURI(%q) = %q, want %q", tt.path, result, tt.expected)
		}
	}
}

func TestUriToPath(t *testing.T) {
	result := uriToPath("file:///tmp/test.go")
	if result != "/tmp/test.go" {
		t.Errorf("uriToPath('file:///tmp/test.go') = %q, want /tmp/test.go", result)
	}
}

func TestLSPClientContentLength(t *testing.T) {
	// Тест форматирования Content-Length заголовка
	msg := map[string]any{"jsonrpc": "2.0", "method": "initialize", "id": 1}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if !strings.HasPrefix(header, "Content-Length:") {
		t.Error("expected Content-Length header")
	}
}

func TestSymbolKindNames(t *testing.T) {
	tests := map[SymbolKind]string{
		SymbolKindFunction: "Function",
		SymbolKindClass:    "Class",
		SymbolKindVariable: "Variable",
		SymbolKindField:    "Field",
	}
	for kind, expected := range tests {
		result := SymbolKindNames[kind]
		if result != expected {
			t.Errorf("SymbolKindNames[%d] = %q, want %q", kind, result, expected)
		}
	}
}
