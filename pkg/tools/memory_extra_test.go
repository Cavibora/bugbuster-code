package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- NewMemoryTool ---

func TestNewMemoryTool(t *testing.T) {
	m := NewMemoryTool("test-session-123")
	if m == nil {
		t.Fatal("expected non-nil MemoryTool")
	}
	if m.filePath == "" {
		t.Error("expected filePath to be set")
	}
	if !strings.Contains(m.filePath, "test-session-123") {
		t.Errorf("expected filePath to contain session ID, got: %s", m.filePath)
	}
}

// --- Description ---

func TestMemoryTool_Description(t *testing.T) {
	m := NewMemoryToolWithPath(filepath.Join(t.TempDir(), "memory.md"))
	desc := m.Description()
	if desc == "" {
		t.Error("expected non-empty description")
	}
}

// --- SetSessionID ---

func TestMemoryTool_SetSessionID(t *testing.T) {
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "old-session.md")
	m := NewMemoryToolWithPath(oldPath)

	// Save a fact
	m.Execute(map[string]string{"action": "save", "key": "test_key", "value": "test_value", "category": "test"})

	// Verify old file exists
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		t.Error("expected old session file to exist")
	}

	// Change session ID — this should update filePath
	m.SetSessionID("new-session-456")

	// The new path should contain the new session ID
	if !strings.Contains(m.filePath, "new-session-456") {
		t.Errorf("expected filePath to contain 'new-session-456', got: %s", m.filePath)
	}

	// Old facts should be preserved in the new file
	result := m.Execute(map[string]string{"action": "load", "key": "test_key"})
	if !strings.Contains(result.Output, "test_value") {
		t.Errorf("expected preserved fact after SetSessionID, got: %s", result.Output)
	}
}

// --- SetSessionIDForProject ---

func TestMemoryTool_SetSessionIDForProject(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "myproject")
	os.MkdirAll(filepath.Join(projectDir, ".bugbuster"), 0755)

	oldPath := filepath.Join(tmpDir, "old-session.md")
	m := NewMemoryToolWithPath(oldPath)

	// Save a fact
	m.Execute(map[string]string{"action": "save", "key": "project_key", "value": "project_value", "category": "project"})

	// Change session ID with project dir
	m.SetSessionIDForProject("proj-session", projectDir)

	// The new path should contain the session ID
	if !strings.Contains(m.filePath, "proj-session") {
		t.Errorf("expected filePath to contain 'proj-session', got: %s", m.filePath)
	}

	// Facts should be preserved
	result := m.Execute(map[string]string{"action": "load", "key": "project_key"})
	if !strings.Contains(result.Output, "project_value") {
		t.Errorf("expected preserved fact after SetSessionIDForProject, got: %s", result.Output)
	}
}

// --- MemoryFilePathForProject ---

func TestMemoryFilePathForProject(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	os.MkdirAll(filepath.Join(projectDir, ".bugbuster"), 0755)

	path := MemoryFilePathForProject("session-1", projectDir)
	if !strings.Contains(path, "session-1") {
		t.Errorf("expected path to contain session ID, got: %s", path)
	}
	if !strings.Contains(path, ".bugbuster") {
		t.Errorf("expected path to contain .bugbuster, got: %s", path)
	}
}

func TestMemoryFilePathForProject_NoBugBusterDir(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project_no_bb")
	// Don't create .bugbuster dir — should fall back

	path := MemoryFilePathForProject("session-2", projectDir)
	// Should still return a valid path (fallback to home dir or cwd)
	if path == "" {
		t.Error("expected non-empty path")
	}
	if !strings.Contains(path, "session-2") {
		t.Errorf("expected path to contain session ID, got: %s", path)
	}
}

// --- Compress edge cases ---

func TestMemoryTool_CompressEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	mt := NewMemoryToolWithPath(filepath.Join(tmpDir, "test.md"))

	result := mt.Execute(map[string]string{"action": "compress"})
	if result.Error != "" {
		t.Fatalf("compress on empty memory should not error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "empty") {
		t.Errorf("expected 'empty' in output, got: %s", result.Output)
	}
}

func TestMemoryTool_CompressWithMaxTokens(t *testing.T) {
	tmpDir := t.TempDir()
	mt := NewMemoryToolWithPath(filepath.Join(tmpDir, "test.md"))

	// Save many facts
	for i := 0; i < 20; i++ {
		mt.Execute(map[string]string{
			"action":   "save",
			"key":      fmt.Sprintf("key_%d", i),
			"value":    fmt.Sprintf("value_%d_with_some_padding_to_make_it_longer", i),
			"category": "test",
		})
	}

	// Compress with very low max_tokens — should keep only some facts
	result := mt.Execute(map[string]string{"action": "compress", "max_tokens": "50"})
	if result.Error != "" {
		t.Fatalf("compress failed: %s", result.Error)
	}
	if !strings.Contains(result.Output, "compressed") {
		t.Errorf("expected 'compressed' in output, got: %s", result.Output)
	}

	// Verify some facts were removed
	listResult := mt.Execute(map[string]string{"action": "list"})
	if listResult.Error != "" {
		t.Fatalf("list failed: %s", listResult.Error)
	}
	// Should have fewer facts than 20
	factCount := strings.Count(listResult.Output, "key_")
	if factCount >= 20 {
		t.Errorf("expected fewer facts after compress, got %d", factCount)
	}
}

// --- LoadAllFacts with multiple categories ---

func TestMemoryTool_LoadAllFactsFormat(t *testing.T) {
	tmpDir := t.TempDir()
	mt := NewMemoryToolWithPath(filepath.Join(tmpDir, "test.md"))

	mt.Execute(map[string]string{"action": "save", "key": "path", "value": "/test", "category": "project"})
	mt.Execute(map[string]string{"action": "save", "key": "host", "value": "localhost", "category": "database"})

	prompt := mt.LoadAllFacts()
	if !strings.Contains(prompt, "Important facts") {
		t.Errorf("expected header in prompt, got: %s", prompt)
	}
	if !strings.Contains(prompt, "[project]") {
		t.Errorf("expected [project] category, got: %s", prompt)
	}
	if !strings.Contains(prompt, "[database]") {
		t.Errorf("expected [database] category, got: %s", prompt)
	}
}

// --- TokenCount with data ---

func TestMemoryTool_TokenCountWithData(t *testing.T) {
	tmpDir := t.TempDir()
	mt := NewMemoryToolWithPath(filepath.Join(tmpDir, "test.md"))

	mt.Execute(map[string]string{"action": "save", "key": "long_key_name", "value": "a long value that should contribute to token count", "category": "test"})

	count := mt.TokenCount()
	if count <= 0 {
		t.Errorf("expected positive token count, got %d", count)
	}
}

// --- Delete by case-insensitive key ---

func TestMemoryTool_DeleteCaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	mt := NewMemoryToolWithPath(filepath.Join(tmpDir, "test.md"))

	mt.Execute(map[string]string{"action": "save", "key": "MyKey", "value": "my_value"})

	result := mt.Execute(map[string]string{"action": "delete", "key": "mykey"})
	if result.Error != "" {
		t.Fatalf("delete failed: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Deleted") {
		t.Errorf("expected 'Deleted' in output, got: %s", result.Output)
	}

	// Verify key is gone
	loadResult := mt.Execute(map[string]string{"action": "load", "key": "MyKey"})
	if strings.Contains(loadResult.Output, "my_value") {
		t.Error("key should be deleted")
	}
}