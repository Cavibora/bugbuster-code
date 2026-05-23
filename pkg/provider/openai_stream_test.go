package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// =============================================================================
// OpenAI Stream + StreamWithCtx тесты с httptest
// =============================================================================

func writeSSE(w http.ResponseWriter, flusher http.Flusher, events []string) {
	for _, evt := range events {
		fmt.Fprintf(w, "%s\n\n", evt)
		flusher.Flush()
	}
}

func TestOpenAIStream_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support Flusher")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSE(w, flusher, []string{
			`data: {"choices":[{"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`,
			`data: {"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`,
			`data: {"choices":[{"delta":{"content":"!"},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		})
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "test-key", Model: "gpt-4o", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := p.Stream([]Message{UserMsg("hi")}, nil)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	var texts []string
	var gotDone bool
	for evt := range ch {
		switch evt.Type {
		case "text_delta":
			texts = append(texts, evt.Text)
		case "done":
			gotDone = true
		}
	}
	if !gotDone {
		t.Error("Expected 'done' event")
	}
	combined := strings.Join(texts, "")
	if combined != "Hello world!" {
		t.Errorf("text = %q, want 'Hello world!'", combined)
	}
}

func TestOpenAIStream_WithToolCalls(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSE(w, flusher, []string{
			`data: {"choices":[{"delta":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","index":0,"function":{"name":"read","arguments":""}}]},"finish_reason":null}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]},"finish_reason":null}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"main.go\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		})
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "test-key", Model: "gpt-4o", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := p.Stream([]Message{UserMsg("read main.go")}, nil)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	var toolStarts, toolDeltas, toolEnds, dones int
	for evt := range ch {
		switch evt.Type {
		case EventToolCallStart:
			toolStarts++
			if evt.ToolCallID != "call_1" {
				t.Errorf("tool call id = %q, want 'call_1'", evt.ToolCallID)
			}
			if evt.ToolName != "read" {
				t.Errorf("tool name = %q, want 'read'", evt.ToolName)
			}
		case EventToolCallDelta:
			toolDeltas++
		case EventToolCallEnd:
			toolEnds++
		case "done":
			dones++
		}
	}
	if toolStarts != 1 {
		t.Errorf("tool_call_start events = %d, want 1", toolStarts)
	}
	if toolDeltas != 2 {
		t.Errorf("tool_call_delta events = %d, want 2", toolDeltas)
	}
	if toolEnds != 1 {
		t.Errorf("tool_call_end events = %d, want 1", toolEnds)
	}
	if dones != 1 {
		t.Errorf("done events = %d, want 1", dones)
	}
}

func TestOpenAIStream_WithThinking(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSE(w, flusher, []string{
			`data: {"choices":[{"delta":{"reasoning_content":"Let me think"},"finish_reason":null}]}`,
			`data: {"choices":[{"delta":{"content":"The answer"},"finish_reason":null}]}`,
			`data: {"choices":[{"delta":{"content":" is 42"},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		})
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "test-key", Model: "gpt-4o", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := p.Stream([]Message{UserMsg("think")}, nil)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	var thinking, text []string
	var gotDone bool
	for evt := range ch {
		switch evt.Type {
		case EventThinking:
			thinking = append(thinking, evt.Text)
		case "text_delta":
			text = append(text, evt.Text)
		case "done":
			gotDone = true
		}
	}
	if !gotDone {
		t.Error("Expected 'done' event")
	}
	if strings.Join(thinking, "") != "Let me think" {
		t.Errorf("thinking = %q", strings.Join(thinking, ""))
	}
	if strings.Join(text, "") != "The answer is 42" {
		t.Errorf("text = %q", strings.Join(text, ""))
	}
}

func TestOpenAIStream_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "server error")
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "test-key", Model: "gpt-4o", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	p.SetRetryPolicy(NoRetryPolicy())

	_, err = p.Stream([]Message{UserMsg("hi")}, nil)
	if err == nil {
		t.Fatal("Expected error for 500")
	}
}

func TestOpenAIStream_NonRetryableHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "bad request")
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "test-key", Model: "gpt-4o", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Stream([]Message{UserMsg("hi")}, nil)
	if err == nil {
		t.Fatal("Expected error for 400")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should contain 400: %v", err)
	}
}

func TestOpenAIStream_ConnectionError(t *testing.T) {
	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "test-key", Model: "gpt-4o", BaseURL: "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	p.SetRetryPolicy(NoRetryPolicy())

	_, err = p.Stream([]Message{UserMsg("hi")}, nil)
	if err == nil {
		t.Fatal("Expected connection error")
	}
}

func TestOpenAIStreamWithCtx_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSE(w, flusher, []string{
			`data: {"choices":[{"delta":{"content":"stream ctx"},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		})
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "test-key", Model: "gpt-4o", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := p.StreamWithCtx(context.Background(), []Message{UserMsg("hi")}, nil)
	if err != nil {
		t.Fatalf("StreamWithCtx error: %v", err)
	}
	var texts []string
	for evt := range ch {
		if evt.Type == "text_delta" {
			texts = append(texts, evt.Text)
		}
	}
	if strings.Join(texts, "") != "stream ctx" {
		t.Errorf("text = %q", strings.Join(texts, ""))
	}
}

func TestOpenAIStreamWithCtx_Cancelled(t *testing.T) {
	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "test-key", Model: "gpt-4o", BaseURL: "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = p.StreamWithCtx(ctx, []Message{UserMsg("hi")}, nil)
	if err == nil {
		t.Fatal("Expected error for cancelled context")
	}
}

// --- OpenAI convertMessage edge cases ---

func TestOpenAIConvertMessage_PlainText(t *testing.T) {
	p := newOpenAIProvider(t)
	msg := Message{Role: "user", Text: "hello"}
	result := p.convertMessage(msg)
	if len(result) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(result))
	}
	if result[0]["role"] != "user" {
		t.Errorf("role = %v", result[0]["role"])
	}
	if result[0]["content"] != "hello" {
		t.Errorf("content = %v", result[0]["content"])
	}
}

func TestOpenAIConvertMessage_ToolResult(t *testing.T) {
	p := newOpenAIProvider(t)
	msg := ToolResultMsg("call_1", "read", "file content", false)
	result := p.convertMessage(msg)
	if len(result) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(result))
	}
	if result[0]["role"] != "tool" {
		t.Errorf("role = %v", result[0]["role"])
	}
	if result[0]["tool_call_id"] != "call_1" {
		t.Errorf("tool_call_id = %v", result[0]["tool_call_id"])
	}
}

func TestOpenAIConvertMessage_ToolResultError(t *testing.T) {
	p := newOpenAIProvider(t)
	msg := ToolResultMsg("call_1", "bash", "command failed", true)
	result := p.convertMessage(msg)
	content := result[0]["content"].(string)
	if !strings.HasPrefix(content, "Error: ") {
		t.Errorf("error tool result content = %q, should start with 'Error: '", content)
	}
}

func TestOpenAIConvertMessage_ToolUseWithText(t *testing.T) {
	p := newOpenAIProvider(t)
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "text", Text: "Let me read that."},
			{Type: "tool_use", ToolUseID: "call_1", ToolName: "read", Input: map[string]any{"path": "x.go"}},
		},
	}
	result := p.convertMessage(msg)
	if len(result) != 1 {
		t.Fatalf("Expected 1 entry (combined), got %d", len(result))
	}
	if result[0]["role"] != "assistant" {
		t.Errorf("role = %v", result[0]["role"])
	}
	toolCalls, ok := result[0]["tool_calls"].([]map[string]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("tool_calls = %v", result[0]["tool_calls"])
	}
	fn := toolCalls[0]["function"].(map[string]any)
	if fn["name"] != "read" {
		t.Errorf("function name = %v", fn["name"])
	}
}

func TestOpenAIConvertMessage_TextContentOnly(t *testing.T) {
	p := newOpenAIProvider(t)
	msg := Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "text", Text: "hello"},
			{Type: "text", Text: "world"},
		},
	}
	result := p.convertMessage(msg)
	if len(result) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(result))
	}
	content, ok := result[0]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("content type = %T", result[0]["content"])
	}
	if len(content) != 2 {
		t.Errorf("content blocks = %d, want 2", len(content))
	}
}
