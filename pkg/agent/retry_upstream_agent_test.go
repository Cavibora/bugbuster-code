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
// on retry behavior, and records every session ID it was given so tests can
// assert that the agent rotates x-opencode-session between retries.
type MockUpstreamFlaky struct {
	failCount int
	calls     int
	text      string

	sessionIDs []string
}

func (m *MockUpstreamFlaky) Name() string { return "mock-upstream-flaky" }
func (m *MockUpstreamFlaky) Model() string { return "mock-upstream-model" }

// SetSessionID makes the mock satisfy the same optional interface the real
// OpenAI/OpenCode provider does, so rotateProviderSession() drives it.
func (m *MockUpstreamFlaky) SetSessionID(id string) { m.sessionIDs = append(m.sessionIDs, id) }

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
	loop.SetSessionIDForProvider("sess_initial_00000000")
	mock.sessionIDs = nil // record only rotations, not the initial assignment
	// tiny delay so the retry loop is fast; budget covers the 3 injected failures
	loop.SetRateLimitConfig(50, 10*time.Millisecond)
	loop.SetUpstreamMaxRetries(5)

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
	// Each retry must rotate the provider session — this is the automatic
	// equivalent of the user restarting the client.
	if len(mock.sessionIDs) < 3 {
		t.Fatalf("expected at least 3 session rotations, got %d", len(mock.sessionIDs))
	}
	seen := map[string]bool{}
	for _, id := range mock.sessionIDs {
		if id == "sess_initial_00000000" {
			t.Errorf("rotation reused the original session ID: %s", id)
		}
		if seen[id] {
			t.Errorf("session ID rotated to a duplicate: %s", id)
		}
		seen[id] = true
	}
}

func TestStreamRetryRequest_UpstreamExhaustsBudget(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	mock := &MockUpstreamFlaky{failCount: 1000, text: "never"}
	loop := NewAgentLoop(mock)
	loop.SetSessionIDForProvider("sess_initial_00000000")
	mock.sessionIDs = nil // record only rotations, not the initial assignment
	loop.SetRateLimitConfig(50, 1*time.Millisecond)
	loop.SetUpstreamMaxRetries(5) // small budget for a fast test

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
	// The budget must stay small — this is what previously looked like a hang.
	if mock.calls > 10 {
		t.Errorf("upstream retry budget too generous: %d calls", mock.calls)
	}
	// Every attempt rotated to a fresh session, and the final state is a
	// rotated ID (not the poisoned original) so the next iteration is clean.
	if len(mock.sessionIDs) != 6 {
		t.Errorf("expected 6 session rotations (one per attempt), got %d", len(mock.sessionIDs))
	}
	if loop.providerSessionID == "sess_initial_00000000" {
		t.Error("session was not rotated after exhausting the upstream budget")
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
