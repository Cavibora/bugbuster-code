package provider

import (
	"context"
	"errors"
	"testing"
)

// mockProvider — простой mock для тестирования context_compat
type mockProvider struct {
	completeResult *CompletionResult
	completeErr    error
	streamCh       chan StreamEvent
	streamErr      error
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Complete(_ []Message, _ []ToolDef) (*CompletionResult, error) {
	return m.completeResult, m.completeErr
}

func (m *mockProvider) CompleteWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error) {
	return CompleteWithCtxDefault(m, ctx, messages, tools)
}

func (m *mockProvider) Stream(messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	return m.streamCh, m.streamErr
}

func (m *mockProvider) StreamWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	return StreamWithCtxDefault(m, ctx, messages, tools)
}

// --- CompleteWithCtxDefault ---

func TestCompleteWithCtxDefault_Success(t *testing.T) {
	mock := &mockProvider{
		completeResult: &CompletionResult{
			Message:    AssistantText("hello"),
			StopReason: "end_turn",
		},
	}
	result, err := CompleteWithCtxDefault(mock, context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message.GetText() != "hello" {
		t.Errorf("result = %q, want %q", result.Message.GetText(), "hello")
	}
}

func TestCompleteWithCtxDefault_Error(t *testing.T) {
	mock := &mockProvider{
		completeErr: errors.New("api error"),
	}
	_, err := CompleteWithCtxDefault(mock, context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "api error" {
		t.Errorf("error = %q, want %q", err.Error(), "api error")
	}
}

func TestCompleteWithCtxDefault_CancelledContext(t *testing.T) {
	mock := &mockProvider{
		completeResult: &CompletionResult{Message: AssistantText("should not reach")},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := CompleteWithCtxDefault(mock, ctx, nil, nil)
	if err == nil {
		t.Fatal("expected context cancelled error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

// --- StreamWithCtxDefault ---

func TestStreamWithCtxDefault_Success(t *testing.T) {
	ch := make(chan StreamEvent, 2)
	ch <- StreamEvent{Type: EventTextDelta, Text: "hello"}
	ch <- StreamEvent{Type: EventDone}
	close(ch)

	mock := &mockProvider{streamCh: ch}
	resultCh, err := StreamWithCtxDefault(mock, context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []StreamEvent
	for event := range resultCh {
		events = append(events, event)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Text != "hello" {
		t.Errorf("events[0].Text = %q, want %q", events[0].Text, "hello")
	}
}

func TestStreamWithCtxDefault_Error(t *testing.T) {
	mock := &mockProvider{streamErr: errors.New("stream error")}
	_, err := StreamWithCtxDefault(mock, context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStreamWithCtxDefault_CancelledContext(t *testing.T) {
	ch := make(chan StreamEvent, 10)
	mock := &mockProvider{streamCh: ch}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := StreamWithCtxDefault(mock, ctx, nil, nil)
	if err == nil {
		t.Fatal("expected context cancelled error, got nil")
	}
}

func TestStreamWithCtxDefault_CancelDuringStream(t *testing.T) {
	ch := make(chan StreamEvent, 10)
	mock := &mockProvider{streamCh: ch}

	ctx, cancel := context.WithCancel(context.Background())

	resultCh, err := StreamWithCtxDefault(mock, ctx, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Send an event and then cancel
	ch <- StreamEvent{Type: EventTextDelta, Text: "before cancel"}
	// Close source channel — the wrapper will drain it and exit
	close(ch)

	cancel()

	// Drain the wrapped channel
	var count int
	for range resultCh {
		count++
	}
	if count < 1 {
		t.Errorf("expected at least 1 event, got %d", count)
	}
}
