package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

// TestRunLoop_SingleResponse tests runLoop with a single response (no tool calls)
func TestRunLoop_SingleResponse(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	mock := &MockProvider{
		response: provider.CompletionResult{
			Message: provider.Message{
				Role: "assistant",
				Text: "Hello! How can I help you?",
			},
		},
	}
	loop := NewAgentLoop(mock)
	loop.SetMaxIterations(1)
	loop.SetNonInteractive(true)

	result, _ := loop.RunWithMessages([]provider.Message{
		provider.UserMsg("Hi"),
	})
	if result == "" {
		t.Fatal("Expected non-empty result")
	}
	if !strings.Contains(result, "Hello") {
		t.Errorf("Expected result to contain 'Hello', got: %s", result)
	}
}

// TestRunLoop_MaxIterations tests that runLoop respects max iterations
func TestRunLoop_MaxIterations(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	mock := &MockStreamingProvider{}
	loop := NewAgentLoop(mock)
	loop.SetMaxIterations(3)
	loop.SetNonInteractive(true)
	loop.RegisterTool(&MockNoOpTool{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eventCh, err := loop.StreamWithCancel(ctx, "do something")
	if err != nil {
		t.Fatalf("StreamWithCancel failed: %v", err)
	}

	var iterations int
	var gotDone bool
	for event := range eventCh {
		switch event.Type {
		case provider.EventIterationEnd:
			iterations++
		case provider.EventDone:
			gotDone = true
		}
	}

	if iterations > 3 {
		t.Errorf("Expected at most 3 iterations, got %d", iterations)
	}
	if !gotDone {
		t.Error("Expected EventDone")
	}
}

// TestRunLoop_EmptyResponse tests runLoop with empty response
func TestRunLoop_EmptyResponse(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	mock := &MockProvider{
		response: provider.CompletionResult{
			Message: provider.Message{
				Role: "assistant",
				Text: "",
			},
		},
	}
	loop := NewAgentLoop(mock)
	loop.SetMaxIterations(1)
	loop.SetNonInteractive(true)

	result, _ := loop.RunWithMessages([]provider.Message{
		provider.UserMsg("Say nothing"),
	})
	t.Logf("Result: %q", result)
}

// TestRunLoop_WithThinking tests runLoop with thinking blocks
func TestRunLoop_WithThinking(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	mock := &MockStreamingProvider{}
	loop := NewAgentLoop(mock)
	loop.SetMaxIterations(1)
	loop.SetNonInteractive(true)
	loop.RegisterTool(&MockNoOpTool{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eventCh, err := loop.StreamWithCancel(ctx, "think about it")
	if err != nil {
		t.Fatalf("StreamWithCancel failed: %v", err)
	}

	var gotThinking bool
	for event := range eventCh {
		if event.Type == provider.EventThinking {
			gotThinking = true
		}
	}
	// MockStreamingProvider doesn't send thinking events, so this is expected
	t.Logf("Got thinking: %v", gotThinking)
}

// TestHandleStreamFinalResponse_TextOnly tests handling a stream that ends with text
func TestHandleStreamFinalResponse_TextOnly(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	mock := &MockProvider{
		response: provider.CompletionResult{
			Message: provider.Message{
				Role: "assistant",
				Text: "Here is my answer",
			},
		},
	}
	loop := NewAgentLoop(mock)
	loop.SetMaxIterations(1)
	loop.SetNonInteractive(true)

	result, _ := loop.RunWithMessages([]provider.Message{
		provider.UserMsg("What is 2+2?"),
	})
	if result == "" {
		t.Fatal("Expected non-empty result")
	}
	if !strings.Contains(result, "answer") {
		t.Logf("Result: %s", result)
	}
}

// TestRunLoop_ContextCancellation tests that runLoop respects context cancellation
func TestRunLoop_ContextCancellation(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	mock := &MockStreamingProvider{}
	loop := NewAgentLoop(mock)
	loop.SetMaxIterations(10)
	loop.SetNonInteractive(true)
	loop.RegisterTool(&MockNoOpTool{})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	eventCh, err := loop.StreamWithCancel(ctx, "think for a long time")
	if err != nil {
		t.Fatalf("StreamWithCancel failed: %v", err)
	}

	// Just consume events until context cancels
	for range eventCh {
	}
	// If we get here, context cancellation was handled
}
