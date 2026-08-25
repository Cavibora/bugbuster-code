package agent

import (
	"fmt"
	"strings"
	"testing"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		minToken int // минимальное ожидаемое количество
		maxToken int // максимальное ожидаемое количество
	}{
		{"empty", "", 0, 0},
		{"short_en", "hello world", 2, 5},
		{"short_ru", "привет мир", 3, 8},
		{"long_en", "This is a longer sentence with more words to estimate tokens accurately.", 10, 25},
		{"mixed", "Hello мир mixed text", 3, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EstimateTokens(tt.text)
			if result < tt.minToken || result > tt.maxToken {
				t.Errorf("EstimateTokens(%q) = %d, want between %d and %d", tt.text, result, tt.minToken, tt.maxToken)
			}
		})
	}
}

func TestEstimateTokens_Cyrillic(t *testing.T) {
	// Cyrillic text should yield more tokens (fewer chars/token)
	ruText := "Привет, как дела? Это тестовое сообщение на русском языке."
	enText := "Hello, how are you? This is a test message in English language."

	ruTokens := EstimateTokens(ruText)
	enTokens := EstimateTokens(enText)

	// Cyrillic text should yield more tokens at the same length
	if ruTokens <= enTokens/2 {
		t.Errorf("Cyrillic text should have more tokens, got ru=%d en=%d", ruTokens, enTokens)
	}
}

func TestEstimateMessagesTokens(t *testing.T) {
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg("Привет"),
		provider.AssistantText("Здравствуй!"),
	}

	tokens := EstimateMessagesTokens(messages)
	if tokens <= 0 {
		t.Error("EstimateMessagesTokens should return positive number")
	}

	// System prompt + 2 messages = at least a few tokens
	if tokens < 5 {
		t.Errorf("Expected at least 5 tokens, got %d", tokens)
	}
}

func TestCompactContext_NoCompactionNeeded(t *testing.T) {
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg("Привет"),
		provider.AssistantText("Здравствуй!"),
	}

	// Large limit — no compaction needed
	result := CompactContext(messages, 10000, 2)
	if len(result) != len(messages) {
		t.Errorf("Expected %d messages, got %d", len(messages), len(result))
	}
}

func TestCompactContext_CompactionNeeded(t *testing.T) {
	var messages []provider.Message
	messages = append(messages, provider.SystemMsg("Ты помощник"))

	// Add 20 messages
	for i := 0; i < 20; i++ {
		messages = append(messages, provider.UserMsg("Сообщение номер которое достаточно длинное чтобы занять токены"))
		messages = append(messages, provider.AssistantText("Ответ на сообщение который тоже занимает некоторое количество токенов в контексте"))
	}

	// Limit of 200 tokens — compaction should happen
	result := CompactContext(messages, 200, 4)

	// The result should be shorter than the original
	if len(result) >= len(messages) {
		t.Errorf("Expected compaction, got %d messages (original: %d)", len(result), len(messages))
	}

	// The system prompt should be preserved
	if result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}
}

func TestCompactContext_SystemPromptPreserved(t *testing.T) {
	messages := []provider.Message{
		provider.SystemMsg("Важный системный промпт"),
		provider.UserMsg("Сообщение 1"),
		provider.AssistantText("Ответ 1"),
		provider.UserMsg("Сообщение 2"),
		provider.AssistantText("Ответ 2"),
	}

	// Very small limit
	result := CompactContext(messages, 10, 2)

	// The system prompt should be first
	if len(result) == 0 || result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}
}

func TestCompactContext_KeepRecent(t *testing.T) {
	var messages []provider.Message
	messages = append(messages, provider.SystemMsg("Ты помощник"))

	for i := 0; i < 10; i++ {
		messages = append(messages, provider.UserMsg("Длинное сообщение для заполнения контекста номер который занимает много токенов"))
	}

	// Compaction with keepRecent=3
	result := CompactContext(messages, 200, 3)

	// The system prompt should be preserved
	if len(result) == 0 || result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}

	// The result should be shorter than the original
	if len(result) >= len(messages) {
		t.Errorf("Expected compaction, got %d messages (original: %d)", len(result), len(messages))
	}

	// The last keepRecent messages should be in the result
	lastOriginal := messages[len(messages)-3:]
	for _, msg := range lastOriginal {
		found := false
		for _, r := range result {
			if r.Role == msg.Role && r.GetText() == msg.GetText() {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected recent message %q to be preserved", msg.GetText()[:30])
		}
	}
}

func TestSummarizeMessages(t *testing.T) {
	i18n.Init("en") // Initialize i18n for compaction labels
	messages := []provider.Message{
		provider.UserMsg("Первый вопрос пользователю"),
		provider.AssistantText("Первый ответ ассистента"),
		provider.UserMsg("Второй вопрос"),
		provider.AssistantText("Второй ответ"),
	}

	summary := SimpleSummarize(messages, 500)
	if summary == "" {
		t.Error("SummarizeMessages should return non-empty string")
	}
	if !compactionContains(summary, "User") && !compactionContains(summary, "Пользователь") {
		t.Error("Summary should contain user label")
	}
	if !compactionContains(summary, "Assistant") && !compactionContains(summary, "Ассистент") {
		t.Error("Summary should contain assistant label")
	}
}

func TestNewConversationContextWithTokens(t *testing.T) {
	ctx := NewConversationContextWithTokens(4000, 4)
	if ctx.MaxTokens != 4000 {
		t.Errorf("Expected MaxTokens=4000, got %d", ctx.MaxTokens)
	}
	if ctx.KeepRecent != 4 {
		t.Errorf("Expected KeepRecent=4, got %d", ctx.KeepRecent)
	}
}

func TestConversationContext_TokenCount(t *testing.T) {
	ctx := NewConversationContext(50)
	ctx.Add(provider.SystemMsg("Ты помощник"))
	ctx.Add(provider.UserMsg("Привет"))

	count := ctx.TokenCount()
	if count <= 0 {
		t.Error("TokenCount should return positive number")
	}
}

func TestConversationContext_TokenBasedTrim(t *testing.T) {
	ctx := NewConversationContextWithTokens(100, 2)
	ctx.Add(provider.SystemMsg("Ты помощник"))

	// Add many messages
	for i := 0; i < 20; i++ {
		ctx.Add(provider.UserMsg("Длинное сообщение для заполнения контекста"))
		ctx.Add(provider.AssistantText("Длинный ответ для заполнения контекста"))
	}

	// Context should be trimmed
	if len(ctx.Messages) >= 41 {
		t.Errorf("Expected compaction, got %d messages", len(ctx.Messages))
	}

	// The system prompt should be preserved
	if ctx.Messages[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}
}

func compactionContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestExtractRecaps_SingleRecap(t *testing.T) {
	messages := []provider.Message{
		provider.UserMsg("Сделай рефакторинг"),
		provider.AssistantText("Готово!\n\n※ Recap: Рефакторинг функции main завершён"),
		provider.UserMsg("Теперь добавь тесты"),
		provider.AssistantText("Тесты добавлены"),
	}

	recaps := extractRecaps(messages)
	if len(recaps) != 1 {
		t.Errorf("Expected 1 recap, got %d", len(recaps))
	}
	if recaps[0] != "Рефакторинг функции main завершён" {
		t.Errorf("Expected recap text, got: %q", recaps[0])
	}
}

func TestExtractRecaps_MultipleRecaps(t *testing.T) {
	messages := []provider.Message{
		provider.AssistantText("Done\n\n※ Recap: Fixed bug in parser"),
		provider.UserMsg("Add tests"),
		provider.AssistantText("Tests added\n\n※ Recap: Added 5 unit tests for parser"),
	}

	recaps := extractRecaps(messages)
	if len(recaps) != 2 {
		t.Errorf("Expected 2 recaps, got %d", len(recaps))
	}
	if recaps[0] != "Fixed bug in parser" {
		t.Errorf("Expected first recap, got: %q", recaps[0])
	}
	if recaps[1] != "Added 5 unit tests for parser" {
		t.Errorf("Expected second recap, got: %q", recaps[1])
	}
}

func TestExtractRecaps_NoRecaps(t *testing.T) {
	messages := []provider.Message{
		provider.UserMsg("Привет"),
		provider.AssistantText("Здравствуй!"),
	}

	recaps := extractRecaps(messages)
	if len(recaps) != 0 {
		t.Errorf("Expected 0 recaps, got %d", len(recaps))
	}
}

func TestExtractRecaps_RecapWithMultiline(t *testing.T) {
	messages := []provider.Message{
		provider.AssistantText("Done\n\n※ Recap: Fixed bug\nMore text after recap"),
	}

	recaps := extractRecaps(messages)
	if len(recaps) != 1 {
		t.Errorf("Expected 1 recap, got %d", len(recaps))
	}
	// Recap should contain only the text up to \n
	if recaps[0] != "Fixed bug" {
		t.Errorf("Expected 'Fixed bug', got: %q", recaps[0])
	}
}

func TestExtractRecaps_SkipsNonAssistant(t *testing.T) {
	messages := []provider.Message{
		provider.SystemMsg("※ Recap: should be ignored"),
		provider.UserMsg("※ Recap: also ignored"),
		provider.AssistantText("※ Recap: this one counts"),
	}

	recaps := extractRecaps(messages)
	if len(recaps) != 1 {
		t.Errorf("Expected 1 recap (only from assistant), got %d", len(recaps))
	}
	if recaps[0] != "this one counts" {
		t.Errorf("Expected 'this one counts', got: %q", recaps[0])
	}
}

func TestCompactContext_PreservesRecaps(t *testing.T) {
	i18n.Init("en")

	var messages []provider.Message
	messages = append(messages, provider.SystemMsg("You are a helper"))

	// Add 20 messages, some with recaps
	for i := 0; i < 10; i++ {
		messages = append(messages, provider.UserMsg("Сообщение номер которое достаточно длинное чтобы занять токены"))
		if i == 3 {
			messages = append(messages, provider.AssistantText("Ответ с рекапом\n\n※ Recap: Рефакторинг завершён"))
		} else if i == 7 {
			messages = append(messages, provider.AssistantText("Другой ответ\n\n※ Recap: Тесты добавлены"))
		} else {
			messages = append(messages, provider.AssistantText("Ответ на сообщение который тоже занимает некоторое количество токенов в контексте"))
		}
	}

	// Compaction with a limit sufficient for summary + recent + recap
	result := CompactContext(messages, 600, 4)

	// Verify that recaps are preserved
	hasRecap := false
	for _, m := range result {
		text := m.GetText()
		if strings.Contains(text, "Рефакторинг завершён") || strings.Contains(text, "Тесты добавлены") {
			hasRecap = true
		}
	}
	if !hasRecap {
		t.Error("Expected recaps to be preserved after compaction")
	}

	// The system prompt should be preserved
	if result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}
}

func TestBuildRecapMessage(t *testing.T) {
	i18n.Init("en")

	recaps := []string{"Fixed parser bug", "Added 5 tests"}
	msg := buildRecapMessage(recaps)

	if msg.Role != "system" {
		t.Errorf("Expected system role, got: %s", msg.Role)
	}
	text := msg.GetText()
	if !strings.Contains(text, "Fixed parser bug") {
		t.Errorf("Expected recap text in message, got: %q", text)
	}
	if !strings.Contains(text, "Added 5 tests") {
		t.Errorf("Expected recap text in message, got: %q", text)
	}
}

func TestCompactContext_SystemMsgsExceedMaxTokens(t *testing.T) {
	// If system messages themselves exceed the limit,
	// only system messages should be returned without panic
	longSystem := provider.SystemMsg(strings.Repeat("very important system instruction ", 100))
	messages := []provider.Message{
		longSystem,
		provider.UserMsg("Hello"),
		provider.AssistantText("Hi"),
	}

	// Very small limit — the system message is definitely larger
	result := CompactContext(messages, 10, 2)

	// Only system messages should remain
	if len(result) == 0 {
		t.Fatal("Expected at least system messages")
	}
	for _, m := range result {
		if m.Role != "system" {
			t.Errorf("Expected only system messages when system exceeds maxTokens, got role=%s", m.Role)
		}
	}
}

func TestCompactContext_NoDuplicateSystemMsgs(t *testing.T) {
	i18n.Init("en")

	// Create messages with a recap in the assistant message
	messages := []provider.Message{
		provider.SystemMsg("You are a helper"),
		provider.UserMsg("Do something"),
		provider.AssistantText("Done\n\n※ Recap: Fixed bug"),
		provider.UserMsg("Next"),
		provider.AssistantText("Done too"),
	}

	// Compaction with keepRecent=2, the limit allows fitting summary + recap
	result := CompactContext(messages, 500, 2)

	// Count system messages
	systemCount := 0
	for _, m := range result {
		if m.Role == "system" {
			systemCount++
		}
	}
	// Expect: original system prompt + summary + recap = 3 system messages maximum
	// But definitely not 4+ (which would indicate duplication)
	if systemCount > 3 {
		t.Errorf("Expected at most 3 system messages, got %d (possible duplication)", systemCount)
	}
}

func TestCompactContext_RecapRespectsTokenLimit(t *testing.T) {
	i18n.Init("en")

	// Create a very long recap
	longRecap := strings.Repeat("a", 1000)
	messages := []provider.Message{
		provider.SystemMsg("Helper"),
		provider.UserMsg("Task 1"),
		provider.AssistantText("Done\n\n※ Recap: " + longRecap),
		provider.UserMsg("Task 2"),
		provider.AssistantText("Done"),
	}

	// Small limit — the recap definitely will not fit
	result := CompactContext(messages, 50, 2)

	// Verify that the recap is NOT added (otherwise it would exceed the limit)
	totalTokens := EstimateMessagesTokens(result)
	if totalTokens > 100 { // generous upper bound
		t.Errorf("Expected compact result to respect token limit, got %d tokens", totalTokens)
	}
}

// --- Tests for new priority compaction functions ---

func TestStripToolResults(t *testing.T) {
	msg := provider.Message{
		Role: "assistant",
		Content: []provider.ContentBlock{
			{Type: "text", Text: "Результат: "},
			{Type: "tool_result", ToolName: "bash", Output: "очень длинный вывод команды..."},
			{Type: "text", Text: "Готово"},
		},
	}

	stripped := stripToolResults(msg)

	// Should be 3 blocks (tool_result replaced with a short one)
	if len(stripped.Content) != 3 {
		t.Errorf("Expected 3 blocks, got %d", len(stripped.Content))
	}

	// tool_result should be replaced with "[output truncated]"
	for _, block := range stripped.Content {
		if block.Type == "tool_result" {
			if block.Output != "[output truncated]" {
				t.Errorf("Expected '[output truncated]', got %q", block.Output)
			}
		}
	}

	// Text blocks should not change
	if stripped.Content[0].Text != "Результат: " {
		t.Errorf("Expected first text block preserved, got %q", stripped.Content[0].Text)
	}
	if stripped.Content[2].Text != "Готово" {
		t.Errorf("Expected last text block preserved, got %q", stripped.Content[2].Text)
	}
}

func TestStripToolResults_NoToolResults(t *testing.T) {
	msg := provider.Message{
		Role: "user",
		Content: []provider.ContentBlock{
			{Type: "text", Text: "Привет"},
		},
	}

	stripped := stripToolResults(msg)
	if len(stripped.Content) != 1 {
		t.Errorf("Expected 1 block, got %d", len(stripped.Content))
	}
	if stripped.Content[0].Text != "Привет" {
		t.Errorf("Expected text preserved, got %q", stripped.Content[0].Text)
	}
}

func TestStripToolCallsFromMessage(t *testing.T) {
	msg := provider.Message{
		Role: "assistant",
		Content: []provider.ContentBlock{
			{Type: "thinking", Text: "Думаю..."},
			{Type: "text", Text: "Результат: "},
			{Type: "tool_use", ToolName: "bash", Input: map[string]any{"command": "ls"}},
			{Type: "tool_result", ToolName: "bash", Output: "file1.txt\nfile2.txt"},
			{Type: "text", Text: "Готово"},
		},
	}

	stripped := stripToolCalls(msg)

	// Only thinking and text should remain
	if len(stripped.Content) != 3 {
		t.Errorf("Expected 3 blocks (thinking + 2 text), got %d", len(stripped.Content))
	}

	for _, block := range stripped.Content {
		if block.Type == "tool_use" || block.Type == "tool_result" {
			t.Errorf("Expected no tool blocks, found %s", block.Type)
		}
	}
}

func TestStripToolCallsFromMessage_AllToolBlocks(t *testing.T) {
	// Message with only tool blocks — should become empty
	msg := provider.Message{
		Role: "assistant",
		Content: []provider.ContentBlock{
			{Type: "tool_use", ToolName: "bash", Input: map[string]any{"command": "ls"}},
			{Type: "tool_result", ToolName: "bash", Output: "output"},
		},
	}

	stripped := stripToolCalls(msg)
	if !isEmptyMessage(stripped) {
		t.Error("Expected empty message after stripping all tool blocks")
	}
}

func TestIsEmptyMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      provider.Message
		expected bool
	}{
		{
			name:     "system message never empty",
			msg:      provider.SystemMsg(""),
			expected: false,
		},
		{
			name:     "text message with content",
			msg:      provider.UserMsg("Привет"),
			expected: false,
		},
		{
			name:     "text message empty",
			msg:      provider.UserMsg(""),
			expected: true,
		},
		{
			name:     "whitespace only",
			msg:      provider.UserMsg("   \t\n  "),
			expected: true,
		},
		{
			name: "tool_use block",
			msg: provider.Message{
				Role: "assistant",
				Content: []provider.ContentBlock{
					{Type: "tool_use", ToolName: "bash"},
				},
			},
			expected: false,
		},
		{
			name: "thinking block with content",
			msg: provider.Message{
				Role: "assistant",
				Content: []provider.ContentBlock{
					{Type: "thinking", Text: "Размышления"},
				},
			},
			expected: false,
		},
		{
			name: "thinking block empty",
			msg: provider.Message{
				Role: "assistant",
				Content: []provider.ContentBlock{
					{Type: "thinking", Text: "  "},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEmptyMessage(tt.msg)
			if result != tt.expected {
				t.Errorf("isEmptyMessage() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCompactByPriority_Phase1(t *testing.T) {
	// Phase 1: tool_result truncation should reduce tokens
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg("Запусти команду"),
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "text", Text: "Выполняю"},
				{Type: "tool_use", ToolName: "bash", Input: map[string]any{"command": "ls"}},
				{Type: "tool_result", ToolName: "bash", Output: strings.Repeat("file\n", 100)},
			},
		},
	}

	originalTokens := EstimateMessagesTokens(messages)
	result := compactByPriority(messages, originalTokens+100)
	// Without compaction — should return as-is
	if len(result) != len(messages) {
		t.Errorf("Expected no compaction when tokens fit, got %d messages", len(result))
	}

	// With a small limit — tool_result should be truncated
	result = compactByPriority(messages, 100)
	// The result should be shorter in tokens
	resultTokens := EstimateMessagesTokens(result)
	if resultTokens >= originalTokens {
		t.Errorf("Expected fewer tokens after compaction, got %d (original: %d)", resultTokens, originalTokens)
	}

	// The system prompt should be preserved
	if len(result) == 0 || result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}
}

func TestCompactByPriority_Phase2(t *testing.T) {
	// Phase 2: remove tool_use and tool_result, keep thinking/text
	// Create many unique messages so phase 1 (tool_result truncation) does not help
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
	}
	for i := 0; i < 5; i++ {
		messages = append(messages, provider.UserMsg(fmt.Sprintf("Запусти команду %d: "+strings.Repeat("x ", 10), i)))
		messages = append(messages, provider.Message{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "thinking", Text: fmt.Sprintf("Размышляю над задачей %d", i)},
				{Type: "text", Text: fmt.Sprintf("Выполняю команду %d", i)},
				{Type: "tool_use", ToolName: "bash", Input: map[string]any{"command": "ls"}},
				{Type: "tool_result", ToolName: "bash", Output: strings.Repeat("file\n", 50)},
			},
		})
	}

	// A limit where phase 1 (tool_result truncation) does not help —
	// even with truncated tool_result, text + tool_use do not fit
	result := compactByPriority(messages, 150)

	// There should be no tool_use or tool_result in the result
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_use" || block.Type == "tool_result" {
				t.Errorf("Expected no tool blocks after phase 2, found %s", block.Type)
			}
		}
	}

	// The system prompt should be preserved
	if len(result) == 0 || result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}
}

func TestCompactByPriority_Phase3(t *testing.T) {
	// Phase 3: remove old messages
	var messages []provider.Message
	messages = append(messages, provider.SystemMsg("Ты помощник"))
	for i := 0; i < 20; i++ {
		messages = append(messages, provider.UserMsg("Длинное сообщение номер которое занимает много токенов в контексте"))
	}

	// Very small limit — only the last messages should remain
	result := compactByPriority(messages, 100)

	// The system prompt should be preserved
	if len(result) == 0 || result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}

	// The result should be shorter than the original
	if len(result) >= len(messages) {
		t.Errorf("Expected compaction, got %d messages (original: %d)", len(result), len(messages))
	}
}

func TestCompactByPriority_Phase4(t *testing.T) {
	// Phase 4: only the system prompt
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg(strings.Repeat("очень длинное сообщение ", 100)),
	}

	// Limit of 10 tokens — only the system prompt
	result := compactByPriority(messages, 10)

	if len(result) != 1 || result[0].Role != "system" {
		t.Errorf("Expected only system message, got %d messages", len(result))
	}
}

func TestCompactByPriority_NoCompactionNeeded(t *testing.T) {
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg("Привет"),
		provider.AssistantText("Здравствуй!"),
	}

	result := compactByPriority(messages, 10000)
	if len(result) != len(messages) {
		t.Errorf("Expected no compaction, got %d messages (original: %d)", len(result), len(messages))
	}
}

func TestCompactContext_PriorityOrder(t *testing.T) {
	// Verify that tool_result is compacted before text
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg("Запусти ls"),
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "text", Text: "Выполняю команду"},
				{Type: "tool_use", ToolName: "bash", Input: map[string]any{"command": "ls -la"}},
				{Type: "tool_result", ToolName: "bash", Output: strings.Repeat("file\n", 50)},
			},
		},
		provider.UserMsg("Теперь запусти pwd"),
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "text", Text: "Текущая директория"},
				{Type: "tool_use", ToolName: "bash", Input: map[string]any{"command": "pwd"}},
				{Type: "tool_result", ToolName: "bash", Output: "/home/user"},
			},
		},
	}

	// Medium limit — tool_result should be truncated, but text preserved
	result := CompactContext(messages, 200, 2)

	// The system prompt should be preserved
	if len(result) == 0 || result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}

	// Text blocks should be preserved
	hasText := false
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "text" && block.Text != "" {
				hasText = true
			}
		}
	}
	if !hasText {
		t.Error("Expected text blocks to be preserved after priority compaction")
	}
}

func TestRemoveToolErrors(t *testing.T) {
	// Error tool_result and the matching tool_use should be removed
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg("Запусти команду"),
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "text", Text: "Выполняю"},
				{Type: "tool_use", ToolName: "bash", ToolUseID: "call_123", Input: map[string]any{"command": "ls"}},
			},
		},
		{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "tool_result", ToolName: "bash", ToolUseID: "call_123", Output: "command not found", IsError: true},
			},
		},
		provider.AssistantText("Попробую другую команду"),
	}

	result := RemoveToolErrors(messages)

	// Error tool_result should be removed
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_result" && block.IsError {
				t.Error("Error tool_result should be removed")
			}
		}
	}

	// The matching tool_use should also be removed
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_use" && block.ToolUseID == "call_123" {
				t.Error("tool_use matching error tool_result should be removed")
			}
		}
	}

	// Text messages should be preserved
	hasText := false
	for _, msg := range result {
		if msg.Role == "assistant" {
			for _, block := range msg.Content {
				if block.Type == "text" && block.Text != "" {
					hasText = true
				}
			}
		}
	}
	if !hasText {
		t.Error("Text blocks should be preserved")
	}
}

func TestRemoveToolErrors_NoErrors(t *testing.T) {
	// If there are no errors — nothing is removed
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg("Привет"),
		provider.AssistantText("Здравствуй!"),
	}

	result := RemoveToolErrors(messages)
	if len(result) != len(messages) {
		t.Errorf("Expected %d messages, got %d", len(messages), len(result))
	}
}

func TestRemoveToolErrors_MixedErrors(t *testing.T) {
	// Error and success tool_result — only the error one is removed
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "tool_use", ToolName: "bash", ToolUseID: "call_err", Input: map[string]any{"command": "bad_cmd"}},
			},
		},
		{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "tool_result", ToolName: "bash", ToolUseID: "call_err", Output: "error", IsError: true},
			},
		},
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "tool_use", ToolName: "bash", ToolUseID: "call_ok", Input: map[string]any{"command": "ls"}},
			},
		},
		{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "tool_result", ToolName: "bash", ToolUseID: "call_ok", Output: "file1.txt\nfile2.txt", IsError: false},
			},
		},
	}

	result := RemoveToolErrors(messages)

	// Successful tool_result should be preserved
	hasSuccessResult := false
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_result" && block.ToolUseID == "call_ok" {
				hasSuccessResult = true
			}
		}
	}
	if !hasSuccessResult {
		t.Error("Successful tool_result should be preserved")
	}

	// Error tool_result should be removed
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_result" && block.IsError {
				t.Error("Error tool_result should be removed")
			}
		}
	}
}

func TestRemoveDuplicates(t *testing.T) {
	// Duplicate assistant messages should be removed, the last one is kept
	// User messages are NOT removed even if duplicates
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg("Привет"),
		provider.AssistantText("Здравствуй!"),
		provider.UserMsg("Привет"),            // дубликат user — НЕ удаляется
		provider.AssistantText("Здравствуй!"), // дубликат assistant — удаляется
		provider.UserMsg("Как дела?"),
	}

	result := RemoveDuplicates(messages)

	// Should remain 5 messages: system + Привет + Здравствуй! + Привет + Как дела?
	// (duplicate assistant "Здравствуй!" removed, but user "Привет" kept)
	userCount := 0
	for _, m := range result {
		if m.Role == "user" {
			userCount++
		}
	}
	if userCount != 3 {
		t.Errorf("Expected 3 user messages (Привет + Привет + Как дела?), got %d", userCount)
	}

	// Assistant duplicates should be removed (1 remains)
	assistantCount := 0
	for _, m := range result {
		if m.Role == "assistant" {
			assistantCount++
		}
	}
	if assistantCount != 1 {
		t.Errorf("Expected 1 assistant message, got %d", assistantCount)
	}
}

func TestRemoveDuplicates_EmptyMessages(t *testing.T) {
	// Empty system and assistant messages should be removed
	// Empty user messages are NOT removed (may contain tool_result)
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg(""),
		provider.AssistantText(""),
		provider.UserMsg("Реальный вопрос"),
	}

	result := RemoveDuplicates(messages)

	// Empty system and assistant messages should be removed
	// Empty user messages are kept (may contain tool_result)
	for _, m := range result {
		if m.Role == "assistant" && m.GetResponseText() == "" {
			t.Errorf("Empty assistant message should be removed")
		}
	}
}

func TestRemoveDuplicates_NoDuplicates(t *testing.T) {
	// If there are no duplicates — nothing is removed
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg("Вопрос 1"),
		provider.AssistantText("Ответ 1"),
		provider.UserMsg("Вопрос 2"),
	}

	result := RemoveDuplicates(messages)
	if len(result) != len(messages) {
		t.Errorf("Expected %d messages, got %d", len(messages), len(result))
	}
}

func TestCompactByPriority_RemovesErrorsAndDuplicates(t *testing.T) {
	// Verify that phases 0a and 0b work in compactByPriority
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg("Привет"),
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "tool_use", ToolName: "bash", ToolUseID: "call_err", Input: map[string]any{"command": "bad_cmd"}},
			},
		},
		{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "tool_result", ToolName: "bash", ToolUseID: "call_err", Output: "command not found", IsError: true},
			},
		},
		provider.AssistantText("Ошибка!"),
		provider.UserMsg("Привет"),        // дубликат
		provider.AssistantText("Ошибка!"), // дубликат
	}

	// Large limit — no compaction needed, but errors and duplicates should be removed
	result := compactByPriority(messages, 10000)

	// Error tool_result should be removed
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_result" && block.IsError {
				t.Error("Error tool_result should be removed in phase 0a")
			}
		}
	}

	// The matching tool_use should also be removed
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_use" && block.ToolUseID == "call_err" {
				t.Error("tool_use matching error tool_result should be removed in phase 0a")
			}
		}
	}

	// Assistant duplicates should be removed, user duplicates are kept
	userMsgs := 0
	for _, msg := range result {
		if msg.Role == "user" && msg.GetResponseText() == "Привет" {
			userMsgs++
		}
	}
	if userMsgs != 2 {
		t.Errorf("Expected 2 'Привет' user messages (duplicates preserved), got %d", userMsgs)
	}
}

// --- Tests for new compaction functions ---

func TestTruncateStringLines(t *testing.T) {
	// Short text — without truncation
	short := "line1\nline2\nline3"
	result := truncateStringLines(short, 5, 5)
	if result != short {
		t.Errorf("Short text should not be truncated, got: %q", result)
	}

	// Long text — with truncation
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i)
	}
	long := strings.Join(lines, "\n")
	result = truncateStringLines(long, 3, 3)
	if !strings.Contains(result, "14 lines truncated") {
		t.Errorf("Expected truncation marker, got: %q", result)
	}
	if !strings.Contains(result, "line0") || !strings.Contains(result, "line19") {
		t.Errorf("Expected head and tail preserved, got: %q", result)
	}
	if strings.Contains(result, "line10") {
		t.Errorf("Middle lines should be truncated, got: %q", result)
	}
}

func TestTruncateToolOutputs_PreservesMetadata(t *testing.T) {
	msg := provider.Message{
		Role: "user",
		Content: []provider.ContentBlock{
			{Type: "tool_result", ToolUseID: "call_1", ToolName: "bash", Output: strings.Repeat("output line\n", 50), IsError: false},
		},
	}
	result := truncateToolOutputs(msg)
	for _, block := range result.Content {
		if block.Type == "tool_result" {
			if block.ToolUseID != "call_1" {
				t.Errorf("ToolUseID not preserved, got %q", block.ToolUseID)
			}
			if block.ToolName != "bash" {
				t.Errorf("ToolName not preserved, got %q", block.ToolName)
			}
			if block.IsError != false {
				t.Errorf("IsError not preserved")
			}
			if !strings.Contains(block.Output, "lines truncated") {
				t.Errorf("Expected truncation marker in output, got: %q", block.Output[:100])
			}
		}
	}
}

func TestTruncateToolOutputs_ShortOutput(t *testing.T) {
	msg := provider.Message{
		Role: "user",
		Content: []provider.ContentBlock{
			{Type: "tool_result", ToolUseID: "call_2", ToolName: "bash", Output: "short output"},
		},
	}
	result := truncateToolOutputs(msg)
	for _, block := range result.Content {
		if block.Type == "tool_result" && block.Output != "short output" {
			t.Errorf("Short output should not be truncated, got: %q", block.Output)
		}
	}
}

func TestTruncateToolArgs(t *testing.T) {
	longScript := strings.Repeat("echo hello; ", 100)
	msg := provider.Message{
		Role: "assistant",
		Content: []provider.ContentBlock{
			{Type: "text", Text: "Running script"},
			{Type: "tool_use", ToolUseID: "call_1", ToolName: "bash", Input: map[string]any{
				"command": longScript,
				"timeout": 30,
			}},
		},
	}

	result := truncateToolArgs(msg, MaxToolArgChars)
	for _, block := range result.Content {
		if block.Type == "tool_use" {
			cmd, _ := block.Input["command"].(string)
			if len(cmd) > MaxToolArgChars+20 {
				t.Errorf("Command should be truncated, got %d chars", len(cmd))
			}
			if !strings.Contains(cmd, "...[truncated]") {
				t.Errorf("Truncated command should contain marker, got: %q", cmd[:50])
			}
			timeout := block.Input["timeout"]
			if timeout != 30 {
				t.Errorf("Non-string values should be preserved, got: %v", timeout)
			}
		}
	}
}

func TestTruncateToolArgs_ShortArgs(t *testing.T) {
	msg := provider.Message{
		Role: "assistant",
		Content: []provider.ContentBlock{
			{Type: "tool_use", ToolUseID: "call_1", ToolName: "bash", Input: map[string]any{
				"command": "ls -la",
			}},
		},
	}

	result := truncateToolArgs(msg, MaxToolArgChars)
	for _, block := range result.Content {
		if block.Type == "tool_use" {
			cmd, _ := block.Input["command"].(string)
			if cmd != "ls -la" {
				t.Errorf("Short args should not be truncated, got: %q", cmd)
			}
		}
	}
}

func TestTruncateToolArgs_NoToolUse(t *testing.T) {
	msg := provider.UserMsg("Hello world")
	result := truncateToolArgs(msg, MaxToolArgChars)
	if result.GetResponseText() != msg.GetResponseText() {
		t.Error("Messages without tool_use should not be modified")
	}
}

func TestEnsureToolPairIntegrity_RemovesOrphanedToolResult(t *testing.T) {
	messages := []provider.Message{
		provider.SystemMsg("system"),
		{Role: "user", Content: []provider.ContentBlock{
			{Type: "tool_result", ToolUseID: "orphan_1", ToolName: "bash", Output: "orphaned result"},
		}},
		provider.AssistantText("done"),
	}
	result := EnsureToolPairIntegrity(messages)
	// Orphaned tool_result in user message should be converted to text block
	found := false
	for _, msg := range result {
		if msg.Role == "user" {
			for _, block := range msg.Content {
				if block.Type == "text" && strings.Contains(block.Text, "orphaned result") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("Orphaned tool_result in user message should be converted to text block")
	}
	// tool_result block should be removed
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_result" && block.ToolUseID == "orphan_1" {
				t.Error("Orphaned tool_result block should be removed")
			}
		}
	}
}

func TestEnsureToolPairIntegrity_RemovesOrphanedToolUse(t *testing.T) {
	messages := []provider.Message{
		provider.SystemMsg("system"),
		{Role: "assistant", Content: []provider.ContentBlock{
			{Type: "tool_use", ToolUseID: "orphan_2", ToolName: "bash", Input: map[string]any{"command": "ls"}},
		}},
		provider.AssistantText("done"),
	}
	result := EnsureToolPairIntegrity(messages)
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_use" && block.ToolUseID == "orphan_2" {
				t.Error("Orphaned tool_use should be removed")
			}
		}
	}
}

func TestEnsureToolPairIntegrity_PreservesPairedBlocks(t *testing.T) {
	messages := []provider.Message{
		{Role: "assistant", Content: []provider.ContentBlock{
			{Type: "tool_use", ToolUseID: "call_ok", ToolName: "bash", Input: map[string]any{"command": "ls"}},
		}},
		{Role: "user", Content: []provider.ContentBlock{
			{Type: "tool_result", ToolUseID: "call_ok", ToolName: "bash", Output: "file.txt"},
		}},
	}
	result := EnsureToolPairIntegrity(messages)
	hasUse := false
	hasResult := false
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_use" && block.ToolUseID == "call_ok" {
				hasUse = true
			}
			if block.Type == "tool_result" && block.ToolUseID == "call_ok" {
				hasResult = true
			}
		}
	}
	if !hasUse || !hasResult {
		t.Error("Paired tool_use/tool_result should be preserved")
	}
}

func TestEnsureToolPairIntegrity_NoOrphans(t *testing.T) {
	messages := []provider.Message{
		provider.SystemMsg("system"),
		provider.UserMsg("hello"),
	}
	result := EnsureToolPairIntegrity(messages)
	if len(result) != len(messages) {
		t.Errorf("Expected %d messages, got %d", len(messages), len(result))
	}
}

func TestExtractExistingSummary(t *testing.T) {
	i18n.Init("en")
	messages := []provider.Message{
		provider.SystemMsg("You are a helper"),
		{Role: "system", Content: []provider.ContentBlock{
			{Type: "text", Text: "[Previous context summary]\nUser asked about Go. Assistant explained goroutines."},
		}},
		provider.UserMsg("Next question"),
	}
	summary := extractExistingSummary(messages)
	if !strings.Contains(summary, "User asked about Go") {
		t.Errorf("Expected to extract summary, got: %q", summary)
	}
}

func TestExtractExistingSummary_NoSummary(t *testing.T) {
	i18n.Init("en")
	messages := []provider.Message{
		provider.SystemMsg("You are a helper"),
		provider.UserMsg("Hello"),
	}
	summary := extractExistingSummary(messages)
	if summary != "" {
		t.Errorf("Expected empty summary, got: %q", summary)
	}
}

func TestAntiThrashing_SkipsAutoCompact(t *testing.T) {
	// Auto-compact (via trim/compact) should skip when lowSaveCount >= 2
	ctx := NewConversationContextWithTokens(100, 2)
	ctx.Add(provider.SystemMsg("system"))
	ctx.Add(provider.UserMsg("hello"))

	// Simulate 2 ineffective compactions
	ctx.lowSaveCount = 2

	tokensBefore := ctx.TokenCount()
	ctx.compact() // внутренний метод — auto-compact
	// Compaction should be skipped
	if ctx.TokenCount() != tokensBefore {
		t.Error("Auto-compact should be skipped when lowSaveCount >= 2 and tokens <= 1.5*MaxTokens")
	}
}

func TestManualCompactBypassesAntiThrashing(t *testing.T) {
	// Manual compaction (Compact) should bypass anti-thrashing
	ctx := NewConversationContextWithTokens(100, 2)
	ctx.AutoCompact = false // отключаем авто-компакцию при Add()
	ctx.Add(provider.SystemMsg("system"))

	// Add many messages to exceed the limit
	for i := 0; i < 50; i++ {
		ctx.Add(provider.UserMsg(fmt.Sprintf("message %d with some content to make it longer than usual", i)))
		ctx.Add(provider.AssistantText(fmt.Sprintf("response %d with some content to make it longer", i)))
	}

	// Simulate 2 ineffective compactions — anti-thrashing should block auto-compact
	ctx.lowSaveCount = 2

	tokensBefore := ctx.TokenCount()
	// Compact() (manual compaction) should reset lowSaveCount and perform compaction
	ctx.Compact()
	tokensAfter := ctx.TokenCount()

	// Compaction should have run and reduced tokens
	if tokensAfter >= tokensBefore {
		t.Errorf("Manual Compact() should bypass anti-thrashing and reduce tokens: before=%d after=%d", tokensBefore, tokensAfter)
	}
}

func TestAntiThrashing_ResetsOnEffectiveCompaction(t *testing.T) {
	// lowSaveCount is reset inside compact() when compaction is effective (>10% savings)
	// and NOT at Add() — this allows anti-thrashing to work correctly
	ctx := NewConversationContextWithTokens(100, 2)
	ctx.Add(provider.SystemMsg("system"))

	// Add many messages to exceed the limit
	for i := 0; i < 20; i++ {
		ctx.Add(provider.UserMsg(fmt.Sprintf("message %d with some content to make it longer", i)))
		ctx.Add(provider.AssistantText(fmt.Sprintf("response %d with some content", i)))
	}

	// lowSaveCount should be 0 after an effective compaction
	if ctx.lowSaveCount != 0 {
		t.Errorf("Expected lowSaveCount=0 after effective compaction, got %d", ctx.lowSaveCount)
	}
}

func TestTruncateAssistantText(t *testing.T) {
	// Short message — not truncated
	msg := provider.Message{
		Role: "assistant",
		Content: []provider.ContentBlock{
			{Type: "text", Text: "short message"},
		},
	}
	result := truncateAssistantText(msg)
	if result.Content[0].Text != "short message" {
		t.Errorf("Short message should not be truncated, got: %s", result.Content[0].Text)
	}

	// Long message — truncated
	longText := strings.Repeat("a", 5000)
	msg = provider.Message{
		Role: "assistant",
		Content: []provider.ContentBlock{
			{Type: "text", Text: longText},
		},
	}
	result = truncateAssistantText(msg)
	resultText := result.Content[0].Text
	if len(resultText) >= len(longText) {
		t.Errorf("Long message should be truncated, got length %d >= %d", len(resultText), len(longText))
	}
	if !strings.Contains(resultText, "chars truncated") {
		t.Errorf("Truncated message should contain truncation marker, got: %s", resultText[:100])
	}
	// Should contain the beginning and end of the original
	if !strings.HasPrefix(resultText, strings.Repeat("a", 500)) {
		t.Error("Truncated message should start with original head")
	}
	if !strings.HasSuffix(resultText, strings.Repeat("a", 500)) {
		t.Error("Truncated message should end with original tail")
	}

	// User message — not truncated
	msg = provider.Message{
		Role: "user",
		Content: []provider.ContentBlock{
			{Type: "text", Text: longText},
		},
	}
	result = truncateAssistantText(msg)
	if result.Content[0].Text != longText {
		t.Error("User message should not be truncated")
	}
}

func TestTruncateAssistantText_MultipleBlocks(t *testing.T) {
	// Message with multiple blocks — only text is truncated
	longText := strings.Repeat("x", 3000)
	msg := provider.Message{
		Role: "assistant",
		Content: []provider.ContentBlock{
			{Type: "thinking", Text: "short thinking"},
			{Type: "text", Text: longText},
		},
	}
	result := truncateAssistantText(msg)
	// thinking should not be truncated (truncateThinking does that)
	if result.Content[0].Text != "short thinking" {
		t.Error("Thinking block should not be truncated by truncateAssistantText")
	}
	// text should be truncated
	if len(result.Content[1].Text) >= len(longText) {
		t.Error("Text block should be truncated")
	}
}

func TestRemoveDuplicates_SemanticDedup(t *testing.T) {
	// 5 messages with the same start — 2 should remain
	messages := []provider.Message{
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Все задачи выполнены! Вот итоговая сводка:\n\n1. Сделано А\n2. Сделано Б"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Все задачи выполнены! Вот итоговая сводка:\n\n1. Сделано В\n2. Сделано Г"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Все задачи выполнены! Вот итоговая сводка:\n\n1. Сделано Д\n2. Сделано Е"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Все задачи выполнены! Вот итоговая сводка:\n\n1. Сделано Ж\n2. Сделано З"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Все задачи выполнены! Вот итоговая сводка:\n\n1. Сделано И\n2. Сделано К"}}},
	}

	result := RemoveDuplicates(messages)
	// Should remain 2 messages (the last 2 with the same prefix)
	if len(result) > 2 {
		t.Errorf("Expected at most 2 messages with same prefix, got %d", len(result))
	}
	if len(result) < 1 {
		t.Errorf("Expected at least 1 message, got %d", len(result))
	}
}

func TestRemoveDuplicates_TwoSimilar(t *testing.T) {
	// 2 messages with the same start — both should remain
	messages := []provider.Message{
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Начну с анализа проблемы. Шаг 1: читаю файл."}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Начну с анализа проблемы. Шаг 2: исправляю код."}}},
	}

	result := RemoveDuplicates(messages)
	if len(result) != 2 {
		t.Errorf("Expected 2 messages (only 2 similar, not 3+), got %d", len(result))
	}
}

func TestCompactByPriority_Phase1c(t *testing.T) {
	// Test: phase 1c truncates long assistant messages
	longText := strings.Repeat("This is a long assistant message. ", 200) // ~8800 chars
	messages := []provider.Message{
		{Role: "system", Content: []provider.ContentBlock{{Type: "text", Text: "system prompt"}}},
		{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hello"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: longText}}},
	}

	// Small MaxTokens to trigger compaction
	result := compactByPriority(messages, 500)
	resultTokens := EstimateMessagesTokens(result)

	// The result should be smaller than the original
	originalTokens := EstimateMessagesTokens(messages)
	if resultTokens >= originalTokens {
		t.Errorf("Expected compaction to reduce tokens: %d -> %d", originalTokens, resultTokens)
	}

	// The result should contain the assistant message (truncated)
	hasAssistant := false
	for _, msg := range result {
		if msg.Role == "assistant" {
			hasAssistant = true
			// Verify that the text is truncated
			for _, block := range msg.Content {
				if block.Type == "text" && strings.Contains(block.Text, "chars truncated") {
					// OK — text truncated
				}
			}
		}
	}
	if !hasAssistant {
		t.Error("Expected assistant message to be preserved (truncated)")
	}
}

func TestDynamicKeepRecent(t *testing.T) {
	// Test: dynamic keepRecent shrinks if the last messages are too large
	longText := strings.Repeat("This is a very long message. ", 500) // ~22000 chars
	messages := []provider.Message{
		{Role: "system", Content: []provider.ContentBlock{{Type: "text", Text: "system"}}},
		{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "msg1"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "short1"}}},
		{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "msg2"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: longText}}},
		{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "msg3"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "short2"}}},
	}

	// CompactContext with keepRecent=6, but small maxTokens
	result := CompactContext(messages, 500, 6)

	// The result should be compact
	resultTokens := EstimateMessagesTokens(result)
	if resultTokens > 600 { // небольшой запас
		t.Errorf("Expected compact result, got %d tokens", resultTokens)
	}
}

func TestTruncateMessageToFit(t *testing.T) {
	// Message with long tool_result — truncated
	longOutput := strings.Repeat("line\n", 100)
	msg := provider.Message{
		Role: "user",
		Content: []provider.ContentBlock{
			{Type: "tool_result", ToolUseID: "1", ToolName: "bash", Output: longOutput},
		},
	}
	result := truncateMessageToFit(msg, 200)
	resultTokens := EstimateMessagesTokens([]provider.Message{result})
	if resultTokens > 250 {
		t.Errorf("Expected < 250 tokens after truncation, got %d", resultTokens)
	}

	// Message with long assistant text — truncated
	longText := strings.Repeat("word ", 5000)
	msg = provider.AssistantText(longText)
	result = truncateMessageToFit(msg, 200)
	resultTokens = EstimateMessagesTokens([]provider.Message{result})
	if resultTokens > 250 {
		t.Errorf("Expected < 250 tokens after truncation, got %d", resultTokens)
	}

	// Short message — not truncated
	msg = provider.UserMsg("hello")
	result = truncateMessageToFit(msg, 500)
	if result.Content[0].Text != "hello" {
		t.Errorf("Short message should not be truncated, got: %s", result.Content[0].Text)
	}
}

func TestCompactContext_FallbackTruncatesLargeMessage(t *testing.T) {
	// When a single message exceeds maxTokens, compaction should truncate it
	// instead of returning only system messages
	longText := strings.Repeat("This is a very long message that should be truncated. ", 500)
	messages := []provider.Message{
		provider.SystemMsg("system prompt"),
		provider.UserMsg("short question"),
		provider.AssistantText(longText),
		provider.UserMsg("another question"),
	}

	// maxTokens = 500 — very small limit
	result := CompactContext(messages, 500, 2)

	// The result should contain more than just the system prompt
	resultTokens := EstimateMessagesTokens(result)
	if resultTokens == 0 {
		t.Error("CompactContext should not return empty result")
	}

	// The result should fit within the limit (with a small margin)
	if resultTokens > 700 {
		t.Errorf("Expected result to fit in ~500 tokens, got %d", resultTokens)
	}

	// The result should contain the system prompt
	hasSystem := false
	for _, m := range result {
		if m.Role == "system" {
			hasSystem = true
		}
	}
	if !hasSystem {
		t.Error("Result should contain system prompt")
	}
}

func TestStripToolCalls_PreservesUserToolResults(t *testing.T) {
	msg := provider.Message{
		Role: "user",
		Content: []provider.ContentBlock{
			{Type: "tool_result", ToolUseID: "tool1", Output: "result1"},
			{Type: "tool_result", ToolUseID: "tool2", Output: "result2"},
		},
	}
	result := stripToolCalls(msg)
	if len(result.Content) != 2 {
		t.Errorf("Expected 2 tool_result blocks preserved, got %d", len(result.Content))
	}
	for _, block := range result.Content {
		if block.Type != "tool_result" {
			t.Errorf("Expected tool_result block, got %s", block.Type)
		}
	}
}

func TestStripToolCalls_RemovesAssistantToolCalls(t *testing.T) {
	msg := provider.Message{
		Role: "assistant",
		Content: []provider.ContentBlock{
			{Type: "text", Text: "Hello"},
			{Type: "tool_use", ToolUseID: "tool1", ToolName: "bash"},
			{Type: "tool_result", ToolUseID: "tool1", Output: "result1"},
		},
	}
	result := stripToolCalls(msg)
	if len(result.Content) != 1 {
		t.Errorf("Expected 1 text block, got %d", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Errorf("Expected text block, got %s", result.Content[0].Type)
	}
}

func TestCompactByPriority_PreservesUserMessages(t *testing.T) {
	// Create messages where user messages contain tool_result blocks
	messages := []provider.Message{
		{Role: "system", Content: []provider.ContentBlock{{Type: "text", Text: "system prompt"}}},
		{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hello"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "hi there"}}},
		{Role: "user", Content: []provider.ContentBlock{
			{Type: "tool_result", ToolUseID: "tool1", Output: "result1"},
		}},
		{Role: "assistant", Content: []provider.ContentBlock{
			{Type: "tool_use", ToolUseID: "tool1", ToolName: "bash", Input: map[string]any{"command": "ls"}},
		}},
		{Role: "user", Content: []provider.ContentBlock{
			{Type: "tool_result", ToolUseID: "tool1", Output: "file1.txt\nfile2.txt"},
		}},
	}

	// Compact with large budget — should preserve all user messages
	result := compactByPriority(messages, 5000)
	
	// Count user messages
	userCount := 0
	for _, msg := range result {
		if msg.Role == "user" {
			userCount++
		}
	}
	if userCount < 2 {
		t.Errorf("Expected at least 2 user messages preserved, got %d", userCount)
	}
}

func TestRemoveDuplicates_NeverSkipsUserMessages(t *testing.T) {
	messages := []provider.Message{
		{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "продолжай"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "OK"}}},
		{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "продолжай"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Done"}}},
	}
	result := RemoveDuplicates(messages)
	userCount := 0
	for _, msg := range result {
		if msg.Role == "user" {
			userCount++
		}
	}
	if userCount != 2 {
		t.Errorf("Expected 2 user messages (duplicates should be kept), got %d", userCount)
	}
}

func TestRemoveDuplicates_KeepsUserToolResults(t *testing.T) {
	messages := []provider.Message{
		{Role: "user", Content: []provider.ContentBlock{
			{Type: "tool_result", ToolUseID: "tool1", Output: "result1"},
		}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "OK"}}},
	}
	result := RemoveDuplicates(messages)
	if len(result) != 2 {
		t.Errorf("Expected 2 messages (user with tool_result should be kept), got %d", len(result))
	}
}
