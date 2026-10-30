package agent

import (
	"context"
	"testing"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

// BlockingProvider emulates a stalled upstream: CompleteWithCtx blocks until the
// caller's context is cancelled (exactly like a real provider whose HTTP request
// hangs). If the agent loop did not pass a deadline, Run() would hang forever.
type BlockingProvider struct{}

func (m *BlockingProvider) Name() string  { return "blocking" }
func (m *BlockingProvider) Model() string { return "blocking-model" }

func (m *BlockingProvider) Complete(messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	select {} // hang forever if no ctx deadline is honored
}

func (m *BlockingProvider) CompleteWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (*provider.CompletionResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *BlockingProvider) Stream(messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	select {}
}

func (m *BlockingProvider) StreamWithCtx(ctx context.Context, messages []provider.Message, toolDefs []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestRunLoop_RequestTimeoutBoundsStall verifies Fix 2: the non-stream runLoop
// wraps each provider call in a context bounded by effectiveRequestTimeout, so
// a stalled upstream cannot hang the loop forever.
func TestRunLoop_RequestTimeoutBoundsStall(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("i18n.Init failed: %v", err)
	}
	loop := NewAgentLoop(&BlockingProvider{})
	loop.SetRequestTimeout(300 * time.Millisecond)

	done := make(chan error, 1)
	go func() {
		_, err := loop.Run("stalled")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		t.Logf("runLoop returned bounded error as expected: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("runLoop did not honor request timeout — would hang forever in prod")
	}
}
