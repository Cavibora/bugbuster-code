package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
	"bugbuster-code/pkg/tools"
)

func TestParseToolCalls_XML(t *testing.T) {
	response := `Я прочитаю файл:
<tool name="read">
<path>main.go</path>
</tool>`

	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(calls))
	}
	if calls[0].Name != "read" {
		t.Errorf("Expected tool 'read', got '%s'", calls[0].Name)
	}
	if calls[0].Params["path"] != "main.go" {
		t.Errorf("Expected path 'main.go', got '%s'", calls[0].Params["path"])
	}
}

func TestParseToolCalls_XMLMultiple(t *testing.T) {
	response := `<tool name="read">
<path>file1.go</path>
</tool>
<tool name="read">
<path>file2.go</path>
</tool>`

	calls := ParseToolCalls(response)
	if len(calls) != 2 {
		t.Fatalf("Expected 2 tool calls, got %d", len(calls))
	}
	if calls[0].Params["path"] != "file1.go" {
		t.Errorf("Expected path 'file1.go', got '%s'", calls[0].Params["path"])
	}
	if calls[1].Params["path"] != "file2.go" {
		t.Errorf("Expected path 'file2.go', got '%s'", calls[1].Params["path"])
	}
}

func TestParseToolCalls_JSON(t *testing.T) {
	response := `{"tool": "bash", "params": {"command": "go test ./..."}}`

	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Errorf("Expected tool 'bash', got '%s'", calls[0].Name)
	}
	if calls[0].Params["command"] != "go test ./..." {
		t.Errorf("Expected command 'go test ./...', got '%s'", calls[0].Params["command"])
	}
}

func TestParseToolCalls_NoCalls(t *testing.T) {
	response := "Это обычный текст без вызовов инструментов."
	calls := ParseToolCalls(response)
	if len(calls) != 0 {
		t.Errorf("Expected 0 tool calls, got %d", len(calls))
	}
}

func TestStripToolCalls_XML(t *testing.T) {
	response := `Вот результат:
<tool name="read">
<path>main.go</path>
</tool>
Конец.`

	result := StripToolCalls(response)
	if result == "" {
		t.Error("Expected non-empty result")
	}
}

func TestStripToolCalls_JSON(t *testing.T) {
	response := `Вот результат:
{"tool": "bash", "params": {"command": "ls"}}
Конец.`

	result := StripToolCalls(response)
	if result == "" {
		t.Error("Expected non-empty result")
	}
}

func TestConvertParsedToContentBlocks(t *testing.T) {
	calls := []ToolCall{
		{Name: "read", Params: map[string]string{"path": "/tmp/test.go"}},
		{Name: "bash", Params: map[string]string{"command": "ls -la"}},
	}
	blocks := convertParsedToContentBlocks(calls)
	if len(blocks) != 2 {
		t.Fatalf("Expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].ToolName != "read" {
		t.Errorf("Expected tool 'read', got '%s'", blocks[0].ToolName)
	}
	if blocks[0].Input["path"] != "/tmp/test.go" {
		t.Errorf("Expected path '/tmp/test.go', got '%v'", blocks[0].Input["path"])
	}
	if blocks[1].ToolName != "bash" {
		t.Errorf("Expected tool 'bash', got '%s'", blocks[1].ToolName)
	}
	if blocks[1].Input["command"] != "ls -la" {
		t.Errorf("Expected command 'ls -la', got '%v'", blocks[1].Input["command"])
	}
}

func TestConvertInputToParams(t *testing.T) {
	input := map[string]any{"path": "/tmp/test.go", "command": "ls"}
	params := convertInputToParams(input)
	if params["path"] != "/tmp/test.go" {
		t.Errorf("Expected path '/tmp/test.go', got '%s'", params["path"])
	}
	if params["command"] != "ls" {
		t.Errorf("Expected command 'ls', got '%s'", params["command"])
	}
}

func TestConvertInputToParams_Empty(t *testing.T) {
	input := map[string]any{}
	params := convertInputToParams(input)
	if len(params) != 0 {
		t.Errorf("Expected empty params, got %v", params)
	}
}

func TestConvertInputToParams_Array(t *testing.T) {
	// Баг: fmt.Sprintf("%v", v) давал Go-синтаксис [map[id:1 ...]]
	// вместо JSON [{"id":"1",...}]
	input := map[string]any{
		"todos": []any{
			map[string]any{"id": "1", "subject": "Test task", "status": "pending"},
			map[string]any{"id": "2", "subject": "Another task", "status": "in_progress"},
		},
	}
	params := convertInputToParams(input)

	todosJSON := params["todos"]
	// Должен быть валидный JSON
	var items []map[string]any
	if err := json.Unmarshal([]byte(todosJSON), &items); err != nil {
		t.Fatalf("todos should be valid JSON, got: %s, error: %v", todosJSON, err)
	}
	if len(items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(items))
	}
	if items[0]["id"] != "1" {
		t.Errorf("Expected id '1', got '%v'", items[0]["id"])
	}
	if items[1]["status"] != "in_progress" {
		t.Errorf("Expected status 'in_progress', got '%v'", items[1]["status"])
	}
}

func TestConvertInputToParams_NestedObject(t *testing.T) {
	input := map[string]any{
		"config": map[string]any{
			"timeout": 30,
			"verbose": true,
		},
	}
	params := convertInputToParams(input)

	configJSON := params["config"]
	var cfg map[string]any
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		t.Fatalf("config should be valid JSON, got: %s, error: %v", configJSON, err)
	}
	if cfg["timeout"] != float64(30) {
		t.Errorf("Expected timeout 30, got %v", cfg["timeout"])
	}
	if cfg["verbose"] != true {
		t.Errorf("Expected verbose true, got %v", cfg["verbose"])
	}
}

func TestConvertInputToParams_Nil(t *testing.T) {
	input := map[string]any{
		"path":    "/tmp/test.go",
		"content": nil,
	}
	params := convertInputToParams(input)
	if params["path"] != "/tmp/test.go" {
		t.Errorf("Expected path '/tmp/test.go', got '%s'", params["path"])
	}
	if params["content"] != "" {
		t.Errorf("Expected empty string for nil, got '%s'", params["content"])
	}
}

func TestConvertInputToParams_Number(t *testing.T) {
	input := map[string]any{
		"line":      10,
		"character": 5,
	}
	params := convertInputToParams(input)
	// Числа должны маршалиться в JSON-строку
	if params["line"] != "10" {
		t.Errorf("Expected line '10', got '%s'", params["line"])
	}
	if params["character"] != "5" {
		t.Errorf("Expected character '5', got '%s'", params["character"])
	}
}

func TestConvertInputToParams_Bool(t *testing.T) {
	input := map[string]any{
		"verbose": true,
		"quiet":   false,
	}
	params := convertInputToParams(input)
	if params["verbose"] != "true" {
		t.Errorf("Expected verbose 'true', got '%s'", params["verbose"])
	}
	if params["quiet"] != "false" {
		t.Errorf("Expected quiet 'false', got '%s'", params["quiet"])
	}
}

func TestParseToolInput_Empty(t *testing.T) {
	result := parseToolInput("")
	if len(result) != 0 {
		t.Errorf("Expected empty map for empty input, got %v", result)
	}
}

func TestParseToolInput_EmptyJSON(t *testing.T) {
	result := parseToolInput("{}")
	if len(result) != 0 {
		t.Errorf("Expected empty map for empty JSON, got %v", result)
	}
}

func TestParseToolInput_ValidJSON(t *testing.T) {
	result := parseToolInput(`{"path": "/tmp/test.go"}`)
	if result["path"] != "/tmp/test.go" {
		t.Errorf("Expected path '/tmp/test.go', got '%v'", result["path"])
	}
}
func TestConversationContext_Add(t *testing.T) {
	ctx := NewConversationContext(10)
	ctx.Add(provider.UserMsg("привет"))
	ctx.Add(provider.AssistantText("здравствуй"))

	if len(ctx.Messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(ctx.Messages))
	}
	if ctx.Messages[0].Role != "user" {
		t.Errorf("Expected RoleUser, got %s", ctx.Messages[0].Role)
	}
	if ctx.Messages[1].Role != "assistant" {
		t.Errorf("Expected RoleAssistant, got %s", ctx.Messages[1].Role)
	}
}

func TestConversationContext_Trim(t *testing.T) {
	// Тест с малым токен-лимитом — при превышении должна сработать компакция
	ctx := NewConversationContextWithTokens(100, 2)
	ctx.Add(provider.SystemMsg("system prompt"))

	for i := 0; i < 20; i++ {
		ctx.Add(provider.UserMsg("This is a longer message to exceed the token limit and trigger compaction"))
	}

	// После компакции системный промпт должен быть сохранён
	if ctx.Messages[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}

	// Сообщений должно быть меньше 21 (компакция сработала)
	if len(ctx.Messages) > 15 {
		t.Errorf("Expected compaction to reduce messages, got %d", len(ctx.Messages))
	}
}

func TestConversationContext_Reset(t *testing.T) {
	ctx := NewConversationContextWithTokens(8000, 6)
	ctx.Add(provider.SystemMsg("system prompt"))
	ctx.Add(provider.UserMsg("hello"))
	ctx.Add(provider.AssistantText("hi"))

	ctx.Reset()

	// Должен остаться только системный промпт
	if len(ctx.Messages) != 1 {
		t.Errorf("Expected 1 message after reset, got %d", len(ctx.Messages))
	}
	if ctx.Messages[0].Role != "system" {
		t.Error("System prompt should be preserved after reset")
	}
}

func TestConversationContext_BuildPrompt(t *testing.T) {
	ctx := NewConversationContext(10)
	ctx.Add(provider.SystemMsg("ты помощник"))
	ctx.Add(provider.UserMsg("привет"))
	ctx.Add(provider.AssistantText("здравствуй"))

	prompt := ctx.BuildPrompt()
	if prompt == "" {
		t.Error("Expected non-empty prompt")
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	toolList := map[string]tools.Tool{}

	prompt := BuildSystemPrompt("/home/user/project", toolList)
	if prompt == "" {
		t.Error("Expected non-empty system prompt")
	}
}

func TestBuildSystemPromptIncludesCapabilities(t *testing.T) {
	// Инициализируем i18n для теста
	i18n.Init("en")

	toolList := map[string]tools.Tool{
		"read": tools.NewReadTool(),
	}

	prompt := BuildSystemPrompt("/tmp", toolList)

	// Должен содержать указание о наличии инструментов
	if !strings.Contains(prompt, "tools") && !strings.Contains(prompt, "инструмент") {
		t.Error("prompt should mention tools")
	}

	// Должен содержать XML пример с name атрибутом
	if !strings.Contains(prompt, `<param name="path">`) {
		t.Error("prompt should contain XML example with name attribute")
	}

	// Должен содержать закрывающий тег tool
	if !strings.Contains(prompt, `</tool>`) {
		t.Error("prompt should contain closing </tool> tag")
	}

	// Должен содержать секцию примеров
	if !strings.Contains(prompt, "Examples:") {
		t.Error("prompt should contain examples section")
	}

	// Должен содержать пример чтения файла
	if !strings.Contains(prompt, `Read a file:`) {
		t.Error("prompt should contain read file example")
	}

	// Должен содержать пример записи файла
	if !strings.Contains(prompt, `Write a file:`) {
		t.Error("prompt should contain write file example")
	}

	// Должен содержать пример bash
	if !strings.Contains(prompt, `Run a command:`) {
		t.Error("prompt should contain bash example")
	}
}

func TestParseToolInput(t *testing.T) {
	// JSON input
	result := parseToolInput(`{"path": "main.go", "content": "hello"}`)
	if result["path"] != "main.go" {
		t.Errorf("Expected path='main.go', got '%v'", result["path"])
	}

	// Non-JSON input
	result = parseToolInput("plain text")
	if result["input"] != "plain text" {
		t.Errorf("Expected input='plain text', got '%v'", result["input"])
	}
}

// MockProvider для тестов
type MockProvider struct {
	response provider.CompletionResult
	err      error
}

func (m *MockProvider) Name() string { return "mock" }
func (m *MockProvider) Model() string { return "mock-model" }

func (m *MockProvider) Complete(messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	return &m.response, m.err
}

func (m *MockProvider) Stream(messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.StreamEvent{Type: "done"}
	close(ch)
	return ch, nil
}

func (m *MockProvider) CompleteWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	return m.Complete(messages, toolDefs)
}

func (m *MockProvider) StreamWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.Stream(messages, toolDefs)
}

func TestMaybeCompact_NonBlockingSend(t *testing.T) {
	i18n.Init("en")
	a := NewAgentLoop(&MockProvider{})
	a.Context.MaxTokens = 10
	a.Context.AutoCompact = false
	a.Context.Messages = []provider.Message{
		provider.SystemMsg("very long system prompt that exceeds the token limit by a lot"),
		provider.UserMsg("hello world this is a test message"),
	}

	// Заполняем канал до отказа — maybeCompact попытается отправить событие
	eventCh := make(chan provider.StreamEvent, 1)
	eventCh <- provider.StreamEvent{Type: provider.EventTextDelta, Text: "fill"}

	done := make(chan bool, 1)
	go func() {
		a.maybeCompact(eventCh, context.Background())
		done <- true
	}()

	select {
	case <-done:
		// OK — не заблокировалось
	case <-time.After(2 * time.Second):
		t.Error("maybeCompact blocked on full event channel")
	}
}

func TestAgentLoop_SetPermissionChecker(t *testing.T) {
	mock := &MockProvider{}
	loop := NewAgentLoop(mock)

	// По умолчанию PermissionChecker nil
	if loop.PermissionChecker != nil {
		t.Error("Expected nil PermissionChecker by default")
	}

	// Устанавливаем deny-режим
	checker := NewDefaultPermissionChecker(PermissionDeny, "/tmp")
	loop.SetPermissionChecker(checker)

	if loop.PermissionChecker == nil {
		t.Error("Expected non-nil PermissionChecker after SetPermissionChecker")
	}

	// Проверяем что deny-режим блокирует bash
	req := PermissionRequest{
		ToolName: "bash",
		Level:    PermDangerFullAccess,
	}
	result := loop.PermissionChecker.CheckPermission(req)
	if result != PermDenied {
		t.Errorf("Expected PermDenied for bash in deny mode, got %s", result)
	}

	// Проверяем что deny-режим разрешает read
	req = PermissionRequest{
		ToolName: "read",
		Level:    PermReadOnly,
	}
	result = loop.PermissionChecker.CheckPermission(req)
	if result != PermApproved {
		t.Errorf("Expected PermApproved for read in deny mode, got %s", result)
	}
}

// TestSessionRestore_SystemPromptPreserved проверяет, что при восстановлении
// сессии текущий системный промпт сохраняется, а старые system-сообщения удаляются.
func TestSessionRestore_SystemPromptPreserved(t *testing.T) {
	loop := NewAgentLoop(nil)
	loop.Context = NewConversationContextWithTokens(8000, 6)

	// Устанавливаем системный промпт (как это делает agent_setup)
	loop.SetSystemPrompt("Ты BugBuster — AI-ассистент для разработки")

	// Проверяем, что системный промпт установлен
	prompt := loop.Context.GetSystemPrompt()
	if prompt != "Ты BugBuster — AI-ассистент для разработки" {
		t.Errorf("Expected system prompt, got: %s", prompt)
	}

	// Симулируем загрузку сессии с 10 сообщениями, включая старый system-промпт
	sessionMessages := []provider.Message{
		provider.SystemMsg("Старый системный промпт из сессии"), // Должен быть удалён
		provider.UserMsg("Привет!"),
		provider.AssistantText("Здравствуй!"),
		provider.UserMsg("Расскажи о проекте"),
		provider.AssistantText("Это проект BugBuster..."),
		provider.UserMsg("Добавь тесты"),
		provider.AssistantText("Добавляю тесты..."),
		provider.UserMsg("Запусти тесты"),
		provider.AssistantText("Тесты пройдены!"),
		provider.UserMsg("Сохрани изменения"),
	}

	// Воспроизводим логику restoreSessionMessages:
	// 1. Сохраняем текущий системный промпт
	currentSystemPrompt := loop.Context.GetSystemPrompt()

	// 2. Удаляем старые system-сообщения из загруженной сессии
	var filtered []provider.Message
	for _, m := range sessionMessages {
		if m.Role != "system" {
			filtered = append(filtered, m)
		}
	}

	// 3. Отключаем автокомпакцию
	wasAutoCompact := loop.Context.AutoCompact
	loop.Context.AutoCompact = false

	// 4. Очищаем контекст и добавляем системный промпт + сообщения сессии
	loop.Context.Messages = nil
	if currentSystemPrompt != "" {
		loop.Context.Messages = append(loop.Context.Messages, provider.SystemMsg(currentSystemPrompt))
	}
	loop.Context.Messages = append(loop.Context.Messages, filtered...)

	// 5. Восстанавливаем автокомпакцию
	loop.Context.AutoCompact = wasAutoCompact

	// Проверяем:
	// 1. Системный промпт — текущий, не старый
	prompt = loop.Context.GetSystemPrompt()
	if prompt != "Ты BugBuster — AI-ассистент для разработки" {
		t.Errorf("Expected current system prompt, got: %s", prompt)
	}

	// 2. Нет старого system-сообщения из сессии
	for i, m := range loop.Context.Messages {
		if m.Role == "system" && m.GetText() == "Старый системный промпт из сессии" {
			t.Errorf("Found old system message at index %d, should have been removed", i)
		}
	}

	// 3. Все не-system сообщения из сессии сохранены
	userCount := 0
	for _, m := range loop.Context.Messages {
		if m.Role == "user" {
			userCount++
		}
	}
	if userCount != 5 {
		t.Errorf("Expected 5 user messages, got %d", userCount)
	}

	// 4. Общее количество сообщений: 1 (system) + 9 (не-system из сессии)
	if len(loop.Context.Messages) != 10 {
		t.Errorf("Expected 10 messages (1 system + 9 session), got %d", len(loop.Context.Messages))
	}

	// 5. Первое сообщение — текущий system-промпт
	if loop.Context.Messages[0].Role != "system" {
		t.Errorf("Expected first message to be system, got %s", loop.Context.Messages[0].Role)
	}
}

// TestSessionRestore_AutoCompactDisabled проверяет, что при загрузке сессии
// автокомпакция не обрезает контекст немедленно.
func TestSessionRestore_AutoCompactDisabled(t *testing.T) {
	loop := NewAgentLoop(nil)
	loop.Context = NewConversationContextWithTokens(100, 3) // Очень маленький лимит

	loop.SetSystemPrompt("System prompt")

	// Добавляем много сообщений (превышает лимит 100 токенов)
	for i := 0; i < 20; i++ {
		loop.Context.Add(provider.UserMsg("This is a test message that should be preserved during session restore"))
	}

	// После добавления контекст должен быть компактным (компакция сработала)
	compactedCount := len(loop.Context.Messages)

	// Теперь симулируем восстановление сессии с отключённой автокомпакцией
	sessionMessages := make([]provider.Message, len(loop.Context.Messages))
	copy(sessionMessages, loop.Context.Messages)

	// Отключаем автокомпакцию
	loop.Context.AutoCompact = false
	loop.Context.Messages = nil
	loop.Context.Messages = append(loop.Context.Messages, provider.SystemMsg("System prompt"))
	for _, m := range sessionMessages {
		if m.Role != "system" {
			loop.Context.Messages = append(loop.Context.Messages, m)
		}
	}

	// Все сообщения должны быть загружены без обрезки
	totalLoaded := len(loop.Context.Messages)
	if totalLoaded < compactedCount {
		t.Errorf("After restore with AutoCompact=false, expected >= %d messages, got %d", compactedCount, totalLoaded)
	}

	// Включаем автокомпакцию обратно
	loop.Context.AutoCompact = true

	// Добавляем ещё одно сообщение — теперь компакция может сработать
	loop.Context.Add(provider.UserMsg("New message after restore"))

	// После компакции контекст должен быть компактным
	if len(loop.Context.Messages) > totalLoaded+1 {
		t.Errorf("Context should be compacted after adding new message, got %d messages", len(loop.Context.Messages))
	}
}

// TestTimeoutDefaults проверяет, что таймауты имеют правильные дефолтные значения
func TestTimeoutDefaults(t *testing.T) {
	loop := NewAgentLoop(nil)

	// По умолчанию таймауты = 0 (используются effective-методы с дефолтами)
	if loop.RequestTimeout != 0 {
		t.Errorf("Expected default RequestTimeout=0, got %v", loop.RequestTimeout)
	}
	if loop.ThinkingTimeout != 0 {
		t.Errorf("Expected default ThinkingTimeout=0, got %v", loop.ThinkingTimeout)
	}
	if loop.IdleTimeout != 0 {
		t.Errorf("Expected default IdleTimeout=0, got %v", loop.IdleTimeout)
	}

	// Effective таймауты должны возвращать дефолтные значения
	if loop.effectiveRequestTimeout() != 40*time.Minute {
		t.Errorf("Expected effective request timeout 40m, got %v", loop.effectiveRequestTimeout())
	}
	if loop.effectiveThinkingTimeout() != 10*time.Minute {
		t.Errorf("Expected effective thinking timeout 10m, got %v", loop.effectiveThinkingTimeout())
	}
	if loop.effectiveIdleTimeout() != 5*time.Minute {
		t.Errorf("Expected effective idle timeout 5m, got %v", loop.effectiveIdleTimeout())
	}
}

// TestTimeoutSetters проверяет сеттеры таймаутов
func TestTimeoutSetters(t *testing.T) {
	loop := NewAgentLoop(nil)

	loop.SetRequestTimeout(30 * time.Minute)
	loop.SetThinkingTimeout(15 * time.Minute)
	loop.SetIdleTimeout(3 * time.Minute)

	if loop.RequestTimeout != 30*time.Minute {
		t.Errorf("Expected RequestTimeout=30m, got %v", loop.RequestTimeout)
	}
	if loop.ThinkingTimeout != 15*time.Minute {
		t.Errorf("Expected ThinkingTimeout=15m, got %v", loop.ThinkingTimeout)
	}
	if loop.IdleTimeout != 3*time.Minute {
		t.Errorf("Expected IdleTimeout=3m, got %v", loop.IdleTimeout)
	}

	// Effective таймауты должны возвращать установленные значения
	if loop.effectiveRequestTimeout() != 30*time.Minute {
		t.Errorf("Expected effective request timeout 30m, got %v", loop.effectiveRequestTimeout())
	}
	if loop.effectiveThinkingTimeout() != 15*time.Minute {
		t.Errorf("Expected effective thinking timeout 15m, got %v", loop.effectiveThinkingTimeout())
	}
	if loop.effectiveIdleTimeout() != 3*time.Minute {
		t.Errorf("Expected effective idle timeout 3m, got %v", loop.effectiveIdleTimeout())
	}
}

// MockStreamingProvider — провайдер, который всегда возвращает tool_use,
// чтобы агент продолжал итерации бесконечно (пока не сработает ограничение).
type MockStreamingProvider struct {
	iterationCount int
	events         []provider.StreamEvent // if set, use these events instead of default tool_use
}

func (m *MockStreamingProvider) Name() string { return "mock-streaming" }
func (m *MockStreamingProvider) Model() string { return "mock-streaming-model" }

func (m *MockStreamingProvider) Complete(messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	return &provider.CompletionResult{}, nil
}

func (m *MockStreamingProvider) Stream(messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.streamEvents()
}

func (m *MockStreamingProvider) CompleteWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	return m.Complete(messages, toolDefs)
}

func (m *MockStreamingProvider) StreamWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.Stream(messages, toolDefs)
}

func (m *MockStreamingProvider) streamEvents() (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent, 10)
	m.iterationCount++

	// If custom events are set, use them
	if len(m.events) > 0 {
		for _, event := range m.events {
			ch <- event
		}
		close(ch)
		return ch, nil
	}

	// Default: always return tool_use — agent will continue iterations
	ch <- provider.StreamEvent{Type: provider.EventToolCallStart, ToolName: "read", ToolCallID: "call_1"}
	ch <- provider.StreamEvent{Type: provider.EventToolCallDelta, ToolDelta: `{"path":"/tmp/test"}`, ToolCallID: "call_1"}
	ch <- provider.StreamEvent{Type: provider.EventToolCallEnd, ToolCallID: "call_1"}
	ch <- provider.StreamEvent{Type: provider.EventDone}
	close(ch)
	return ch, nil
}

// MockNoOpTool — инструмент, который ничего не делает (для тестов)
type MockNoOpTool struct{}

func (t *MockNoOpTool) Name() string        { return "read" }
func (t *MockNoOpTool) Description() string { return "mock read tool" }
func (t *MockNoOpTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		"required": []string{"path"},
	}
}
func (t *MockNoOpTool) Execute(params map[string]string) tools.ToolResult {
	return tools.ToolResult{Output: "mock output"}
}

func TestStreamWithCancel_MaxIterations(t *testing.T) {
	i18n.Init("en")

	mock := &MockStreamingProvider{}
	loop := NewAgentLoop(mock)
	loop.SetMaxIterations(3)
	loop.RegisterTool(&MockNoOpTool{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventCh, err := loop.StreamWithCancel(ctx, "test query")
	if err != nil {
		t.Fatalf("StreamWithCancel failed: %v", err)
	}

	// Собираем все события
	var iterations int
	var gotDone bool
	for event := range eventCh {
		switch event.Type {
		case provider.EventIterationEnd:
			iterations++
		case provider.EventDone:
			gotDone = true
		case provider.EventError:
			t.Errorf("Unexpected error event: %v", event.Error)
		}
	}

	// Агент должен был остановиться после maxIterations (3)
	if iterations > 3 {
		t.Errorf("Expected at most 3 iterations, got %d", iterations)
	}
	if !gotDone {
		t.Error("Expected EventDone to be sent after maxIterations exceeded")
	}
}

func TestStreamWithCancel_NoMaxIterations(t *testing.T) {
	// Проверяем что без maxIterations (0) ограничение не срабатывает
	// Используем короткий таймаут контекста чтобы тест не висел
	i18n.Init("en")

	mock := &MockStreamingProvider{}
	loop := NewAgentLoop(mock)
	// maxIterations = 0 (безлимит по умолчанию)
	loop.RegisterTool(&MockNoOpTool{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eventCh, err := loop.StreamWithCancel(ctx, "test query")
	if err != nil {
		t.Fatalf("StreamWithCancel failed: %v", err)
	}

	// Просто ждём завершения — без maxIterations агент работает пока:
	// 1) модель не перестанет вызывать инструменты, или
	// 2) контекст не отменится
	var gotAnyEvent bool
	for event := range eventCh {
		gotAnyEvent = true
		_ = event
	}

	if !gotAnyEvent {
		t.Error("Expected at least one event from StreamWithCancel")
	}
}

func TestLooksLikeCompletion(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		// Recap markers (various formats)
		{"※ Recap: Fixed the bug", true},
		{"Some text\n※ Recap: Done", true},
		{"※ Recap — fixed the bug", true},  // without colon
		{"※ Итог: Исправлен баг", true},
		{"※ Итог — исправлен баг", true},   // without colon
		{"※ Итоги: Исправлены баги", true},
		{"※ Итоги — исправлены баги", true}, // without colon
		{"Recap: all changes applied", true},
		{"итог: задача выполнена", true},
		{"итоги: задачи выполнены", true},
		{"Summary: fixed 3 bugs", true},
		{"резюме: всё сделано", true},
		{"результаты: применены", true},
		// ※ symbol alone means recap
		{"※ Fixed the bug and updated the file", true},
		// Explicit completion signals
		{"Всё готово!", true},
		{"Всё сделано", true},
		{"Готово!", true},
		{"All done!", true},
		{"Everything works correctly", true},
		{"Task is complete", true},
		{"Task is done", true},
		{"Work is done", true},
		{"Nothing more to do", true},
		{"No more changes needed", true},
		{"No further action required", true},
		{"All changes have been applied", true},
		// Short answers
		{"Да", true},
		{"Нет, это не так", true},
		{"Yes", true},
		{"No", true},
		{"OK", true},
		// Not completion
		{"Let me check the code and fix the bug", false},
		{"I'll continue working on this", false},
		{"", false},
		{"Here's what I found:\n1. Bug in line 5\n2. Need to fix", false},
	}

	for _, tt := range tests {
		result := LooksLikeCompletion(tt.text)
		if result != tt.expected {
			t.Errorf("LooksLikeCompletion(%q) = %v, want %v", tt.text, result, tt.expected)
		}
	}
}
