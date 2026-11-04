package serve

import (
	"log/slog"
	"os"
	"sort"

	"bugbuster-code/pkg/agent"
	"bugbuster-code/pkg/provider"
)

// store persists served sessions in the same JSONL layout the CLI uses, so a
// session started over HTTP can be resumed later with `bugbuster --session <id>`
// and vice versa.
//
// Persistence is best-effort by design: a failed write logs a warning but never
// breaks a live run. An empty dir disables everything.
type store struct {
	dir string
	mgr *agent.SessionManager
}

func newStore(dir string) *store {
	st := &store{dir: dir}
	if dir != "" {
		st.mgr = agent.NewSessionManager(dir)
	}
	return st
}

// save writes the session transcript. No-op when persistence is disabled or
// the session was closed before it ever had messages.
func (st *store) save(ssn *Session) error {
	if st.mgr == nil || ssn == nil {
		return nil
	}
	msgs := ssn.loop.Context.GetMessages()
	rec := &agent.Session{
		ID:       ssn.ID,
		Name:     ssn.Name,
		Messages: stripSystem(msgs),
	}
	if p := ssn.loop.GetProvider(); p != nil {
		rec.ProviderName = p.Name()
	}
	if err := st.mgr.SaveSessionMessages(rec); err != nil {
		slog.Debug("serve: persist failed", "session", ssn.ID, "error", err)
		return err
	}
	return nil
}

// forget removes a persisted transcript.
func (st *store) forget(id string) {
	if st.mgr == nil || id == "" {
		return
	}
	if err := st.mgr.DeleteSession(id); err != nil {
		slog.Debug("serve: forget failed", "session", id, "error", err)
	}
}

// load restores previously persisted sessions into the registry so that
// `GET /sessions` keeps working across a server restart and a task can be
// appended to an old conversation. Restored sessions start idle; the agent loop
// is rebuilt from the factory and the transcript is replayed into its context.
func (st *store) load(srv *Server) error {
	if st.mgr == nil || srv == nil {
		return nil
	}
	if err := os.MkdirAll(st.dir, 0755); err != nil {
		return err
	}
	list, err := st.mgr.ListSessions()
	if err != nil {
		return err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })

	for _, rec := range list {
		if len(srv.List()) >= srv.cfg.MaxSessions {
			slog.Info("serve: not restoring more sessions (max reached)", "restored", len(srv.List()))
			break
		}
		loop, err := srv.cfg.Factory(rec.ID, rec.Name)
		if err != nil {
			slog.Warn("serve: failed to rebuild loop for restored session", "session", rec.ID, "error", err)
			continue
		}
		restoreMessages(loop, rec.Messages)

		ssn := newSession(rec.ID, rec.Name, loop, sessionOptions{
			maxRingEvents:      srv.cfg.MaxRingEvents,
			maxToolResultBytes: srv.cfg.MaxToolResultBytes,
			onIdle:             srv.persist,
		})
		ssn.CreatedAt = rec.CreatedAt

		srv.mu.Lock()
		srv.sessions[rec.ID] = ssn
		srv.order = append(srv.order, rec.ID)
		srv.mu.Unlock()
	}
	if len(srv.List()) > 0 {
		slog.Info("serve: restored persisted sessions", "count", len(srv.List()))
	}
	return nil
}

// restoreMessages replays a stored transcript into a fresh loop context,
// keeping the loop's own system prompt (tool descriptions may have changed).
func restoreMessages(loop *agent.AgentLoop, messages []provider.Message) {
	systemPrompt := loop.Context.GetSystemPrompt()
	messages = agent.RemoveToolErrors(messages)
	messages = agent.RemoveDuplicates(messages)
	messages = agent.EnsureToolPairIntegrity(messages)

	loop.Context.Reset()
	if systemPrompt != "" {
		loop.Context.Add(provider.SystemMsg(systemPrompt))
	}
	for _, msg := range stripSystem(messages) {
		loop.Context.Add(msg)
	}
}

// stripSystem drops system messages — they are regenerated per loop and would
// otherwise accumulate one duplicate per save/load cycle.
func stripSystem(msgs []provider.Message) []provider.Message {
	out := make([]provider.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		out = append(out, m)
	}
	return out
}
