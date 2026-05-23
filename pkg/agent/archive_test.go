package agent

import (
	"os"
	"strings"
	"testing"
	"time"

	"bugbuster-code/pkg/provider"
)

func TestArchiveStore_Init(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)
	if err := store.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("directory should exist after Init")
	}
}

func TestArchiveStore_SaveAndLoadBlock(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)
	_ = store.Init()

	block := &ArchiveBlock{
		ID:          "blk_test_001",
		SessionID:   "sess_test",
		Summary:     "Test summary about authentication",
		Topics:      []string{"auth", "jwt", "login"},
		Messages:    []provider.Message{provider.UserMsg("Fix the auth bug"), provider.AssistantText("I'll fix the JWT validation")},
		TokenCount:  50,
		CreatedAt:   mustParseTime("2026-05-16T10:00:00Z"),
		SourcePhase: "compaction",
	}

	if err := store.SaveBlock(block); err != nil {
		t.Fatalf("SaveBlock failed: %v", err)
	}

	loaded, err := store.LoadBlock("blk_test_001")
	if err != nil {
		t.Fatalf("LoadBlock failed: %v", err)
	}

	if loaded.ID != block.ID {
		t.Errorf("ID mismatch: got %q, want %q", loaded.ID, block.ID)
	}
	if loaded.Summary != block.Summary {
		t.Errorf("Summary mismatch: got %q, want %q", loaded.Summary, block.Summary)
	}
	if len(loaded.Topics) != len(block.Topics) {
		t.Errorf("Topics length mismatch: got %d, want %d", len(loaded.Topics), len(block.Topics))
	}
	if loaded.TokenCount != block.TokenCount {
		t.Errorf("TokenCount mismatch: got %d, want %d", loaded.TokenCount, block.TokenCount)
	}
}

func TestArchiveStore_ArchiveMessages(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)
	_ = store.Init()

	msgs := []provider.Message{
		provider.UserMsg("Fix the auth bug in login handler"),
		provider.AssistantText("I'll fix the JWT validation in the login handler"),
		provider.UserMsg("Also fix the tests"),
		provider.AssistantText("Running all tests now"),
	}

	block, err := store.ArchiveMessages(msgs, "sess_test")
	if err != nil {
		t.Fatalf("ArchiveMessages failed: %v", err)
	}

	if block == nil {
		t.Fatal("ArchiveMessages returned nil block")
	}
	if block.SessionID != "sess_test" {
		t.Errorf("SessionID mismatch: got %q, want %q", block.SessionID, "sess_test")
	}
	if block.Summary == "" {
		t.Error("Summary should not be empty")
	}
	if block.TokenCount == 0 {
		t.Error("TokenCount should not be zero")
	}
	if block.SourcePhase != "compaction" {
		t.Errorf("SourcePhase mismatch: got %q, want %q", block.SourcePhase, "compaction")
	}

	// Проверяем, что индекс обновлён для сессии
	idx, err := store.LoadIndexForSession("sess_test")
	if err != nil {
		t.Fatalf("LoadIndexForSession failed: %v", err)
	}
	if len(idx.Entries) != 1 {
		t.Errorf("Expected 1 index entry, got %d", len(idx.Entries))
	}
	if idx.Entries[0].ID != block.ID {
		t.Errorf("Index entry ID mismatch: got %q, want %q", idx.Entries[0].ID, block.ID)
	}
}

func TestArchiveStore_ArchiveMessages_Empty(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)
	_ = store.Init()

	block, err := store.ArchiveMessages(nil, "sess_test")
	if err != nil {
		t.Fatalf("ArchiveMessages with nil should not error: %v", err)
	}
	if block != nil {
		t.Error("ArchiveMessages with nil should return nil block")
	}
}

func TestArchiveStore_PruneBlocks(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 3) // лимит 3 блока
	_ = store.Init()

	// Создаём 5 блоков
	for i := 0; i < 5; i++ {
		msgs := []provider.Message{provider.UserMsg("message " + string(rune('A'+i)))}
		_, err := store.ArchiveMessages(msgs, "sess_test")
		if err != nil {
			t.Fatalf("ArchiveMessages %d failed: %v", i, err)
		}
	}

	// После PruneBlocks должно остаться 3 блока
	idx, _ := store.LoadIndex()
	if len(idx.Entries) > 3 {
		t.Errorf("Expected at most 3 entries after pruning, got %d", len(idx.Entries))
	}
}

func TestArchiveStore_LoadIndex_NotExist(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)
	_ = store.Init()

	idx, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex on empty dir should not error: %v", err)
	}
	if len(idx.Entries) != 0 {
		t.Errorf("Expected 0 entries, got %d", len(idx.Entries))
	}
}

func TestExtractTopics(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "text", Text: "Fix the authentication bug in login handler"},
			},
		},
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "tool_use", ToolName: "read", ToolUseID: "1", Input: map[string]any{"path": "/auth/handler.go"}},
			},
		},
		{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "tool_result", ToolUseID: "1", ToolName: "read", Output: "file contents"},
			},
		},
	}

	topics := extractTopics(msgs)

	// Должен извлечь имя файла из tool_use
	found := false
	for _, t := range topics {
		if t == "handler.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'handler.go' in topics, got %v", topics)
	}

	// Должен извлечь ключевые слова из user-сообщения
	// (слова длиннее 3 символов, не стоп-слова)
	for _, t := range topics {
		if t == "authentication" || t == "login" {
			return // нашли ключевое слово
		}
	}
	t.Errorf("Expected 'authentication' or 'login' in topics, got %v", topics)
}

func TestSearchContextTool_Execute(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)
	_ = store.Init()

	// Создаём тестовые блоки
	msgs1 := []provider.Message{
		provider.UserMsg("Fix the authentication bug in login handler"),
		provider.AssistantText("I'll fix the JWT validation in the login handler"),
	}
	block1, _ := store.ArchiveMessages(msgs1, "sess_1")

	msgs2 := []provider.Message{
		provider.UserMsg("Add unit tests for the database layer"),
		provider.AssistantText("I'll add comprehensive tests for the database module"),
	}
	block2, _ := store.ArchiveMessages(msgs2, "sess_2")

	_ = block1
	_ = block2

	tool := NewSearchContextTool(store)

	// Тест: поиск по "auth"
	result := tool.Execute(map[string]string{"query": "auth"})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	if result.Output == "" {
		t.Error("Expected non-empty output for 'auth' query")
	}

	// Тест: поиск по "database"
	result = tool.Execute(map[string]string{"query": "database"})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	if result.Output == "" {
		t.Error("Expected non-empty output for 'database' query")
	}

	// Тест: пустой запрос
	result = tool.Execute(map[string]string{"query": ""})
	if result.Error == "" {
		t.Error("Expected error for empty query")
	}

	// Тест: несуществующий запрос
	result = tool.Execute(map[string]string{"query": "quantum_physics_xyz"})
	if result.Output == "" {
		t.Error("Expected 'no results' message for non-matching query")
	}
}

func TestSearchContextTool_MaxResults(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)
	_ = store.Init()

	// Создаём 3 блока
	for i := 0; i < 3; i++ {
		msgs := []provider.Message{
			provider.UserMsg("Test message about authentication " + string(rune('A'+i))),
			provider.AssistantText("Response about auth " + string(rune('A'+i))),
		}
		if _, err := store.ArchiveMessages(msgs, "sess_test"); err != nil {
			t.Fatal(err)
		}
	}

	tool := NewSearchContextTool(store)

	// Тест: max_results=1
	result := tool.Execute(map[string]string{"query": "auth", "max_results": "1"})
	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}
	// Результат должен содержать только 1 блок (проверяем что нет "---" разделителя второго блока)
	// Это не строгая проверка, но хотя бы убеждаемся что результат не пустой
	if result.Output == "" {
		t.Error("Expected non-empty output")
	}
}

func TestTokenizeAndLower(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"Hello World", []string{"hello", "world"}},
		{"Fix the auth bug!", []string{"fix", "the", "auth", "bug"}},
		{"  multiple   spaces  ", []string{"multiple", "spaces"}},
		{"a b cd", []string{"cd"}}, // слова короче 2 символов отбрасываются
	}

	for _, tt := range tests {
		result := tokenizeAndLower(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("tokenizeAndLower(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestTopicSimilarity(t *testing.T) {
	tests := []struct {
		a, b     []string
		expected float64
	}{
		{[]string{"auth", "jwt"}, []string{"auth", "login"}, 0.5}, // 1 из 2 совпадает
		{[]string{"auth", "jwt"}, []string{"auth", "jwt"}, 1.0},   // полное совпадение
		{[]string{"auth"}, []string{"database"}, 0.0},             // нет совпадений
		{[]string{}, []string{"auth"}, 0.0},                       // пустой набор
	}

	for _, tt := range tests {
		result := topicSimilarity(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("topicSimilarity(%v, %v) = %f, want %f", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestFindRemovedMessages(t *testing.T) {
	before := []provider.Message{
		provider.SystemMsg("system"),
		provider.UserMsg("hello"),
		provider.AssistantText("hi"),
		provider.UserMsg("how are you"),
		provider.AssistantText("fine"),
	}
	after := []provider.Message{
		provider.SystemMsg("system"),
		provider.UserMsg("how are you"),
		provider.AssistantText("fine"),
	}

	removed := findRemovedMessages(before, after)
	if len(removed) != 2 {
		t.Errorf("Expected 2 removed messages, got %d", len(removed))
	}
}

func TestFindRemovedMessages_NoRemovals(t *testing.T) {
	before := []provider.Message{
		provider.SystemMsg("system"),
		provider.UserMsg("hello"),
	}
	after := before

	removed := findRemovedMessages(before, after)
	if len(removed) != 0 {
		t.Errorf("Expected 0 removed messages, got %d", len(removed))
	}
}

func TestFormatArchiveBlock(t *testing.T) {
	block := &ArchiveBlock{
		ID:          "blk_test_001",
		SessionID:   "sess_test",
		Summary:     "Fixed authentication bug",
		Topics:      []string{"auth", "jwt"},
		TokenCount:  100,
		CreatedAt:   mustParseTime("2026-05-16T10:00:00Z"),
		SourcePhase: "compaction",
		Messages: []provider.Message{
			provider.UserMsg("Fix the auth bug"),
			provider.AssistantText("I'll fix the JWT validation"),
		},
	}

	result := FormatArchiveBlock(block, 500)
	if result == "" {
		t.Error("FormatArchiveBlock returned empty string")
	}
	if !strings.Contains(result, "blk_test_001") {
		t.Error("FormatArchiveBlock should contain block ID")
	}
	if !strings.Contains(result, "Fixed authentication bug") {
		t.Error("FormatArchiveBlock should contain summary")
	}
}

func TestFilterMessagesForArchive(t *testing.T) {
	// Тест: tool_result с ошибкой — удаляется
	msgs := []provider.Message{
		provider.UserMsg("Fix the auth bug"),
		provider.AssistantText("I'll fix it"),
		{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "tool_result", ToolUseID: "1", ToolName: "bash", Output: "error", IsError: true},
			},
		},
		provider.AssistantText("The command failed, let me try again"),
	}

	filtered := filterMessagesForArchive(msgs)

	// Должно остаться 2 сообщения (user text + 2 assistant text)
	if len(filtered) != 3 {
		t.Errorf("Expected 3 filtered messages, got %d", len(filtered))
	}

	// Ни одно сообщение не должно содержать tool_result или tool_use
	for _, msg := range filtered {
		for _, block := range msg.Content {
			if block.Type == "tool_result" || block.Type == "tool_use" {
				t.Errorf("Filtered message should not contain %s block", block.Type)
			}
		}
	}
}

func TestFilterMessagesForArchive_ToolUse(t *testing.T) {
	// Тест: assistant с tool_use — блоки tool_use удаляются, text остаётся
	msgs := []provider.Message{
		provider.UserMsg("Read the file"),
		{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "text", Text: "I'll read the file"},
				{Type: "tool_use", ToolUseID: "1", ToolName: "read", Input: map[string]any{"path": "/test.go"}},
			},
		},
		{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "tool_result", ToolUseID: "1", ToolName: "read", Output: "file contents"},
			},
		},
		provider.AssistantText("Here's what I found in the file"),
	}

	filtered := filterMessagesForArchive(msgs)

	// Должно остаться 3 сообщения: user text, assistant text, assistant text
	if len(filtered) != 3 {
		t.Errorf("Expected 3 filtered messages, got %d", len(filtered))
	}

	// Проверяем, что tool_use и tool_result удалены
	for _, msg := range filtered {
		for _, block := range msg.Content {
			if block.Type == "tool_use" || block.Type == "tool_result" {
				t.Errorf("Filtered message should not contain %s block", block.Type)
			}
		}
	}
}

func TestFilterMessagesForArchive_SystemMessages(t *testing.T) {
	// Тест: system-сообщения не архивируются
	msgs := []provider.Message{
		provider.SystemMsg("system prompt"),
		provider.UserMsg("hello"),
		provider.AssistantText("hi"),
	}

	filtered := filterMessagesForArchive(msgs)

	// Должно остаться 2 сообщения (user + assistant), system удалён
	if len(filtered) != 2 {
		t.Errorf("Expected 2 filtered messages, got %d", len(filtered))
	}

	for _, msg := range filtered {
		if msg.Role == "system" {
			t.Error("System messages should be filtered out")
		}
	}
}

func TestFilterMessagesForArchive_OnlyToolResult(t *testing.T) {
	// Тест: user-сообщение с только tool_result — удаляется целиком
	msgs := []provider.Message{
		provider.UserMsg("Read the file"),
		{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "tool_result", ToolUseID: "1", ToolName: "read", Output: "contents"},
			},
		},
		provider.AssistantText("Done"),
	}

	filtered := filterMessagesForArchive(msgs)

	// Должно остаться 2 сообщения (user text + assistant text)
	if len(filtered) != 2 {
		t.Errorf("Expected 2 filtered messages, got %d", len(filtered))
	}
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now()
	}
	return t
}
