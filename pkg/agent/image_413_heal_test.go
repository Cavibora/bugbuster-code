package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

// largePNGBase64 builds an oversized, hard-to-compress PNG and returns it as
// base64. Used to seed a history image that a size-limited gateway would reject.
func largePNGBase64(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	seed := uint32(987654321)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			seed = seed*1664525 + 1013904223
			img.SetRGBA(x, y, color.RGBA{uint8(seed >> 16), uint8(seed >> 8), uint8(seed), 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// MockSizeLimited behaves like a gateway with a request-body cap: it returns
// ImageTooLargeError (413) whenever the total base64 image payload in the
// request exceeds its byte budget, and a normal text stream otherwise. This
// lets us assert the agent self-heals by progressively downscaling (and only
// as a last resort stripping) oversized images.
type MockSizeLimited struct {
	calls        int
	budget       int // max total base64 image bytes accepted
	text         string
	acceptedWith int // image bytes present at the accepted request
	everAccepted bool
}

func (m *MockSizeLimited) Name() string  { return "mock-size-limited" }
func (m *MockSizeLimited) Model() string { return "mock" }
func (m *MockSizeLimited) Complete(a []provider.Message, b []provider.ToolDef) (*provider.CompletionResult, error) {
	return &provider.CompletionResult{Message: provider.AssistantText(m.text), StopReason: "end_turn"}, nil
}
func (m *MockSizeLimited) CompleteWithCtx(ctx context.Context, a []provider.Message, b []provider.ToolDef) (*provider.CompletionResult, error) {
	return m.Complete(a, b)
}
func (m *MockSizeLimited) Stream(a []provider.Message, b []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.StreamWithCtx(context.Background(), a, b)
}
func (m *MockSizeLimited) StreamWithCtx(ctx context.Context, msgs []provider.Message, _ []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	m.calls++
	total := 0
	for _, msg := range msgs {
		for _, b := range msg.Content {
			if b.Type == "image" {
				total += len(b.ImageSource)
			}
		}
	}
	if total > m.budget {
		return nil, &provider.ImageTooLargeError{
			StatusCode: 413,
			Body:       `{"error":{"message":"request body is too large","type":"invalid_request_error","code":"payload_too_large"}}`,
		}
	}
	m.everAccepted = true
	m.acceptedWith = total
	ch := make(chan provider.StreamEvent, 10)
	go func() {
		ch <- provider.StreamEvent{Type: provider.EventTextDelta, Text: m.text}
		ch <- provider.StreamEvent{Type: provider.EventDone}
		close(ch)
	}()
	return ch, nil
}

// When a history image is oversized but still shrinkable, the agent must
// downscale it (keeping the picture) and succeed — NOT strip it outright.
func TestStreamRetryRequest_SelfHeals413ByDownscaling(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}

	// A gateway that accepts up to ~600 KB of image base64. A 2400x1800 noise
	// PNG is well over that, so the first request is rejected; after the agent
	// downscales it fits and the image survives.
	mock := &MockSizeLimited{budget: 600 * 1024, text: "answered with a downscaled image"}
	loop := NewAgentLoop(mock)
	loop.SetRateLimitConfig(50, 1*time.Millisecond)

	big := largePNGBase64(t, 2400, 1800)
	loop.Context.Add(provider.UserMsg("look at this screenshot"))
	loop.Context.Add(provider.UserImageTextMsg("[Screenshot]", big, "png"))

	eventCh := make(chan provider.StreamEvent, 256)
	stream, err := loop.streamRetryRequest(context.Background(), eventCh)
	if err != nil {
		t.Fatalf("expected self-heal to recover the request, got error: %v", err)
	}
	got := ""
	for ev := range stream {
		if ev.Type == provider.EventTextDelta {
			got += ev.Text
		}
	}
	if got != mock.text {
		t.Errorf("expected healed response %q, got %q", mock.text, got)
	}

	// The image must have been downscaled, not dropped.
	if !loop.Context.HasImages() {
		t.Error("image should be kept (downscaled), not stripped, when shrinking is enough")
	}
	// And the accepted payload must fit the budget.
	if !mock.everAccepted || mock.acceptedWith > mock.budget {
		t.Errorf("expected an accepted request within budget, acceptedWith=%d budget=%d", mock.acceptedWith, mock.budget)
	}
	// The original was rejected at least once before healing.
	if mock.calls < 2 {
		t.Errorf("expected a rejected attempt then a healed one, calls=%d", mock.calls)
	}
}

// When even the smallest downscale can't fit (e.g. an undecodable format), the
// agent must strip images so the conversation stays usable, not spin forever.
func TestStreamRetryRequest_SelfHeals413ByStrippingUndecodable(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}

	// Budget smaller than a single fake image, and the image is not decodable,
	// so downscaling can't help — the agent must fall back to stripping.
	mock := &MockSizeLimited{budget: 10, text: "answered without the image"}
	loop := NewAgentLoop(mock)
	loop.SetRateLimitConfig(50, 1*time.Millisecond)

	loop.Context.Add(provider.UserMsg("hello"))
	loop.Context.Add(provider.UserImageTextMsg("[Screenshot]", "AAAA-not-decodable", "png"))

	eventCh := make(chan provider.StreamEvent, 256)
	stream, err := loop.streamRetryRequest(context.Background(), eventCh)
	if err != nil {
		t.Fatalf("expected strip self-heal to recover, got error: %v", err)
	}
	for range stream {
	}

	if loop.Context.HasImages() {
		t.Error("undecodable oversized image must be stripped after downscaling fails")
	}
	if !mock.everAccepted {
		t.Error("image-free resend must have been accepted")
	}
}

// A gateway that keeps rejecting even an image-free payload must not spin the
// heal loop forever — the agent gives up and returns the error.
func TestStreamRetryRequest_413DoesNotLoopForever(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}

	// budget 0 + no images ever present → every request rejected, nothing to
	// shrink or strip.
	mock := &alwaysTooLarge{}
	loop := NewAgentLoop(mock)
	loop.SetRateLimitConfig(50, 1*time.Millisecond)
	loop.Context.Add(provider.UserMsg("no images here at all"))

	eventCh := make(chan provider.StreamEvent, 256)
	_, err := loop.streamRetryRequest(context.Background(), eventCh)
	if err == nil {
		t.Fatal("expected an error when healing is impossible")
	}
	if !provider.IsImageTooLargeError(err) {
		t.Errorf("expected ImageTooLargeError, got %T: %v", err, err)
	}
	if mock.calls > 4 {
		t.Errorf("413 heal loop must not spin; calls=%d", mock.calls)
	}
}

type alwaysTooLarge struct{ calls int }

func (m *alwaysTooLarge) Name() string  { return "always-413" }
func (m *alwaysTooLarge) Model() string { return "m" }
func (m *alwaysTooLarge) Complete(a []provider.Message, b []provider.ToolDef) (*provider.CompletionResult, error) {
	return nil, &provider.ImageTooLargeError{StatusCode: 413, Body: "payload_too_large"}
}
func (m *alwaysTooLarge) CompleteWithCtx(ctx context.Context, a []provider.Message, b []provider.ToolDef) (*provider.CompletionResult, error) {
	return m.Complete(a, b)
}
func (m *alwaysTooLarge) Stream(a []provider.Message, b []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.StreamWithCtx(context.Background(), a, b)
}
func (m *alwaysTooLarge) StreamWithCtx(ctx context.Context, a []provider.Message, b []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	m.calls++
	return nil, &provider.ImageTooLargeError{StatusCode: 413, Body: "payload_too_large"}
}
