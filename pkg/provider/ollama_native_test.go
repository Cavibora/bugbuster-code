package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaProvider_Name(t *testing.T) {
	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:  "ollama",
		Model: "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}
	if p.Name() != "test-ollama" {
		t.Errorf("Name() = %q, want %q", p.Name(), "test-ollama")
	}
}

func TestOllamaProvider_DefaultBaseURL(t *testing.T) {
	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:  "ollama",
		Model: "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}
	if p.baseURL != "http://localhost:11434" {
		t.Errorf("baseURL = %q, want %q", p.baseURL, "http://localhost:11434")
	}
}

func TestOllamaProvider_DefaultModel(t *testing.T) {
	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type: "ollama",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}
	if p.model != "qwen-fast-27b" {
		t.Errorf("model = %q, want %q", p.model, "qwen-fast-27b")
	}
}

func TestOllamaProvider_Complete_NativeSuccess(t *testing.T) {
	// Create a server that fails OpenAI-compatible API but succeeds native API
	openaiCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			openaiCalled = true
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "not supported"}`))
			return
		}
		// Native Ollama API
		if r.URL.Path == "/api/chat" {
			resp := map[string]any{
				"message": map[string]string{
					"role":    "assistant",
					"content": "Hello from Ollama!",
				},
				"done": true,
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:    "ollama",
		BaseURL: server.URL,
		Model:   "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	result, err := p.Complete([]Message{UserMsg("hello")}, nil)
	if err != nil {
		t.Errorf("Complete() error: %v", err)
	}
	if !openaiCalled {
		t.Error("OpenAI-compatible API should be tried first")
	}
	if result.Message.GetText() != "Hello from Ollama!" {
		t.Errorf("Complete() text = %q, want %q", result.Message.GetText(), "Hello from Ollama!")
	}
}

func TestOllamaProvider_Complete_NativeWithThinking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Path == "/api/chat" {
			resp := map[string]any{
				"message": map[string]string{
					"role":     "assistant",
					"content":  "The answer is 42",
					"thinking": "Let me think about this...",
				},
				"done": true,
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:    "ollama",
		BaseURL: server.URL,
		Model:   "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	result, err := p.Complete([]Message{UserMsg("hello")}, nil)
	if err != nil {
		t.Errorf("Complete() error: %v", err)
	}
	thinking := result.Message.GetThinking()
	if thinking != "Let me think about this..." {
		t.Errorf("thinking = %q, want %q", thinking, "Let me think about this...")
	}
}

func TestOllamaProvider_Complete_NativeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer server.Close()

	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:    "ollama",
		BaseURL: server.URL,
		Model:   "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	_, err = p.Complete([]Message{UserMsg("hello")}, nil)
	if err == nil {
		t.Error("Complete() should return error for HTTP 500")
	}
}

func TestOllamaProvider_Stream_NativeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Path == "/api/chat" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"Hello"},"done":false}` + "\n"))
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":""},"done":true}` + "\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:    "ollama",
		BaseURL: server.URL,
		Model:   "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	ch, err := p.Stream([]Message{UserMsg("hello")}, nil)
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var texts []string
	for event := range ch {
		if event.Type == "text_delta" {
			texts = append(texts, event.Text)
		}
	}
	if len(texts) == 0 {
		t.Error("Stream() should produce text events")
	}
}

func TestOllamaProvider_Stream_NativeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:    "ollama",
		BaseURL: server.URL,
		Model:   "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	_, err = p.Stream([]Message{UserMsg("hello")}, nil)
	if err == nil {
		t.Error("Stream() should return error for HTTP 500")
	}
}

func TestOllamaProvider_CompleteWithCtx_NativeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Path == "/api/chat" {
			resp := map[string]any{
				"message": map[string]string{
					"role":    "assistant",
					"content": "Hello with context!",
				},
				"done": true,
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:    "ollama",
		BaseURL: server.URL,
		Model:   "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	ctx := context.Background()
	result, err := p.CompleteWithCtx(ctx, []Message{UserMsg("hello")}, nil)
	if err != nil {
		t.Errorf("CompleteWithCtx() error: %v", err)
	}
	if result.Message.GetText() != "Hello with context!" {
		t.Errorf("CompleteWithCtx() text = %q, want %q", result.Message.GetText(), "Hello with context!")
	}
}

func TestOllamaProvider_StreamWithCtx_NativeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Path == "/api/chat" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"Stream"},"done":false}` + "\n"))
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":""},"done":true}` + "\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:    "ollama",
		BaseURL: server.URL,
		Model:   "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	ctx := context.Background()
	ch, err := p.StreamWithCtx(ctx, []Message{UserMsg("hello")}, nil)
	if err != nil {
		t.Fatalf("StreamWithCtx() error: %v", err)
	}

	var texts []string
	for event := range ch {
		if event.Type == "text_delta" {
			texts = append(texts, event.Text)
		}
	}
	if len(texts) == 0 {
		t.Error("StreamWithCtx() should produce text events")
	}
}

func TestOllamaProvider_buildNativeRequest(t *testing.T) {
	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:          "ollama",
		Model:         "llama3",
		MaxTokens:     4096,
		ContextWindow: 8192,
		Temperature:   0.7,
		TopP:          0.9,
		TopK:          40,
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	req := p.buildNativeRequest([]Message{UserMsg("hello")}, nil, false)

	if req["model"] != "llama3" {
		t.Errorf("model = %v, want llama3", req["model"])
	}
	if req["stream"] != false {
		t.Errorf("stream = %v, want false", req["stream"])
	}

	options, ok := req["options"].(map[string]any)
	if !ok {
		t.Fatal("options should be a map")
	}
	if options["num_predict"] != 4096 {
		t.Errorf("num_predict = %v, want 4096", options["num_predict"])
	}
	if options["num_ctx"] != 8192 {
		t.Errorf("num_ctx = %v, want 8192", options["num_ctx"])
	}
	if options["temperature"] != 0.7 {
		t.Errorf("temperature = %v, want 0.7", options["temperature"])
	}
	if options["top_p"] != 0.9 {
		t.Errorf("top_p = %v, want 0.9", options["top_p"])
	}
	if options["top_k"] != 40 {
		t.Errorf("top_k = %v, want 40", options["top_k"])
	}
}

func TestOllamaProvider_buildNativeRequest_DefaultNumPredict(t *testing.T) {
	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:  "ollama",
		Model: "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	req := p.buildNativeRequest([]Message{UserMsg("hello")}, nil, false)

	options, ok := req["options"].(map[string]any)
	if !ok {
		t.Fatal("options should be a map")
	}
	if options["num_predict"] != 32768 {
		t.Errorf("num_predict = %v, want 32768 (default)", options["num_predict"])
	}
}

func TestOllamaProvider_buildNativeRequest_WithTools(t *testing.T) {
	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:  "ollama",
		Model: "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	tools := []ToolDef{
		{
			Name:        "bash",
			Description: "Run a bash command",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string"},
				},
			},
		},
	}

	req := p.buildNativeRequest([]Message{UserMsg("hello")}, tools, true)

	if req["stream"] != true {
		t.Errorf("stream = %v, want true", req["stream"])
	}
	ollamaTools, ok := req["tools"].([]map[string]any)
	if !ok {
		t.Fatal("tools should be a slice of maps")
	}
	if len(ollamaTools) != 1 {
		t.Errorf("len(tools) = %d, want 1", len(ollamaTools))
	}
	if ollamaTools[0]["function"].(map[string]any)["name"] != "bash" {
		t.Errorf("tool name = %v, want bash", ollamaTools[0]["function"])
	}
}

func TestOllamaProvider_parseNativeResponse(t *testing.T) {
	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:  "ollama",
		Model: "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	body := []byte(`{"message":{"role":"assistant","content":"Hello"},"done":true}`)
	result, err := p.parseNativeResponse(body)
	if err != nil {
		t.Fatalf("parseNativeResponse() error: %v", err)
	}
	if result.Message.GetText() != "Hello" {
		t.Errorf("text = %q, want %q", result.Message.GetText(), "Hello")
	}
}

func TestOllamaProvider_parseNativeResponse_WithThinking(t *testing.T) {
	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:  "ollama",
		Model: "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	body := []byte(`{"message":{"role":"assistant","content":"The answer","thinking":"Let me think"},"done":true}`)
	result, err := p.parseNativeResponse(body)
	if err != nil {
		t.Fatalf("parseNativeResponse() error: %v", err)
	}
	if result.Message.GetThinking() != "Let me think" {
		t.Errorf("thinking = %q, want %q", result.Message.GetThinking(), "Let me think")
	}
	// GetText returns all text content blocks concatenated
	text := result.Message.GetText()
	if text != "Let me think\nThe answer" && text != "The answer" {
		t.Errorf("text = %q, want thinking+answer or just answer", text)
	}
}

func TestOllamaProvider_parseNativeResponse_InvalidJSON(t *testing.T) {
	p, err := NewOllamaProvider("test-ollama", ProviderConfig{
		Type:  "ollama",
		Model: "llama3",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}

	_, err = p.parseNativeResponse([]byte(`invalid json`))
	if err == nil {
		t.Error("parseNativeResponse() should return error for invalid JSON")
	}
}
