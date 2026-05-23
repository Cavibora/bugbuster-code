package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"bugbuster-code/pkg/tools"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// mockTool — тестовый инструмент
type mockTool struct {
	name        string
	description string
	params      map[string]any
	result      tools.ToolResult
}

func (t *mockTool) Name() string               { return t.name }
func (t *mockTool) Description() string        { return t.description }
func (t *mockTool) Parameters() map[string]any { return t.params }
func (t *mockTool) Execute(params map[string]string) tools.ToolResult {
	return t.result
}

func TestToolAdapter_NameWithPrefix(t *testing.T) {
	tool := &mockTool{name: "read", description: "Read a file", params: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		"required": []string{"path"},
	}, result: tools.ToolResult{Output: "file content"}}

	serverTool := ToolAdapter(tool, "bugbuster_")

	if serverTool.Tool.Name != "bugbuster_read" {
		t.Errorf("expected name 'bugbuster_read', got '%s'", serverTool.Tool.Name)
	}
	if serverTool.Tool.Description != "Read a file" {
		t.Errorf("expected description 'Read a file', got '%s'", serverTool.Tool.Description)
	}
}

func TestToolAdapter_SchemaConversion(t *testing.T) {
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		"required": []string{"path"},
	}
	tool := &mockTool{name: "read", description: "Read", params: params, result: tools.ToolResult{Output: "ok"}}

	serverTool := ToolAdapter(tool, "bugbuster_")

	// Проверяем что RawInputSchema установлен
	if serverTool.Tool.RawInputSchema == nil {
		t.Fatal("expected RawInputSchema to be set")
	}

	// Проверяем что это валидный JSON
	var schema map[string]any
	if err := json.Unmarshal(serverTool.Tool.RawInputSchema, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("expected schema type 'object', got '%v'", schema["type"])
	}
}

func TestToolAdapter_HandlerSuccess(t *testing.T) {
	tool := &mockTool{
		name:        "read",
		description: "Read",
		params:      map[string]any{"type": "object"},
		result:      tools.ToolResult{Output: "file content here"},
	}

	serverTool := ToolAdapter(tool, "bugbuster_")

	req := mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "bugbuster_read",
			Arguments: map[string]any{
				"path": "/tmp/test.txt",
			},
		},
	}

	result, err := serverTool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected no error in result")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	textContent, ok := result.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if textContent.Text != "file content here" {
		t.Errorf("expected 'file content here', got '%s'", textContent.Text)
	}
}

func TestToolAdapter_HandlerError(t *testing.T) {
	tool := &mockTool{
		name:        "read",
		description: "Read",
		params:      map[string]any{"type": "object"},
		result:      tools.ToolResult{Error: "file not found"},
	}

	serverTool := ToolAdapter(tool, "bugbuster_")

	req := mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name:      "bugbuster_read",
			Arguments: map[string]any{"path": "/nonexistent"},
		},
	}

	result, err := serverTool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error in result")
	}
}

func TestAdaptTools(t *testing.T) {
	toolsList := []tools.Tool{
		&mockTool{name: "read", description: "Read", params: map[string]any{"type": "object"}, result: tools.ToolResult{Output: "ok"}},
		&mockTool{name: "bash", description: "Bash", params: map[string]any{"type": "object"}, result: tools.ToolResult{Output: "ok"}},
	}

	serverTools := AdaptTools(toolsList, "bb_")
	if len(serverTools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(serverTools))
	}
	if serverTools[0].Tool.Name != "bb_read" {
		t.Errorf("expected 'bb_read', got '%s'", serverTools[0].Tool.Name)
	}
	if serverTools[1].Tool.Name != "bb_bash" {
		t.Errorf("expected 'bb_bash', got '%s'", serverTools[1].Tool.Name)
	}
}

func TestToolAdapter_EmptyPrefix(t *testing.T) {
	tool := &mockTool{name: "read", description: "Read", params: map[string]any{"type": "object"}, result: tools.ToolResult{Output: "ok"}}
	serverTool := ToolAdapter(tool, "")
	if serverTool.Tool.Name != "read" {
		t.Errorf("expected 'read', got '%s'", serverTool.Tool.Name)
	}
}

func TestToolAdapter_NilSchema(t *testing.T) {
	tool := &mockTool{name: "test", description: "Test", params: nil, result: tools.ToolResult{Output: "ok"}}
	serverTool := ToolAdapter(tool, "bugbuster_")
	// nil schema должен маршалиться в null, но NewToolWithRawSchema должен работать
	if serverTool.Tool.Name != "bugbuster_test" {
		t.Errorf("expected 'bugbuster_test', got '%s'", serverTool.Tool.Name)
	}
}
