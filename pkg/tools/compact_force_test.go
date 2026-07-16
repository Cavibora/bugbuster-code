package tools

import (
	"strings"
	"testing"
	"time"
)

// mockCompactForceContext implements CompactForceContext for testing
type mockCompactForceContext struct {
	tokenCount int
	maxTokens  int
	compacted  bool
}

func (m *mockCompactForceContext) CompactForce() {
	m.compacted = true
	m.tokenCount = m.tokenCount / 2 // simulate compaction reducing tokens
}

func (m *mockCompactForceContext) TokenCount() int  { return m.tokenCount }
func (m *mockCompactForceContext) MaxTokensValue() int { return m.maxTokens }

// mockCooldown implements CompactForceCooldown for testing
type mockCooldown struct {
	cooldown bool
}

func (m *mockCooldown) IsCompactForceCooldown() bool { return m.cooldown }
func (m *mockCooldown) SetCompactForceCooldown()      { m.cooldown = true }

func TestCompactForceTool_BasicCompaction(t *testing.T) {
	ctx := &mockCompactForceContext{tokenCount: 10000, maxTokens: 20000}
	tool := NewCompactForceTool(ctx)

	result := tool.Execute(map[string]string{})
	if result.Error != "" {
		t.Errorf("Expected no error, got: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Force compacted") {
		t.Errorf("Expected success message, got: %s", result.Output)
	}
	if !ctx.compacted {
		t.Error("Expected CompactForce to be called")
	}
}

func TestCompactForceTool_CooldownViaTime(t *testing.T) {
	ctx := &mockCompactForceContext{tokenCount: 10000, maxTokens: 20000}
	tool := NewCompactForceTool(ctx)

	// First call should succeed
	result := tool.Execute(map[string]string{})
	if result.Error != "" {
		t.Errorf("Expected no error on first call, got: %s", result.Error)
	}

	// Second call immediately after should be blocked by time-based cooldown
	result2 := tool.Execute(map[string]string{})
	if strings.Contains(result2.Output, "Force compacted") {
		t.Error("Expected second call to be blocked by cooldown, but compaction happened")
	}
	if !strings.Contains(result2.Output, "cooldown") && !strings.Contains(result2.Output, "wait") {
		t.Errorf("Expected cooldown message, got: %s", result2.Output)
	}
}

func TestCompactForceTool_CooldownViaInterface(t *testing.T) {
	ctx := &mockCompactForceContext{tokenCount: 10000, maxTokens: 20000}
	tool := NewCompactForceTool(ctx)
	cooldown := &mockCooldown{cooldown: true}
	tool.SetCooldown(cooldown)

	// Call should be blocked because cooldown is active
	result := tool.Execute(map[string]string{})
	if strings.Contains(result.Output, "Force compacted") {
		t.Error("Expected call to be blocked by cooldown interface, but compaction happened")
	}
	if !strings.Contains(result.Output, "recently compacted") {
		t.Errorf("Expected 'recently compacted' message, got: %s", result.Output)
	}
}

func TestCompactForceTool_CooldownInterfaceSetAfterSuccess(t *testing.T) {
	ctx := &mockCompactForceContext{tokenCount: 10000, maxTokens: 20000}
	tool := NewCompactForceTool(ctx)
	cooldown := &mockCooldown{cooldown: false}
	tool.SetCooldown(cooldown)

	// First call should succeed and set cooldown
	result := tool.Execute(map[string]string{})
	if result.Error != "" {
		t.Errorf("Expected no error, got: %s", result.Error)
	}
	if !cooldown.cooldown {
		t.Error("Expected cooldown to be set after successful compact_force call")
	}
}

func TestCompactForceTool_NoContext(t *testing.T) {
	tool := NewCompactForceTool(nil)
	result := tool.Execute(map[string]string{})
	if result.Error == "" {
		t.Error("Expected error when no context available")
	}
}

func TestCompactForceTool_SetMinInterval(t *testing.T) {
	ctx := &mockCompactForceContext{tokenCount: 10000, maxTokens: 20000}
	tool := NewCompactForceTool(ctx)
	tool.SetMinInterval(100 * time.Millisecond)

	// First call should succeed
	result := tool.Execute(map[string]string{})
	if result.Error != "" {
		t.Errorf("Expected no error on first call, got: %s", result.Error)
	}

	// Wait for cooldown to expire
	time.Sleep(150 * time.Millisecond)

	// Second call should succeed after cooldown expires
	result2 := tool.Execute(map[string]string{})
	if strings.Contains(result2.Output, "cooldown") {
		t.Errorf("Expected second call to succeed after cooldown expired, got: %s", result2.Output)
	}
}

func TestCompactForceTool_NameAndDescription(t *testing.T) {
	ctx := &mockCompactForceContext{tokenCount: 10000, maxTokens: 20000}
	tool := NewCompactForceTool(ctx)

	if tool.Name() != "compact_force" {
		t.Errorf("Expected name 'compact_force', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Expected non-empty description")
	}
}