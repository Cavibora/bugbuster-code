package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ===================== GlobTool Execute additional tests =====================

func TestGlobTool_Execute_MissingPattern(t *testing.T) {
	tool := NewGlobTool()
	result := tool.Execute(map[string]string{})
	if result.Error == "" {
		t.Error("expected error for missing pattern")
	}
}

func TestGlobTool_Execute_EmptyPattern(t *testing.T) {
	tool := NewGlobTool()
	result := tool.Execute(map[string]string{"pattern": ""})
	if result.Error == "" {
		t.Error("expected error for empty pattern")
	}
}

func TestGlobTool_Execute_SimplePattern(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("package main"), 0644)

	tool := NewGlobTool()
	result := tool.Execute(map[string]string{
		"pattern": "*.txt",
		"path":    tmpDir,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "test.txt") {
		t.Errorf("expected 'test.txt' in output, got: %s", result.Output)
	}
	if strings.Contains(result.Output, "test.go") {
		t.Errorf("did not expect 'test.go' in output, got: %s", result.Output)
	}
}

func TestGlobTool_Execute_RecursivePattern(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "nested.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "root.go"), []byte("package main"), 0644)

	tool := NewGlobTool()
	result := tool.Execute(map[string]string{
		"pattern": "**/*.go",
		"path":    tmpDir,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "root.go") {
		t.Errorf("expected 'root.go' in output, got: %s", result.Output)
	}
}

func TestGlobTool_Execute_HiddenDirsSkipped(t *testing.T) {
	tmpDir := t.TempDir()
	hiddenDir := filepath.Join(tmpDir, ".hidden")
	os.MkdirAll(hiddenDir, 0755)
	os.WriteFile(filepath.Join(hiddenDir, "secret.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "visible.go"), []byte("package main"), 0644)

	tool := NewGlobTool()
	result := tool.Execute(map[string]string{
		"pattern": "**/*.go",
		"path":    tmpDir,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if strings.Contains(result.Output, "secret.go") {
		t.Errorf("hidden dir files should be skipped, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "visible.go") {
		t.Errorf("expected 'visible.go' in output, got: %s", result.Output)
	}
}

func TestGlobTool_Execute_MaxResults(t *testing.T) {
	tmpDir := t.TempDir()
	for i := 0; i < 20; i++ {
		os.WriteFile(filepath.Join(tmpDir, "file_"+strings.Repeat("x", 10)+".txt"), []byte("x"), 0644)
	}

	tool := NewGlobTool()
	tool.MaxResults = 5

	result := tool.Execute(map[string]string{
		"pattern": "*.txt",
		"path":    tmpDir,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	// Output should be truncated
	lines := strings.Split(result.Output, "\n")
	// Should have at most 5 results + possible truncation message
	if len(lines) > 7 {
		t.Errorf("expected at most ~5 results + truncation message, got %d lines", len(lines))
	}
}

func TestGlobTool_Execute_PathTraversal(t *testing.T) {
	tool := NewGlobTool()
	result := tool.Execute(map[string]string{
		"pattern": "*.go",
		"path":    "../../../etc",
	})
	if result.Error == "" {
		t.Error("expected error for path traversal")
	}
}

func TestGlobTool_Execute_AllowedDirs(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0644)

	tool := NewGlobTool()
	tool.AllowedDirs = []string{tmpDir}

	// Allowed path
	result := tool.Execute(map[string]string{
		"pattern": "*.txt",
		"path":    tmpDir,
	})
	if result.Error != "" {
		t.Errorf("expected success for allowed dir, got: %s", result.Error)
	}

	// Disallowed path
	result = tool.Execute(map[string]string{
		"pattern": "*.txt",
		"path":    "/etc",
	})
	if result.Error == "" {
		t.Error("expected error for disallowed dir")
	}
}

func TestGlobTool_Execute_NonExistentPath(t *testing.T) {
	tool := NewGlobTool()
	result := tool.Execute(map[string]string{
		"pattern": "*.go",
		"path":    "/nonexistent/path/that/does/not/exist",
	})
	// Should return error or no files found
	// Both are acceptable behaviors
	if result.Error != "" && !strings.Contains(result.Output, "no files") {
		// Error is acceptable for non-existent path
		t.Logf("error for non-existent path: %s", result.Error)
	}
}

func TestGlobTool_Execute_ExtensionMatch(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "readme.md"), []byte("# Readme"), 0644)

	tool := NewGlobTool()
	result := tool.Execute(map[string]string{
		"pattern": "*.go",
		"path":    tmpDir,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "main.go") {
		t.Errorf("expected 'main.go' in output, got: %s", result.Output)
	}
	if strings.Contains(result.Output, "readme.md") {
		t.Errorf("did not expect 'readme.md' in output, got: %s", result.Output)
	}
}

func TestGlobTool_Name(t *testing.T) {
	tool := NewGlobTool()
	if tool.Name() != "glob" {
		t.Errorf("expected 'glob', got '%s'", tool.Name())
	}
}

func TestGlobTool_Description(t *testing.T) {
	tool := NewGlobTool()
	desc := tool.Description()
	if desc == "" {
		t.Error("expected non-empty description")
	}
}