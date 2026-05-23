package tools

import (
	"encoding/json"
	"testing"
)

func TestTodoWriteToolExecute(t *testing.T) {
	tw := NewTodoWriteTool()

	// Пишем чеклист
	params := map[string]string{
		"todos": `[{"id":"1","subject":"Setup project","status":"completed"},{"id":"2","subject":"Add tests","status":"in_progress"},{"id":"3","subject":"Deploy","status":"pending"}]`,
	}
	result := tw.Execute(params)
	if result.Error != "" {
		t.Fatalf("Execute failed: %s", result.Error)
	}

	// Проверяем что Output — валидный JSON
	var items []TodoItem
	if err := json.Unmarshal([]byte(result.Output), &items); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(items))
	}
	if items[0].Status != "completed" {
		t.Errorf("Item 0 status = %q, want 'completed'", items[0].Status)
	}
	if items[1].Status != "in_progress" {
		t.Errorf("Item 1 status = %q, want 'in_progress'", items[1].Status)
	}
	if items[2].Status != "pending" {
		t.Errorf("Item 2 status = %q, want 'pending'", items[2].Status)
	}
}

func TestTodoWriteToolMissingParam(t *testing.T) {
	tw := NewTodoWriteTool()
	result := tw.Execute(map[string]string{})
	if result.Error == "" {
		t.Error("Expected error for missing 'todos' param")
	}
}

func TestTodoWriteToolInvalidJSON(t *testing.T) {
	tw := NewTodoWriteTool()
	result := tw.Execute(map[string]string{"todos": "not json"})
	if result.Error == "" {
		t.Error("Expected error for invalid JSON")
	}
}

func TestTodoWriteToolInvalidStatus(t *testing.T) {
	tw := NewTodoWriteTool()
	params := map[string]string{
		"todos": `[{"id":"1","subject":"Test","status":"unknown"}]`,
	}
	result := tw.Execute(params)
	if result.Error != "" {
		t.Fatalf("Execute failed: %s", result.Error)
	}

	var items []TodoItem
	json.Unmarshal([]byte(result.Output), &items)
	if items[0].Status != "pending" {
		t.Errorf("Invalid status should default to 'pending', got %q", items[0].Status)
	}
}

func TestTodoReadToolEmpty(t *testing.T) {
	tw := NewTodoWriteTool()
	tr := NewTodoReadTool(tw)

	result := tr.Execute(map[string]string{})
	if result.Error != "" {
		t.Fatalf("Execute failed: %s", result.Error)
	}
	// i18n.T() returns the key when i18n is not initialized
	if result.Output == "" {
		t.Error("Expected non-empty output for empty todo list")
	}
}

func TestTodoReadToolAfterWrite(t *testing.T) {
	tw := NewTodoWriteTool()
	tr := NewTodoReadTool(tw)

	// Пишем чеклист
	tw.Execute(map[string]string{
		"todos": `[{"id":"1","subject":"Task 1","status":"completed"}]`,
	})

	// Читаем
	result := tr.Execute(map[string]string{})
	if result.Error != "" {
		t.Fatalf("Execute failed: %s", result.Error)
	}

	var items []TodoItem
	if err := json.Unmarshal([]byte(result.Output), &items); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(items))
	}
	if items[0].Subject != "Task 1" {
		t.Errorf("Item subject = %q, want 'Task 1'", items[0].Subject)
	}
}

func TestTodoWriteToolUpdate(t *testing.T) {
	tw := NewTodoWriteTool()

	// Первый вызов
	tw.Execute(map[string]string{
		"todos": `[{"id":"1","subject":"Task 1","status":"pending"}]`,
	})

	// Обновляем статус
	result := tw.Execute(map[string]string{
		"todos": `[{"id":"1","subject":"Task 1","status":"completed"}]`,
	})
	if result.Error != "" {
		t.Fatalf("Execute failed: %s", result.Error)
	}

	var items []TodoItem
	json.Unmarshal([]byte(result.Output), &items)
	if items[0].Status != "completed" {
		t.Errorf("Updated status = %q, want 'completed'", items[0].Status)
	}
}

func TestTodoWriteToolName(t *testing.T) {
	tw := NewTodoWriteTool()
	if tw.Name() != "todo_write" {
		t.Errorf("Name = %q, want 'todo_write'", tw.Name())
	}
}

func TestTodoReadToolName(t *testing.T) {
	tr := NewTodoReadTool(NewTodoWriteTool())
	if tr.Name() != "todo_read" {
		t.Errorf("Name = %q, want 'todo_read'", tr.Name())
	}
}

func TestTodoWriteToolParameters(t *testing.T) {
	tw := NewTodoWriteTool()
	params := tw.Parameters()
	if params["type"] != "object" {
		t.Errorf("Parameters type = %v, want 'object'", params["type"])
	}
}

func TestGetTodos(t *testing.T) {
	tw := NewTodoWriteTool()
	tw.Execute(map[string]string{
		"todos": `[{"id":"1","subject":"A","status":"pending"},{"id":"2","subject":"B","status":"completed"}]`,
	})

	todos := tw.GetTodos()
	if len(todos) != 2 {
		t.Errorf("GetTodos returned %d items, want 2", len(todos))
	}
}
