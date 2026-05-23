package provider

import (
	"strings"
	"testing"
)

// --- ParseSSELines ---

func TestParseSSELines_MultipleEvents(t *testing.T) {
	input := "event: message\ndata: hello\n\nevent: message\ndata: world\n\n"
	lines, err := ParseSSELines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseSSELines error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("Expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "hello" {
		t.Errorf("lines[0] = %q, want %q", lines[0], "hello")
	}
	if lines[1] != "world" {
		t.Errorf("lines[1] = %q, want %q", lines[1], "world")
	}
}

func TestParseSSELines_MultiLineData(t *testing.T) {
	input := "data: line1\ndata: line2\n\n"
	lines, err := ParseSSELines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseSSELines error: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("Expected 1 line, got %d", len(lines))
	}
	if lines[0] != "line1\nline2" {
		t.Errorf("lines[0] = %q, want %q", lines[0], "line1\nline2")
	}
}

// --- ConcatDataChunks ---

func TestConcatDataChunks(t *testing.T) {
	chunks := []string{`{"part":`, `1}`, `{"part":`, `2}`}
	result := ConcatDataChunks(chunks)
	expected := `{"part":1}{"part":2}`
	if result != expected {
		t.Errorf("ConcatDataChunks() = %q, want %q", result, expected)
	}
}

func TestConcatDataChunks_Empty(t *testing.T) {
	result := ConcatDataChunks(nil)
	if result != "" {
		t.Errorf("ConcatDataChunks(nil) = %q, want empty", result)
	}
}

// --- ReadFullBody ---

func TestReadFullBody(t *testing.T) {
	input := "hello world body content"
	data, err := ReadFullBody(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ReadFullBody error: %v", err)
	}
	if string(data) != input {
		t.Errorf("ReadFullBody() = %q, want %q", string(data), input)
	}
}

// --- IsJSON ---

func TestIsJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{`{"key": "value"}`, true},
		{`[1, 2, 3]`, true},
		{`  {"key": "value"}  `, true},
		{`plain text`, false},
		{``, false},
		{`   `, false},
		{`null`, false},
	}
	for _, tt := range tests {
		got := IsJSON(tt.input)
		if got != tt.expected {
			t.Errorf("IsJSON(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

// --- ExtractJSONFromSSE edge cases ---

func TestExtractJSONFromSSE_WithDoneMarker(t *testing.T) {
	input := "data: {\"content\": \"hi\"}\n\ndata: [DONE]\n\n"
	var extracted []string
	err := ExtractJSONFromSSE(strings.NewReader(input), func(jsonStr string) error {
		extracted = append(extracted, jsonStr)
		return nil
	})
	if err != nil {
		t.Fatalf("ExtractJSONFromSSE error: %v", err)
	}
	if len(extracted) != 1 {
		t.Fatalf("Expected 1 JSON, got %d", len(extracted))
	}
	if extracted[0] != `{"content": "hi"}` {
		t.Errorf("extracted[0] = %q, want %q", extracted[0], `{"content": "hi"}`)
	}
}

func TestExtractJSONFromSSE_MultipleJSONInOneData(t *testing.T) {
	input := "data: {\"a\":1}{\"b\":2}\n\n"
	var extracted []string
	err := ExtractJSONFromSSE(strings.NewReader(input), func(jsonStr string) error {
		extracted = append(extracted, jsonStr)
		return nil
	})
	if err != nil {
		t.Fatalf("ExtractJSONFromSSE error: %v", err)
	}
	if len(extracted) != 2 {
		t.Fatalf("Expected 2 JSON objects, got %d: %v", len(extracted), extracted)
	}
}

// --- ParseSSE with comments and ignored fields ---

func TestParseSSE_IgnoresComments(t *testing.T) {
	input := ": this is a comment\ndata: payload\n\n"
	var gotData string
	err := ParseSSE(strings.NewReader(input), func(event, data string) {
		gotData = data
	})
	if err != nil {
		t.Fatalf("ParseSSE error: %v", err)
	}
	if gotData != "payload" {
		t.Errorf("data = %q, want %q", gotData, "payload")
	}
}

func TestParseSSE_NoTrailingNewline(t *testing.T) {
	input := "data: lastevent"
	var gotData string
	err := ParseSSE(strings.NewReader(input), func(event, data string) {
		gotData = data
	})
	if err != nil {
		t.Fatalf("ParseSSE error: %v", err)
	}
	if gotData != "lastevent" {
		t.Errorf("data = %q, want %q", gotData, "lastevent")
	}
}

// --- splitJSON ---

func TestSplitJSON_Simple(t *testing.T) {
	input := `{"a":1}{"b":2}`
	result := splitJSON(input)
	if len(result) != 2 {
		t.Fatalf("Expected 2 JSON objects, got %d: %v", len(result), result)
	}
}

func TestSplitJSON_WithEscapedStrings(t *testing.T) {
	// JSON с экранированными строками, содержащими { и }
	// Это реальный кейс из OpenAI streaming с tool_calls
	input := `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]},"finish_reason":null}]}`
	result := splitJSON(input)
	if len(result) != 1 {
		t.Fatalf("Expected 1 JSON object, got %d: %v", len(result), result)
	}
}

func TestSplitJSON_WithNestedEscapedStrings(t *testing.T) {
	input := `{"a":"{\"b\":{\"c\":1}}"}`
	result := splitJSON(input)
	if len(result) != 1 {
		t.Fatalf("Expected 1 JSON object, got %d: %v", len(result), result)
	}
}
