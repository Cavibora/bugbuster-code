package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAICompatProvider_Name(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p, err := NewOpenAICompatProvider("test-compat", ProviderConfig{
		Type:    "openai-compat",
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatProvider: %v", err)
	}
	if p.Name() != "test-compat" {
		t.Errorf("Name() = %q, want %q", p.Name(), "test-compat")
	}
}

func TestOpenAICompatProvider_NoBaseURL(t *testing.T) {
	_, err := NewOpenAICompatProvider("test-compat", ProviderConfig{
		Type:   "openai-compat",
		APIKey: "test-key",
		Model:  "test-model",
	})
	if err == nil {
		t.Error("NewOpenAICompatProvider should return error without BaseURL")
	}
}

func TestOpenAICompatProvider_DefaultModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p, err := NewOpenAICompatProvider("test-compat", ProviderConfig{
		Type:    "openai-compat",
		BaseURL: server.URL,
		APIKey:  "test-key",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatProvider: %v", err)
	}
	if p.delegate.model != "default" {
		t.Errorf("model = %q, want %q", p.delegate.model, "default")
	}
}

func TestOpenAICompatProvider_TrailingSlash(t *testing.T) {
	p, err := NewOpenAICompatProvider("test-compat", ProviderConfig{
		Type:    "openai-compat",
		BaseURL: "https://api.example.com/",
		APIKey:  "test-key",
		Model:   "test-model",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatProvider: %v", err)
	}
	if strings.HasSuffix(p.delegate.baseURL, "/") {
		t.Error("baseURL should not have trailing slash")
	}
}

func TestOpenAICompatProvider_CompleteWithCtx(t *testing.T) {
	p, err := NewOpenAICompatProvider("test-compat", ProviderConfig{
		Type:    "openai-compat",
		BaseURL: "https://api.example.com",
		APIKey:  "test-key",
		Model:   "test-model",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatProvider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// CompleteWithCtx will fail without server
	_, err = p.CompleteWithCtx(ctx, []Message{UserMsg("hello")}, nil)
	if err == nil {
		t.Error("CompleteWithCtx should return error without server")
	}
}

func TestOpenAICompatProvider_StreamWithCtx(t *testing.T) {
	p, err := NewOpenAICompatProvider("test-compat", ProviderConfig{
		Type:    "openai-compat",
		BaseURL: "https://api.example.com",
		APIKey:  "test-key",
		Model:   "test-model",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatProvider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// StreamWithCtx will fail without server
	_, err = p.StreamWithCtx(ctx, []Message{UserMsg("hello")}, nil)
	if err == nil {
		t.Error("StreamWithCtx should return error without server")
	}
}
