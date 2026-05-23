package agent

import (
	"context"
	"testing"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
	"bugbuster-code/pkg/tools"
)

// mockTool is a simple mock tool for testing
type mockTool struct {
	name   string
	result tools.ToolResult
}

func (m *mockTool) Name() string                                      { return m.name }
func (m *mockTool) Description() string                               { return "mock tool" }
func (m *mockTool) Parameters() map[string]any                        { return nil }
func (m *mockTool) Execute(params map[string]string) tools.ToolResult { return m.result }

// TestHandleStreamToolCalls_UnknownTool tests handling of unknown tool
func TestHandleStreamToolCalls_UnknownTool(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	loop := NewAgentLoop(&MockProvider{})
	loop.Tools = map[string]tools.Tool{}

	eventCh := make(chan provider.StreamEvent, 100)

	toolCalls := []provider.ContentBlock{
		{
			Type:      "tool_use",
			ToolName:  "unknown_tool",
			ToolUseID: "tool-1",
			Input:     map[string]any{"arg": "value"},
		},
	}

	continueLoop := loop.handleStreamToolCalls(context.Background(), toolCalls, eventCh)
	close(eventCh)

	// Unknown tool should not continue loop
	if continueLoop {
		t.Error("handleStreamToolCalls should return false for unknown tool")
	}

	// Should have error message in events
	var foundError bool
	for evt := range eventCh {
		if evt.Type == provider.EventTextDelta && containsStr(evt.Text, "ERROR") {
			foundError = true
		}
	}
	if !foundError {
		t.Error("should have error event for unknown tool")
	}
}

// TestHandleStreamToolCalls_ParseError tests handling of parse error in tool input
func TestHandleStreamToolCalls_ParseError(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	loop := NewAgentLoop(&MockProvider{})
	loop.Tools = map[string]tools.Tool{
		"bash": &mockTool{name: "bash", result: tools.ToolResult{Output: "hello"}},
	}

	eventCh := make(chan provider.StreamEvent, 100)

	toolCalls := []provider.ContentBlock{
		{
			Type:      "tool_use",
			ToolName:  "bash",
			ToolUseID: "tool-1",
			Input: map[string]any{
				"_parse_error": "invalid JSON",
				"_raw_input":   "{bad json}",
			},
		},
	}

	continueLoop := loop.handleStreamToolCalls(context.Background(), toolCalls, eventCh)
	close(eventCh)

	if continueLoop {
		t.Error("handleStreamToolCalls should return false for parse error")
	}

	var foundError bool
	for evt := range eventCh {
		if evt.Type == provider.EventTextDelta && containsStr(evt.Text, "ERROR") {
			foundError = true
		}
	}
	if !foundError {
		t.Error("should have error event for parse error")
	}
}

// TestHandleStreamToolCalls_PermissionDenied tests handling of denied permission
func TestHandleStreamToolCalls_PermissionDenied(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	loop := NewAgentLoop(&MockProvider{})
	loop.Tools = map[string]tools.Tool{
		"bash": &mockTool{name: "bash", result: tools.ToolResult{Output: "hello"}},
	}
	loop.SetPermissionChecker(&denyAllChecker{})

	eventCh := make(chan provider.StreamEvent, 100)

	toolCalls := []provider.ContentBlock{
		{
			Type:      "tool_use",
			ToolName:  "bash",
			ToolUseID: "tool-1",
			Input:     map[string]any{"command": "echo hello"},
		},
	}

	continueLoop := loop.handleStreamToolCalls(context.Background(), toolCalls, eventCh)
	close(eventCh)

	if continueLoop {
		t.Error("handleStreamToolCalls should return false for denied permission")
	}

	var foundForbidden bool
	for evt := range eventCh {
		if evt.Type == provider.EventTextDelta && containsStr(evt.Text, "FORBIDDEN") {
			foundForbidden = true
		}
	}
	if !foundForbidden {
		t.Error("should have FORBIDDEN event for denied permission")
	}
}

// TestHandleStreamToolCalls_LoopDetected tests loop detection during tool execution
func TestHandleStreamToolCalls_LoopDetected(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	loop := NewAgentLoop(&MockProvider{})
	loop.Tools = map[string]tools.Tool{
		"bash": &mockTool{name: "bash", result: tools.ToolResult{Output: "hello"}},
	}
	loop.SetLoopRepeatThreshold(2)
	loop.SetLoopToolRepeatThreshold(2)
	loop.SetLoopWindowSize(5)

	// First call — should not detect loop
	toolCalls := []provider.ContentBlock{
		{
			Type:      "tool_use",
			ToolName:  "bash",
			ToolUseID: "tool-1",
			Input:     map[string]any{"command": "echo hello"},
		},
	}

	eventCh1 := make(chan provider.StreamEvent, 100)
	continueLoop := loop.handleStreamToolCalls(context.Background(), toolCalls, eventCh1)
	close(eventCh1)

	if continueLoop {
		t.Error("first call should not detect loop")
	}

	// Second call — same tool, should detect loop
	eventCh2 := make(chan provider.StreamEvent, 100)
	continueLoop = loop.handleStreamToolCalls(context.Background(), toolCalls, eventCh2)
	close(eventCh2)

	if !continueLoop {
		t.Error("second call should detect loop")
	}
}

// TestHandleStreamToolCalls_CancelledContext tests context cancellation
func TestHandleStreamToolCalls_CancelledContext(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	loop := NewAgentLoop(&MockProvider{})
	loop.Tools = map[string]tools.Tool{
		"bash": &mockTool{name: "bash", result: tools.ToolResult{Output: "hello"}},
	}

	eventCh := make(chan provider.StreamEvent, 100)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	toolCalls := []provider.ContentBlock{
		{
			Type:      "tool_use",
			ToolName:  "bash",
			ToolUseID: "tool-1",
			Input:     map[string]any{"command": "echo hello"},
		},
	}

	continueLoop := loop.handleStreamToolCalls(ctx, toolCalls, eventCh)
	close(eventCh)

	if continueLoop {
		t.Error("handleStreamToolCalls should return false for cancelled context")
	}

	var foundError bool
	for evt := range eventCh {
		if evt.Type == provider.EventError {
			foundError = true
		}
	}
	if !foundError {
		t.Error("should have error event for cancelled context")
	}
}

// TestMaybeCompact tests context compaction trigger
func TestMaybeCompact(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	loop := NewAgentLoop(&MockProvider{})
	loop.SetMaxTokens(100) // Very low limit to trigger compaction

	// Add many messages to exceed token limit
	for i := 0; i < 50; i++ {
		loop.Context.Messages = append(loop.Context.Messages, provider.UserMsg("This is a test message that adds tokens to the context"))
	}

	eventCh := make(chan provider.StreamEvent, 100)
	loop.maybeCompact(eventCh, context.Background())
	close(eventCh)

	// Compaction should have been triggered
	if len(loop.Context.Messages) >= 50 {
		t.Error("maybeCompact should have reduced messages")
	}
}

// TestMaybeCompact_NoCompactionNeeded tests that compaction is not triggered when not needed
func TestMaybeCompact_NoCompactionNeeded(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	loop := NewAgentLoop(&MockProvider{})
	loop.SetMaxTokens(100000) // Very high limit

	// Add few messages
	loop.Context.Messages = append(loop.Context.Messages, provider.UserMsg("Hello"))

	eventCh := make(chan provider.StreamEvent, 100)
	loop.maybeCompact(eventCh, context.Background())
	close(eventCh)

	// Messages should not be reduced
	if len(loop.Context.Messages) != 1 {
		t.Errorf("maybeCompact should not reduce messages, got %d", len(loop.Context.Messages))
	}
}

// TestHandleStreamFinalResponse tests final response handling
func TestHandleStreamFinalResponse(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	loop := NewAgentLoop(&MockProvider{})

	eventCh := make(chan provider.StreamEvent, 100)

	continueLoop, err := loop.handleStreamFinalResponse(
		"Hello, I'm done!",
		1,
		time.Now(),
		time.Now(),
		eventCh,
	)
	close(eventCh)

	if err != nil {
		t.Errorf("handleStreamFinalResponse error: %v", err)
	}
	if continueLoop {
		t.Error("handleStreamFinalResponse should return false for final response")
	}
}

// TestHandleStreamFinalResponse_LoopDetected tests loop detection in final response
func TestHandleStreamFinalResponse_LoopDetected(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	loop := NewAgentLoop(&MockProvider{})
	loop.SetLoopRepeatThreshold(2)
	loop.SetLoopWindowSize(5)

	// First call — should not detect loop
	eventCh1 := make(chan provider.StreamEvent, 100)
	_, _ = loop.handleStreamFinalResponse("I'll help you", 1, time.Now(), time.Now(), eventCh1)
	close(eventCh1)

	// Second call with same text — should detect loop
	eventCh2 := make(chan provider.StreamEvent, 100)
	continueLoop, _ := loop.handleStreamFinalResponse("I'll help you", 2, time.Now(), time.Now(), eventCh2)
	close(eventCh2)

	if !continueLoop {
		t.Error("second call should detect loop")
	}
}

// denyAllChecker denies all permissions
type denyAllChecker struct{}

func (d *denyAllChecker) CheckPermission(req PermissionRequest) PermissionResult {
	return PermDenied
}
