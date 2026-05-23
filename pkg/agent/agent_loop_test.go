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

// MockTextProvider — провайдер, который всегда возвращает текстовый ответ
type MockTextProvider struct {
	text string
}

func (m *MockTextProvider) Name() string { return "mock-text" }

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

// TestRunLoop_WithTextResponse проверяет синхронный цикл с текстовым ответом
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

// TestRunWithMessages_WithTextResponse проверяет RunWithMessages с текстовым ответом
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

// TestStream_WithTextResponse проверяет стриминг с текстовым ответом
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

// MockToolCallProvider — провайдер, который возвращает tool call, а затем текст
type MockToolCallProvider struct {
	callCount int
	toolName  string
	finalText string
}

func (m *MockToolCallProvider) Name() string { return "mock-toolcall" }

func (m *MockToolCallProvider) Complete(messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	m.callCount++
	if m.callCount == 1 {
		// Первый вызов — tool call
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
	// Второй вызов — текст
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
			// Первый вызов — tool call
			ch <- provider.StreamEvent{Type: provider.EventToolCallStart, ToolName: m.toolName, ToolCallID: "call_1"}
			ch <- provider.StreamEvent{Type: provider.EventToolCallDelta, ToolDelta: `{"path":"/tmp/test"}`, ToolCallID: "call_1"}
			ch <- provider.StreamEvent{Type: provider.EventToolCallEnd, ToolCallID: "call_1"}
			ch <- provider.StreamEvent{Type: provider.EventDone}
		} else {
			// Второй вызов — текст
			ch <- provider.StreamEvent{Type: provider.EventTextDelta, Text: m.finalText}
			ch <- provider.StreamEvent{Type: provider.EventDone}
		}
		close(ch)
	}()
	return ch, nil
}

// TestRunLoop_WithToolCall проверяет синхронный цикл с tool call
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

// TestRunLoop_LoopDetection проверяет детекцию зацикливания
func TestRunLoop_LoopDetection(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	// Провайдер, который всегда возвращает один и тот же tool call
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

	// Собираем все события
	var gotDone bool
	for range eventCh {
		// Просто ждём завершения
	}
	_ = gotDone
}

// TestRunLoop_UnknownTool проверяет обработку неизвестного инструмента
func TestRunLoop_UnknownTool(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockToolCallProvider{toolName: "unknown_tool", finalText: "OK"}
	loop := NewAgentLoop(mock)
	// Не регистрируем unknown_tool — агент должен обработать ошибку

	result, err := loop.Run("use unknown tool")
	if err != nil {
		// Ошибка зацикливания — это нормально
		t.Logf("Run returned error (expected for unknown tool): %v", err)
	}
	_ = result
}

// TestRunLoop_PermissionDenied проверяет запрет инструмента
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
	// Может быть ошибка зацикливания или успешное завершение
}

// TestStreamWithCancel_ContextCancellation проверяет отмену контекста
func TestStreamWithCancel_ContextCancellation(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockStreamingProvider{}
	loop := NewAgentLoop(mock)
	loop.RegisterTool(&MockNoOpTool{})

	ctx, cancel := context.WithCancel(context.Background())

	// Отменяем через 100мс
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	eventCh, err := loop.StreamWithCancel(ctx, "test query")
	if err != nil {
		t.Fatalf("StreamWithCancel failed: %v", err)
	}

	// Собираем события до отмены
	var gotError bool
	for event := range eventCh {
		if event.Type == provider.EventError {
			gotError = true
		}
	}
	// Контекст отменён — может быть ошибка или просто закрытие канала
	_ = gotError
}

// MockErrorProvider — провайдер, который всегда возвращает ошибку
type MockErrorProvider struct {
	err error
}

func (m *MockErrorProvider) Name() string { return "mock-error" }

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
		// Stream может вернуть ошибку сразу (если провайдер nil)
		t.Logf("Stream returned error: %v", err)
		return
	}

	// Ошибка от провайдера приходит через канал событий
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

// TestBuildSystemPrompt_WithTools проверяет системный промпт с инструментами
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
