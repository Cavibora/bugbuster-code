package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// --- File persistence tests ---

func TestTodoWriteToolWithFilePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "todo.json")
	tw := NewTodoWriteToolWithPath(filePath)

	// Write todos
	result := tw.Execute(map[string]string{
		"todos": `[{"id":"1","subject":"Setup project","status":"completed"},{"id":"2","subject":"Add tests","status":"in_progress"}]`,
	})
	if result.Error != "" {
		t.Fatalf("Execute failed: %s", result.Error)
	}

	// Verify file was created
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Todo file not created: %v", err)
	}

	var items []TodoItem
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("File content is not valid JSON: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("Expected 2 items in file, got %d", len(items))
	}
	if items[0].Subject != "Setup project" {
		t.Errorf("Item 0 subject = %q, want 'Setup project'", items[0].Subject)
	}
}

func TestTodoWriteToolLoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "todo.json")

	// Pre-create file with todos
	items := []TodoItem{
		{ID: "1", Subject: "Existing task", Status: "pending"},
		{ID: "2", Subject: "Done task", Status: "completed"},
	}
	data, _ := json.Marshal(items)
	os.WriteFile(filePath, data, 0644)

	// Create tool and read
	tw := NewTodoWriteToolWithPath(filePath)
	todos := tw.GetTodos()
	if len(todos) != 2 {
		t.Fatalf("Expected 2 items loaded from file, got %d", len(todos))
	}
	if todos[0].Subject != "Existing task" {
		t.Errorf("Item 0 subject = %q, want 'Existing task'", todos[0].Subject)
	}
}

func TestTodoWriteToolFilePersistenceRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "todo.json")

	// Write with first tool instance
	tw1 := NewTodoWriteToolWithPath(filePath)
	tw1.Execute(map[string]string{
		"todos": `[{"id":"1","subject":"Task A","status":"pending"},{"id":"2","subject":"Task B","status":"completed"}]`,
	})

	// Load with second tool instance (simulates crash/restart)
	tw2 := NewTodoWriteToolWithPath(filePath)
	todos := tw2.GetTodos()
	if len(todos) != 2 {
		t.Fatalf("Expected 2 items after reload, got %d", len(todos))
	}
	if todos[0].Subject != "Task A" {
		t.Errorf("Item 0 subject = %q, want 'Task A'", todos[0].Subject)
	}
	if todos[1].Status != "completed" {
		t.Errorf("Item 1 status = %q, want 'completed'", todos[1].Status)
	}
}

func TestTodoWriteToolNoFilePath(t *testing.T) {
	// Without filePath — in-memory only, backward compatible
	tw := NewTodoWriteTool()
	tw.Execute(map[string]string{
		"todos": `[{"id":"1","subject":"In-memory task","status":"pending"}]`,
	})

	todos := tw.GetTodos()
	if len(todos) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(todos))
	}
}

func TestTodoWriteToolCorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "todo.json")

	// Write corrupted JSON
	os.WriteFile(filePath, []byte("not valid json{{{"), 0644)

	tw := NewTodoWriteToolWithPath(filePath)
	todos := tw.GetTodos()
	// Should return empty list, not panic
	if len(todos) != 0 {
		t.Errorf("Expected empty list for corrupted file, got %d items", len(todos))
	}
}

func TestTodoWriteToolMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "nonexistent", "todo.json")

	tw := NewTodoWriteToolWithPath(filePath)
	todos := tw.GetTodos()
	// Should return empty list, not panic
	if len(todos) != 0 {
		t.Errorf("Expected empty list for missing file, got %d items", len(todos))
	}
}

func TestTodoWriteToolCreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "subdir", "nested", "todo.json")

	tw := NewTodoWriteToolWithPath(filePath)
	tw.Execute(map[string]string{
		"todos": `[{"id":"1","subject":"Nested task","status":"pending"}]`,
	})

	// Verify file was created in nested dir
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Todo file not created in nested dir: %v", err)
	}

	var items []TodoItem
	json.Unmarshal(data, &items)
	if len(items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(items))
	}
}

func TestTodoReadToolWithFilePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "todo.json")

	// Pre-create file
	items := []TodoItem{
		{ID: "1", Subject: "File task", Status: "in_progress"},
	}
	data, _ := json.Marshal(items)
	os.WriteFile(filePath, data, 0644)

	tw := NewTodoWriteToolWithPath(filePath)
	tr := NewTodoReadTool(tw)

	result := tr.Execute(map[string]string{})
	if result.Error != "" {
		t.Fatalf("Execute failed: %s", result.Error)
	}

	var readItems []TodoItem
	json.Unmarshal([]byte(result.Output), &readItems)
	if len(readItems) != 1 {
		t.Errorf("Expected 1 item, got %d", len(readItems))
	}
	if readItems[0].Subject != "File task" {
		t.Errorf("Item subject = %q, want 'File task'", readItems[0].Subject)
	}
}

func TestTodoWriteToolOverwriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "todo.json")

	tw := NewTodoWriteToolWithPath(filePath)

	// First write
	tw.Execute(map[string]string{
		"todos": `[{"id":"1","subject":"First","status":"pending"}]`,
	})

	// Overwrite with new todos
	tw.Execute(map[string]string{
		"todos": `[{"id":"2","subject":"Second","status":"completed"},{"id":"3","subject":"Third","status":"pending"}]`,
	})

	// Verify file has latest todos
	data, _ := os.ReadFile(filePath)
	var items []TodoItem
	json.Unmarshal(data, &items)
	if len(items) != 2 {
		t.Fatalf("Expected 2 items after overwrite, got %d", len(items))
	}
	if items[0].Subject != "Second" {
		t.Errorf("Item 0 subject = %q, want 'Second'", items[0].Subject)
	}
}
