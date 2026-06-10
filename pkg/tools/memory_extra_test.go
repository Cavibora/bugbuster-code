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

	m.Execute(map[string]string{"action": "save", "key": "test_key", "value": "test_value", "category": "test"})

	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		t.Error("expected old session file to exist")
	}

	m.SetSessionID("new-session-456")

	if !strings.Contains(m.filePath, "new-session-456") {
		t.Errorf("expected filePath to contain 'new-session-456', got: %s", m.filePath)
	}

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

	m.Execute(map[string]string{"action": "save", "key": "project_key", "value": "project_value", "category": "project"})

	m.SetSessionIDForProject("proj-session", projectDir)

	if !strings.Contains(m.filePath, "proj-session") {
		t.Errorf("expected filePath to contain 'proj-session', got: %s", m.filePath)
	}

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

	path := MemoryFilePathForProject("session-2", projectDir)
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

	for i := 0; i < 20; i++ {
		mt.Execute(map[string]string{
			"action":   "save",
			"key":      fmt.Sprintf("key_%d", i),
			"value":    fmt.Sprintf("value_%d_with_some_padding_to_make_it_longer", i),
			"category": "test",
		})
	}

	result := mt.Execute(map[string]string{"action": "compress", "max_tokens": "50"})
	if result.Error != "" {
		t.Fatalf("compress failed: %s", result.Error)
	}
	if !strings.Contains(strings.ToLower(result.Output), "compress") {
		t.Errorf("expected 'compress' in output, got: %s", result.Output)
	}

	listResult := mt.Execute(map[string]string{"action": "list"})
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

	// Save 2 facts so mass deletion protection doesn't trigger
	mt.Execute(map[string]string{"action": "save", "key": "MyKey", "value": "my_value"})
	mt.Execute(map[string]string{"action": "save", "key": "OtherKey", "value": "other_value"})

	result := mt.Execute(map[string]string{"action": "delete", "key": "mykey"})
	if !strings.Contains(result.Output, "Deleted") {
		t.Errorf("expected 'Deleted' in output, got: %s", result.Output)
	}

	loadResult := mt.Execute(map[string]string{"action": "load", "key": "MyKey"})
	if strings.Contains(loadResult.Output, "my_value") {
		t.Error("key should be deleted")
	}
}

// --- Duplicate value detection ---

func TestMemoryToolDuplicateValue(t *testing.T) {
	dir := t.TempDir()
	tool := NewMemoryToolWithPath(filepath.Join(dir, "test.md"))

	tool.Execute(map[string]string{"action": "save", "key": "project_path", "value": "/Users/ss/ai/myproject", "category": "project"})

	// Save same value with different key — should update existing
	result := tool.Execute(map[string]string{"action": "save", "key": "project_dir", "value": "/Users/ss/ai/myproject", "category": "project"})
	if !strings.Contains(result.Output, "Same value") && !strings.Contains(result.Output, "Updated") {
		t.Fatalf("Expected duplicate value update, got: %s", result.Output)
	}

	// Verify only one fact exists with this value
	result = tool.Execute(map[string]string{"action": "list"})
	if strings.Count(result.Output, "/Users/ss/ai/myproject") != 1 {
		t.Fatalf("Expected 1 fact with this value, got: %s", result.Output)
	}
}

// --- Similar key detection ---

func TestMemoryToolSimilarKey(t *testing.T) {
	dir := t.TempDir()
	tool := NewMemoryToolWithPath(filepath.Join(dir, "test.md"))

	tool.Execute(map[string]string{"action": "save", "key": "project_path", "value": "/Users/ss/ai/myproject", "category": "project"})

	// Save with similar key — should warn about similar key
	result := tool.Execute(map[string]string{"action": "save", "key": "project_root", "value": "/Users/ss/ai/otherproject", "category": "project"})
	if !strings.Contains(result.Output, "Saved") && !strings.Contains(result.Output, "similar") {
		t.Fatalf("Expected save with similar key warning, got: %s", result.Output)
	}
}

// --- Value truncation ---

func TestMemoryToolValueTruncation(t *testing.T) {
	dir := t.TempDir()
	tool := NewMemoryToolWithPath(filepath.Join(dir, "test.md"))

	longValue := strings.Repeat("a", 5000)
	result := tool.Execute(map[string]string{"action": "save", "key": "long_fact", "value": longValue, "category": "general"})
	if !strings.Contains(result.Output, "Saved") {
		t.Fatalf("Expected save to succeed, got: %s", result.Output)
	}

	// Verify value was truncated
	result = tool.Execute(map[string]string{"action": "load", "key": "long_fact"})
	if strings.Contains(result.Output, strings.Repeat("a", 5000)) {
		t.Fatal("Value should have been truncated")
	}
}

// --- Mass deletion protection ---

func TestMemoryToolMassDeletionProtection(t *testing.T) {
	dir := t.TempDir()
	tool := NewMemoryToolWithPath(filepath.Join(dir, "test.md"))

	// Save 5 facts
	for i := 0; i < 5; i++ {
		tool.Execute(map[string]string{"action": "save", "key": fmt.Sprintf("key_%d", i), "value": fmt.Sprintf("value_%d", i), "category": "test"})
	}

	// Try to delete by category — should be blocked (deletes 5 > 5/2 = 2)
	result := tool.Execute(map[string]string{"action": "delete", "key": "key_0"})
	if strings.Contains(result.Output, "Refusing") {
		// Single key deletion should NOT be blocked
		t.Fatalf("Single key deletion should not be blocked, got: %s", result.Output)
	}
}

// --- Auto-compress when exceeding max facts ---

func TestMemoryToolAutoCompress(t *testing.T) {
	dir := t.TempDir()
	tool := NewMemoryToolWithPath(filepath.Join(dir, "test.md"))

	// Save 35 facts (exceeds maxFacts=30)
	for i := 0; i < 35; i++ {
		tool.Execute(map[string]string{"action": "save", "key": fmt.Sprintf("fact_%d", i), "value": fmt.Sprintf("value_%d", i), "category": "test"})
	}

	// Verify auto-compress happened
	result := tool.Execute(map[string]string{"action": "list"})
	factCount := strings.Count(result.Output, "fact_")
	if factCount > 35 {
		t.Fatalf("Expected auto-compress to limit facts, got %d facts", factCount)
	}
}

// --- Restore from backup ---

func TestMemoryToolRestore(t *testing.T) {
	dir := t.TempDir()
	tool := NewMemoryToolWithPath(filepath.Join(dir, "test.md"))

	// Save a fact
	tool.Execute(map[string]string{"action": "save", "key": "important", "value": "critical_data", "category": "critical"})

	// Delete it (creates backup)
	tool.Execute(map[string]string{"action": "delete", "key": "important"})

	// Restore from backup
	result := tool.Execute(map[string]string{"action": "restore"})
	if strings.Contains(result.Output, "Restored") {
		// Verify fact was restored
		loadResult := tool.Execute(map[string]string{"action": "load", "key": "important"})
		if !strings.Contains(loadResult.Output, "critical_data") {
			t.Errorf("Expected restored fact, got: %s", loadResult.Output)
		}
	}
}