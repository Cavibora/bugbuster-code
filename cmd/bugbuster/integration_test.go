package main

import (
	"strings"
	"testing"
	"time"

	"bugbuster-code/pkg/config"
	"bugbuster-code/pkg/theme"
)

// TestMain инициализирует тему для тестов
func TestMain(m *testing.M) {
	appTheme = theme.ResolveTheme(config.ThemeConfig{Mode: "dark", WordWrap: 80})
	m.Run()
}

func TestGlamourRendererStreamingCodeBlockIntegration(t *testing.T) {
	gr := NewGlamourRenderer()

	// Симулируем полный поток: текст → код → текст
	gr.Render("Here is some code:\n")
	gr.Render("```go\n")
	gr.Render("func main() {\n")
	gr.Render("    fmt.Println(\"hello\")\n")
	gr.Render("}\n")
	gr.Render("```\n")
	gr.Render("Done!\n")

	result := gr.Flush()

	// Glamour рендерит кодовый блок — должен содержать код
	if !strings.Contains(result, "hello") {
		t.Error("Code block content missing")
	}
	// Glamour может разбивать текст на отдельные ANSI-фрагменты,
	// поэтому проверяем "Done" без "!"
	if !strings.Contains(result, "Done") {
		t.Error("Text after code block missing")
	}
}

func TestGlamourRendererMultipleCodeBlocks(t *testing.T) {
	gr := NewGlamourRenderer()

	input := "```\ncode1\n```\ntext\n```\ncode2\n```\n"
	gr.Render(input)
	result := gr.Flush()

	// Glamour должен отрендерить оба блока
	if !strings.Contains(result, "code1") {
		t.Errorf("First code block missing, got: %q", result)
	}
	if !strings.Contains(result, "code2") {
		t.Errorf("Second code block missing, got: %q", result)
	}
}

func TestGlamourRendererInlineCode(t *testing.T) {
	gr := NewGlamourRenderer()

	input := "Use `fmt.Println` to print\n"
	gr.Render(input)
	result := gr.Flush()

	// Glamour должен отрендерить inline code
	if !strings.Contains(result, "fmt.Println") {
		t.Errorf("Inline code content missing, got: %q", result)
	}
}

func TestGlamourRendererTable(t *testing.T) {
	gr := NewGlamourRenderer()

	input := "| Name | Value |\n|------|-------|\n| a | 1 |\n| b | 2 |\n"
	gr.Render(input)
	result := gr.Flush()

	if !strings.Contains(result, "Name") {
		t.Errorf("Table content missing, got: %q", result)
	}
}

func TestSpinnerStartStop(t *testing.T) {
	s := NewSpinner("test")
	if s.IsActive() {
		t.Error("Spinner should not be active before Start()")
	}

	s.Start()
	if !s.IsActive() {
		t.Error("Spinner should be active after Start()")
	}

	s.Stop()
	if s.IsActive() {
		t.Error("Spinner should not be active after Stop()")
	}
}

func TestFormatContextBarColors(t *testing.T) {
	// Зеленый (< 50%)
	green := FormatContextBar(10, 100)
	if !strings.Contains(green, appTheme.Success.ANSICode()) {
		t.Error("Low usage should be green")
	}

	// Жёлтый (50-80%)
	yellow := FormatContextBar(60, 100)
	if !strings.Contains(yellow, appTheme.Warning.ANSICode()) {
		t.Error("Medium usage should be yellow")
	}

	// Красный (> 80%)
	red := FormatContextBar(90, 100)
	if !strings.Contains(red, appTheme.Error.ANSICode()) {
		t.Error("High usage should be red")
	}
}

func TestFormatToolCallEndWithDuration(t *testing.T) {
	result := FormatToolCallEnd("bash", true, "output", "output", 5*time.Second, nil)
	if !strings.Contains(result, "5.0s") {
		t.Errorf("Should show duration, got: %q", result)
	}
}

func TestFormatToolCallEndShortDuration(t *testing.T) {
	result := FormatToolCallEnd("read", true, "5 lines", "5 lines", 150*time.Millisecond, nil)
	if !strings.Contains(result, "150ms") {
		t.Errorf("Should show ms duration, got: %q", result)
	}
}
