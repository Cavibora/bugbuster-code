package serve

import (
	"bugbuster-code/pkg/provider"
)

// --- event fan-out ---------------------------------------------------------

// broadcast converts one provider event, appends it to the replay ring and
// pushes it to every subscriber.
//
// Locking note: subscribers are signalled while holding s.mu. That keeps the
// ring and the live channels consistent (no event can be missed between a
// replay and a subscribe). It is safe because delivery is non-blocking: a slow
// client is dropped rather than allowed to hold the lock and stall the agent.
func (s *Session) broadcast(se provider.StreamEvent) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	ev := fromStreamEvent(se, s.nextSeq, s.maxResultBytes)
	s.nextSeq++

	s.ring = append(s.ring, ev)
	if over := len(s.ring) - s.maxRingEvents; over > 0 {
		s.ring = append(s.ring[:0:0], s.ring[over:]...)
	}

	for ch := range s.subscribers {
		select {
		case ch <- ev:
		default:
			// Subscriber is behind: drop it so the run keeps moving.
			close(ch)
			delete(s.subscribers, ch)
		}
	}
	s.mu.Unlock()
}

// broadcastRaw is a convenience for events produced outside the loop
// (panics, start failures).
func (s *Session) broadcastRaw(se provider.StreamEvent) { s.broadcast(se) }

// Subscribe returns a channel receiving live events and a release function.
//
// since: replay events with Seq > since before attaching to the live stream.
// Pass 0 for "everything still in the ring", or the last Seq you saw to resume.
// The second return value is the replayed backlog.
func (s *Session) Subscribe(since int) (chan Event, []Event, func()) {
	s.mu.Lock()
	// since=0 means "everything still in the ring": Seq starts at 1, so the
	// plain comparison covers it. Guarding on since > 0 here would silently
	// return an empty backlog and leave a reconnecting client hanging.
	var backlog []Event
	for _, ev := range s.ring {
		if ev.Seq > since {
			backlog = append(backlog, ev)
		}
	}
	ch := make(chan Event, 256)
	if s.closed {
		s.mu.Unlock()
		close(ch)
		return ch, backlog, func() {}
	}
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()

	return ch, backlog, func() { s.unsubscribe(ch) }
}

func (s *Session) unsubscribe(ch chan Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subscribers[ch]; ok {
		delete(s.subscribers, ch)
		close(ch)
	}
}

// Events returns a copy of the replay buffer (bounded by the ring size).
func (s *Session) Events(since int) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Event, 0, len(s.ring))
	for _, ev := range s.ring {
		if ev.Seq > since {
			out = append(out, ev)
		}
	}
	return out
}

// LastSeq is the sequence number of the most recent event.
func (s *Session) LastSeq() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextSeq - 1
}

// --- introspection ---------------------------------------------------------

// State is the JSON view of a session for GET /sessions/{id}.
type State struct {
	ID              string   `json:"id"`
	Name            string   `json:"name,omitempty"`
	Status          Status   `json:"status"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	Iteration       int      `json:"iteration"`
	Messages        int      `json:"messages"`
	Tokens          int      `json:"tokens"`
	MaxTokens       int      `json:"max_tokens"`
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	Tools           []string `json:"tools,omitempty"`
	LastSeq         int      `json:"last_seq"`
	LastError       string   `json:"last_error,omitempty"`
	PendingQuestion string   `json:"pending_question,omitempty"`
	OriginalTask    string   `json:"original_task,omitempty"`
}

// State builds a snapshot of the session and its agent context.
func (s *Session) State() State {
	s.mu.Lock()
	status := s.status
	lastError := s.lastError
	pending := s.pendingQuestion
	lastSeq := s.nextSeq - 1
	created := s.CreatedAt
	updated := s.updatedAt
	name := s.Name
	id := s.ID
	closed := s.closed
	s.mu.Unlock()

	st := State{
		ID:              id,
		Name:            name,
		Status:          status,
		CreatedAt:       created.Format(timeRFC3339),
		UpdatedAt:       updated.Format(timeRFC3339),
		LastSeq:         lastSeq,
		LastError:       lastError,
		PendingQuestion: pending,
	}
	if closed {
		st.Status = StatusClosed
	}

	if s.loop != nil {
		st.Messages = s.loop.Context.MessageCount()
		st.Tokens = s.loop.Context.TokenCount()
		st.MaxTokens = s.loop.Context.MaxTokensValue()
		st.OriginalTask = s.loop.Context.OriginalTaskValue()
		st.Iteration = s.loop.CurrentIteration()
		if p := s.loop.GetProvider(); p != nil {
			st.Provider = p.Name()
			st.Model = p.Model()
		}
		st.Tools = s.loop.ToolNames()
	}
	return st
}

// Transcript returns the conversation as plain text, skipping system prompt
// and tool payloads that only add noise for a remote reader.
func (s *Session) Transcript(max int) []Message {
	if s.loop == nil {
		return nil
	}
	msgs := s.loop.Context.GetMessages()
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		cm := Message{Role: m.Role, Text: m.GetText()}
		for _, b := range m.Content {
			switch b.Type {
			case "tool_use":
				cm.Tool = b.ToolName
			case "tool_result":
				if cm.Tool == "" {
					cm.Tool = "tool_result"
				}
			case "image":
				cm.HasImage = true
			}
		}
		if cm.Text == "" && cm.Tool == "" && !cm.HasImage {
			continue
		}
		out = append(out, cm)
	}
	if max > 0 && len(out) > max {
		out = out[len(out)-max:]
	}
	return out
}

// Message is one transcript entry for the API.
type Message struct {
	Role     string `json:"role"`
	Text     string `json:"text,omitempty"`
	Tool     string `json:"tool,omitempty"`
	HasImage bool   `json:"has_image,omitempty"`
}
