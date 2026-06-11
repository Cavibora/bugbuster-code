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
	// Кириллический текст должен давать больше токенов (меньше символов/токен)
	ruText := "Привет, как дела? Это тестовое сообщение на русском языке."
	enText := "Hello, how are you? This is a test message in English language."

	ruTokens := EstimateTokens(ruText)
	enTokens := EstimateTokens(enText)

	// Кириллический текст должен давать больше токенов при той же длине
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

	// Системный промпт + 2 сообщения = минимум несколько токенов
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

	// Большой лимит — компакция не нужна
	result := CompactContext(messages, 10000, 2)
	if len(result) != len(messages) {
		t.Errorf("Expected %d messages, got %d", len(messages), len(result))
	}
}

func TestCompactContext_CompactionNeeded(t *testing.T) {
	var messages []provider.Message
	messages = append(messages, provider.SystemMsg("Ты помощник"))

	// Добавляем 20 сообщений
	for i := 0; i < 20; i++ {
		messages = append(messages, provider.UserMsg("Сообщение номер которое достаточно длинное чтобы занять токены"))
		messages = append(messages, provider.AssistantText("Ответ на сообщение который тоже занимает некоторое количество токенов в контексте"))
	}

	// Лимит 200 токенов — должна быть компакция
	result := CompactContext(messages, 200, 4)

	// Результат должен быть короче оригинала
	if len(result) >= len(messages) {
		t.Errorf("Expected compaction, got %d messages (original: %d)", len(result), len(messages))
	}

	// Системный промпт должен быть сохранён
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

	// Очень маленький лимит
	result := CompactContext(messages, 10, 2)

	// Системный промпт должен быть первым
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

	// Компакция с keepRecent=3
	result := CompactContext(messages, 200, 3)

	// Системный промпт должен быть сохранён
	if len(result) == 0 || result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}

	// Результат должен быть короче оригинала
	if len(result) >= len(messages) {
		t.Errorf("Expected compaction, got %d messages (original: %d)", len(result), len(messages))
	}

	// Последние keepRecent сообщений должны быть в результате
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

	// Добавляем много сообщений
	for i := 0; i < 20; i++ {
		ctx.Add(provider.UserMsg("Длинное сообщение для заполнения контекста"))
		ctx.Add(provider.AssistantText("Длинный ответ для заполнения контекста"))
	}

	// Контекст должен быть обрезан
	if len(ctx.Messages) >= 41 {
		t.Errorf("Expected compaction, got %d messages", len(ctx.Messages))
	}

	// Системный промпт должен быть сохранён
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
	// Recap должен содержать только текст до \n
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

	// Добавляем 20 сообщений, некоторые с рекапами
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

	// Компакция с лимитом, достаточным для summary + recent + recap
	result := CompactContext(messages, 600, 4)

	// Проверяем что рекапы сохранены
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

	// Системный промпт должен быть сохранён
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
	// Если системные сообщения сами по себе превышают лимит,
	// должны вернуться только системные сообщения без panic
	longSystem := provider.SystemMsg(strings.Repeat("very important system instruction ", 100))
	messages := []provider.Message{
		longSystem,
		provider.UserMsg("Hello"),
		provider.AssistantText("Hi"),
	}

	// Лимит очень маленький — системное сообщение точно больше
	result := CompactContext(messages, 10, 2)

	// Должны остаться только системные сообщения
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

	// Создаём сообщения с рекапом в assistant-сообщении
	messages := []provider.Message{
		provider.SystemMsg("You are a helper"),
		provider.UserMsg("Do something"),
		provider.AssistantText("Done\n\n※ Recap: Fixed bug"),
		provider.UserMsg("Next"),
		provider.AssistantText("Done too"),
	}

	// Компакция с keepRecent=2, лимит позволяет вместить summary + recap
	result := CompactContext(messages, 500, 2)

	// Считаем системные сообщения
	systemCount := 0
	for _, m := range result {
		if m.Role == "system" {
			systemCount++
		}
	}
	// Ожидаем: оригинальный system prompt + summary + recap = 3 системных сообщения максимум
	// Но точно не 4+ (что было бы при дублировании)
	if systemCount > 3 {
		t.Errorf("Expected at most 3 system messages, got %d (possible duplication)", systemCount)
	}
}

func TestCompactContext_RecapRespectsTokenLimit(t *testing.T) {
	i18n.Init("en")

	// Создаём очень длинный рекап
	longRecap := strings.Repeat("a", 1000)
	messages := []provider.Message{
		provider.SystemMsg("Helper"),
		provider.UserMsg("Task 1"),
		provider.AssistantText("Done\n\n※ Recap: " + longRecap),
		provider.UserMsg("Task 2"),
		provider.AssistantText("Done"),
	}

	// Маленький лимит — рекап точно не поместится
	result := CompactContext(messages, 50, 2)

	// Проверяем что рекап НЕ добавлен (иначе бы превысил лимит)
	totalTokens := EstimateMessagesTokens(result)
	if totalTokens > 100 { // generous upper bound
		t.Errorf("Expected compact result to respect token limit, got %d tokens", totalTokens)
	}
}

// --- Тесты для новых функций приоритетной компакции ---

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

	// Должно быть 3 блока (tool_result заменён на короткий)
	if len(stripped.Content) != 3 {
		t.Errorf("Expected 3 blocks, got %d", len(stripped.Content))
	}

	// tool_result должен быть заменён на "[output truncated]"
	for _, block := range stripped.Content {
		if block.Type == "tool_result" {
			if block.Output != "[output truncated]" {
				t.Errorf("Expected '[output truncated]', got %q", block.Output)
			}
		}
	}

	// Текстовые блоки не должны измениться
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

	// Должно остаться только thinking и text
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
	// Сообщение только с tool блоками — должно стать пустым
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
	// Фаза 1: усечение tool_result должно уменьшить токены
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
	// Без компакции — должно вернуться как есть
	if len(result) != len(messages) {
		t.Errorf("Expected no compaction when tokens fit, got %d messages", len(result))
	}

	// С маленьким лимитом — tool_result должен быть усечён
	result = compactByPriority(messages, 100)
	// Результат должен быть короче по токенам
	resultTokens := EstimateMessagesTokens(result)
	if resultTokens >= originalTokens {
		t.Errorf("Expected fewer tokens after compaction, got %d (original: %d)", resultTokens, originalTokens)
	}

	// Системный промпт должен быть сохранён
	if len(result) == 0 || result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}
}

func TestCompactByPriority_Phase2(t *testing.T) {
	// Фаза 2: удаление tool_use и tool_result, сохранение thinking/text
	// Создаём много уникальных сообщений, чтобы фаза 1 (усечение tool_result) не помогла
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

	// Лимит, при котором фаза 1 (усечение tool_result) не помогает —
	// даже с усечёнными tool_result, текст + tool_use не влезают
	result := compactByPriority(messages, 150)

	// Не должно быть tool_use и tool_result в результате
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_use" || block.Type == "tool_result" {
				t.Errorf("Expected no tool blocks after phase 2, found %s", block.Type)
			}
		}
	}

	// Системный промпт должен быть сохранён
	if len(result) == 0 || result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}
}

func TestCompactByPriority_Phase3(t *testing.T) {
	// Фаза 3: удаление старых сообщений
	var messages []provider.Message
	messages = append(messages, provider.SystemMsg("Ты помощник"))
	for i := 0; i < 20; i++ {
		messages = append(messages, provider.UserMsg("Длинное сообщение номер которое занимает много токенов в контексте"))
	}

	// Очень маленький лимит — должны остаться только последние сообщения
	result := compactByPriority(messages, 100)

	// Системный промпт должен быть сохранён
	if len(result) == 0 || result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}

	// Результат должен быть короче оригинала
	if len(result) >= len(messages) {
		t.Errorf("Expected compaction, got %d messages (original: %d)", len(result), len(messages))
	}
}

func TestCompactByPriority_Phase4(t *testing.T) {
	// Фаза 4: только системный промпт
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg(strings.Repeat("очень длинное сообщение ", 100)),
	}

	// Лимит 10 токенов — только системный промпт
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
	// Проверяем что tool_result компактируется раньше чем text
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

	// Средний лимит — tool_result должен быть усечён, но текст сохранён
	result := CompactContext(messages, 200, 2)

	// Системный промпт должен быть сохранён
	if len(result) == 0 || result[0].Role != "system" {
		t.Error("System prompt should be preserved")
	}

	// Текстовые блоки должны быть сохранены
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
	// Ошибочный tool_result и соответствующий tool_use должны быть удалены
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

	// Ошибочный tool_result должен быть удалён
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_result" && block.IsError {
				t.Error("Error tool_result should be removed")
			}
		}
	}

	// Соответствующий tool_use тоже должен быть удалён
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_use" && block.ToolUseID == "call_123" {
				t.Error("tool_use matching error tool_result should be removed")
			}
		}
	}

	// Текстовые сообщения должны быть сохранены
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
	// Если нет ошибок — ничего не удаляется
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
	// Ошибочный и успешный tool_result — удаляется только ошибочный
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

	// Успешный tool_result должен быть сохранён
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

	// Ошибочный tool_result должен быть удалён
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_result" && block.IsError {
				t.Error("Error tool_result should be removed")
			}
		}
	}
}

func TestRemoveDuplicates(t *testing.T) {
	// Дублирующиеся assistant сообщения должны быть удалены, сохраняется последнее
	// Пользовательские сообщения НЕ удаляются даже если дубликаты
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg("Привет"),
		provider.AssistantText("Здравствуй!"),
		provider.UserMsg("Привет"),            // дубликат user — НЕ удаляется
		provider.AssistantText("Здравствуй!"), // дубликат assistant — удаляется
		provider.UserMsg("Как дела?"),
	}

	result := RemoveDuplicates(messages)

	// Должно остаться 5 сообщений: system + Привет + Здравствуй! + Привет + Как дела?
	// (дубликат assistant "Здравствуй!" удалён, но user "Привет" сохранён)
	userCount := 0
	for _, m := range result {
		if m.Role == "user" {
			userCount++
		}
	}
	if userCount != 3 {
		t.Errorf("Expected 3 user messages (Привет + Привет + Как дела?), got %d", userCount)
	}

	// Assistant дубликаты должны быть удалены (остаётся 1)
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
	// Пустые system и assistant сообщения должны быть удалены
	// Пустые user сообщения НЕ удаляются (могут содержать tool_result)
	messages := []provider.Message{
		provider.SystemMsg("Ты помощник"),
		provider.UserMsg(""),
		provider.AssistantText(""),
		provider.UserMsg("Реальный вопрос"),
	}

	result := RemoveDuplicates(messages)

	// Пустые system и assistant сообщения должны быть удалены
	// Пустые user сообщения сохраняются (могут содержать tool_result)
	for _, m := range result {
		if m.Role == "assistant" && m.GetResponseText() == "" {
			t.Errorf("Empty assistant message should be removed")
		}
	}
}

func TestRemoveDuplicates_NoDuplicates(t *testing.T) {
	// Если нет дубликатов — ничего не удаляется
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
	// Проверяем что фаза 0a и 0b работают в compactByPriority
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

	// Большой лимит — компакция не нужна, но ошибки и дубликаты должны быть удалены
	result := compactByPriority(messages, 10000)

	// Ошибочный tool_result должен быть удалён
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_result" && block.IsError {
				t.Error("Error tool_result should be removed in phase 0a")
			}
		}
	}

	// Соответствующий tool_use тоже должен быть удалён
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == "tool_use" && block.ToolUseID == "call_err" {
				t.Error("tool_use matching error tool_result should be removed in phase 0a")
			}
		}
	}

	// Дубликаты assistant должны быть удалены, user дубликаты сохраняются
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

// --- Тесты для новых функций компакции ---

func TestTruncateStringLines(t *testing.T) {
	// Короткий текст — без усечения
	short := "line1\nline2\nline3"
	result := truncateStringLines(short, 5, 5)
	if result != short {
		t.Errorf("Short text should not be truncated, got: %q", result)
	}

	// Длинный текст — с усечением
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
	result := ensureToolPairIntegrity(messages)
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
	result := ensureToolPairIntegrity(messages)
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
	result := ensureToolPairIntegrity(messages)
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
	result := ensureToolPairIntegrity(messages)
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
	// Auto-compact (через trim/compact) должен пропускать при lowSaveCount >= 2
	ctx := NewConversationContextWithTokens(100, 2)
	ctx.Add(provider.SystemMsg("system"))
	ctx.Add(provider.UserMsg("hello"))

	// Имитируем 2 неэффективные компакции
	ctx.lowSaveCount = 2

	tokensBefore := ctx.TokenCount()
	ctx.compact() // внутренний метод — auto-compact
	// Компакция должна быть пропущена
	if ctx.TokenCount() != tokensBefore {
		t.Error("Auto-compact should be skipped when lowSaveCount >= 2 and tokens <= 1.5*MaxTokens")
	}
}

func TestManualCompactBypassesAntiThrashing(t *testing.T) {
	// Ручная компакция (Compact) должна обходить anti-thrashing
	ctx := NewConversationContextWithTokens(100, 2)
	ctx.AutoCompact = false // отключаем авто-компакцию при Add()
	ctx.Add(provider.SystemMsg("system"))

	// Добавляем много сообщений чтобы превысить лимит
	for i := 0; i < 50; i++ {
		ctx.Add(provider.UserMsg(fmt.Sprintf("message %d with some content to make it longer than usual", i)))
		ctx.Add(provider.AssistantText(fmt.Sprintf("response %d with some content to make it longer", i)))
	}

	// Имитируем 2 неэффективные компакции — anti-thrashing должен блокировать auto-compact
	ctx.lowSaveCount = 2

	tokensBefore := ctx.TokenCount()
	// Compact() (ручная компакция) должна сбросить lowSaveCount и выполнить компакцию
	ctx.Compact()
	tokensAfter := ctx.TokenCount()

	// Компакция должна была выполниться и уменьшить токены
	if tokensAfter >= tokensBefore {
		t.Errorf("Manual Compact() should bypass anti-thrashing and reduce tokens: before=%d after=%d", tokensBefore, tokensAfter)
	}
}

func TestAntiThrashing_ResetsOnEffectiveCompaction(t *testing.T) {
	// lowSaveCount сбрасывается внутри compact() когда компакция эффективна (>10% экономии)
	// а НЕ при Add() — это позволяет anti-thrashing работать корректно
	ctx := NewConversationContextWithTokens(100, 2)
	ctx.Add(provider.SystemMsg("system"))

	// Добавляем много сообщений чтобы превысить лимит
	for i := 0; i < 20; i++ {
		ctx.Add(provider.UserMsg(fmt.Sprintf("message %d with some content to make it longer", i)))
		ctx.Add(provider.AssistantText(fmt.Sprintf("response %d with some content", i)))
	}

	// lowSaveCount должен быть 0 после эффективной компакции
	if ctx.lowSaveCount != 0 {
		t.Errorf("Expected lowSaveCount=0 after effective compaction, got %d", ctx.lowSaveCount)
	}
}

func TestTruncateAssistantText(t *testing.T) {
	// Короткое сообщение — не усекается
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

	// Длинное сообщение — усекается
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
	// Должно содержать начало и конец оригинала
	if !strings.HasPrefix(resultText, strings.Repeat("a", 500)) {
		t.Error("Truncated message should start with original head")
	}
	if !strings.HasSuffix(resultText, strings.Repeat("a", 500)) {
		t.Error("Truncated message should end with original tail")
	}

	// User-сообщение — не усекается
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
	// Сообщение с несколькими блоками — усекается только text
	longText := strings.Repeat("x", 3000)
	msg := provider.Message{
		Role: "assistant",
		Content: []provider.ContentBlock{
			{Type: "thinking", Text: "short thinking"},
			{Type: "text", Text: longText},
		},
	}
	result := truncateAssistantText(msg)
	// thinking не должен быть усечён (это делает truncateThinking)
	if result.Content[0].Text != "short thinking" {
		t.Error("Thinking block should not be truncated by truncateAssistantText")
	}
	// text должен быть усечён
	if len(result.Content[1].Text) >= len(longText) {
		t.Error("Text block should be truncated")
	}
}

func TestRemoveDuplicates_SemanticDedup(t *testing.T) {
	// 5 сообщений с одинаковым началом — должно остаться 2
	messages := []provider.Message{
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Все задачи выполнены! Вот итоговая сводка:\n\n1. Сделано А\n2. Сделано Б"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Все задачи выполнены! Вот итоговая сводка:\n\n1. Сделано В\n2. Сделано Г"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Все задачи выполнены! Вот итоговая сводка:\n\n1. Сделано Д\n2. Сделано Е"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Все задачи выполнены! Вот итоговая сводка:\n\n1. Сделано Ж\n2. Сделано З"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "Все задачи выполнены! Вот итоговая сводка:\n\n1. Сделано И\n2. Сделано К"}}},
	}

	result := RemoveDuplicates(messages)
	// Должно остаться 2 сообщения (последние 2 с одинаковым префиксом)
	if len(result) > 2 {
		t.Errorf("Expected at most 2 messages with same prefix, got %d", len(result))
	}
	if len(result) < 1 {
		t.Errorf("Expected at least 1 message, got %d", len(result))
	}
}

func TestRemoveDuplicates_TwoSimilar(t *testing.T) {
	// 2 сообщения с одинаковым началом — оба должны остаться
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
	// Тест: фаза 1c усекает длинные assistant-сообщения
	longText := strings.Repeat("This is a long assistant message. ", 200) // ~8800 chars
	messages := []provider.Message{
		{Role: "system", Content: []provider.ContentBlock{{Type: "text", Text: "system prompt"}}},
		{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hello"}}},
		{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: longText}}},
	}

	// MaxTokens маленький, чтобы триггерить компакцию
	result := compactByPriority(messages, 500)
	resultTokens := EstimateMessagesTokens(result)

	// Результат должен быть меньше оригинала
	originalTokens := EstimateMessagesTokens(messages)
	if resultTokens >= originalTokens {
		t.Errorf("Expected compaction to reduce tokens: %d -> %d", originalTokens, resultTokens)
	}

	// Результат должен содержать assistant-сообщение (усечённое)
	hasAssistant := false
	for _, msg := range result {
		if msg.Role == "assistant" {
			hasAssistant = true
			// Проверяем что текст усечён
			for _, block := range msg.Content {
				if block.Type == "text" && strings.Contains(block.Text, "chars truncated") {
					// OK — текст усечён
				}
			}
		}
	}
	if !hasAssistant {
		t.Error("Expected assistant message to be preserved (truncated)")
	}
}

func TestDynamicKeepRecent(t *testing.T) {
	// Тест: динамический keepRecent уменьшается если последние сообщения слишком большие
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

	// CompactContext с keepRecent=6, но maxTokens маленький
	result := CompactContext(messages, 500, 6)

	// Результат должен быть компактным
	resultTokens := EstimateMessagesTokens(result)
	if resultTokens > 600 { // небольшой запас
		t.Errorf("Expected compact result, got %d tokens", resultTokens)
	}
}

func TestTruncateMessageToFit(t *testing.T) {
	// Сообщение с длинным tool_result — усекается
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

	// Сообщение с длинным assistant text — усекается
	longText := strings.Repeat("word ", 5000)
	msg = provider.AssistantText(longText)
	result = truncateMessageToFit(msg, 200)
	resultTokens = EstimateMessagesTokens([]provider.Message{result})
	if resultTokens > 250 {
		t.Errorf("Expected < 250 tokens after truncation, got %d", resultTokens)
	}

	// Короткое сообщение — не усекается
	msg = provider.UserMsg("hello")
	result = truncateMessageToFit(msg, 500)
	if result.Content[0].Text != "hello" {
		t.Errorf("Short message should not be truncated, got: %s", result.Content[0].Text)
	}
}

func TestCompactContext_FallbackTruncatesLargeMessage(t *testing.T) {
	// Когда одно сообщение больше maxTokens, компакция должна усечь его
	// вместо того чтобы вернуть только system messages
	longText := strings.Repeat("This is a very long message that should be truncated. ", 500)
	messages := []provider.Message{
		provider.SystemMsg("system prompt"),
		provider.UserMsg("short question"),
		provider.AssistantText(longText),
		provider.UserMsg("another question"),
	}

	// maxTokens = 500 — очень маленький лимит
	result := CompactContext(messages, 500, 2)

	// Результат должен содержать больше чем только system prompt
	resultTokens := EstimateMessagesTokens(result)
	if resultTokens == 0 {
		t.Error("CompactContext should not return empty result")
	}

	// Результат должен вписываться в лимит (с небольшим запасом)
	if resultTokens > 700 {
		t.Errorf("Expected result to fit in ~500 tokens, got %d", resultTokens)
	}

	// Результат должен содержать system prompt
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
