package agent

import (
	"testing"
	"time"
)

func TestSetSystemPrompt(t *testing.T) {
	a := NewAgentLoop(nil)
	a.SetSystemPrompt("test prompt")
	if len(a.Context.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(a.Context.Messages))
	}
	if a.Context.Messages[0].Role != "system" {
		t.Errorf("Expected role 'system', got '%s'", a.Context.Messages[0].Role)
	}
	if a.Context.Messages[0].Content[0].Text != "test prompt" {
		t.Errorf("Expected text 'test prompt', got '%s'", a.Context.Messages[0].Content[0].Text)
	}
	// Set again — should replace
	a.SetSystemPrompt("new prompt")
	systemMsgs := 0
	for _, m := range a.Context.Messages {
		if m.Role == "system" {
			systemMsgs++
			if m.Content[0].Text != "new prompt" {
				t.Errorf("Expected text 'new prompt', got '%s'", m.Content[0].Text)
			}
		}
	}
	if systemMsgs != 1 {
		t.Errorf("Expected 1 system message, got %d", systemMsgs)
	}
}

func TestSetVerbose(t *testing.T) {
	a := NewAgentLoop(nil)
	a.SetVerbose(true)
	if !a.verbose {
		t.Error("Expected verbose=true")
	}
	a.SetVerbose(false)
	if a.verbose {
		t.Error("Expected verbose=false")
	}
}

func TestSetDebug(t *testing.T) {
	a := NewAgentLoop(nil)
	a.SetDebug(true)
	if !a.debug {
		t.Error("Expected debug=true")
	}
	a.SetDebug(false)
	if a.debug {
		t.Error("Expected debug=false")
	}
}

func TestSetDebugDir(t *testing.T) {
	a := NewAgentLoop(nil)
	a.SetDebugDir("/tmp/test")
	if a.debugDir != "/tmp/test" {
		t.Errorf("Expected debugDir='/tmp/test', got '%s'", a.debugDir)
	}
}

func TestSetMaxTokens(t *testing.T) {
	a := NewAgentLoop(nil)
	a.SetMaxTokens(16000)
	if a.Context.MaxTokens != 16000 {
		t.Errorf("Expected MaxTokens=16000, got %d", a.Context.MaxTokens)
	}
}

func TestSetKeepRecent(t *testing.T) {
	a := NewAgentLoop(nil)
	a.SetKeepRecent(30)
	if a.Context.KeepRecent != 30 {
		t.Errorf("Expected KeepRecent=30, got %d", a.Context.KeepRecent)
	}
}

func TestSetProvider(t *testing.T) {
	a := NewAgentLoop(nil)
	mock := &MockProvider{}
	a.SetProvider(mock)
	if a.provider != mock {
		t.Error("Expected provider to be set")
	}
}

func TestSetNonInteractive(t *testing.T) {
	a := NewAgentLoop(nil)
	a.SetNonInteractive(true)
	if !a.nonInteractive {
		t.Error("Expected nonInteractive=true")
	}
}

func TestSetPermissionChecker(t *testing.T) {
	a := NewAgentLoop(nil)
	pc := &DefaultPermissionChecker{}
	a.SetPermissionChecker(pc)
	if a.PermissionChecker != pc {
		t.Error("Expected PermissionChecker to be set")
	}
}

func TestSetMaxIterations(t *testing.T) {
	a := NewAgentLoop(nil)
	a.SetMaxIterations(10)
	if a.maxIterations != 10 {
		t.Errorf("Expected maxIterations=10, got %d", a.maxIterations)
	}
}

func TestSetRequestTimeout(t *testing.T) {
	a := NewAgentLoop(nil)
	d := 5 * time.Minute
	a.SetRequestTimeout(d)
	if a.RequestTimeout != d {
		t.Errorf("Expected RequestTimeout=%v, got %v", d, a.RequestTimeout)
	}
}

func TestSetThinkingTimeout(t *testing.T) {
	a := NewAgentLoop(nil)
	d := 3 * time.Minute
	a.SetThinkingTimeout(d)
	if a.ThinkingTimeout != d {
		t.Errorf("Expected ThinkingTimeout=%v, got %v", d, a.ThinkingTimeout)
	}
}

func TestSetIdleTimeout(t *testing.T) {
	a := NewAgentLoop(nil)
	d := 1 * time.Minute
	a.SetIdleTimeout(d)
	if a.IdleTimeout != d {
		t.Errorf("Expected IdleTimeout=%v, got %v", d, a.IdleTimeout)
	}
}

func TestSetLoopRepeatThreshold(t *testing.T) {
	a := NewAgentLoop(nil)
	a.SetLoopRepeatThreshold(5)
	if a.LoopDetector.repeatThreshold != 5 {
		t.Errorf("Expected repeatThreshold=5, got %d", a.LoopDetector.repeatThreshold)
	}
}

func TestSetLoopToolRepeatThreshold(t *testing.T) {
	a := NewAgentLoop(nil)
	a.SetLoopToolRepeatThreshold(10)
	if a.LoopDetector.toolRepeatThreshold != 10 {
		t.Errorf("Expected toolRepeatThreshold=10, got %d", a.LoopDetector.toolRepeatThreshold)
	}
}

func TestSetLoopWindowSize(t *testing.T) {
	a := NewAgentLoop(nil)
	a.SetLoopWindowSize(8)
	if a.LoopDetector.windowSize != 8 {
		t.Errorf("Expected windowSize=8, got %d", a.LoopDetector.windowSize)
	}
}

func TestSetLoopTextSimilarityThreshold(t *testing.T) {
	a := NewAgentLoop(nil)
	a.SetLoopTextSimilarityThreshold(0.9)
	if a.LoopDetector.textSimilarityThreshold != 0.9 {
		t.Errorf("Expected textSimilarityThreshold=0.9, got %f", a.LoopDetector.textSimilarityThreshold)
	}
}

func TestSetLoopTextSimilarityWindow(t *testing.T) {
	a := NewAgentLoop(nil)
	a.SetLoopTextSimilarityWindow(5)
	if a.LoopDetector.textSimilarityWindow != 5 {
		t.Errorf("Expected textSimilarityWindow=5, got %d", a.LoopDetector.textSimilarityWindow)
	}
}

func TestSetLoopRepeatThreshold_NilDetector(t *testing.T) {
	a := &AgentLoop{}
	a.SetLoopRepeatThreshold(5) // should not panic
}

func TestSetLoopToolRepeatThreshold_NilDetector(t *testing.T) {
	a := &AgentLoop{}
	a.SetLoopToolRepeatThreshold(10) // should not panic
}

func TestSetLoopWindowSize_NilDetector(t *testing.T) {
	a := &AgentLoop{}
	a.SetLoopWindowSize(8) // should not panic
}

func TestSetLoopTextSimilarityThreshold_NilDetector(t *testing.T) {
	a := &AgentLoop{}
	a.SetLoopTextSimilarityThreshold(0.9) // should not panic
}

func TestSetLoopTextSimilarityWindow_NilDetector(t *testing.T) {
	a := &AgentLoop{}
	a.SetLoopTextSimilarityWindow(5) // should not panic
}

func TestEffectiveTimeouts(t *testing.T) {
	a := NewAgentLoop(nil)

	// Default timeouts (0 = use defaults)
	if a.effectiveRequestTimeout() != 20*time.Minute {
		t.Errorf("Expected default RequestTimeout=20m, got %v", a.effectiveRequestTimeout())
	}
	if a.effectiveThinkingTimeout() != 10*time.Minute {
		t.Errorf("Expected default ThinkingTimeout=10m, got %v", a.effectiveThinkingTimeout())
	}
	if a.effectiveIdleTimeout() != 5*time.Minute {
		t.Errorf("Expected default IdleTimeout=5m, got %v", a.effectiveIdleTimeout())
	}

	// Custom timeouts
	a.SetRequestTimeout(5 * time.Minute)
	a.SetThinkingTimeout(3 * time.Minute)
	a.SetIdleTimeout(30 * time.Second)

	if a.effectiveRequestTimeout() != 5*time.Minute {
		t.Errorf("Expected RequestTimeout=5m, got %v", a.effectiveRequestTimeout())
	}
	if a.effectiveThinkingTimeout() != 3*time.Minute {
		t.Errorf("Expected ThinkingTimeout=3m, got %v", a.effectiveThinkingTimeout())
	}
	if a.effectiveIdleTimeout() != 30*time.Second {
		t.Errorf("Expected IdleTimeout=30s, got %v", a.effectiveIdleTimeout())
	}
}

func TestRegisterTool(t *testing.T) {
	a := NewAgentLoop(nil)
	// NewAgentLoop already registers 6 tools
	if len(a.Tools) < 6 {
		t.Errorf("Expected at least 6 tools, got %d", len(a.Tools))
	}
}

func TestNewAgentLoop_NilProvider(t *testing.T) {
	a := NewAgentLoop(nil)
	if a.provider != nil {
		t.Error("Expected nil provider")
	}
	if a.Context == nil {
		t.Error("Expected non-nil Context")
	}
	if a.LoopDetector == nil {
		t.Error("Expected non-nil LoopDetector")
	}
}

func TestNewAgentLoop_WithProvider(t *testing.T) {
	mock := &MockProvider{}
	a := NewAgentLoop(mock)
	if a.provider != mock {
		t.Error("Expected provider to be set")
	}
}
