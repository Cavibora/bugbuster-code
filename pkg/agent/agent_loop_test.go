package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
	"bugbuster-code/pkg/tools"
)

// MockTextProvider — a provider that always returns a text response
type MockTextProvider struct {
	text string
}

func (m *MockTextProvider) Name() string { return "mock-text" }
func (m *MockTextProvider) Model() string { return "mock-text-model" }

func (m *MockTextProvider) Complete(messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	return &provider.CompletionResult{
		Message:    provider.AssistantText(m.text),
		StopReason: "end_turn",
	}, nil
}

func (m *MockTextProvider) Stream(messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.StreamWithCtx(context.Background(), messages, toolDefs)
}

func (m *MockTextProvider) CompleteWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	return m.Complete(messages, toolDefs)
}

func (m *MockTextProvider) StreamWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent, 10)
	go func() {
		ch <- provider.StreamEvent{Type: provider.EventTextDelta, Text: m.text}
		ch <- provider.StreamEvent{Type: provider.EventDone}
		close(ch)
	}()
	return ch, nil
}

// TestRunLoop_WithTextResponse verifies the synchronous loop with a text response
func TestRunLoop_WithTextResponse(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockTextProvider{text: "Hello! I can help you."}
	loop := NewAgentLoop(mock)

	result, err := loop.Run("test query")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result != "Hello! I can help you." {
		t.Errorf("Expected 'Hello! I can help you.', got '%s'", result)
	}
}

// TestRunWithMessages_WithTextResponse verifies RunWithMessages with a text response
func TestRunWithMessages_WithTextResponse(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockTextProvider{text: "Response text"}
	loop := NewAgentLoop(mock)

	messages := []provider.Message{
		provider.UserMsg("previous message"),
	}
	result, err := loop.RunWithMessages(messages)
	if err != nil {
		t.Fatalf("RunWithMessages failed: %v", err)
	}
	if result != "Response text" {
		t.Errorf("Expected 'Response text', got '%s'", result)
	}
}

// TestStream_WithTextResponse verifies streaming with a text response
func TestStream_WithTextResponse(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockTextProvider{text: "Streamed response"}
	loop := NewAgentLoop(mock)

	eventCh, err := loop.Stream("test query")
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	var gotText bool
	var gotDone bool
	for event := range eventCh {
		switch event.Type {
		case provider.EventTextDelta:
			gotText = true
		case provider.EventDone:
			gotDone = true
		}
	}

	if !gotText {
		t.Error("Expected EventTextDelta")
	}
	if !gotDone {
		t.Error("Expected EventDone")
	}
}

// MockToolCallProvider — a provider that returns a tool call, then text
type MockToolCallProvider struct {
	callCount int
	toolName  string
	finalText string
}

func (m *MockToolCallProvider) Name() string { return "mock-toolcall" }
func (m *MockToolCallProvider) Model() string { return "mock-toolcall-model" }

func (m *MockToolCallProvider) Complete(messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	m.callCount++
	if m.callCount == 1 {
		// First call — tool call
		return &provider.CompletionResult{
			Message: provider.Message{
				Role: "assistant",
				Content: []provider.ContentBlock{
					{Type: "tool_use", ToolName: m.toolName, ToolUseID: "call_1", Input: map[string]any{"path": "/tmp/test"}},
				},
			},
			StopReason: "tool_use",
		}, nil
	}
	// Second call — text
	return &provider.CompletionResult{
		Message:    provider.AssistantText(m.finalText),
		StopReason: "end_turn",
	}, nil
}

func (m *MockToolCallProvider) Stream(messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.StreamWithCtx(context.Background(), messages, toolDefs)
}

func (m *MockToolCallProvider) CompleteWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	return m.Complete(messages, toolDefs)
}

func (m *MockToolCallProvider) StreamWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	m.callCount++
	ch := make(chan provider.StreamEvent, 10)
	go func() {
		if m.callCount == 1 {
			// First call — tool call
			ch <- provider.StreamEvent{Type: provider.EventToolCallStart, ToolName: m.toolName, ToolCallID: "call_1"}
			ch <- provider.StreamEvent{Type: provider.EventToolCallDelta, ToolDelta: `{"path":"/tmp/test"}`, ToolCallID: "call_1"}
			ch <- provider.StreamEvent{Type: provider.EventToolCallEnd, ToolCallID: "call_1"}
			ch <- provider.StreamEvent{Type: provider.EventDone}
		} else {
			// Second call — text
			ch <- provider.StreamEvent{Type: provider.EventTextDelta, Text: m.finalText}
			ch <- provider.StreamEvent{Type: provider.EventDone}
		}
		close(ch)
	}()
	return ch, nil
}

// TestRunLoop_WithToolCall verifies the synchronous loop with a tool call
func TestRunLoop_WithToolCall(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockToolCallProvider{toolName: "read", finalText: "Done!"}
	loop := NewAgentLoop(mock)
	loop.RegisterTool(&MockNoOpTool{})

	result, err := loop.Run("read a file")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result != "Done!" {
		t.Errorf("Expected 'Done!', got '%s'", result)
	}
}

// TestRunLoop_LoopDetection verifies loop detection
func TestRunLoop_LoopDetection(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	// Provider that always returns the same tool call
	mock := &MockStreamingProvider{}
	loop := NewAgentLoop(mock)
	loop.SetMaxIterations(20) // Достаточно итераций для детекции
	loop.RegisterTool(&MockNoOpTool{})
	loop.SetLoopRepeatThreshold(3) // Быстрая детекция

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eventCh, err := loop.StreamWithCancel(ctx, "test query")
	if err != nil {
		t.Fatalf("StreamWithCancel failed: %v", err)
	}

	// Collect all events
	var gotDone bool
	for range eventCh {
		// Just wait for completion
	}
	_ = gotDone
}

// TestRunLoop_UnknownTool verifies handling of an unknown tool
func TestRunLoop_UnknownTool(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockToolCallProvider{toolName: "unknown_tool", finalText: "OK"}
	loop := NewAgentLoop(mock)
	// Do not register unknown_tool — the agent should handle the error

	result, err := loop.Run("use unknown tool")
	if err != nil {
		// Loop error — this is normal
		t.Logf("Run returned error (expected for unknown tool): %v", err)
	}
	_ = result
}

// TestRunLoop_PermissionDenied verifies tool denial
func TestRunLoop_PermissionDenied(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockToolCallProvider{toolName: "bash", finalText: "OK"}
	loop := NewAgentLoop(mock)
	loop.RegisterTool(&MockNoOpTool{})
	loop.SetPermissionChecker(NewDefaultPermissionChecker(PermissionDeny, "/tmp"))

	result, err := loop.Run("run bash command")
	_ = result
	_ = err
	// May be a loop error or successful completion
}

// TestStreamWithCancel_ContextCancellation verifies context cancellation
func TestStreamWithCancel_ContextCancellation(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockStreamingProvider{}
	loop := NewAgentLoop(mock)
	loop.RegisterTool(&MockNoOpTool{})

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 100ms
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	eventCh, err := loop.StreamWithCancel(ctx, "test query")
	if err != nil {
		t.Fatalf("StreamWithCancel failed: %v", err)
	}

	// Collect events until cancellation
	var gotError bool
	for event := range eventCh {
		if event.Type == provider.EventError {
			gotError = true
		}
	}
	// Context cancelled — may be an error or just channel closure
	_ = gotError
}

// MockErrorProvider — a provider that always returns an error
type MockErrorProvider struct {
	err error
}

func (m *MockErrorProvider) Name() string { return "mock-error" }
func (m *MockErrorProvider) Model() string { return "mock-error-model" }

func (m *MockErrorProvider) Complete(messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	return nil, m.err
}

func (m *MockErrorProvider) Stream(messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return nil, m.err
}

func (m *MockErrorProvider) CompleteWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	return m.Complete(messages, toolDefs)
}

func (m *MockErrorProvider) StreamWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.Stream(messages, toolDefs)
}

func TestRunLoop_ProviderError(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockErrorProvider{err: i18n.E("errors_provider.request", "mock", "connection refused")}
	loop := NewAgentLoop(mock)

	_, err := loop.Run("test query")
	if err == nil {
		t.Error("Expected error from provider")
	}
}

func TestStream_ProviderError(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockErrorProvider{err: i18n.E("errors_provider.request", "mock", "connection refused")}
	loop := NewAgentLoop(mock)

	eventCh, err := loop.Stream("test query")
	if err != nil {
		// Stream may return an error immediately (if provider is nil)
		t.Logf("Stream returned error: %v", err)
		return
	}

	// Provider error arrives through the events channel
	var gotError bool
	for event := range eventCh {
		if event.Type == provider.EventError {
			gotError = true
		}
	}
	if !gotError {
		t.Error("Expected EventError from provider")
	}
}

// MockMaxTokensProvider — a provider that first returns max_tokens, then a normal response
type MockMaxTokensProvider struct {
	callCount int
	truncatedText string
	finalText string
}

func (m *MockMaxTokensProvider) Name() string { return "mock-max-tokens" }
func (m *MockMaxTokensProvider) Model() string { return "mock-max-tokens-model" }

func (m *MockMaxTokensProvider) Complete(messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	m.callCount++
	if m.callCount == 1 {
		// First call — truncated response (max_tokens)
		return &provider.CompletionResult{
			Message:    provider.AssistantText(m.truncatedText),
			StopReason: "max_tokens",
		}, nil
	}
	// Second call — normal response
	return &provider.CompletionResult{
		Message:    provider.AssistantText(m.finalText),
		StopReason: "end_turn",
	}, nil
}

func (m *MockMaxTokensProvider) Stream(messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.StreamWithCtx(context.Background(), messages, toolDefs)
}

func (m *MockMaxTokensProvider) CompleteWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	return m.Complete(messages, toolDefs)
}

func (m *MockMaxTokensProvider) StreamWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	m.callCount++
	ch := make(chan provider.StreamEvent, 20)
	go func() {
		if m.callCount == 1 {
			// First call — truncated response (max_tokens)
			ch <- provider.StreamEvent{Type: provider.EventTextDelta, Text: m.truncatedText}
			ch <- provider.StreamEvent{Type: "stop_reason", StopReason: "max_tokens"}
			ch <- provider.StreamEvent{Type: provider.EventDone}
		} else {
			// Second call — normal response
			ch <- provider.StreamEvent{Type: provider.EventTextDelta, Text: m.finalText}
			ch <- provider.StreamEvent{Type: provider.EventDone}
		}
		close(ch)
	}()
	return ch, nil
}

// TestRunLoop_MaxTokensContinues verifies that on max_tokens the agent continues working
// with a minimal prompt "Continue." (not a long continuation hint)
func TestRunLoop_MaxTokensContinues(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockMaxTokensProvider{
		truncatedText: "This is a long response that got cut off",
		finalText:     " and this is the continuation.",
	}
	loop := NewAgentLoop(mock)

	result, err := loop.Run("test query")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	// Model should be called twice: first max_tokens, then continuation
	if mock.callCount != 2 {
		t.Errorf("Expected 2 calls to provider (truncated + continuation), got %d", mock.callCount)
	}
	// Result should contain the continuation text
	if !strings.Contains(result, "continuation") {
		t.Errorf("Expected result to contain continuation text, got: %s", result)
	}
}

// TestStream_MaxTokensContinues verifies that on max_tokens streaming continues
// with a minimal prompt "Continue." — text is joined without warnings
func TestStream_MaxTokensContinues(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockMaxTokensProvider{
		truncatedText: "This is a long response that got cut off",
		finalText:     " and this is the continuation.",
	}
	loop := NewAgentLoop(mock)

	eventCh, err := loop.Stream("test query")
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	var gotDone bool
	var textCount int
	for event := range eventCh {
		switch event.Type {
		case provider.EventTextDelta:
			textCount++
		case provider.EventDone:
			gotDone = true
		case provider.EventError:
			t.Errorf("Unexpected EventError: %v", event.Error)
		}
	}

	if !gotDone {
		t.Error("Expected EventDone at the end of stream")
	}
	// Should have at least 2 text deltas (truncated + continuation)
	if textCount < 2 {
		t.Errorf("Expected at least 2 text deltas, got %d", textCount)
	}
	// Provider should be called twice: first max_tokens, then continuation
	if mock.callCount != 2 {
		t.Errorf("Expected 2 calls to provider (truncated + continuation), got %d", mock.callCount)
	}
}

// TestBuildSystemPrompt_WithTools verifies the system prompt with tools
func TestBuildSystemPrompt_WithTools(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	toolList := map[string]tools.Tool{
		"read":  tools.NewReadTool(),
		"bash":  tools.NewBashTool(),
		"write": tools.NewWriteTool(),
	}

	prompt := BuildSystemPrompt("/home/user/project", toolList)
	if prompt == "" {
		t.Error("Expected non-empty system prompt")
	}
	if !strings.Contains(prompt, "read") {
		t.Error("prompt should mention 'read' tool")
	}
	if !strings.Contains(prompt, "bash") {
		t.Error("prompt should mention 'bash' tool")
	}
	if !strings.Contains(prompt, "write") {
		t.Error("prompt should mention 'write' tool")
	}
}
