package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenCodeProvider_DefaultBaseURL(t *testing.T) {
	cfg := ProviderConfig{
		Type:  "opencode",
		Model: "opencode-go/glm-5.3-flash",
	}
	p, err := NewOpenCodeProvider("test", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if p.delegate.baseURL != "https://opencode.ai/zen/go/v1" {
		t.Errorf("default base URL = %q, want https://opencode.ai/zen/go/v1", p.delegate.baseURL)
	}
}

func TestOpenCodeProvider_SessionHeaderSent(t *testing.T) {
	var gotSession string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSession = r.Header.Get("x-opencode-session")
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("auth header = %q, want Bearer test-key", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":          "chatcmpl-1",
			"object":      "chat.completion",
			"created":     0,
			"model":       "opencode-go/kimi-k3",
			"choices":     []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
			"usage":       map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	defer srv.Close()

	cfg := ProviderConfig{
		Type:    "opencode",
		Model:   "opencode-go/kimi-k3",
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}
	p, err := NewOpenCodeProvider("test", cfg)
	if err != nil {
		t.Fatal(err)
	}
	p.SetSessionID("sess_20260915_123456_abcd")

	msgs := []Message{{Role: "user", Content: []ContentBlock{{Type: "text", Text: "hi"}}}}
	res, err := p.Complete(msgs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if gotSession != "sess_20260915_123456_abcd" {
		t.Errorf("x-opencode-session header = %q, want sess_20260915_123456_abcd", gotSession)
	}
}

func TestOpenCodeProvider_NoSessionHeaderWhenEmpty(t *testing.T) {
	var gotSession string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSession = r.Header.Get("x-opencode-session")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-1",
			"object":  "chat.completion",
			"model":   "opencode-go/kimi-k3",
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	defer srv.Close()

	cfg := ProviderConfig{
		Type:    "opencode",
		Model:   "opencode-go/kimi-k3",
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}
	p, err := NewOpenCodeProvider("test", cfg)
	if err != nil {
		t.Fatal(err)
	}
	// No SetSessionID call — header should be absent
	msgs := []Message{{Role: "user", Content: []ContentBlock{{Type: "text", Text: "hi"}}}}
	if _, err := p.Complete(msgs, nil); err != nil {
		t.Fatal(err)
	}
	if gotSession != "" {
		t.Errorf("x-opencode-session header = %q, want empty when not set", gotSession)
	}
	if !strings.Contains(cfg.BaseURL, srv.URL) {
		t.Error("base URL mismatch")
	}
}
