package serve

import (
	"context"
	"strings"
	"testing"
	"time"

	"bugbuster-code/pkg/provider"
)

func TestSessionStartEmitsTextAndDone(t *testing.T) {
	p := &mockProvider{events: textStream("hello from agent")}
	ssn := newSession("s1", "test", newMockLoop(p), sessionOptions{maxRingEvents: 64, maxToolResultBytes: 256})

	mustTask(t, ssn, "say hello")
	if ssn.Status() != StatusRunning {
		t.Fatalf("expected running, got %s", ssn.Status())
	}

	events := collect(t, ssn, 3*time.Second)
	if len(events) == 0 {
		t.Fatal("no events received")
	}

	var text strings.Builder
	gotDone := false
	for i, ev := range events {
		if ev.Seq != i+1 {
			t.Fatalf("seq must be contiguous: got %d at index %d", ev.Seq, i)
		}
		if ev.Type == provider.EventTextDelta {
			text.WriteString(ev.Text)
		}
		if ev.Type == provider.EventDone {
			gotDone = true
		}
	}
	if !gotDone {
		t.Fatalf("expected a done event, got %v", eventTypes(events))
	}
	if !strings.Contains(text.String(), "hello from agent") {
		t.Errorf("expected agent text in stream, got %q", text.String())
	}

	waitFor(t, 2*time.Second, func() bool { return ssn.Status() == StatusIdle }, "session should return to idle")
}

func TestSessionRejectsConcurrentTasks(t *testing.T) {
	p := &mockProvider{
		stall:  20 * time.Millisecond,
		events: textStream("slow"),
	}
	ssn := newSession("s2", "busy", newMockLoop(p), sessionOptions{})

	mustTask(t, ssn, "first")
	err := ssn.Start(context.Background(), "second")
	if err != ErrBusy {
		t.Fatalf("expected ErrBusy, got %v", err)
	}

	// A closed session refuses work outright.
	ssn.Cancel()
	ssn.Wait(2 * time.Second)
	ssn.Close()
	if err := ssn.Start(context.Background(), "third"); err != ErrClosed {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestSessionCancelStopsRun(t *testing.T) {
	p := &mockProvider{
		stall:  30 * time.Millisecond,
		events: textStream("this should never finish"),
	}
	ssn := newSession("s3", "cancel", newMockLoop(p), sessionOptions{})

	mustTask(t, ssn, "long task")
	if err := ssn.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if !ssn.Wait(3 * time.Second) {
		t.Fatal("run did not exit after cancel")
	}
	if ssn.Status() != StatusIdle {
		t.Errorf("expected idle after cancel, got %s", ssn.Status())
	}
}

func TestSessionEmptyTask(t *testing.T) {
	ssn := newSession("s4", "empty", newMockLoop(&mockProvider{}), sessionOptions{})
	if err := ssn.Start(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty task")
	}
}

func TestSubscribeReplaysFromSeq(t *testing.T) {
	p := &mockProvider{events: textStream("one two three")}
	ssn := newSession("s5", "replay", newMockLoop(p), sessionOptions{maxRingEvents: 64})

	mustTask(t, ssn, "go")
	waitFor(t, 2*time.Second, func() bool { return ssn.Status() == StatusIdle }, "run to finish")

	all := ssn.Events(0)
	if len(all) < 3 {
		t.Fatalf("expected at least 3 buffered events, got %d", len(all))
	}

	// Resuming after the first event must replay exactly the tail.
	rest := ssn.Events(all[0].Seq)
	if len(rest) == 0 || rest[0].Seq != all[1].Seq {
		t.Fatalf("replay from %d returned wrong tail: %v", all[0].Seq, eventTypes(rest))
	}

	// Subscribe with a cursor hands back the backlog synchronously.
	_, backlog, release := ssn.Subscribe(all[0].Seq)
	defer release()
	if len(backlog) != len(rest) {
		t.Fatalf("Subscribe backlog = %d, Events = %d", len(backlog), len(rest))
	}
}

func TestRingDropsOldestEvents(t *testing.T) {
	ssn := newSession("s6", "ring", newMockLoop(&mockProvider{}), sessionOptions{maxRingEvents: 3})

	for i := 0; i < 10; i++ {
		ssn.broadcast(provider.StreamEvent{Type: provider.EventTextDelta, Text: "x"})
	}
	events := ssn.Events(0)
	if len(events) != 3 {
		t.Fatalf("ring must hold 3 events, got %d", len(events))
	}
	if events[0].Seq != 8 || events[2].Seq != 10 {
		t.Errorf("expected the newest 3 events (8..10), got %d..%d", events[0].Seq, events[2].Seq)
	}
	if ssn.LastSeq() != 10 {
		t.Errorf("LastSeq = %d, want 10", ssn.LastSeq())
	}
}

func TestSlowSubscriberIsDroppedNotBlocking(t *testing.T) {
	ssn := newSession("s7", "slow", newMockLoop(&mockProvider{}), sessionOptions{maxRingEvents: 1024})

	slow, _, _ := ssn.Subscribe(0)
	// Do not drain `slow`: fill its 256-slot buffer plus headroom.
	for i := 0; i < 400; i++ {
		ssn.broadcast(provider.StreamEvent{Type: provider.EventTextDelta, Text: "y"})
	}

	// The stalled subscriber must have been evicted...
	ssn.mu.Lock()
	_, stillSubscribed := ssn.subscribers[slow]
	ssn.mu.Unlock()
	if stillSubscribed {
		t.Fatal("stalled subscriber should have been dropped")
	}

	// ...and a healthy subscriber still receives everything.
	fresh, _, release := ssn.Subscribe(ssn.LastSeq())
	defer release()
	for i := 0; i < 5; i++ {
		ssn.broadcast(provider.StreamEvent{Type: provider.EventTextDelta, Text: "z"})
	}
	for i := 0; i < 5; i++ {
		select {
		case ev := <-fresh:
			if ev.Type != provider.EventTextDelta {
				t.Fatalf("unexpected event %s", ev.Type)
			}
		case <-time.After(time.Second):
			t.Fatalf("live subscriber stopped at event %d", i)
		}
	}
}

func TestToolResultTruncation(t *testing.T) {
	big := strings.Repeat("A", 1000)
	ssn := newSession("s8", "trunc", newMockLoop(&mockProvider{}), sessionOptions{maxToolResultBytes: 100})

	ssn.broadcast(provider.StreamEvent{
		Type:           provider.EventToolCallEnd,
		ToolName:       "bash",
		ToolFullResult: big,
		ToolOK:         true,
	})
	events := ssn.Events(0)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if len(ev.ToolResult) > 130 {
		t.Errorf("tool result not truncated: %d bytes", len(ev.ToolResult))
	}
	if ev.ToolResultLen != 1000 {
		t.Errorf("ToolResultLen = %d, want the full 1000", ev.ToolResultLen)
	}
	if !strings.Contains(ev.ToolResult, "truncated") {
		t.Error("truncated payload should say so")
	}
}

func TestPanicInStreamSurfacesAsErrorEvent(t *testing.T) {
	// A provider that panics mid-stream must not kill the session or the server:
	// the loop's recover turns it into an error event and the run ends.
	p := &mockProvider{onStream: func(ctx context.Context, call int) []provider.StreamEvent {
		panic("boom in provider")
	}}
	ssn := newSession("s9", "panic", newMockLoop(p), sessionOptions{})

	mustTask(t, ssn, "trigger panic")
	events := collect(t, ssn, 3*time.Second)

	var sawError bool
	for _, ev := range events {
		if ev.Type == provider.EventError {
			sawError = true
			if !strings.Contains(ev.Error, "panic") {
				t.Errorf("error event should mention the panic: %q", ev.Error)
			}
		}
	}
	if !sawError {
		t.Fatalf("expected an error event after a provider panic, got %v", eventTypes(events))
	}
	waitFor(t, 2*time.Second, func() bool { return ssn.Status() == StatusIdle }, "session to recover")
}

func TestInjectReachesRunningLoop(t *testing.T) {
	var seen int32
	p := &mockProvider{onStream: func(ctx context.Context, call int) []provider.StreamEvent {
		if call == 1 {
			// Stall so the inject lands while the run is in flight.
			time.Sleep(150 * time.Millisecond)
			return textStream("first")
		}
		return textStream("second")
	}}
	_ = seen
	ssn := newSession("s10", "inject", newMockLoop(p), sessionOptions{})

	mustTask(t, ssn, "task")
	if err := ssn.Inject("quick correction"); err != nil {
		t.Fatalf("Inject: %v", err)
	}
	waitFor(t, 3*time.Second, func() bool { return ssn.Status() == StatusIdle }, "run to finish")

	found := false
	for _, m := range ssn.loop.Context.GetMessages() {
		if m.Role == "user" && strings.Contains(m.GetText(), "quick correction") {
			found = true
		}
	}
	if !found {
		t.Error("injected message never reached the agent context")
	}
}

func TestInjectOnClosedSession(t *testing.T) {
	ssn := newSession("s11", "closed", newMockLoop(&mockProvider{}), sessionOptions{})
	ssn.Close()
	if err := ssn.Inject("too late"); err != ErrClosed {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestCompactReducesContext(t *testing.T) {
	loop := newMockLoop(&mockProvider{})
	// Auto-compaction is off so the context really grows before we compact.
	loop.Context.SetAutoCompact(false)
	for i := 0; i < 40; i++ {
		loop.Context.Add(provider.UserMsg(strings.Repeat("filler ", 200)))
	}
	before := loop.Context.MessageCount()
	loop.Context.SetAutoCompact(true)

	ssn := newSession("s12", "compact", loop, sessionOptions{})
	if err := ssn.Compact(); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if after := ssn.loop.Context.MessageCount(); after >= before {
		t.Errorf("compaction removed nothing: %d -> %d messages", before, after)
	}
}

func TestAnswerWithoutPendingQuestion(t *testing.T) {
	ssn := newSession("s13", "ask", newMockLoop(&mockProvider{}), sessionOptions{})
	if ssn.Answer("random answer") {
		t.Error("Answer should report nothing pending")
	}
	if ssn.PendingQuestion() != "" {
		t.Error("no question should be pending")
	}
}

func TestCancelIdleSessionFails(t *testing.T) {
	ssn := newSession("s14", "idle", newMockLoop(&mockProvider{}), sessionOptions{})
	if err := ssn.Cancel(); err == nil {
		t.Error("cancelling an idle session should fail")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	ssn := newSession("s15", "close", newMockLoop(&mockProvider{}), sessionOptions{})
	ssn.Close()
	ssn.Close() // must not panic on the second call
	if ssn.Status() != StatusClosed {
		t.Errorf("expected closed, got %s", ssn.Status())
	}
	// Subscribing to a closed session yields a closed channel, not a hang.
	events, _, release := ssn.Subscribe(0)
	defer release()
	select {
	case _, open := <-events:
		if open {
			t.Error("closed session should not deliver events")
		}
	case <-time.After(time.Second):
		t.Error("subscribe on closed session hung")
	}
}

// eventTypes is a test-failure helper: compact list of what arrived.
func eventTypes(events []Event) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		out = append(out, ev.Type)
	}
	return out
}
