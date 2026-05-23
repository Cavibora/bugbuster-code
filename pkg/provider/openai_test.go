package provider

import (
	"strings"
	"testing"
)

func newOpenAIProvider(t *testing.T) *OpenAIProvider {
	t.Helper()
	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type:    "openai",
		APIKey:  "test-key",
		Model:   "gpt-4o",
		BaseURL: "https://api.openai.com/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider error: %v", err)
	}
	return p
}

// --- OpenAI parseResponse ---

func TestOpenAIParseResponse_TextOnly(t *testing.T) {
	p := newOpenAIProvider(t)
	body := `{
		"choices": [{
			"message": {"role": "assistant", "content": "Hello!"},
			"finish_reason": "stop"
		}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 5}
	}`
	result, err := p.parseResponse([]byte(body))
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if result.Message.GetText() != "Hello!" {
		t.Errorf("text = %q, want 'Hello!'", result.Message.GetText())
	}
	if result.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want 'end_turn'", result.StopReason)
	}
}

func TestOpenAIParseResponse_WithReasoning(t *testing.T) {
	p := newOpenAIProvider(t)
	body := `{
		"choices": [{
			"message": {
				"role": "assistant",
				"reasoning_content": "I need to think about this",
				"content": "The answer is 42"
			},
			"finish_reason": "stop"
		}],
		"usage": {"prompt_tokens": 20, "completion_tokens": 10}
	}`
	result, err := p.parseResponse([]byte(body))
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	thinking := result.Message.GetThinking()
	if thinking != "I need to think about this" {
		t.Errorf("thinking = %q, want 'I need to think about this'", thinking)
	}
	response := result.Message.GetResponseText()
	if response != "The answer is 42" {
		t.Errorf("response = %q, want 'The answer is 42'", response)
	}
}

func TestOpenAIParseResponse_ToolCalls(t *testing.T) {
	p := newOpenAIProvider(t)
	body := `{
		"choices": [{
			"message": {
				"role": "assistant",
				"content": null,
				"tool_calls": [{
					"id": "call_abc",
					"type": "function",
					"function": {
						"name": "read",
						"arguments": {"path": "main.go"}
					}
				}]
			},
			"finish_reason": "tool_calls"
		}],
		"usage": {"prompt_tokens": 50, "completion_tokens": 20}
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
	if calls[0].ToolUseID != "call_abc" {
		t.Errorf("tool id = %q, want 'call_abc'", calls[0].ToolUseID)
	}
}

func TestOpenAIParseResponse_MaxTokens(t *testing.T) {
	p := newOpenAIProvider(t)
	body := `{
		"choices": [{
			"message": {"role": "assistant", "content": "truncated"},
			"finish_reason": "length"
		}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 4096}
	}`
	result, err := p.parseResponse([]byte(body))
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if result.StopReason != "max_tokens" {
		t.Errorf("stop_reason = %q, want 'max_tokens'", result.StopReason)
	}
}

func TestOpenAIParseResponse_EmptyChoices(t *testing.T) {
	p := newOpenAIProvider(t)
	body := `{"choices": [], "usage": {}}`
	_, err := p.parseResponse([]byte(body))
	if err == nil {
		t.Fatal("Expected error for empty choices")
	}
}

func TestOpenAIParseResponse_InvalidJSON(t *testing.T) {
	p := newOpenAIProvider(t)
	_, err := p.parseResponse([]byte("not json"))
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
}

// --- OpenAI buildRequest ---

func TestOpenAIBuildRequest_Basic(t *testing.T) {
	p := newOpenAIProvider(t)
	msgs := []Message{
		SystemMsg("you are helpful"),
		UserMsg("hi"),
	}
	req := p.buildRequest(msgs, nil, false)
	if req["model"] != "gpt-4o" {
		t.Errorf("model = %v, want 'gpt-4o'", req["model"])
	}
	if _, ok := req["stream"]; ok {
		t.Error("Expected no stream in non-streaming request")
	}
}

func TestOpenAIBuildRequest_Stream(t *testing.T) {
	p := newOpenAIProvider(t)
	req := p.buildRequest(nil, nil, true)
	if req["stream"] != true {
		t.Error("Expected stream=true")
	}
}

func TestOpenAIBuildRequest_WithTools(t *testing.T) {
	p := newOpenAIProvider(t)
	tools := []ToolDef{
		{Name: "bash", Description: "Run commands", Parameters: map[string]any{"type": "object"}},
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
	if toolsList[0]["type"] != "function" {
		t.Errorf("tool type = %v, want 'function'", toolsList[0]["type"])
	}
	fn := toolsList[0]["function"].(map[string]any)
	if fn["name"] != "bash" {
		t.Errorf("function name = %v, want 'bash'", fn["name"])
	}
}

// --- OpenAI setHeaders ---

func TestOpenAISetHeaders(t *testing.T) {
	p := newOpenAIProvider(t)
	req, _ := newRequest("POST", "/test", nil)
	p.setHeaders(req)
	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", req.Header.Get("Content-Type"))
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") {
		t.Errorf("Authorization = %q, want Bearer token", req.Header.Get("Authorization"))
	}
}

// --- OpenAI Name ---

func TestOpenAIProvider_Name(t *testing.T) {
	p := newOpenAIProvider(t)
	if p.Name() != "test" {
		t.Errorf("Name() = %q, want 'test'", p.Name())
	}
}
