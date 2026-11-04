package serve

import (
	"context"
	"sync"
	"testing"
	"time"

	"bugbuster-code/pkg/agent"
	"bugbuster-code/pkg/provider"
)

// mockProvider drives the agent loop without any network. It replays a scripted
// list of stream events per call and can be told to stall, so tests exercise
// cancellation and slow-client paths.
type mockProvider struct {
	mu sync.Mutex

	// events is returned by every StreamWithCtx call.
	events []provider.StreamEvent
	// stall delays each event, used to test Cancel.
	stall time.Duration
	// calls counts invocations.
	calls int
	// lastMessages records the prompt the loop sent.
	lastMessages []provider.Message

	// onStream, if set, replaces the scripted replay.
	onStream func(ctx context.Context, call int) []provider.StreamEvent
}

func (m *mockProvider) Name() string  { return "mock" }
func (m *mockProvider) Model() string { return "mock-model" }

func (m *mockProvider) Complete(messages []provider.Message, tools []provider.ToolDef) (*provider.CompletionResult, error) {
	return &provider.CompletionResult{
		Message:    provider.AssistantText("mock completion"),
		StopReason: "end_turn",
	}, nil
}

func (m *mockProvider) CompleteWithCtx(ctx context.Context, messages []provider.Message, tools []provider.ToolDef) (*provider.CompletionResult, error) {
	return m.Complete(messages, tools)
}

func (m *mockProvider) Stream(messages []provider.Message, tools []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.StreamWithCtx(context.Background(), messages, tools)
}

func (m *mockProvider) StreamWithCtx(ctx context.Context, messages []provider.Message, tools []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	events := m.events
	stall := m.stall
	m.lastMessages = messages
	onStream := m.onStream
	m.mu.Unlock()

	if onStream != nil {
		events = onStream(ctx, call)
	}

	ch := make(chan provider.StreamEvent, len(events)+2)
	go func() {
		defer close(ch)
		for _, ev := range events {
			if stall > 0 {
				select {
				case <-time.After(stall):
				case <-ctx.Done():
					return
				}
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// textStream is the common case: a couple of deltas then a clean finish.
func textStream(text string) []provider.StreamEvent {
	return []provider.StreamEvent{
		{Type: provider.EventIterationStart, Iteration: 1},
		{Type: provider.EventTextDelta, Text: text},
		{Type: provider.EventDone, StopReason: "end_turn"},
	}
}

// newMockLoop builds a minimal agent loop over a mock provider. It deliberately
// does not use cmd/bugbuster's createAgentLoop — serve must work with any loop.
func newMockLoop(p provider.Provider) *agent.AgentLoop {
	loop := agent.NewAgentLoop(p)
	loop.SetNonInteractive(true)
	return loop
}

// testServer returns a server whose factory always builds a loop on top of the
// given provider behaviour.
func testServer(t *testing.T, factory LoopFactory, cfg func(*Config)) *Server {
	t.Helper()
	c := Config{
		Addr:               "127.0.0.1:0",
		Token:              "test-token",
		MaxSessions:        4,
		MaxToolResultBytes: 256,
		MaxRingEvents:      64,
		HeartbeatInterval:  100 * time.Millisecond,
		Factory:            factory,
	}
	if cfg != nil {
		cfg(&c)
	}
	srv, err := New(c)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv
}

// collect drains a session's stream until timeout, returning the events seen.
func collect(t *testing.T, ssn *Session, timeout time.Duration) []Event {
	t.Helper()
	events, backlog, release := ssn.Subscribe(0)
	defer release()

	out := append([]Event(nil), backlog...)
	for _, ev := range out {
		if isTerminalEvent(ev) {
			return out
		}
	}

	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return out
			}
			out = append(out, ev)
			if ev.Type == provider.EventDone || ev.Type == provider.EventError {
				return out
			}
		case <-deadline:
			return out
		}
	}
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out after %s: %s", timeout, msg)
}

func mustTask(t *testing.T, ssn *Session, task string) {
	t.Helper()
	if err := ssn.Start(context.Background(), task); err != nil {
		t.Fatalf("Start(%q): %v", task, err)
	}
}
