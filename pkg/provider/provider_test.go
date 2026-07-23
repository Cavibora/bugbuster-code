package provider

import (
	"os"
	"strings"
	"testing"
)

func TestSystemMsg(t *testing.T) {
	msg := SystemMsg("ты помощник")
	if msg.Role != "system" {
		t.Errorf("Expected role 'system', got '%s'", msg.Role)
	}
	if msg.GetText() != "ты помощник" {
		t.Errorf("Expected text 'ты помощник', got '%s'", msg.GetText())
	}
}

func TestUserMsg(t *testing.T) {
	msg := UserMsg("привет")
	if msg.Role != "user" {
		t.Errorf("Expected role 'user', got '%s'", msg.Role)
	}
}

func TestAssistantText(t *testing.T) {
	msg := AssistantText("здравствуй")
	if msg.Role != "assistant" {
		t.Errorf("Expected role 'assistant', got '%s'", msg.Role)
	}
}

func TestAssistantToolCalls(t *testing.T) {
	blocks := []ContentBlock{
		{Type: "tool_use", ToolUseID: "call_1", ToolName: "read", Input: map[string]any{"path": "main.go"}},
	}
	msg := AssistantToolCalls(blocks)
	if msg.Role != "assistant" {
		t.Errorf("Expected role 'assistant', got '%s'", msg.Role)
	}
	if !msg.HasToolCalls() {
		t.Error("Expected HasToolCalls() to be true")
	}
	calls := msg.GetToolCalls()
	if len(calls) != 1 {
		t.Errorf("Expected 1 tool call, got %d", len(calls))
	}
	if calls[0].ToolName != "read" {
		t.Errorf("Expected tool name 'read', got '%s'", calls[0].ToolName)
	}
}

func TestToolResultMsg(t *testing.T) {
	msg := ToolResultMsg("call_1", "read", "file contents", false)
	if msg.Role != "user" {
		t.Errorf("Expected role 'user', got '%s'", msg.Role)
	}
	if len(msg.Content) != 1 {
		t.Errorf("Expected 1 content block, got %d", len(msg.Content))
	}
	if msg.Content[0].Type != "tool_result" {
		t.Errorf("Expected type 'tool_result', got '%s'", msg.Content[0].Type)
	}
}

func TestMessagesToText(t *testing.T) {
	messages := []Message{
		SystemMsg("ты помощник"),
		UserMsg("привет"),
		AssistantText("здравствуй"),
	}

	text := MessagesToText(messages)
	if text == "" {
		t.Error("Expected non-empty text")
	}
	if !strings.Contains(text, "system") {
		t.Error("Expected text to contain 'system'")
	}
	if !strings.Contains(text, "привет") {
		t.Error("Expected text to contain 'привет'")
	}
}

func TestProviderConfigDefaultBaseURL(t *testing.T) {
	tests := []struct {
		providerType string
		expected     string
	}{
		{"openai", "https://api.openai.com/v1"},
		{"anthropic", "https://api.anthropic.com"},
		{"ollama", "http://localhost:11434"},
		{"cavibora", "https://api.cavibora.com"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		cfg := ProviderConfig{Type: tt.providerType}
		result := cfg.DefaultBaseURL()
		if result != tt.expected {
			t.Errorf("ProviderConfig{Type:'%s'}.DefaultBaseURL() = '%s', want '%s'",
				tt.providerType, result, tt.expected)
		}
	}
}

func TestProviderConfigGetBaseURL(t *testing.T) {
	// С явным URL
	cfg := ProviderConfig{Type: "openai", BaseURL: "https://custom.api.com/v1"}
	if cfg.GetBaseURL() != "https://custom.api.com/v1" {
		t.Errorf("Expected custom URL, got '%s'", cfg.GetBaseURL())
	}

	// Без URL — дефолтный
	cfg = ProviderConfig{Type: "ollama"}
	if cfg.GetBaseURL() != "http://localhost:11434" {
		t.Errorf("Expected default Ollama URL, got '%s'", cfg.GetBaseURL())
	}
}

func TestNewFromConfig(t *testing.T) {
	tests := []struct {
		providerType string
		shouldError  bool
	}{
		{"openai", false},
		{"anthropic", false},
		{"ollama", false},
		{"cavibora", false},
		{"openai_compat", false}, // has base_url
		{"unknown", true},
	}

	for _, tt := range tests {
		cfg := ProviderConfig{
			Type:    tt.providerType,
			Model:   "test-model",
			APIKey:  "test-key",
			BaseURL: "http://localhost:8080/v1", // для openai_compat
		}

		if tt.providerType == "openai_compat" {
			cfg.BaseURL = "http://localhost:8080/v1"
		}

		_, err := NewFromConfig("test", cfg)
		if tt.shouldError && err == nil {
			t.Errorf("Expected error for provider type '%s'", tt.providerType)
		}
		if !tt.shouldError && err != nil {
			t.Errorf("Unexpected error for provider type '%s': %v", tt.providerType, err)
		}
	}
}

func TestLoadSystemPromptFile_Empty(t *testing.T) {
	cfg := ProviderConfig{SystemPromptFile: ""}
	result := cfg.LoadSystemPromptFile()
	if result != "" {
		t.Errorf("Expected empty string for empty SystemPromptFile, got '%s'", result)
	}
}

func TestLoadSystemPromptFile_NotFound(t *testing.T) {
	cfg := ProviderConfig{SystemPromptFile: "/nonexistent/path/prompt.md"}
	result := cfg.LoadSystemPromptFile()
	if result != "" {
		t.Errorf("Expected empty string for nonexistent file, got '%s'", result)
	}
}

func TestLoadSystemPromptFile_Valid(t *testing.T) {
	// Create a temp file with content
	tmpFile, err := os.CreateTemp("", "system_prompt_*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	content := "# Custom System Prompt\n\nYou are a Rust expert.\nAlways follow best practices."
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	cfg := ProviderConfig{SystemPromptFile: tmpFile.Name()}
	result := cfg.LoadSystemPromptFile()
	if result != content {
		t.Errorf("Expected '%s', got '%s'", content, result)
	}
}

func TestLoadSystemPromptFile_TrimsWhitespace(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "system_prompt_*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	content := "  \n  Hello World  \n  "
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	cfg := ProviderConfig{SystemPromptFile: tmpFile.Name()}
	result := cfg.LoadSystemPromptFile()
	if result != "Hello World" {
		t.Errorf("Expected trimmed 'Hello World', got '%s'", result)
	}
}

func TestProviderConfig_SystemPromptFileAndSkillsDir(t *testing.T) {
	// Test that SystemPromptFile and SkillsDir fields are properly set
	cfg := ProviderConfig{
		Type:             "openai",
		SystemPromptFile: "/path/to/prompt.md",
		SkillsDir:        "/path/to/skills",
		Skills:           []string{"debug", "refactor"},
	}
	if cfg.SystemPromptFile != "/path/to/prompt.md" {
		t.Errorf("Expected SystemPromptFile='/path/to/prompt.md', got '%s'", cfg.SystemPromptFile)
	}
	if cfg.SkillsDir != "/path/to/skills" {
		t.Errorf("Expected SkillsDir='/path/to/skills', got '%s'", cfg.SkillsDir)
	}
	if len(cfg.Skills) != 2 {
		t.Errorf("Expected 2 skills, got %d", len(cfg.Skills))
	}
}

func TestSSEParser(t *testing.T) {
	input := "event: message\ndata: {\"content\": \"hello\"}\n\nevent: done\ndata: [DONE]\n\n"

	var events []string
	var dataItems []string

	err := ParseSSE(strings.NewReader(input), func(event, data string) {
		events = append(events, event)
		dataItems = append(dataItems, data)
	})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}
}

func TestExtractJSONFromSSE(t *testing.T) {
	input := "data: {\"choices\": [{\"delta\": {\"content\": \"hi\"}}]}\n\ndata: [DONE]\n\n"

	var jsonItems []string
	err := ExtractJSONFromSSE(strings.NewReader(input), func(jsonStr string) error {
		jsonItems = append(jsonItems, jsonStr)
		return nil
	})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(jsonItems) != 1 {
		t.Errorf("Expected 1 JSON item, got %d", len(jsonItems))
	}
}
