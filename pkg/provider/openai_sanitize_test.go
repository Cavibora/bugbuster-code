package provider

import (
	"testing"
)

// TestSanitizeToolMessages_OrphanToolBecomesUser verifies that a "tool" message
// without a preceding assistant message with matching tool_calls is converted
// to a user text message (preventing HTTP 400 from OpenAI-compatible APIs).
func TestSanitizeToolMessages_OrphanToolBecomesUser(t *testing.T) {
	msgs := []map[string]any{
		{"role": "system", "content": "sys"},
		{"role": "user", "content": "do the task"},
		// Orphan tool message — no preceding assistant with tool_calls
		{"role": "tool", "tool_call_id": "call_orphan", "content": "some output"},
	}

	result := sanitizeToolMessages(msgs)

	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	// The orphan tool message must be converted to a user message
	last := result[2]
	if role, _ := last["role"].(string); role != "user" {
		t.Errorf("orphan tool message should become user, got role=%q", role)
	}
	content, _ := last["content"].(string)
	if content != "[Tool output]\nsome output" {
		t.Errorf("unexpected content: %q", content)
	}
	if _, hasID := last["tool_call_id"]; hasID {
		t.Errorf("converted message must not have tool_call_id")
	}
}

// TestSanitizeToolMessages_ValidPairKept verifies that a valid
// assistant(tool_calls) -> tool sequence is left untouched.
func TestSanitizeToolMessages_ValidPairKept(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "do the task"},
		{
			"role": "assistant",
			"tool_calls": []map[string]any{
				{"id": "call_1", "type": "function", "function": map[string]any{"name": "read", "arguments": "{}"}},
			},
		},
		{"role": "tool", "tool_call_id": "call_1", "content": "file contents"},
	}

	result := sanitizeToolMessages(msgs)

	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if role, _ := result[2]["role"].(string); role != "tool" {
		t.Errorf("valid tool message should stay tool, got role=%q", role)
	}
}

// TestSanitizeToolMessages_InterveningMessageBreaksPair verifies that a tool
// message is treated as orphan if another message appears between the
// assistant tool_calls and the tool result.
func TestSanitizeToolMessages_InterveningMessageBreaksPair(t *testing.T) {
	msgs := []map[string]any{
		{
			"role": "assistant",
			"tool_calls": []map[string]any{
				{"id": "call_1", "type": "function", "function": map[string]any{"name": "read", "arguments": "{}"}},
			},
		},
		// Intervening user message — breaks the immediate assistant→tool sequence
		{"role": "user", "content": "wait"},
		{"role": "tool", "tool_call_id": "call_1", "content": "file contents"},
	}

	result := sanitizeToolMessages(msgs)

	// The tool message must be converted to user (orphan)
	last := result[2]
	if role, _ := last["role"].(string); role != "user" {
		t.Errorf("tool after intervening message should become user, got role=%q", role)
	}
}

// TestSanitizeToolMessages_MultipleToolCalls verifies that multiple tool results
// following a single assistant message with multiple tool_calls are all kept.
func TestSanitizeToolMessages_MultipleToolCalls(t *testing.T) {
	msgs := []map[string]any{
		{
			"role": "assistant",
			"tool_calls": []map[string]any{
				{"id": "call_1", "type": "function", "function": map[string]any{"name": "read", "arguments": "{}"}},
				{"id": "call_2", "type": "function", "function": map[string]any{"name": "bash", "arguments": "{}"}},
			},
		},
		{"role": "tool", "tool_call_id": "call_1", "content": "out1"},
		{"role": "tool", "tool_call_id": "call_2", "content": "out2"},
	}

	result := sanitizeToolMessages(msgs)

	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	for i, m := range result[1:] {
		if role, _ := m["role"].(string); role != "tool" {
			t.Errorf("message %d should stay tool, got role=%q", i+1, role)
		}
	}
}

// TestSanitizeToolMessages_EmptyOrphanDropped verifies that an orphan tool
// message with empty content is dropped entirely.
func TestSanitizeToolMessages_EmptyOrphanDropped(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "task"},
		{"role": "tool", "tool_call_id": "call_x", "content": "   "},
	}

	result := sanitizeToolMessages(msgs)

	if len(result) != 1 {
		t.Fatalf("expected 1 message (empty orphan dropped), got %d", len(result))
	}
}

// TestSanitizeToolMessages_Empty verifies no panic on empty input.
func TestSanitizeToolMessages_Empty(t *testing.T) {
	result := sanitizeToolMessages(nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

// TestSanitizeToolMessages_UnansweredToolCallsStripped verifies that an
// assistant message with tool_calls not followed by tool responses has its
// tool_calls removed (prevents "tool_calls must be followed by tool messages").
func TestSanitizeToolMessages_UnansweredToolCallsStripped(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "task"},
		{
			"role": "assistant",
			"tool_calls": []map[string]any{
				{"id": "call_1", "type": "function", "function": map[string]any{"name": "read", "arguments": "{}"}},
			},
		},
		// No tool response follows — tool_calls must be stripped
		{"role": "user", "content": "next"},
	}

	result := sanitizeToolMessages(msgs)

	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if _, hasTC := result[1]["tool_calls"]; hasTC {
		t.Errorf("unanswered tool_calls should be stripped from assistant message")
	}
}

// TestSanitizeToolMessages_PartialToolCallsStripped verifies that only the
// unanswered tool_calls are removed when some are answered.
func TestSanitizeToolMessages_PartialToolCallsStripped(t *testing.T) {
	msgs := []map[string]any{
		{
			"role": "assistant",
			"tool_calls": []map[string]any{
				{"id": "call_1", "type": "function", "function": map[string]any{"name": "read", "arguments": "{}"}},
				{"id": "call_2", "type": "function", "function": map[string]any{"name": "bash", "arguments": "{}"}},
			},
		},
		{"role": "tool", "tool_call_id": "call_1", "content": "out1"},
		// call_2 unanswered
		{"role": "user", "content": "next"},
	}

	result := sanitizeToolMessages(msgs)

	tcs, ok := result[0]["tool_calls"].([]map[string]any)
	if !ok {
		t.Fatalf("assistant message should still have tool_calls")
	}
	if len(tcs) != 1 {
		t.Fatalf("expected 1 remaining tool_call, got %d", len(tcs))
	}
	if id, _ := tcs[0]["id"].(string); id != "call_1" {
		t.Errorf("expected call_1 to remain, got %q", id)
	}
}
