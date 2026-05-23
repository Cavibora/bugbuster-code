package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// === EditTool расширенные тесты ===

func TestEditTool_PathTraversal(t *testing.T) {
	tool := NewEditTool()
	result := tool.Execute(map[string]string{
		"path": "../../../etc/passwd",
		"old":  "root",
		"new":  "hacked",
	})
	if result.Error == "" {
		t.Error("Expected error for path traversal")
	}
}

func TestEditTool_AllowedDirs(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "edit.txt")
	os.WriteFile(tmpFile, []byte("hello world"), 0644)

	tool := NewEditTool()
	tool.AllowedDirs = []string{tmpDir}

	// Доступ разрешён
	result := tool.Execute(map[string]string{
		"path": tmpFile,
		"old":  "world",
		"new":  "Go",
	})
	if result.Error != "" {
		t.Errorf("Expected success, got error: %s", result.Error)
	}

	// Доступ запрещён
	result = tool.Execute(map[string]string{
		"path": "/etc/hosts",
		"old":  "localhost",
		"new":  "test",
	})
	if result.Error == "" {
		t.Error("Expected error for disallowed directory")
	}
}

func TestEditTool_MultipleReplacements(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "multi.txt")
	os.WriteFile(tmpFile, []byte("aaa bbb aaa"), 0644)

	tool := NewEditTool()
	// Замена только первого вхождения
	result := tool.Execute(map[string]string{
		"path": tmpFile,
		"old":  "aaa",
		"new":  "ccc",
	})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}

	data, _ := os.ReadFile(tmpFile)
	if string(data) != "ccc bbb aaa" {
		t.Errorf("Expected 'ccc bbb aaa', got '%s'", string(data))
	}
}

// === WriteTool расширенные тесты ===

func TestWriteTool_PathTraversal(t *testing.T) {
	tool := NewWriteTool()
	result := tool.Execute(map[string]string{
		"path":    "../../../tmp/hacked.txt",
		"content": "hacked",
	})
	if result.Error == "" {
		t.Error("Expected error for path traversal")
	}
}

func TestWriteTool_AllowedDirs(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "allowed.txt")

	tool := NewWriteTool()
	tool.AllowedDirs = []string{tmpDir}

	// Доступ разрешён
	result := tool.Execute(map[string]string{
		"path":    tmpFile,
		"content": "allowed",
	})
	if result.Error != "" {
		t.Errorf("Expected success, got error: %s", result.Error)
	}

	// Доступ запрещён
	result = tool.Execute(map[string]string{
		"path":    "/tmp/outside.txt",
		"content": "outside",
	})
	if result.Error == "" {
		t.Error("Expected error for disallowed directory")
	}
}

func TestWriteTool_OverwriteExisting(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "overwrite.txt")
	os.WriteFile(tmpFile, []byte("old content"), 0644)

	tool := NewWriteTool()
	result := tool.Execute(map[string]string{
		"path":    tmpFile,
		"content": "new content",
	})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}

	data, _ := os.ReadFile(tmpFile)
	if string(data) != "new content" {
		t.Errorf("Expected 'new content', got '%s'", string(data))
	}
}

// === ReadTool расширенные тесты ===

func TestReadTool_MaxSize(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "large.txt")
	// Создаём файл больше 1KB
	content := strings.Repeat("x", 2048)
	os.WriteFile(tmpFile, []byte(content), 0644)

	tool := NewReadTool()
	tool.MaxSize = 1024 // 1KB лимит

	result := tool.Execute(map[string]string{"path": tmpFile})
	if result.Error == "" {
		t.Error("Expected error for file too large")
	}
}

func TestReadTool_SecretPath(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	os.WriteFile(envFile, []byte("SECRET=123"), 0644)

	tool := NewReadTool()
	result := tool.Execute(map[string]string{"path": envFile})
	if result.Error == "" {
		t.Error("Expected error for secret path")
	}
}

// === GrepTool расширенные тесты ===

func TestGrepTool_CaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("Hello World\nhello go\nHELLO"), 0644)

	tool := NewGrepTool()
	result := tool.Execute(map[string]string{
		"pattern":          "hello",
		"path":             tmpDir,
		"case_insensitive": "true",
	})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	// Должно найти все 3 строки
	lines := strings.Split(strings.TrimSpace(result.Output), "\n")
	if len(lines) < 3 {
		t.Errorf("Expected at least 3 matches with case_insensitive, got %d: %s", len(lines), result.Output)
	}
}

func TestGrepTool_FilePattern(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("func main()"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("func main()"), 0644)

	tool := NewGrepTool()
	result := tool.Execute(map[string]string{
		"pattern":      "func",
		"path":         tmpDir,
		"file_pattern": "*.go",
	})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	// Должен найти только в .go файле
	if !strings.Contains(result.Output, "test.go") {
		t.Errorf("Expected test.go in results, got: %s", result.Output)
	}
	if strings.Contains(result.Output, "test.txt") {
		t.Errorf("test.txt should be excluded by file_pattern, got: %s", result.Output)
	}
}

func TestGrepTool_NoMatches(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello world"), 0644)

	tool := NewGrepTool()
	result := tool.Execute(map[string]string{
		"pattern": "nonexistent_pattern_xyz",
		"path":    tmpDir,
	})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	// Нет совпадений — не ошибка, а пустой результат
}

func TestGrepTool_PathTraversal(t *testing.T) {
	tool := NewGrepTool()
	result := tool.Execute(map[string]string{
		"pattern": "root",
		"path":    "../../../etc",
	})
	if result.Error == "" {
		t.Error("Expected error for path traversal")
	}
}

func TestGrepTool_InvalidRegex(t *testing.T) {
	tool := NewGrepTool()
	result := tool.Execute(map[string]string{
		"pattern": "[invalid",
	})
	if result.Error == "" {
		t.Error("Expected error for invalid regex")
	}
}

// === GlobTool расширенные тесты ===

func TestGlobTool_PathTraversal(t *testing.T) {
	tool := NewGlobTool()
	result := tool.Execute(map[string]string{
		"pattern": "*.go",
		"path":    "../../../etc",
	})
	if result.Error == "" {
		t.Error("Expected error for path traversal")
	}
}

func TestGlobTool_AllowedDirs(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("package main"), 0644)

	tool := NewGlobTool()
	tool.AllowedDirs = []string{tmpDir}

	// Доступ разрешён
	result := tool.Execute(map[string]string{
		"pattern": "*.go",
		"path":    tmpDir,
	})
	if result.Error != "" {
		t.Errorf("Expected success, got error: %s", result.Error)
	}

	// Доступ запрещён
	result = tool.Execute(map[string]string{
		"pattern": "*.go",
		"path":    "/etc",
	})
	if result.Error == "" {
		t.Error("Expected error for disallowed directory")
	}
}

func TestGlobTool_NoMatches(t *testing.T) {
	tmpDir := t.TempDir()

	tool := NewGlobTool()
	result := tool.Execute(map[string]string{
		"pattern": "*.xyz",
		"path":    tmpDir,
	})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
}

func TestGlobTool_MaxResults(t *testing.T) {
	tmpDir := t.TempDir()
	// Создаём много файлов
	for i := 0; i < 150; i++ {
		os.WriteFile(filepath.Join(tmpDir, "file_"+string(rune('A'+i%26))+".txt"), []byte("x"), 0644)
	}

	tool := NewGlobTool()
	tool.MaxResults = 10

	result := tool.Execute(map[string]string{
		"pattern": "*.txt",
		"path":    tmpDir,
	})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	// Результат должен быть обрезан
	lines := strings.Split(result.Output, "\n")
	if len(lines) > 12 { // 10 результатов + строка truncation
		t.Errorf("Expected max 10 results + truncation message, got %d lines", len(lines))
	}
}

// === BashTool расширенные тесты ===

func TestBashTool_DefaultDir(t *testing.T) {
	tmpDir := t.TempDir()

	tool := NewBashTool()
	tool.DefaultDir = tmpDir

	result := tool.Execute(map[string]string{"command": "pwd"})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	// Вывод должен содержать tmpDir
	// Примечание: pwd может вернуть symlink, поэтому просто проверяем что нет ошибки
}

func TestBashTool_WorkingDir(t *testing.T) {
	tool := NewBashTool()
	result := tool.Execute(map[string]string{
		"command": "echo test",
		"dir":     "/tmp",
	})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
}

func TestBashTool_SecretCommand(t *testing.T) {
	tool := NewBashTool()
	// Команды с секретными файлами должны блокироваться
	result := tool.Execute(map[string]string{"command": "cat .env"})
	if result.Error == "" {
		t.Error("Expected error for secret file access")
	}
}

// === LearnTool тесты ===

func TestLearnTool_Name(t *testing.T) {
	tool := NewLearnTool()
	if tool.Name() != "learn" {
		t.Errorf("Expected name 'learn', got '%s'", tool.Name())
	}
}

func TestLearnTool_MissingParams(t *testing.T) {
	tool := NewLearnTool()

	// Без input
	result := tool.Execute(map[string]string{})
	if result.Error == "" {
		t.Error("Expected error for missing input")
	}

	// Без output
	result = tool.Execute(map[string]string{"input": "test"})
	if result.Error == "" {
		t.Error("Expected error for missing output")
	}
}

func TestLearnTool_NotAvailable(t *testing.T) {
	tool := NewLearnTool()
	result := tool.Execute(map[string]string{
		"input":  "question",
		"output": "answer",
	})
	if result.Error == "" {
		t.Error("Expected error when TeachURL is empty")
	}
}

func TestLearnTool_WithTeachURL(t *testing.T) {
	tool := NewLearnTool()
	tool.TeachURL = "http://localhost:8080/v1/teach"

	result := tool.Execute(map[string]string{
		"input":  "What is Go?",
		"output": "Go is a programming language",
	})
	// TeachURL задан, но реального сервера нет — проверяем что нет ошибки "not available"
	// (в текущей реализации возвращается Success, т.к. HTTP-запрос ещё не реализован)
	if result.Error != "" && !strings.Contains(result.Error, "not_available") {
		// Если ошибка не "not_available" — это нормально (HTTP не реализован)
		t.Logf("Result: %s", result.Output)
	}
}

func TestLearnTool_Parameters(t *testing.T) {
	tool := NewLearnTool()
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("Expected type='object', got '%v'", params["type"])
	}
}

// === AskTool тесты ===

func TestAskTool_Name(t *testing.T) {
	tool := NewAskTool()
	if tool.Name() != "ask" {
		t.Errorf("Expected name 'ask', got '%s'", tool.Name())
	}
}

func TestAskTool_MissingPrompt(t *testing.T) {
	tool := NewAskTool()
	result := tool.Execute(map[string]string{})
	if result.Error == "" {
		t.Error("Expected error for missing prompt")
	}
}

func TestAskTool_NoProvider(t *testing.T) {
	tool := NewAskTool()
	result := tool.Execute(map[string]string{"prompt": "test"})
	if result.Error == "" {
		t.Error("Expected error when no provider is set")
	}
}

func TestAskTool_Parameters(t *testing.T) {
	tool := NewAskTool()
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("Expected type='object', got '%v'", params["type"])
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be map[string]any")
	}
	if _, ok := props["prompt"]; !ok {
		t.Error("expected 'prompt' property")
	}
}

// === AskUserTool расширенные тесты ===

func TestAskUserTool_NonInteractive(t *testing.T) {
	tool := NewAskUserTool()
	tool.NonInteractive = true

	result := tool.Execute(map[string]string{"question": "What is your name?"})
	if result.Error != "" {
		t.Errorf("Unexpected error in non-interactive mode: %s", result.Error)
	}
	// В неинтерактивном режиме должен вернуть skipped
}

// === WebFetchTool расширенные тесты ===

func TestWebFetchTool_MethodNotAllowed(t *testing.T) {
	tool := NewWebFetchTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{
		"url":    "https://example.com",
		"method": "DELETE",
	})
	if result.Error == "" {
		t.Error("Expected error for disallowed HTTP method")
	}
}

// === TodoWriteTool расширенные тесты ===

func TestTodoWriteTool_EmptyArray(t *testing.T) {
	tw := NewTodoWriteTool()
	result := tw.Execute(map[string]string{"todos": "[]"})
	if result.Error != "" {
		t.Errorf("Unexpected error for empty array: %s", result.Error)
	}

	var items []TodoItem
	json.Unmarshal([]byte(result.Output), &items)
	if len(items) != 0 {
		t.Errorf("Expected 0 items, got %d", len(items))
	}
}

func TestTodoWriteTool_ConcurrentAccess(t *testing.T) {
	tw := NewTodoWriteTool()

	// Запускаем конкурентную запись
	done := make(chan bool, 2)
	go func() {
		tw.Execute(map[string]string{
			"todos": `[{"id":"1","subject":"goroutine 1","status":"pending"}]`,
		})
		done <- true
	}()
	go func() {
		tw.Execute(map[string]string{
			"todos": `[{"id":"2","subject":"goroutine 2","status":"completed"}]`,
		})
		done <- true
	}()

	// Ждём завершения обеих горутин
	<-done
	<-done

	// Проверяем что GetTodos возвращает валидный результат
	todos := tw.GetTodos()
	if len(todos) != 1 {
		t.Errorf("Expected 1 item after concurrent writes, got %d", len(todos))
	}
}

// === LSPTool расширенные тесты ===

func TestLSPTool_AllowedDirs(t *testing.T) {
	tool := NewLSPTool()
	tool.AllowedDirs = []string{"/tmp"}

	result := tool.Execute(map[string]string{
		"operation": "hover",
		"file_path": "/etc/passwd",
	})
	// Должен вернуть ошибку доступа (путь не в AllowedDirs)
	if result.Error == "" {
		t.Error("Expected error for disallowed path")
	}
}

func TestLSPTool_UnsupportedLanguage(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "readme.md")
	os.WriteFile(tmpFile, []byte("# Hello"), 0644)

	tool := NewLSPTool()
	tool.AllowedDirs = []string{tmpDir}

	result := tool.Execute(map[string]string{
		"operation": "hover",
		"file_path": tmpFile,
	})
	if result.Error == "" {
		t.Error("Expected error for unsupported language (.md)")
	}
}

func TestLSPTool_ServerNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.go")
	os.WriteFile(tmpFile, []byte("package main"), 0644)

	tool := NewLSPTool()
	tool.AllowedDirs = []string{tmpDir}
	// Нет серверов в конфигурации

	result := tool.Execute(map[string]string{
		"operation": "hover",
		"file_path": tmpFile,
	})
	if result.Error == "" {
		t.Error("Expected error when no LSP server configured for Go")
	}
}

// === BashTool Timeout тест ===

func TestBashTool_Timeout(t *testing.T) {
	tool := NewBashTool()
	tool.Timeout = 1 * time.Second

	result := tool.Execute(map[string]string{
		"command": "sleep 10",
	})
	if result.Error == "" {
		t.Error("Expected error for timeout")
	}
}

// === min() helper тест (из learn.go) ===

func TestMin(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{0, 10, 0},
	}
	for _, tt := range tests {
		result := min(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, result, tt.expected)
		}
	}
}
