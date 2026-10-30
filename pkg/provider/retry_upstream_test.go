package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// looksLikeUpstreamError — detecting transient upstream failures disguised
// as client-looking 4xx (the OpenCode Zen "Upstream response was not valid
// JSON" class of errors).
// =============================================================================

func TestLooksLikeUpstreamError(t *testing.T) {
	opencodeBody := []byte(`{"error":{"type":"server_error","code":"server_error","message":"Upstream request failed: [server_error] Upstream response was not valid JSON"}}`)

	tests := []struct {
		name       string
		statusCode int
		body       []byte
		want       bool
	}{
		{"opencode disguised 400", 400, opencodeBody, true},
		{"server_error type on 400", 400, []byte(`{"error":{"type":"server_error","message":"boom"}}`), true},
		{"server_error code on 401", 401, []byte(`{"error":{"code":"server_error","message":"boom"}}`), true},
		{"upstream not valid json on 400", 400, []byte(`{"error":{"message":"Upstream response was not valid JSON"}}`), true},
		{"upstream overloaded on 503", 503, []byte(`{"error":{"message":"Upstream is overloaded"}}`), false}, // 5xx handled elsewhere
		{"genuine invalid_request 400", 400, []byte(`{"error":{"type":"invalid_request_error","message":"context_length_exceeded"}}`), false},
		{"genuine content_filter 400", 400, []byte(`{"error":{"type":"invalid_request_error","message":"content filtered"}}`), false},
		{"plain text 400", 400, []byte(`Bad Request`), false},
		{"non-json upstream text 400", 400, []byte(`upstream server_error occurred`), true},
		{"429 never here", 429, opencodeBody, false},
		{"200 never here", 200, opencodeBody, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeUpstreamError(tt.statusCode, tt.body); got != tt.want {
				t.Errorf("looksLikeUpstreamError(%d) = %v, want %v", tt.statusCode, got, tt.want)
			}
		})
	}
}

func TestFormatHTTPError_DisguisedUpstreamBecomesUpstreamError(t *testing.T) {
	body := []byte(`{"error":{"type":"server_error","code":"server_error","message":"Upstream request failed: [server_error] Upstream response was not valid JSON"}}`)

	err := FormatHTTPError(400, body)
	if !IsUpstreamError(err) {
		t.Fatalf("expected UpstreamError, got %T: %v", err, err)
	}
	var ue *UpstreamError
	if !asUpstream(err, &ue) {
		t.Fatal("type assertion failed")
	}
	if ue.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", ue.StatusCode)
	}
}

func TestFormatHTTPError_GenuineClientErrorStaysPlain(t *testing.T) {
	body := []byte(`{"error":{"type":"invalid_request_error","message":"context_length_exceeded"}}`)

	err := FormatHTTPError(400, body)
	if IsUpstreamError(err) {
		t.Errorf("genuine client error must NOT become UpstreamError: %v", err)
	}
}

func TestIsUpstreamError(t *testing.T) {
	if IsUpstreamError(&UpstreamError{StatusCode: 400}) != true {
		t.Error("UpstreamError should be detected")
	}
	if IsUpstreamError(fmt.Errorf("HTTP 400: bad")) != false {
		t.Error("plain error is not UpstreamError")
	}
}

// asUpstream is a tiny helper mirroring errors.As for *UpstreamError.
func asUpstream(err error, target **UpstreamError) bool {
	ue, ok := err.(*UpstreamError)
	if ok {
		*target = ue
	}
	return ok
}

// =============================================================================
// End-to-end: a provider that returns a disguised 400 upstream failure on the
// first attempt must be retried and succeed on the next attempt.
// =============================================================================

func fastRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
		// 4xx disguised upstream is retried via IsUpstreamError, not by status.
		RetryableErrors: []int{500, 502, 503, 504},
	}
}

func TestOpenAIComplete_RetriesDisguisedUpstream400(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":{"type":"server_error","code":"server_error","message":"Upstream request failed: [server_error] Upstream response was not valid JSON"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":"recovered"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{Type: "openai", APIKey: "k", Model: "m", BaseURL: ts.URL})
	if err != nil {
		t.Fatal(err)
	}
	p.SetRetryPolicy(fastRetryPolicy())

	result, err := p.Complete([]Message{UserMsg("hi")}, nil)
	if err != nil {
		t.Fatalf("Complete should retry disguised 400 and succeed, got: %v", err)
	}
	if result.Message.GetText() != "recovered" {
		t.Errorf("text = %q, want recovered", result.Message.GetText())
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("server hit %d times, want 3 (2 disguised failures + 1 success)", got)
	}
}

func TestOpenAIComplete_DoesNotRetryGenuine400(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":{"type":"invalid_request_error","message":"context_length_exceeded"}}`)
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{Type: "openai", APIKey: "k", Model: "m", BaseURL: ts.URL})
	if err != nil {
		t.Fatal(err)
	}
	p.SetRetryPolicy(fastRetryPolicy())

	_, err = p.Complete([]Message{UserMsg("hi")}, nil)
	if err == nil {
		t.Fatal("genuine 400 must return an error")
	}
	if IsUpstreamError(err) {
		t.Errorf("genuine client 400 must NOT be an UpstreamError: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("genuine 400 should not be retried: server hit %d times, want 1", got)
	}
}

func TestOpenAIStream_RetriesDisguisedUpstream400(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 2 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":{"type":"server_error","code":"server_error","message":"Upstream request failed: [server_error] Upstream response was not valid JSON"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer ts.Close()

	p, err := NewOpenAIProvider("test", ProviderConfig{Type: "openai", APIKey: "k", Model: "m", BaseURL: ts.URL})
	if err != nil {
		t.Fatal(err)
	}
	p.SetRetryPolicy(fastRetryPolicy())

	ch, err := p.Stream([]Message{UserMsg("hi")}, nil)
	if err != nil {
		t.Fatalf("Stream should retry disguised 400 and succeed, got: %v", err)
	}
	var text string
	for ev := range ch {
		if ev.Type == EventTextDelta {
			text += ev.Text
		}
	}
	if text != "ok" {
		t.Errorf("streamed text = %q, want ok", text)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("server hit %d times, want 2", got)
	}
}
