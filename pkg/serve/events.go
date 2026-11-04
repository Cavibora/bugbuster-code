// Package serve exposes a running BugBuster agent over HTTP.
//
// It is the control-plane counterpart of the CLI: instead of typing into a
// terminal, an external process (CI job, bot, IDE plugin, another agent)
// creates a session, posts a task, streams the agent's events as SSE, injects
// mid-run clarifications and cancels the run.
//
// The package deliberately owns no provider/config logic: the caller supplies
// a SessionFactory that returns a fully wired *agent.AgentLoop (see
// cmd/bugbuster/serve.go). That keeps serve unit-testable with a mock provider
// and keeps the wiring of tools/timeouts/hub in one place — createAgentLoop.
package serve

import (
	"time"

	"bugbuster-code/pkg/provider"
)

// Event is the wire representation of provider.StreamEvent.
//
// provider.StreamEvent is not JSON-serialisable as-is: it carries a time.Duration,
// an error interface and several fields that only matter to the terminal UI.
// Event is the stable, versioned subset that API clients can rely on.
// Field names are lowercase and self-describing; clients should ignore unknown
// fields so that adding one is not a breaking change.
type Event struct {
	// Seq is a monotonically increasing per-session event number, starting at 1.
	// SSE clients use it for resumable streaming (Last-Event-ID).
	Seq int `json:"seq"`
	// Type mirrors provider.StreamEvent.Type (EventTextDelta, EventToolCallEnd, ...).
	Type string `json:"type"`
	// TS is when the event was produced (server clock).
	TS time.Time `json:"ts"`

	Text       string `json:"text,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`

	// Tool results are truncated by default (see Server.MaxToolResultBytes) —
	// a single bash log can be megabytes and would flood every subscriber.
	ToolResult    string `json:"tool_result,omitempty"`
	ToolResultLen int    `json:"tool_result_len,omitempty"` // full length before truncation
	ToolOK        bool   `json:"tool_ok,omitempty"`

	Iteration    int `json:"iteration,omitempty"`
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`

	// DurationMS is the wall-clock duration of the step, in milliseconds.
	DurationMS int64 `json:"duration_ms,omitempty"`

	// Error message, present only for type == provider.EventError.
	Error string `json:"error,omitempty"`

	// Compaction fields (type == EventCompaction / EventCompactionDone).
	TokensBefore   int    `json:"tokens_before,omitempty"`
	TokensAfter    int    `json:"tokens_after,omitempty"`
	MessagesBefore int    `json:"messages_before,omitempty"`
	MessagesAfter  int    `json:"messages_after,omitempty"`
	CompactKind    string `json:"compaction_type,omitempty"`
}

// fromStreamEvent converts one provider event into the wire form.
// seq is assigned by the caller (session.broadcast), so conversion stays pure.
func fromStreamEvent(se provider.StreamEvent, seq int, maxResultBytes int) Event {
	ev := Event{
		Seq:            seq,
		Type:           se.Type,
		TS:             time.Now().UTC(),
		Text:           se.Text,
		ToolName:       se.ToolName,
		ToolCallID:     se.ToolCallID,
		StopReason:     se.StopReason,
		Iteration:      se.Iteration,
		InputTokens:    se.InputTokens,
		OutputTokens:   se.OutputTokens,
		ToolOK:         se.ToolOK,
		TokensBefore:   se.TokensBefore,
		TokensAfter:    se.TokensAfter,
		MessagesBefore: se.MessagesBefore,
		MessagesAfter:  se.MessagesAfter,
		CompactKind:    se.CompactionType,
	}

	if se.Duration > 0 {
		ev.DurationMS = se.Duration.Milliseconds()
	}
	if se.Error != nil {
		ev.Error = se.Error.Error()
	}

	// Prefer the full result for the payload, fall back to the short header text.
	result := se.ToolFullResult
	if result == "" {
		result = se.ToolResult
	}
	if result != "" {
		ev.ToolResultLen = len(result)
		if maxResultBytes > 0 && len(result) > maxResultBytes {
			result = result[:maxResultBytes] + "\n…[truncated]"
		}
		ev.ToolResult = result
	}

	return ev
}

// Status is the lifecycle state of a served session.
type Status string

const (
	// StatusIdle — session exists, no run in flight.
	StatusIdle Status = "idle"
	// StatusRunning — a task is executing (streaming loop is alive).
	StatusRunning Status = "running"
	// StatusCancelling — cancel requested, loop has not exited yet.
	StatusCancelling Status = "cancelling"
	// StatusClosed — session was closed; it accepts no further work.
	StatusClosed Status = "closed"
)

// IsTerminal reports whether the status can still accept a new task.
func (s Status) IsTerminal() bool { return s == StatusClosed }
