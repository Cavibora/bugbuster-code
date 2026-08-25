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
	// Bug: fmt.Sprintf("%v", v) produced Go syntax [map[id:1 ...]]
	// instead of JSON [{"id":"1",...}]
	input := map[string]any{
		"todos": []any{
			map[string]any{"id": "1", "subject": "Test task", "status": "pending"},
			map[string]any{"id": "2", "subject": "Another task", "status": "in_progress"},
		},
	}
	params := convertInputToParams(input)

	todosJSON := params["todos"]
	// Should be valid JSON
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
	// Numbers should marshal into a JSON string
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
	// Test with a small token limit — compaction should trigger when exceeded
	ctx := NewConversationContextWithTokens(100, 2)
	ctx.Add(provider.SystemMsg("system prompt"))

	for i := 0; i < 20; i++ {
		ctx.Add(provider.UserMsg("This is a longer message to exceed the token limit and trigger compaction"))
	}

	// After compaction the system prompt should be preserved
	if ctx.Messages[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}

	// Messages should be fewer than 21 (compaction triggered)
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

	// Only the system prompt should remain
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
	// Initialize i18n for the test
	i18n.Init("en")

	toolList := map[string]tools.Tool{
		"read": tools.NewReadTool(),
	}

	prompt := BuildSystemPrompt("/tmp", toolList)

	// Should contain a note about available tools
	if !strings.Contains(prompt, "tools") && !strings.Contains(prompt, "инструмент") {
		t.Error("prompt should mention tools")
	}

	// Should contain an XML example with a name attribute
	if !strings.Contains(prompt, `<param name="path">`) {
		t.Error("prompt should contain XML example with name attribute")
	}

	// Should contain the closing tool tag
	if !strings.Contains(prompt, `</tool>`) {
		t.Error("prompt should contain closing </tool> tag")
	}

	// Should contain an examples section
	if !strings.Contains(prompt, "Examples:") {
		t.Error("prompt should contain examples section")
	}

	// Should contain a file read example
	if !strings.Contains(prompt, `Read a file:`) {
		t.Error("prompt should contain read file example")
	}

	// Should contain a file write example
	if !strings.Contains(prompt, `Write a file:`) {
		t.Error("prompt should contain write file example")
	}

	// Should contain a bash example
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

// MockProvider for tests
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

	// Fill the channel to capacity — maybeCompact will try to send an event
	eventCh := make(chan provider.StreamEvent, 1)
	eventCh <- provider.StreamEvent{Type: provider.EventTextDelta, Text: "fill"}

	done := make(chan bool, 1)
	go func() {
		a.maybeCompact(eventCh, context.Background())
		done <- true
	}()

	select {
	case <-done:
		// OK — did not block
	case <-time.After(2 * time.Second):
		t.Error("maybeCompact blocked on full event channel")
	}
}

func TestAgentLoop_SetPermissionChecker(t *testing.T) {
	mock := &MockProvider{}
	loop := NewAgentLoop(mock)

	// By default PermissionChecker is nil
	if loop.PermissionChecker != nil {
		t.Error("Expected nil PermissionChecker by default")
	}

	// Set deny mode
	checker := NewDefaultPermissionChecker(PermissionDeny, "/tmp")
	loop.SetPermissionChecker(checker)

	if loop.PermissionChecker == nil {
		t.Error("Expected non-nil PermissionChecker after SetPermissionChecker")
	}

	// Verify that deny mode blocks bash
	req := PermissionRequest{
		ToolName: "bash",
		Level:    PermDangerFullAccess,
	}
	result := loop.PermissionChecker.CheckPermission(req)
	if result != PermDenied {
		t.Errorf("Expected PermDenied for bash in deny mode, got %s", result)
	}

	// Verify that deny mode allows read
	req = PermissionRequest{
		ToolName: "read",
		Level:    PermReadOnly,
	}
	result = loop.PermissionChecker.CheckPermission(req)
	if result != PermApproved {
		t.Errorf("Expected PermApproved for read in deny mode, got %s", result)
	}
}

// TestSessionRestore_SystemPromptPreserved verifies that on restore
// the current system prompt is preserved, and old system messages are removed.
func TestSessionRestore_SystemPromptPreserved(t *testing.T) {
	loop := NewAgentLoop(nil)
	loop.Context = NewConversationContextWithTokens(8000, 6)

	// Set the system prompt (as agent_setup does)
	loop.SetSystemPrompt("Ты BugBuster — AI-ассистент для разработки")

	// Verify that the system prompt is set
	prompt := loop.Context.GetSystemPrompt()
	if prompt != "Ты BugBuster — AI-ассистент для разработки" {
		t.Errorf("Expected system prompt, got: %s", prompt)
	}

	// Simulate loading a session with 10 messages, including an old system prompt
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

	// Reproduce the restoreSessionMessages logic:
	// 1. Save the current system prompt
	currentSystemPrompt := loop.Context.GetSystemPrompt()

	// 2. Remove old system messages from the loaded session
	var filtered []provider.Message
	for _, m := range sessionMessages {
		if m.Role != "system" {
			filtered = append(filtered, m)
		}
	}

	// 3. Disable auto-compaction
	wasAutoCompact := loop.Context.AutoCompact
	loop.Context.AutoCompact = false

	// 4. Clear the context and add system prompt + session messages
	loop.Context.Messages = nil
	if currentSystemPrompt != "" {
		loop.Context.Messages = append(loop.Context.Messages, provider.SystemMsg(currentSystemPrompt))
	}
	loop.Context.Messages = append(loop.Context.Messages, filtered...)

	// 5. Restore auto-compaction
	loop.Context.AutoCompact = wasAutoCompact

	// Verify:
	// 1. System prompt is the current one, not the old one
	prompt = loop.Context.GetSystemPrompt()
	if prompt != "Ты BugBuster — AI-ассистент для разработки" {
		t.Errorf("Expected current system prompt, got: %s", prompt)
	}

	// 2. No old system message from the session
	for i, m := range loop.Context.Messages {
		if m.Role == "system" && m.GetText() == "Старый системный промпт из сессии" {
			t.Errorf("Found old system message at index %d, should have been removed", i)
		}
	}

	// 3. All non-system messages from the session are preserved
	userCount := 0
	for _, m := range loop.Context.Messages {
		if m.Role == "user" {
			userCount++
		}
	}
	if userCount != 5 {
		t.Errorf("Expected 5 user messages, got %d", userCount)
	}

	// 4. Total message count: 1 (system) + 9 (non-system from session)
	if len(loop.Context.Messages) != 10 {
		t.Errorf("Expected 10 messages (1 system + 9 session), got %d", len(loop.Context.Messages))
	}

	// 5. First message is the current system prompt
	if loop.Context.Messages[0].Role != "system" {
		t.Errorf("Expected first message to be system, got %s", loop.Context.Messages[0].Role)
	}
}

// TestSessionRestore_AutoCompactDisabled verifies that when loading a session
// auto-compaction does not trim the context immediately.
func TestSessionRestore_AutoCompactDisabled(t *testing.T) {
	loop := NewAgentLoop(nil)
	loop.Context = NewConversationContextWithTokens(100, 3) // Очень маленький лимит

	loop.SetSystemPrompt("System prompt")

	// Add many messages (exceeds the 100 token limit)
	for i := 0; i < 20; i++ {
		loop.Context.Add(provider.UserMsg("This is a test message that should be preserved during session restore"))
	}

	// After adding, the context should be compact (compaction triggered)
	compactedCount := len(loop.Context.Messages)

	// Now simulate session restore with auto-compaction disabled
	sessionMessages := make([]provider.Message, len(loop.Context.Messages))
	copy(sessionMessages, loop.Context.Messages)

	// Disable auto-compaction
	loop.Context.AutoCompact = false
	loop.Context.Messages = nil
	loop.Context.Messages = append(loop.Context.Messages, provider.SystemMsg("System prompt"))
	for _, m := range sessionMessages {
		if m.Role != "system" {
			loop.Context.Messages = append(loop.Context.Messages, m)
		}
	}

	// All messages should be loaded without trimming
	totalLoaded := len(loop.Context.Messages)
	if totalLoaded < compactedCount {
		t.Errorf("After restore with AutoCompact=false, expected >= %d messages, got %d", compactedCount, totalLoaded)
	}

	// Re-enable auto-compaction
	loop.Context.AutoCompact = true

	// Add one more message — now compaction can trigger
	loop.Context.Add(provider.UserMsg("New message after restore"))

	// After compaction the context should be compact
	if len(loop.Context.Messages) > totalLoaded+1 {
		t.Errorf("Context should be compacted after adding new message, got %d messages", len(loop.Context.Messages))
	}
}

// TestTimeoutDefaults verifies that timeouts have correct default values
func TestTimeoutDefaults(t *testing.T) {
	loop := NewAgentLoop(nil)

	// By default timeouts = 0 (effective methods with defaults are used)
	if loop.RequestTimeout != 0 {
		t.Errorf("Expected default RequestTimeout=0, got %v", loop.RequestTimeout)
	}
	if loop.ThinkingTimeout != 0 {
		t.Errorf("Expected default ThinkingTimeout=0, got %v", loop.ThinkingTimeout)
	}
	if loop.IdleTimeout != 0 {
		t.Errorf("Expected default IdleTimeout=0, got %v", loop.IdleTimeout)
	}

	// Effective timeouts should return default values
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

// TestTimeoutSetters verifies the timeout setters
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

	// Effective timeouts should return the set values
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

// MockStreamingProvider — a provider that always returns tool_use,
// so the agent keeps iterating indefinitely (until the limit triggers).
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

// MockNoOpTool — a tool that does nothing (for tests)
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

	// Collect all events
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

	// The agent should have stopped after maxIterations (3)
	if iterations > 3 {
		t.Errorf("Expected at most 3 iterations, got %d", iterations)
	}
	if !gotDone {
		t.Error("Expected EventDone to be sent after maxIterations exceeded")
	}
}

func TestStreamWithCancel_NoMaxIterations(t *testing.T) {
	// Verify that without maxIterations (0) the limit does not trigger
	// Use a short context timeout so the test does not hang
	i18n.Init("en")

	mock := &MockStreamingProvider{}
	loop := NewAgentLoop(mock)
	// maxIterations = 0 (unlimited by default)
	loop.RegisterTool(&MockNoOpTool{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eventCh, err := loop.StreamWithCancel(ctx, "test query")
	if err != nil {
		t.Fatalf("StreamWithCancel failed: %v", err)
	}

	// Just wait for completion — without maxIterations the agent works while:
	// 1) the model will not stop calling tools, or
	// 2) the context will not be cancelled
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
		// Standalone recap words at start of text (without colon/dash)
		{"Recap\nFixed the bug", true},
		{"Recap Fixed the bug", true},
		{"Recap. Here's what was done.", true},
		{"Итог\nИсправлен баг", true},
		{"Итоги\nИсправлены баги", true},
		{"Summary\nAll changes applied", true},
		{"Резюме\nВсё сделано", true},
		{"Результаты\nПрименены", true},
		// Recap word NOT at start should NOT match (unless with colon/dash)
		{"Here is the recap of what we did", false},
		// Context compaction acknowledgment
		{"[Context was compacted to save space]", true},
		{"Context was compacted, let me continue", true},
		{"Let me check the current state of the project", true},
		{"Let me re-establish what we were working on", true},
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
		// Additional completion phrases
		{"That's all for now", true},
		{"That is all", true},
		{"That's it, the task is done", true},
		{"Nothing else to do here", true},
		{"No more work needed", true},
		{"The task is finished", true},
		{"Work complete", true},
		{"Mission accomplished", true},
		{"Всё", true},
		{"Конец", true},
		{"Завершено", true},
		{"Выполнено", true},
		{"Changes applied successfully", true},
		{"Changes made to the file", true},
		{"Fixed the issue", true},
		{"Fixed the bug", true},
		{"Исправлено", true},
		{"Исправлено!", true},
		{"Изменения внесены", true},
		{"Изменения применены", true},
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
		{"The bug is in the main function", false},
		{"I need to read the file first", false},
		// Markdown headings
		{"## Recap\n\n- Fixed bug\n- Added tests", true},
		{"## Итог\n\n- Исправлен баг\n- Добавлены тесты", true},
		{"## Summary\n\nAll changes applied", true},
		{"# Recap\n\nDone", true},
		// Standalone completion words
		{"Done", true},
		{"Done.", true},
		{"Done!", true},
		{"Готово", true},
		{"Готово.", true},
		{"Готово!", true},
		// "all tests pass" in middle of text is NOT completion (model continues)
		{"All tests pass. Now let me commit:", false},
		// "committed and pushed" as completion signal
		{"Changes committed and pushed", true},
	}

	for _, tt := range tests {
		result := LooksLikeCompletion(tt.text)
		if result != tt.expected {
			t.Errorf("LooksLikeCompletion(%q) = %v, want %v", tt.text, result, tt.expected)
		}
	}
}
