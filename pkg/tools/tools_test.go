package tools

import (
	"strings"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadTool_Name(t *testing.T) {
	tool := NewReadTool()
	if tool.Name() != "read" {
		t.Errorf("Expected name 'read', got '%s'", tool.Name())
	}
}

func TestReadTool_MissingPath(t *testing.T) {
	tool := NewReadTool()
	result := tool.Execute(map[string]string{})
	if result.Error == "" {
		t.Error("Expected error for missing path")
	}
}

func TestReadTool_FileNotFound(t *testing.T) {
	tool := NewReadTool()
	result := tool.Execute(map[string]string{"path": "/nonexistent/file.txt"})
	if result.Error == "" {
		t.Error("Expected error for nonexistent file")
	}
}

func TestReadTool_ReadFile(t *testing.T) {
	// Создаём временный файл
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "hello world"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewReadTool()
	result := tool.Execute(map[string]string{"path": tmpFile})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	if result.Output != content {
		t.Errorf("Expected '%s', got '%s'", content, result.Output)
	}
}

func TestReadTool_ReadDir(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("a"), 0644)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	tool := NewReadTool()
	result := tool.Execute(map[string]string{"path": tmpDir})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	if result.Output == "" {
		t.Error("Expected non-empty output for directory listing")
	}
}

func TestReadTool_PathTraversal(t *testing.T) {
	tool := NewReadTool()
	result := tool.Execute(map[string]string{"path": "../../../etc/passwd"})
	if result.Error == "" {
		t.Error("Expected error for path traversal")
	}
}

func TestReadTool_AllowedDirs(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(tmpFile, []byte("content"), 0644)

	tool := NewReadTool()
	tool.AllowedDirs = []string{tmpDir}

	// Доступ разрешён
	result := tool.Execute(map[string]string{"path": tmpFile})
	if result.Error != "" {
		t.Errorf("Expected success, got error: %s", result.Error)
	}

	// Доступ запрещён
	result = tool.Execute(map[string]string{"path": "/etc/hosts"})
	if result.Error == "" {
		t.Error("Expected error for disallowed directory")
	}
}

func TestWriteTool_WriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "output.txt")
	content := "test content"

	tool := NewWriteTool()
	result := tool.Execute(map[string]string{
		"path":    tmpFile,
		"content": content,
	})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}

	// Проверяем что файл создан
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("Expected '%s', got '%s'", content, string(data))
	}
}

func TestWriteTool_MissingParams(t *testing.T) {
	tool := NewWriteTool()

	result := tool.Execute(map[string]string{"path": "test.txt"})
	if result.Error == "" {
		t.Error("Expected error for missing content")
	}

	result = tool.Execute(map[string]string{"content": "test"})
	if result.Error == "" {
		t.Error("Expected error for missing path")
	}
}

func TestWriteTool_CreateDirs(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "sub", "dir", "file.txt")

	tool := NewWriteTool()
	result := tool.Execute(map[string]string{
		"path":    tmpFile,
		"content": "nested",
	})
	if result.Error != "" {
		t.Errorf("Unexpected error creating nested dirs: %s", result.Error)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "nested" {
		t.Errorf("Expected 'nested', got '%s'", string(data))
	}
}

func TestEditTool_Replace(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "edit.txt")
	os.WriteFile(tmpFile, []byte("hello world"), 0644)

	tool := NewEditTool()
	result := tool.Execute(map[string]string{
		"path": tmpFile,
		"old":  "world",
		"new":  "Go",
	})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}

	data, _ := os.ReadFile(tmpFile)
	if string(data) != "hello Go" {
		t.Errorf("Expected 'hello Go', got '%s'", string(data))
	}
}

func TestEditTool_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "edit.txt")
	os.WriteFile(tmpFile, []byte("hello world"), 0644)

	tool := NewEditTool()
	result := tool.Execute(map[string]string{
		"path": tmpFile,
		"old":  "nonexistent",
		"new":  "replacement",
	})
	if result.Error == "" {
		t.Error("Expected error for text not found")
	}
}

func TestEditTool_MissingParams(t *testing.T) {
	tool := NewEditTool()

	result := tool.Execute(map[string]string{"path": "test.txt"})
	if result.Error == "" {
		t.Error("Expected error for missing params")
	}
}

func TestBashTool_Echo(t *testing.T) {
	tool := NewBashTool()
	result := tool.Execute(map[string]string{"command": "echo hello"})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	if result.Output == "" {
		t.Error("Expected non-empty output")
	}
}

func TestBashTool_MissingCommand(t *testing.T) {
	tool := NewBashTool()
	result := tool.Execute(map[string]string{})
	if result.Error == "" {
		t.Error("Expected error for missing command")
	}
}

func TestBashTool_DangerousCommand(t *testing.T) {
	tool := NewBashTool()
	result := tool.Execute(map[string]string{"command": "rm -rf /"})
	if result.Error == "" {
		t.Error("Expected error for dangerous command")
	}
}

func TestBashTool_BlockedCommands(t *testing.T) {
	tool := NewBashTool()
	tool.BlockedCommands = []string{"rm -rf /", "mkfs", "shutdown"}
	result := tool.Execute(map[string]string{"command": "shutdown now"})
	if result.Error == "" {
		t.Error("Expected error for blocked command")
	}
}

func TestBashTool_NetworkBlocked(t *testing.T) {
	tool := NewBashTool()
	tool.AllowNetwork = false
	result := tool.Execute(map[string]string{"command": "curl https://example.com"})
	if result.Error == "" {
		t.Error("Expected error for network command when AllowNetwork=false")
	}
}

func TestBashTool_NetworkAllowed(t *testing.T) {
	tool := NewBashTool()
	tool.AllowNetwork = true
	// Не выполняем реальный curl, просто проверяем что не блокируется
	result := tool.Execute(map[string]string{"command": "echo curl"})
	if result.Error != "" {
		t.Errorf("Unexpected error when AllowNetwork=true: %s", result.Error)
	}
}

func TestBashTool_TimeoutParsing(t *testing.T) {
	tool := NewBashTool()
	tool.Timeout = 5 * time.Second
	result := tool.Execute(map[string]string{"command": "echo hello", "timeout": "10"})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	// Проверяем что таймаут парсится (не можем проверить напрямую, но хотя бы нет ошибки)
}

func TestGrepTool_Search(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello world\nfoo bar\nhello Go"), 0644)

	tool := NewGrepTool()
	result := tool.Execute(map[string]string{
		"pattern": "hello",
		"path":    tmpDir,
	})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	if result.Output == "" {
		t.Error("Expected non-empty output")
	}
}

func TestGrepTool_MissingPattern(t *testing.T) {
	tool := NewGrepTool()
	result := tool.Execute(map[string]string{})
	if result.Error == "" {
		t.Error("Expected error for missing pattern")
	}
}

func TestGlobTool_Search(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0644)

	tool := NewGlobTool()
	result := tool.Execute(map[string]string{
		"pattern": "*.go",
		"path":    tmpDir,
	})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	if result.Output == "" {
		t.Error("Expected non-empty output")
	}
}

func TestGlobTool_MissingPattern(t *testing.T) {
	tool := NewGlobTool()
	result := tool.Execute(map[string]string{})
	if result.Error == "" {
		t.Error("Expected error for missing pattern")
	}
}

func TestSuccess(t *testing.T) {
	result := Success("ok")
	if result.Output != "ok" || result.Error != "" {
		t.Errorf("Expected Output='ok', Error='', got Output='%s', Error='%s'", result.Output, result.Error)
	}

	result = Success("count: %d", 42)
	if result.Output != "count: 42" {
		t.Errorf("Expected 'count: 42', got '%s'", result.Output)
	}
}

func TestError(t *testing.T) {
	result := Error("error: %s", "test")
	if result.Error == "" || result.Output != "" {
		t.Errorf("Expected non-empty Error and empty Output")
	}
}

func TestToolParameters(t *testing.T) {
	tools := []Tool{
		NewReadTool(),
		NewWriteTool(),
		NewEditTool(),
		NewBashTool(),
		NewGrepTool(),
		NewGlobTool(),
		NewAskTool(),
		NewLearnTool(),
		NewWebFetchTool(),
		NewAskUserTool(),
	}

	for _, tool := range tools {
		params := tool.Parameters()
		if params == nil {
			t.Errorf("Tool %s: Parameters() returned nil", tool.Name())
		}
		if _, ok := params["type"]; !ok {
			t.Errorf("Tool %s: Parameters() missing 'type'", tool.Name())
		}
		if params["type"] != "object" {
			t.Errorf("Tool %s: Parameters() type should be 'object', got '%v'", tool.Name(), params["type"])
		}
	}
}

func TestBashTool_BackgroundRedirect(t *testing.T) {
	tool := NewBashTool()
	result := tool.Execute(map[string]string{"command": "./server &"})
	if result.Error != "" {
		t.Errorf("Expected no error, got: %s", result.Error)
	}
	if !strings.Contains(result.Output, "background") {
		t.Errorf("Expected redirect to background tool, got: %s", result.Output)
	}
	if strings.Contains(result.Output, "&") && !strings.Contains(result.Output, "not supported") {
		t.Error("Should warn that & is not supported")
	}
}

func TestBashTool_BackgroundRedirectWithLog(t *testing.T) {
	tool := NewBashTool()
	result := tool.Execute(map[string]string{"command": "./server &>/tmp/log.txt &"})
	if result.Error != "" {
		t.Errorf("Expected no error, got: %s", result.Error)
	}
	if !strings.Contains(result.Output, "background") {
		t.Errorf("Expected redirect to background tool, got: %s", result.Output)
	}
}

func TestBashTool_BackgroundRedirectNoAmpersand(t *testing.T) {
	tool := NewBashTool()
	result := tool.Execute(map[string]string{"command": "echo hello"})
	if strings.Contains(result.Output, "background") {
		t.Error("Regular commands should not redirect to background")
	}
}
