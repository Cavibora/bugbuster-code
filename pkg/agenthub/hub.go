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
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
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
	PID              int               `json:"pid"`               // Process ID for sending signals (SIGUSR1)
}

// IsAlive checks if the agent is still alive (heartbeat within timeout)
func (p *AgentProfile) IsAlive(timeout time.Duration) bool {
	if p.HeartbeatSeconds == 0 {
		// No heartbeat configured — check if registered within last 24h
		return time.Since(p.RegisteredAt) < 24*time.Hour
	}
	return time.Since(p.LastHeartbeat) < timeout
}

// MessageStatus represents the delivery/read status of a message
type MessageStatus string

const (
	MsgStatusSent      MessageStatus = "sent"       // Message sent, not yet delivered
	MsgStatusDelivered MessageStatus = "delivered"   // Message delivered to recipient
	MsgStatusRead      MessageStatus = "read"        // Recipient read the message
	MsgStatusAcked     MessageStatus = "acked"       // Recipient acknowledged (took note)
	MsgStatusReplied   MessageStatus = "replied"     // Recipient replied to this message
	MsgStatusIgnored   MessageStatus = "ignored"     // Recipient ignored this message
	MsgStatusDeleted   MessageStatus = "deleted"      // Message deleted by recipient
)

// MessageComment represents a comment added to a message
type MessageComment struct {
	AgentID   string    `json:"agent_id"`   // Who added the comment
	Content   string    `json:"content"`    // Comment text
	Timestamp time.Time `json:"timestamp"`  // When the comment was added
}

// Message represents a message between agents
type Message struct {
	ID         string           `json:"id"`          // Unique message ID
	From       string           `json:"from"`         // Sender agent ID
	To         string           `json:"to"`           // Receiver agent ID (empty = broadcast)
	Type       string           `json:"type"`         // Message type: "direct", "broadcast", "alert", "status", "request", "response"
	Content    string           `json:"content"`      // Message content
	Priority   string           `json:"priority"`     // Priority: "low", "normal", "high", "urgent"
	Action     string           `json:"action"`       // Requested action: "do", "redo", "stop", "wait", "review", "test", "fix"
	ReplyTo    string           `json:"reply_to"`     // ID of the message this is a reply to (for request/response)
	Accepted   *bool            `json:"accepted"`     // For responses: whether the request was accepted (nil = no response yet)
	Timestamp  time.Time        `json:"timestamp"`    // When the message was sent
	Read       bool             `json:"read"`         // Whether the recipient has read it
	// --- New fields: status tracking & comments ---
	Status        MessageStatus    `json:"status"`        // Current status of the message
	StatusHistory []StatusChange   `json:"status_history"` // History of status changes
	Comments      []MessageComment `json:"comments"`      // Comments added by agents
	EditedAt      *time.Time       `json:"edited_at"`     // When the content was last edited (nil = not edited)
	EditedBy      string           `json:"edited_by"`     // Who edited the content (agent ID)
	OriginalContent string         `json:"original_content"` // Original content before editing (empty = not edited)
}

// StatusChange represents a status change in a message's history
type StatusChange struct {
	Status    MessageStatus `json:"status"`     // New status
	ChangedBy string        `json:"changed_by"` // Who changed the status (agent ID)
	ChangedAt time.Time     `json:"changed_at"` // When the status was changed
	Note      string        `json:"note"`       // Optional note about the change
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

	// Clean up stale agents (dead processes, expired heartbeats)
	h.CleanupStaleAgents()

	return nil
}

// CleanupStaleAgents removes agents whose processes are no longer running
// or whose heartbeats have expired. This prevents ghost agents from
// accumulating in the hub after crashes or disconnects.
// It does NOT remove the current agent (selfID).
func (h *Hub) CleanupStaleAgents() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	var toRemove []string

	for id, agent := range h.agents {
		// Never remove self
		if id == h.selfID {
			continue
		}

		stale := false
		reason := ""

		// Check 1: If agent has a PID, check if the process is still alive
		if agent.PID != 0 {
			process, err := os.FindProcess(agent.PID)
			if err != nil {
				stale = true
				reason = fmt.Sprintf("process lookup failed (pid %d)", agent.PID)
			} else {
				// On Unix, FindProcess always succeeds. Send signal 0 to check existence.
				if err := process.Signal(syscall.Signal(0)); err != nil {
					stale = true
					reason = fmt.Sprintf("process %d is dead: %v", agent.PID, err)
				}
			}
		}

		// Check 2: Heartbeat timeout (if heartbeat is configured)
		if !stale && agent.HeartbeatSeconds > 0 {
			heartbeatTimeout := time.Duration(agent.HeartbeatSeconds*3) * time.Second
			if heartbeatTimeout < 30*time.Second {
				heartbeatTimeout = 30 * time.Second
			}
			if time.Since(agent.LastHeartbeat) > heartbeatTimeout {
				stale = true
				reason = fmt.Sprintf("heartbeat expired (last: %v, timeout: %v)", agent.LastHeartbeat, heartbeatTimeout)
			}
		}

		// Check 3: No heartbeat configured and registered more than 24h ago
		if !stale && agent.HeartbeatSeconds == 0 && agent.PID == 0 {
			if time.Since(agent.RegisteredAt) > 24*time.Hour {
				stale = true
				reason = "no PID, no heartbeat, registered >24h ago"
			}
		}

		if stale {
			toRemove = append(toRemove, id)
			// Broadcast departure
			msg := &Message{
				ID:        generateID(),
				From:      id,
				To:        "",
				Type:      "status",
				Content:   fmt.Sprintf("Agent %s (%s/%s) removed from hub (stale: %s)", agent.Name, agent.Provider, agent.Model, reason),
				Timestamp: time.Now(),
			}
			h.messages = append(h.messages, msg)
			h.saveMessage(msg)
		}
	}

	// Remove stale agents
	for _, id := range toRemove {
		delete(h.agents, id)
		agentFile := filepath.Join(h.dir, "agents", id+".json")
		os.Remove(agentFile)
	}

	return len(toRemove)
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

	// Notify others on status change — save status message for history
	// but do NOT send SIGUSR1 to other agents. Status changes are informational
	// and will be visible via hub_list/hub_check when explicitly requested.
	// This prevents context pollution and model freezes from status spam.
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
		// No NotifyAllAgents() — status changes don't need immediate attention
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

	// Notify the target agent (outside lock — NotifyAgent has its own lock)
	go h.NotifyAgent(resolvedID)

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

	// Notify all agents
	go h.NotifyAllAgents()

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

	// Notify all agents
	go h.NotifyAllAgents()

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

	// Notify the target agent
	go h.NotifyAgent(resolvedID)

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

	// Notify the requesting agent about the response
	go h.NotifyAgent(original.From)

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
// GetAgents returns a copy of the agents map for read-only access
func (h *Hub) GetAgents() map[string]*AgentProfile {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make(map[string]*AgentProfile, len(h.agents))
	for id, a := range h.agents {
		result[id] = a
	}
	return result
}

// GetSelfID returns this agent's ID
func (h *Hub) GetSelfID() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.selfID
}

// GetAllMessages returns all messages (with limit, 0 = all)
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

// DrainUnreadMessages returns all unread messages for this agent and marks them as read.
// This is used by the agent loop to inject hub messages as user messages.
// Also updates message status to "read" for backward compatibility with old messages
// that have empty/sent/delivered status.
func (h *Hub) DrainUnreadMessages() []*Message {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Refresh messages from disk
	h.loadMessagesFromDisk()

	var result []*Message
	for _, m := range h.messages {
		if !m.Read && (m.To == h.selfID || m.To == "") && m.From != h.selfID {
			result = append(result, m)
			m.Read = true
			// Backward compatibility: upgrade old messages with empty/sent/delivered status to "read"
			if m.Status == "" || m.Status == MsgStatusSent || m.Status == MsgStatusDelivered {
				m.Status = MsgStatusRead
				m.StatusHistory = append(m.StatusHistory, StatusChange{
					Status:    MsgStatusRead,
					ChangedBy: h.selfID,
					ChangedAt: time.Now(),
				})
			}
		}
	}

	return result
}

// DrainAndFormatMessages returns all unread messages formatted as user text
// and marks them as read. This is used by the agent loop to inject hub messages
// as user messages when SIGUSR1 is received.
// Messages with status "deleted", "ignored", or already "replied"/"acked" are skipped
// to prevent context window pollution (context sclerosis).
func (h *Hub) DrainAndFormatMessages() []string {
	messages := h.DrainUnreadMessages()
	if len(messages) == 0 {
		return nil
	}

	// Get agents map for name resolution
	h.mu.RLock()
	agents := make(map[string]*AgentProfile, len(h.agents))
	for id, a := range h.agents {
		agents[id] = a
	}
	h.mu.RUnlock()

	result := make([]string, 0, len(messages))
	for _, m := range messages {
	// Skip status messages — they are informational only (agent joined, left, changed status)
	// and should NOT be injected into the model's context to prevent context pollution.
	// Status messages are visible via hub_list/hub_check tools when the model explicitly asks.
	if m.Type == "status" {
		continue
	}
	// Skip deleted messages — they're gone
	if m.Status == MsgStatusDeleted {
		continue
	}
	// Skip messages that were already acknowledged or replied to
	// (prevents context sclerosis — re-injecting old messages)
	if m.Status == MsgStatusAcked || m.Status == MsgStatusReplied || m.Status == MsgStatusIgnored {
		continue
	}
		result = append(result, FormatMessageForInjection(m, agents))
	}
	return result
}

// FormatMessageForInjection formats a message for injection as a user message.
// Includes status indicator for the recipient's benefit.
func FormatMessageForInjection(m *Message, agents map[string]*AgentProfile) string {
	fromName := m.From
	if a, ok := agents[m.From]; ok {
		fromName = a.Name
	}

	// Status suffix for context
	statusNote := ""
	if m.Status != "" && m.Status != MsgStatusSent {
		statusNote = fmt.Sprintf(" [%s]", string(m.Status))
	}

	switch m.Type {
	case "direct":
		return fmt.Sprintf("📨 Message from %s%s: %s", fromName, statusNote, m.Content)
	case "broadcast":
		return fmt.Sprintf("📢 Broadcast from %s%s: %s", fromName, statusNote, m.Content)
	case "alert":
		return m.Content
	case "request":
		actionLabel := m.Action
		if actionLabel == "" {
			actionLabel = "task"
		}
		return fmt.Sprintf("📋 Request from %s [%s, priority: %s]%s: %s", fromName, actionLabel, m.Priority, statusNote, m.Content)
	case "response":
		acceptIcon := "✅ accepted"
		if m.Accepted != nil && !*m.Accepted {
			acceptIcon = "❌ declined"
		}
		return fmt.Sprintf("%s Response from %s (%s): %s", acceptIcon, fromName, m.ReplyTo, m.Content)
	case "status":
		return fmt.Sprintf("🔹 %s", m.Content)
	default:
		return fmt.Sprintf("💬 %s%s: %s", fromName, statusNote, m.Content)
	}
}

// SelfID returns this agent's ID
func (h *Hub) SelfID() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.selfID
}

// NotifyAgent sends a SIGUSR1 signal to the target agent's process.
// This wakes up the agent so it can check for new hub messages.
// If the agent has no PID or the process doesn't exist, it's a no-op.
func (h *Hub) NotifyAgent(agentID string) error {
	h.mu.RLock()
	agent, ok := h.agents[agentID]
	h.mu.RUnlock()

	if !ok {
		// Try loading from disk
		h.mu.RLock()
		h.loadAgentFromDisk(agentID)
		agent, ok = h.agents[agentID]
		h.mu.RUnlock()
	}

	if !ok || agent.PID == 0 {
		return nil // no PID, nothing to notify
	}

	// Send SIGUSR1 to the target process
	process, err := os.FindProcess(agent.PID)
	if err != nil {
		return fmt.Errorf("find process %d: %w", agent.PID, err)
	}
	if err := process.Signal(syscall.SIGUSR1); err != nil {
		// Process might not exist anymore, that's OK
		return nil
	}
	return nil
}

// NotifyAllAgents sends SIGUSR1 to all registered agents except self.
// Used for broadcasts and alerts.
func (h *Hub) NotifyAllAgents() {
	h.mu.RLock()
	ids := make([]string, 0, len(h.agents))
	for id := range h.agents {
		if id != h.selfID {
			ids = append(ids, id)
		}
	}
	h.mu.RUnlock()

	for _, id := range ids {
		h.NotifyAgent(id)
	}
}

// HubSignalHandler sets up a SIGUSR1 handler and returns a channel that receives
// notification when the agent should check for new hub messages.
// The channel receives true each time SIGUSR1 is received.
func HubSignalHandler() <-chan bool {
	ch := make(chan bool, 16)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR1)

	go func() {
		for range sigCh {
			select {
			case ch <- true:
			default:
				// Channel full, signal already pending
			}
		}
	}()

	return ch
}

// UpdateMessageStatus changes the status of a message and records the change in history.
// Only the recipient can change the status (or the sender for "sent" → "delivered").
func (h *Hub) UpdateMessageStatus(messageID string, status MessageStatus, note string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	msg, err := h.findMessageUnlocked(messageID)
	if err != nil {
		return err
	}

	// Verify this agent is involved in the message
	if msg.To != h.selfID && msg.From != h.selfID {
		return fmt.Errorf("you are not a participant in this message")
	}

	// Record status change
	change := StatusChange{
		Status:    status,
		ChangedBy: h.selfID,
		ChangedAt: time.Now(),
		Note:      note,
	}
	msg.StatusHistory = append(msg.StatusHistory, change)
	msg.Status = status

	// Also update Read flag for backward compatibility
	if status == MsgStatusRead || status == MsgStatusAcked || status == MsgStatusReplied {
		msg.Read = true
	}

	// Save to disk
	h.saveMessage(msg)

	// Notify the other agent about status change
	otherAgentID := msg.From
	if msg.From == h.selfID {
		otherAgentID = msg.To
	}
	if otherAgentID != "" && otherAgentID != h.selfID {
		go h.NotifyAgent(otherAgentID)
	}

	return nil
}

// AddComment adds a comment to a message.
func (h *Hub) AddComment(messageID, content string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	msg, err := h.findMessageUnlocked(messageID)
	if err != nil {
		return err
	}

	// Verify this agent is involved in the message
	if msg.To != h.selfID && msg.From != h.selfID {
		return fmt.Errorf("you are not a participant in this message")
	}

	comment := MessageComment{
		AgentID:   h.selfID,
		Content:   content,
		Timestamp: time.Now(),
	}
	msg.Comments = append(msg.Comments, comment)

	// Save to disk
	h.saveMessage(msg)

	// Notify the other agent
	otherAgentID := msg.From
	if msg.From == h.selfID {
		otherAgentID = msg.To
	}
	if otherAgentID != "" && otherAgentID != h.selfID {
		go h.NotifyAgent(otherAgentID)
	}

	return nil
}

// EditMessage edits the content of a message. Only the sender can edit.
// The original content is preserved in OriginalContent.
func (h *Hub) EditMessage(messageID, newContent string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	msg, err := h.findMessageUnlocked(messageID)
	if err != nil {
		return err
	}

	// Only the sender can edit
	if msg.From != h.selfID {
		return fmt.Errorf("only the sender can edit a message")
	}

	// Preserve original content
	if msg.OriginalContent == "" {
		msg.OriginalContent = msg.Content
	}
	msg.Content = newContent
	now := time.Now()
	msg.EditedAt = &now
	msg.EditedBy = h.selfID

	// Save to disk
	h.saveMessage(msg)

	// Notify the recipient
	if msg.To != "" && msg.To != h.selfID {
		go h.NotifyAgent(msg.To)
	} else if msg.To == "" {
		// Broadcast — notify all
		go h.NotifyAllAgents()
	}

	return nil
}

// DeleteMessage soft-deletes a message (marks status as "deleted").
// The recipient can delete messages sent to them.
// The sender can delete their own messages.
func (h *Hub) DeleteMessage(messageID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	msg, err := h.findMessageUnlocked(messageID)
	if err != nil {
		return err
	}

	// Verify this agent is involved
	if msg.To != h.selfID && msg.From != h.selfID {
		return fmt.Errorf("you are not a participant in this message")
	}

	// Mark as deleted
	msg.Status = MsgStatusDeleted
	msg.StatusHistory = append(msg.StatusHistory, StatusChange{
		Status:    MsgStatusDeleted,
		ChangedBy: h.selfID,
		ChangedAt: time.Now(),
		Note:      "message deleted",
	})

	// Save to disk
	h.saveMessage(msg)

	return nil
}

// findMessageUnlocked finds a message by ID (caller must hold lock)
func (h *Hub) findMessageUnlocked(messageID string) (*Message, error) {
	for _, m := range h.messages {
		if m.ID == messageID {
			return m, nil
		}
	}
	// Try loading from disk
	h.loadMessagesFromDisk()
	for _, m := range h.messages {
		if m.ID == messageID {
			return m, nil
		}
	}
	return nil, fmt.Errorf("message '%s' not found", messageID)
}

// ResolveAgentID resolves an agent ID from a possibly partial or name-based identifier.
// It tries: exact ID match, partial ID prefix match, exact name match, partial name match.
func (h *Hub) ResolveAgentID(idOrName string) (string, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Refresh from disk
	h.loadFromDisk()

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

// FormatMessagesForUser returns a detailed formatted string of all messages for the /agent_messages command.
// Shows status, comments, edit history, and allows the user to see the full picture.
func FormatMessagesForUser(messages []*Message, agents map[string]*AgentProfile, selfID string) string {
	if len(messages) == 0 {
		return "📭 No messages in the hub."
	}

	var sb strings.Builder
	sb.WriteString("📬 Agent Hub — Messages\n")
	sb.WriteString(strings.Repeat("─", 60) + "\n")

	for _, m := range messages {
		// Skip deleted messages
		if m.Status == MsgStatusDeleted {
			continue
		}

		fromName := m.From
		if a, ok := agents[m.From]; ok {
			fromName = a.Name
		}

		// Direction arrow
		isOutgoing := m.From == selfID
		var dir string
		if isOutgoing {
			dir = "→"
		} else {
			dir = "←"
		}

		// Type icon
		typeIcon := "💬"
		switch m.Type {
		case "direct":
			typeIcon = "📨"
		case "broadcast":
			typeIcon = "📢"
		case "alert":
			typeIcon = "🚨"
		case "request":
			typeIcon = "📋"
		case "response":
			typeIcon = "↩️"
		case "status":
			typeIcon = "🔹"
		}

		// Status icon
		si := statusIcon(m.Status)
		if m.Status == "" {
			si = "📤"
		}

		// Time
		timeStr := m.Timestamp.Format("15:04:05")

		// Message ID (short, last 6 chars)
		shortID := m.ID
		if len(m.ID) > 6 {
			shortID = m.ID[len(m.ID)-6:]
		}

		// Header line: type icon + direction + from/to + status + time + id
		if m.Type == "direct" || m.Type == "request" || m.Type == "response" {
			toName := m.To
			if a, ok := agents[m.To]; ok {
				toName = a.Name
			}
			sb.WriteString(fmt.Sprintf("  %s %s %s %s %s %s [%s]\n", typeIcon, dir, fromName, toName, si, timeStr, shortID))
		} else if m.Type == "broadcast" {
			sb.WriteString(fmt.Sprintf("  %s %s %s (all) %s %s [%s]\n", typeIcon, dir, fromName, si, timeStr, shortID))
		} else {
			sb.WriteString(fmt.Sprintf("  %s %s %s %s [%s]\n", typeIcon, dir, fromName, si, shortID))
		}

		// Content
		contentLines := strings.Split(m.Content, "\n")
		for _, line := range contentLines {
			if len(line) > 100 {
				line = line[:97] + "..."
			}
			sb.WriteString(fmt.Sprintf("    %s\n", line))
		}

		// Priority for requests
		if m.Type == "request" && m.Priority != "" && m.Priority != "normal" {
			sb.WriteString(fmt.Sprintf("    ⚡ Priority: %s\n", m.Priority))
		}

		// Edited marker
		if m.EditedAt != nil {
			editedByName := m.EditedBy
			if a, ok := agents[m.EditedBy]; ok {
				editedByName = a.Name
			}
			sb.WriteString(fmt.Sprintf("    ✏️ Edited by %s at %s\n", editedByName, m.EditedAt.Format("15:04:05")))
			if m.OriginalContent != "" {
				origLines := strings.Split(m.OriginalContent, "\n")
				for _, line := range origLines {
					if len(line) > 100 {
						line = line[:97] + "..."
					}
					sb.WriteString(fmt.Sprintf("      Original: %s\n", line))
				}
			}
		}

		// Status history
		if len(m.StatusHistory) > 0 {
			lastChange := m.StatusHistory[len(m.StatusHistory)-1]
			changedByName := lastChange.ChangedBy
			if a, ok := agents[lastChange.ChangedBy]; ok {
				changedByName = a.Name
			}
			sb.WriteString(fmt.Sprintf("    %s Status: %s (by %s)\n", statusIcon(lastChange.Status), string(lastChange.Status), changedByName))
		}

		// Comments
		for _, c := range m.Comments {
			commenterName := c.AgentID
			if a, ok := agents[c.AgentID]; ok {
				commenterName = a.Name
			}
			sb.WriteString(fmt.Sprintf("    💬 %s: %s\n", commenterName, c.Content))
		}

		sb.WriteString("\n")
	}

	return sb.String()
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

// statusIcon returns an emoji for a message status
func statusIcon(s MessageStatus) string {
	switch s {
	case MsgStatusSent:
		return "📤"
	case MsgStatusDelivered:
		return "📦"
	case MsgStatusRead:
		return "👁️"
	case MsgStatusAcked:
		return "✅"
	case MsgStatusReplied:
		return "↩️"
	case MsgStatusIgnored:
		return "🚫"
	case MsgStatusDeleted:
		return "🗑️"
	default:
		return "📤"
	}
}

// FormatMessageHistory returns a formatted string of messages with status info
func FormatMessageHistory(messages []*Message, agents map[string]*AgentProfile) string {
	if len(messages) == 0 {
		return "No messages in the hub."
	}

	var sb strings.Builder
	sb.WriteString("💬 Agent Hub — Message History\n")
	sb.WriteString(strings.Repeat("─", 60) + "\n")

	for _, m := range messages {
		// Skip deleted messages
		if m.Status == MsgStatusDeleted {
			continue
		}

		fromName := m.From
		if a, ok := agents[m.From]; ok {
			fromName = a.Name
		}

		// Status indicator
		si := statusIcon(m.Status)
		if m.Status == "" {
			si = "📤" // default: sent
		}

		// Edited indicator
		editedMark := ""
		if m.EditedAt != nil {
			editedMark = " ✏️"
		}

		switch m.Type {
		case "direct":
			toName := m.To
			if a, ok := agents[m.To]; ok {
				toName = a.Name
			}
			sb.WriteString(fmt.Sprintf("  📨 %s → %s %s%s: %s\n", fromName, toName, si, editedMark, m.Content))
		case "broadcast":
			sb.WriteString(fmt.Sprintf("  📢 %s (broadcast) %s%s: %s\n", fromName, si, editedMark, m.Content))
		case "alert":
			sb.WriteString(fmt.Sprintf("  %s %s%s\n", si, editedMark, m.Content))
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
			sb.WriteString(fmt.Sprintf("  %s %s → %s [%s] %s %s%s: %s\n", priorityIcon, fromName, toName, actionLabel, m.Priority, si, editedMark, m.Content))
		case "response":
			toName := m.To
			if a, ok := agents[m.To]; ok {
				toName = a.Name
			}
			acceptIcon := "✅"
			if m.Accepted != nil && !*m.Accepted {
				acceptIcon = "❌"
			}
			sb.WriteString(fmt.Sprintf("  %s %s → %s %s%s: %s\n", acceptIcon, fromName, toName, si, editedMark, m.Content))
		default:
			sb.WriteString(fmt.Sprintf("  💬 %s %s%s: %s\n", fromName, si, editedMark, m.Content))
		}

		// Show comments
		for _, c := range m.Comments {
			commenterName := c.AgentID
			if a, ok := agents[c.AgentID]; ok {
				commenterName = a.Name
			}
			sb.WriteString(fmt.Sprintf("    💬 %s: %s\n", commenterName, c.Content))
		}
	}

	return sb.String()
}