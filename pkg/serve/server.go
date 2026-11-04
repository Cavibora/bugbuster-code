package serve

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"bugbuster-code/pkg/agent"
)

const timeRFC3339 = time.RFC3339

// LoopFactory builds a fully wired agent loop for a new session.
// The caller (cmd/bugbuster) owns provider/tool configuration; serve only
// drives the loop. sessionID is the persistent session identity, useful for
// scoping memory/todo files.
type LoopFactory func(sessionID, name string) (*agent.AgentLoop, error)

// Config for the HTTP control plane.
type Config struct {
	// Addr to listen on. Bind to localhost unless you really mean it —
	// this API executes arbitrary shell commands through the agent.
	Addr string
	// Token — bearer token required on every request. Empty means the server
	// refuses to start unless AllowNoToken is set (and then only on loopback).
	Token string
	// AllowNoToken permits an unauthenticated server. Only honoured when Addr
	// is a loopback interface.
	AllowNoToken bool
	// MaxSessions — cap on live sessions; older idle ones are evicted first.
	MaxSessions int
	// IdleTTL — sessions unused for this long are closed and evicted.
	// 0 disables eviction.
	IdleTTL time.Duration
	// MaxToolResultBytes — truncation for tool payloads on the wire (default 4096).
	MaxToolResultBytes int
	// MaxRingEvents — replay buffer per session (default 512).
	MaxRingEvents int
	// SessionsDir — where transcripts are persisted. Empty disables persistence.
	SessionsDir string
	// HeartbeatInterval — SSE comment period to keep proxies from buffering
	// the stream (default 15s).
	HeartbeatInterval time.Duration
	// MaxStreamDuration — hard cap on one SSE connection (default 2h).
	MaxStreamDuration time.Duration
	// Version — reported by /health (the bugbuster build).
	Version string
	// Factory produces agent loops. Required.
	Factory LoopFactory
}

// Server is the HTTP control plane for agent sessions.
type Server struct {
	cfg      Config
	mu       sync.Mutex
	sessions map[string]*Session
	order    []string // creation order, for eviction
	http     *http.Server
	store    *store
}

// New validates cfg and returns a server (not yet listening).
func New(cfg Config) (*Server, error) {
	if cfg.Factory == nil {
		return nil, fmt.Errorf("serve: Config.Factory is required")
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8765"
	}
	if cfg.MaxSessions <= 0 {
		cfg.MaxSessions = 8
	}
	if cfg.MaxToolResultBytes <= 0 {
		cfg.MaxToolResultBytes = 4096
	}
	if cfg.MaxRingEvents <= 0 {
		cfg.MaxRingEvents = 512
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 15 * time.Second
	}
	if cfg.Token == "" && !cfg.AllowNoToken {
		return nil, fmt.Errorf("serve: no token configured — pass --token, or --allow-no-token for a loopback-only server")
	}
	if cfg.Token == "" && cfg.AllowNoToken && !isLoopbackAddr(cfg.Addr) {
		return nil, fmt.Errorf("serve: refusing to run unauthenticated on a non-loopback address (%s)", cfg.Addr)
	}

	srv := &Server{
		cfg:      cfg,
		sessions: make(map[string]*Session),
		store:    newStore(cfg.SessionsDir),
	}
	srv.http = &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.auth(srv.routes()),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv, nil
}

// Serve listens until ctx is cancelled, then drains.
func (s *Server) Serve(ctx context.Context) error {
	if err := s.store.load(s); err != nil {
		slog.Warn("serve: could not restore persisted sessions", "error", err)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	// Idle eviction runs independently of the request loop.
	evictStop := make(chan struct{})
	go s.loopEviction(ctx, evictStop)

	select {
	case <-ctx.Done():
		close(evictStop)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.Shutdown()
		if err := s.http.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	case err := <-errCh:
		close(evictStop)
		return err
	}
}

func (s *Server) loopEviction(ctx context.Context, stop chan struct{}) {
	if s.cfg.IdleTTL <= 0 {
		return
	}
	interval := s.cfg.IdleTTL / 2
	if interval < time.Minute {
		interval = time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-t.C:
			s.evictIdle()
		}
	}
}

// Shutdown closes every session (cancelling runs) and stops persisting.
func (s *Server) Shutdown() {
	s.mu.Lock()
	all := make([]*Session, 0, len(s.sessions))
	for _, ssn := range s.sessions {
		all = append(all, ssn)
	}
	s.mu.Unlock()
	for _, ssn := range all {
		s.persist(ssn)
		ssn.Close()
	}
}

// Handler exposes the HTTP handler — used by tests with httptest.Server.
func (s *Server) Handler() http.Handler { return s.http.Handler }

// Addr is the effective listen address.
func (s *Server) Addr() string { return s.cfg.Addr }

// --- session registry ------------------------------------------------------

// CreateSession builds a new session and registers it.
func (s *Server) CreateSession(name string) (*Session, error) {
	if name == "" {
		name = newID("sess")
	}
	id := newID("api")

	loop, err := s.cfg.Factory(id, name)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	ssn := newSession(id, name, loop, sessionOptions{
		maxRingEvents:      s.cfg.MaxRingEvents,
		maxToolResultBytes: s.cfg.MaxToolResultBytes,
		onIdle:             s.persist,
	})

	s.mu.Lock()
	if len(s.sessions) >= s.cfg.MaxSessions {
		if err := s.evictOneIdleLocked(); err != nil {
			s.mu.Unlock()
			return nil, err
		}
	}
	s.sessions[id] = ssn
	s.order = append(s.order, id)
	s.mu.Unlock()

	s.persist(ssn)
	return ssn, nil
}

// StartTask runs task on an existing session (empty task = no-op registration).
func (s *Server) StartTask(ctx context.Context, id, task string) error {
	ssn := s.Get(id)
	if ssn == nil {
		return fmt.Errorf("unknown session %q", id)
	}
	if task == "" {
		return nil
	}
	return ssn.Start(ctx, task)
}

// Get returns a session by ID or by name.
func (s *Server) Get(idOrName string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ssn, ok := s.sessions[idOrName]; ok {
		return ssn
	}
	for _, id := range s.order {
		if ssn := s.sessions[id]; ssn != nil && ssn.Name == idOrName {
			return ssn
		}
	}
	return nil
}

// List returns session states, newest last.
func (s *Server) List() []State {
	s.mu.Lock()
	ids := append([]string(nil), s.order...)
	all := make([]*Session, 0, len(ids))
	for _, id := range ids {
		if ssn, ok := s.sessions[id]; ok {
			all = append(all, ssn)
		}
	}
	s.mu.Unlock()

	states := make([]State, 0, len(all))
	for _, ssn := range all {
		states = append(states, ssn.State())
	}
	sort.SliceStable(states, func(i, j int) bool { return states[i].CreatedAt < states[j].CreatedAt })
	return states
}

// CloseSession terminates and unregisters a session.
func (s *Server) CloseSession(id string) error {
	s.mu.Lock()
	ssn, ok := s.sessions[id]
	if ok {
		delete(s.sessions, id)
		for i, other := range s.order {
			if other == id {
				s.order = append(s.order[:i], s.order[i+1:]...)
				break
			}
		}
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown session %q", id)
	}
	s.persist(ssn)
	ssn.Close()
	if s.cfg.SessionsDir != "" {
		s.store.forget(id)
	}
	return nil
}

// evictOneIdleLocked closes the least recently used idle session to free a
// slot. The caller must hold s.mu; the actual teardown runs in a goroutine so
// the lock is never held across a wait for the agent loop to exit.
func (s *Server) evictOneIdleLocked() error {
	var victim *Session
	var lastSeen time.Time
	for _, id := range s.order {
		ssn := s.sessions[id]
		if ssn == nil {
			continue
		}
		st := ssn.Status()
		if st == StatusRunning || st == StatusCancelling {
			continue
		}
		act := ssn.LastActivity()
		if victim == nil || act.Before(lastSeen) {
			victim, lastSeen = ssn, act
		}
	}
	if victim == nil {
		return fmt.Errorf("all %d session slots are busy — close one first (max_sessions=%d)", len(s.sessions), s.cfg.MaxSessions)
	}
	id := victim.ID
	delete(s.sessions, id)
	for i, other := range s.order {
		if other == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	s.persist(victim)
	go victim.Close()
	slog.Info("serve: evicted idle session to free a slot", "session", id)
	return nil
}

// evictIdle closes sessions that have been unused for longer than IdleTTL.
func (s *Server) evictIdle() {
	if s.cfg.IdleTTL <= 0 {
		return
	}
	cutoff := time.Now().Add(-s.cfg.IdleTTL)
	s.mu.Lock()
	var stale []*Session
	for _, id := range s.order {
		ssn := s.sessions[id]
		if ssn == nil {
			continue
		}
		switch ssn.Status() {
		case StatusRunning, StatusCancelling:
			continue
		}
		if ssn.LastActivity().Before(cutoff) {
			stale = append(stale, ssn)
		}
	}
	for _, ssn := range stale {
		delete(s.sessions, ssn.ID)
	}
	// rebuild order
	kept := s.order[:0:0]
	for _, id := range s.order {
		if _, ok := s.sessions[id]; ok {
			kept = append(kept, id)
		}
	}
	s.order = kept
	s.mu.Unlock()

	for _, ssn := range stale {
		slog.Info("serve: closing idle session", "session", ssn.ID, "idle_for", time.Since(ssn.LastActivity()).String())
		s.persist(ssn)
		ssn.Close()
	}
}

// persist writes the transcript to disk if persistence is enabled.
func (s *Server) persist(ssn *Session) {
	if s.cfg.SessionsDir == "" || ssn == nil {
		return
	}
	if err := s.store.save(ssn); err != nil {
		slog.Warn("serve: failed to persist session", "session", ssn.ID, "error", err)
	}
}

// isLoopbackAddr reports whether addr binds only to the local machine.
func isLoopbackAddr(addr string) bool {
	host := addr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		host = addr[:i]
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "localhost" {
		return true
	}
	switch host {
	case "127.0.0.1", "::1":
		return true
	}
	return strings.HasPrefix(host, "127.")
}
