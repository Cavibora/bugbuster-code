package tools

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestHookedToolBeforeExecute tests the BeforeExecute hook
func TestHookedToolBeforeExecute(t *testing.T) {
	var capturedName string
	var capturedParams map[string]string

	hook := &ToolHook{
		Name: "test_hook",
		BeforeExecute: func(toolName string, params map[string]string) (map[string]string, error) {
			capturedName = toolName
			capturedParams = params
			// Modify parameters
			params["modified"] = "true"
			return params, nil
		},
	}

	readTool := NewReadTool()
	hooked := NewHookedTool(readTool, hook)

	if hooked.Name() != "read" {
		t.Errorf("Expected name 'read', got '%s'", hooked.Name())
	}

	// Call execute (we will read a non-existent file — this is fine for a hook test)
	result := hooked.Execute(map[string]string{"path": "/nonexistent/test.txt"})

	// The hook should have captured the name and parameters
	if capturedName != "read" {
		t.Errorf("Hook should capture tool name 'read', got '%s'", capturedName)
	}
	if capturedParams["modified"] != "true" {
		t.Error("Hook should have modified params")
	}

	// The result will be an error (file does not exist), but the hook should have run
	_ = result
}

// TestHookedToolAfterExecute tests the AfterExecute hook
func TestHookedToolAfterExecute(t *testing.T) {
	var capturedName string
	var capturedDuration time.Duration

	hook := &ToolHook{
		Name: "after_hook",
		AfterExecute: func(toolName string, params map[string]string, result ToolResult, duration time.Duration) {
			capturedName = toolName
			capturedDuration = duration
		},
	}

	readTool := NewReadTool()
	hooked := NewHookedTool(readTool, hook)

	hooked.Execute(map[string]string{"path": "/nonexistent"})

	if capturedName != "read" {
		t.Errorf("AfterExecute hook should capture 'read', got '%s'", capturedName)
	}
	if capturedDuration < 0 {
		t.Error("Duration should be non-negative")
	}
}

// TestHookedToolBeforeExecuteError tests interruption via hook
func TestHookedToolBeforeExecuteError(t *testing.T) {
	hook := &ToolHook{
		Name: "blocking_hook",
		BeforeExecute: func(toolName string, params map[string]string) (map[string]string, error) {
			return nil, fmt.Errorf("blocked by hook")
		},
	}

	readTool := NewReadTool()
	hooked := NewHookedTool(readTool, hook)

	result := hooked.Execute(map[string]string{"path": "/some/file"})

	if result.Error == "" {
		t.Error("Expected error from blocking hook")
	}
}

// TestMultipleHooks tests the hook chain
func TestMultipleHooks(t *testing.T) {
	var order []string

	hook1 := &ToolHook{
		Name: "hook1",
		BeforeExecute: func(toolName string, params map[string]string) (map[string]string, error) {
			order = append(order, "before1")
			return params, nil
		},
		AfterExecute: func(toolName string, params map[string]string, result ToolResult, duration time.Duration) {
			order = append(order, "after1")
		},
	}

	hook2 := &ToolHook{
		Name: "hook2",
		BeforeExecute: func(toolName string, params map[string]string) (map[string]string, error) {
			order = append(order, "before2")
			return params, nil
		},
		AfterExecute: func(toolName string, params map[string]string, result ToolResult, duration time.Duration) {
			order = append(order, "after2")
		},
	}

	readTool := NewReadTool()
	hooked := NewHookedTool(readTool, hook1, hook2)

	hooked.Execute(map[string]string{"path": "/nonexistent"})

	if len(order) != 4 {
		t.Errorf("Expected 4 hook calls, got %d: %v", len(order), order)
	}
	if order[0] != "before1" || order[1] != "before2" {
		t.Errorf("BeforeExecute hooks should be called in order, got: %v", order)
	}
	if order[2] != "after1" || order[3] != "after2" {
		t.Errorf("AfterExecute hooks should be called in order, got: %v", order)
	}
}

// TestHookedToolExecuteAsync_DelegatesToAsyncTool verifies that HookedTool delegates ExecuteAsync to the inner AsyncTool
func TestHookedToolExecuteAsync_DelegatesToAsyncTool(t *testing.T) {
	bashTool := NewBashTool()
	hooked := NewHookedTool(bashTool)

	ch := hooked.ExecuteAsync(map[string]string{"command": "echo hello"})
	if ch == nil {
		t.Fatal("ExecuteAsync returned nil channel")
	}

	var result AsyncEvent
	for evt := range ch {
		if evt.Done {
			result = evt
			break
		}
	}

	if !result.Done {
		t.Fatal("expected Done=true")
	}
	if !strings.Contains(result.Output, "hello") {
		t.Fatalf("expected 'hello' in output, got: %s", result.Output)
	}
}

// TestHookedToolExecuteAsync_FallbackForNonAsync verifies the fallback for non-async tools
func TestHookedToolExecuteAsync_FallbackForNonAsync(t *testing.T) {
	readTool := NewReadTool()
	hooked := NewHookedTool(readTool)

	ch := hooked.ExecuteAsync(map[string]string{"path": "/nonexistent"})
	if ch == nil {
		t.Fatal("ExecuteAsync returned nil channel")
	}

	var result AsyncEvent
	for evt := range ch {
		if evt.Done {
			result = evt
			break
		}
	}

	if !result.Done {
		t.Fatal("expected Done=true")
	}
	// ReadTool will return an error for a non-existent file
	if result.Error == "" {
		t.Fatal("expected error for nonexistent file")
	}
}
