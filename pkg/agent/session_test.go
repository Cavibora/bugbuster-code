package agent

import (
	"os"
	"strings"
	"testing"

	"bugbuster-code/pkg/provider"
)

func TestNewSessionManager(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	if sm.SessionsDir != tmpDir {
		t.Errorf("Expected SessionsDir=%s, got %s", tmpDir, sm.SessionsDir)
	}
}

func TestSessionManager_Init(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := tmpDir + "/sessions"
	sm := NewSessionManager(sessionsDir)

	if err := sm.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	// Verify that the directory was created
	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		t.Error("Sessions directory should exist after Init")
	}
}

func TestNewSession(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	session := sm.NewSession()

	if session.ID == "" {
		t.Error("Session ID should not be empty")
	}
	if !strings.HasPrefix(session.ID, "sess_") {
		t.Errorf("Session ID should start with 'sess_', got %s", session.ID)
	}
	if session.CreatedAt.IsZero() {
		t.Error("Session CreatedAt should not be zero")
	}
	if len(session.Messages) != 0 {
		t.Error("New session should have no messages")
	}
}

func TestSaveAndLoadSession(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	session := sm.NewSession()
	session.Messages = append(session.Messages, provider.SystemMsg("Ты помощник"))
	session.Messages = append(session.Messages, provider.UserMsg("Привет"))
	session.Messages = append(session.Messages, provider.AssistantText("Здравствуй!"))

	// Save
	if err := sm.SaveSessionMessages(session); err != nil {
		t.Fatalf("SaveSessionMessages error: %v", err)
	}

	// Load
	loaded, err := sm.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("LoadSession error: %v", err)
	}

	if loaded.ID != session.ID {
		t.Errorf("Expected ID=%s, got %s", session.ID, loaded.ID)
	}
	if len(loaded.Messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Role != "system" {
		t.Errorf("Expected first message role=system, got %s", loaded.Messages[0].Role)
	}
}

func TestLoadSession_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	_, err := sm.LoadSession("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent session")
	}
}

func TestListSessions(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	// Create 3 sessions
	for i := 0; i < 3; i++ {
		session := sm.NewSession()
		session.Messages = append(session.Messages, provider.UserMsg("test"))
		if err := sm.SaveSessionMessages(session); err != nil {
			t.Fatalf("SaveSessionMessages error: %v", err)
		}
	}

	sessions, err := sm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions error: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("Expected 3 sessions, got %d", len(sessions))
	}
}

func TestDeleteSession(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	session := sm.NewSession()
	session.Messages = append(session.Messages, provider.UserMsg("test"))
	if err := sm.SaveSessionMessages(session); err != nil {
		t.Fatalf("SaveSessionMessages error: %v", err)
	}

	// Remove
	if err := sm.DeleteSession(session.ID); err != nil {
		t.Fatalf("DeleteSession error: %v", err)
	}

	// Verify that the session was deleted
	_, err := sm.LoadSession(session.ID)
	if err == nil {
		t.Error("Expected error after deletion")
	}
}

func TestSessionManager_SaveAndLoadEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	session := sm.NewSession()
	// Empty session without messages

	if err := sm.SaveSessionMessages(session); err != nil {
		t.Fatalf("SaveSessionMessages error: %v", err)
	}

	loaded, err := sm.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("LoadSession error: %v", err)
	}

	if len(loaded.Messages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(loaded.Messages))
	}
}

func TestGenerateSessionID(t *testing.T) {
	id1 := generateSessionID()
	id2 := generateSessionID()

	if id1 == id2 {
		t.Error("Session IDs should be unique")
	}
	if !strings.HasPrefix(id1, "sess_") {
		t.Errorf("Session ID should start with 'sess_', got %s", id1)
	}
}

func TestRenameSession(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	// Create a session
	session := sm.NewSession()
	session.Messages = append(session.Messages, provider.UserMsg("test"))
	if err := sm.SaveSessionMessages(session); err != nil {
		t.Fatalf("SaveSessionMessages error: %v", err)
	}

	// Rename
	if err := sm.RenameSession(session.ID, "my-session"); err != nil {
		t.Fatalf("RenameSession error: %v", err)
	}

	// Load and verify the name
	loaded, err := sm.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("LoadSession error: %v", err)
	}
	if loaded.Name != "my-session" {
		t.Errorf("Expected name='my-session', got %q", loaded.Name)
	}
}

func TestRenameSession_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	err := sm.RenameSession("nonexistent", "name")
	if err == nil {
		t.Error("Expected error for nonexistent session")
	}
}

func TestSaveAndLoadSession_InputHistory(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	session := sm.NewSession()
	session.Messages = append(session.Messages, provider.SystemMsg("Ты помощник"))
	session.Messages = append(session.Messages, provider.UserMsg("Привет"))
	session.InputHistory = []string{"Привет", "Как дела?", "Расскажи о себе", "Помоги с кодом"}

	// Save
	if err := sm.SaveSessionMessages(session); err != nil {
		t.Fatalf("SaveSessionMessages error: %v", err)
	}

	// Load
	loaded, err := sm.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("LoadSession error: %v", err)
	}

	if len(loaded.InputHistory) != 4 {
		t.Errorf("Expected 4 input history items, got %d", len(loaded.InputHistory))
	}
	if loaded.InputHistory[0] != "Привет" {
		t.Errorf("Expected first history item='Привет', got %q", loaded.InputHistory[0])
	}
	if loaded.InputHistory[3] != "Помоги с кодом" {
		t.Errorf("Expected last history item='Помоги с кодом', got %q", loaded.InputHistory[3])
	}
}

func TestSaveAndLoadSession_EmptyInputHistory(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	session := sm.NewSession()
	session.Messages = append(session.Messages, provider.UserMsg("test"))
	// InputHistory is nil — should not be saved

	if err := sm.SaveSessionMessages(session); err != nil {
		t.Fatalf("SaveSessionMessages error: %v", err)
	}

	loaded, err := sm.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("LoadSession error: %v", err)
	}

	if len(loaded.InputHistory) != 0 {
		t.Errorf("Expected 0 input history items for empty history, got %d", len(loaded.InputHistory))
	}
}

func TestResolveSessionID_ByID(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	// Create a session with a name
	session := sm.NewSession()
	session.Name = "junior"
	if err := sm.SaveSessionMessages(session); err != nil {
		t.Fatalf("SaveSessionMessages error: %v", err)
	}

	// Resolve by full ID
	resolved, err := sm.ResolveSessionID(session.ID)
	if err != nil {
		t.Fatalf("ResolveSessionID by ID error: %v", err)
	}
	if resolved != session.ID {
		t.Errorf("Expected %s, got %s", session.ID, resolved)
	}
}

func TestResolveSessionID_ByName(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	// Create a session with a name
	session := sm.NewSession()
	session.Name = "junior"
	if err := sm.SaveSessionMessages(session); err != nil {
		t.Fatalf("SaveSessionMessages error: %v", err)
	}

	// Resolve by name
	resolved, err := sm.ResolveSessionID("junior")
	if err != nil {
		t.Fatalf("ResolveSessionID by name error: %v", err)
	}
	if resolved != session.ID {
		t.Errorf("Expected %s, got %s", session.ID, resolved)
	}
}

func TestResolveSessionID_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	_, err := sm.ResolveSessionID("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent session, got nil")
	}
}

func TestResolveSessionID_ByPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	// Create a session
	session := sm.NewSession()
	if err := sm.SaveSessionMessages(session); err != nil {
		t.Fatalf("SaveSessionMessages error: %v", err)
	}

	// Resolve by partial ID (first 10 chars)
	prefix := session.ID[:10]
	resolved, err := sm.ResolveSessionID(prefix)
	if err != nil {
		t.Fatalf("ResolveSessionID by prefix error: %v", err)
	}
	if resolved != session.ID {
		t.Errorf("Expected %s, got %s", session.ID, resolved)
	}
}
func TestLoadSession_LargeToolResult(t *testing.T) {
	// Test: tool_result longer than 64KB (default bufio.Scanner buffer)
	// should load fully, not be lost
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	session := sm.NewSession()
	session.Messages = append(session.Messages, provider.SystemMsg("Ты помощник"))
	session.Messages = append(session.Messages, provider.UserMsg("Запусти команду"))

	// Create a very long tool_result (> 64KB)
	longOutput := strings.Repeat("output line with some data\n", 3000) // ~75KB
	session.Messages = append(session.Messages, provider.Message{
		Role: "assistant",
		Content: []provider.ContentBlock{
			{Type: "tool_use", ToolName: "bash", Input: map[string]any{"command": "ls"}},
		},
	})
	session.Messages = append(session.Messages, provider.Message{
		Role: "user",
		Content: []provider.ContentBlock{
			{Type: "tool_result", ToolName: "bash", Output: longOutput},
		},
	})
	session.Messages = append(session.Messages, provider.AssistantText("Готово"))
	session.Messages = append(session.Messages, provider.UserMsg("Следующий вопрос"))

	// Save
	if err := sm.SaveSessionMessages(session); err != nil {
		t.Fatalf("SaveSessionMessages error: %v", err)
	}

	// Load
	loaded, err := sm.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("LoadSession error: %v", err)
	}

	// All messages should be loaded (including system)
	expectedCount := len(session.Messages)
	if len(loaded.Messages) != expectedCount {
		// Debug: show what was loaded
		for i, m := range loaded.Messages {
			t.Logf("  [%d] role=%s text=%q", i, m.Role, m.GetResponseText()[:min(50, len(m.GetResponseText()))])
		}
		t.Errorf("Expected %d messages, got %d — large tool_result was probably truncated by scanner buffer", expectedCount, len(loaded.Messages))
	}

	// The last message should be "Следующий вопрос"
	if len(loaded.Messages) >= 1 {
		lastMsg := loaded.Messages[len(loaded.Messages)-1]
		if lastMsg.Role != "user" {
			t.Errorf("Expected last message role=user, got %s", lastMsg.Role)
		}
		text := lastMsg.GetResponseText()
		if text != "Следующий вопрос" {
			t.Errorf("Expected last message text='Следующий вопрос', got %q", text)
		}
	}
}
