package serve

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// API version reported by /health. Bump when a breaking change lands.
const APIVersion = "1"

// routes builds the mux. Paths are versioned under /v1 so clients can pin.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)

	mux.HandleFunc("GET /v1/sessions", s.handleListSessions)
	mux.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /v1/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("DELETE /v1/sessions/{id}", s.handleDeleteSession)

	mux.HandleFunc("POST /v1/sessions/{id}/task", s.handleTask)
	mux.HandleFunc("POST /v1/sessions/{id}/inject", s.handleInject)
	mux.HandleFunc("POST /v1/sessions/{id}/answer", s.handleAnswer)
	mux.HandleFunc("POST /v1/sessions/{id}/cancel", s.handleCancel)
	mux.HandleFunc("POST /v1/sessions/{id}/compact", s.handleCompact)
	mux.HandleFunc("GET /v1/sessions/{id}/events", s.handleEvents)
	mux.HandleFunc("GET /v1/sessions/{id}/transcript", s.handleTranscript)

	mux.HandleFunc("POST /v1/run", s.handleRun)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, fmt.Sprintf("unknown path %s — see GET /health", r.URL.Path))
	})

	return mux
}

// auth gates everything except /health.
//
// The token is accepted as "Authorization: Bearer <token>" or ?token=<token>
// (the query form exists for SSE consumers behind proxies that strip headers).
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		if s.cfg.Token == "" {
			// New() only allows this on loopback with an explicit opt-in.
			next.ServeHTTP(w, r)
			return
		}
		got := bearerToken(r)
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer realm=\"bugbuster-serve\"")
			writeErr(w, http.StatusUnauthorized, "invalid or missing bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if len(h) >= 7 && strings.EqualFold(h[:7], "bearer ") {
			return strings.TrimSpace(h[7:])
		}
		return strings.TrimSpace(h)
	}
	return r.URL.Query().Get("token")
}

// --- handlers --------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"api":      APIVersion,
		"sessions": len(s.List()),
		"auth":     s.cfg.Token != "",
		"version":  s.cfg.Version,
	})
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sessions": s.List()})
}

// createRequest is the body of POST /v1/sessions and POST /v1/run.
type createRequest struct {
	Name string `json:"name,omitempty"`
	Task string `json:"task,omitempty"`
	Wait bool   `json:"wait,omitempty"`
	// TimeoutSec bounds a blocking run (POST /v1/run?wait=true). 0 = server default.
	TimeoutSec int `json:"timeout_sec,omitempty"`
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	ssn, err := s.CreateSession(req.Name)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	if req.Task != "" {
		// context.Background(): the run must outlive this HTTP request. A
		// client that disconnects after POST keeps the agent working; stopping
		// it is an explicit DELETE/POST .../cancel.
		if err := ssn.Start(context.Background(), req.Task); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"session":  ssn.State(),
		"events":   fmt.Sprintf("/v1/sessions/%s/events", ssn.ID),
		"accepted": req.Task != "",
	}); err != nil {
		return
	}
}

func (s *Server) sessionFromPath(w http.ResponseWriter, r *http.Request) (*Session, bool) {
	id := r.PathValue("id")
	ssn := s.Get(id)
	if ssn == nil {
		writeErr(w, http.StatusNotFound, fmt.Sprintf("unknown session %q", id))
		return nil, false
	}
	return ssn, true
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	ssn, ok := s.sessionFromPath(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, ssn.State())
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	ssn, ok := s.sessionFromPath(w, r)
	if !ok {
		return
	}
	if err := s.CloseSession(ssn.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"closed": ssn.ID})
}

// taskRequest is the body of POST /v1/sessions/{id}/task.
type taskRequest struct {
	Task string `json:"task"`
}

func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	var req taskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Task == "" {
		writeErr(w, http.StatusBadRequest, "task is required")
		return
	}
	ssn, ok := s.sessionFromPath(w, r)
	if !ok {
		return
	}
	// The run outlives the request: disconnecting is not a cancel signal here.
	if err := ssn.Start(context.Background(), req.Task); err != nil {
		status := http.StatusBadRequest
		if err == ErrBusy {
			status = http.StatusConflict
		}
		writeErr(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"session": ssn.ID,
		"status":  ssn.Status(),
		"events":  fmt.Sprintf("/v1/sessions/%s/events", ssn.ID),
	})
}

// textRequest is the body for inject / answer.
type textRequest struct {
	Text string `json:"text"`
}

func (s *Server) handleInject(w http.ResponseWriter, r *http.Request) {
	ssn, ok := s.sessionFromPath(w, r)
	if !ok {
		return
	}
	var req textRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := ssn.Inject(req.Text); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"injected": true, "session": ssn.ID})
}

func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request) {
	ssn, ok := s.sessionFromPath(w, r)
	if !ok {
		return
	}
	var req textRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if !ssn.Answer(req.Text) {
		writeErr(w, http.StatusConflict, "no pending ask_user question for this session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"answered": true, "session": ssn.ID})
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	ssn, ok := s.sessionFromPath(w, r)
	if !ok {
		return
	}
	if err := ssn.Cancel(); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cancelling": true, "session": ssn.ID})
}

func (s *Server) handleCompact(w http.ResponseWriter, r *http.Request) {
	ssn, ok := s.sessionFromPath(w, r)
	if !ok {
		return
	}
	if err := ssn.Compact(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ssn.State())
}

func (s *Server) handleTranscript(w http.ResponseWriter, r *http.Request) {
	ssn, ok := s.sessionFromPath(w, r)
	if !ok {
		return
	}
	limit := queryInt(r, "limit", 0)
	writeJSON(w, http.StatusOK, map[string]any{
		"session":    ssn.ID,
		"messages":   ssn.Transcript(limit),
		"message_ct": ssn.State().Messages,
	})
}

// --- helpers ---------------------------------------------------------------

// decodeJSON reads a JSON body, rejecting unknown fields so a typo in a client
// payload fails loudly instead of being silently ignored.
func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil // empty body = zero value, fine for optional payloads
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if strings.Contains(err.Error(), "EOF") {
			return nil
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Headers are already sent; the only thing left is to stop writing.
		return
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg, "status": status})
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
