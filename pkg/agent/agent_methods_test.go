package agent

import (
	"testing"
)

func TestEffectiveMaxTokens(t *testing.T) {
	a := NewAgentLoop(nil)
	// Default: 8000 (from NewConversationContextWithTokens)
	if a.effectiveMaxTokens() != 8000 {
		t.Errorf("Expected default MaxTokens=8000, got %d", a.effectiveMaxTokens())
	}
	a.SetMaxTokens(16000)
	if a.effectiveMaxTokens() != 16000 {
		t.Errorf("Expected MaxTokens=16000, got %d", a.effectiveMaxTokens())
	}
}

func TestEnableSubagents(t *testing.T) {
	a := NewAgentLoop(nil)
	a.EnableSubagents(DefaultSubagentConfig())
	// Should register delegate_task tool
	if _, ok := a.Tools[DelegateTaskToolName]; !ok {
		t.Error("Expected delegate_task tool to be registered after EnableSubagents")
	}
}

func TestInjectUserMessage(t *testing.T) {
	a := NewAgentLoop(nil)
	result := a.InjectUserMessage("test message")
	if !result {
		t.Error("Expected InjectUserMessage to return true")
	}
	// Channel should have the message
	select {
	case msg := <-a.userInject:
		if msg != "test message" {
			t.Errorf("Expected 'test message', got '%s'", msg)
		}
	default:
		t.Error("Expected message in userInject channel")
	}
}

func TestInjectUserMessage_Multiple(t *testing.T) {
	a := NewAgentLoop(nil)
	a.InjectUserMessage("msg1")
	a.InjectUserMessage("msg2")
	a.InjectUserMessage("msg3")

	messages := []string{}
	for i := 0; i < 3; i++ {
		select {
		case msg := <-a.userInject:
			messages = append(messages, msg)
		default:
			t.Error("Expected message in userInject channel")
		}
	}
	if len(messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(messages))
	}
}

func TestInjectUserMessage_ChannelFull(t *testing.T) {
	a := NewAgentLoop(nil)
	// Fill the channel (capacity 16)
	for i := 0; i < 16; i++ {
		result := a.InjectUserMessage("msg")
		if !result {
			t.Errorf("Expected InjectUserMessage to return true for message %d", i)
		}
	}
	// Next message should fail
	result := a.InjectUserMessage("overflow")
	if result {
		t.Error("Expected InjectUserMessage to return false when channel is full")
	}
}
