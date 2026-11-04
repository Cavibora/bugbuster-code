package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"bugbuster-code/pkg/provider"
)

// handleEvents streams session events as Server-Sent Events.
//
// Query params:
//   - since=<seq>  — replay buffered events with Seq > since first (resumable)
//   - types=a,b    — filter by event type (client-side bandwidth saver)
//   - text_only=1  — only text_delta / thinking / ask_user / done / error
//
// The stream ends when the session emits done/error and no run is active, or
// when the client disconnects. A comment heartbeat is written periodically so
// proxies do not buffer the response.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	ssn, ok := s.sessionFromPath(w, r)
	if !ok {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported by this server")
		return
	}

	since := eventSince(r, ssn)
	filter := eventFilter(r)

	events, backlog, release := ssn.Subscribe(since)
	defer release()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // nginx: disable response buffering
	w.WriteHeader(http.StatusOK)

	// Replay what we missed before going live.
	for _, ev := range backlog {
		if !filter.allows(ev) {
			continue
		}
		if !writeSSE(w, ev) {
			return
		}
	}
	flusher.Flush()

	// A client catching up on a finished run gets the whole history and must not
	// be left hanging: if the replay already contained the terminal event and
	// no new run is in flight, close the stream.
	if !followMode(r) && hasTerminal(backlog) && ssn.Status() != StatusRunning {
		return
	}

	heartbeat := time.NewTicker(s.cfg.HeartbeatInterval)
	defer heartbeat.Stop()

	// Safety net: a client that never receives a terminal event would hang on
	// proxies with long read timeouts. maxStream bounds one connection.
	maxStream := time.NewTimer(s.streamTimeout())
	defer maxStream.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, alive := <-events:
			if !alive {
				writeSSEDone(w, ssn, "closed")
				return
			}
			if filter.allows(ev) {
				if !writeSSE(w, ev) {
					return
				}
			}
			flusher.Flush()
			// Close the stream once the run is over, unless the client asked
			// for a long-lived subscription (?follow=1).
			if isTerminalEvent(ev) && !followMode(r) {
				return
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-maxStream.C:
			fmt.Fprint(w, ": stream timeout\n\n")
			flusher.Flush()
			return
		}
	}
}

// streamTimeout bounds a single SSE connection so a wedged agent cannot keep a
// handler alive forever. Generous by design: real runs are long.
func (s *Server) streamTimeout() time.Duration {
	if s.cfg.MaxStreamDuration > 0 {
		return s.cfg.MaxStreamDuration
	}
	return 2 * time.Hour
}

func isTerminalEvent(ev Event) bool {
	switch ev.Type {
	case provider.EventDone, provider.EventError:
		return true
	}
	return false
}

// hasTerminal reports whether any event in the batch ends a run. The terminal
// event is not necessarily last: the loop can still emit bookkeeping events
// (usage, memory stats) after done.
func hasTerminal(evs []Event) bool {
	for _, ev := range evs {
		if isTerminalEvent(ev) {
			return true
		}
	}
	return false
}

func followMode(r *http.Request) bool {
	v := r.URL.Query().Get("follow")
	return v == "1" || v == "true"
}

// eventSince resolves the replay cursor from Last-Event-ID (per the SSE spec,
// sent automatically on reconnect) or ?since=.
func eventSince(r *http.Request, ssn *Session) int {
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	if v := r.URL.Query().Get("since"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	if r.URL.Query().Get("replay") == "none" {
		return ssn.LastSeq()
	}
	return 0
}

// typeFilter selects which events reach the wire.
type typeFilter struct {
	enabled bool
	types   map[string]bool
	text    bool
}

func (f typeFilter) allows(ev Event) bool {
	if !f.enabled {
		return true
	}
	if f.text {
		switch ev.Type {
		case provider.EventTextDelta, provider.EventThinking, EventAskUser,
			provider.EventDone, provider.EventError:
			return true
		}
		return false
	}
	return f.types[ev.Type]
}

func eventFilter(r *http.Request) typeFilter {
	q := r.URL.Query()
	if q.Get("text_only") == "1" || q.Get("text_only") == "true" {
		return typeFilter{enabled: true, text: true}
	}
	types := q.Get("types")
	if types == "" {
		return typeFilter{}
	}
	f := typeFilter{enabled: true, types: make(map[string]bool)}
	for _, t := range splitCSV(types) {
		f.types[t] = true
	}
	return f
}

func splitCSV(s string) []string {
	out := make([]string, 0, 4)
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := trimSpace(s[start:i])
			if part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}

func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t') {
		j--
	}
	return s[i:j]
}

// writeSSE emits one event. Returns false when the client is gone.
func writeSSE(w http.ResponseWriter, ev Event) bool {
	payload, err := json.Marshal(ev)
	if err != nil {
		return true // skip an unencodable event rather than kill the stream
	}
	if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Type, payload); err != nil {
		return false
	}
	return true
}

// writeSSEDone signals the end of a stream to clients that reconnect on close.
func writeSSEDone(w http.ResponseWriter, ssn *Session, reason string) {
	fmt.Fprintf(w, "event: stream_end\ndata: {\"session\":%q,\"reason\":%q}\n\n", ssn.ID, reason)
}

// handleRun is the one-shot convenience endpoint: create a session, run the
// task, return the final text. With wait=false it behaves like POST /v1/sessions
// with a task and returns immediately with a session handle.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Task == "" {
		writeErr(w, http.StatusBadRequest, "task is required")
		return
	}

	ssn, err := s.CreateSession(req.Name)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	// The run outlives the request: disconnecting is not a cancel signal here.
	if err := ssn.Start(context.Background(), req.Task); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// ?wait=1 overrides the body flag, so curl can use either form.
	wait := req.Wait
	if v := r.URL.Query().Get("wait"); v != "" {
		wait = v == "1" || v == "true"
	}
	if !wait {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"session": ssn.ID,
			"status":  ssn.Status(),
			"events":  fmt.Sprintf("/v1/sessions/%s/events", ssn.ID),
		})
		return
	}

	timeout := time.Duration(req.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	if !waitUntilIdle(ssn, timeout) {
		writeJSON(w, http.StatusGatewayTimeout, map[string]any{
			"session": ssn.ID,
			"status":  ssn.Status(),
			"error":   "timed out waiting for the run to finish; the agent keeps running — poll /v1/sessions/" + ssn.ID,
		})
		return
	}

	state := ssn.State()
	status := http.StatusOK
	if state.LastError != "" {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, map[string]any{
		"session":    ssn.ID,
		"result":     lastAssistantText(ssn),
		"last_error": state.LastError,
		"state":      state,
	})
}

// waitUntilIdle polls for the run to finish. Uses the session's done channel
// through Wait so there is no busy loop.
func waitUntilIdle(ssn *Session, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining > 30*time.Second {
			remaining = 30 * time.Second
		} else if remaining <= 0 {
			return false
		}
		if ssn.Wait(remaining) && ssn.Status() == StatusIdle {
			return true
		}
		if ssn.Status() == StatusClosed {
			return true
		}
	}
	return false
}

// lastAssistantText returns the text of the final assistant message.
func lastAssistantText(ssn *Session) string {
	msgs := ssn.loop.Context.GetMessages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "assistant" {
			continue
		}
		if t := msgs[i].GetResponseText(); t != "" {
			return t
		}
	}
	return ""
}
