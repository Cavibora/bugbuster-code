package tools

import (
	"time"

	"bugbuster-code/pkg/i18n"
)

// ToolResult — execution result tool
type ToolResult struct {
	Output   string         // text result
	Error    string         // error (empty line if not)
	Metadata map[string]any // optional metadata (e.g., image_base64 for screenshots)
}

// Success returns successful result with translation via i18n.T()
// Note: first argument is an i18n key, not a Go format string.
func Success(key string, args ...any) ToolResult {
	return ToolResult{Output: i18n.T(key, args...)}
}

// Error returns error result with translation via i18n.T()
// Note: first argument is an i18n key, not a Go format string.
func Error(key string, args ...any) ToolResult {
	return ToolResult{Error: i18n.T(key, args...)}
}

// Tool is a tool interface for the agent
type Tool interface {
	// Name returns tool name (e.g. "read", "bash")
	Name() string
	// Description returns description for system prompt
	Description() string
	// Execute executes tool with given parameters
	Execute(params map[string]string) ToolResult
	// Parameters returns JSON Schema tool parameters (for function calling)
	Parameters() map[string]any
}

// AsyncEvent is an asynchronous tool execution event
type AsyncEvent struct {
	Type   string // "progress" or "result"
	Output string // partial output (for progress) or full result
	Error  string // error (if any)
	Done   bool   // true when execution is complete
}

// AsyncTool is an interface for tools supporting async execution
type AsyncTool interface {
	Tool
	// ExecuteAsync executes tool asynchronously, returning a channel with progress events
	ExecuteAsync(params map[string]string) <-chan AsyncEvent
}

// ToolHook is a tool hook (called before/after execution)
type ToolHook struct {
	Name string
	// BeforeExecute is called before tool execution.
	// Returns modified parameters or error to abort.
	BeforeExecute func(toolName string, params map[string]string) (map[string]string, error)
	// AfterExecute is called after tool execution.
	AfterExecute func(toolName string, params map[string]string, result ToolResult, duration time.Duration)
}

// HookedTool is a wrapper over a tool with hook support
type HookedTool struct {
	inner Tool
	hooks []*ToolHook
}

// NewHookedTool creates a wrapped tool with hooks
func NewHookedTool(inner Tool, hooks ...*ToolHook) *HookedTool {
	return &HookedTool{
		inner: inner,
		hooks: hooks,
	}
}

func (ht *HookedTool) Name() string               { return ht.inner.Name() }
func (ht *HookedTool) Description() string        { return ht.inner.Description() }
func (ht *HookedTool) Parameters() map[string]any { return ht.inner.Parameters() }

func (ht *HookedTool) Execute(params map[string]string) ToolResult {
	// Call BeforeExecute hooks
	modifiedParams := params
	for _, hook := range ht.hooks {
		if hook.BeforeExecute != nil {
			newParams, err := hook.BeforeExecute(ht.inner.Name(), modifiedParams)
			if err != nil {
				return ToolResult{Error: err.Error()}
			}
			modifiedParams = newParams
		}
	}

	// Execute tool
	start := time.Now()
	result := ht.inner.Execute(modifiedParams)
	duration := time.Since(start)

	// Call AfterExecute hooks
	for _, hook := range ht.hooks {
		if hook.AfterExecute != nil {
			hook.AfterExecute(ht.inner.Name(), modifiedParams, result, duration)
		}
	}

	return result
}

func (ht *HookedTool) ExecuteAsync(params map[string]string) <-chan AsyncEvent {
	// Check if internal tool supports async execution
	if asyncTool, ok := ht.inner.(AsyncTool); ok {
		return asyncTool.ExecuteAsync(params)
	}
	// Fallback: start synchronously in goroutine
	ch := make(chan AsyncEvent, 1)
	go func() {
		defer close(ch)
		result := ht.Execute(params)
		ch <- AsyncEvent{Type: "result", Output: result.Output, Error: result.Error, Done: true}
	}()
	return ch
}
