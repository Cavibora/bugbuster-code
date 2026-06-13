package agent

import (
	"context"
	"strings"
	"testing"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

// TestCompactContext_SingleOtherMessage verifies that compaction does not panic
// when there is only 1 non-system message (keepRecent=2 but len(otherMsgs)=1).
func TestCompactContext_SingleOtherMessage(t *testing.T) {
	i18n.Init("en")

	messages := []provider.Message{
		provider.SystemMsg("You are a helpful assistant."),
		provider.UserMsg("This is a very long message that should trigger compaction because it exceeds the token limit. " + strings.Repeat("padding ", 100)),
	}

	// This should NOT panic
	result := CompactContext(messages, 10, 5)

	if len(result) == 0 {
		t.Error("Expected non-empty result")
	}
}

// TestCompactContextWithCompactor_SingleOtherMessage verifies the same for CompactContextWithCompactor.
func TestCompactContextWithCompactor_SingleOtherMessage(t *testing.T) {
	i18n.Init("en")

	messages := []provider.Message{
		provider.SystemMsg("You are a helpful assistant."),
		provider.UserMsg("This is a very long message that should trigger compaction because it exceeds the token limit. " + strings.Repeat("padding ", 100)),
	}

	mc := &mockCompactor{summarizeResult: "Summary"}

	// This should NOT panic
	result := CompactContextWithCompactor(messages, 10, 5, mc, context.Background())

	if len(result) == 0 {
		t.Error("Expected non-empty result")
	}
}

// TestCompactContext_TwoOtherMessages verifies compaction with exactly 2 non-system messages.
func TestCompactContext_TwoOtherMessages(t *testing.T) {
	i18n.Init("en")

	messages := []provider.Message{
		provider.SystemMsg("You are a helpful assistant."),
		provider.UserMsg("Short question"),
		provider.AssistantText("Short answer"),
	}

	result := CompactContext(messages, 10, 5)

	if len(result) == 0 {
		t.Error("Expected non-empty result")
	}
}

// TestCompactContext_ZeroOtherMessages verifies compaction with only system messages.
func TestCompactContext_ZeroOtherMessages(t *testing.T) {
	i18n.Init("en")

	messages := []provider.Message{
		provider.SystemMsg("You are a helpful assistant. " + strings.Repeat("padding ", 100)),
	}

	result := CompactContext(messages, 10, 5)

	// Should return system messages only
	if len(result) == 0 {
		t.Error("Expected non-empty result")
	}
}
