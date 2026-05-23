package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCaviboraProvider_Name(t *testing.T) {
	p, err := NewCaviboraProvider("test-ng", ProviderConfig{
		Type:   "cavibora",
		APIKey: "test-key",
		Model:  "cavibora-v1",
	})
	if err != nil {
		t.Fatalf("NewCaviboraProvider: %v", err)
	}
	if p.Name() != "test-ng" {
		t.Errorf("Name() = %q, want %q", p.Name(), "test-ng")
	}
}

func TestCaviboraProvider_DefaultBaseURL(t *testing.T) {
	p, err := NewCaviboraProvider("test-ng", ProviderConfig{
		Type:   "cavibora",
		APIKey: "test-key",
		Model:  "cavibora-v1",
	})
	if err != nil {
		t.Fatalf("NewCaviboraProvider: %v", err)
	}
	if p.baseURL != "https://api.cavibora.com" {
		t.Errorf("baseURL = %q, want %q", p.baseURL, "https://api.cavibora.com")
	}
}

func TestCaviboraProvider_CustomBaseURL(t *testing.T) {
	p, err := NewCaviboraProvider("test-ng", ProviderConfig{
		Type:    "cavibora",
		BaseURL: "https://custom.api.com/",
		APIKey:  "test-key",
		Model:   "cavibora-v1",
	})
	if err != nil {
		t.Fatalf("NewCaviboraProvider: %v", err)
	}
	if p.baseURL != "https://custom.api.com" {
		t.Errorf("baseURL = %q, want %q", p.baseURL, "https://custom.api.com")
	}
}

func TestCaviboraProvider_DefaultModel(t *testing.T) {
	p, err := NewCaviboraProvider("test-ng", ProviderConfig{
		Type:   "cavibora",
		APIKey: "test-key",
	})
	if err != nil {
		t.Fatalf("NewCaviboraProvider: %v", err)
	}
	if p.model != "cavibora-v1" {
		t.Errorf("model = %q, want %q", p.model, "cavibora-v1")
	}
}

func TestCaviboraProvider_Teach_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/teach" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing Authorization header")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	p, err := NewCaviboraProvider("test-ng", ProviderConfig{
		Type:    "cavibora",
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "cavibora-v1",
	})
	if err != nil {
		t.Fatalf("NewCaviboraProvider: %v", err)
	}

	err = p.Teach("input text", "output text")
	if err != nil {
		t.Errorf("Teach() error: %v", err)
	}
}

func TestCaviboraProvider_Teach_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer server.Close()

	p, err := NewCaviboraProvider("test-ng", ProviderConfig{
		Type:    "cavibora",
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "cavibora-v1",
	})
	if err != nil {
		t.Fatalf("NewCaviboraProvider: %v", err)
	}

	err = p.Teach("input text", "output text")
	if err == nil {
		t.Error("Teach() should return error for HTTP 500")
	}
}

func TestCaviboraProvider_Teach_NoAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("Authorization header should be empty when no API key")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p, err := NewCaviboraProvider("test-ng", ProviderConfig{
		Type:    "cavibora",
		BaseURL: server.URL,
		APIKey:  "",
		Model:   "cavibora-v1",
	})
	if err != nil {
		t.Fatalf("NewCaviboraProvider: %v", err)
	}

	err = p.Teach("input", "output")
	if err != nil {
		t.Errorf("Teach() error: %v", err)
	}
}

func TestCaviboraProvider_CompleteWithCtx(t *testing.T) {
	p, err := NewCaviboraProvider("test-ng", ProviderConfig{
		Type:   "cavibora",
		APIKey: "test-key",
		Model:  "cavibora-v1",
	})
	if err != nil {
		t.Fatalf("NewCaviboraProvider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// CompleteWithCtx delegates to delegate which will fail without server
	_, err = p.CompleteWithCtx(ctx, []Message{UserMsg("hello")}, nil)
	if err == nil {
		t.Error("CompleteWithCtx should return error without server")
	}
}

func TestCaviboraProvider_StreamWithCtx(t *testing.T) {
	p, err := NewCaviboraProvider("test-ng", ProviderConfig{
		Type:   "cavibora",
		APIKey: "test-key",
		Model:  "cavibora-v1",
	})
	if err != nil {
		t.Fatalf("NewCaviboraProvider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// StreamWithCtx delegates to delegate which will fail without server
	_, err = p.StreamWithCtx(ctx, []Message{UserMsg("hello")}, nil)
	if err == nil {
		t.Error("StreamWithCtx should return error without server")
	}
}
