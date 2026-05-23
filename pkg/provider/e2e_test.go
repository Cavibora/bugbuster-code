package provider

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

// skipIfNoOllama skips the test if Ollama is not running locally.
func skipIfNoOllama(t *testing.T) {
	t.Helper()

	if os.Getenv("E2E_OLLAMA") == "" {
		t.Skip("Skipping E2E test: set E2E_OLLAMA=1 to run")
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		t.Skip("Skipping E2E test: Ollama not running on localhost:11434")
	}
	resp.Body.Close()
}

func newTestOllamaProvider(t *testing.T) *OllamaProvider {
	t.Helper()

	cfg := ProviderConfig{
		Type:    "ollama",
		BaseURL: "http://localhost:11434",
		Model:   "glm-5.1:cloud",
	}

	p, err := NewOllamaProvider("test-ollama", cfg)
	if err != nil {
		t.Fatalf("Failed to create OllamaProvider: %v", err)
	}
	return p
}

func TestE2E_OllamaComplete(t *testing.T) {
	skipIfNoOllama(t)
	p := newTestOllamaProvider(t)

	msgs := []Message{
		UserMsg("Say 'hello world' and nothing else."),
	}

	result, err := p.Complete(msgs, nil)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	if result == nil {
		t.Fatal("Result is nil")
	}

	if result.Message.GetText() == "" {
		t.Error("Expected non-empty response text")
	}

	t.Logf("Response: %s", truncate(result.Message.GetText(), 200))
}

func TestE2E_OllamaStream(t *testing.T) {
	skipIfNoOllama(t)
	p := newTestOllamaProvider(t)

	msgs := []Message{
		UserMsg("Say 'hello world' and nothing else."),
	}

	eventCh, err := p.Stream(msgs, nil)
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	var text string
	for event := range eventCh {
		if event.Type == EventTextDelta {
			text += event.Text
		}
	}

	if text == "" {
		t.Error("Expected non-empty streamed text")
	}

	t.Logf("Streamed response: %s", truncate(text, 200))
}

func TestE2E_OllamaStreamWithCtx(t *testing.T) {
	skipIfNoOllama(t)
	p := newTestOllamaProvider(t)

	tools := []ToolDef{
		{
			Name:        "read",
			Description: "Read a file at the given path",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to the file",
					},
				},
				"required": []any{"path"},
			},
		},
	}

	msgs := []Message{
		UserMsg("Read the file /etc/hostname using the read tool."),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	eventCh, err := p.StreamWithCtx(ctx, msgs, tools)
	if err != nil {
		t.Fatalf("StreamWithCtx with tools failed: %v", err)
	}

	var text string
	var toolCallCount int
	for event := range eventCh {
		switch event.Type {
		case EventTextDelta:
			text += event.Text
		case EventToolCallStart:
			toolCallCount++
		case EventToolCallEnd:
			t.Logf("Tool call end: %s input=%v", event.ToolName, event.ToolInput)
		}
	}

	t.Logf("Response text: %s", truncate(text, 200))
	t.Logf("Tool call count: %d", toolCallCount)
}

func TestE2E_OllamaCompleteWithTools(t *testing.T) {
	skipIfNoOllama(t)
	p := newTestOllamaProvider(t)

	tools := []ToolDef{
		{
			Name:        "read",
			Description: "Read a file at the given path",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to the file",
					},
				},
				"required": []any{"path"},
			},
		},
	}

	msgs := []Message{
		UserMsg("Read the file /etc/hostname using the read tool."),
	}

	result, err := p.Complete(msgs, tools)
	if err != nil {
		t.Fatalf("Complete with tools failed: %v", err)
	}

	if result == nil {
		t.Fatal("Result is nil")
	}

	t.Logf("Response text: %s", truncate(result.Message.GetText(), 200))
}

func TestE2E_OllamaMultiTurn(t *testing.T) {
	skipIfNoOllama(t)
	p := newTestOllamaProvider(t)

	msgs := []Message{
		UserMsg("My name is TestBot. Remember it."),
	}

	result, err := p.Complete(msgs, nil)
	if err != nil {
		t.Fatalf("First turn failed: %v", err)
	}

	if result == nil || result.Message.GetText() == "" {
		t.Fatal("First turn: empty response")
	}

	// Second turn
	msgs = append(msgs, result.Message, UserMsg("What is my name?"))

	result2, err := p.Complete(msgs, nil)
	if err != nil {
		t.Fatalf("Second turn failed: %v", err)
	}

	if result2 == nil || result2.Message.GetText() == "" {
		t.Fatal("Second turn: empty response")
	}

	t.Logf("First turn: %s", truncate(result.Message.GetText(), 100))
	t.Logf("Second turn: %s", truncate(result2.Message.GetText(), 100))
}

func TestE2E_OllamaErrorWrapping(t *testing.T) {
	skipIfNoOllama(t)

	// Test with invalid model to trigger an error
	cfg := ProviderConfig{
		Type:    "ollama",
		BaseURL: "http://localhost:11434",
		Model:   "nonexistent-model-xyz",
	}

	p, err := NewOllamaProvider("test-ollama", cfg)
	if err != nil {
		t.Fatalf("Failed to create OllamaProvider: %v", err)
	}

	msgs := []Message{
		UserMsg("Hello"),
	}

	_, err = p.Complete(msgs, nil)
	if err == nil {
		t.Fatal("Expected error for nonexistent model, got nil")
	}

	t.Logf("Error (expected): %v", err)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
