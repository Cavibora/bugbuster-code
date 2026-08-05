package agent

import (
	"testing"

	"bugbuster-code/pkg/provider"
)

func TestDeduplicateRepeats(t *testing.T) {
	tests := []struct {
		name     string
		messages []provider.Message
		want     int // expected number of messages after dedup
	}{
		{
			name: "no repeats",
			messages: []provider.Message{
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Hello"}}},
				{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Hi there"}}},
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "How are you?"}}},
			},
			want: 3,
		},
		{
			name: "5 identical Continue messages",
			messages: []provider.Message{
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Continue."}}},
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Continue."}}},
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Continue."}}},
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Continue."}}},
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Continue."}}},
			},
			want: 1,
		},
		{
			name: "mixed: 3 Continue + assistant + 2 Continue",
			messages: []provider.Message{
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Continue."}}},
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Continue."}}},
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Continue."}}},
				{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Working..."}}},
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Continue."}}},
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Continue."}}},
			},
			want: 3, // "Continue. (×3)" + "Working..." + "Continue. (×2)"
		},
		{
			name: "long user message not deduplicated",
			messages: []provider.Message{
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "This is a very long user message that exceeds the 100 character limit and should not be deduplicated because it contains important context that the user typed manually"}}},
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "This is a very long user message that exceeds the 100 character limit and should not be deduplicated because it contains important context that the user typed manually"}}},
			},
			want: 2,
		},
		{
			name: "different short messages not deduplicated",
			messages: []provider.Message{
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Continue."}}},
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Go on"}}},
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Keep going"}}},
			},
			want: 3,
		},
		{
			name: "empty slice",
			messages: []provider.Message{},
			want:     0,
		},
		{
			name: "single message",
			messages: []provider.Message{
				{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Hello"}}},
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeduplicateRepeats(tt.messages)
			if len(result) != tt.want {
				t.Errorf("DeduplicateRepeats returned %d messages, want %d", len(result), tt.want)
				for i, m := range result {
					t.Logf("  [%d] %s: %q", i, m.Role, m.GetResponseText())
				}
			}
		})
	}
}

func TestDeduplicateRepeats_AnnotatedText(t *testing.T) {
	messages := []provider.Message{
		{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Continue."}}},
		{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Continue."}}},
		{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "Continue."}}},
	}

	result := DeduplicateRepeats(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	text := result[0].GetResponseText()
	expected := "Continue. (×3)"
	if text != expected {
		t.Errorf("expected %q, got %q", expected, text)
	}
}