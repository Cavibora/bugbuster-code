package agent

import (
	"strings"
	"testing"

	"bugbuster-code/pkg/provider"
)

// assistantMsg creates a simple assistant message with text content.
func assistantMsg(text string) provider.Message {
	return provider.Message{
		Role: "assistant",
		Content: []provider.ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

func TestCleanupTransients_RemovesOldAutoContinue(t *testing.T) {
	ctx := NewConversationContextWithTokens(8000, 20)
	ctx.Add(provider.SystemMsg("system"))
	ctx.Add(provider.UserMsg("Do the task"))
	ctx.Add(assistantMsg("Working on it..."))
	// Old auto-continue — should be removed
	ctx.Add(provider.UserMsg("Continue working. Use tools to read files, run commands, or search code. Do not just describe what to do — actually do it using tools."))
	ctx.Add(assistantMsg("Still working..."))
	// Old "Continue." — should be removed
	ctx.Add(provider.UserMsg("Continue."))
	ctx.Add(assistantMsg("More work..."))
	// Latest "Continue." — should be kept
	ctx.Add(provider.UserMsg("Continue."))
	ctx.Add(assistantMsg("Final answer"))

	before := len(ctx.Messages)
	ctx.CleanupTransients()
	after := len(ctx.Messages)

	if after >= before {
		t.Errorf("CleanupTransients should reduce message count: before=%d, after=%d", before, after)
	}

	// Check that only the last "Continue." remains
	continueCount := 0
	for _, m := range ctx.Messages {
		if m.Role == "user" && m.GetResponseText() == "Continue." {
			continueCount++
		}
	}
	if continueCount != 1 {
		t.Errorf("Expected 1 'Continue.' message after cleanup, got %d", continueCount)
	}

	// Check that the long auto-continue hint was removed
	for _, m := range ctx.Messages {
		text := m.GetResponseText()
		if strings.Contains(text, "Continue working. Use tools") {
			t.Errorf("Old auto-continue hint should be removed, found: %s", text[:min(50, len(text))])
		}
	}
}

func TestCleanupTransients_RemovesTransientErrors(t *testing.T) {
	ctx := NewConversationContextWithTokens(8000, 20)
	ctx.Add(provider.SystemMsg("system"))
	ctx.Add(provider.UserMsg("Do the task"))
	ctx.Add(assistantMsg("Working on it..."))
	// Transient error message — should be removed
	ctx.Add(provider.UserMsg("⚠️ stream_idle_timeout (300s). If the model is thinking for a long time, consider using compact_force."))
	ctx.Add(assistantMsg("Continuing..."))
	// Normal message — should be kept
	ctx.Add(provider.UserMsg("Please fix the bug in main.go"))

	before := len(ctx.Messages)
	ctx.CleanupTransients()
	after := len(ctx.Messages)

	if after >= before {
		t.Errorf("CleanupTransients should reduce message count: before=%d, after=%d", before, after)
	}

	// Check that idle timeout hint was removed
	for _, m := range ctx.Messages {
		text := m.GetResponseText()
		if strings.Contains(text, "stream_idle") {
			t.Errorf("Transient error message should be removed, found: %s", text[:min(50, len(text))])
		}
	}
}

func TestCleanupTransients_KeepsNormalMessages(t *testing.T) {
	ctx := NewConversationContextWithTokens(8000, 20)
	ctx.Add(provider.SystemMsg("system"))
	ctx.Add(provider.UserMsg("Do the task"))
	ctx.Add(assistantMsg("Working on it..."))
	ctx.Add(provider.UserMsg("What about the error in line 42?"))
	ctx.Add(assistantMsg("Let me check..."))

	before := len(ctx.Messages)
	ctx.CleanupTransients()
	after := len(ctx.Messages)

	if after != before {
		t.Errorf("CleanupTransients should not remove normal messages: before=%d, after=%d", before, after)
	}
}

func TestCleanupTransients_EmptyContext(t *testing.T) {
	ctx := NewConversationContextWithTokens(8000, 20)
	ctx.CleanupTransients() // should not panic
	if len(ctx.Messages) != 0 {
		t.Errorf("Empty context should remain empty after cleanup")
	}
}

func TestIsAutoContinueMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  provider.Message
		want bool
	}{
		{
			name: "Continue.",
			msg:  provider.UserMsg("Continue."),
			want: true,
		},
		{
			name: "Continue without dot",
			msg:  provider.UserMsg("Continue"),
			want: true,
		},
		{
			name: "Continue working",
			msg:  provider.UserMsg("Continue working. Use tools to read files."),
			want: true,
		},
		{
			name: "You responded with text only",
			msg:  provider.UserMsg("You responded with text only, but your task is not done yet.\nOriginal task: fix the bug"),
			want: true,
		},
		{
			name: "Normal user message",
			msg:  provider.UserMsg("Please fix the bug"),
			want: false,
		},
		{
			name: "Assistant message",
			msg:  assistantMsg("Continue."),
			want: false,
		},
		{
			name: "Empty message",
			msg:  provider.UserMsg(""),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAutoContinueMessage(tt.msg)
			if got != tt.want {
				t.Errorf("isAutoContinueMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTransientErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  provider.Message
		want bool
	}{
		{
			name: "idle timeout hint",
			msg:  provider.UserMsg("⚠️ stream_idle_timeout (300s). If the model is thinking..."),
			want: true,
		},
		{
			name: "loop detection hint",
			msg:  provider.UserMsg("loop_detector.hint_thinking_loop: detected repeated thinking"),
			want: true,
		},
		{
			name: "strategy hint",
			msg:  provider.UserMsg("loop_detector.strategy_hint: try a different approach"),
			want: true,
		},
		{
			name: "Normal user message",
			msg:  provider.UserMsg("Please fix the bug"),
			want: false,
		},
		{
			name: "Assistant message",
			msg:  assistantMsg("stream_idle_timeout"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransientErrorMessage(tt.msg)
			if got != tt.want {
				t.Errorf("isTransientErrorMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}
