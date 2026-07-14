package tools

import (
	"fmt"
	"sync"
)

// CompactForceContext is an interface that allows the compact_force tool
// to trigger aggressive context compaction on the agent loop.
type CompactForceContext interface {
	CompactForce()
	TokenCount() int
	MaxTokensValue() int
}

// CompactForceTool allows the model to self-trigger aggressive compaction
// when it detects slowdown via the performance mirror.
type CompactForceTool struct {
	mu      sync.Mutex
	context CompactForceContext
}

// NewCompactForceTool creates a new compact_force tool
func NewCompactForceTool(ctx CompactForceContext) *CompactForceTool {
	return &CompactForceTool{context: ctx}
}

func (t *CompactForceTool) Name() string { return "compact_force" }

func (t *CompactForceTool) Description() string {
	return "Force compact the context — strips all tool calls, errors, thinking blocks, and low-value data. Use this when you notice slowdown in the performance mirror or when context is getting large. This will significantly reduce context size and speed up your responses."
}

func (t *CompactForceTool) Execute(params map[string]string) ToolResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.context == nil {
		return Error("compact_force: no context available")
	}

	tokensBefore := t.context.TokenCount()
	t.context.CompactForce()
	tokensAfter := t.context.TokenCount()
	saved := tokensBefore - tokensAfter

	return Success(fmt.Sprintf("Force compacted: %d → %d tokens (saved: %d). Context has been aggressively reduced — tool calls, errors, and thinking blocks have been removed.", tokensBefore, tokensAfter, saved))
}

func (t *CompactForceTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}
