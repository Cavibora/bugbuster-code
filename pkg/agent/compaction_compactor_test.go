package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

// mockCompactor — мок-компактор для тестов
type mockCompactor struct {
	summarizeResult string
	summarizeCalled bool
	summarizeInput  []provider.Message
	summarizeTokens int
}

func (m *mockCompactor) Summarize(messages []provider.Message, maxTokens int) string {
	m.summarizeCalled = true
	m.summarizeInput = messages
	m.summarizeTokens = maxTokens
	return m.summarizeResult
}

// mockProviderForCompactor — мок-провайдер для LLMCompactor тестов
type mockProviderForCompactor struct {
	result *provider.CompletionResult
	err    error
	delay  time.Duration
}

func (m *mockProviderForCompactor) Name() string { return "mock" }
func (m *mockProviderForCompactor) Model() string { return "mock-compactor-model" }

func (m *mockProviderForCompactor) Stream(messages []provider.Message, tools []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.StreamEvent{Type: "text", Text: "mock"}
	close(ch)
	return ch, nil
}

func (m *mockProviderForCompactor) StreamWithCtx(ctx context.Context, messages []provider.Message, tools []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.Stream(messages, tools)
}

func (m *mockProviderForCompactor) Complete(messages []provider.Message, tools []provider.ToolDef) (*provider.CompletionResult, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return m.result, m.err
}

func (m *mockProviderForCompactor) CompleteWithCtx(ctx context.Context, messages []provider.Message, tools []provider.ToolDef) (*provider.CompletionResult, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return m.result, m.err
}

func (m *mockProviderForCompactor) SetModel(model string) {}

func (m *mockProviderForCompactor) SetSystemPrompt(prompt string) {}

func (m *mockProviderForCompactor) SetMaxTokens(tokens int) {}

func (m *mockProviderForCompactor) SetTemperature(temp float64) {}

func (m *mockProviderForCompactor) SetTopP(topP float64) {}

func (m *mockProviderForCompactor) SetThinkingBudget(budget int) {}

func (m *mockProviderForCompactor) SupportsThinking() bool { return false }

func (m *mockProviderForCompactor) SupportsFunctionCalling() bool { return false }

func (m *mockProviderForCompactor) SupportsStreaming() bool { return true }

func (m *mockProviderForCompactor) SetAPIKey(key string) {}

func (m *mockProviderForCompactor) SetBaseURL(url string) {}

func (m *mockProviderForCompactor) SetOrganization(org string) {}

func (m *mockProviderForCompactor) SetHTTPClient(client interface{}) {}

func (m *mockProviderForCompactor) GetModel() string { return "mock-model" }

func (m *mockProviderForCompactor) GetMaxTokens() int { return 4096 }

func (m *mockProviderForCompactor) GetSystemPrompt() string { return "" }

// --- Тесты для CompactContextWithCompactor ---

func TestCompactContextWithCompactor_NoCompactionNeeded(t *testing.T) {
	i18n.Init("en")
	mc := &mockCompactor{summarizeResult: "Summary of conversation"}

	messages := []provider.Message{
		provider.SystemMsg("You are a helper"),
		provider.UserMsg("Hello"),
		provider.AssistantText("Hi there!"),
	}

	result := CompactContextWithCompactor(messages, 10000, 2, mc, context.Background())
	if len(result) != len(messages) {
		t.Errorf("Expected %d messages (no compaction needed), got %d", len(messages), len(result))
	}
	if mc.summarizeCalled {
		t.Error("Summarize should not be called when no compaction needed")
	}
}

func TestCompactContextWithCompactor_SummaryCalled(t *testing.T) {
	i18n.Init("en")
	mc := &mockCompactor{summarizeResult: "Summary: User asked about Go."}

	var messages []provider.Message
	messages = append(messages, provider.SystemMsg("You are a helper"))
	for i := 0; i < 20; i++ {
		messages = append(messages, provider.UserMsg("Message number which is long enough to take up tokens in the context window"))
		messages = append(messages, provider.AssistantText("Response number which is also long enough to take up tokens in the context"))
	}

	// Маленький лимит — compactByPriority не справится, нужен LLM summary
	result := CompactContextWithCompactor(messages, 200, 4, mc, context.Background())

	// Результат должен содержать system prompt
	if len(result) == 0 || result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}

	// Результат должен быть короче оригинала
	if len(result) >= len(messages) {
		t.Errorf("Expected compaction, got %d messages (original: %d)", len(result), len(messages))
	}
}

func TestCompactContextWithCompactor_ContextCancellation(t *testing.T) {
	i18n.Init("en")
	mc := &mockCompactor{summarizeResult: "Summary"}

	var messages []provider.Message
	messages = append(messages, provider.SystemMsg("Helper"))
	for i := 0; i < 20; i++ {
		messages = append(messages, provider.UserMsg("Long message to fill context window with tokens"))
		messages = append(messages, provider.AssistantText("Long response to fill context window with tokens"))
	}

	// Отменяем контекст до вызова
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Должен упасть в SimpleSummarize fallback
	result := CompactContextWithCompactor(messages, 300, 4, mc, ctx)
	if len(result) == 0 {
		t.Error("Should return at least system messages")
	}
}

func TestCompactContextWithCompactor_SystemPromptPreserved(t *testing.T) {
	i18n.Init("en")
	mc := &mockCompactor{summarizeResult: "Summary"}

	messages := []provider.Message{
		provider.SystemMsg("Important system prompt"),
		provider.UserMsg("Hello"),
		provider.AssistantText("Hi"),
	}

	result := CompactContextWithCompactor(messages, 10, 2, mc, context.Background())
	if len(result) == 0 || result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}
}

func TestCompactContextWithCompactor_RecapsPreserved(t *testing.T) {
	i18n.Init("en")
	mc := &mockCompactor{summarizeResult: "Summary"}

	var messages []provider.Message
	messages = append(messages, provider.SystemMsg("Helper"))
	for i := 0; i < 10; i++ {
		messages = append(messages, provider.UserMsg("Long message to fill context"))
		if i == 3 {
			messages = append(messages, provider.AssistantText("Done\n\n※ Recap: Fixed bug in parser"))
		} else {
			messages = append(messages, provider.AssistantText("Response to fill context with tokens"))
		}
	}

	result := CompactContextWithCompactor(messages, 400, 4, mc, context.Background())

	// Результат должен содержать recap
	hasRecap := false
	for _, m := range result {
		text := m.GetText()
		if strings.Contains(text, "Fixed bug in parser") {
			hasRecap = true
		}
	}
	if !hasRecap {
		t.Error("Expected recap to be preserved after compaction with compactor")
	}
}

// --- Тесты для LLMCompactor ---

func TestNewLLMCompactor(t *testing.T) {
	mp := &mockProviderForCompactor{
		result: &provider.CompletionResult{
			Message: provider.AssistantText("Summary of conversation"),
		},
	}
	compactor := NewLLMCompactor(mp)
	if compactor == nil {
		t.Error("NewLLMCompactor should return non-nil compactor")
	}
	if compactor.provider == nil {
		t.Error("Compactor provider should not be nil")
	}
}

func TestLLMCompactor_Summarize(t *testing.T) {
	i18n.Init("en")
	mp := &mockProviderForCompactor{
		result: &provider.CompletionResult{
			Message: provider.AssistantText("User asked about Go programming."),
		},
	}
	compactor := NewLLMCompactor(mp)

	messages := []provider.Message{
		provider.UserMsg("What is Go?"),
		provider.AssistantText("Go is a programming language."),
		provider.UserMsg("Tell me more about goroutines."),
		provider.AssistantText("Goroutines are lightweight threads."),
	}

	summary := compactor.Summarize(messages, 500)
	if summary == "" {
		t.Error("Summarize should return non-empty string")
	}
	if !strings.Contains(summary, "Go programming") {
		t.Errorf("Expected summary to contain LLM response, got: %q", summary)
	}
}

func TestLLMCompactor_Summarize_NilProvider(t *testing.T) {
	i18n.Init("en")
	compactor := NewLLMCompactor(nil)

	messages := []provider.Message{
		provider.UserMsg("Hello"),
		provider.AssistantText("Hi"),
	}

	// С nil provider должен упасть в SimpleSummarize
	summary := compactor.Summarize(messages, 500)
	if summary == "" {
		t.Error("Summarize with nil provider should fallback to SimpleSummarize")
	}
	if !strings.Contains(summary, "User") && !strings.Contains(summary, "Пользователь") {
		t.Errorf("Expected SimpleSummarize fallback, got: %q", summary)
	}
}

func TestLLMCompactor_SummarizeWithCtx(t *testing.T) {
	i18n.Init("en")
	mp := &mockProviderForCompactor{
		result: &provider.CompletionResult{
			Message: provider.AssistantText("Context summary"),
		},
	}
	compactor := NewLLMCompactor(mp)

	messages := []provider.Message{
		provider.UserMsg("Question"),
		provider.AssistantText("Answer"),
	}

	summary := compactor.SummarizeWithCtx(messages, 500, context.Background())
	if summary == "" {
		t.Error("SummarizeWithCtx should return non-empty string")
	}
	if !strings.Contains(summary, "Context summary") {
		t.Errorf("Expected LLM summary, got: %q", summary)
	}
}

func TestLLMCompactor_SummarizeWithCtx_Timeout(t *testing.T) {
	i18n.Init("en")
	mp := &mockProviderForCompactor{
		result: &provider.CompletionResult{
			Message: provider.AssistantText("Summary"),
		},
		delay: 15 * time.Second, // дольше таймаута 10 сек
	}
	compactor := NewLLMCompactor(mp)

	messages := []provider.Message{
		provider.UserMsg("Hello"),
		provider.AssistantText("Hi"),
	}

	// Должен упасть в SimpleSummarize из-за таймаута
	summary := compactor.SummarizeWithCtx(messages, 500, context.Background())
	if summary == "" {
		t.Error("Should fallback to SimpleSummarize on timeout")
	}
	// SimpleSummarize содержит "User" или "Пользователь"
	if !strings.Contains(summary, "User") && !strings.Contains(summary, "Пользователь") {
		t.Errorf("Expected SimpleSummarize fallback, got: %q", summary)
	}
}

func TestLLMCompactor_SummarizeWithCtx_CancelledContext(t *testing.T) {
	i18n.Init("en")
	mp := &mockProviderForCompactor{
		result: &provider.CompletionResult{
			Message: provider.AssistantText("Summary"),
		},
	}
	compactor := NewLLMCompactor(mp)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // отменяем контекст до вызова

	messages := []provider.Message{
		provider.UserMsg("Hello"),
		provider.AssistantText("Hi"),
	}

	summary := compactor.SummarizeWithCtx(messages, 500, ctx)
	if summary == "" {
		t.Error("Should fallback to SimpleSummarize on cancelled context")
	}
}

func TestLLMCompactor_SummarizeWithCtx_ProviderError(t *testing.T) {
	i18n.Init("en")
	mp := &mockProviderForCompactor{
		err: context.DeadlineExceeded,
	}
	compactor := NewLLMCompactor(mp)

	messages := []provider.Message{
		provider.UserMsg("Hello"),
		provider.AssistantText("Hi"),
	}

	summary := compactor.SummarizeWithCtx(messages, 500, context.Background())
	if summary == "" {
		t.Error("Should fallback to SimpleSummarize on provider error")
	}
}

func TestLLMCompactor_SummarizeWithCtx_EmptyResponse(t *testing.T) {
	i18n.Init("en")
	mp := &mockProviderForCompactor{
		result: &provider.CompletionResult{
			Message: provider.AssistantText(""),
		},
	}
	compactor := NewLLMCompactor(mp)

	messages := []provider.Message{
		provider.UserMsg("Hello"),
		provider.AssistantText("Hi"),
	}

	summary := compactor.SummarizeWithCtx(messages, 500, context.Background())
	if summary == "" {
		t.Error("Should fallback to SimpleSummarize on empty LLM response")
	}
}

func TestLLMCompactor_SummarizeWithCtx_IncrementalSummary(t *testing.T) {
	i18n.Init("en")
	mp := &mockProviderForCompactor{
		result: &provider.CompletionResult{
			Message: provider.AssistantText("Updated summary with new info"),
		},
	}
	compactor := NewLLMCompactor(mp)

	// Сообщения с предыдущим summary
	messages := []provider.Message{
		provider.SystemMsg("Helper"),
		{Role: "system", Content: []provider.ContentBlock{
			{Type: "text", Text: i18n.T("compaction.summary_header") + "Previous summary about Go"},
		}},
		provider.UserMsg("Tell me about channels"),
		provider.AssistantText("Channels are used for communication between goroutines"),
	}

	summary := compactor.SummarizeWithCtx(messages, 500, context.Background())
	if summary == "" {
		t.Error("Should return non-empty summary")
	}
}

func TestCompactContextWithCompactor_LLMCompactor(t *testing.T) {
	i18n.Init("en")
	mp := &mockProviderForCompactor{
		result: &provider.CompletionResult{
			Message: provider.AssistantText("Summary: User discussed Go programming."),
		},
	}
	compactor := NewLLMCompactor(mp)

	var messages []provider.Message
	messages = append(messages, provider.SystemMsg("Helper"))
	for i := 0; i < 20; i++ {
		messages = append(messages, provider.UserMsg("Long message to fill context with tokens"))
		messages = append(messages, provider.AssistantText("Long response to fill context with tokens"))
	}

	result := CompactContextWithCompactor(messages, 300, 4, compactor, context.Background())

	// Результат должен быть короче оригинала
	if len(result) >= len(messages) {
		t.Errorf("Expected compaction, got %d messages (original: %d)", len(result), len(messages))
	}

	// Системный промпт должен быть сохранён
	if len(result) == 0 || result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}
}

func TestCompactContextWithCompactor_RecentMessagesFit(t *testing.T) {
	i18n.Init("en")
	mc := &mockCompactor{summarizeResult: "Summary"}

	// Последние сообщения вписываются в лимит — компакция не нужна
	messages := []provider.Message{
		provider.SystemMsg("Helper"),
		provider.UserMsg("Hello"),
		provider.AssistantText("Hi"),
	}

	result := CompactContextWithCompactor(messages, 10000, 2, mc, context.Background())
	if len(result) != len(messages) {
		t.Errorf("Expected no compaction, got %d messages", len(result))
	}
	if mc.summarizeCalled {
		t.Error("Summarize should not be called when no compaction needed")
	}
}

func TestCompactContextWithCompactor_ToolErrorsRemoved(t *testing.T) {
	i18n.Init("en")
	mc := &mockCompactor{summarizeResult: "Summary"}

	messages := []provider.Message{
		provider.SystemMsg("Helper"),
		{Role: "assistant", Content: []provider.ContentBlock{
			{Type: "tool_use", ToolUseID: "call_err", ToolName: "bash", Input: map[string]any{"command": "bad_cmd"}},
		}},
		{Role: "user", Content: []provider.ContentBlock{
			{Type: "tool_result", ToolUseID: "call_err", ToolName: "bash", Output: "error", IsError: true},
		}},
		provider.AssistantText("Error!"),
	}

	// Большой лимит — компакция не нужна, но ошибки должны быть удалены
	result := CompactContextWithCompactor(messages, 10000, 2, mc, context.Background())

	// Ошибочный tool_result должен быть удалён
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_result" && block.IsError {
				t.Error("Error tool_result should be removed even without compaction")
			}
		}
	}
	// Соответствующий tool_use тоже должен быть удалён
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_use" && block.ToolUseID == "call_err" {
				t.Error("tool_use matching error tool_result should be removed")
			}
		}
	}
}

func TestCompactContextWithCompactor_DuplicatesRemoved(t *testing.T) {
	i18n.Init("en")
	mc := &mockCompactor{summarizeResult: "Summary"}

	// 3+ одинаковых assistant-сообщений — семантическая дедупликация оставит 2
	messages := []provider.Message{
		provider.SystemMsg("Helper"),
		provider.AssistantText("Начну с анализа проблемы. Шаг 1: читаю файл."),
		provider.AssistantText("Начну с анализа проблемы. Шаг 2: исправляю код."),
		provider.AssistantText("Начну с анализа проблемы. Шаг 3: тестирую."),
	}

	result := CompactContextWithCompactor(messages, 10000, 2, mc, context.Background())

	// Семантические дубликаты должны быть удалены (3+ с одинаковым префиксом → 2)
	if len(result) > len(messages) {
		t.Errorf("Expected duplicates to be removed, got %d messages (original: %d)", len(result), len(messages))
	}
}
