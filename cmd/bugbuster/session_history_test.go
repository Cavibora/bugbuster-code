package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"bugbuster-code/pkg/config"
	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
	"bugbuster-code/pkg/theme"
)

func init() {
	// Инициализируем i18n и тему для тестов
	i18n.Init("en")
	appTheme = theme.ResolveTheme(config.ThemeConfig{Mode: "dark"})
}

// stripANSI убирает ANSI escape-коды из строки для проверки содержимого
func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}

// captureOutput перенаправляет stdout и возвращает перехваченный вывод
func captureOutput(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	buf := make([]byte, 16384)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

func TestRenderSessionHistoryCLI_BasicConversation(t *testing.T) {
	messages := []provider.Message{
		provider.SystemMsg("system prompt"),
		provider.UserMsg("Hello"),
		provider.AssistantText("Hi there!"),
		provider.UserMsg("How are you?"),
		provider.AssistantText("I'm fine, thanks!"),
	}

	result := captureOutput(func() {
		renderSessionHistoryCLI(messages)
	})

	// Системное сообщение не должно отображаться
	if strings.Contains(result, "system prompt") {
		t.Error("system message should not be rendered")
	}

	// Сообщения пользователя должны быть
	if !strings.Contains(result, "❯ Hello") {
		t.Error("user message 'Hello' should be rendered")
	}
	if !strings.Contains(result, "❯ How are you?") {
		t.Error("user message 'How are you?' should be rendered")
	}

	// Ответы ассистента должны быть (glamour оборачивает в ANSI, проверяем без них)
	plain := stripANSI(result)
	if !strings.Contains(plain, "Hi there!") {
		t.Error("assistant text 'Hi there!' should be rendered")
	}
	if !strings.Contains(plain, "I'm fine, thanks!") {
		t.Error("assistant text should be rendered")
	}
}

func TestRenderSessionHistoryCLI_ToolCalls(t *testing.T) {
	messages := []provider.Message{
		provider.UserMsg("Read the file"),
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "tool_use", ToolName: "read", ToolUseID: "tool_1", Input: map[string]any{"path": "/tmp/test.go"}},
			},
		},
		{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "tool_result", ToolUseID: "tool_1", ToolName: "read", Output: "package main\n\nfunc main() {\n}\n", IsError: false},
			},
		},
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "text", Text: "Here is the file content."},
			},
		},
	}

	result := captureOutput(func() {
		renderSessionHistoryCLI(messages)
	})

	// Должен быть вызов инструмента
	if !strings.Contains(result, "⏺") {
		t.Error("tool call indicator should be rendered")
	}
	if !strings.Contains(result, "read") {
		t.Error("tool name 'read' should be rendered")
	}

	// Должен быть результат инструмента
	if !strings.Contains(result, "read") {
		t.Error("tool result should contain tool name 'read'")
	}

	// tool_result в user-сообщении не должен рендерить ❯
	userMsgCount := strings.Count(result, "❯")
	if userMsgCount != 1 {
		t.Errorf("expected 1 user message indicator, got %d", userMsgCount)
	}
}

func TestRenderSessionHistoryCLI_ToolError(t *testing.T) {
	messages := []provider.Message{
		provider.UserMsg("Read missing file"),
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "tool_use", ToolName: "read", ToolUseID: "tool_1", Input: map[string]any{"path": "/nonexistent"}},
			},
		},
		{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "tool_result", ToolUseID: "tool_1", ToolName: "read", Output: "file not found", IsError: true},
			},
		},
	}

	result := captureOutput(func() {
		renderSessionHistoryCLI(messages)
	})

	// Проверяем что ошибка рендерится
	if !strings.Contains(result, "file not found") {
		t.Error("tool error output should be rendered")
	}
}

func TestRenderSessionHistoryCLI_ThinkingBlock(t *testing.T) {
	messages := []provider.Message{
		provider.UserMsg("Think about it"),
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "thinking", Text: "Let me analyze this..."},
				{Type: "text", Text: "Here is my analysis."},
			},
		},
	}

	result := captureOutput(func() {
		renderSessionHistoryCLI(messages)
	})

	if !strings.Contains(result, "Thinking") {
		t.Error("thinking indicator should be rendered")
	}
	if !strings.Contains(result, "Let me analyze this...") {
		t.Error("thinking text should be rendered")
	}
	plain := stripANSI(result)
	if !strings.Contains(plain, "Here is my analysis.") {
		t.Error("response text should be rendered")
	}
}

func TestRenderSessionHistoryCLI_Empty(t *testing.T) {
	// Пустые сообщения — просто не должно быть паники
	renderSessionHistoryCLI(nil)

	// Только системное сообщение
	messages := []provider.Message{provider.SystemMsg("system")}
	renderSessionHistoryCLI(messages)
}

func TestWrapTextCLI(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		indent   int
		maxCols  int
		expected string
	}{
		{
			name:     "short line",
			text:     "hello",
			indent:   2,
			maxCols:  80,
			expected: "  hello",
		},
		{
			name:     "exact fit",
			text:     "hello",
			indent:   2,
			maxCols:  7,
			expected: "  hello",
		},
		{
			name:     "wrap long line",
			text:     "abcdefghij",
			indent:   2,
			maxCols:  7,
			expected: "  abcde\n  fghij",
		},
		{
			name:     "multiline",
			text:     "hello\nworld",
			indent:   2,
			maxCols:  80,
			expected: "  hello\n  world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapTextCLI(tt.text, tt.indent, tt.maxCols)
			if result != tt.expected {
				t.Errorf("wrapTextCLI(%q, %d, %d) = %q, want %q",
					tt.text, tt.indent, tt.maxCols, result, tt.expected)
			}
		})
	}
}

func TestIsToolResultOnly(t *testing.T) {
	tests := []struct {
		name     string
		msg      provider.Message
		expected bool
	}{
		{
			name:     "pure tool_result",
			msg:      provider.Message{Role: "user", Content: []provider.ContentBlock{{Type: "tool_result"}}},
			expected: true,
		},
		{
			name:     "user text",
			msg:      provider.UserMsg("hello"),
			expected: false,
		},
		{
			name:     "empty content",
			msg:      provider.Message{Role: "user", Content: []provider.ContentBlock{}},
			expected: false,
		},
		{
			name:     "mixed content",
			msg:      provider.Message{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}, {Type: "tool_result"}}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isToolResultOnly(tt.msg); got != tt.expected {
				t.Errorf("isToolResultOnly() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRenderSessionHistoryCLI_MultipleToolCalls(t *testing.T) {
	messages := []provider.Message{
		provider.UserMsg("Read two files"),
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "tool_use", ToolName: "read", ToolUseID: "t1", Input: map[string]any{"path": "/a.go"}},
				{Type: "tool_use", ToolName: "read", ToolUseID: "t2", Input: map[string]any{"path": "/b.go"}},
			},
		},
		{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "tool_result", ToolUseID: "t1", ToolName: "read", Output: "package a\n\nfunc A() {}\n"},
				{Type: "tool_result", ToolUseID: "t2", ToolName: "read", Output: "package b\n\nfunc B() {}\n"},
			},
		},
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "text", Text: "Both files are similar."},
			},
		},
	}

	result := captureOutput(func() {
		renderSessionHistoryCLI(messages)
	})

	// Должны быть оба файла
	if !strings.Contains(result, "/a.go") {
		t.Error("first file path should be rendered")
	}
	if !strings.Contains(result, "/b.go") {
		t.Error("second file path should be rendered")
	}
	plain := stripANSI(result)
	if !strings.Contains(plain, "Both files are similar.") {
		t.Error("final assistant text should be rendered")
	}
}