package agent

import (
	"context"
	"testing"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

// imageUnsupportedBody is the exact gufo text-only payload that rejected the
// screenshot-laden history with a deterministic HTTP 400.
const imageUnsupportedBody = `{"error":{"message":"model does not support image input","type":"invalid_request_error","code":"invalid_messages"}}`

// MockImageRejecting behaves like a text-only OpenAI-compatible endpoint: it
// returns ImageUnsupportedError for any request whose messages still contain
// image blocks, and a normal text stream once the history is image-free. This
// lets us assert the agent self-heals by stripping images and resending,
// instead of killing the session.
type MockImageRejecting struct {
	calls   int
	text    string
	striped bool // becomes true once a request arrives with no images
}

func (m *MockImageRejecting) Name() string  { return "mock-image-rejecting" }
func (m *MockImageRejecting) Model() string { return "mock-text-only" }

func (m *MockImageRejecting) Complete(messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	return &provider.CompletionResult{Message: provider.AssistantText(m.text), StopReason: "end_turn"}, nil
}

func (m *MockImageRejecting) CompleteWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	return m.Complete(messages, toolDefs)
}

func (m *MockImageRejecting) Stream(messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.StreamWithCtx(context.Background(), messages, toolDefs)
}

func (m *MockImageRejecting) StreamWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	m.calls++

	hasImage := false
	for _, msg := range messages {
		for _, b := range msg.Content {
			if b.Type == "image" {
				hasImage = true
			}
		}
	}

	if hasImage {
		return nil, &provider.ImageUnsupportedError{StatusCode: 400, Body: imageUnsupportedBody}
	}

	// Image-free request — the endpoint is happy.
	m.striped = true
	ch := make(chan provider.StreamEvent, 10)
	go func() {
		ch <- provider.StreamEvent{Type: provider.EventTextDelta, Text: m.text}
		ch <- provider.StreamEvent{Type: provider.EventDone}
		close(ch)
	}()
	return ch, nil
}

func TestStreamRetryRequest_SelfHealsByStrippingImages(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}

	mock := &MockImageRejecting{text: "answered without the image"}
	loop := NewAgentLoop(mock)
	loop.SetRateLimitConfig(50, 1*time.Millisecond)

	// Seed a history that contains an image, exactly like a session where the
	// user (or a screenshot tool) attached a picture.
	loop.Context.Add(provider.UserMsg("hello"))
	loop.Context.Add(provider.UserImageTextMsg("[Screenshot from tool screenshot]", "AAAA", "png"))
	loop.Context.Add(provider.AssistantText("I see something"))
	loop.Context.Add(provider.UserMsg("and now?"))

	if !loop.Context.HasImages() {
		t.Fatal("test precondition: history must contain an image")
	}

	eventCh := make(chan provider.StreamEvent, 256)
	stream, err := loop.streamRetryRequest(context.Background(), eventCh)
	if err != nil {
		t.Fatalf("expected self-heal to recover the request, got error: %v", err)
	}
	if stream == nil {
		t.Fatal("expected non-nil stream after healing")
	}

	// Drain the stream to confirm it is a real response.
	got := ""
	for ev := range stream {
		if ev.Type == provider.EventTextDelta {
			got += ev.Text
		}
	}
	if got != mock.text {
		t.Errorf("expected healed response %q, got %q", mock.text, got)
	}

	// The agent must have stripped the images from history (so the session
	// stays usable for every later message) and resent without them.
	if loop.Context.HasImages() {
		t.Error("images must be removed from history after self-heal")
	}
	if !mock.striped {
		t.Error("second attempt must have been sent image-free")
	}
	if mock.calls != 2 {
		t.Errorf("expected 2 calls (rejected-with-image + healed), got %d", mock.calls)
	}

	// A heal notification event should have surfaced to the UI.
	sawHeal := false
	for {
		select {
		case ev := <-eventCh:
			if ev.Type == provider.EventRateLimit && containsStr(ev.Text, "does not accept images") {
				sawHeal = true
			}
		default:
			if !sawHeal {
				t.Error("expected a 'stripped images' notification event")
			}
			return
		}
	}
}

// If the endpoint keeps rejecting even an image-free payload, the heal loop
// must not spin forever — it gives up and returns the error.
func TestStreamRetryRequest_ImageHealDoesNotLoopForever(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}

	// Always rejects, even with no images present.
	mock := &alwaysImageReject{}
	loop := NewAgentLoop(mock)
	loop.SetRateLimitConfig(50, 1*time.Millisecond)
	loop.Context.Add(provider.UserMsg("no images here at all"))

	eventCh := make(chan provider.StreamEvent, 256)
	_, err := loop.streamRetryRequest(context.Background(), eventCh)
	if err == nil {
		t.Fatal("expected an error to be returned when healing is impossible")
	}
	if !provider.IsImageUnsupportedError(err) {
		t.Errorf("expected ImageUnsupportedError, got %T: %v", err, err)
	}
	// With no images to strip it must break immediately (1 call), not loop.
	if mock.calls > 2 {
		t.Errorf("heal loop must not spin; calls=%d", mock.calls)
	}
}

type alwaysImageReject struct{ calls int }

func (m *alwaysImageReject) Name() string  { return "always-reject" }
func (m *alwaysImageReject) Model() string { return "m" }
func (m *alwaysImageReject) Complete(a []provider.Message, b []provider.ToolDef) (*provider.CompletionResult, error) {
	return nil, &provider.ImageUnsupportedError{StatusCode: 400, Body: imageUnsupportedBody}
}
func (m *alwaysImageReject) CompleteWithCtx(ctx context.Context, a []provider.Message, b []provider.ToolDef) (*provider.CompletionResult, error) {
	return m.Complete(a, b)
}
func (m *alwaysImageReject) Stream(a []provider.Message, b []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.StreamWithCtx(context.Background(), a, b)
}
func (m *alwaysImageReject) StreamWithCtx(ctx context.Context, a []provider.Message, b []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	m.calls++
	return nil, &provider.ImageUnsupportedError{StatusCode: 400, Body: imageUnsupportedBody}
}
