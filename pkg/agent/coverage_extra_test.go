package agent

import (
	"strings"
	"testing"

	"bugbuster-code/pkg/provider"
)

func TestGetProvider(t *testing.T) {
	a := NewAgentLoop(nil)
	// nil provider
	if p := a.GetProvider(); p != nil {
		t.Error("Expected nil provider for nil input")
	}

	// With provider
	mock := &MockProvider{}
	a.SetProvider(mock)
	if p := a.GetProvider(); p != mock {
		t.Error("Expected provider to match mock")
	}
}

func TestSetAutoContinue(t *testing.T) {
	a := NewAgentLoop(nil)

	// Default is false
	if a.autoContinue {
		t.Error("Expected autoContinue=false by default")
	}

	// Enable
	a.SetAutoContinue(true)
	if !a.autoContinue {
		t.Error("Expected autoContinue=true after SetAutoContinue(true)")
	}

	// Disable
	a.SetAutoContinue(false)
	if a.autoContinue {
		t.Error("Expected autoContinue=false after SetAutoContinue(false)")
	}
}

func TestGetMessages(t *testing.T) {
	ctx := NewConversationContext(8000)

	// Empty context
	msgs := ctx.GetMessages()
	if len(msgs) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(msgs))
	}

	// Add messages
	ctx.Add(provider.SystemMsg("system"))
	ctx.Add(provider.UserMsg("hello"))

	msgs = ctx.GetMessages()
	if len(msgs) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("Expected first message role=system, got %s", msgs[0].Role)
	}
	if msgs[1].Role != "user" {
		t.Errorf("Expected second message role=user, got %s", msgs[1].Role)
	}

	// Verify it returns a copy (modifying returned slice doesn't affect context)
	msgs[0] = provider.UserMsg("modified")
	original := ctx.GetMessages()
	if original[0].Role != "system" {
		t.Error("GetMessages should return a copy that doesn't affect original")
	}
}

func TestDefaultParamName(t *testing.T) {
	tests := []struct {
		toolName string
		expected string
	}{
		{"read", "path"},
		{"write", "path"},
		{"edit", "path"},
		{"bash", "command"},
		{"grep", "pattern"},
		{"glob", "pattern"},
		{"memory", "action"},
		{"lsp", "operation"},
		{"browse", "action"},
		{"web_fetch", "url"},
		{"ask_user", "question"},
		{"unknown_tool", "param"},
	}

	for _, tt := range tests {
		result := defaultParamName(tt.toolName)
		if result != tt.expected {
			t.Errorf("defaultParamName(%q) = %q, want %q", tt.toolName, result, tt.expected)
		}
	}
}

func TestBuildToolResultSummary(t *testing.T) {
	// Empty results
	result := buildToolResultSummary(nil)
	if result == "" {
		t.Error("Expected non-empty summary for nil results")
	}

	// Single result
	result = buildToolResultSummary([]string{"file contents"})
	if !strings.Contains(result, "file contents") {
		t.Errorf("Expected summary to contain 'file contents', got %q", result)
	}

	// Many results (should limit to 10)
	results := make([]string, 15)
	for i := range results {
		results[i] = "result line"
	}
	result = buildToolResultSummary(results)
	if !strings.Contains(result, "5 more results") {
		t.Errorf("Expected summary to mention '5 more results', got %q", result)
	}

	// Long result (should truncate at 200 chars)
	longResult := strings.Repeat("x", 300)
	result = buildToolResultSummary([]string{longResult})
	if !strings.Contains(result, "...") {
		t.Errorf("Expected summary to truncate long results, got %q", result)
	}
}