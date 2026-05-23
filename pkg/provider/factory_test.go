package provider

import (
	"testing"
)

func TestNewDefaultProvider_EmptyProviders(t *testing.T) {
	_, err := NewDefaultProvider(nil, "")
	if err == nil {
		t.Error("Expected error for empty providers")
	}
}

func TestNewDefaultProvider_NoDefault(t *testing.T) {
	providers := map[string]ProviderConfig{
		"test": {
			Type:   "unknown_type",
			APIKey: "test",
			Model:  "test",
		},
	}
	_, err := NewDefaultProvider(providers, "")
	// Should try to create provider with unknown type and fail
	if err == nil {
		t.Error("Expected error for unknown provider type")
	}
}

func TestNewDefaultProvider_NotFound(t *testing.T) {
	providers := map[string]ProviderConfig{
		"openai": {
			Type:   "openai",
			APIKey: "test",
			Model:  "gpt-4",
		},
	}
	_, err := NewDefaultProvider(providers, "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent provider name")
	}
}

func TestNewFromConfig_UnknownType(t *testing.T) {
	cfg := ProviderConfig{
		Type:   "unknown",
		APIKey: "test",
		Model:  "test",
	}
	_, err := NewFromConfig("unknown", cfg)
	if err == nil {
		t.Error("Expected error for unknown provider type")
	}
}

func TestNewFromConfig_OpenAI(t *testing.T) {
	cfg := ProviderConfig{
		Type:   "openai",
		APIKey: "test-key",
		Model:  "gpt-4",
	}
	p, err := NewFromConfig("openai", cfg)
	if err != nil {
		t.Fatalf("NewFromConfig error: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("Expected name 'openai', got '%s'", p.Name())
	}
}

func TestNewFromConfig_Anthropic(t *testing.T) {
	cfg := ProviderConfig{
		Type:   "anthropic",
		APIKey: "test-key",
		Model:  "claude-3",
	}
	p, err := NewFromConfig("anthropic", cfg)
	if err != nil {
		t.Fatalf("NewFromConfig error: %v", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("Expected name 'anthropic', got '%s'", p.Name())
	}
}

func TestNewFromConfig_Ollama(t *testing.T) {
	cfg := ProviderConfig{
		Type:    "ollama",
		Model:   "llama3",
		BaseURL: "http://localhost:11434",
	}
	p, err := NewFromConfig("ollama", cfg)
	if err != nil {
		t.Fatalf("NewFromConfig error: %v", err)
	}
	if p.Name() != "ollama" {
		t.Errorf("Expected name 'ollama', got '%s'", p.Name())
	}
}

func TestNewFromConfig_OpenAICompat(t *testing.T) {
	cfg := ProviderConfig{
		Type:    "openai_compat",
		APIKey:  "test-key",
		Model:   "custom-model",
		BaseURL: "http://localhost:8080/v1",
	}
	p, err := NewFromConfig("openai_compat", cfg)
	if err != nil {
		t.Fatalf("NewFromConfig error: %v", err)
	}
	if p.Name() != "openai_compat" {
		t.Errorf("Expected name 'openai_compat', got '%s'", p.Name())
	}
}

func TestMessageHelpers(t *testing.T) {
	// Test SystemMsg
	msg := SystemMsg("system prompt")
	if msg.Role != "system" {
		t.Errorf("Expected role 'system', got '%s'", msg.Role)
	}
	if len(msg.Content) != 1 || msg.Content[0].Text != "system prompt" {
		t.Errorf("Expected content 'system prompt', got %v", msg.Content)
	}

	// Test UserMsg
	msg = UserMsg("user message")
	if msg.Role != "user" {
		t.Errorf("Expected role 'user', got '%s'", msg.Role)
	}

	// Test AssistantText
	msg = AssistantText("assistant response")
	if msg.Role != "assistant" {
		t.Errorf("Expected role 'assistant', got '%s'", msg.Role)
	}

	// Test ToolResultMsg
	msg = ToolResultMsg("tool-1", "read", "result data", false)
	// Anthropic uses role=user for tool_result
	if msg.Role != "user" {
		t.Errorf("Expected role 'user' for tool_result, got '%s'", msg.Role)
	}
}

func TestMessageGetThinking(t *testing.T) {
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "thinking", Text: "I need to think about this"},
			{Type: "text", Text: "Here is my answer"},
		},
	}
	thinking := msg.GetThinking()
	if thinking != "I need to think about this" {
		t.Errorf("Expected thinking 'I need to think about this', got '%s'", thinking)
	}
}

func TestMessageGetResponseText(t *testing.T) {
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "thinking", Text: "thinking..."},
			{Type: "text", Text: "response text"},
		},
	}
	text := msg.GetResponseText()
	if text != "response text" {
		t.Errorf("Expected 'response text', got '%s'", text)
	}
}

func TestMessageGetToolCalls(t *testing.T) {
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "tool_use", ToolUseID: "call-1", ToolName: "read", Input: map[string]any{"path": "main.go"}},
		},
	}
	calls := msg.GetToolCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(calls))
	}
	if calls[0].ToolName != "read" {
		t.Errorf("Expected tool 'read', got '%s'", calls[0].ToolName)
	}
}

func TestMessageHasToolCalls(t *testing.T) {
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "text", Text: "I'll read the file"},
			{Type: "tool_use", ToolUseID: "call-1", ToolName: "read"},
		},
	}
	if !msg.HasToolCalls() {
		t.Error("Expected HasToolCalls=true")
	}

	msg2 := Message{
		Role:    "assistant",
		Content: []ContentBlock{{Type: "text", Text: "No tools here"}},
	}
	if msg2.HasToolCalls() {
		t.Error("Expected HasToolCalls=false")
	}
}

func TestMessageToPlainText(t *testing.T) {
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "text", Text: "Hello"},
			{Type: "text", Text: "World"},
		},
	}
	text := msg.ToPlainText()
	if text != "Hello\nWorld" {
		t.Errorf("Expected 'Hello\\nWorld', got '%s'", text)
	}
}
