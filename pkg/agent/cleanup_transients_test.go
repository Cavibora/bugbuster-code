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

func TestIsCompactionReminder(t *testing.T) {
	tests := []struct {
		name string
		msg  provider.Message
		want bool
	}{
		{
			name: "Context was compacted",
			msg:  provider.UserMsg("[Context was compacted to save space. Your original task: fix the bug]"),
			want: true,
		},
		{
			name: "Auto-compacted",
			msg:  provider.UserMsg("[Auto-compacted] Context was critically slow"),
			want: true,
		},
		{
			name: "Normal user message",
			msg:  provider.UserMsg("Please fix the bug"),
			want: false,
		},
		{
			name: "Assistant message",
			msg:  assistantMsg("[Context was compacted]"),
			want: false,
		},
		{
			name: "System message",
			msg:  provider.SystemMsg("[Context was compacted]"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCompactionReminder(tt.msg)
			if got != tt.want {
				t.Errorf("isCompactionReminder() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsPerformanceMirror(t *testing.T) {
	tests := []struct {
		name string
		msg  provider.Message
		want bool
	}{
		{
			name: "Performance mirror",
			msg:  provider.SystemMsg("[Performance mirror] Iteration 5. Avg iteration time: 4.7s."),
			want: true,
		},
		{
			name: "Normal system message",
			msg:  provider.SystemMsg("Ты — BugBuster Code"),
			want: false,
		},
		{
			name: "User message",
			msg:  provider.UserMsg("[Performance mirror] Iteration 5."),
			want: false,
		},
		{
			name: "Assistant message",
			msg:  assistantMsg("[Performance mirror] Iteration 5."),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPerformanceMirror(tt.msg)
			if got != tt.want {
				t.Errorf("isPerformanceMirror() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemoveCompactionReminders_KeepsOnlyLast(t *testing.T) {
	msgs := []provider.Message{
		provider.SystemMsg("system"),
		provider.UserMsg("Do the task"),
		assistantMsg("Working..."),
		provider.UserMsg("[Context was compacted to save space. Your original task: task1]"),
		assistantMsg("Continuing..."),
		provider.UserMsg("[Context was compacted to save space. Your original task: task2]"),
		assistantMsg("More work..."),
		provider.UserMsg("[Auto-compacted] Context was slow"),
		assistantMsg("Final answer"),
	}

	result := RemoveCompactionReminders(msgs)

	// Count remaining compaction reminders — should be exactly 1 (the last one)
	count := 0
	for _, m := range result {
		if isCompactionReminder(m) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Expected 1 compaction reminder after removal, got %d", count)
	}

	// The remaining one should be the last "[Auto-compacted]" message
	for _, m := range result {
		if isCompactionReminder(m) {
			if !strings.Contains(m.GetResponseText(), "[Auto-compacted]") {
				t.Errorf("Expected the last compaction reminder to remain, got: %s", m.GetResponseText())
			}
		}
	}

	// Normal messages should be preserved
	if len(result) != 7 {
		t.Errorf("Expected 7 messages (9 original - 2 removed reminders), got %d", len(result))
	}
}

func TestRemoveCompactionReminders_NoReminders(t *testing.T) {
	msgs := []provider.Message{
		provider.SystemMsg("system"),
		provider.UserMsg("Do the task"),
		assistantMsg("Working..."),
	}

	result := RemoveCompactionReminders(msgs)
	if len(result) != len(msgs) {
		t.Errorf("Should not modify messages without reminders: before=%d, after=%d", len(msgs), len(result))
	}
}

func TestRemovePerformanceMirrors_KeepsOnlyLast(t *testing.T) {
	msgs := []provider.Message{
		provider.SystemMsg("system"),
		provider.UserMsg("Do the task"),
		provider.SystemMsg("[Performance mirror] Iteration 5. Avg iteration time: 4.7s."),
		assistantMsg("Working..."),
		provider.SystemMsg("[Performance mirror] Iteration 10. Avg iteration time: 4.5s."),
		assistantMsg("More work..."),
		provider.SystemMsg("[Performance mirror] Iteration 15. Avg iteration time: 3.9s."),
		assistantMsg("Final answer"),
	}

	result := RemovePerformanceMirrors(msgs)

	// Count remaining mirrors — should be exactly 1 (the last one)
	count := 0
	for _, m := range result {
		if isPerformanceMirror(m) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Expected 1 performance mirror after removal, got %d", count)
	}

	// The remaining one should be Iteration 15
	for _, m := range result {
		if isPerformanceMirror(m) {
			if !strings.Contains(m.GetResponseText(), "Iteration 15") {
				t.Errorf("Expected the last mirror (Iteration 15) to remain, got: %s", m.GetResponseText())
			}
		}
	}

	// Normal messages should be preserved
	if len(result) != 6 {
		t.Errorf("Expected 6 messages (2 system + 2 user + 2 assistant), got %d", len(result))
	}
}

func TestRemovePerformanceMirrors_NoMirrors(t *testing.T) {
	msgs := []provider.Message{
		provider.SystemMsg("system"),
		provider.UserMsg("Do the task"),
		assistantMsg("Working..."),
	}

	result := RemovePerformanceMirrors(msgs)
	if len(result) != len(msgs) {
		t.Errorf("Should not modify messages without mirrors: before=%d, after=%d", len(msgs), len(result))
	}
}

func TestCleanupTransients_RemovesOldCompactionReminders(t *testing.T) {
	ctx := NewConversationContextWithTokens(8000, 20)
	ctx.Add(provider.SystemMsg("system"))
	ctx.Add(provider.UserMsg("Do the task"))
	ctx.Add(assistantMsg("Working..."))
	// Old compaction reminders — should be removed
	ctx.Add(provider.UserMsg("[Context was compacted to save space. Your original task: task1]"))
	ctx.Add(assistantMsg("Continuing..."))
	ctx.Add(provider.UserMsg("[Context was compacted to save space. Your original task: task2]"))
	ctx.Add(assistantMsg("More work..."))
	// Latest compaction reminder — should be kept
	ctx.Add(provider.UserMsg("[Auto-compacted] Context was slow"))
	ctx.Add(assistantMsg("Final answer"))

	before := len(ctx.Messages)
	ctx.CleanupTransients()
	after := len(ctx.Messages)

	if after >= before {
		t.Errorf("CleanupTransients should reduce message count: before=%d, after=%d", before, after)
	}

	// Only the last compaction reminder should remain
	count := 0
	for _, m := range ctx.Messages {
		if isCompactionReminder(m) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Expected 1 compaction reminder after cleanup, got %d", count)
	}
}

func TestCleanupTransients_RemovesOldPerformanceMirrors(t *testing.T) {
	ctx := NewConversationContextWithTokens(8000, 20)
	ctx.Add(provider.SystemMsg("system"))
	ctx.Add(provider.UserMsg("Do the task"))
	// Old performance mirrors — should be removed
	ctx.Add(provider.SystemMsg("[Performance mirror] Iteration 5. Avg iteration time: 4.7s."))
	ctx.Add(assistantMsg("Working..."))
	ctx.Add(provider.SystemMsg("[Performance mirror] Iteration 10. Avg iteration time: 4.5s."))
	ctx.Add(assistantMsg("More work..."))
	// Latest performance mirror — should be kept
	ctx.Add(provider.SystemMsg("[Performance mirror] Iteration 15. Avg iteration time: 3.9s."))
	ctx.Add(assistantMsg("Final answer"))

	before := len(ctx.Messages)
	ctx.CleanupTransients()
	after := len(ctx.Messages)

	if after >= before {
		t.Errorf("CleanupTransients should reduce message count: before=%d, after=%d", before, after)
	}

	// Only the last performance mirror should remain
	count := 0
	for _, m := range ctx.Messages {
		if isPerformanceMirror(m) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Expected 1 performance mirror after cleanup, got %d", count)
	}
}

func TestCleanupTransients_SystemAutoCompactedNotices(t *testing.T) {
	ctx := NewConversationContextWithTokens(8000, 20)
	ctx.Add(provider.SystemMsg("system prompt"))
	ctx.Add(provider.UserMsg("Do the task"))
	// Old system [Auto-compacted] notices with different percentages — should be removed
	ctx.Add(provider.SystemMsg("[Auto-compacted] Context was critically slow (4.0x degradation, 78% usage). Compacted from 78% to 17%. Continue working on your task."))
	ctx.Add(assistantMsg("Working..."))
	ctx.Add(provider.SystemMsg("[Auto-compacted] Context was critically slow (3.5x degradation, 65% usage). Compacted from 65% to 20%. Continue working on your task."))
	ctx.Add(assistantMsg("More work..."))
	// Latest system notice — should be kept
	ctx.Add(provider.SystemMsg("[Auto-compacted] 60%→15%. Continue your task."))
	ctx.Add(assistantMsg("Final answer"))

	before := len(ctx.Messages)
	ctx.CleanupTransients()
	after := len(ctx.Messages)

	if after >= before {
		t.Errorf("CleanupTransients should reduce message count: before=%d, after=%d", before, after)
	}

	// Only the latest system notice should remain
	count := 0
	for _, m := range ctx.Messages {
		if isCompactionReminder(m) && m.Role == "system" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Expected 1 system compaction notice after cleanup, got %d", count)
	}
	// The latest one must be the short one
	last := ctx.Messages[len(ctx.Messages)-2]
	if last.Role != "system" || !strings.Contains(last.GetText(), "60%→15%") {
		t.Errorf("Latest system notice should be kept, got: %v", last)
	}
}

func TestCleanupTransients_KeepsLatestUserAndSystemReminders(t *testing.T) {
	ctx := NewConversationContextWithTokens(8000, 20)
	ctx.Add(provider.SystemMsg("system prompt"))
	ctx.Add(provider.UserMsg("Do the task"))
	ctx.Add(provider.UserMsg("[Context was compacted to save space. Your original task: task1]"))
	ctx.Add(provider.SystemMsg("[Auto-compacted] 78%→17%. Continue your task."))
	ctx.Add(assistantMsg("Working..."))
	ctx.Add(provider.UserMsg("[Context was compacted to save space. Your original task: task2]"))
	ctx.Add(provider.SystemMsg("[Auto-compacted] 60%→15%. Continue your task."))
	ctx.Add(assistantMsg("Final answer"))

	ctx.CleanupTransients()

	// One user reminder (latest) AND one system notice (latest) should remain
	userCount, sysCount := 0, 0
	for _, m := range ctx.Messages {
		if !isCompactionReminder(m) {
			continue
		}
		if m.Role == "user" {
			userCount++
		} else if m.Role == "system" {
			sysCount++
		}
	}
	if userCount != 1 {
		t.Errorf("Expected 1 user compaction reminder, got %d", userCount)
	}
	if sysCount != 1 {
		t.Errorf("Expected 1 system compaction notice, got %d", sysCount)
	}
}

func TestRemoveCompactionReminders_SystemNoticesOnly(t *testing.T) {
	msgs := []provider.Message{
		provider.SystemMsg("system prompt"),
		provider.SystemMsg("[Auto-compacted] 78%→17%. Continue your task."),
		provider.SystemMsg("[Auto-compacted] 65%→20%. Continue your task."),
		provider.SystemMsg("[Auto-compacted] 60%→15%. Continue your task."),
	}

	result := RemoveCompactionReminders(msgs)

	if len(result) != 2 {
		t.Errorf("Expected 2 messages (system prompt + latest notice), got %d", len(result))
	}
	if len(result) > 0 && !strings.Contains(result[len(result)-1].GetText(), "60%→15%") {
		t.Errorf("Latest notice should be kept, got: %s", result[len(result)-1].GetText())
	}
}

func TestCompactContext_RemovesSystemAutoCompactedNotices(t *testing.T) {
	msgs := []provider.Message{
		provider.SystemMsg("system prompt"),
		provider.UserMsg("task"),
		provider.SystemMsg("[Auto-compacted] Context was critically slow (4.0x degradation, 78% usage). Compacted from 78% to 17%."),
		provider.SystemMsg("[Auto-compacted] Context was critically slow (3.5x degradation, 65% usage). Compacted from 65% to 20%."),
		assistantMsg("work"),
	}

	result := CompactContext(msgs, 100000, 20)

	// Only the latest system notice may remain — no accumulation
	sysNoticeCount := 0
	for _, m := range result {
		if m.Role == "system" && strings.HasPrefix(m.GetText(), "[Auto-compacted]") {
			sysNoticeCount++
		}
	}
	if sysNoticeCount > 1 {
		t.Errorf("CompactContext should keep at most 1 system [Auto-compacted] notice, got %d", sysNoticeCount)
	}
}
