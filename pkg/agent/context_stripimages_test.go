package agent

import (
	"testing"

	"bugbuster-code/pkg/provider"
)

func countImageBlocks(msgs []provider.Message) int {
	n := 0
	for _, m := range msgs {
		for _, b := range m.Content {
			if b.Type == "image" {
				n++
			}
		}
	}
	return n
}

func TestStripImagesRemovesImageBlocks(t *testing.T) {
	c := NewConversationContextWithTokens(8000, 6)
	c.Add(provider.UserMsg("hello"))
	c.Add(provider.UserImageTextMsg("[Screenshot]", "AAAA", "png"))
	c.Add(provider.AssistantText("I see a red square"))

	if !c.HasImages() {
		t.Fatal("expected HasImages() == true before strip")
	}

	removed := c.StripImages()
	if removed != 1 {
		t.Fatalf("expected 1 image removed, got %d", removed)
	}
	if c.HasImages() {
		t.Error("expected HasImages() == false after strip")
	}
	if countImageBlocks(c.Messages) != 0 {
		t.Error("image blocks still present after strip")
	}

	// Text must survive: the image-bearing message keeps its text part.
	var texts []string
	for _, m := range c.Messages {
		texts = append(texts, m.GetText())
	}
	joined := ""
	for _, tx := range texts {
		joined += tx + "|"
	}
	if !containsStr(joined, "[Screenshot]") {
		t.Errorf("expected screenshot caption text preserved, got %q", joined)
	}
	if !containsStr(joined, "hello") || !containsStr(joined, "red square") {
		t.Errorf("non-image messages must be preserved, got %q", joined)
	}
}

func TestStripImagesDropsImageOnlyMessage(t *testing.T) {
	c := NewConversationContextWithTokens(8000, 6)
	c.Add(provider.UserMsg("keep me"))
	c.Add(provider.UserImageMsg("AAAA", "png")) // image only, no text

	removed := c.StripImages()
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}
	if len(c.Messages) != 1 {
		t.Fatalf("image-only message should be dropped, got %d messages", len(c.Messages))
	}
	if c.Messages[0].GetText() != "keep me" {
		t.Errorf("wrong surviving message: %q", c.Messages[0].GetText())
	}
}

func TestStripImagesNoImagesIsNoop(t *testing.T) {
	c := NewConversationContextWithTokens(8000, 6)
	c.Add(provider.UserMsg("plain"))
	c.Add(provider.AssistantText("reply"))

	if c.HasImages() {
		t.Fatal("no images expected")
	}
	if removed := c.StripImages(); removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}
	if len(c.Messages) != 2 {
		t.Errorf("messages must be untouched, got %d", len(c.Messages))
	}
}

// A tool_result message that bundles tool_result + text + image (the exact
// shape the screenshot tool produces via direct append) must keep the
// tool_result and text but lose the image, so the OpenAI tool-call chain
// stays valid. Added directly to Messages (not via Add) to mirror production.
func TestStripImagesPreservesToolResultChain(t *testing.T) {
	c := NewConversationContextWithTokens(8000, 6)
	c.Messages = append(c.Messages, provider.Message{
		Role: "user",
		Content: []provider.ContentBlock{
			{Type: "tool_result", ToolUseID: "tc1", Output: "screenshot taken"},
			{Type: "text", Text: "result text"},
			{Type: "image", ImageSource: "AAAA", ImageFormat: "png"},
		},
	})

	removed := c.StripImages()
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}
	if len(c.Messages) != 1 {
		t.Fatalf("message should survive (has tool_result), got %d", len(c.Messages))
	}
	var hasToolResult, hasText, hasImage bool
	for _, b := range c.Messages[0].Content {
		switch b.Type {
		case "tool_result":
			hasToolResult = true
		case "text":
			hasText = true
		case "image":
			hasImage = true
		}
	}
	if !hasToolResult {
		t.Error("tool_result block must be preserved")
	}
	if !hasText {
		t.Error("text block must be preserved")
	}
	if hasImage {
		t.Error("image block must be removed")
	}
}

