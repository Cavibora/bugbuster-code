package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

func TestExtractToolCallsInfo(t *testing.T) {
	tests := []struct {
		name     string
		msg      provider.Message
		wantLen  int
		wantName string
	}{
		{
			name: "no tool calls",
			msg: provider.Message{
				Role: "assistant",
				Content: []provider.ContentBlock{
					{Type: "text", Text: "Hello"},
				},
			},
			wantLen: 0,
		},
		{
			name: "single tool call",
			msg: provider.Message{
				Role: "assistant",
				Content: []provider.ContentBlock{
					{Type: "tool_use", ToolName: "read", ToolUseID: "call_1", Input: map[string]any{"path": "test.go"}},
				},
			},
			wantLen:  1,
			wantName: "read",
		},
		{
			name: "multiple tool calls",
			msg: provider.Message{
				Role: "assistant",
				Content: []provider.ContentBlock{
					{Type: "tool_use", ToolName: "read", ToolUseID: "call_1", Input: map[string]any{"path": "a.go"}},
					{Type: "tool_use", ToolName: "bash", ToolUseID: "call_2", Input: map[string]any{"command": "ls"}},
				},
			},
			wantLen:  2,
			wantName: "read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractToolCallsInfo(tt.msg)
			if len(result) != tt.wantLen {
				t.Errorf("got %d results, want %d", len(result), tt.wantLen)
			}
			if tt.wantLen > 0 && result[0].Name != tt.wantName {
				t.Errorf("got name %q, want %q", result[0].Name, tt.wantName)
			}
		})
	}
}

func TestSaveSession(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)
	if err := sm.Init(); err != nil {
		t.Fatal(err)
	}

	// Создаём сессию
	sess := sm.NewSession()

	// Добавляем сообщения
	sess.Messages = append(sess.Messages, provider.UserMsg("Hello"))
	sess.Messages = append(sess.Messages, provider.AssistantText("Hi there!"))

	// Сохраняем
	err := sm.SaveSession(sess)
	if err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Проверяем что файл создан
	sessionFile := filepath.Join(tmpDir, sess.ID+".jsonl")
	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		t.Error("session file not created")
	}

	// Загружаем и проверяем
	loaded, err := sm.LoadSession(sess.ID)
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if loaded.ID != sess.ID {
		t.Errorf("got ID %q, want %q", loaded.ID, sess.ID)
	}
	if len(loaded.Messages) != 2 {
		t.Errorf("got %d messages, want 2", len(loaded.Messages))
	}
}

func TestSaveSession_EmptySession(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)
	if err := sm.Init(); err != nil {
		t.Fatal(err)
	}

	sess := sm.NewSession()

	err := sm.SaveSession(sess)
	if err != nil {
		t.Fatalf("SaveSession empty session failed: %v", err)
	}
}

func TestSaveSession_MultipleMessages(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)
	if err := sm.Init(); err != nil {
		t.Fatal(err)
	}

	sess := sm.NewSession()
	sess.Messages = append(sess.Messages, provider.UserMsg("msg1"))
	sess.Messages = append(sess.Messages, provider.AssistantText("reply1"))
	sess.Messages = append(sess.Messages, provider.UserMsg("msg2"))
	sess.Messages = append(sess.Messages, provider.AssistantText("reply2"))

	err := sm.SaveSession(sess)
	if err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	sessionFile := filepath.Join(tmpDir, sess.ID+".jsonl")
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatalf("failed to read session file: %v", err)
	}
	if len(data) == 0 {
		t.Error("session file is empty")
	}
}

func TestSaveSessionMessages(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)
	if err := sm.Init(); err != nil {
		t.Fatal(err)
	}

	sess := sm.NewSession()
	sess.Messages = append(sess.Messages, provider.UserMsg("test"))

	err := sm.SaveSessionMessages(sess)
	if err != nil {
		t.Fatalf("SaveSessionMessages failed: %v", err)
	}

	sessionFile := filepath.Join(tmpDir, sess.ID+".jsonl")
	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		t.Error("session file not created")
	}
}

func TestSubagentDescription(t *testing.T) {
	i18n.Init("en")
	tool := NewSubagentTool(DefaultSubagentConfig(), nil, nil)
	desc := tool.Description()
	if desc == "" {
		t.Error("Description() returned empty string")
	}
}

func TestGeneralizeOldBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewArchiveStore(tmpDir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	// Создаём блоки
	store.ArchiveMessages([]provider.Message{
		provider.UserMsg("old message 1"),
		provider.AssistantText("old reply 1"),
	}, "session1")

	store.ArchiveMessages([]provider.Message{
		provider.UserMsg("old message 2"),
		provider.AssistantText("old reply 2"),
	}, "session1")

	optimizer := NewArchiveOptimizer(store, nil)
	result := optimizer.Optimize(context.Background())
	_ = result
	// Проверяем что оптимизация не падает
}

func TestRemoveEmptyBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewArchiveStore(tmpDir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	optimizer := NewArchiveOptimizer(store, nil)
	result := optimizer.Optimize(context.Background())
	_ = result
	// Проверяем что оптимизация пустого архива не падает
}

func TestRebuildIndex(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewArchiveStore(tmpDir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	// Создаём блок
	store.ArchiveMessages([]provider.Message{
		provider.UserMsg("test"),
		provider.AssistantText("reply"),
	}, "session1")

	optimizer := NewArchiveOptimizer(store, nil)
	result := optimizer.Optimize(context.Background())
	_ = result
}

func TestMergeSimilarBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewArchiveStore(tmpDir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	// Создаём похожие блоки
	store.ArchiveMessages([]provider.Message{
		provider.UserMsg("read file.go"),
		provider.AssistantText("package main"),
	}, "session1")

	store.ArchiveMessages([]provider.Message{
		provider.UserMsg("read file.go"),
		provider.AssistantText("package main"),
	}, "session1")

	optimizer := NewArchiveOptimizer(store, nil)
	result := optimizer.Optimize(context.Background())
	_ = result
}

func TestRunLoop_NoProvider(t *testing.T) {
	a := NewAgentLoop(nil)
	_, err := a.Run("test")
	if err == nil {
		t.Error("expected error for nil provider")
	}
}

func TestRunWithMessages_NoProvider(t *testing.T) {
	a := NewAgentLoop(nil)
	_, err := a.RunWithMessages([]provider.Message{provider.UserMsg("test")})
	if err == nil {
		t.Error("expected error for nil provider")
	}
}

func TestStream_NoProvider(t *testing.T) {
	a := NewAgentLoop(nil)
	_, err := a.Stream("test")
	if err == nil {
		t.Error("expected error for nil provider")
	}
}
