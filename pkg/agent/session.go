package agent

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

// Session — serializable session state
type Session struct {
	ID        string             `json:"id"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	Messages  []provider.Message `json:"messages"`
}

// SessionManager is the session manager (save/load)
type SessionManager struct {
	SessionsDir string // directory for storing sessions
}

// NewSessionManager creates session manager
func NewSessionManager(sessionsDir string) *SessionManager {
	return &SessionManager{
		SessionsDir: sessionsDir,
	}
}

// Init creates directory for sessions
func (sm *SessionManager) Init() error {
	return os.MkdirAll(sm.SessionsDir, 0755)
}

// NewSession creates a new session
func (sm *SessionManager) NewSession() *Session {
	now := time.Now()
	return &Session{
		ID:        generateSessionID(),
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []provider.Message{},
	}
}

// SaveSession saves session (atomic write)
func (sm *SessionManager) SaveSession(session *Session) error {
	if err := sm.Init(); err != nil {
		return fmt.Errorf(i18n.T("errors_session.create_dir"), err)
	}

	session.UpdatedAt = time.Now()

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf(i18n.T("errors_session.serialize"), err)
	}

	// Atomic write: write to temporary file, then rename
	tmpFile := filepath.Join(sm.SessionsDir, session.ID+".tmp")
	finalFile := filepath.Join(sm.SessionsDir, session.ID+".jsonl")

	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf(i18n.T("errors_session.write_temp"), err)
	}

	if err := os.Rename(tmpFile, finalFile); err != nil {
		os.Remove(tmpFile) // cleanup
		return fmt.Errorf(i18n.T("errors_session.rename"), err)
	}

	return nil
}

// SaveSessionMessages saves session messages in JSONL format (line by line)
func (sm *SessionManager) SaveSessionMessages(session *Session) error {
	if err := sm.Init(); err != nil {
		return fmt.Errorf(i18n.T("errors_session.create_dir"), err)
	}

	session.UpdatedAt = time.Now()

	finalFile := filepath.Join(sm.SessionsDir, session.ID+".jsonl")
	tmpFile := filepath.Join(sm.SessionsDir, session.ID+".tmp")

	f, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf(i18n.T("errors_session.create_temp"), err)
	}

	// Write session metadata as first line
	meta := map[string]string{
		"id":         session.ID,
		"created_at": session.CreatedAt.Format(time.RFC3339),
		"updated_at": session.UpdatedAt.Format(time.RFC3339),
	}
	metaData, _ := json.Marshal(meta)
	fmt.Fprintf(f, "%s\n", metaData)

	// Write each message as a separate line
	for _, msg := range session.Messages {
		msgData, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		fmt.Fprintf(f, "%s\n", msgData)
	}

	f.Close()

	if err := os.Rename(tmpFile, finalFile); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf(i18n.T("errors_session.rename"), err)
	}

	return nil
}

// LoadSession loads session by ID
func (sm *SessionManager) LoadSession(sessionID string) (*Session, error) {
	filePath := filepath.Join(sm.SessionsDir, sessionID+".jsonl")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("errors_session.not_found", sessionID), err)
	}

	// Try JSONL format (line by line)
	session, err := sm.parseJSONL(data)
	if err == nil {
		session.ID = sessionID
		return session, nil
	}

	// Fallback: try regular JSON
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf(i18n.T("errors_session.parse"), err)
	}
	s.ID = sessionID
	return &s, nil
}

// parseJSONL parses JSONL format sessions
func (sm *SessionManager) parseJSONL(data []byte) (*Session, error) {
	session := &Session{
		Messages: []provider.Message{},
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	// Increase scanner buffer to 1MB — tool_result can be very long
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lineNum++

		// First line — metadata
		if lineNum == 1 {
			var meta map[string]string
			if err := json.Unmarshal([]byte(line), &meta); err == nil {
				if id, ok := meta["id"]; ok {
					session.ID = id
				}
				if createdAt, ok := meta["created_at"]; ok {
					if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
						session.CreatedAt = t
					}
				}
				if updatedAt, ok := meta["updated_at"]; ok {
					if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
						session.UpdatedAt = t
					}
				}
				continue
			}
		}

		// Remaining lines — messages
		var msg provider.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // Skip invalid lines
		}
		session.Messages = append(session.Messages, msg)
	}

	if session.ID == "" {
		return nil, i18n.E("errors_session.empty")
	}

	return session, nil
}

// ListSessions returns list of accessible sessions
func (sm *SessionManager) ListSessions() ([]*Session, error) {
	if err := sm.Init(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(sm.SessionsDir)
	if err != nil {
		return nil, err
	}

	var sessions []*Session
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
		session, err := sm.LoadSession(sessionID)
		if err != nil {
			continue
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// DeleteSession deletes session
func (sm *SessionManager) DeleteSession(sessionID string) error {
	filePath := filepath.Join(sm.SessionsDir, sessionID+".jsonl")
	return os.Remove(filePath)
}

// generateSessionID generates unique ID sessions
func generateSessionID() string {
	ts := time.Now()
	b := make([]byte, 4)
	rand.Read(b)
	suffix := hex.EncodeToString(b)
	return fmt.Sprintf("sess_%s_%s", ts.Format("20060102_150405"), suffix)
}
