package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

// upstreamBody is the exact OpenCode Zen payload that disguised a transient
// gateway failure as an HTTP 400.
const upstreamBody = `{"error":{"type":"server_error","code":"server_error","message":"Upstream request failed: [server_error] Upstream response was not valid JSON"}}`

// MockUpstreamFlaky returns an UpstreamError for the first `failCount` stream
// calls, then a normal text stream. It counts every call so tests can assert
// on retry behavior.
type MockUpstreamFlaky struct {
	failCount int
	calls     int
	text      string
}

func (m *MockUpstreamFlaky) Name() string { return "mock-upstream-flaky" }
func (m *MockUpstreamFlaky) Model() string { return "mock-upstream-model" }

func (m *MockUpstreamFlaky) Complete(messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	return &provider.CompletionResult{Message: provider.AssistantText(m.text), StopReason: "end_turn"}, nil
}

func (m *MockUpstreamFlaky) CompleteWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	return m.Complete(messages, toolDefs)
}

func (m *MockUpstreamFlaky) Stream(messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.StreamWithCtx(context.Background(), messages, toolDefs)
}

func (m *MockUpstreamFlaky) StreamWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	m.calls++
	if m.failCount > 0 {
		m.failCount--
		return nil, &provider.UpstreamError{StatusCode: 400, Body: upstreamBody}
	}
	ch := make(chan provider.StreamEvent, 10)
	go func() {
		ch <- provider.StreamEvent{Type: provider.EventTextDelta, Text: m.text}
		ch <- provider.StreamEvent{Type: provider.EventDone}
		close(ch)
	}()
	return ch, nil
}

func TestStreamRetryRequest_RetriesUpstreamThenSucceeds(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockUpstreamFlaky{failCount: 3, text: "recovered"}
	loop := NewAgentLoop(mock)
	// tiny delay so the retry loop is fast; generous budget so 3 failures recover
	loop.SetRateLimitConfig(50, 10*time.Millisecond)

	eventCh := make(chan provider.StreamEvent, 256)
	stream, err := loop.streamRetryRequest(context.Background(), eventCh)
	if err != nil {
		t.Fatalf("expected upstream to recover, got error: %v", err)
	}
	if stream == nil {
		t.Fatal("expected non-nil stream")
	}
	if mock.calls != 4 {
		t.Errorf("expected 4 calls (3 failures + 1 success), got %d", mock.calls)
	}
}

func TestStreamRetryRequest_UpstreamExhaustsBudget(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockUpstreamFlaky{failCount: 1000, text: "never"}
	loop := NewAgentLoop(mock)
	loop.SetRateLimitConfig(5, 1*time.Millisecond) // small budget for a fast test

	eventCh := make(chan provider.StreamEvent, 256)
	_, err := loop.streamRetryRequest(context.Background(), eventCh)
	if err == nil {
		t.Fatal("expected error after exhausting upstream budget")
	}
	if !provider.IsUpstreamError(err) {
		t.Errorf("expected UpstreamError to be returned, got: %v", err)
	}
	// 5 retries + initial attempt = 6 calls (no fallback provider configured)
	if mock.calls != 6 {
		t.Errorf("expected 6 calls before giving up, got %d", mock.calls)
	}
}

func TestStreamRetryRequest_Genuine400NotRetried(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	// A genuine client error must NOT be retried patiently.
	mock := &MockErrorProvider{err: errors.New("HTTP 400: {\"error\":{\"type\":\"invalid_request_error\",\"message\":\"context length exceeded\"}}")}
	loop := NewAgentLoop(mock)
	loop.SetRateLimitConfig(50, 1*time.Millisecond)

	eventCh := make(chan provider.StreamEvent, 256)
	_, err := loop.streamRetryRequest(context.Background(), eventCh)
	if err == nil {
		t.Fatal("expected genuine 400 to fail")
	}
	if provider.IsUpstreamError(err) {
		t.Errorf("genuine invalid_request must not be classified as upstream error")
	}
}
