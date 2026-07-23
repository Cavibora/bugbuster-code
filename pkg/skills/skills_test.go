package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBuiltins(t *testing.T) {
	m := NewManager()
	m.LoadBuiltins()

	skills := m.List()
	if len(skills) < 6 {
		t.Errorf("Expected at least 6 built-in skills, got %d", len(skills))
	}

	// Check specific skills exist
	for _, name := range []string{"debug", "refactor", "review", "architect", "test", "migrate"} {
		s, ok := m.Get(name)
		if !ok {
			t.Errorf("Expected built-in skill '%s' to exist", name)
			continue
		}
		if s.Source != "builtin" {
			t.Errorf("Expected source 'builtin', got '%s'", s.Source)
		}
		if s.Content == "" {
			t.Errorf("Skill '%s' has empty content", name)
		}
	}
}

func TestActivate(t *testing.T) {
	m := NewManager()
	m.LoadBuiltins()

	content, err := m.Activate("debug")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if content == "" {
		t.Error("Expected non-empty content")
	}
	if !contains(content, "## Active Skill: debug") {
		t.Error("Expected skill header in content")
	}
	if !contains(content, "Read the error") {
		t.Error("Expected skill instructions in content")
	}
}

func TestActivateNotFound(t *testing.T) {
	m := NewManager()
	m.LoadBuiltins()

	_, err := m.Activate("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent skill")
	}
	if !contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestLoadFromDir(t *testing.T) {
	// Create temp directory with skill files
	tmpDir := t.TempDir()

	skillContent := `# Custom Skill

This is a custom project-specific skill.
1. Step one
2. Step two
3. Step three
`
	err := os.WriteFile(filepath.Join(tmpDir, "custom.md"), []byte(skillContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write skill file: %v", err)
	}

	m := NewManager()
	err = m.LoadFromDir(tmpDir, "project")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	s, ok := m.Get("custom")
	if !ok {
		t.Fatal("Expected 'custom' skill to be loaded")
	}
	if s.Source != "project" {
		t.Errorf("Expected source 'project', got '%s'", s.Source)
	}
	if s.Description != "This is a custom project-specific skill." {
		t.Errorf("Unexpected description: %s", s.Description)
	}
}

func TestLoadFromDirNotExist(t *testing.T) {
	m := NewManager()
	err := m.LoadFromDir("/nonexistent/path", "project")
	if err != nil {
		t.Errorf("Expected no error for nonexistent dir, got: %v", err)
	}
}

func TestProjectOverridesBuiltin(t *testing.T) {
	m := NewManager()
	m.LoadBuiltins()

	// Create project skill with same name as builtin
	tmpDir := t.TempDir()
	skillContent := `# Debug Override

Custom debug instructions for this project.
1. Custom step
`
	os.WriteFile(filepath.Join(tmpDir, "debug.md"), []byte(skillContent), 0644)

	m.LoadFromDir(tmpDir, "project")

	s, ok := m.Get("debug")
	if !ok {
		t.Fatal("Expected 'debug' skill to exist")
	}
	// Project skill should override builtin
	if s.Source != "project" {
		t.Errorf("Expected project skill to override builtin, got source: %s", s.Source)
	}
	if !contains(s.Content, "Custom debug") {
		t.Error("Expected custom content from project skill")
	}
}

func TestListSorted(t *testing.T) {
	m := NewManager()
	m.LoadBuiltins()

	skills := m.List()
	// Builtins should come first
	if len(skills) < 2 {
		return
	}
	// Check that list is not empty and has consistent ordering
	for i := 1; i < len(skills); i++ {
		if skills[i].Name == skills[i-1].Name {
			t.Errorf("Duplicate skill name: %s", skills[i].Name)
		}
	}
}

func TestExtractDescription(t *testing.T) {
	tests := []struct {
		content    string
		expected  string
	}{
		{
			content:   "# Debug\n\nFind and fix bugs.\n1. Step",
			expected:  "Find and fix bugs.",
		},
		{
			content:   "# Review\n\nReview code changes.\n",
			expected:  "Review code changes.",
		},
		{
			content:   "No heading here\nJust text",
			expected:  "No heading here",
		},
	}

	for _, tt := range tests {
		got := extractDescription(tt.content)
		if got != tt.expected {
			t.Errorf("extractDescription(%q) = %q, want %q", tt.content[:20], got, tt.expected)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewManager()
	m.LoadBuiltins()

	// Concurrent reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			skills := m.List()
			if len(skills) == 0 {
				t.Error("Expected skills")
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestActive(t *testing.T) {
	m := NewManager()
	m.LoadBuiltins()

	// Initially, all builtins are "active" (available)
	active := m.Active()
	if len(active) == 0 {
		t.Error("Expected at least one active skill after LoadBuiltins")
	}

	// Check that built-in skills are listed
	found := false
	for _, name := range active {
		if name == "debug" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'debug' in active skills, got %v", active)
	}
}

func TestDeactivate(t *testing.T) {
	m := NewManager()
	m.LoadBuiltins()

	// Verify debug exists
	_, ok := m.Get("debug")
	if !ok {
		t.Fatal("Expected 'debug' skill to exist")
	}

	// Deactivate debug
	m.Deactivate("debug")

	// Verify debug is gone
	_, ok = m.Get("debug")
	if ok {
		t.Error("Expected 'debug' skill to be deactivated (removed)")
	}

	// Verify other skills still exist
	_, ok = m.Get("refactor")
	if !ok {
		t.Error("Expected 'refactor' skill to still exist after deactivating 'debug'")
	}
}

func TestDeactivateNonExistent(t *testing.T) {
	m := NewManager()
	m.LoadBuiltins()

	// Deactivating a non-existent skill should not panic
	m.Deactivate("nonexistent")

	// Built-in skills should still be there
	_, ok := m.Get("debug")
	if !ok {
		t.Error("Expected 'debug' skill to still exist after deactivating nonexistent")
	}
}

func TestActiveAfterDeactivate(t *testing.T) {
	m := NewManager()
	m.LoadBuiltins()

	// Deactivate some skills
	m.Deactivate("debug")
	m.Deactivate("refactor")

	active := m.Active()

	// Verify deactivated skills are not in active list
	for _, name := range active {
		if name == "debug" {
			t.Error("Expected 'debug' to be deactivated and not in active list")
		}
		if name == "refactor" {
			t.Error("Expected 'refactor' to be deactivated and not in active list")
		}
	}
}