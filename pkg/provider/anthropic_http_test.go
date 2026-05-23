package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// HTTP-тесты для Anthropic: Complete, Stream, StreamWithCtx
// =============================================================================

func TestAnthropicComplete_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %q, want /v1/messages", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("x-api-key = %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("anthropic-version = %q", r.Header.Get("anthropic-version"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"content":[{"type":"text","text":"Hello from Claude!"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`)
	}))
	defer ts.Close()

	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type: "anthropic", APIKey: "test-key", Model: "test-model", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Complete([]Message{UserMsg("hi")}, nil)
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if result.Message.GetText() != "Hello from Claude!" {
		t.Errorf("text = %q", result.Message.GetText())
	}
	if result.Usage.InputTokens != 10 {
		t.Errorf("input_tokens = %d", result.Usage.InputTokens)
	}
}

func TestAnthropicComplete_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintf(w, `{"error": "rate limited"}`)
	}))
	defer ts.Close()

	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type: "anthropic", APIKey: "test-key", Model: "test-model", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	p.retryPolicy = NoRetryPolicy()

	_, err = p.Complete([]Message{UserMsg("hi")}, nil)
	if err == nil {
		t.Fatal("Expected error for 429")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should contain 429: %v", err)
	}
}

func TestAnthropicComplete_ConnectionError(t *testing.T) {
	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type: "anthropic", APIKey: "test-key", Model: "test-model", BaseURL: "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	p.retryPolicy = NoRetryPolicy()

	_, err = p.Complete([]Message{UserMsg("hi")}, nil)
	if err == nil {
		t.Fatal("Expected connection error")
	}
}

func TestAnthropicCompleteWithCtx_Cancelled(t *testing.T) {
	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type: "anthropic", APIKey: "test-key", Model: "test-model", BaseURL: "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = p.CompleteWithCtx(ctx, []Message{UserMsg("test")}, nil)
	if err == nil {
		t.Fatal("Expected error for cancelled context")
	}
}

func TestAnthropicStream_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\"}\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"content_block\":{\"type\":\"text\"}}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\" Claude!\"}}\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\"}\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n",
		}
		for _, evt := range events {
			fmt.Fprintf(w, "%s\n", evt)
			flusher.Flush()
		}
	}))
	defer ts.Close()

	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type: "anthropic", APIKey: "test-key", Model: "test-model", BaseURL: ts.URL,
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
	if combined != "Hello Claude!" {
		t.Errorf("text = %q, want 'Hello Claude!'", combined)
	}
}

func TestAnthropicStream_WithThinking(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\"}\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"content_block\":{\"type\":\"thinking\"}}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"Let me think\"}}\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\"}\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"content_block\":{\"type\":\"text\"}}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"42\"}}\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\"}\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n",
		}
		for _, evt := range events {
			fmt.Fprintf(w, "%s\n", evt)
			flusher.Flush()
		}
	}))
	defer ts.Close()

	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type: "anthropic", APIKey: "test-key", Model: "test-model", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := p.Stream([]Message{UserMsg("think")}, nil)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	var thinking, text []string
	for evt := range ch {
		switch evt.Type {
		case EventThinking:
			thinking = append(thinking, evt.Text)
		case "text_delta":
			text = append(text, evt.Text)
		}
	}
	if strings.Join(thinking, "") != "Let me think" {
		t.Errorf("thinking = %q", strings.Join(thinking, ""))
	}
	if strings.Join(text, "") != "42" {
		t.Errorf("text = %q", strings.Join(text, ""))
	}
}

func TestAnthropicStream_WithToolUse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\"}\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"content_block\":{\"type\":\"tool_use\",\"id\":\"tool_1\",\"name\":\"read\"}}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\"}}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"x.go\\\"}\"}}\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\"}\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n",
		}
		for _, evt := range events {
			fmt.Fprintf(w, "%s\n", evt)
			flusher.Flush()
		}
	}))
	defer ts.Close()

	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type: "anthropic", APIKey: "test-key", Model: "test-model", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := p.Stream([]Message{UserMsg("read x.go")}, nil)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	var toolStarts, toolDeltas, toolEnds int
	for evt := range ch {
		switch evt.Type {
		case EventToolCallStart:
			toolStarts++
			if evt.ToolCallID != "tool_1" {
				t.Errorf("tool id = %q", evt.ToolCallID)
			}
			if evt.ToolName != "read" {
				t.Errorf("tool name = %q", evt.ToolName)
			}
		case EventToolCallDelta:
			toolDeltas++
		case EventToolCallEnd:
			toolEnds++
		}
	}
	if toolStarts != 1 {
		t.Errorf("tool_start = %d, want 1", toolStarts)
	}
	if toolDeltas != 2 {
		t.Errorf("tool_delta = %d, want 2", toolDeltas)
	}
	if toolEnds != 1 {
		t.Errorf("tool_end = %d, want 1", toolEnds)
	}
}

func TestAnthropicStream_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "server error")
	}))
	defer ts.Close()

	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type: "anthropic", APIKey: "test-key", Model: "test-model", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	p.retryPolicy = NoRetryPolicy()

	_, err = p.Stream([]Message{UserMsg("hi")}, nil)
	if err == nil {
		t.Fatal("Expected error for 500")
	}
}

func TestAnthropicStream_ConnectionError(t *testing.T) {
	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type: "anthropic", APIKey: "test-key", Model: "test-model", BaseURL: "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	p.retryPolicy = NoRetryPolicy()

	_, err = p.Stream([]Message{UserMsg("hi")}, nil)
	if err == nil {
		t.Fatal("Expected connection error")
	}
}

func TestAnthropicStreamWithCtx_Cancelled(t *testing.T) {
	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type: "anthropic", APIKey: "test-key", Model: "test-model", BaseURL: "http://127.0.0.1:1",
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

func TestAnthropicStream_WithReasoningContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\"}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"reasoning_content\":\"reasoning here\"}}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"answer\"}}\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n",
		}
		for _, evt := range events {
			fmt.Fprintf(w, "%s\n", evt)
			flusher.Flush()
		}
	}))
	defer ts.Close()

	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type: "anthropic", APIKey: "test-key", Model: "test-model", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := p.Stream([]Message{UserMsg("think")}, nil)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	var thinking []string
	var text []string
	for evt := range ch {
		switch evt.Type {
		case EventThinking:
			thinking = append(thinking, evt.Text)
		case "text_delta":
			text = append(text, evt.Text)
		}
	}
	if strings.Join(thinking, "") != "reasoning here" {
		t.Errorf("thinking = %q", strings.Join(thinking, ""))
	}
	if strings.Join(text, "") != "answer" {
		t.Errorf("text = %q", strings.Join(text, ""))
	}
}

func TestAnthropicCompleteWithCtx_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"content":[{"type":"text","text":"ctx response"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":3}}`)
	}))
	defer ts.Close()

	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type: "anthropic", APIKey: "test-key", Model: "test-model", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.CompleteWithCtx(context.Background(), []Message{UserMsg("test")}, nil)
	if err != nil {
		t.Fatalf("CompleteWithCtx error: %v", err)
	}
	if result.Message.GetText() != "ctx response" {
		t.Errorf("text = %q", result.Message.GetText())
	}
}

func TestAnthropicStreamWithCtx_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\"}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"ctx stream\"}}\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n",
		}
		for _, evt := range events {
			fmt.Fprintf(w, "%s\n", evt)
			flusher.Flush()
		}
	}))
	defer ts.Close()

	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type: "anthropic", APIKey: "test-key", Model: "test-model", BaseURL: ts.URL,
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
	if strings.Join(texts, "") != "ctx stream" {
		t.Errorf("text = %q", strings.Join(texts, ""))
	}
}

func TestAnthropicCompleteWithCtx_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer ts.Close()

	p, err := NewAnthropicProvider("test", ProviderConfig{
		Type: "anthropic", APIKey: "test-key", Model: "test-model", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = p.CompleteWithCtx(ctx, []Message{UserMsg("test")}, nil)
	if err == nil {
		t.Fatal("Expected timeout error")
	}
}
