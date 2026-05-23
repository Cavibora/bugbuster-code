package provider

import "context"

// CompleteWithCtxDefault — default CompleteWithCtx implementation,
// which delegates to Complete without context.
// Providers can override this method for cancellation support.
func CompleteWithCtxDefault(p Provider, ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error) {
	// Check context before request
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Execute regular request
	result, err := p.Complete(messages, tools)
	if err != nil {
		return nil, err
	}

	// Check context after request
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return result, nil
}

// StreamWithCtxDefault — default StreamWithCtx implementation,
// which delegates to Stream without context.
func StreamWithCtxDefault(p Provider, ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	// Check context before request
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Execute regular stream
	ch, err := p.Stream(messages, tools)
	if err != nil {
		return nil, err
	}

	// Wrap channel for context checking
	wrappedCh := make(chan StreamEvent, 100)
	go func() {
		defer close(wrappedCh)
		for event := range ch {
			if ctx.Err() != nil {
				wrappedCh <- StreamEvent{Type: "error", Error: ctx.Err()}
				return
			}
			wrappedCh <- event
		}
	}()

	return wrappedCh, nil
}
