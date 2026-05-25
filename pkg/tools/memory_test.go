package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryToolSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	result := tool.Execute(map[string]string{
		"action": "save", "key": "project_path", "value": "/Users/test/myproject", "category": "project",
	})
	if result.Error != "" {
		t.Fatalf("save failed: %s", result.Error)
	}

	result = tool.Execute(map[string]string{"action": "load", "key": "project_path"})
	if result.Error != "" {
		t.Fatalf("load failed: %s", result.Error)
	}
	if !contains(result.Output, "/Users/test/myproject") {
		t.Fatalf("load did not contain value: %s", result.Output)
	}
}

func TestMemoryToolList(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{"action": "save", "key": "db_host", "value": "localhost:5432", "category": "database"})
	tool.Execute(map[string]string{"action": "save", "key": "db_user", "value": "admin", "category": "database"})
	tool.Execute(map[string]string{"action": "save", "key": "project_path", "value": "/home/user/project", "category": "project"})

	result := tool.Execute(map[string]string{"action": "list"})
	if result.Error != "" {
		t.Fatalf("list failed: %s", result.Error)
	}
	if !contains(result.Output, "database") {
		t.Fatalf("expected 'database' category, got: %s", result.Output)
	}
	if !contains(result.Output, "project") {
		t.Fatalf("expected 'project' category, got: %s", result.Output)
	}
}

func TestMemoryToolDelete(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{"action": "save", "key": "temp_key", "value": "temp_value"})
	result := tool.Execute(map[string]string{"action": "delete", "key": "temp_key"})
	if result.Error != "" {
		t.Fatalf("delete failed: %s", result.Error)
	}

	result = tool.Execute(map[string]string{"action": "load", "key": "temp_key"})
	if contains(result.Output, "temp_value") {
		t.Fatal("entry should have been deleted")
	}
}

func TestMemoryToolUpdateExisting(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{"action": "save", "key": "version", "value": "1.0"})
	tool.Execute(map[string]string{"action": "save", "key": "version", "value": "2.0"})

	result := tool.Execute(map[string]string{"action": "load", "key": "version"})
	if !contains(result.Output, "2.0") {
		t.Fatalf("expected 2.0, got: %s", result.Output)
	}
	if contains(result.Output, "1.0") {
		t.Fatalf("old value should be replaced, got: %s", result.Output)
	}
}

func TestMemoryToolEmpty(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	result := tool.Execute(map[string]string{"action": "list"})
	if !contains(result.Output, "empty") {
		t.Fatalf("expected empty message, got: %s", result.Output)
	}
}

func TestMemoryToolMissingParams(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	result := tool.Execute(map[string]string{"action": "save", "value": "test"})
	if result.Error == "" {
		t.Fatal("expected error for missing key")
	}

	result = tool.Execute(map[string]string{"action": "save", "key": "test"})
	if result.Error == "" {
		t.Fatal("expected error for missing value")
	}
}

func TestMemoryToolUnknownAction(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	result := tool.Execute(map[string]string{"action": "unknown"})
	if result.Error == "" {
		t.Fatal("expected error for unknown action")
	}
}

func TestMemoryToolFilePersistence(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")

	tool1 := NewMemoryToolWithPath(fp)
	tool1.Execute(map[string]string{"action": "save", "key": "persistent_key", "value": "persistent_value", "category": "test"})

	tool2 := NewMemoryToolWithPath(fp)
	result := tool2.Execute(map[string]string{"action": "load", "key": "persistent_key"})
	if !contains(result.Output, "persistent_value") {
		t.Fatalf("data should persist across instances, got: %s", result.Output)
	}
}

func TestLoadAllFacts(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{"action": "save", "key": "project_path", "value": "/home/user/project", "category": "project"})
	tool.Execute(map[string]string{"action": "save", "key": "db_host", "value": "localhost:5432", "category": "database"})

	prompt := tool.LoadAllFacts()
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !contains(prompt, "Important facts") {
		t.Fatalf("expected header, got: %s", prompt)
	}
	if !contains(prompt, "/home/user/project") {
		t.Fatalf("expected project_path, got: %s", prompt)
	}
	if !contains(prompt, "localhost:5432") {
		t.Fatalf("expected db_host, got: %s", prompt)
	}
}

func TestLoadAllFactsEmpty(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	prompt := tool.LoadAllFacts()
	if prompt != "" {
		t.Fatalf("expected empty prompt for non-existent file, got: %s", prompt)
	}
}

func TestMemoryToolCategoryFilter(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{"action": "save", "key": "k1", "value": "v1", "category": "alpha"})
	tool.Execute(map[string]string{"action": "save", "key": "k2", "value": "v2", "category": "beta"})

	result := tool.Execute(map[string]string{"action": "load", "category": "alpha"})
	if !contains(result.Output, "v1") {
		t.Fatalf("expected v1, got: %s", result.Output)
	}
	if contains(result.Output, "v2") {
		t.Fatalf("should not contain v2 from other category, got: %s", result.Output)
	}
}

func TestMemoryToolFileFormat(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{"action": "save", "key": "test_key", "value": "test_value", "category": "test_cat"})

	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !contains(content, "# BugBuster Agent Memory") {
		t.Fatalf("expected header, got: %s", content)
	}
	if !contains(content, "## test_cat") {
		t.Fatalf("expected category header, got: %s", content)
	}
	if !contains(content, "- **test_key**: test_value") {
		t.Fatalf("expected entry, got: %s", content)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
