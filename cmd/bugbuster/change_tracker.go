package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bugbuster-code/pkg/i18n"
)

// FileChange — record of file change
type FileChange struct {
	Path       string    `json:"path"`        // file path
	OldContent string   `json:"old_content"` // previous content (empty for new files)
	NewContent string   `json:"new_content"` // new content (empty for deleted)
	Timestamp  time.Time `json:"timestamp"`
	Operation  string    `json:"operation"` // "create", "modify", "delete"
}

// ChangeTracker — tracks file changes for undo
type ChangeTracker struct {
	mu      sync.Mutex
	changes []FileChange
	maxSize int // max storage size per file
}

// NewChangeTracker creates a new change tracker
func NewChangeTracker() *ChangeTracker {
	return &ChangeTracker{
		changes: make([]FileChange, 0),
		maxSize: 1024 * 1024, // 1MB per file
	}
}

// RecordWrite records change on file write
func (ct *ChangeTracker) RecordWrite(path string, newContent string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	// Read old content
	oldContent := ""
	operation := "create"

	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		data, err := os.ReadFile(path)
		if err == nil {
			oldContent = string(data)
			operation = "modify"
		}
	}

	// Do not save if file is too large
	if len(oldContent) > ct.maxSize || len(newContent) > ct.maxSize {
		return
	}

	ct.changes = append(ct.changes, FileChange{
		Path:       path,
		OldContent: oldContent,
		NewContent: newContent,
		Timestamp:  time.Now(),
		Operation:  operation,
	})
}

// RecordEdit records change on file edit
func (ct *ChangeTracker) RecordEdit(path string, oldContent, newContent string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if len(oldContent) > ct.maxSize || len(newContent) > ct.maxSize {
		return
	}

	ct.changes = append(ct.changes, FileChange{
		Path:       path,
		OldContent: oldContent,
		NewContent: newContent,
		Timestamp:  time.Now(),
		Operation:  "modify",
	})
}

// Undo reverts last change
func (ct *ChangeTracker) Undo() (string, error) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if len(ct.changes) == 0 {
		return "", i18n.E("cli.undo_nothing")
	}

	// Take last change
	last := ct.changes[len(ct.changes)-1]
	ct.changes = ct.changes[:len(ct.changes)-1]

	// Restore old content
	if last.Operation == "create" {
		// File was created — delete
		if err := os.Remove(last.Path); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("undo error: %w", err)
		}
		return fmt.Sprintf("deleted: %s", last.Path), nil
	}

	// File was modified — restore
	dir := filepath.Dir(last.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("undo error: %w", err)
	}

	if err := os.WriteFile(last.Path, []byte(last.OldContent), 0644); err != nil {
		return "", fmt.Errorf("undo error: %w", err)
	}

	return fmt.Sprintf("restored: %s", last.Path), nil
}

// UndoAll reverts all changes
func (ct *ChangeTracker) UndoAll() (int, error) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	count := len(ct.changes)
	if count == 0 {
		return 0, nil
	}

	errCount := 0
	// Revert in reverse order
	for i := len(ct.changes) - 1; i >= 0; i-- {
		ch := ct.changes[i]
		if ch.Operation == "create" {
			if err := os.Remove(ch.Path); err != nil && !os.IsNotExist(err) {
				errCount++
			}
		} else {
			dir := filepath.Dir(ch.Path)
			os.MkdirAll(dir, 0755)
			if err := os.WriteFile(ch.Path, []byte(ch.OldContent), 0644); err != nil {
				errCount++
			}
		}
	}

	ct.changes = ct.changes[:0]

	if errCount > 0 {
		return count, fmt.Errorf("failed to undo %d changes", errCount)
	}
	return count, nil
}

// Diff returns list of recent changes
func (ct *ChangeTracker) Diff() []FileChange {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	result := make([]FileChange, len(ct.changes))
	copy(result, ct.changes)
	return result
}

// Clear clears change history
func (ct *ChangeTracker) Clear() {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.changes = ct.changes[:0]
}

// Count returns count of recorded changes
func (ct *ChangeTracker) Count() int {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return len(ct.changes)
}

// FormatDiff formats diff for output
func FormatDiff(changes []FileChange) string {
	if len(changes) == 0 {
		return i18n.T("cli.diff_empty")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s%s%s\n", ansiBold, i18n.T("cli.diff_header"), ansiReset))

	for i, ch := range changes {
		var opIcon string
		var opColor string
		switch ch.Operation {
		case "create":
			opIcon = "+"
			opColor = appTheme.Success.ANSICode()
		case "modify":
			opIcon = "~"
			opColor = appTheme.Warning.ANSICode()
		case "delete":
			opIcon = "-"
			opColor = appTheme.Error.ANSICode()
		default:
			opIcon = "?"
			opColor = appTheme.Dim.ANSICode()
		}

		sb.WriteString(fmt.Sprintf("  %s%s%s %s%s%s (%s)\n",
			opColor, opIcon, ansiReset,
			appTheme.Primary.ANSICode(), ch.Path, ansiReset,
			ch.Timestamp.Format("15:04:05"),
		))

		// Show diff for modify
		if ch.Operation == "modify" && i < 20 {
			oldLines := strings.Split(ch.OldContent, "\n")
			newLines := strings.Split(ch.NewContent, "\n")
			maxLines := 3
			shownOld := oldLines
			shownNew := newLines
			if len(shownOld) > maxLines {
				shownOld = append(shownOld[:maxLines], "...")
			}
			if len(shownNew) > maxLines {
				shownNew = append(shownNew[:maxLines], "...")
			}
			if len(shownOld) > 0 && ch.OldContent != ch.NewContent {
				for _, line := range shownOld[:ctMin(len(shownOld), maxLines)] {
					if line != "" {
						sb.WriteString(fmt.Sprintf("    %s-%s%s\n", appTheme.Error.ANSICode(), line, ansiReset))
					}
				}
				for _, line := range shownNew[:ctMin(len(shownNew), maxLines)] {
					if line != "" {
						sb.WriteString(fmt.Sprintf("    %s+%s%s\n", appTheme.Success.ANSICode(), line, ansiReset))
					}
				}
			}
		}
	}

	return sb.String()
}

func ctMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SaveToFile saves change history to JSON file
func (ct *ChangeTracker) SaveToFile(path string) error {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(ct.changes, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// LoadFromFile loads change history from JSON file
func (ct *ChangeTracker) LoadFromFile(path string) error {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &ct.changes)
}
