// Package agenthub provides a shared workspace for multiple BugBuster Code agents.
//
// It enables inter-agent communication through:
//   - Agent Registry: each agent registers its profile (name, model, project, role, intelligence level)
//   - Message Board: agents can send direct messages, broadcast, and read each other's history
//   - Shared State: agents can see what others are working on and coordinate
//
// The hub uses file-based storage in .bugbuster/hub/ for persistence across processes.
package agenthub

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// AgentStatus represents the current status of an agent
type AgentStatus string

const (
	StatusIdle      AgentStatus = "idle"
	StatusWorking   AgentStatus = "working"
	StatusWaiting   AgentStatus = "waiting"
	StatusError     AgentStatus = "error"
	StatusCompleted AgentStatus = "completed"
)

// IntelligenceLevel represents the intelligence/capability level of an agent
type IntelligenceLevel int

const (
	IntelligenceLow      IntelligenceLevel = 1 // Small models, limited reasoning
	IntelligenceMedium   IntelligenceLevel = 2 // Mid-range models
	IntelligenceHigh     IntelligenceLevel = 3 // Advanced models
	IntelligenceExpert   IntelligenceLevel = 4 // Top-tier models (GPT-4, Claude Opus)
	IntelligenceSuperior IntelligenceLevel = 5 // Most capable models available
)

// String returns a human-readable label for the intelligence level
func (l IntelligenceLevel) String() string {
	switch l {
	case IntelligenceLow:
		return "low"
	case IntelligenceMedium:
		return "medium"
	case IntelligenceHigh:
		return "high"
	case IntelligenceExpert:
		return "expert"
	case IntelligenceSuperior:
		return "superior"
	default:
		return fmt.Sprintf("level_%d", int(l))
	}
}

// ParseIntelligenceLevel parses a string into IntelligenceLevel
func ParseIntelligenceLevel(s string) IntelligenceLevel {
	switch strings.ToLower(s) {
	case "1", "low":
		return IntelligenceLow
	case "2", "medium":
		return IntelligenceMedium
	case "3", "high":
		return IntelligenceHigh
	case "4", "expert":
		return IntelligenceExpert
	case "5", "superior":
		return IntelligenceSuperior
	default:
		return IntelligenceMedium
	}
}

// AgentTask represents a task in an agent's todo list
type AgentTask struct {
	ID      string `json:"id"`      // Task ID
	Subject string `json:"subject"`  // Task description
	Status  string `json:"status"`   // "pending", "in_progress", "completed"
}

// AgentProfile describes a registered agent in the hub
type AgentProfile struct {
	ID               string            `json:"id"`                // Unique agent ID (session ID)
	Name             string            `json:"name"`              // Display name (e.g., "bugbuster-1")
	Provider         string            `json:"provider"`          // Provider name (e.g., "openai", "anthropic")
	Model            string            `json:"model"`             // Model name (e.g., "gpt-4o", "claude-3-opus")
	Project          string            `json:"project"`           // Working project directory
	Role             string            `json:"role"`              // Agent role (e.g., "coder", "reviewer", "tester")
	Intelligence     IntelligenceLevel `json:"intelligence"`      // Intelligence level (1-5)
	Status           AgentStatus       `json:"status"`            // Current status
	CurrentTask      string            `json:"current_task"`      // What the agent is currently working on
	Tasks            []AgentTask       `json:"tasks"`             // Agent's current task list (shared with hub)
	SystemPrompt     string            `json:"system_prompt"`     // Agent's system prompt (for other agents to see)
	RegisteredAt     time.Time         `json:"registered_at"`     // When the agent registered
	LastHeartbeat    time.Time         `json:"last_heartbeat"`    // Last heartbeat timestamp
	HeartbeatSeconds int               `json:"heartbeat_seconds"` // Heartbeat interval in seconds (0 = no heartbeat)
}

// IsAlive checks if the agent is still alive (heartbeat within timeout)
func (p *AgentProfile) IsAlive(timeout time.Duration) bool {
	if p.HeartbeatSeconds == 0 {
		// No heartbeat configured — check if registered within last 24h
		return time.Since(p.RegisteredAt) < 24*time.Hour
	}
	return time.Since(p.LastHeartbeat) < timeout
}

// Message represents a message between agents
type Message struct {
	ID         string    `json:"id"`          // Unique message ID
	From       string    `json:"from"`        // Sender agent ID
	To         string    `json:"to"`          // Receiver agent ID (empty = broadcast)
	Type       string    `json:"type"`        // Message type: "direct", "broadcast", "alert", "status", "request", "response"
	Content    string    `json:"content"`     // Message content
	Priority   string    `json:"priority"`    // Priority: "low", "normal", "high", "urgent"
	Action     string    `json:"action"`      // Requested action: "do", "redo", "stop", "wait", "review", "test", "fix"
	ReplyTo    string    `json:"reply_to"`    // ID of the message this is a reply to (for request/response)
	Accepted   *bool     `json:"accepted"`    // For responses: whether the request was accepted (nil = no response yet)
	Timestamp  time.Time `json:"timestamp"`   // When the message was sent
	Read       bool      `json:"read"`        // Whether the recipient has read it
}

// Hub is the shared workspace for inter-agent communication
type Hub struct {
	mu       sync.RWMutex
	dir      string           // Base directory for file storage
	agents   map[string]*AgentProfile // Registered agents by ID
	messages []*Message       // Message board (in-memory + file)
	selfID   string           // This agent's ID
}

// NewHub creates a new hub with file-based storage
func NewHub(dir string) *Hub {
	return &Hub{
		dir:      dir,
		agents:   make(map[string]*AgentProfile),
		messages: make([]*Message, 0),
	}
}

// Init initializes the hub directory and loads existing data
func (h *Hub) Init() error {
	if err := os.MkdirAll(h.dir, 0755); err != nil {
		return fmt.Errorf("create hub dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(h.dir, "agents"), 0755); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(h.dir, "messages"), 0755); err != nil {
		return fmt.Errorf("create messages dir: %w", err)
	}
	// Load existing agents and messages from disk
	h.loadFromDisk()
	return nil
}

// Register registers this agent in the hub
func (h *Hub) Register(profile *AgentProfile) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	profile.RegisteredAt = time.Now()
	profile.LastHeartbeat = time.Now()
	h.selfID = profile.ID
	h.agents[profile.ID] = profile

	// Save to disk
	if err := h.saveAgent(profile); err != nil {
		return fmt.Errorf("save agent: %w", err)
	}

	// Broadcast registration
	msg := &Message{
		ID:        generateID(),
		From:      profile.ID,
		To:        "", // broadcast
		Type:      "status",
		Content:   fmt.Sprintf("Agent %s (%s/%s) joined the hub. Role: %s, Intelligence: %s", profile.Name, profile.Provider, profile.Model, profile.Role, profile.Intelligence),
		Timestamp: time.Now(),
	}
	h.messages = append(h.messages, msg)
	h.saveMessage(msg)

	return nil
}

// Unregister removes this agent from the hub
func (h *Hub) Unregister() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.selfID == "" {
		return nil
	}

	profile, ok := h.agents[h.selfID]
	if !ok {
		return nil
	}

	// Broadcast departure
	msg := &Message{
		ID:        generateID(),
		From:      h.selfID,
		To:        "",
		Type:      "status",
		Content:   fmt.Sprintf("Agent %s (%s/%s) left the hub.", profile.Name, profile.Provider, profile.Model),
		Timestamp: time.Now(),
	}
	h.messages = append(h.messages, msg)
	h.saveMessage(msg)

	// Remove from memory and disk
	delete(h.agents, h.selfID)
	agentFile := filepath.Join(h.dir, "agents", h.selfID+".json")
	os.Remove(agentFile)

	h.selfID = ""
	return nil
}

// UpdateStatus updates the agent's current status and task
func (h *Hub) UpdateStatus(status AgentStatus, task string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	profile, ok := h.agents[h.selfID]
	if !ok {
		return fmt.Errorf("agent not registered")
	}

	oldStatus := profile.Status
	profile.Status = status
	profile.CurrentTask = task
	profile.LastHeartbeat = time.Now()

	if err := h.saveAgent(profile); err != nil {
		return err
	}

	// Notify others on status change
	if oldStatus != status {
		msg := &Message{
			ID:        generateID(),
			From:      h.selfID,
			To:        "",
			Type:      "status",
			Content:   fmt.Sprintf("Agent %s: %s → %s (%s)", profile.Name, oldStatus, status, task),
			Timestamp: time.Now(),
		}
		h.messages = append(h.messages, msg)
		h.saveMessage(msg)
	}

	return nil
}

// UpdateTasks updates the agent's shared task list
func (h *Hub) UpdateTasks(tasks []AgentTask) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	profile, ok := h.agents[h.selfID]
	if !ok {
		return fmt.Errorf("agent not registered")
	}

	profile.Tasks = tasks
	profile.LastHeartbeat = time.Now()
	return h.saveAgent(profile)
}

// Heartbeat updates the agent's last heartbeat time
func (h *Hub) Heartbeat() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	profile, ok := h.agents[h.selfID]
	if !ok {
		return fmt.Errorf("agent not registered")
	}

	profile.LastHeartbeat = time.Now()
	return h.saveAgent(profile)
}

// SendMessage sends a direct message to another agent
func (h *Hub) SendMessage(toAgentID, content string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.selfID == "" {
		return fmt.Errorf("agent not registered")
	}

	// Resolve agent ID (supports partial IDs and names)
	resolvedID, err := h.resolveAgentIDUnlocked(toAgentID)
	if err != nil {
		return err
	}

	msg := &Message{
		ID:        generateID(),
		From:      h.selfID,
		To:        resolvedID,
		Type:      "direct",
		Content:   content,
		Timestamp: time.Now(),
	}
	h.messages = append(h.messages, msg)
	h.saveMessage(msg)
	return nil
}

// Broadcast sends a message to all agents
func (h *Hub) Broadcast(content string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.selfID == "" {
		return fmt.Errorf("agent not registered")
	}

	msg := &Message{
		ID:        generateID(),
		From:      h.selfID,
		To:        "",
		Type:      "broadcast",
		Content:   content,
		Timestamp: time.Now(),
	}
	h.messages = append(h.messages, msg)
	h.saveMessage(msg)
	return nil
}

// Alert sends an urgent alert to all agents (e.g., "I broke the tests!")
func (h *Hub) Alert(content string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.selfID == "" {
		return fmt.Errorf("agent not registered")
	}

	profile := h.agents[h.selfID]

	msg := &Message{
		ID:        generateID(),
		From:      h.selfID,
		To:        "",
		Type:      "alert",
		Content:   fmt.Sprintf("⚠️ ALERT from %s (%s/%s): %s", profile.Name, profile.Provider, profile.Model, content),
		Timestamp: time.Now(),
	}
	h.messages = append(h.messages, msg)
	h.saveMessage(msg)
	return nil
}

// SendRequest sends a task request to another agent.
// action: "do" (do this task), "redo" (redo/rewrite), "stop" (stop what you're doing),
// "wait" (wait until I'm done), "review" (review my code), "test" (run tests), "fix" (fix this bug)
func (h *Hub) SendRequest(toAgentID, action, content, priority string) (*Message, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.selfID == "" {
		return nil, fmt.Errorf("agent not registered")
	}

	// Resolve agent ID (supports partial IDs and names)
	resolvedID, err := h.resolveAgentIDUnlocked(toAgentID)
	if err != nil {
		return nil, err
	}

	if priority == "" {
		priority = "normal"
	}

	msg := &Message{
		ID:        generateID(),
		From:      h.selfID,
		To:        resolvedID,
		Type:      "request",
		Content:   content,
		Priority:  priority,
		Action:    action,
		Timestamp: time.Now(),
	}
	h.messages = append(h.messages, msg)
	h.saveMessage(msg)
	return msg, nil
}

// RespondToRequest sends a response to a request.
// accepted: true = accept the request, false = decline
// content: explanation or result
func (h *Hub) RespondToRequest(requestID, content string, accepted bool) (*Message, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.selfID == "" {
		return nil, fmt.Errorf("agent not registered")
	}

	// Find the original request
	var original *Message
	for _, m := range h.messages {
		if m.ID == requestID {
			original = m
			break
		}
	}
	if original == nil {
		return nil, fmt.Errorf("request '%s' not found", requestID)
	}
	if original.Type != "request" {
		return nil, fmt.Errorf("message '%s' is not a request", requestID)
	}
	if original.To != h.selfID {
		return nil, fmt.Errorf("request was sent to agent '%s', not to you ('%s')", original.To, h.selfID)
	}

	acceptedBool := accepted
	msg := &Message{
		ID:       generateID(),
		From:     h.selfID,
		To:       original.From,
		Type:     "response",
		Content:  content,
		Priority: original.Priority,
		ReplyTo:  requestID,
		Accepted: &acceptedBool,
		Timestamp: time.Now(),
	}
	h.messages = append(h.messages, msg)
	h.saveMessage(msg)

	// Mark original request as read
	original.Read = true

	return msg, nil
}

// GetPendingRequests returns unresponded requests addressed to this agent
func (h *Hub) GetPendingRequests() []*Message {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Refresh messages from disk
	h.loadMessagesFromDisk()

	var result []*Message
	for _, m := range h.messages {
		if m.To == h.selfID && m.Type == "request" && m.Accepted == nil {
			result = append(result, m)
		}
	}
	return result
}

// GetUnreadMessages returns all unread messages for this agent
func (h *Hub) GetUnreadMessages() []*Message {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Refresh messages from disk
	h.loadMessagesFromDisk()

	var result []*Message
	for _, m := range h.messages {
		if !m.Read && (m.To == h.selfID || m.To == "") && m.From != h.selfID {
			result = append(result, m)
		}
	}
	return result
}

// ListAgents returns all registered agents (sorted by intelligence, then name)
func (h *Hub) ListAgents() []*AgentProfile {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Refresh from disk first
	h.loadFromDisk()

	result := make([]*AgentProfile, 0, len(h.agents))
	for _, a := range h.agents {
		result = append(result, a)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Intelligence != result[j].Intelligence {
			return result[i].Intelligence > result[j].Intelligence // Higher intelligence first
		}
		return result[i].Name < result[j].Name
	})
	return result
}

// GetAgent returns a specific agent's profile
func (h *Hub) GetAgent(id string) (*AgentProfile, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Try to load from disk if not in memory
	if _, ok := h.agents[id]; !ok {
		h.loadAgentFromDisk(id)
	}

	a, ok := h.agents[id]
	if !ok {
		return nil, false
	}
	return a, true
}

// GetHistory returns messages involving a specific agent (sent or received)
func (h *Hub) GetHistory(agentID string, limit int) []*Message {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Refresh messages from disk
	h.loadMessagesFromDisk()

	var result []*Message
	for i := len(h.messages) - 1; i >= 0; i-- {
		m := h.messages[i]
		if m.From == agentID || m.To == agentID || m.To == "" {
			result = append(result, m)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result
}

// GetMessages returns unread messages for this agent
func (h *Hub) GetMessages(since time.Time) []*Message {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Refresh messages from disk
	h.loadMessagesFromDisk()

	var result []*Message
	for _, m := range h.messages {
		if m.Timestamp.After(since) && (m.To == h.selfID || m.To == "" || m.From == h.selfID) {
			result = append(result, m)
		}
	}
	return result
}

// GetAllMessages returns all messages (for display purposes)
func (h *Hub) GetAllMessages(limit int) []*Message {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Refresh messages from disk
	h.loadMessagesFromDisk()

	start := 0
	if limit > 0 && len(h.messages) > limit {
		start = len(h.messages) - limit
	}
	return h.messages[start:]
}

// MarkRead marks messages as read
func (h *Hub) MarkRead(messageIDs []string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	idSet := make(map[string]bool, len(messageIDs))
	for _, id := range messageIDs {
		idSet[id] = true
	}

	for _, m := range h.messages {
		if idSet[m.ID] && m.To == h.selfID {
			m.Read = true
		}
	}
}

// SelfID returns this agent's ID
func (h *Hub) SelfID() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.selfID
}

// ResolveAgentID resolves an agent ID from a possibly partial or name-based identifier.
// It tries: exact ID match, partial ID prefix match, exact name match, partial name match.
func (h *Hub) ResolveAgentID(idOrName string) (string, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.resolveAgentIDUnlocked(idOrName)
}

// --- File-based persistence ---

func (h *Hub) saveAgent(profile *AgentProfile) error {
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(h.dir, "agents", profile.ID+".json")
	return os.WriteFile(path, data, 0644)
}

func (h *Hub) saveMessage(msg *Message) error {
	data, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(h.dir, "messages", msg.ID+".json")
	return os.WriteFile(path, data, 0644)
}

func (h *Hub) loadFromDisk() {
	// Load agents
	agentsDir := filepath.Join(h.dir, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if _, exists := h.agents[id]; !exists {
			h.loadAgentFromDisk(id)
		}
	}

	// Load messages
	h.loadMessagesFromDisk()
}

func (h *Hub) loadAgentFromDisk(id string) {
	path := filepath.Join(h.dir, "agents", id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var profile AgentProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return
	}
	h.agents[id] = &profile
}

func (h *Hub) loadMessagesFromDisk() {
	msgsDir := filepath.Join(h.dir, "messages")
	entries, err := os.ReadDir(msgsDir)
	if err != nil {
		return
	}

	// Track which message IDs we already have
	existingIDs := make(map[string]bool, len(h.messages))
	for _, m := range h.messages {
		existingIDs[m.ID] = true
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		msgID := strings.TrimSuffix(entry.Name(), ".json")
		if existingIDs[msgID] {
			continue
		}

		data, err := os.ReadFile(filepath.Join(msgsDir, entry.Name()))
		if err != nil {
			continue
		}
		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		h.messages = append(h.messages, &msg)
		existingIDs[msg.ID] = true
	}

	// Sort by timestamp
	sort.Slice(h.messages, func(i, j int) bool {
		return h.messages[i].Timestamp.Before(h.messages[j].Timestamp)
	})
}

// resolveAgentID resolves an agent ID from a possibly partial or name-based identifier.
// It tries: exact ID match, partial ID prefix match, exact name match, partial name match.
func (h *Hub) resolveAgentID(idOrName string) (string, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Refresh from disk
	h.loadFromDisk()

	return h.resolveAgentIDUnlocked(idOrName)
}

// resolveAgentIDUnlocked resolves agent ID without locking (caller must hold lock).
func (h *Hub) resolveAgentIDUnlocked(idOrName string) (string, error) {

	// 1. Exact ID match
	if _, ok := h.agents[idOrName]; ok {
		return idOrName, nil
	}

	// 2. Partial ID prefix match (e.g., "sess_202" matches "sess_20260722_072741_31b8047b")
	var matches []string
	for id := range h.agents {
		if strings.HasPrefix(id, idOrName) {
			matches = append(matches, id)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("ambiguous agent ID '%s' matches %d agents: %s", idOrName, len(matches), strings.Join(matches, ", "))
	}

	// 3. Exact name match
	for id, a := range h.agents {
		if a.Name == idOrName {
			return id, nil
		}
	}

	// 4. Partial name match
	for id, a := range h.agents {
		if strings.Contains(a.Name, idOrName) {
			matches = append(matches, id)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("ambiguous agent name '%s' matches %d agents: %s", idOrName, len(matches), strings.Join(matches, ", "))
	}

	// 5. Case-insensitive search
	idLower := strings.ToLower(idOrName)
	for id, a := range h.agents {
		if strings.ToLower(a.Name) == idLower || strings.ToLower(a.ID) == idLower {
			return id, nil
		}
	}

	return "", fmt.Errorf("agent '%s' not found", idOrName)
}

// generateID generates a unique message/agent ID
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// DetectIntelligence determines the intelligence level based on model name
// and optional model_intelligence mapping from config.
func DetectIntelligence(model string, modelIntelligence map[string]string) IntelligenceLevel {
	// Check explicit mapping first
	if modelIntelligence != nil {
		if level, ok := modelIntelligence[model]; ok {
			return ParseIntelligenceLevel(level)
		}
		// Try partial match (e.g., "gpt-4o" matches "gpt-4*")
		for pattern, level := range modelIntelligence {
			if strings.Contains(model, pattern) {
				return ParseIntelligenceLevel(level)
			}
		}
	}

	// Auto-detect from model name
	modelLower := strings.ToLower(model)

	// Superior (5) — most capable models
	superiorPatterns := []string{"opus", "o3", "o1-pro", "ultra", "max"}
	for _, p := range superiorPatterns {
		if strings.Contains(modelLower, p) {
			return IntelligenceSuperior
		}
	}

	// Expert (4) — top-tier models
	expertPatterns := []string{"gpt-4", "claude-3.5-sonnet", "claude-3-sonnet", "gemini-1.5-pro", "llama-3.1-405", "llama-3.1-70b", "qwen2.5-72b"}
	for _, p := range expertPatterns {
		if strings.Contains(modelLower, p) {
			return IntelligenceExpert
		}
	}

	// Medium (2) — mid-range models (check BEFORE high/expert to catch "flash" in "gemini-2.0-flash")
	mediumPatterns := []string{"haiku", "mini", "flash", "small", "7b", "8b", "9b", "13b", "14b"}
	for _, p := range mediumPatterns {
		if strings.Contains(modelLower, p) {
			return IntelligenceMedium
		}
	}

	// High (3) — advanced models
	highPatterns := []string{"sonnet", "mistral-large", "deepseek-r1", "gemini-2."}
	for _, p := range highPatterns {
		if strings.Contains(modelLower, p) {
			return IntelligenceHigh
		}
	}

	// Low (1) — small models
	lowPatterns := []string{"1b", "2b", "3b", "tiny", "micro"}
	for _, p := range lowPatterns {
		if strings.Contains(modelLower, p) {
			return IntelligenceLow
		}
	}

	// Default to medium
	return IntelligenceMedium
}

// FormatAgentList returns a formatted string listing all agents
func FormatAgentList(agents []*AgentProfile) string {
	if len(agents) == 0 {
		return "No agents registered in the hub."
	}

	var sb strings.Builder
	sb.WriteString("🤖 Agent Hub — Registered Agents\n")
	sb.WriteString(strings.Repeat("─", 60) + "\n")

	for _, a := range agents {
		intelligence := strings.Repeat("★", int(a.Intelligence)) + strings.Repeat("☆", 5-int(a.Intelligence))
		sb.WriteString(fmt.Sprintf("  %-20s  %s/%s  [%s]  %s\n",
			a.Name, a.Provider, a.Model, intelligence, a.Status))
		sb.WriteString(fmt.Sprintf("  ID: %s\n", a.ID))
		if a.CurrentTask != "" {
			sb.WriteString(fmt.Sprintf("    Task: %s\n", a.CurrentTask))
		}
		if len(a.Tasks) > 0 {
			sb.WriteString("    Tasks:\n")
			for _, t := range a.Tasks {
				statusIcon := "⬜"
				switch t.Status {
				case "in_progress":
					statusIcon = "🔄"
				case "completed":
					statusIcon = "✅"
				}
				sb.WriteString(fmt.Sprintf("      %s %s: %s\n", statusIcon, t.ID, t.Subject))
			}
		}
		if a.Project != "" {
			sb.WriteString(fmt.Sprintf("    Project: %s\n", a.Project))
		}
	}

	return sb.String()
}

// FormatMessageHistory returns a formatted string of messages
func FormatMessageHistory(messages []*Message, agents map[string]*AgentProfile) string {
	if len(messages) == 0 {
		return "No messages in the hub."
	}

	var sb strings.Builder
	sb.WriteString("💬 Agent Hub — Message History\n")
	sb.WriteString(strings.Repeat("─", 60) + "\n")

	for _, m := range messages {
		fromName := m.From
		if a, ok := agents[m.From]; ok {
			fromName = a.Name
		}

		switch m.Type {
		case "direct":
			toName := m.To
			if a, ok := agents[m.To]; ok {
				toName = a.Name
			}
			sb.WriteString(fmt.Sprintf("  📨 %s → %s: %s\n", fromName, toName, m.Content))
		case "broadcast":
			sb.WriteString(fmt.Sprintf("  📢 %s (broadcast): %s\n", fromName, m.Content))
		case "alert":
			sb.WriteString(fmt.Sprintf("  %s\n", m.Content))
		case "status":
			sb.WriteString(fmt.Sprintf("  🔹 %s\n", m.Content))
		case "request":
			toName := m.To
			if a, ok := agents[m.To]; ok {
				toName = a.Name
			}
			priorityIcon := "📋"
			if m.Priority == "urgent" {
				priorityIcon = "🔴"
			} else if m.Priority == "high" {
				priorityIcon = "🟠"
			}
			actionLabel := m.Action
			if actionLabel == "" {
				actionLabel = "task"
			}
			sb.WriteString(fmt.Sprintf("  %s %s → %s [%s] %s: %s\n", priorityIcon, fromName, toName, actionLabel, m.Priority, m.Content))
		case "response":
			toName := m.To
			if a, ok := agents[m.To]; ok {
				toName = a.Name
			}
			acceptIcon := "✅"
			if m.Accepted != nil && !*m.Accepted {
				acceptIcon = "❌"
			}
			sb.WriteString(fmt.Sprintf("  %s %s → %s: %s\n", acceptIcon, fromName, toName, m.Content))
		}
	}

	return sb.String()
}