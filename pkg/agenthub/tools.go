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