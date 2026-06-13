package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"bugbuster-code/pkg/i18n"
)

// TodoItem — checklist item
type TodoItem struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Status  string `json:"status"` // "pending" | "in_progress" | "completed"
}

// TodoWriteTool — checklist write tool with file persistence.
// When filePath is set, todos are saved to/loaded from a JSON file,
// so they survive crashes and session restarts.
type TodoWriteTool struct {
	todos    []TodoItem
	mu       sync.Mutex
	filePath string // empty = in-memory only (backward compatible)
	loaded   bool
}

// NewTodoWriteTool creates a new checklist write tool (in-memory, no file persistence)
func NewTodoWriteTool() *TodoWriteTool {
	return &TodoWriteTool{}
}

// NewTodoWriteToolWithPath creates a new checklist write tool with file persistence
func NewTodoWriteToolWithPath(filePath string) *TodoWriteTool {
	return &TodoWriteTool{filePath: filePath}
}

func (t *TodoWriteTool) Name() string { return "todo_write" }

func (t *TodoWriteTool) Description() string {
	return i18n.T("tools.todo_write.description")
}

func (t *TodoWriteTool) Execute(params map[string]string) ToolResult {
	todosJSON, ok := params["todos"]
	if !ok || todosJSON == "" {
		return Error("tools.todo_write.param_todos")
	}

	var items []TodoItem
	if err := json.Unmarshal([]byte(todosJSON), &items); err != nil {
		return Error("tools.todo_write.invalid_json", err)
	}

	// Validate statuses
	for i, item := range items {
		switch item.Status {
		case "pending", "in_progress", "completed":
			// OK
		default:
			items[i].Status = "pending"
		}
	}

	t.mu.Lock()
	t.todos = items
	t.saveToFileLocked()
	t.mu.Unlock()

	// Return JSON for UI rendering
	out, err := json.Marshal(items)
	if err != nil {
		return ToolResult{Error: err.Error()}
	}

	return ToolResult{Output: string(out)}
}

func (t *TodoWriteTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"todos": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{
							"type":        "string",
							"description": "unique task id",
						},
						"subject": map[string]any{
							"type":        "string",
							"description": "task description",
						},
						"status": map[string]any{
							"type":        "string",
							"enum":        []any{"pending", "in_progress", "completed"},
							"description": "task status",
						},
					},
					"required": []any{"id", "subject", "status"},
				},
				"description": "list of tasks with id, subject, and status",
			},
		},
		"required": []any{"todos"},
	}
}

// GetTodos returns current checklist (thread-safe)
func (t *TodoWriteTool) GetTodos() []TodoItem {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.loadFromFile()
	result := make([]TodoItem, len(t.todos))
	copy(result, t.todos)
	return result
}

// --- File persistence ---

// loadFromFile loads todos from file (caller must hold t.mu)
func (t *TodoWriteTool) loadFromFile() {
	if t.filePath == "" || t.loaded {
		return
	}

	data, err := os.ReadFile(t.filePath)
	if err != nil {
		// File doesn't exist yet — that's OK
		t.loaded = true
		return
	}

	var items []TodoItem
	if err := json.Unmarshal(data, &items); err != nil {
		// Corrupted file — ignore
		t.loaded = true
		return
	}

	t.todos = items
	t.loaded = true
}

// saveToFileLocked saves todos to file (caller must hold t.mu)
func (t *TodoWriteTool) saveToFileLocked() {
	if t.filePath == "" {
		return
	}

	dir := filepath.Dir(t.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	data, err := json.Marshal(t.todos)
	if err != nil {
		return
	}

	os.WriteFile(t.filePath, data, 0644)
}

// TodoFilePathForProject returns the path to the todo file for a session,
// using the given project directory as the first choice.
func TodoFilePathForProject(sessionID, projectDir string) string {
	bbDir := filepath.Join(projectDir, ".bugbuster")
	if info, err := os.Stat(bbDir); err == nil && info.IsDir() {
		return filepath.Join(projectDir, ".bugbuster", "todo", sessionID+".json")
	}
	if cwd, err := os.Getwd(); err == nil {
		bbDir := filepath.Join(cwd, ".bugbuster")
		if info, err := os.Stat(bbDir); err == nil && info.IsDir() {
			return filepath.Join(cwd, ".bugbuster", "todo", sessionID+".json")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".bugbuster", "todo", sessionID+".json")
}

// TodoReadTool — checklist read tool
type TodoReadTool struct {
	writeTool *TodoWriteTool
}

// NewTodoReadTool creates a new checklist read tool
func NewTodoReadTool(writeTool *TodoWriteTool) *TodoReadTool {
	return &TodoReadTool{writeTool: writeTool}
}

func (t *TodoReadTool) Name() string { return "todo_read" }

func (t *TodoReadTool) Description() string {
	return i18n.T("tools.todo_read.description")
}

func (t *TodoReadTool) Execute(params map[string]string) ToolResult {
	todos := t.writeTool.GetTodos()
	if len(todos) == 0 {
		return Success("tools.todo_read.empty")
	}

	out, err := json.Marshal(todos)
	if err != nil {
		return ToolResult{Error: err.Error()}
	}

	return ToolResult{Output: string(out)}
}

func (t *TodoReadTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}