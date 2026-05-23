package tools

import (
	"testing"
	"time"

	"bugbuster-code/pkg/i18n"
)

func TestTodoWriteTool_Description(t *testing.T) {
	i18n.Init("en")
	tool := NewTodoWriteTool()
	desc := tool.Description()
	if desc == "" {
		t.Error("Description should not be empty")
	}
}

func TestTodoWriteTool_Parameters(t *testing.T) {
	i18n.Init("en")
	tool := NewTodoWriteTool()
	params := tool.Parameters()
	if params == nil {
		t.Error("Parameters should not be nil")
	}
}

func TestTodoReadTool_Description(t *testing.T) {
	i18n.Init("en")
	writeTool := NewTodoWriteTool()
	tool := NewTodoReadTool(writeTool)
	desc := tool.Description()
	if desc == "" {
		t.Error("Description should not be empty")
	}
}

func TestTodoReadTool_Parameters(t *testing.T) {
	i18n.Init("en")
	writeTool := NewTodoWriteTool()
	tool := NewTodoReadTool(writeTool)
	params := tool.Parameters()
	if params == nil {
		t.Error("Parameters should not be nil")
	}
}

func TestWebFetchTool_Description(t *testing.T) {
	i18n.Init("en")
	tool := NewWebFetchTool()
	desc := tool.Description()
	if desc == "" {
		t.Error("Description should not be empty")
	}
}

func TestWebFetchTool_Execute_NoURL(t *testing.T) {
	tool := NewWebFetchTool()
	result := tool.Execute(map[string]string{})
	if result.Error == "" {
		t.Error("expected error for missing URL")
	}
}

func TestWebFetchTool_Execute_NetworkDisabled(t *testing.T) {
	tool := NewWebFetchTool()
	tool.AllowNetwork = false
	result := tool.Execute(map[string]string{"url": "https://example.com"})
	if result.Error == "" {
		t.Error("expected error when network is disabled")
	}
}

func TestWebFetchTool_Execute_InvalidURL(t *testing.T) {
	tool := NewWebFetchTool()
	result := tool.Execute(map[string]string{"url": "not-a-url"})
	if result.Error == "" {
		t.Error("expected error for invalid URL")
	}
}

func TestHookedTool_Description(t *testing.T) {
	i18n.Init("en")
	inner := NewReadTool()
	hooked := NewHookedTool(inner)
	desc := hooked.Description()
	if desc == "" {
		t.Error("Description should not be empty")
	}
}

func TestHookedTool_Parameters(t *testing.T) {
	i18n.Init("en")
	inner := NewReadTool()
	hooked := NewHookedTool(inner)
	params := hooked.Parameters()
	if params == nil {
		t.Error("Parameters should not be nil")
	}
}

func TestHookedTool_Execute_WithHooks(t *testing.T) {
	i18n.Init("en")
	inner := NewReadTool()
	beforeCalled := false
	afterCalled := false
	hook := &ToolHook{
		Name: "test-hook",
		BeforeExecute: func(toolName string, params map[string]string) (map[string]string, error) {
			beforeCalled = true
			return params, nil
		},
		AfterExecute: func(toolName string, params map[string]string, result ToolResult, duration time.Duration) {
			afterCalled = true
		},
	}
	hooked := NewHookedTool(inner, hook)
	result := hooked.Execute(map[string]string{})
	if !beforeCalled {
		t.Error("BeforeExecute hook should be called")
	}
	if !afterCalled {
		t.Error("AfterExecute hook should be called")
	}
	if result.Error == "" {
		t.Error("expected error for missing file path")
	}
}
