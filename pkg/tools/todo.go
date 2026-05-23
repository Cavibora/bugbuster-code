package tools

import (
	"encoding/json"
	"sync"

	"bugbuster-code/pkg/i18n"
)

// TodoItem — checklist item
type TodoItem struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Status  string `json:"status"` // "pending" | "in_progress" | "completed"
}

// TodoWriteTool — checklist write tool
type TodoWriteTool struct {
	todos []TodoItem
	mu    sync.Mutex
}

// NewTodoWriteTool creates a new checklist write tool
func NewTodoWriteTool() *TodoWriteTool {
	return &TodoWriteTool{}
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
	result := make([]TodoItem, len(t.todos))
	copy(result, t.todos)
	return result
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
