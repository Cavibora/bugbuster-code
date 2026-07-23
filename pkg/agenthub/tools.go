package agenthub

import (
	"fmt"
	"strings"
	"time"

	"bugbuster-code/pkg/tools"
)

// HubListTool lists all agents in the shared workspace
type HubListTool struct {
	hub *Hub
}

// NewHubListTool creates a new hub_list tool
func NewHubListTool(hub *Hub) *HubListTool {
	return &HubListTool{hub: hub}
}

// Name returns the tool name
func (t *HubListTool) Name() string { return "hub_list" }

// Description returns the tool description
func (t *HubListTool) Description() string {
	return "hub_list — list all agents in the shared workspace. Shows each agent's name, model, intelligence level, status, and current task. Use this to see who else is working on the project and coordinate with them."
}

// Parameters returns the tool parameters
func (t *HubListTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

// Execute runs the tool
func (t *HubListTool) Execute(params map[string]string) tools.ToolResult {
	agents := t.hub.ListAgents()
	return tools.ToolResult{Output: FormatAgentList(agents)}
}

// HubMessageTool sends a direct message to another agent
type HubMessageTool struct {
	hub *Hub
}

// NewHubMessageTool creates a new hub_message tool
func NewHubMessageTool(hub *Hub) *HubMessageTool {
	return &HubMessageTool{hub: hub}
}

// Name returns the tool name
func (t *HubMessageTool) Name() string { return "hub_message" }

// Description returns the tool description
func (t *HubMessageTool) Description() string {
	return "hub_message — send a message to another agent in the shared workspace. Parameters: agent_id (required) — target agent ID, content (required) — message text. Use this to coordinate with other agents, ask questions, or share information."
}

// Parameters returns the tool parameters
func (t *HubMessageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Target agent ID to send the message to",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Message content to send",
			},
		},
		"required": []string{"agent_id", "content"},
	}
}

// Execute runs the tool
func (t *HubMessageTool) Execute(params map[string]string) tools.ToolResult {
	agentID := params["agent_id"]
	content := params["content"]

	if agentID == "" {
		return tools.ToolResult{Error: "agent_id is required"}
	}
	if content == "" {
		return tools.ToolResult{Error: "content is required"}
	}

	// Verify target agent exists
	if _, ok := t.hub.GetAgent(agentID); !ok {
		agents := t.hub.ListAgents()
		var ids []string
		for _, a := range agents {
			ids = append(ids, fmt.Sprintf("%s (%s)", a.Name, a.ID))
		}
		return tools.ToolResult{Error: fmt.Sprintf("agent '%s' not found. Available agents: %s", agentID, strings.Join(ids, ", "))}
	}

	if err := t.hub.SendMessage(agentID, content); err != nil {
		return tools.ToolResult{Error: err.Error()}
	}

	return tools.ToolResult{Output: fmt.Sprintf("📨 Message sent to agent %s", agentID)}
}

// HubBroadcastTool broadcasts a message to all agents
type HubBroadcastTool struct {
	hub *Hub
}

// NewHubBroadcastTool creates a new hub_broadcast tool
func NewHubBroadcastTool(hub *Hub) *HubBroadcastTool {
	return &HubBroadcastTool{hub: hub}
}

// Name returns the tool name
func (t *HubBroadcastTool) Name() string { return "hub_broadcast" }

// Description returns the tool description
func (t *HubBroadcastTool) Description() string {
	return "hub_broadcast — broadcast a message to all agents in the shared workspace. Parameters: content (required) — message text. Use this for important announcements like 'I broke the tests, please wait' or 'I'm starting work on the auth module'."
}

// Parameters returns the tool parameters
func (t *HubBroadcastTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "Message content to broadcast to all agents",
			},
		},
		"required": []string{"content"},
	}
}

// Execute runs the tool
func (t *HubBroadcastTool) Execute(params map[string]string) tools.ToolResult {
	content := params["content"]
	if content == "" {
		return tools.ToolResult{Error: "content is required"}
	}

	if err := t.hub.Broadcast(content); err != nil {
		return tools.ToolResult{Error: err.Error()}
	}

	return tools.ToolResult{Output: "📢 Broadcast sent to all agents"}
}

// HubAlertTool sends an urgent alert to all agents
type HubAlertTool struct {
	hub *Hub
}

// NewHubAlertTool creates a new hub_alert tool
func NewHubAlertTool(hub *Hub) *HubAlertTool {
	return &HubAlertTool{hub: hub}
}

// Name returns the tool name
func (t *HubAlertTool) Name() string { return "hub_alert" }

// Description returns the tool description
func (t *HubAlertTool) Description() string {
	return "hub_alert — send an urgent alert to all agents. Parameters: content (required) — alert text. Use this for critical notifications like 'Tests are broken!' or 'Deploy in progress, do not push!'. Alerts are highlighted and always visible."
}

// Parameters returns the tool parameters
func (t *HubAlertTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "Alert content — critical information for all agents",
			},
		},
		"required": []string{"content"},
	}
}

// Execute runs the tool
func (t *HubAlertTool) Execute(params map[string]string) tools.ToolResult {
	content := params["content"]
	if content == "" {
		return tools.ToolResult{Error: "content is required"}
	}

	if err := t.hub.Alert(content); err != nil {
		return tools.ToolResult{Error: err.Error()}
	}

	return tools.ToolResult{Output: "⚠️ Alert sent to all agents"}
}

// HubInfoTool gets detailed info about a specific agent
type HubInfoTool struct {
	hub *Hub
}

// NewHubInfoTool creates a new hub_info tool
func NewHubInfoTool(hub *Hub) *HubInfoTool {
	return &HubInfoTool{hub: hub}
}

// Name returns the tool name
func (t *HubInfoTool) Name() string { return "hub_info" }

// Description returns the tool description
func (t *HubInfoTool) Description() string {
	return "hub_info — get detailed info about a specific agent including their system prompt, role, model, and intelligence level. Parameters: agent_id (required) — agent ID to inspect. Use this to understand another agent's capabilities and role before coordinating."
}

// Parameters returns the tool parameters
func (t *HubInfoTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Agent ID to get info about",
			},
		},
		"required": []string{"agent_id"},
	}
}

// Execute runs the tool
func (t *HubInfoTool) Execute(params map[string]string) tools.ToolResult {
	agentID := params["agent_id"]
	if agentID == "" {
		return tools.ToolResult{Error: "agent_id is required"}
	}

	agent, ok := t.hub.GetAgent(agentID)
	if !ok {
		return tools.ToolResult{Error: fmt.Sprintf("agent '%s' not found", agentID)}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🤖 Agent: %s\n", agent.Name))
	sb.WriteString(fmt.Sprintf("   ID:             %s\n", agent.ID))
	sb.WriteString(fmt.Sprintf("   Provider:       %s\n", agent.Provider))
	sb.WriteString(fmt.Sprintf("   Model:          %s\n", agent.Model))
	sb.WriteString(fmt.Sprintf("   Intelligence:   %s (%s)\n", strings.Repeat("★", int(agent.Intelligence))+strings.Repeat("☆", 5-int(agent.Intelligence)), agent.Intelligence))
	sb.WriteString(fmt.Sprintf("   Role:           %s\n", agent.Role))
	sb.WriteString(fmt.Sprintf("   Status:         %s\n", agent.Status))
	if agent.CurrentTask != "" {
		sb.WriteString(fmt.Sprintf("   Current Task:   %s\n", agent.CurrentTask))
	}
	if agent.Project != "" {
		sb.WriteString(fmt.Sprintf("   Project:        %s\n", agent.Project))
	}
	sb.WriteString(fmt.Sprintf("   Registered:     %s\n", agent.RegisteredAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("   Last Heartbeat: %s\n", agent.LastHeartbeat.Format(time.RFC3339)))
	if agent.SystemPrompt != "" {
		// Show first 500 chars of system prompt
		prompt := agent.SystemPrompt
		if len(prompt) > 500 {
			prompt = prompt[:500] + "..."
		}
		sb.WriteString(fmt.Sprintf("\n📝 System Prompt (first 500 chars):\n%s\n", prompt))
	}

	return tools.ToolResult{Output: sb.String()}
}

// HubHistoryTool views message history in the shared workspace
type HubHistoryTool struct {
	hub *Hub
}

// NewHubHistoryTool creates a new hub_history tool
func NewHubHistoryTool(hub *Hub) *HubHistoryTool {
	return &HubHistoryTool{hub: hub}
}

// Name returns the tool name
func (t *HubHistoryTool) Name() string { return "hub_history" }

// Description returns the tool description
func (t *HubHistoryTool) Description() string {
	return "hub_history — view message history in the shared workspace. Parameters: agent_id (optional) — filter by agent, limit (optional) — max messages to show (default 20). Use this to see what other agents have been discussing and coordinate work."
}

// Parameters returns the tool parameters
func (t *HubHistoryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Filter messages by agent ID (optional)",
			},
			"limit": map[string]any{
				"type":        "string",
				"description": "Max messages to show (default 20)",
			},
		},
	}
}

// Execute runs the tool
func (t *HubHistoryTool) Execute(params map[string]string) tools.ToolResult {
	agentID := params["agent_id"]
	limit := 20

	if l := params["limit"]; l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	var messages []*Message
	if agentID != "" {
		messages = t.hub.GetHistory(agentID, limit)
	} else {
		messages = t.hub.GetAllMessages(limit)
	}

	// Build agent map for display
	agents := make(map[string]*AgentProfile)
	for _, a := range t.hub.ListAgents() {
		agents[a.ID] = a
	}

	return tools.ToolResult{Output: FormatMessageHistory(messages, agents)}
}

// HubRequestTool sends a task request to another agent
type HubRequestTool struct {
	hub *Hub
}

// NewHubRequestTool creates a new hub_request tool
func NewHubRequestTool(hub *Hub) *HubRequestTool {
	return &HubRequestTool{hub: hub}
}

// Name returns the tool name
func (t *HubRequestTool) Name() string { return "hub_request" }

// Description returns the tool description
func (t *HubRequestTool) Description() string {
	return `hub_request — ask another agent to do something. Parameters: agent_id (required) — target agent ID, action (required) — what to ask: "do" (do a task), "redo" (redo/rewrite something), "stop" (stop what you're doing), "wait" (wait until I'm done), "review" (review my code), "test" (run tests), "fix" (fix a bug), content (required) — description of the request, priority (optional) — "low", "normal", "high", "urgent" (default: "normal"). Use this to delegate tasks, ask for help, or coordinate work between agents. The other agent can accept or decline.`
}

// Parameters returns the tool parameters
func (t *HubRequestTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Target agent ID to send the request to",
			},
			"action": map[string]any{
				"type":        "string",
				"description": `What to ask: "do" (do a task), "redo" (redo/rewrite), "stop" (stop what you're doing), "wait" (wait until I'm done), "review" (review my code), "test" (run tests), "fix" (fix a bug)`,
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Description of the request — what you want the other agent to do",
			},
			"priority": map[string]any{
				"type":        "string",
				"description": `Priority: "low", "normal", "high", "urgent" (default: "normal")`,
			},
		},
		"required": []string{"agent_id", "action", "content"},
	}
}

// Execute runs the tool
func (t *HubRequestTool) Execute(params map[string]string) tools.ToolResult {
	agentID := params["agent_id"]
	action := params["action"]
	content := params["content"]
	priority := params["priority"]

	if agentID == "" {
		return tools.ToolResult{Error: "agent_id is required"}
	}
	if action == "" {
		return tools.ToolResult{Error: "action is required (do, redo, stop, wait, review, test, fix)"}
	}
	if content == "" {
		return tools.ToolResult{Error: "content is required"}
	}

	// Validate action
	validActions := map[string]bool{"do": true, "redo": true, "stop": true, "wait": true, "review": true, "test": true, "fix": true}
	if !validActions[action] {
		return tools.ToolResult{Error: fmt.Sprintf("invalid action '%s'. Valid actions: do, redo, stop, wait, review, test, fix", action)}
	}

	// Validate priority
	if priority != "" {
		validPriorities := map[string]bool{"low": true, "normal": true, "high": true, "urgent": true}
		if !validPriorities[priority] {
			return tools.ToolResult{Error: fmt.Sprintf("invalid priority '%s'. Valid: low, normal, high, urgent", priority)}
		}
	}

	msg, err := t.hub.SendRequest(agentID, action, content, priority)
	if err != nil {
		return tools.ToolResult{Error: err.Error()}
	}

	// Get agent name for display
	agentName := agentID
	if a, ok := t.hub.GetAgent(agentID); ok {
		agentName = a.Name
	}

	priorityIcon := "📋"
	if priority == "urgent" {
		priorityIcon = "🔴"
	} else if priority == "high" {
		priorityIcon = "🟠"
	}

	return tools.ToolResult{Output: fmt.Sprintf("%s Request sent to %s [%s/%s]: %s\nRequest ID: %s\nThe agent can accept or decline this request.", priorityIcon, agentName, action, priority, content, msg.ID)}
}

// HubRespondTool responds to a request from another agent
type HubRespondTool struct {
	hub *Hub
}

// NewHubRespondTool creates a new hub_respond tool
func NewHubRespondTool(hub *Hub) *HubRespondTool {
	return &HubRespondTool{hub: hub}
}

// Name returns the tool name
func (t *HubRespondTool) Name() string { return "hub_respond" }

// Description returns the tool description
func (t *HubRespondTool) Description() string {
	return `hub_respond — respond to a request from another agent. Parameters: request_id (required) — ID of the request to respond to, accept (required) — "true" to accept the request, "false" to decline, content (required) — your response or explanation. Use this to accept or decline task requests from other agents.`
}

// Parameters returns the tool parameters
func (t *HubRespondTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"request_id": map[string]any{
				"type":        "string",
				"description": "ID of the request to respond to (from hub_check or hub_history)",
			},
			"accept": map[string]any{
				"type":        "string",
				"description": `"true" to accept the request, "false" to decline`,
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Your response — explanation of why you accept/decline, or the result of completing the task",
			},
		},
		"required": []string{"request_id", "accept", "content"},
	}
}

// Execute runs the tool
func (t *HubRespondTool) Execute(params map[string]string) tools.ToolResult {
	requestID := params["request_id"]
	acceptStr := params["accept"]
	content := params["content"]

	if requestID == "" {
		return tools.ToolResult{Error: "request_id is required"}
	}
	if acceptStr == "" {
		return tools.ToolResult{Error: "accept is required (true/false)"}
	}
	if content == "" {
		return tools.ToolResult{Error: "content is required"}
	}

	accept := strings.ToLower(acceptStr) == "true" || acceptStr == "1" || strings.ToLower(acceptStr) == "yes"

	msg, err := t.hub.RespondToRequest(requestID, content, accept)
	if err != nil {
		return tools.ToolResult{Error: err.Error()}
	}

	icon := "✅"
	if !accept {
		icon = "❌"
	}

	// Get agent name for display
	fromName := msg.To
	if a, ok := t.hub.GetAgent(msg.To); ok {
		fromName = a.Name
	}

	return tools.ToolResult{Output: fmt.Sprintf("%s Response sent to %s: %s\nResponse ID: %s", icon, fromName, content, msg.ID)}
}

// HubCheckTool checks for unread messages and pending requests
type HubCheckTool struct {
	hub *Hub
}

// NewHubCheckTool creates a new hub_check tool
func NewHubCheckTool(hub *Hub) *HubCheckTool {
	return &HubCheckTool{hub: hub}
}

// Name returns the tool name
func (t *HubCheckTool) Name() string { return "hub_check" }

// Description returns the tool description
func (t *HubCheckTool) Description() string {
	return `hub_check — check for unread messages and pending requests addressed to you. Shows: 1) Unread direct messages, 2) Unread broadcasts/alerts, 3) Pending task requests that need your response. Use this to stay aware of what other agents are asking you to do.`
}

// Parameters returns the tool parameters
func (t *HubCheckTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

// Execute runs the tool
func (t *HubCheckTool) Execute(params map[string]string) tools.ToolResult {
	unread := t.hub.GetUnreadMessages()
	pending := t.hub.GetPendingRequests()

	// Build agent map for display
	agents := make(map[string]*AgentProfile)
	for _, a := range t.hub.ListAgents() {
		agents[a.ID] = a
	}

	var sb strings.Builder

	if len(unread) == 0 && len(pending) == 0 {
		return tools.ToolResult{Output: "📭 No unread messages or pending requests."}
	}

	if len(pending) > 0 {
		sb.WriteString(fmt.Sprintf("📋 Pending Requests (%d):\n", len(pending)))
		sb.WriteString(strings.Repeat("─", 40) + "\n")
		for _, req := range pending {
			fromName := req.From
			if a, ok := agents[req.From]; ok {
				fromName = a.Name
			}
			priorityIcon := "📋"
			if req.Priority == "urgent" {
				priorityIcon = "🔴"
			} else if req.Priority == "high" {
				priorityIcon = "🟠"
			}
			sb.WriteString(fmt.Sprintf("  %s [%s] From: %s\n", priorityIcon, req.Action, fromName))
			sb.WriteString(fmt.Sprintf("     %s\n", req.Content))
			sb.WriteString(fmt.Sprintf("     Request ID: %s (use hub_respond to accept/decline)\n", req.ID))
		}
		sb.WriteString("\n")
	}

	if len(unread) > 0 {
		// Filter out pending requests (already shown above)
		var otherUnread []*Message
		for _, m := range unread {
			if m.Type != "request" {
				otherUnread = append(otherUnread, m)
			}
		}
		if len(otherUnread) > 0 {
			sb.WriteString(fmt.Sprintf("📨 Unread Messages (%d):\n", len(otherUnread)))
			sb.WriteString(strings.Repeat("─", 40) + "\n")
			for _, m := range otherUnread {
				fromName := m.From
				if a, ok := agents[m.From]; ok {
					fromName = a.Name
				}
				switch m.Type {
				case "direct":
					sb.WriteString(fmt.Sprintf("  📨 %s: %s\n", fromName, m.Content))
				case "broadcast":
					sb.WriteString(fmt.Sprintf("  📢 %s (broadcast): %s\n", fromName, m.Content))
				case "alert":
					sb.WriteString(fmt.Sprintf("  %s\n", m.Content))
				case "status":
					sb.WriteString(fmt.Sprintf("  🔹 %s\n", m.Content))
				case "response":
					acceptIcon := "✅"
					if m.Accepted != nil && !*m.Accepted {
						acceptIcon = "❌"
					}
					sb.WriteString(fmt.Sprintf("  %s %s responded: %s\n", acceptIcon, fromName, m.Content))
				}
			}
		}
	}

	return tools.ToolResult{Output: sb.String()}
}