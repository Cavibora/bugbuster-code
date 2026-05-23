package provider

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// --- Anthropic convertMessage ---

func newAnthropicProvider(t *testing.T) *AnthropicProvider {
	t.Helper()
	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type:    "anthropic",
		APIKey:  "test-key",
		Model:   "test-model",
		BaseURL: "https://api.example.com",
	})
	if err != nil {
		t.Fatalf("NewAnthropicProvider error: %v", err)
	}
	return p
}

func TestAnthropicConvertMessage_PlainText(t *testing.T) {
	p := newAnthropicProvider(t)
	result := p.convertMessage(UserMsg("hello"))
	if len(result) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(result))
	}
	if result[0]["role"] != "user" {
		t.Errorf("role = %v, want 'user'", result[0]["role"])
	}
	// UserMsg has Content blocks, so it's []map[string]any
	content, ok := result[0]["content"].([]map[string]any)
	if ok {
		if len(content) != 1 || content[0]["text"] != "hello" {
			t.Errorf("content = %v, want text 'hello'", content)
		}
	} else {
		// Fallback: plain string content
		if result[0]["content"] != "hello" {
			t.Errorf("content = %v, want 'hello'", result[0]["content"])
		}
	}
}

func TestAnthropicConvertMessage_TextContent(t *testing.T) {
	p := newAnthropicProvider(t)
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "text", Text: "response"},
		},
	}
	result := p.convertMessage(msg)
	if len(result) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(result))
	}
	if result[0]["role"] != "assistant" {
		t.Errorf("role = %v, want 'assistant'", result[0]["role"])
	}
	content := result[0]["content"].([]map[string]any)
	if len(content) != 1 {
		t.Fatalf("Expected 1 content block, got %d", len(content))
	}
	if content[0]["type"] != "text" {
		t.Errorf("block type = %v, want 'text'", content[0]["type"])
	}
	if content[0]["text"] != "response" {
		t.Errorf("block text = %v, want 'response'", content[0]["text"])
	}
}

func TestAnthropicConvertMessage_ToolUse(t *testing.T) {
	p := newAnthropicProvider(t)
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "tool_use", ToolUseID: "call_1", ToolName: "read", Input: map[string]any{"path": "main.go"}},
		},
	}
	result := p.convertMessage(msg)
	content := result[0]["content"].([]map[string]any)
	if content[0]["type"] != "tool_use" {
		t.Errorf("block type = %v, want 'tool_use'", content[0]["type"])
	}
	if content[0]["id"] != "call_1" {
		t.Errorf("block id = %v, want 'call_1'", content[0]["id"])
	}
	if content[0]["name"] != "read" {
		t.Errorf("block name = %v, want 'read'", content[0]["name"])
	}
}

func TestAnthropicConvertMessage_ToolResult(t *testing.T) {
	p := newAnthropicProvider(t)
	msg := ToolResultMsg("call_1", "read", "file content", false)
	result := p.convertMessage(msg)
	content := result[0]["content"].([]map[string]any)
	if content[0]["type"] != "tool_result" {
		t.Errorf("block type = %v, want 'tool_result'", content[0]["type"])
	}
	if content[0]["tool_use_id"] != "call_1" {
		t.Errorf("block tool_use_id = %v, want 'call_1'", content[0]["tool_use_id"])
	}
	if content[0]["content"] != "file content" {
		t.Errorf("block content = %v, want 'file content'", content[0]["content"])
	}
}

func TestAnthropicConvertMessage_ToolResultError(t *testing.T) {
	p := newAnthropicProvider(t)
	msg := ToolResultMsg("call_1", "bash", "command failed", true)
	result := p.convertMessage(msg)
	content := result[0]["content"].([]map[string]any)
	if content[0]["is_error"] != true {
		t.Error("Expected is_error=true for error tool result")
	}
}

func TestAnthropicConvertMessage_MixedContent(t *testing.T) {
	p := newAnthropicProvider(t)
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "thinking", Text: "let me think"},
			{Type: "text", Text: "here is the answer"},
			{Type: "tool_use", ToolUseID: "call_1", ToolName: "read", Input: map[string]any{"path": "x.go"}},
		},
	}
	result := p.convertMessage(msg)
	content := result[0]["content"].([]map[string]any)
	if len(content) != 3 {
		t.Fatalf("Expected 3 blocks, got %d", len(content))
	}
}

// --- Anthropic buildRequest ---

func TestAnthropicBuildRequest_SystemMessage(t *testing.T) {
	p := newAnthropicProvider(t)
	messages := []Message{
		SystemMsg("you are a helper"),
		UserMsg("hi"),
	}
	req := p.buildRequest(messages, nil, false)

	sys, ok := req["system"]
	if !ok {
		t.Fatal("Expected 'system' key in request")
	}
	sysStr, ok := sys.(string)
	if !ok || !strings.Contains(sysStr, "you are a helper") {
		t.Errorf("system = %v, want to contain 'you are a helper'", sys)
	}
}

func TestAnthropicBuildRequest_WithTools(t *testing.T) {
	p := newAnthropicProvider(t)
	tools := []ToolDef{
		{
			Name:        "read",
			Description: "Read file",
			Parameters:  map[string]any{"type": "object"},
		},
	}
	req := p.buildRequest(nil, tools, false)

	toolsArr, ok := req["tools"]
	if !ok {
		t.Fatal("Expected 'tools' key in request")
	}
	toolsList, ok := toolsArr.([]map[string]any)
	if !ok || len(toolsList) != 1 {
		t.Fatalf("tools = %v, want 1 tool", toolsArr)
	}
	if toolsList[0]["name"] != "read" {
		t.Errorf("tool name = %v, want 'read'", toolsList[0]["name"])
	}
}

func TestAnthropicBuildRequest_Stream(t *testing.T) {
	p := newAnthropicProvider(t)
	req := p.buildRequest(nil, nil, true)
	if req["stream"] != true {
		t.Error("Expected stream=true")
	}
}

func TestAnthropicBuildRequest_NoStream(t *testing.T) {
	p := newAnthropicProvider(t)
	req := p.buildRequest(nil, nil, false)
	if _, ok := req["stream"]; ok {
		t.Error("Expected no stream key in non-streaming request")
	}
}

// --- Anthropic parseResponse ---

func TestAnthropicParseResponse_TextOnly(t *testing.T) {
	p := newAnthropicProvider(t)
	body := `{
		"content": [
			{"type": "text", "text": "Hello!"}
		],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`

	result, err := p.parseResponse([]byte(body))
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if result.Message.GetText() != "Hello!" {
		t.Errorf("text = %q, want %q", result.Message.GetText(), "Hello!")
	}
	if result.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want %q", result.StopReason, "end_turn")
	}
	if result.Usage.InputTokens != 10 {
		t.Errorf("input_tokens = %d, want 10", result.Usage.InputTokens)
	}
}

func TestAnthropicParseResponse_ToolUse(t *testing.T) {
	p := newAnthropicProvider(t)
	body := `{
		"content": [
			{"type": "text", "text": "Let me read that file."},
			{"type": "tool_use", "id": "call_123", "name": "read", "input": {"path": "main.go"}}
		],
		"stop_reason": "tool_use",
		"usage": {"input_tokens": 50, "output_tokens": 20}
	}`

	result, err := p.parseResponse([]byte(body))
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if result.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want 'tool_use'", result.StopReason)
	}
	calls := result.Message.GetToolCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(calls))
	}
	if calls[0].ToolName != "read" {
		t.Errorf("tool name = %q, want 'read'", calls[0].ToolName)
	}
	if calls[0].ToolUseID != "call_123" {
		t.Errorf("tool id = %q, want 'call_123'", calls[0].ToolUseID)
	}
}

func TestAnthropicParseResponse_MaxTokens(t *testing.T) {
	p := newAnthropicProvider(t)
	body := `{
		"content": [{"type": "text", "text": "truncated"}],
		"stop_reason": "max_tokens",
		"usage": {"input_tokens": 10, "output_tokens": 4096}
	}`
	result, err := p.parseResponse([]byte(body))
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if result.StopReason != "max_tokens" {
		t.Errorf("stop_reason = %q, want 'max_tokens'", result.StopReason)
	}
}

func TestAnthropicParseResponse_InvalidJSON(t *testing.T) {
	p := newAnthropicProvider(t)
	_, err := p.parseResponse([]byte("not json"))
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
}

// --- Anthropic setHeaders ---

func TestAnthropicSetHeaders(t *testing.T) {
	p := newAnthropicProvider(t)
	req, _ := newRequest("POST", "/test", nil)
	p.setHeaders(req)

	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want 'application/json'", req.Header.Get("Content-Type"))
	}
	if req.Header.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("anthropic-version = %q, want '2023-06-01'", req.Header.Get("anthropic-version"))
	}
	if req.Header.Get("x-api-key") != "test-key" {
		t.Errorf("x-api-key = %q, want 'test-key'", req.Header.Get("x-api-key"))
	}
}

// --- Anthropic Name ---

func TestAnthropicProvider_Name(t *testing.T) {
	p := newAnthropicProvider(t)
	if p.Name() != "test" {
		t.Errorf("Name() = %q, want 'test'", p.Name())
	}
}

// --- Anthropic buildRequest thinking ---

func TestAnthropicBuildRequest_ThinkingEnabled(t *testing.T) {
	p := newAnthropicProvider(t)
	req := p.buildRequest(nil, nil, false)
	thinking, ok := req["thinking"]
	if !ok {
		t.Fatal("Expected 'thinking' key in request")
	}
	thinkingMap, ok := thinking.(map[string]any)
	if !ok {
		t.Fatalf("thinking = %T, want map", thinking)
	}
	if thinkingMap["type"] != "enabled" {
		t.Errorf("thinking type = %v, want 'enabled'", thinkingMap["type"])
	}
}

// --- helper ---

func newRequest(method, url string, body io.Reader) (*http.Request, error) {
	return http.NewRequest(method, url, body)
}
