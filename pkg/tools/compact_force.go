package tools

import (
	"fmt"
	"sync"
	"time"
)

// CompactForceContext is an interface that allows the compact_force tool
// to trigger aggressive context compaction on the agent loop.
type CompactForceContext interface {
	CompactForce()
	TokenCount() int
	MaxTokensValue() int
}

// CompactForceCooldown is an interface that allows the compact_force tool
// to check and set cooldown state on the agent loop.
// This prevents repeated compaction calls within the cooldown period.
type CompactForceCooldown interface {
	IsCompactForceCooldown() bool
	SetCompactForceCooldown()
}

// CompactForceTool allows the model to self-trigger aggressive compaction
// when it detects slowdown via the performance mirror.
type CompactForceTool struct {
	mu      sync.Mutex
	context CompactForceContext
	cooldown CompactForceCooldown
	lastCall time.Time // time of last successful compact_force call
	minInterval time.Duration // minimum interval between calls (default: 60s)
}

// NewCompactForceTool creates a new compact_force tool
func NewCompactForceTool(ctx CompactForceContext) *CompactForceTool {
	return &CompactForceTool{
		context:     ctx,
		minInterval: 60 * time.Second,
	}
}

// SetCooldown sets the cooldown interface for preventing repeated compaction.
func (t *CompactForceTool) SetCooldown(cd CompactForceCooldown) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cooldown = cd
}

// SetMinInterval sets the minimum interval between compact_force calls.
func (t *CompactForceTool) SetMinInterval(d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.minInterval = d
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

	// Check time-based cooldown — prevent calling more than once per minInterval
	now := time.Now()
	if !t.lastCall.IsZero() && now.Sub(t.lastCall) < t.minInterval {
		remaining := t.minInterval - now.Sub(t.lastCall)
		return Success(fmt.Sprintf(
			"Compact force was already called recently. Please wait %s before calling again. "+
				"Context compaction has a cooldown to prevent excessive context reduction.",
			remaining.Truncate(time.Second).String(),
		))
	}

	// Check agent-level cooldown (set by injectSpeedMirror auto-compact)
	if t.cooldown != nil && t.cooldown.IsCompactForceCooldown() {
		return Success(
			"Context was recently compacted (auto-compaction or previous compact_force call). "+
				"Please wait before calling compact_force again. The cooldown prevents excessive context reduction.",
		)
	}

	tokensBefore := t.context.TokenCount()
	t.context.CompactForce()
	tokensAfter := t.context.TokenCount()
	saved := tokensBefore - tokensAfter

	// Record the time of this successful call
	t.lastCall = now

	// Set agent-level cooldown
	if t.cooldown != nil {
		t.cooldown.SetCompactForceCooldown()
	}

	return Success(fmt.Sprintf("Force compacted: %d → %d tokens (saved: %d). Context has been aggressively reduced — tool calls, errors, and thinking blocks have been removed.", tokensBefore, tokensAfter, saved))
}

func (t *CompactForceTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}