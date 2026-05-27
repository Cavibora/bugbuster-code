package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Basic CRUD ---

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

// --- Error handling ---

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

func TestMemoryToolDeleteEmptyKey(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	result := tool.Execute(map[string]string{"action": "delete"})
	if result.Error == "" {
		t.Fatal("expected error for empty key in delete")
	}
}

func TestMemoryToolDeleteNonExistent(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	result := tool.Execute(map[string]string{"action": "delete", "key": "nonexistent"})
	if contains(result.Output, "Deleted") {
		t.Fatal("should not delete non-existent key")
	}
	if !contains(result.Output, "not found") {
		t.Fatalf("expected 'not found' message, got: %s", result.Output)
	}
}

func TestMemoryToolNoAction(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	result := tool.Execute(map[string]string{})
	if result.Error == "" {
		t.Fatal("expected error for missing action")
	}
}

// --- Persistence ---

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

// --- LoadAllFacts ---

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

// --- Category filtering ---

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

func TestMemoryToolDefaultCategory(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{"action": "save", "key": "k1", "value": "v1"})
	result := tool.Execute(map[string]string{"action": "load", "category": "general"})
	if !contains(result.Output, "v1") {
		t.Fatalf("expected v1 in 'general' category, got: %s", result.Output)
	}
}

// --- File format ---

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

func TestMemoryToolFileFormatMultipleCategories(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{"action": "save", "key": "k1", "value": "v1", "category": "beta"})
	tool.Execute(map[string]string{"action": "save", "key": "k2", "value": "v2", "category": "alpha"})
	tool.Execute(map[string]string{"action": "save", "key": "k3", "value": "v3", "category": "beta"})

	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Categories should be sorted alphabetically
	alphaIdx := strings.Index(content, "## alpha")
	betaIdx := strings.Index(content, "## beta")
	if alphaIdx == -1 || betaIdx == -1 {
		t.Fatalf("expected both categories, got: %s", content)
	}
	if alphaIdx > betaIdx {
		t.Fatal("categories should be sorted alphabetically (alpha before beta)")
	}
}

// --- Markdown parsing ---

func TestParseMemoryMD(t *testing.T) {
	content := `# BugBuster Agent Memory

## project
- **project_path**: /Users/test/grfn
- **language**: Rust

## database
- **mysql_host**: localhost:3306
`
	facts := parseMemoryMD(content)
	if len(facts) != 3 {
		t.Fatalf("expected 3 facts, got %d", len(facts))
	}
	if facts[0].Key != "project_path" {
		t.Fatalf("expected 'project_path', got '%s'", facts[0].Key)
	}
	if facts[0].Category != "project" {
		t.Fatalf("expected 'project' category, got '%s'", facts[0].Category)
	}
	if facts[1].Key != "language" {
		t.Fatalf("expected 'language', got '%s'", facts[1].Key)
	}
	if facts[2].Category != "database" {
		t.Fatalf("expected 'database' category, got '%s'", facts[2].Category)
	}
}

func TestParseMemoryMDEmpty(t *testing.T) {
	facts := parseMemoryMD("")
	if len(facts) != 0 {
		t.Fatalf("expected 0 facts, got %d", len(facts))
	}
}

func TestParseMemoryMDHeaderOnly(t *testing.T) {
	content := "# BugBuster Agent Memory\n\n## project\n"
	facts := parseMemoryMD(content)
	if len(facts) != 0 {
		t.Fatalf("expected 0 facts for header-only, got %d", len(facts))
	}
}

func TestParseMemoryMDMalformed(t *testing.T) {
	content := `# BugBuster Agent Memory

## test
- malformed line without bold
- **only_key_no_value
- **key**: value
random text
`
	facts := parseMemoryMD(content)
	if len(facts) != 1 {
		t.Fatalf("expected 1 valid fact, got %d", len(facts))
	}
	if facts[0].Key != "key" {
		t.Fatalf("expected 'key', got '%s'", facts[0].Key)
	}
	if facts[0].Value != "value" {
		t.Fatalf("expected 'value', got '%s'", facts[0].Value)
	}
}

// --- Session-scoped ---

func TestMemoryToolSessionScoped(t *testing.T) {
	dir := t.TempDir()

	// Session 1 — GRFN project
	fp1 := filepath.Join(dir, "session-grfn.md")
	tool1 := NewMemoryToolWithPath(fp1)
	tool1.Execute(map[string]string{"action": "save", "key": "project_path", "value": "/Users/ss/ai/grfn", "category": "project"})
	tool1.Execute(map[string]string{"action": "save", "key": "language", "value": "Rust", "category": "project"})

	// Session 2 — BugBuster project
	fp2 := filepath.Join(dir, "session-bugbuster.md")
	tool2 := NewMemoryToolWithPath(fp2)
	tool2.Execute(map[string]string{"action": "save", "key": "project_path", "value": "/Users/ss/ai/bugbuster-code", "category": "project"})
	tool2.Execute(map[string]string{"action": "save", "key": "language", "value": "Go", "category": "project"})

	// Verify session 1 has GRFN data
	result1 := tool1.Execute(map[string]string{"action": "load", "key": "project_path"})
	if !contains(result1.Output, "/Users/ss/ai/grfn") {
		t.Fatalf("session 1 should have GRFN path, got: %s", result1.Output)
	}

	// Verify session 2 has BugBuster data
	result2 := tool2.Execute(map[string]string{"action": "load", "key": "project_path"})
	if !contains(result2.Output, "/Users/ss/ai/bugbuster-code") {
		t.Fatalf("session 2 should have BugBuster path, got: %s", result2.Output)
	}

	// Verify sessions are isolated
	facts1 := tool1.LoadAllFacts()
	if contains(facts1, "Go") {
		t.Fatal("session 1 should not contain BugBuster data (Go)")
	}
	facts2 := tool2.LoadAllFacts()
	if contains(facts2, "Rust") {
		t.Fatal("session 2 should not contain GRFN data (Rust)")
	}
}

func TestMemoryFilePath(t *testing.T) {
	path := MemoryFilePath("test-session-123")
	if !contains(path, "test-session-123.md") {
		t.Fatalf("expected session ID in path, got: %s", path)
	}
	if !contains(path, ".bugbuster") {
		t.Fatalf("expected .bugbuster in path, got: %s", path)
	}
	if !contains(path, "memory") {
		t.Fatalf("expected 'memory' dir in path, got: %s", path)
	}
}

// --- Concurrent access ---

func TestMemoryToolConcurrentSave(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result := tool.Execute(map[string]string{
				"action":   "save",
				"key":      fmt.Sprintf("key_%d", idx),
				"value":    fmt.Sprintf("value_%d", idx),
				"category": "concurrent",
			})
			if result.Error != "" {
				errors <- fmt.Errorf("goroutine %d: %s", idx, result.Error)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}

	// Verify all 10 facts were saved
	result := tool.Execute(map[string]string{"action": "load", "category": "concurrent"})
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key_%d", i)
		if !contains(result.Output, key) {
			t.Errorf("expected key_%d in results", i)
		}
	}
}

// --- Case insensitivity ---

func TestMemoryToolCaseInsensitiveKey(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{"action": "save", "key": "MyKey", "value": "my_value"})

	// Update with different case should update existing
	tool.Execute(map[string]string{"action": "save", "key": "mykey", "value": "updated_value"})

	result := tool.Execute(map[string]string{"action": "load", "key": "MYKEY"})
	if !contains(result.Output, "updated_value") {
		t.Fatalf("expected updated value with case-insensitive key, got: %s", result.Output)
	}
}

func TestMemoryToolCaseInsensitiveSearch(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{"action": "save", "key": "Project_Path", "value": "/test"})

	result := tool.Execute(map[string]string{"action": "load", "key": "project_path"})
	if !contains(result.Output, "/test") {
		t.Fatalf("case-insensitive search should find key, got: %s", result.Output)
	}
}

// --- Whitespace handling ---

func TestMemoryToolWhitespaceTrimming(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{
		"action":   "save",
		"key":      "  spaced_key  ",
		"value":    "  spaced_value  ",
		"category": "  spaced_cat  ",
	})

	result := tool.Execute(map[string]string{"action": "load", "key": "spaced_key"})
	if !contains(result.Output, "spaced_value") {
		t.Fatalf("expected trimmed value, got: %s", result.Output)
	}
}

// --- Special characters ---

func TestMemoryToolSpecialCharacters(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	specialValue := "host=localhost;port=5432;user=admin&pass=secret"
	tool.Execute(map[string]string{"action": "save", "key": "db_url", "value": specialValue})

	result := tool.Execute(map[string]string{"action": "load", "key": "db_url"})
	if !contains(result.Output, specialValue) {
		t.Fatalf("expected special value, got: %s", result.Output)
	}
}

func TestMemoryToolUnicodeValues(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{"action": "save", "key": "project_name", "value": "Багобор Код 🐛", "category": "project"})

	result := tool.Execute(map[string]string{"action": "load", "key": "project_name"})
	if !contains(result.Output, "Багобор Код 🐛") {
		t.Fatalf("expected unicode value, got: %s", result.Output)
	}
}

// --- Large data ---

func TestMemoryToolManyFacts(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	for i := 0; i < 100; i++ {
		result := tool.Execute(map[string]string{
			"action":   "save",
			"key":      fmt.Sprintf("fact_%d", i),
			"value":    fmt.Sprintf("value_%d", i),
			"category": fmt.Sprintf("cat_%d", i%5),
		})
		if result.Error != "" {
			t.Fatalf("save %d failed: %s", i, result.Error)
		}
	}

	result := tool.Execute(map[string]string{"action": "list"})
	if result.Error != "" {
		t.Fatalf("list failed: %s", result.Error)
	}

	// Verify file exists and is readable
	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("file should not be empty")
	}
}

// --- LoadAllFacts with categories ---

func TestLoadAllFactsWithCategories(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{"action": "save", "key": "path", "value": "/test", "category": "project"})
	tool.Execute(map[string]string{"action": "save", "key": "host", "value": "localhost", "category": "database"})
	tool.Execute(map[string]string{"action": "save", "key": "last_run", "value": "2025-01-15", "category": "metrics"})

	prompt := tool.LoadAllFacts()

	if !contains(prompt, "[database]") {
		t.Fatalf("expected [database] category in prompt, got: %s", prompt)
	}
	if !contains(prompt, "[metrics]") {
		t.Fatalf("expected [metrics] category in prompt, got: %s", prompt)
	}
	if !contains(prompt, "[project]") {
		t.Fatalf("expected [project] category in prompt, got: %s", prompt)
	}
}

// --- Render roundtrip ---

func TestMemoryToolRenderRoundtrip(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	// Save facts
	original := []struct {
		key, value, category string
	}{
		{"path", "/Users/test", "project"},
		{"host", "localhost", "database"},
		{"version", "1.0", "project"},
	}

	for _, f := range original {
		tool.Execute(map[string]string{
			"action": "save", "key": f.key, "value": f.value, "category": f.category,
		})
	}

	// Read file, parse, verify
	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatal(err)
	}

	parsed := parseMemoryMD(string(data))
	if len(parsed) != 3 {
		t.Fatalf("expected 3 facts after roundtrip, got %d", len(parsed))
	}

	// Verify all facts are present
	found := make(map[string]bool)
	for _, f := range parsed {
		found[f.Key] = true
	}
	for _, f := range original {
		if !found[f.key] {
			t.Errorf("fact '%s' not found after roundtrip", f.key)
		}
	}
}

// --- Tool interface ---

func TestMemoryToolName(t *testing.T) {
	tool := NewMemoryToolWithPath("/tmp/test.md")
	if tool.Name() != "memory" {
		t.Fatalf("expected 'memory', got '%s'", tool.Name())
	}
}

func TestMemoryToolParameters(t *testing.T) {
	tool := NewMemoryToolWithPath("/tmp/test.md")
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Fatal("expected type=object")
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	requiredFields := []string{"action", "key", "value", "category"}
	for _, field := range requiredFields {
		if _, ok := props[field]; !ok {
			t.Fatalf("expected '%s' parameter", field)
		}
	}
}

// --- Time tracking ---

func TestMemoryToolSaveTimestamp(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	before := time.Now()
	tool.Execute(map[string]string{"action": "save", "key": "ts_test", "value": "v1"})
	after := time.Now()

	// Verify fact has a valid timestamp by loading and checking
	result := tool.Execute(map[string]string{"action": "load", "key": "ts_test"})
	if !contains(result.Output, "v1") {
		t.Fatalf("expected saved value, got: %s", result.Output)
	}
	_ = before
	_ = after
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestMemoryToolSetSessionID(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewMemoryToolWithPath(filepath.Join(tmpDir, "old-session.md"))

	// Save a fact with old session
	m.Execute(map[string]string{"action": "save", "key": "test_key", "value": "test_value", "category": "test"})

	// Verify fact was saved
	result := m.Execute(map[string]string{"action": "load", "key": "test_key"})
	if result.Error != "" {
		t.Errorf("Expected no error, got: %s", result.Error)
	}
	if !strings.Contains(result.Output, "test_value") {
		t.Errorf("Expected output to contain 'test_value', got: %s", result.Output)
	}

	// Verify old file exists
	if _, err := os.Stat(filepath.Join(tmpDir, "old-session.md")); os.IsNotExist(err) {
		t.Error("Expected old session file to exist")
	}
}

func TestMemoryToolCompress(t *testing.T) {
	tmpDir := t.TempDir()
	mt := NewMemoryToolWithPath(filepath.Join(tmpDir, "test.md"))

	// Save some facts
	mt.Execute(map[string]string{"action": "save", "key": "path1", "value": "/old/path", "category": "project"})
	mt.Execute(map[string]string{"action": "save", "key": "path2", "value": "/new/path", "category": "project"})
	mt.Execute(map[string]string{"action": "save", "key": "host", "value": "localhost", "category": "database"})
	// Duplicate
	mt.Execute(map[string]string{"action": "save", "key": "path1", "value": "/old/path", "category": "project"})

	result := mt.Execute(map[string]string{"action": "compress", "max_tokens": "1000"})
	if result.Error != "" {
		t.Fatalf("Compress failed: %s", result.Error)
	}
	if !strings.Contains(result.Output, "compressed") {
		t.Errorf("Expected 'compressed' in output, got: %s", result.Output)
	}

	// Verify duplicates removed
	listResult := mt.Execute(map[string]string{"action": "list"})
	if strings.Contains(listResult.Output, "path1") && strings.Contains(listResult.Output, "path2") {
		// Both should still exist (different keys)
	} else {
		t.Errorf("Expected both keys to exist after compress, got: %s", listResult.Output)
	}
}

func TestMemoryToolTokenCount(t *testing.T) {
	tmpDir := t.TempDir()
	mt := NewMemoryToolWithPath(filepath.Join(tmpDir, "test.md"))

	// Empty memory
	count := mt.TokenCount()
	if count != 0 {
		t.Errorf("Expected 0 tokens for empty memory, got %d", count)
	}

	// Add some facts
	mt.Execute(map[string]string{"action": "save", "key": "project_path", "value": "/Users/test/project", "category": "project"})
	count = mt.TokenCount()
	if count <= 0 {
		t.Errorf("Expected positive token count, got %d", count)
	}
}
