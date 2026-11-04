package serve

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"bugbuster-code/pkg/agent"
	"bugbuster-code/pkg/provider"
	"bugbuster-code/pkg/tools"
)

// Session is one agent loop plus its fan-out of events.
//
// Concurrency contract:
//   - exactly one run may be in flight per session (Start returns ErrBusy otherwise);
//   - the streaming goroutine is the only writer to loop.Context;
//   - subscribers receive from their own buffered channel and are dropped (not
//     blocked) when they fall behind, because a stalled client must never
//     stall the agent.
type Session struct {
	ID        string
	Name      string
	CreatedAt time.Time

	mu sync.Mutex

	loop   *agent.AgentLoop
	status Status
	// runCtx is cancelled to stop the in-flight run.
	runCancel context.CancelFunc
	runDone   chan struct{}
	lastError string
	updatedAt time.Time

	// pendingQuestion is the ask_user question awaiting an answer ("" = none).
	pendingQuestion string

	subscribers map[chan Event]struct{}
	// ring keeps the last N events so a reconnecting client can catch up.
	ring          []Event
	nextSeq       int
	closed        bool
	maxRingEvents int

	askCh      *tools.AskChannel
	askAnswer  chan string
	askLoopRun sync.Once

	// maxResultBytes truncates tool results on the wire. Immutable after
	// construction, so it is read without locking.
	maxResultBytes int
	// onIdle is called after a run finishes (persist the transcript).
	onIdle func(*Session)
	// closeCh is closed by Close() so the ask_user bridge stops waiting for an
	// answer instead of holding a goroutine for its full timeout.
	closeCh chan struct{}
}

// ErrBusy is returned when a session already has a run in flight.
var ErrBusy = fmt.Errorf("session is busy: a task is already running")

// ErrClosed is returned when the session has been closed.
var ErrClosed = fmt.Errorf("session is closed")

// newSession builds a session around an already-configured loop.
func newSession(id, name string, loop *agent.AgentLoop, opts sessionOptions) *Session {
	maxRing := opts.maxRingEvents
	if maxRing <= 0 {
		maxRing = 512
	}
	now := time.Now().UTC()
	return &Session{
		ID:             id,
		Name:           name,
		CreatedAt:      now,
		updatedAt:      now,
		loop:           loop,
		status:         StatusIdle,
		subscribers:    make(map[chan Event]struct{}),
		ring:           make([]Event, 0, maxRing),
		nextSeq:        1,
		maxRingEvents:  maxRing,
		askCh:          &tools.AskChannel{Question: make(chan string, 1), Answer: make(chan string, 1)},
		askAnswer:      make(chan string, 1),
		maxResultBytes: opts.maxToolResultBytes,
		onIdle:         opts.onIdle,
		closeCh:        make(chan struct{}),
	}
}

// sessionOptions carries the per-session limits inherited from the server.
type sessionOptions struct {
	maxRingEvents      int
	maxToolResultBytes int
	onIdle             func(*Session)
}

func newID(prefix string) string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		// Time is a fine fallback — IDs only need to be unique per server.
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b)
}

// Status returns the current lifecycle state.
func (s *Session) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// LastActivity is the timestamp of the last state change (used for idle GC).
func (s *Session) LastActivity() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updatedAt
}

// touch records activity and keeps the session alive for idle GC.
func (s *Session) touch() {
	s.mu.Lock()
	s.updatedAt = time.Now().UTC()
	s.mu.Unlock()
}

// Start launches task in the background. Events land in the session stream.
func (s *Session) Start(ctx context.Context, task string) error {
	if task == "" {
		return fmt.Errorf("task is required")
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	if s.status == StatusRunning || s.status == StatusCancelling {
		s.mu.Unlock()
		return ErrBusy
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.runCancel = cancel
	s.runDone = done
	s.status = StatusRunning
	s.lastError = ""
	s.updatedAt = time.Now().UTC()
	s.mu.Unlock()

	s.startAskBridge()

	go s.pump(runCtx, cancel, done, task)
	return nil
}

// pump runs the agent loop and republishes its events.
//
// It must never die silently: an unrecovered panic in a provider or tool would
// otherwise leave the session stuck in "running" with the HTTP client waiting
// forever. The deferred recover closes the run and emits an error event.
func (s *Session) pump(ctx context.Context, cancel context.CancelFunc, done chan struct{}, task string) {
	defer close(done)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			slog.Error("serve: panic in agent run", "session", s.ID, "panic", r, "stack", string(debug.Stack()))
			s.broadcastRaw(provider.StreamEvent{
				Type:  provider.EventError,
				Error: fmt.Errorf("internal panic: %v", r),
			})
			s.finish("internal panic")
		}
	}()

	eventCh, err := s.loop.StreamWithCancel(ctx, task)
	if err != nil {
		slog.Warn("serve: failed to start stream", "session", s.ID, "error", err)
		s.broadcastRaw(provider.StreamEvent{Type: provider.EventError, Error: err})
		s.finish(err.Error())
		return
	}

	for se := range eventCh {
		s.broadcast(se)
		switch se.Type {
		case provider.EventDone:
			s.finish("")
		case provider.EventError:
			msg := ""
			if se.Error != nil {
				msg = se.Error.Error()
			}
			s.finish(msg)
		}
	}

	// Channel closed without a terminal event (cancellation or provider death).
	s.finish("")
}

// finish marks the run as complete exactly once.
func (s *Session) finish(errMsg string) {
	s.mu.Lock()
	if s.status == StatusIdle || s.status == StatusClosed {
		s.mu.Unlock()
		return
	}
	s.status = StatusIdle
	if errMsg != "" {
		s.lastError = errMsg
	}
	s.pendingQuestion = ""
	s.updatedAt = time.Now().UTC()
	s.mu.Unlock()

	if s.onIdle != nil {
		s.onIdle(s)
	}
}

// Cancel requests termination of the in-flight run.
func (s *Session) Cancel() error {
	s.mu.Lock()
	if s.status != StatusRunning && s.status != StatusCancelling {
		s.mu.Unlock()
		return fmt.Errorf("session %s is not running", s.ID)
	}
	cancel := s.runCancel
	s.status = StatusCancelling
	s.updatedAt = time.Now().UTC()
	s.mu.Unlock()

	// Release a waiting ask_user first — otherwise the loop blocks up to 10 min.
	s.Answer("")

	if cancel != nil {
		cancel()
	}
	return nil
}

// Wait blocks until the in-flight run exits (or timeout elapses).
func (s *Session) Wait(timeout time.Duration) bool {
	s.mu.Lock()
	done := s.runDone
	s.mu.Unlock()
	if done == nil {
		return true
	}
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// Inject pushes a user message into the running loop.
func (s *Session) Inject(text string) error {
	if text == "" {
		return fmt.Errorf("text is required")
	}
	if s.Status() == StatusClosed {
		return ErrClosed
	}
	if !s.loop.InjectUserMessage(text) {
		return fmt.Errorf("agent cannot keep up: injection buffer is full")
	}
	s.touch()
	return nil
}

// Compact requests forced context compaction (the compact_force tool path).
//
// While a run is in flight the loop owns the context, so we only post a
// request — the loop applies it between iterations. Compacting from here would
// race with the streaming goroutine (it reads and writes the message list).
// When the session is idle there is no concurrent writer, so we compact now.
func (s *Session) Compact() error {
	if s.Status() == StatusClosed {
		return ErrClosed
	}
	if s.Status() == StatusRunning || s.Status() == StatusCancelling {
		s.loop.RequestCompact()
		s.touch()
		return nil
	}
	s.loop.Context.CompactForce()
	s.touch()
	return nil
}

// --- ask_user bridge -------------------------------------------------------

// startAskBridge wires the loop's ask_user tool to the session event stream
// once per session. Without this, ask_user returns "no answer" in serve mode
// because neither the TUI channel nor the readline callback is set.
func (s *Session) startAskBridge() {
	s.askLoopRun.Do(func() {
		if askTool, ok := s.loop.Tools["ask_user"].(*tools.AskUserTool); ok {
			askTool.SetAskChannel(s.askCh)
		}
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Debug("serve: ask bridge stopped", "session", s.ID, "panic", r)
				}
			}()
			for q := range s.askCh.Question {
				s.mu.Lock()
				s.pendingQuestion = q
				s.mu.Unlock()

				s.broadcast(provider.StreamEvent{Type: EventAskUser, Text: q})

				var answer string
				select {
				case answer = <-s.askAnswer:
				case <-s.closeCh:
					return // session is gone; stop the bridge
				case <-time.After(9 * time.Minute): // tool itself times out at 10
				}

				s.mu.Lock()
				s.pendingQuestion = ""
				s.mu.Unlock()

				select {
				case s.askCh.Answer <- answer:
				default:
				}
			}
		}()
	})
}

// EventAskUser is emitted when the agent asks a question through ask_user.
const EventAskUser = "ask_user"

// Answer replies to a pending ask_user question.
func (s *Session) Answer(answer string) bool {
	s.mu.Lock()
	pending := s.pendingQuestion
	s.mu.Unlock()
	if pending == "" {
		return false
	}
	select {
	case s.askAnswer <- answer:
		s.touch()
		return true
	default:
		return false
	}
}

// PendingQuestion returns the question awaiting an answer, if any.
func (s *Session) PendingQuestion() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pendingQuestion
}

// Close terminates the session: cancels the run, stops delivering events and
// frees the ask_user bridge. Idempotent.
func (s *Session) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	cancel := s.runCancel
	done := s.runDone
	close(s.closeCh)
	subs := make([]chan Event, 0, len(s.subscribers))
	for ch := range s.subscribers {
		subs = append(subs, ch)
		delete(s.subscribers, ch)
	}
	s.status = StatusClosed
	s.updatedAt = time.Now().UTC()
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			slog.Warn("serve: session run did not exit after cancel", "session", s.ID)
		}
	}
	if askTool, ok := s.loop.Tools["ask_user"].(*tools.AskUserTool); ok {
		askTool.SetAskChannel(nil)
	}
	close(s.askCh.Question)
	for _, ch := range subs {
		close(ch)
	}
}
