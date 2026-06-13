package agent

import (
	"context"
	"testing"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

// --- parseYAMLToolCalls tests ---

func TestParseYAMLToolCalls_ReadTool(t *testing.T) {
	// YAML format requires "tool:" field and indented params
	response := "tool: read\n  path: main.go"
	calls := parseYAMLToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "read" {
		t.Errorf("expected name read, got %s", calls[0].Name)
	}
	if calls[0].Params["path"] != "main.go" {
		t.Errorf("expected path=main.go, got %v", calls[0].Params)
	}
}

func TestParseYAMLToolCalls_BashTool(t *testing.T) {
	response := "tool: bash\n  command: ls -la"
	calls := parseYAMLToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Errorf("expected name bash, got %s", calls[0].Name)
	}
	if calls[0].Params["command"] != "ls -la" {
		t.Errorf("expected command=ls -la, got %v", calls[0].Params)
	}
}

func TestParseYAMLToolCalls_EditTool(t *testing.T) {
	response := "tool: edit\n  path: main.go\n  old: func main()\n  new: func main() { fmt.Println(\"hello\") }"
	calls := parseYAMLToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "edit" {
		t.Errorf("expected name edit, got %s", calls[0].Name)
	}
	if calls[0].Params["path"] != "main.go" {
		t.Errorf("expected path=main.go, got %v", calls[0].Params)
	}
	if calls[0].Params["old"] != "func main()" {
		t.Errorf("expected old=func main(), got %v", calls[0].Params)
	}
}

func TestParseYAMLToolCalls_UnknownTool(t *testing.T) {
	response := "tool: unknown_tool\nparam: value"
	calls := parseYAMLToolCalls(response)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls for unknown tool, got %d", len(calls))
	}
}

func TestParseYAMLToolCalls_NoParams(t *testing.T) {
	// YAML with tool: but no params — should not produce a call
	response := "tool: bash"
	calls := parseYAMLToolCalls(response)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls for YAML without params, got %d", len(calls))
	}
}

func TestParseYAMLToolCalls_MultipleBlocks(t *testing.T) {
	response := "tool: read\n  path: file1.go\n\ntool: bash\n  command: ls"
	calls := parseYAMLToolCalls(response)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Name != "read" {
		t.Errorf("expected first call name=read, got %s", calls[0].Name)
	}
	if calls[1].Name != "bash" {
		t.Errorf("expected second call name=bash, got %s", calls[1].Name)
	}
}

// --- parseMarkdownCodeBlocks tests ---

func TestParseMarkdownCodeBlocks_Bash(t *testing.T) {
	response := "```bash\nls -la /tmp\n```"
	calls := parseMarkdownCodeBlocks(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Errorf("expected name bash, got %s", calls[0].Name)
	}
	if calls[0].Params["command"] != "ls -la /tmp" {
		t.Errorf("expected command=ls -la /tmp, got %v", calls[0].Params)
	}
}

func TestParseMarkdownCodeBlocks_Shell(t *testing.T) {
	response := "```sh\necho hello\n```"
	calls := parseMarkdownCodeBlocks(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Errorf("expected name bash for sh, got %s", calls[0].Name)
	}
}

func TestParseMarkdownCodeBlocks_Python(t *testing.T) {
	response := "```python\nprint('hello')\n```"
	calls := parseMarkdownCodeBlocks(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Errorf("expected name bash for python, got %s", calls[0].Name)
	}
	if calls[0].Params["command"] != "python3 print('hello')" {
		t.Errorf("expected command=python3 print('hello'), got %v", calls[0].Params)
	}
}

func TestParseMarkdownCodeBlocks_JavaScript(t *testing.T) {
	response := "```javascript\nconsole.log('hello')\n```"
	calls := parseMarkdownCodeBlocks(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Errorf("expected name bash for javascript, got %s", calls[0].Name)
	}
}

func TestParseMarkdownCodeBlocks_Go(t *testing.T) {
	response := "```go\nfmt.Println(\"hello\")\n```"
	calls := parseMarkdownCodeBlocks(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Errorf("expected name bash for go, got %s", calls[0].Name)
	}
}

func TestParseMarkdownCodeBlocks_EmptyCode(t *testing.T) {
	response := "```bash\n\n```"
	calls := parseMarkdownCodeBlocks(response)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls for empty code block, got %d", len(calls))
	}
}

func TestParseMarkdownCodeBlocks_UnknownLanguage(t *testing.T) {
	response := "```ruby\nputs 'hello'\n```"
	calls := parseMarkdownCodeBlocks(response)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls for unknown language, got %d", len(calls))
	}
}

func TestParseMarkdownCodeBlocks_MultipleBlocks(t *testing.T) {
	response := "```bash\nls -la\n```\n\n```python\nprint('hello')\n```"
	calls := parseMarkdownCodeBlocks(response)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Errorf("expected first call name=bash, got %s", calls[0].Name)
	}
	if calls[1].Name != "bash" {
		t.Errorf("expected second call name=bash (python), got %s", calls[1].Name)
	}
}

// --- findAutoDetectJSONInText tests ---

func TestFindAutoDetectJSONInText_BashCommand(t *testing.T) {
	text := `I'll run the tests. Here's the command: {"command": "go test ./..."}`
	calls := findAutoDetectJSONInText(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Errorf("expected name bash, got %s", calls[0].Name)
	}
}

func TestFindAutoDetectJSONInText_ReadPath(t *testing.T) {
	text := `Let me read the file: {"path": "/tmp/main.go"}`
	calls := findAutoDetectJSONInText(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "read" {
		t.Errorf("expected name read, got %s", calls[0].Name)
	}
}

func TestFindAutoDetectJSONInText_TodoArray(t *testing.T) {
	text := `[{"id": "1", "subject": "Read files", "status": "in_progress"}, {"id": "2", "subject": "Write code", "status": "pending"}]`
	calls := findAutoDetectJSONInText(text)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Name != "todo_write" {
		t.Errorf("expected name todo_write, got %s", calls[0].Name)
	}
}

func TestFindAutoDetectJSONInText_EmbeddedInText(t *testing.T) {
	text := "I'll update the todo list:\n[{\"id\": \"1\", \"subject\": \"Task\", \"status\": \"completed\"}]\nDone."
	calls := findAutoDetectJSONInText(text)
	if len(calls) < 1 {
		t.Fatalf("expected at least 1 call, got %d", len(calls))
	}
	if calls[0].Name != "todo_write" {
		t.Errorf("expected name todo_write, got %s", calls[0].Name)
	}
}

func TestFindAutoDetectJSONInText_NoToolStructures(t *testing.T) {
	text := "This is just regular text without any JSON tool calls."
	calls := findAutoDetectJSONInText(text)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestFindAutoDetectJSONInText_WriteTool(t *testing.T) {
	text := `{"path": "main.go", "content": "package main\nfunc main() {}"}`
	calls := findAutoDetectJSONInText(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "write" {
		t.Errorf("expected name write, got %s", calls[0].Name)
	}
}

func TestFindAutoDetectJSONInText_EditTool(t *testing.T) {
	text := `{"path": "main.go", "old": "hello", "new": "world"}`
	calls := findAutoDetectJSONInText(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "edit" {
		t.Errorf("expected name edit, got %s", calls[0].Name)
	}
}

func TestFindAutoDetectJSONInText_GrepTool(t *testing.T) {
	text := `{"pattern": "TODO", "path": "src/"}`
	calls := findAutoDetectJSONInText(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "grep" {
		t.Errorf("expected name grep, got %s", calls[0].Name)
	}
}

func TestFindAutoDetectJSONInText_WebFetchTool(t *testing.T) {
	text := `{"url": "https://example.com"}`
	calls := findAutoDetectJSONInText(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "web_fetch" {
		t.Errorf("expected name web_fetch, got %s", calls[0].Name)
	}
}

func TestFindAutoDetectJSONInText_MemoryTool(t *testing.T) {
	text := `{"action": "save", "key": "project_path", "value": "/tmp/project"}`
	calls := findAutoDetectJSONInText(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "memory" {
		t.Errorf("expected name memory, got %s", calls[0].Name)
	}
}

// --- fmtJSON tests ---

func TestFmtJSON_String(t *testing.T) {
	result := fmtJSON("hello")
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestFmtJSON_Float(t *testing.T) {
	result := fmtJSON(float64(42))
	if result != "42" {
		t.Errorf("expected '42', got %q", result)
	}
}

func TestFmtJSON_Bool(t *testing.T) {
	result := fmtJSON(true)
	if result != "true" {
		t.Errorf("expected 'true', got %q", result)
	}
}

func TestFmtJSON_Nil(t *testing.T) {
	result := fmtJSON(nil)
	if result != "" {
		t.Errorf("expected '', got %q", result)
	}
}

func TestFmtJSON_Array(t *testing.T) {
	result := fmtJSON([]interface{}{"a", "b"})
	if result != `["a","b"]` {
		t.Errorf("expected '[\"a\",\"b\"]', got %q", result)
	}
}

func TestFmtJSON_Object(t *testing.T) {
	result := fmtJSON(map[string]interface{}{"key": "value"})
	if result != `{"key":"value"}` {
		t.Errorf("expected '{\"key\":\"value\"}', got %q", result)
	}
}

// --- topicSimilarity tests ---

func TestTopicSimilarity_Identical(t *testing.T) {
	result := topicSimilarity([]string{"go", "test"}, []string{"go", "test"})
	if result != 1.0 {
		t.Errorf("expected 1.0, got %f", result)
	}
}

func TestTopicSimilarity_NoOverlap(t *testing.T) {
	result := topicSimilarity([]string{"go", "test"}, []string{"python", "web"})
	if result != 0.0 {
		t.Errorf("expected 0.0, got %f", result)
	}
}

func TestTopicSimilarity_PartialOverlap(t *testing.T) {
	result := topicSimilarity([]string{"go", "test", "code"}, []string{"go", "web"})
	// 1 match out of min(3, 2) = 2 → 0.5
	if result < 0.49 || result > 0.51 {
		t.Errorf("expected ~0.5, got %f", result)
	}
}

func TestTopicSimilarity_Empty(t *testing.T) {
	result := topicSimilarity([]string{}, []string{"go"})
	if result != 0.0 {
		t.Errorf("expected 0.0 for empty, got %f", result)
	}
}

func TestTopicSimilarity_CaseInsensitive(t *testing.T) {
	result := topicSimilarity([]string{"Go", "Test"}, []string{"go", "test"})
	if result != 1.0 {
		t.Errorf("expected 1.0 for case-insensitive match, got %f", result)
	}
}

// --- ArchiveOptimizer tests ---

func TestArchiveOptimizer_MergeSimilarBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewArchiveStore(tmpDir, 100)

	// Create two blocks with similar topics
	block1 := &ArchiveBlock{
		ID:         "block1",
		Summary:    "Discussion about Go testing",
		Topics:     []string{"go", "testing", "code"},
		TokenCount: 100,
		CreatedAt:  time.Now().Add(-48 * time.Hour),
		Messages: []provider.Message{
			provider.UserMsg("How to write tests in Go?"),
			provider.AssistantText("Use testing package..."),
		},
	}
	block2 := &ArchiveBlock{
		ID:         "block2",
		Summary:    "Discussion about Go benchmarks",
		Topics:     []string{"go", "benchmarking", "code"},
		TokenCount: 80,
		CreatedAt:  time.Now().Add(-36 * time.Hour),
		Messages: []provider.Message{
			provider.UserMsg("How to benchmark in Go?"),
			provider.AssistantText("Use testing.B..."),
		},
	}

	if err := store.SaveBlock(block1); err != nil {
		t.Fatalf("SaveBlock error: %v", err)
	}
	if err := store.SaveBlock(block2); err != nil {
		t.Fatalf("SaveBlock error: %v", err)
	}

	// Create index
	idx := &ArchiveIndex{
		Entries: []ArchiveIndexEntry{
			{ID: "block1", Summary: block1.Summary, Topics: block1.Topics, TokenCount: block1.TokenCount, CreatedAt: block1.CreatedAt},
			{ID: "block2", Summary: block2.Summary, Topics: block2.Topics, TokenCount: block2.TokenCount, CreatedAt: block2.CreatedAt},
		},
	}
	if err := store.SaveIndex(idx); err != nil {
		t.Fatalf("SaveIndex error: %v", err)
	}

	optimizer := NewArchiveOptimizer(store, nil)
	err := optimizer.Optimize(context.Background())
	if err != nil {
		t.Fatalf("Optimize error: %v", err)
	}

	// Block2 should be deleted, block1 should contain merged topics
	_, err = store.LoadBlock("block2")
	if err == nil {
		t.Error("Expected block2 to be deleted after merge")
	}

	mergedBlock, err := store.LoadBlock("block1")
	if err != nil {
		t.Fatalf("LoadBlock error for block1: %v", err)
	}

	// Check that topics are merged
	topicSet := make(map[string]bool)
	for _, t := range mergedBlock.Topics {
		topicSet[t] = true
	}
	if !topicSet["go"] || !topicSet["testing"] || !topicSet["benchmarking"] {
		t.Errorf("Expected merged topics to include go, testing, benchmarking, got %v", mergedBlock.Topics)
	}
}

func TestArchiveOptimizer_RemoveEmptyBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewArchiveStore(tmpDir, 100)

	// Create an empty block
	emptyBlock := &ArchiveBlock{
		ID:         "empty1",
		Summary:    "",
		Topics:     []string{},
		TokenCount: 0,
		CreatedAt:  time.Now(),
		Messages:   []provider.Message{},
	}
	if err := store.SaveBlock(emptyBlock); err != nil {
		t.Fatalf("SaveBlock error: %v", err)
	}

	idx := &ArchiveIndex{
		Entries: []ArchiveIndexEntry{
			{ID: "empty1", Summary: "", Topics: []string{}, TokenCount: 0, CreatedAt: time.Now()},
		},
	}
	if err := store.SaveIndex(idx); err != nil {
		t.Fatalf("SaveIndex error: %v", err)
	}

	optimizer := NewArchiveOptimizer(store, nil)
	err := optimizer.Optimize(context.Background())
	if err != nil {
		t.Fatalf("Optimize error: %v", err)
	}

	// Empty block should be deleted
	_, err = store.LoadBlock("empty1")
	if err == nil {
		t.Error("Expected empty block to be deleted")
	}
}

func TestArchiveOptimizer_GeneralizeOldBlocks(t *testing.T) {
	i18n.Init("en")

	tmpDir := t.TempDir()
	store := NewArchiveStore(tmpDir, 100)

	// Create an old block (older than 24 hours)
	oldBlock := &ArchiveBlock{
		ID:         "old1",
		Summary:    "",
		Topics:     []string{"go", "testing"},
		TokenCount: 200,
		CreatedAt:  time.Now().Add(-48 * time.Hour),
		Optimized:  false,
		Messages: []provider.Message{
			provider.UserMsg("How to write tests in Go?"),
			provider.AssistantText("Use the testing package. Write test functions starting with Test..."),
		},
	}
	if err := store.SaveBlock(oldBlock); err != nil {
		t.Fatalf("SaveBlock error: %v", err)
	}

	idx := &ArchiveIndex{
		Entries: []ArchiveIndexEntry{
			{ID: "old1", Summary: "", Topics: oldBlock.Topics, TokenCount: oldBlock.TokenCount, CreatedAt: oldBlock.CreatedAt},
		},
	}
	if err := store.SaveIndex(idx); err != nil {
		t.Fatalf("SaveIndex error: %v", err)
	}

	optimizer := NewArchiveOptimizer(store, nil) // nil compactor → SimpleSummarize
	err := optimizer.Optimize(context.Background())
	if err != nil {
		t.Fatalf("Optimize error: %v", err)
	}

	// Old block should be generalized (Optimized = true)
	generalized, err := store.LoadBlock("old1")
	if err != nil {
		t.Fatalf("LoadBlock error: %v", err)
	}
	if !generalized.Optimized {
		t.Error("Expected old block to be optimized")
	}
	if generalized.Summary == "" {
		t.Error("Expected old block to have a summary after generalization")
	}
}

func TestArchiveOptimizer_NoBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewArchiveStore(tmpDir, 100)

	// Empty index
	idx := &ArchiveIndex{Entries: []ArchiveIndexEntry{}}
	if err := store.SaveIndex(idx); err != nil {
		t.Fatalf("SaveIndex error: %v", err)
	}

	optimizer := NewArchiveOptimizer(store, nil)
	err := optimizer.Optimize(context.Background())
	if err != nil {
		t.Fatalf("Optimize should not error on empty index: %v", err)
	}
}

func TestArchiveOptimizer_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewArchiveStore(tmpDir, 100)

	// Create a block
	block := &ArchiveBlock{
		ID:         "cancel1",
		Summary:    "Test block",
		Topics:     []string{"test"},
		TokenCount: 100,
		CreatedAt:  time.Now().Add(-48 * time.Hour),
		Optimized:  false,
		Messages: []provider.Message{
			provider.UserMsg("Question"),
			provider.AssistantText("Answer"),
		},
	}
	if err := store.SaveBlock(block); err != nil {
		t.Fatalf("SaveBlock error: %v", err)
	}

	idx := &ArchiveIndex{
		Entries: []ArchiveIndexEntry{
			{ID: "cancel1", Summary: block.Summary, Topics: block.Topics, TokenCount: block.TokenCount, CreatedAt: block.CreatedAt},
		},
	}
	if err := store.SaveIndex(idx); err != nil {
		t.Fatalf("SaveIndex error: %v", err)
	}

	optimizer := NewArchiveOptimizer(store, nil)

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := optimizer.Optimize(ctx)
	// Should not panic, may return context error or succeed
	_ = err
}

// --- ArchiveStore additional tests ---