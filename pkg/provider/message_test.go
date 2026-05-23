package provider

import (
	"strings"
	"testing"
)

// --- GetThinking ---

func TestGetThinking_WithThinkingBlock(t *testing.T) {
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "thinking", Text: "размышляю о задаче"},
			{Type: "text", Text: "вот ответ"},
		},
	}
	got := msg.GetThinking()
	if got != "размышляю о задаче" {
		t.Errorf("GetThinking() = %q, want %q", got, "размышляю о задаче")
	}
}

func TestGetThinking_NoThinkingBlock(t *testing.T) {
	msg := UserMsg("привет")
	got := msg.GetThinking()
	if got != "" {
		t.Errorf("GetThinking() = %q, want empty", got)
	}
}

func TestGetThinking_MultipleThinkingBlocks(t *testing.T) {
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "thinking", Text: "шаг 1"},
			{Type: "text", Text: "промежуточный"},
			{Type: "thinking", Text: "шаг 2"},
		},
	}
	got := msg.GetThinking()
	if !strings.Contains(got, "шаг 1") || !strings.Contains(got, "шаг 2") {
		t.Errorf("GetThinking() = %q, want both steps", got)
	}
}

func TestGetThinking_EmptyContent(t *testing.T) {
	msg := Message{Role: "assistant"}
	got := msg.GetThinking()
	if got != "" {
		t.Errorf("GetThinking() = %q, want empty", got)
	}
}

// --- GetResponseText ---

func TestGetResponseText_WithThinking(t *testing.T) {
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "thinking", Text: "думаем"},
			{Type: "text", Text: "ответ"},
		},
	}
	got := msg.GetResponseText()
	if got != "ответ" {
		t.Errorf("GetResponseText() = %q, want %q", got, "ответ")
	}
}

func TestGetResponseText_OnlyThinking(t *testing.T) {
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "thinking", Text: "только думаю"},
		},
	}
	got := msg.GetResponseText()
	if got != "" {
		t.Errorf("GetResponseText() = %q, want empty", got)
	}
}

func TestGetResponseText_Fallback(t *testing.T) {
	msg := Message{Role: "assistant", Text: "fallback text"}
	got := msg.GetResponseText()
	if got != "fallback text" {
		t.Errorf("GetResponseText() = %q, want %q", got, "fallback text")
	}
}

// --- ToPlainText ---

func TestToPlainText_WithContent(t *testing.T) {
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "text", Text: "hello"},
		},
	}
	got := msg.ToPlainText()
	if got != "hello" {
		t.Errorf("ToPlainText() = %q, want %q", got, "hello")
	}
}

func TestToPlainText_Fallback(t *testing.T) {
	msg := Message{Role: "assistant", Text: "fallback"}
	got := msg.ToPlainText()
	if got != "fallback" {
		t.Errorf("ToPlainText() = %q, want %q", got, "fallback")
	}
}

func TestToPlainText_Empty(t *testing.T) {
	msg := Message{Role: "assistant"}
	got := msg.ToPlainText()
	if got != "" {
		t.Errorf("ToPlainText() = %q, want empty", got)
	}
}

// --- HasToolCalls edge cases ---

func TestHasToolCalls_WithToolResult(t *testing.T) {
	// tool_result — это не tool_use
	msg := ToolResultMsg("id1", "read", "content", false)
	if msg.HasToolCalls() {
		t.Error("tool_result should not be detected as tool_use")
	}
}

func TestHasToolCalls_EmptyContent(t *testing.T) {
	msg := Message{Role: "assistant"}
	if msg.HasToolCalls() {
		t.Error("empty content should not have tool calls")
	}
}

// --- GetText with tool_result (no tool name) ---

func TestGetText_ToolResultNoName(t *testing.T) {
	msg := Message{
		Role: "user",
		Content: []ContentBlock{
			{Type: "tool_result", Output: "some result", IsError: false},
		},
	}
	got := msg.GetText()
	if !strings.Contains(got, "[Result]") {
		t.Errorf("Expected [Result] in %q", got)
	}
}

// --- GetToolCalls mixed content ---

func TestGetToolCalls_MixedContent(t *testing.T) {
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "text", Text: "text"},
			{Type: "tool_use", ToolName: "read", ToolUseID: "id1"},
			{Type: "text", Text: "more text"},
			{Type: "tool_use", ToolName: "bash", ToolUseID: "id2"},
		},
	}
	calls := msg.GetToolCalls()
	if len(calls) != 2 {
		t.Fatalf("GetToolCalls() returned %d calls, want 2", len(calls))
	}
	if calls[0].ToolName != "read" {
		t.Errorf("calls[0].ToolName = %q, want 'read'", calls[0].ToolName)
	}
	if calls[1].ToolName != "bash" {
		t.Errorf("calls[1].ToolName = %q, want 'bash'", calls[1].ToolName)
	}
}
