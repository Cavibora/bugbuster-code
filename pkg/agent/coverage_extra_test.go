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

func TestCompactForceCooldown(t *testing.T) {
	a := NewAgentLoop(nil)

	// Initially no cooldown
	if a.IsCompactForceCooldown() {
		t.Error("Expected no cooldown initially")
	}

	// Set cooldown
	a.SetCompactForceCooldown()
	if !a.IsCompactForceCooldown() {
		t.Error("Expected cooldown after SetCompactForceCooldown")
	}

	// Cooldown should expire after 60 seconds — test by checking the internal field
	// We can't wait 60s in a test, so just verify the field was set
	if a.compactForceCooldownUntil.IsZero() {
		t.Error("Expected compactForceCooldownUntil to be set")
	}
}

func TestResetAutoContinue(t *testing.T) {
	a := NewAgentLoop(nil)

	// Increment auto-continue count
	a.autoContinueCount = 2
	a.ResetAutoContinue()
	if a.autoContinueCount != 0 {
		t.Errorf("Expected autoContinueCount=0 after ResetAutoContinue, got %d", a.autoContinueCount)
	}
}

func TestLooksLikeCompletion_CompactionAck(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		// Compaction acknowledgment — model is re-establishing context
		{"[Context was compacted to save space. Your original task: fix the bug]", true},
		{"Context was compacted — let me check the current state", true},
		{"Let me check the current state of the project", true},
		{"Let me re-establish what was happening", true},
		{"I'll check the current situation", true},
		{"Позвольте мне проверить текущее состояние проекта", true},
		{"Давайте проверю текущее состояние", true},
		// Recap without colon
		{"※ Recap — fixed the bug", true},
		{"Recap — all changes applied", true},
		{"Итог — задача выполнена", true},
		{"Summary — fixed 3 bugs", true},
		// Not completion
		{"I will now fix the bug by editing main.go", false},
		{"Let me read the file first", false},
	}

	for _, tt := range tests {
		result := LooksLikeCompletion(tt.text)
		if result != tt.expected {
			t.Errorf("LooksLikeCompletion(%q) = %v, want %v", tt.text, result, tt.expected)
		}
	}
}