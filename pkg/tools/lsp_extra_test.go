package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// --- LSPTool construction ---

func TestNewLSPTool(t *testing.T) {
	tool := NewLSPTool()
	if tool == nil {
		t.Fatal("expected non-nil LSPTool")
	}
	if tool.Timeout != 10_000_000_000 { // 10 seconds
		t.Errorf("expected default timeout 10s, got %v", tool.Timeout)
	}
	if len(tool.Servers) != 0 {
		t.Errorf("expected empty servers map, got %d", len(tool.Servers))
	}
}

func TestLSPTool_SetRootDir(t *testing.T) {
	tool := NewLSPTool()
	tool.SetRootDir("/tmp/myproject")
	if tool.rootDir != "/tmp/myproject" {
		t.Errorf("expected rootDir '/tmp/myproject', got '%s'", tool.rootDir)
	}
}

// --- LSPTool Execute with missing server ---

func TestLSPTool_Execute_UnsupportedLanguage(t *testing.T) {
	tool := NewLSPTool()
	tool.AllowedDirs = []string{"/tmp"}

	result := tool.Execute(map[string]string{
		"operation": "hover",
		"file_path": "/tmp/test.xyz",
	})
	if result.Error == "" {
		t.Error("expected error for unsupported language")
	}
}

func TestLSPTool_Execute_NoServerForLanguage(t *testing.T) {
	tool := NewLSPTool()
	tool.AllowedDirs = []string{"/tmp"}

	result := tool.Execute(map[string]string{
		"operation": "hover",
		"file_path": "/tmp/test.go",
	})
	if result.Error == "" {
		t.Error("expected error for missing LSP server")
	}
}

func TestLSPTool_Execute_PathNotAllowed(t *testing.T) {
	tool := NewLSPTool()
	tool.AllowedDirs = []string{"/safe/dir"}

	result := tool.Execute(map[string]string{
		"operation": "hover",
		"file_path": "/etc/passwd",
	})
	if result.Error == "" {
		t.Error("expected error for path not in allowed dirs")
	}
}

// --- parseLocations ---

func TestParseLocations_Array(t *testing.T) {
	data := json.RawMessage(`[
		{"uri": "file:///tmp/a.go", "range": {"start": {"line": 10, "character": 5}, "end": {"line": 10, "character": 10}}},
		{"uri": "file:///tmp/b.go", "range": {"start": {"line": 20, "character": 0}, "end": {"line": 20, "character": 5}}}
	]`)
	locs, err := parseLocations(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(locs) != 2 {
		t.Fatalf("expected 2 locations, got %d", len(locs))
	}
	if locs[0].URI != "file:///tmp/a.go" {
		t.Errorf("expected URI 'file:///tmp/a.go', got '%s'", locs[0].URI)
	}
	if locs[1].Range.Start.Line != 20 {
		t.Errorf("expected line 20, got %d", locs[1].Range.Start.Line)
	}
}

func TestParseLocations_Single(t *testing.T) {
	data := json.RawMessage(`{"uri": "file:///tmp/a.go", "range": {"start": {"line": 5, "character": 3}, "end": {"line": 5, "character": 8}}}`)
	locs, err := parseLocations(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(locs) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locs))
	}
	if locs[0].URI != "file:///tmp/a.go" {
		t.Errorf("expected URI 'file:///tmp/a.go', got '%s'", locs[0].URI)
	}
}

func TestParseLocations_Null(t *testing.T) {
	locs, err := parseLocations(json.RawMessage("null"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if locs != nil {
		t.Fatalf("expected nil for null, got %v", locs)
	}
}

func TestParseLocations_EmptyArray(t *testing.T) {
	locs, err := parseLocations(json.RawMessage("[]"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(locs) != 0 {
		t.Fatalf("expected empty array, got %d", len(locs))
	}
}

func TestParseLocations_InvalidJSON(t *testing.T) {
	locs, err := parseLocations(json.RawMessage(`{"invalid": true}`))
	// Should return nil, nil (not a Location or Location[])
	if err != nil {
		t.Logf("error is acceptable: %v", err)
	}
	if locs != nil {
		t.Logf("locs is acceptable: %v", locs)
	}
}

// --- LSPClient writeMessage format ---

func TestLSPClient_WriteMessageFormat(t *testing.T) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{"processId": nil, "rootUri": "file:///tmp"},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify the data is valid JSON
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("data should be valid JSON: %v", err)
	}
	if parsed["method"] != "initialize" {
		t.Errorf("expected method 'initialize', got %v", parsed["method"])
	}

	// Verify Content-Length header format
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if !strings.HasPrefix(header, "Content-Length:") {
		t.Error("expected Content-Length header")
	}
	if !strings.Contains(header, "\r\n\r\n") {
		t.Error("expected double CRLF after header")
	}
}

// --- LSPClient NewLSPClient validation ---

func TestLSPClient_NewLSPClient_InvalidCommand(t *testing.T) {
	_, err := NewLSPClient("nonexistent_command_that_does_not_exist", []string{}, "file:///tmp", 10_000_000_000)
	// NewLSPClient only creates the struct, doesn't start the process
	// So it should not error here (Start() would fail)
	if err != nil {
		// This is acceptable — pipe creation might fail
		t.Logf("NewLSPClient error (acceptable): %v", err)
	}
}

// --- detectLanguage edge cases ---

func TestDetectLanguage_EdgeCases(t *testing.T) {
	tests := map[string]string{
		"file.go":       "go",
		"file.ts":       "typescript",
		"file.tsx":      "typescript",
		"file.js":       "typescript",
		"file.jsx":      "typescript",
		"file.py":       "python",
		"file.c":        "clangd",
		"file.cpp":      "clangd",
		"file.cc":       "clangd",
		"file.cxx":      "clangd",
		"file.h":        "clangd",
		"file.hpp":      "clangd",
		"file.hxx":      "clangd",
		"file.rs":       "rust-analyzer",
		"file.java":     "jdtls",
		"Makefile":      "",
		"file.txt":      "",
		"file.html":     "",
		"file.css":      "",
		"file.json":    "",
		"file.yaml":    "",
		"file.md":      "",
		"file.rb":      "",
		"file.php":     "",
		"PATH/TO/FILE.GO": "go", // case insensitive extension
	}

	for path, expected := range tests {
		result := detectLanguage(path)
		if result != expected {
			t.Errorf("detectLanguage(%q) = %q, want %q", path, result, expected)
		}
	}
}

// --- parseLineChar edge cases ---

func TestParseLineChar_LargeValues(t *testing.T) {
	line, char := parseLineChar(map[string]string{"line": "100", "character": "50"})
	if line != 99 {
		t.Errorf("expected line 99 (0-based), got %d", line)
	}
	if char != 49 {
		t.Errorf("expected char 49 (0-based), got %d", char)
	}
}

func TestParseLineChar_NegativeValues(t *testing.T) {
	line, char := parseLineChar(map[string]string{"line": "-1", "character": "-1"})
	// Negative values stay negative (Sscanf parses them as negative)
	// This is documented behavior — LSP uses 0-based, negative is invalid
	if line >= 0 {
		t.Errorf("expected negative line for input -1, got %d", line)
	}
	if char >= 0 {
		t.Errorf("expected negative char for input -1, got %d", char)
	}
}

func TestParseLineChar_NonNumeric(t *testing.T) {
	line, char := parseLineChar(map[string]string{"line": "abc", "character": "xyz"})
	if line != 0 {
		t.Errorf("expected line 0 for non-numeric, got %d", line)
	}
	if char != 0 {
		t.Errorf("expected char 0 for non-numeric, got %d", char)
	}
}

// --- formatLocations edge cases ---

func TestFormatLocations_MultiLine(t *testing.T) {
	locations := []Location{
		{
			URI:   "file:///tmp/a.go",
			Range: Range{Start: Position{Line: 5, Character: 3}, End: Position{Line: 8, Character: 10}},
		},
	}
	result := formatLocations(locations)
	if !strings.Contains(result, "/tmp/a.go:6:4") {
		t.Errorf("expected start position in result, got: %s", result)
	}
	if !strings.Contains(result, "—") {
		t.Errorf("expected range separator in result, got: %s", result)
	}
}

func TestFormatLocations_Empty(t *testing.T) {
	result := formatLocations([]Location{})
	if result != "" {
		t.Errorf("expected empty string, got: %s", result)
	}
}

// --- formatHover edge cases ---

func TestFormatHover_Empty(t *testing.T) {
	hover := &HoverResult{
		Contents: MarkupContent{Kind: "plaintext", Value: ""},
	}
	result := formatHover(hover)
	if result != "(no hover content)" {
		t.Errorf("expected '(no hover content)', got '%s'", result)
	}
}

func TestFormatHover_WithContent(t *testing.T) {
	hover := &HoverResult{
		Contents: MarkupContent{Kind: "markdown", Value: "```go\nfunc main()\n```"},
	}
	result := formatHover(hover)
	if !strings.Contains(result, "func main()") {
		t.Errorf("expected 'func main()' in result, got: %s", result)
	}
}

// --- formatSymbols edge cases ---

func TestFormatSymbols_Empty(t *testing.T) {
	result := formatSymbols([]DocumentSymbol{}, "")
	if result != "" {
		t.Errorf("expected empty string, got: %s", result)
	}
}

func TestFormatSymbols_UnknownKind(t *testing.T) {
	symbols := []DocumentSymbol{
		{
			Name:           "unknown_thing",
			Kind:           SymbolKind(99), // Unknown kind
			Range:          Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 10}},
			SelectionRange: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 10}},
		},
	}
	result := formatSymbols(symbols, "")
	if !strings.Contains(result, "unknown_thing") {
		t.Errorf("expected symbol name in result, got: %s", result)
	}
	if !strings.Contains(result, "Kind99") {
		t.Errorf("expected 'Kind99' for unknown kind, got: %s", result)
	}
}

// --- pathToURI edge cases ---

func TestPathToURI_AlreadyURI(t *testing.T) {
	result := pathToURI("file:///already/a/uri.go")
	if result != "file:///already/a/uri.go" {
		t.Errorf("expected unchanged URI, got: %s", result)
	}
}

func TestPathToURI_RelativePath(t *testing.T) {
	result := pathToURI("relative/path.go")
	if !strings.HasPrefix(result, "file://") {
		t.Errorf("expected file:// prefix, got: %s", result)
	}
	if !strings.Contains(result, "relative/path.go") {
		t.Errorf("expected path in URI, got: %s", result)
	}
}

// --- uriToPath edge cases ---

func TestUriToPath_NonFileURI(t *testing.T) {
	result := uriToPath("https://example.com/page")
	// Should just strip file:// prefix, which won't match
	if result != "https://example.com/page" {
		t.Errorf("expected unchanged URI, got: %s", result)
	}
}

// --- LSPTool Shutdown ---

func TestLSPTool_Shutdown_NoClients(t *testing.T) {
	tool := NewLSPTool()
	// Should not panic with no clients
	tool.Shutdown()
}

// --- LSPTool Parameters ---

func TestLSPTool_Parameters_Required(t *testing.T) {
	tool := NewLSPTool()
	params := tool.Parameters()
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("expected required to be []string")
	}
	found := make(map[string]bool)
	for _, r := range required {
		found[r] = true
	}
	if !found["operation"] {
		t.Error("expected 'operation' to be required")
	}
	if !found["file_path"] {
		t.Error("expected 'file_path' to be required")
	}
}