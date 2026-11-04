package serve

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bugbuster-code/pkg/agent"
	"bugbuster-code/pkg/provider"
)

func do(t *testing.T, method, url, token, body string) (*http.Response, string) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, string(data)
}

func TestHealthNeedsNoToken(t *testing.T) {
	srv := testServer(t, func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(&mockProvider{}), nil }, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, body := do(t, "GET", ts.URL+"/health", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health = %d, want 200 (%s)", resp.StatusCode, body)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("health is not JSON: %v", err)
	}
	if out["api"] != APIVersion {
		t.Errorf("api = %v, want %v", out["api"], APIVersion)
	}
	if out["auth"] != true {
		t.Error("auth should be reported as enabled")
	}
}

func TestAuthEnforced(t *testing.T) {
	srv := testServer(t, func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(&mockProvider{}), nil }, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	if resp, _ := do(t, "GET", ts.URL+"/v1/sessions", "", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("missing token = %d, want 401", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", ts.URL+"/v1/sessions", "wrong", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong token = %d, want 401", resp.StatusCode)
	}
	resp, _ := do(t, "GET", ts.URL+"/v1/sessions", "test-token", "")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("good token = %d, want 200", resp.StatusCode)
	}
}

func TestTokenQueryParamWorksForSSE(t *testing.T) {
	srv := testServer(t, func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(&mockProvider{}), nil }, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, _ := do(t, "GET", ts.URL+"/v1/sessions?token=test-token", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("?token= = %d, want 200", resp.StatusCode)
	}
}

func TestUnauthenticatedNonLoopbackRefused(t *testing.T) {
	_, err := New(Config{
		Addr:         "0.0.0.0:9999",
		AllowNoToken: true,
		Factory:      func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(&mockProvider{}), nil },
	})
	if err == nil {
		t.Fatal("must refuse to run open on a non-loopback address")
	}
	if !strings.Contains(err.Error(), "non-loopback") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMissingTokenWithoutOptInRefused(t *testing.T) {
	_, err := New(Config{
		Addr:    "127.0.0.1:0",
		Factory: func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(&mockProvider{}), nil },
	})
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("expected a token error, got %v", err)
	}
}

func TestLoopbackNoTokenAllowed(t *testing.T) {
	srv, err := New(Config{
		Addr:         "127.0.0.1:0",
		AllowNoToken: true,
		Factory:      func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(&mockProvider{}), nil },
	})
	if err != nil {
		t.Fatalf("loopback without token should start: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	resp, _ := do(t, "GET", ts.URL+"/v1/sessions", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("open loopback = %d, want 200", resp.StatusCode)
	}
}

func TestSessionLifecycleOverHTTP(t *testing.T) {
	// stall keeps the run in flight long enough for the cancel/collision
	// assertions below to be deterministic rather than racy.
	p := &mockProvider{stall: 50 * time.Millisecond, events: textStream("done the work")}
	srv := testServer(t, func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(p), nil }, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	const tok = "test-token"

	// Create with a task.
	resp, body := do(t, "POST", ts.URL+"/v1/sessions", tok, `{"name":"ci","task":"build it"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d, want 201 (%s)", resp.StatusCode, body)
	}
	var created struct {
		Session State  `json:"session"`
		Events  string `json:"events"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("create body: %v", err)
	}
	id := created.Session.ID
	if id == "" {
		t.Fatal("no session id returned")
	}
	if !strings.Contains(created.Events, id) {
		t.Errorf("events URL %q should contain the session id", created.Events)
	}

	// Poll until the run finishes.
	deadline := time.Now().Add(5 * time.Second)
	var state State
	for time.Now().Before(deadline) {
		resp, body = do(t, "GET", ts.URL+"/v1/sessions/"+id, tok, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("get = %d (%s)", resp.StatusCode, body)
		}
		if err := json.Unmarshal([]byte(body), &state); err != nil {
			t.Fatalf("state body: %v", err)
		}
		if state.Status == StatusIdle {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if state.Status != StatusIdle {
		t.Fatalf("run never finished, status = %s", state.Status)
	}
	if state.Provider != "mock" || state.Model != "mock-model" {
		t.Errorf("provider/model = %q/%q", state.Provider, state.Model)
	}
	if state.Messages == 0 {
		t.Error("state should report message count")
	}

	// List shows it.
	resp, body = do(t, "GET", ts.URL+"/v1/sessions", tok, "")
	var list struct {
		Sessions []State `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("list body: %v", err)
	}
	if len(list.Sessions) != 1 || list.Sessions[0].Name != "ci" {
		t.Errorf("list = %+v", list.Sessions)
	}

	// Transcript contains the exchange.
	resp, body = do(t, "GET", ts.URL+"/v1/sessions/"+id+"/transcript", tok, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("transcript = %d", resp.StatusCode)
	}
	if !strings.Contains(body, "build it") || !strings.Contains(body, "done the work") {
		t.Errorf("transcript missing content: %s", body)
	}

	// Second task on the same session is fine once idle.
	resp, body = do(t, "POST", ts.URL+"/v1/sessions/"+id+"/task", tok, `{"task":"again"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("task = %d (%s)", resp.StatusCode, body)
	}

	// Busy session rejects a parallel task with 409.
	resp, body = do(t, "POST", ts.URL+"/v1/sessions/"+id+"/task", tok, `{"task":"collision"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("parallel task = %d, want 409 (%s)", resp.StatusCode, body)
	}

	// Cancel the in-flight run.
	resp, body = do(t, "POST", ts.URL+"/v1/sessions/"+id+"/cancel", tok, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel = %d (%s)", resp.StatusCode, body)
	}

	// Close, then the id is gone.
	resp, body = do(t, "DELETE", ts.URL+"/v1/sessions/"+id, tok, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete = %d (%s)", resp.StatusCode, body)
	}
	resp, _ = do(t, "GET", ts.URL+"/v1/sessions/"+id, tok, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("get after delete = %d, want 404", resp.StatusCode)
	}
}

func TestBadRequests(t *testing.T) {
	srv := testServer(t, func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(&mockProvider{}), nil }, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	const tok = "test-token"

	cases := []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/v1/sessions/" + strings.Repeat("x", 4) + "/task", `{}`, http.StatusBadRequest},
		{"POST", "/v1/sessions/unknown/task", `{"task":"hi"}`, http.StatusNotFound},
		{"POST", "/v1/sessions/unknown/inject", `{"text":"hi"}`, http.StatusNotFound},
		{"POST", "/v1/run", `{"nope":1}`, http.StatusBadRequest}, // unknown field
		{"GET", "/nope", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		resp, body := do(t, tc.method, ts.URL+tc.path, tok, tc.body)
		if resp.StatusCode != tc.want {
			t.Errorf("%s %s = %d, want %d (%s)", tc.method, tc.path, resp.StatusCode, tc.want, body)
		}
	}
}

func TestRunEndpointWaitsForResult(t *testing.T) {
	p := &mockProvider{events: textStream("the answer is 42")}
	srv := testServer(t, func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(p), nil }, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, body := do(t, "POST", ts.URL+"/v1/run?wait=1", "test-token", `{"task":"what is it"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run = %d (%s)", resp.StatusCode, body)
	}
	var out struct {
		Result  string `json:"result"`
		Session string `json:"session"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("run body: %v", err)
	}
	if !strings.Contains(out.Result, "the answer is 42") {
		t.Errorf("result = %q", out.Result)
	}
	if out.Session == "" {
		t.Error("run should report the session it used")
	}
}

func TestRunEndpointWithoutWaitReturnsAccepted(t *testing.T) {
	p := &mockProvider{stall: 50 * time.Millisecond, events: textStream("later")}
	srv := testServer(t, func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(p), nil }, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	start := time.Now()
	resp, body := do(t, "POST", ts.URL+"/v1/run", "test-token", `{"task":"fire and forget"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("run = %d (%s)", resp.StatusCode, body)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("no-wait run blocked for %s", elapsed)
	}
}

// --- SSE -------------------------------------------------------------------

type sseEvent struct {
	ID   string
	Type string
	Data string
}

// readSSE parses an SSE stream until the reader is closed.
func readSSE(t *testing.T, r io.Reader, max int) []sseEvent {
	t.Helper()
	br := bufio.NewReader(r)
	var out []sseEvent
	var cur sseEvent
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if len(out) > 0 && err == io.EOF {
				return out
			}
			if err == io.EOF {
				t.Fatalf("SSE ended after %d events: %v", len(out), err)
			}
			t.Fatalf("SSE read: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if cur.Data != "" {
				out = append(out, cur)
				cur = sseEvent{}
				if len(out) >= max {
					return out
				}
			}
			cur = sseEvent{}
		case strings.HasPrefix(line, "id: "):
			cur.ID = line[4:]
		case strings.HasPrefix(line, "event: "):
			cur.Type = line[7:]
		case strings.HasPrefix(line, "data: "):
			cur.Data = line[6:]
		case strings.HasPrefix(line, ":"):
			// comment / heartbeat — ignore
		}
	}
}

func TestSSEStreamDeliversEvents(t *testing.T) {
	p := &mockProvider{stall: 20 * time.Millisecond, events: textStream("streamed text")}
	srv := testServer(t, func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(p), nil }, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, body := do(t, "POST", ts.URL+"/v1/sessions", "test-token", `{"task":"stream me"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d (%s)", resp.StatusCode, body)
	}
	var created struct {
		Session State `json:"session"`
	}
	json.Unmarshal([]byte(body), &created)

	req, _ := http.NewRequest("GET", ts.URL+"/v1/sessions/"+created.Session.ID+"/events?follow=0", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	streamResp, err := srvClient.Do(req)
	if err != nil {
		t.Fatalf("sse: %v", err)
	}
	defer streamResp.Body.Close()
	if ct := streamResp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type = %q", ct)
	}

	events := readSSE(t, streamResp.Body, 50)
	if len(events) == 0 {
		t.Fatal("no SSE events")
	}
	var text strings.Builder
	sawDone := false
	for _, ev := range events {
		var parsed Event
		if err := json.Unmarshal([]byte(ev.Data), &parsed); err != nil {
			t.Fatalf("event data not JSON: %v (%s)", err, ev.Data)
		}
		if ev.ID != fmt.Sprint(parsed.Seq) {
			t.Errorf("SSE id %q != seq %d", ev.ID, parsed.Seq)
		}
		if ev.Type != parsed.Type {
			t.Errorf("event name %q != payload type %q", ev.Type, parsed.Type)
		}
		if parsed.Type == provider.EventTextDelta {
			text.WriteString(parsed.Text)
		}
		if parsed.Type == provider.EventDone {
			sawDone = true
		}
	}
	if !sawDone {
		t.Errorf("stream should end with done, got %v", eventTypes(toEvents(events)))
	}
	if !strings.Contains(text.String(), "streamed text") {
		t.Errorf("text = %q", text.String())
	}
}

func toEvents(events []sseEvent) []Event {
	out := make([]Event, 0, len(events))
	for _, e := range events {
		var ev Event
		json.Unmarshal([]byte(e.Data), &ev)
		out = append(out, ev)
	}
	return out
}

func TestSSETextOnlyFilter(t *testing.T) {
	p := &mockProvider{events: []provider.StreamEvent{
		{Type: provider.EventIterationStart, Iteration: 1},
		{Type: provider.EventToolCallStart, ToolName: "read", ToolCallID: "t1"},
		{Type: provider.EventToolCallEnd, ToolName: "read", ToolCallID: "t1", ToolFullResult: "file body"},
		{Type: provider.EventUsage, InputTokens: 10, OutputTokens: 5},
		{Type: provider.EventTextDelta, Text: "only this"},
		{Type: provider.EventDone},
	}}
	srv := testServer(t, func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(p), nil }, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, body := do(t, "POST", ts.URL+"/v1/sessions", "test-token", `{"task":"filter me"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d (%s)", resp.StatusCode, body)
	}
	var created struct {
		Session State `json:"session"`
	}
	json.Unmarshal([]byte(body), &created)

	events := fetchSSE(t, ts.URL+"/v1/sessions/"+created.Session.ID+"/events?text_only=1", "test-token", 20)
	for _, ev := range events {
		switch ev.Type {
		case provider.EventTextDelta, provider.EventThinking, EventAskUser,
			provider.EventDone, provider.EventError:
		default:
			t.Errorf("text_only leaked event %q", ev.Type)
		}
	}
	if len(events) == 0 {
		t.Error("text_only returned nothing")
	}
}

func TestSSESinceReplaysHistory(t *testing.T) {
	p := &mockProvider{events: textStream("already finished")}
	srv := testServer(t, func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(p), nil }, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	_, body := do(t, "POST", ts.URL+"/v1/sessions", "test-token", `{"task":"past"}`)
	var created struct {
		Session State `json:"session"`
	}
	json.Unmarshal([]byte(body), &created)

	// Wait for the run to end so the whole stream is already in the ring.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, st := do(t, "GET", ts.URL+"/v1/sessions/"+created.Session.ID, "test-token", "")
		var state State
		json.Unmarshal([]byte(st), &state)
		if state.Status == StatusIdle {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	all := fetchSSE(t, ts.URL+"/v1/sessions/"+created.Session.ID+"/events", "test-token", 50)
	if len(all) < 2 {
		t.Fatalf("expected a replayed stream, got %d events", len(all))
	}
	// Resume after the first event: everything except the first must arrive.
	tail := fetchSSE(t, fmt.Sprintf("%s/v1/sessions/%s/events?since=%d", ts.URL, created.Session.ID, all[0].Seq), "test-token", 50)
	if len(tail)+1 != len(all) {
		t.Errorf("since=%d replayed %d of %d events (expected %d)", all[0].Seq, len(tail), len(all), len(all)-1)
	}
	if len(tail) > 0 && tail[0].Seq != all[1].Seq {
		t.Errorf("tail starts at %d, expected %d", tail[0].Seq, all[1].Seq)
	}
}

func fetchSSE(t *testing.T, url, token string, max int) []Event {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := srvClient.Do(req)
	if err != nil {
		t.Fatalf("sse %s: %v", url, err)
	}
	defer resp.Body.Close()
	raw := readSSE(t, resp.Body, max)
	out := make([]Event, 0, len(raw))
	for _, e := range raw {
		var ev Event
		if err := json.Unmarshal([]byte(e.Data), &ev); err != nil {
			t.Fatalf("bad event payload: %v", err)
		}
		out = append(out, ev)
	}
	return out
}

// srvClient has no timeout: SSE streams are long-lived by nature.
var srvClient = &http.Client{}

func TestSSEUnauthorized(t *testing.T) {
	srv := testServer(t, func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(&mockProvider{}), nil }, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := srvClient.Get(ts.URL + "/v1/sessions/whatever/events")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("sse without token = %d, want 401", resp.StatusCode)
	}
}

func TestInjectAndCompactEndpoints(t *testing.T) {
	p := &mockProvider{stall: 100 * time.Millisecond, events: textStream("ok")}
	var loops []*agent.AgentLoop
	srv := testServer(t, func(id, name string) (*agent.AgentLoop, error) {
		l := newMockLoop(p)
		loops = append(loops, l)
		return l, nil
	}, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	const tok = "test-token"

	_, body := do(t, "POST", ts.URL+"/v1/sessions", tok, `{"task":"working"}`)
	var created struct {
		Session State `json:"session"`
	}
	json.Unmarshal([]byte(body), &created)
	id := created.Session.ID

	resp, body := do(t, "POST", ts.URL+"/v1/sessions/"+id+"/inject", tok, `{"text":"use the make target"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inject = %d (%s)", resp.StatusCode, body)
	}

	// No pending question → 409.
	resp, body = do(t, "POST", ts.URL+"/v1/sessions/"+id+"/answer", tok, `{"text":"yes"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("answer with nothing pending = %d (%s)", resp.StatusCode, body)
	}

	// Compact on a session with content.
	resp, body = do(t, "POST", ts.URL+"/v1/sessions/"+id+"/compact", tok, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("compact = %d (%s)", resp.StatusCode, body)
	}

	do(t, "POST", ts.URL+"/v1/sessions/"+id+"/cancel", tok, "")
}

func TestMaxSessionsEvictsIdle(t *testing.T) {
	srv := testServer(t, func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(&mockProvider{}), nil }, func(c *Config) {
		c.MaxSessions = 2
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	const tok = "test-token"

	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		resp, body := do(t, "POST", ts.URL+"/v1/sessions", tok, fmt.Sprintf(`{"name":"s%d"}`, i))
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create %d = %d (%s)", i, resp.StatusCode, body)
		}
		var created struct {
			Session State `json:"session"`
		}
		json.Unmarshal([]byte(body), &created)
		ids = append(ids, created.Session.ID)
		time.Sleep(10 * time.Millisecond) // keep a clear LRU order
	}

	_, body := do(t, "GET", ts.URL+"/v1/sessions", tok, "")
	var list struct {
		Sessions []State `json:"sessions"`
	}
	json.Unmarshal([]byte(body), &list)
	if len(list.Sessions) != 2 {
		t.Fatalf("expected the cap to hold at 2, got %d", len(list.Sessions))
	}
	// The oldest idle session must be the one that left.
	for _, s := range list.Sessions {
		if s.ID == ids[0] {
			t.Error("oldest session should have been evicted")
		}
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := &mockProvider{events: textStream("persisted answer")}

	factory := func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(p), nil }
	srv := testServer(t, factory, func(c *Config) { c.SessionsDir = dir })
	srv.cfg.Factory = factory

	ssn, err := srv.CreateSession("keepme")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	mustTask(t, ssn, "remember this")
	waitFor(t, 3*time.Second, func() bool { return ssn.Status() == StatusIdle }, "run to finish")
	// finish() persists via onIdle; give the write a moment.
	waitFor(t, 2*time.Second, func() bool {
		entries, err := os.ReadDir(dir)
		return err == nil && len(entries) > 0
	}, "transcript file to appear")

	// A brand-new server restores the transcript and can continue it.
	srv2 := testServer(t, factory, func(c *Config) { c.SessionsDir = dir })
	if err := srv2.store.load(srv2); err != nil {
		t.Fatalf("load: %v", err)
	}
	list := srv2.List()
	if len(list) != 1 {
		t.Fatalf("restored %d sessions, want 1", len(list))
	}
	if list[0].Name != "keepme" {
		t.Errorf("restored name = %q", list[0].Name)
	}
	restored := srv2.Get(list[0].ID)
	if restored == nil {
		t.Fatal("restored session not addressable")
	}
	found := false
	for _, m := range restored.Transcript(0) {
		if strings.Contains(m.Text, "persisted answer") {
			found = true
		}
	}
	if !found {
		t.Errorf("restored transcript lost the assistant answer: %+v", restored.Transcript(0))
	}

	// And the transcript file is CLI-readable JSONL (first line = metadata).
	data, err := os.ReadFile(filepath.Join(dir, list[0].ID+".jsonl"))
	if err != nil {
		t.Fatalf("transcript file: %v", err)
	}
	if !bytes.Contains(data, []byte(`"id"`)) {
		t.Errorf("transcript is not session JSONL: %s", firstLine(string(data)))
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func TestServerServeAndShutdown(t *testing.T) {
	srv := testServer(t, func(id, name string) (*agent.AgentLoop, error) { return newMockLoop(&mockProvider{}), nil }, func(c *Config) {
		c.Addr = "127.0.0.1:0"
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()

	waitFor(t, 3*time.Second, func() bool { return srv.http != nil }, "server to be constructed")
	cancel()
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}
