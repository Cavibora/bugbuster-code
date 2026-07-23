package agenthub

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewHub(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub(dir)
	if hub == nil {
		t.Fatal("Expected non-nil hub")
	}
	if hub.dir != dir {
		t.Errorf("Expected dir=%s, got %s", dir, hub.dir)
	}
}

func TestHubInit(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub(dir)
	if err := hub.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	// Check directories were created
	if _, err := os.Stat(filepath.Join(dir, "agents")); os.IsNotExist(err) {
		t.Error("agents directory not created")
	}
	if _, err := os.Stat(filepath.Join(dir, "messages")); os.IsNotExist(err) {
		t.Error("messages directory not created")
	}
}

func TestHubRegisterUnregister(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub(dir)
	if err := hub.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	profile := &AgentProfile{
		ID:           "test-agent-1",
		Name:         "bugbuster-test",
		Provider:     "openai",
		Model:        "gpt-4o",
		Project:      "/tmp/project",
		Role:         "coder",
		Intelligence: IntelligenceExpert,
		Status:       StatusIdle,
	}

	if err := hub.Register(profile); err != nil {
		t.Fatalf("Register error: %v", err)
	}

	// Check agent was registered
	agents := hub.ListAgents()
	if len(agents) != 1 {
		t.Errorf("Expected 1 agent, got %d", len(agents))
	}
	if agents[0].Name != "bugbuster-test" {
		t.Errorf("Expected name 'bugbuster-test', got '%s'", agents[0].Name)
	}

	// Check file was created
	agentFile := filepath.Join(dir, "agents", "test-agent-1.json")
	if _, err := os.Stat(agentFile); os.IsNotExist(err) {
		t.Error("Agent file not created on disk")
	}

	// Unregister
	if err := hub.Unregister(); err != nil {
		t.Fatalf("Unregister error: %v", err)
	}

	agents = hub.ListAgents()
	if len(agents) != 0 {
		t.Errorf("Expected 0 agents after unregister, got %d", len(agents))
	}
}

func TestHubSendMessage(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub(dir)
	if err := hub.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	// Register agent-1 (selfID becomes "agent-1")
	profile1 := &AgentProfile{
		ID:           "agent-1",
		Name:         "bugbuster-1",
		Provider:     "openai",
		Model:        "gpt-4o",
		Intelligence: IntelligenceExpert,
	}
	hub.Register(profile1)

	// Also register agent-2 manually (without changing selfID)
	profile2 := &AgentProfile{
		ID:           "agent-2",
		Name:         "bugbuster-2",
		Provider:     "anthropic",
		Model:        "claude-3-opus",
		Intelligence: IntelligenceSuperior,
	}
	hub.mu.Lock()
	profile2.RegisteredAt = time.Now()
	profile2.LastHeartbeat = time.Now()
	hub.agents["agent-2"] = profile2
	hub.mu.Unlock()
	hub.saveAgent(profile2)

	// Send message from agent-1 to agent-2
	if err := hub.SendMessage("agent-2", "Hello, can you check the tests?"); err != nil {
		t.Fatalf("SendMessage error: %v", err)
	}

	// Check message was created
	messages := hub.GetAllMessages(10)
	if len(messages) < 1 {
		t.Errorf("Expected at least 1 message, got %d", len(messages))
	}

	// Find the direct message
	var foundDirect bool
	for _, m := range messages {
		if m.Type == "direct" && m.From == "agent-1" && m.To == "agent-2" {
			foundDirect = true
			if m.Content != "Hello, can you check the tests?" {
				t.Errorf("Expected content 'Hello, can you check the tests?', got '%s'", m.Content)
			}
		}
	}
	if !foundDirect {
		t.Error("Direct message not found")
	}
}

func TestHubBroadcast(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub(dir)
	if err := hub.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	profile := &AgentProfile{
		ID:           "agent-1",
		Name:         "bugbuster-1",
		Provider:     "openai",
		Model:        "gpt-4o",
		Intelligence: IntelligenceExpert,
	}
	hub.Register(profile)

	if err := hub.Broadcast("I broke the tests, please wait!"); err != nil {
		t.Fatalf("Broadcast error: %v", err)
	}

	messages := hub.GetAllMessages(10)
	var foundBroadcast bool
	for _, m := range messages {
		if m.Type == "broadcast" && m.From == "agent-1" {
			foundBroadcast = true
		}
	}
	if !foundBroadcast {
		t.Error("Broadcast message not found")
	}
}

func TestHubAlert(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub(dir)
	if err := hub.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	profile := &AgentProfile{
		ID:           "agent-1",
		Name:         "bugbuster-1",
		Provider:     "openai",
		Model:        "gpt-4o",
		Intelligence: IntelligenceExpert,
	}
	hub.Register(profile)

	if err := hub.Alert("Tests are broken!"); err != nil {
		t.Fatalf("Alert error: %v", err)
	}

	messages := hub.GetAllMessages(10)
	var foundAlert bool
	for _, m := range messages {
		if m.Type == "alert" {
			foundAlert = true
			if m.Content != "⚠️ ALERT from bugbuster-1 (openai/gpt-4o): Tests are broken!" {
				t.Errorf("Unexpected alert content: %s", m.Content)
			}
		}
	}
	if !foundAlert {
		t.Error("Alert message not found")
	}
}

func TestHubUpdateStatus(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub(dir)
	if err := hub.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	profile := &AgentProfile{
		ID:           "agent-1",
		Name:         "bugbuster-1",
		Provider:     "openai",
		Model:        "gpt-4o",
		Intelligence: IntelligenceExpert,
	}
	hub.Register(profile)

	if err := hub.UpdateStatus(StatusWorking, "Fixing auth module"); err != nil {
		t.Fatalf("UpdateStatus error: %v", err)
	}

	agent, ok := hub.GetAgent("agent-1")
	if !ok {
		t.Fatal("Agent not found")
	}
	if agent.Status != StatusWorking {
		t.Errorf("Expected status 'working', got '%s'", agent.Status)
	}
	if agent.CurrentTask != "Fixing auth module" {
		t.Errorf("Expected task 'Fixing auth module', got '%s'", agent.CurrentTask)
	}
}

func TestHubGetHistory(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub(dir)
	if err := hub.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	profile := &AgentProfile{
		ID:           "agent-1",
		Name:         "bugbuster-1",
		Provider:     "openai",
		Model:        "gpt-4o",
		Intelligence: IntelligenceExpert,
	}
	hub.Register(profile)

	hub.SendMessage("agent-1", "Self message")
	hub.Broadcast("Broadcast message")

	history := hub.GetHistory("agent-1", 10)
	if len(history) < 2 {
		t.Errorf("Expected at least 2 messages in history, got %d", len(history))
	}
}

func TestIntelligenceLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected IntelligenceLevel
	}{
		{"1", IntelligenceLow},
		{"low", IntelligenceLow},
		{"2", IntelligenceMedium},
		{"medium", IntelligenceMedium},
		{"3", IntelligenceHigh},
		{"high", IntelligenceHigh},
		{"4", IntelligenceExpert},
		{"expert", IntelligenceExpert},
		{"5", IntelligenceSuperior},
		{"superior", IntelligenceSuperior},
		{"unknown", IntelligenceMedium}, // default
	}

	for _, tt := range tests {
		result := ParseIntelligenceLevel(tt.input)
		if result != tt.expected {
			t.Errorf("ParseIntelligenceLevel(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestIntelligenceLevelString(t *testing.T) {
	tests := []struct {
		level    IntelligenceLevel
		expected string
	}{
		{IntelligenceLow, "low"},
		{IntelligenceMedium, "medium"},
		{IntelligenceHigh, "high"},
		{IntelligenceExpert, "expert"},
		{IntelligenceSuperior, "superior"},
	}

	for _, tt := range tests {
		result := tt.level.String()
		if result != tt.expected {
			t.Errorf("IntelligenceLevel(%d).String() = %q, want %q", tt.level, result, tt.expected)
		}
	}
}

func TestDetectIntelligence(t *testing.T) {
	tests := []struct {
		model    string
		mapping  map[string]string
		expected IntelligenceLevel
	}{
		{"gpt-4o", nil, IntelligenceExpert},
		{"claude-3-opus", nil, IntelligenceSuperior},
		{"claude-3.5-sonnet", nil, IntelligenceExpert},
		{"claude-3-haiku", nil, IntelligenceMedium},
		{"gemini-2.0-flash", nil, IntelligenceMedium},
		{"qwen2.5-72b", nil, IntelligenceExpert},
		{"llama-3.1-8b", nil, IntelligenceMedium},
		{"deepseek-r1", nil, IntelligenceHigh},
		{"unknown-model", nil, IntelligenceMedium},
		{"my-custom-model", map[string]string{"my-custom": "superior"}, IntelligenceSuperior},
		{"gpt-4o", map[string]string{"gpt-4o": "low"}, IntelligenceLow},
	}

	for _, tt := range tests {
		result := DetectIntelligence(tt.model, tt.mapping)
		if result != tt.expected {
			t.Errorf("DetectIntelligence(%q, %v) = %d (%s), want %d (%s)",
				tt.model, tt.mapping, result, result, tt.expected, tt.expected)
		}
	}
}

func TestAgentProfileIsAlive(t *testing.T) {
	tests := []struct {
		name     string
		profile  *AgentProfile
		timeout  time.Duration
		expected bool
	}{
		{
			name: "recent heartbeat",
			profile: &AgentProfile{
				RegisteredAt:     time.Now().Add(-1 * time.Hour),
				LastHeartbeat:    time.Now(),
				HeartbeatSeconds: 30,
			},
			timeout:  5 * time.Minute,
			expected: true,
		},
		{
			name: "stale heartbeat",
			profile: &AgentProfile{
				RegisteredAt:     time.Now().Add(-2 * time.Hour),
				LastHeartbeat:    time.Now().Add(-10 * time.Minute),
				HeartbeatSeconds: 30,
			},
			timeout:  5 * time.Minute,
			expected: false,
		},
		{
			name: "no heartbeat but recent registration",
			profile: &AgentProfile{
				RegisteredAt:     time.Now().Add(-1 * time.Hour),
				LastHeartbeat:    time.Time{},
				HeartbeatSeconds: 0,
			},
			timeout:  5 * time.Minute,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.profile.IsAlive(tt.timeout)
			if result != tt.expected {
				t.Errorf("IsAlive() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFormatAgentList(t *testing.T) {
	agents := []*AgentProfile{
		{
			Name:         "bugbuster-1",
			Provider:     "openai",
			Model:        "gpt-4o",
			Intelligence: IntelligenceExpert,
			Status:       StatusWorking,
			CurrentTask:  "Fixing auth module",
		},
		{
			Name:         "bugbuster-2",
			Provider:     "anthropic",
			Model:        "claude-3-opus",
			Intelligence: IntelligenceSuperior,
			Status:       StatusIdle,
		},
	}

	result := FormatAgentList(agents)
	if len(result) == 0 {
		t.Error("FormatAgentList returned empty string")
	}
	if !contains(result, "bugbuster-1") || !contains(result, "bugbuster-2") {
		t.Error("FormatAgentList missing agent names")
	}
}

func TestFormatMessageHistory(t *testing.T) {
	messages := []*Message{
		{From: "agent-1", To: "agent-2", Type: "direct", Content: "Hello!"},
		{From: "agent-1", To: "", Type: "broadcast", Content: "Tests broken!"},
		{From: "agent-1", To: "", Type: "alert", Content: "⚠️ ALERT: Critical issue!"},
	}

	agents := map[string]*AgentProfile{
		"agent-1": {Name: "bugbuster-1"},
		"agent-2": {Name: "bugbuster-2"},
	}

	result := FormatMessageHistory(messages, agents)
	if len(result) == 0 {
		t.Error("FormatMessageHistory returned empty string")
	}
}

func TestHubPersistence(t *testing.T) {
	dir := t.TempDir()

	// Create and register agent in first hub instance
	hub1 := NewHub(dir)
	if err := hub1.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	profile := &AgentProfile{
		ID:           "persistent-agent",
		Name:         "bugbuster-persistent",
		Provider:     "openai",
		Model:        "gpt-4o",
		Intelligence: IntelligenceExpert,
	}
	hub1.Register(profile)
	hub1.Broadcast("Hello from hub1!")

	// Create second hub instance and verify data persists
	hub2 := NewHub(dir)
	if err := hub2.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	// hub2 should see the agent from hub1
	agents := hub2.ListAgents()
	found := false
	for _, a := range agents {
		if a.ID == "persistent-agent" {
			found = true
			if a.Name != "bugbuster-persistent" {
				t.Errorf("Expected name 'bugbuster-persistent', got '%s'", a.Name)
			}
		}
	}
	if !found {
		t.Error("Agent not found in second hub instance (persistence failed)")
	}
}

func TestHubEmptyList(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub(dir)
	if err := hub.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	agents := hub.ListAgents()
	if len(agents) != 0 {
		t.Errorf("Expected 0 agents, got %d", len(agents))
	}

	result := FormatAgentList(agents)
	if result != "No agents registered in the hub." {
		t.Errorf("Unexpected empty list message: %s", result)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestHubSendRequest(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub(dir)
	if err := hub.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	// Register agent-1 (self)
	profile1 := &AgentProfile{
		ID:           "agent-1",
		Name:         "bugbuster-1",
		Provider:     "openai",
		Model:        "gpt-4o",
		Intelligence: IntelligenceExpert,
	}
	hub.Register(profile1)

	// Register agent-2 manually
	profile2 := &AgentProfile{
		ID:           "agent-2",
		Name:         "bugbuster-2",
		Provider:     "anthropic",
		Model:        "claude-3-opus",
		Intelligence: IntelligenceSuperior,
	}
	hub.mu.Lock()
	profile2.RegisteredAt = time.Now()
	profile2.LastHeartbeat = time.Now()
	hub.agents["agent-2"] = profile2
	hub.mu.Unlock()
	hub.saveAgent(profile2)

	// Send a request
	msg, err := hub.SendRequest("agent-2", "review", "Please review my PR for the auth module", "high")
	if err != nil {
		t.Fatalf("SendRequest error: %v", err)
	}

	if msg.Type != "request" {
		t.Errorf("Expected type 'request', got '%s'", msg.Type)
	}
	if msg.To != "agent-2" {
		t.Errorf("Expected To='agent-2', got '%s'", msg.To)
	}
	if msg.Action != "review" {
		t.Errorf("Expected Action='review', got '%s'", msg.Action)
	}
	if msg.Priority != "high" {
		t.Errorf("Expected Priority='high', got '%s'", msg.Priority)
	}
}

func TestHubRespondToRequest(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub(dir)
	if err := hub.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	// Register agent-1 (self)
	profile1 := &AgentProfile{
		ID:           "agent-1",
		Name:         "bugbuster-1",
		Provider:     "openai",
		Model:        "gpt-4o",
		Intelligence: IntelligenceExpert,
	}
	hub.Register(profile1)

	// Register agent-2 manually
	profile2 := &AgentProfile{
		ID:           "agent-2",
		Name:         "bugbuster-2",
		Provider:     "anthropic",
		Model:        "claude-3-opus",
		Intelligence: IntelligenceSuperior,
	}
	hub.mu.Lock()
	profile2.RegisteredAt = time.Now()
	profile2.LastHeartbeat = time.Now()
	hub.agents["agent-2"] = profile2
	hub.mu.Unlock()
	hub.saveAgent(profile2)

	// agent-1 sends a request to agent-2
	req, err := hub.SendRequest("agent-2", "review", "Please review my PR", "high")
	if err != nil {
		t.Fatalf("SendRequest error: %v", err)
	}

	// Now agent-2 responds (simulate by creating a response hub)
	hub2 := NewHub(dir)
	if err := hub2.Init(); err != nil {
		t.Fatalf("Init hub2 error: %v", err)
	}
	hub2.Register(profile2)

	// Respond to the request
	resp, err := hub2.RespondToRequest(req.ID, "I'll review it right away, give me 5 minutes", true)
	if err != nil {
		t.Fatalf("RespondToRequest error: %v", err)
	}

	if resp.Type != "response" {
		t.Errorf("Expected type 'response', got '%s'", resp.Type)
	}
	if resp.To != "agent-1" {
		t.Errorf("Expected To='agent-1', got '%s'", resp.To)
	}
	if resp.Accepted == nil || !*resp.Accepted {
		t.Error("Expected Accepted=true")
	}
	if resp.ReplyTo != req.ID {
		t.Errorf("Expected ReplyTo='%s', got '%s'", req.ID, resp.ReplyTo)
	}
}

func TestHubGetPendingRequests(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub(dir)
	if err := hub.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	// Register agent-1 (self)
	profile1 := &AgentProfile{
		ID:           "agent-1",
		Name:         "bugbuster-1",
		Provider:     "openai",
		Model:        "gpt-4o",
		Intelligence: IntelligenceExpert,
	}
	hub.Register(profile1)

	// Register agent-2 manually
	profile2 := &AgentProfile{
		ID:           "agent-2",
		Name:         "bugbuster-2",
		Provider:     "anthropic",
		Model:        "claude-3-opus",
		Intelligence: IntelligenceSuperior,
	}
	hub.mu.Lock()
	profile2.RegisteredAt = time.Now()
	profile2.LastHeartbeat = time.Now()
	hub.agents["agent-2"] = profile2
	hub.mu.Unlock()
	hub.saveAgent(profile2)

	// Initially no pending requests
	pending := hub.GetPendingRequests()
	if len(pending) != 0 {
		t.Errorf("Expected 0 pending requests, got %d", len(pending))
	}

	// Send a request from agent-2 to agent-1 (manually create message)
	msg := &Message{
		ID:       generateID(),
		From:     "agent-2",
		To:       "agent-1",
		Type:     "request",
		Content:  "Can you fix the login bug?",
		Action:   "fix",
		Priority: "medium",
	}
	hub.mu.Lock()
	hub.messages = append(hub.messages, msg)
	hub.mu.Unlock()
	hub.saveMessage(msg)

	// Now agent-1 should have 1 pending request
	pending = hub.GetPendingRequests()
	if len(pending) != 1 {
		t.Errorf("Expected 1 pending request, got %d", len(pending))
	}
	if pending[0].Action != "fix" {
		t.Errorf("Expected Action='fix', got '%s'", pending[0].Action)
	}
}

func TestHubGetUnreadMessages(t *testing.T) {
	dir := t.TempDir()
	hub := NewHub(dir)
	if err := hub.Init(); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	profile1 := &AgentProfile{
		ID:           "agent-1",
		Name:         "bugbuster-1",
		Provider:     "openai",
		Model:        "gpt-4o",
		Intelligence: IntelligenceExpert,
	}
	hub.Register(profile1)

	// Initially no unread messages
	unread := hub.GetUnreadMessages()
	if len(unread) != 0 {
		t.Errorf("Expected 0 unread messages, got %d", len(unread))
	}

	// Create a second hub (agent-2) and send a message to agent-1
	hub2 := NewHub(dir)
	if err := hub2.Init(); err != nil {
		t.Fatalf("Init hub2 error: %v", err)
	}
	profile2 := &AgentProfile{
		ID:           "agent-2",
		Name:         "bugbuster-2",
		Provider:     "anthropic",
		Model:        "claude-3-opus",
		Intelligence: IntelligenceSuperior,
	}
	hub2.Register(profile2)

	// agent-2 sends a direct message to agent-1
	if err := hub2.SendMessage("agent-1", "Hey, can you check the tests?"); err != nil {
		t.Fatalf("SendMessage error: %v", err)
	}

	// Now agent-1 should have unread messages (at least 1 from agent-2)
	unread = hub.GetUnreadMessages()
	if len(unread) < 1 {
		t.Errorf("Expected at least 1 unread message, got %d", len(unread))
	}

	// Find the direct message from agent-2
	var foundDirect bool
	for _, m := range unread {
		if m.From == "agent-2" && m.To == "agent-1" && m.Type == "direct" {
			foundDirect = true
		}
	}
	if !foundDirect {
		t.Error("Direct message from agent-2 not found in unread messages")
	}
}