package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// HTTP-тесты для OpenAI: Complete, Stream, parseStream, convertMessage
// =============================================================================

func TestOpenAIComplete_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if reqBody["model"] != "gpt-4o" {
			t.Errorf("model = %v", reqBody["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":"Hello from GPT!"},"_finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "test-key", Model: "gpt-4o", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Complete([]Message{UserMsg("hi")}, nil)
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if result.Message.GetText() != "Hello from GPT!" {
		t.Errorf("text = %q", result.Message.GetText())
	}
	if result.Usage.InputTokens != 10 {
		t.Errorf("input_tokens = %d", result.Usage.InputTokens)
	}
}

func TestOpenAIComplete_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintf(w, `{"error": "rate limited"}`)
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "test-key", Model: "gpt-4o", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	p.SetRetryPolicy(NoRetryPolicy())

	_, err = p.Complete([]Message{UserMsg("hi")}, nil)
	if err == nil {
		t.Fatal("Expected error for 429")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should contain 429: %v", err)
	}
}

func TestOpenAIComplete_ConnectionError(t *testing.T) {
	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "test-key", Model: "gpt-4o", BaseURL: "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	p.SetRetryPolicy(NoRetryPolicy())

	_, err = p.Complete([]Message{UserMsg("hi")}, nil)
	if err == nil {
		t.Fatal("Expected connection error")
	}
}

func TestOpenAICompleteWithCtx_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":"ctx response"},"_finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3}}`)
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "test-key", Model: "gpt-4o", BaseURL: ts.URL,
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

func TestOpenAICompleteWithCtx_Cancelled(t *testing.T) {
	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "test-key", Model: "gpt-4o", BaseURL: "http://127.0.0.1:1",
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
	if ctx.Err() != context.Canceled {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestOpenAICompleteWithCtx_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "test-key", Model: "gpt-4o", BaseURL: ts.URL,
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

func TestOpenAIDoRequestWithCtx_BadURL(t *testing.T) {
	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "key", Model: "gpt-4o", BaseURL: "http://invalid host with spaces",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = p.doRequestWithCtx(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("Expected error for invalid URL")
	}
}

func TestOpenAIDoRequest_InvalidURL(t *testing.T) {
	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "key", Model: "gpt-4o", BaseURL: "ht tp://bad",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = p.doRequest([]byte(`{}`))
	if err == nil {
		t.Fatal("Expected error for invalid URL")
	}
}

func TestOpenAIDoRequest_ConnectionRefused(t *testing.T) {
	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "key", Model: "gpt-4o", BaseURL: "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = p.doRequest([]byte(`{}`))
	if err == nil {
		t.Fatal("Expected connection refused error")
	}
}

func TestOpenAIDoRequest_Non200Response(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "internal server error"}`)
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "key", Model: "gpt-4o", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, statusCode, err := p.doRequest([]byte(`{"model":"gpt-4o","messages":[]}`))
	if err == nil {
		t.Fatal("Expected error for 500")
	}
	if statusCode != 500 {
		t.Errorf("statusCode = %d, want 500", statusCode)
	}
}

func TestOpenAIDoRequest_InvalidResponseBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{invalid json}`)
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type: "openai", APIKey: "key", Model: "gpt-4o", BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = p.doRequest([]byte(`{"model":"gpt-4o","messages":[]}`))
	if err == nil {
		t.Fatal("Expected parse error for invalid JSON response")
	}
}
