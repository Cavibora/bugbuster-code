package agent

import (
	"strings"
	"testing"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

func TestIsTransientNoise(t *testing.T) {
	i18n.Init("en")

	tests := []struct {
		name string
		msg  provider.Message
		want bool
	}{
		{"compaction reminder user", provider.UserMsg("[Context was compacted to save space. Your original task: fix bug]"), true},
		{"compaction reminder system", provider.SystemMsg("[Auto-compacted] 78%→17%. Continue your task."), true},
		{"compacted short user", provider.UserMsg("[Compacted. Original task: fix bug. Continue.]"), true},
		{"performance mirror", provider.SystemMsg("[Performance mirror] Iter 5. Avg 4.7s/iter."), true},
		{"auto-continue short", provider.UserMsg("Continue. Use tools."), true},
		{"auto-continue with task", provider.UserMsg("Text-only reply, task not done. Original task: fix bug. Continue using tools."), true},
		{"hub notice", provider.SystemMsg("[Hub] Agent A changed file X"), true},
		{"regular user message", provider.UserMsg("Fix the bug in main.go"), false},
		{"regular assistant message", provider.AssistantText("I fixed the bug"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientNoise(tt.msg); got != tt.want {
				t.Errorf("isTransientNoise(%q) = %v, want %v", tt.msg.GetText(), got, tt.want)
			}
		})
	}
}

func TestStripToolResultNoise(t *testing.T) {
	i18n.Init("en")

	msg := provider.Message{
		Role: "user",
		Content: []provider.ContentBlock{
			{Type: "tool_result", Text: "huge tool output\nline1\nline2\nline3"},
			{Type: "text", Text: "Please fix the bug"},
		},
	}

	got := stripToolResultNoise(msg)
	if strings.Contains(got, "huge tool output") {
		t.Errorf("tool_result should be stripped, got: %q", got)
	}
	if !strings.Contains(got, "Please fix the bug") {
		t.Errorf("text content should be kept, got: %q", got)
	}
}

func TestSimpleSummarize_FiltersNoise(t *testing.T) {
	i18n.Init("en")

	messages := []provider.Message{
		provider.UserMsg("Fix the bug in parser.go"),
		provider.AssistantText("Let me look at the file"),
		provider.UserMsg("[Context was compacted to save space. Your original task: Fix the bug in parser.go]"),
		provider.SystemMsg("[Performance mirror] Iter 10. Avg 4.5s/iter. Context: 70000/131072 tokens (53%)."),
		provider.SystemMsg("[Auto-compacted] 78%→17%. Continue your task."),
		provider.UserMsg("Continue. Use tools."),
		provider.UserMsg("Text-only reply, task not done. Original task: Fix the bug. Continue using tools."),
		provider.AssistantText("Done, fixed the parser bug"),
	}

	summary := SimpleSummarize(messages, 2000)

	// Noise must NOT appear in summary
	for _, noise := range []string{
		"Performance mirror",
		"Auto-compacted",
		"Context was compacted",
		"Text-only reply",
	} {
		if strings.Contains(summary, noise) {
			t.Errorf("Summary should not contain noise %q:\n%s", noise, summary)
		}
	}

	// Real content must be present
	if !strings.Contains(summary, "parser.go") {
		t.Errorf("Summary should contain real content:\n%s", summary)
	}
}

func TestExtractRecaps_DedupAndLimit(t *testing.T) {
	i18n.Init("en")

	// Build 10 assistant messages with recaps (7 unique + 3 duplicates)
	var messages []provider.Message
	unique := []string{
		"※ Recap: fixed bug one",
		"※ Recap: fixed bug two",
		"※ Recap: fixed bug three",
		"※ Recap: fixed bug four",
		"※ Recap: fixed bug five",
		"※ Recap: fixed bug six",
		"※ Recap: fixed bug seven",
	}
	for _, r := range unique {
		messages = append(messages, provider.AssistantText(r))
	}
	// Duplicates of earlier recaps
	messages = append(messages, provider.AssistantText("※ Recap: fixed bug one"))
	messages = append(messages, provider.AssistantText("※ Recap: fixed bug three"))
	messages = append(messages, provider.AssistantText("※ Recap: fixed bug five"))

	recaps := extractRecaps(messages)

	// Dedup: each unique recap should appear at most once
	counts := make(map[string]int)
	for _, r := range recaps {
		counts[r]++
	}
	for r, c := range counts {
		if c > 1 {
			t.Errorf("Recap %q appears %d times, want 1", r, c)
		}
	}

	// Limit: no more than 5 recaps
	if len(recaps) > 5 {
		t.Errorf("Expected at most 5 recaps, got %d: %v", len(recaps), recaps)
	}
}