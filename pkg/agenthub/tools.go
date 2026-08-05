package agenthub

import (
	"encoding/json"
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
	return "hub_message — send a message to another agent in the shared workspace. Parameters: agent_id (required) — target agent ID or name (supports partial match, e.g. 'bugbuster-coder' or 'sess_202'), content (required) — message text. Use hub_list first to see available agents and their IDs."
}

// Parameters returns the tool parameters
func (t *HubMessageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Target agent ID or name (supports partial match, e.g. 'bugbuster-coder' or 'sess_202'). Use hub_list to see available agents.",
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

	// Resolve agent ID (supports partial IDs and names)
	resolvedID, err := t.hub.resolveAgentID(agentID)
	if err != nil {
		agents := t.hub.ListAgents()
		var ids []string
		for _, a := range agents {
			ids = append(ids, fmt.Sprintf("%s (ID: %s)", a.Name, a.ID))
		}
		return tools.ToolResult{Error: fmt.Sprintf("%s. Available agents: %s", err.Error(), strings.Join(ids, ", "))}
	}

	if err := t.hub.SendMessage(resolvedID, content); err != nil {
		return tools.ToolResult{Error: err.Error()}
	}

	// Get agent name for display
	agentName := resolvedID
	if a, ok := t.hub.GetAgent(resolvedID); ok {
		agentName = a.Name
	}

	// Get self name for display
	selfName := "me"
	if profile, ok := t.hub.GetAgent(t.hub.SelfID()); ok {
		selfName = profile.Name
	}

	return tools.ToolResult{Output: fmt.Sprintf("📨 Message sent to %s (%s)\n   From: %s\n   Content: %s", agentName, resolvedID, selfName, content)}
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

	// Get self name for display
	selfName := "me"
	if profile, ok := t.hub.GetAgent(t.hub.SelfID()); ok {
		selfName = profile.Name
	}

	return tools.ToolResult{Output: fmt.Sprintf("📢 Broadcast sent to all agents\n   From: %s\n   Content: %s", selfName, content)}
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

	// Get self name for display
	selfName := "me"
	if profile, ok := t.hub.GetAgent(t.hub.SelfID()); ok {
		selfName = profile.Name
	}

	return tools.ToolResult{Output: fmt.Sprintf("⚠️ Alert sent to all agents\n   From: %s\n   Content: %s", selfName, content)}
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
	return "hub_info — get detailed info about a specific agent including their system prompt, role, model, and intelligence level. Parameters: agent_id (required) — agent ID or name (supports partial match). Use hub_list first to see available agents."
}

// Parameters returns the tool parameters
func (t *HubInfoTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Agent ID or name (supports partial match). Use hub_list to see available agents.",
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

	// Resolve agent ID (supports partial IDs and names)
	resolvedID, err := t.hub.resolveAgentID(agentID)
	if err != nil {
		return tools.ToolResult{Error: err.Error()}
	}

	agent, ok := t.hub.GetAgent(resolvedID)
	if !ok {
		return tools.ToolResult{Error: fmt.Sprintf("agent '%s' not found", resolvedID)}
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

// HubTasksTool views another agent's task list
type HubTasksTool struct {
	hub *Hub
}

// NewHubTasksTool creates a new hub_tasks tool
func NewHubTasksTool(hub *Hub) *HubTasksTool {
	return &HubTasksTool{hub: hub}
}

// Name returns the tool name
func (t *HubTasksTool) Name() string { return "hub_tasks" }

// Description returns the tool description
func (t *HubTasksTool) Description() string {
	return "hub_tasks — view another agent's task list. Parameters: agent_id (optional) — agent ID or name (supports partial match, shows all agents if omitted). Use this to see what other agents are working on and coordinate tasks to avoid conflicts."
}

// Parameters returns the tool parameters
func (t *HubTasksTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Agent ID or name (supports partial match, optional — shows all agents if omitted)",
			},
		},
	}
}

// Execute runs the tool
func (t *HubTasksTool) Execute(params map[string]string) tools.ToolResult {
	agentID := params["agent_id"]

	if agentID != "" {
		// Resolve agent ID (supports partial IDs and names)
		resolvedID, err := t.hub.resolveAgentID(agentID)
		if err != nil {
			return tools.ToolResult{Error: err.Error()}
		}

		// Show specific agent's tasks
		agent, ok := t.hub.GetAgent(resolvedID)
		if !ok {
			return tools.ToolResult{Error: fmt.Sprintf("agent '%s' not found", resolvedID)}
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("📋 Tasks for %s (%s):\n", agent.Name, agent.ID))
		sb.WriteString(strings.Repeat("─", 40) + "\n")

		if len(agent.Tasks) == 0 {
			sb.WriteString("  (no tasks)\n")
		} else {
			for _, task := range agent.Tasks {
				statusIcon := "⬜"
				switch task.Status {
				case "in_progress":
					statusIcon = "🔄"
				case "completed":
					statusIcon = "✅"
				}
				sb.WriteString(fmt.Sprintf("  %s [%s] %s\n", statusIcon, task.ID, task.Subject))
			}
		}

		return tools.ToolResult{Output: sb.String()}
	}

	// Show all agents' tasks
	agents := t.hub.ListAgents()
	var sb strings.Builder
	sb.WriteString("📋 All Agents' Tasks:\n")
	sb.WriteString(strings.Repeat("─", 40) + "\n")

	hasTasks := false
	for _, agent := range agents {
		if len(agent.Tasks) > 0 {
			hasTasks = true
			sb.WriteString(fmt.Sprintf("\n🤖 %s (%s):\n", agent.Name, agent.ID))
			for _, task := range agent.Tasks {
				statusIcon := "⬜"
				switch task.Status {
				case "in_progress":
					statusIcon = "🔄"
				case "completed":
					statusIcon = "✅"
				}
				sb.WriteString(fmt.Sprintf("  %s [%s] %s\n", statusIcon, task.ID, task.Subject))
			}
		}
	}

	if !hasTasks {
		sb.WriteString("\n  (no agents have tasks)\n")
	}

	return tools.ToolResult{Output: sb.String()}
}

// HubStatusTool updates own status and current task in the hub
type HubStatusTool struct {
	hub *Hub
}

// NewHubStatusTool creates a new hub_status tool
func NewHubStatusTool(hub *Hub) *HubStatusTool {
	return &HubStatusTool{hub: hub}
}

// Name returns the tool name
func (t *HubStatusTool) Name() string { return "hub_status" }

// Description returns the tool description
func (t *HubStatusTool) Description() string {
	return `hub_status — update your own status and current task in the shared workspace. Parameters: status (optional) — your status: "idle", "working", "waiting", "done", "error"; task (optional) — description of what you're currently working on; tasks (optional) — JSON array of tasks, e.g. [{"id":"1","subject":"fix bug","status":"in_progress"}]. Use this to let other agents know what you're doing and coordinate work. At least one parameter is required.`
}

// Parameters returns the tool parameters
func (t *HubStatusTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status": map[string]any{
				"type":        "string",
				"description": `Your status: "idle", "working", "waiting", "done", "error"`,
			},
			"task": map[string]any{
				"type":        "string",
				"description": "Description of what you're currently working on",
			},
			"tasks": map[string]any{
				"type":        "string",
				"description": `JSON array of tasks, e.g. [{"id":"1","subject":"fix bug","status":"in_progress"}]`,
			},
		},
	}
}

// Execute runs the tool
func (t *HubStatusTool) Execute(params map[string]string) tools.ToolResult {
	status := params["status"]
	task := params["task"]
	tasksJSON := params["tasks"]

	if status == "" && task == "" && tasksJSON == "" {
		return tools.ToolResult{Error: "at least one parameter is required: status, task, or tasks"}
	}

	// Validate status
	if status != "" {
		validStatuses := map[string]bool{
			"idle": true, "working": true, "waiting": true, "done": true, "error": true,
		}
		if !validStatuses[status] {
			return tools.ToolResult{Error: fmt.Sprintf("invalid status '%s'. Valid: idle, working, waiting, done, error", status)}
		}
	}

	// Update status and task
	if status != "" || task != "" {
		agentStatus := AgentStatus(status)
		if status == "" {
			// Keep current status
			profile, ok := t.hub.GetAgent(t.hub.selfID)
			if ok {
				agentStatus = profile.Status
			}
		}
		if err := t.hub.UpdateStatus(agentStatus, task); err != nil {
			return tools.ToolResult{Error: err.Error()}
		}
	}

	// Update tasks if provided
	if tasksJSON != "" {
		var tasks []AgentTask
		if err := json.Unmarshal([]byte(tasksJSON), &tasks); err != nil {
			return tools.ToolResult{Error: fmt.Sprintf("invalid tasks JSON: %v", err)}
		}
		if err := t.hub.UpdateTasks(tasks); err != nil {
			return tools.ToolResult{Error: err.Error()}
		}
	}

	// Build response
	var sb strings.Builder
	sb.WriteString("✅ Status updated\n")
	if status != "" {
		sb.WriteString(fmt.Sprintf("   Status: %s\n", status))
	}
	if task != "" {
		sb.WriteString(fmt.Sprintf("   Task:   %s\n", task))
	}
	if tasksJSON != "" {
		var tasks []AgentTask
		json.Unmarshal([]byte(tasksJSON), &tasks)
		sb.WriteString(fmt.Sprintf("   Tasks:  %d task(s)\n", len(tasks)))
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
				"description": "Filter messages by agent ID or name (supports partial match, optional)",
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

	// Resolve agent ID if specified (supports partial IDs and names)
	resolvedID := agentID
	if agentID != "" {
		id, err := t.hub.resolveAgentID(agentID)
		if err != nil {
			return tools.ToolResult{Error: err.Error()}
		}
		resolvedID = id
	}

	var messages []*Message
	if resolvedID != "" {
		messages = t.hub.GetHistory(resolvedID, limit)
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
	return `hub_request — ask another agent to do something. Parameters: agent_id (required) — target agent ID or name (supports partial match), action (required) — what to ask: "do" (do a task), "redo" (redo/rewrite something), "stop" (stop what you're doing), "wait" (wait until I'm done), "review" (review my code), "test" (run tests), "fix" (fix a bug), content (required) — description of the request, priority (optional) — "low", "normal", "high", "urgent" (default: "normal"). Use hub_list first to see available agents. The other agent can accept or decline.`
}

// Parameters returns the tool parameters
func (t *HubRequestTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Target agent ID or name (supports partial match). Use hub_list to see available agents.",
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

	// Resolve agent ID (supports partial IDs and names)
	resolvedID, err := t.hub.resolveAgentID(agentID)
	if err != nil {
		return tools.ToolResult{Error: err.Error()}
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

	msg, err := t.hub.SendRequest(resolvedID, action, content, priority)
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

	// Get self name for display
	selfName := "me"
	if profile, ok := t.hub.GetAgent(t.hub.SelfID()); ok {
		selfName = profile.Name
	}

	return tools.ToolResult{Output: fmt.Sprintf("%s Request sent to %s [%s/%s]\n   From: %s\n   Content: %s\n   Request ID: %s\n   The agent can accept or decline this request.", priorityIcon, agentName, action, priority, selfName, content, msg.ID)}
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

	// Get self name for display
	selfName := "me"
	if profile, ok := t.hub.GetAgent(t.hub.SelfID()); ok {
		selfName = profile.Name
	}

	// Get target agent name for display
	targetName := "agent"
	if msg.To != "" {
		if a, ok := t.hub.GetAgent(msg.To); ok {
			targetName = a.Name
		}
	}

	return tools.ToolResult{Output: fmt.Sprintf("%s Response sent to %s\n   From: %s\n   Content: %s\n   Response ID: %s", icon, targetName, selfName, content, msg.ID)}
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
		sb.WriteString("📭 No unread messages or pending requests.\n")
	} else {
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
	}

	// Show status of recent messages involving this agent (including outgoing),
	// so the model can see whether its messages were read / replied to.
	selfID := t.hub.SelfID()
	recent := t.hub.GetHistory(selfID, 8)
	if len(recent) > 0 {
		sb.WriteString("\n📊 Recent Message Statuses:\n")
		sb.WriteString(strings.Repeat("─", 40) + "\n")
		for _, m := range recent {
			// Skip deleted
			if m.Status == MsgStatusDeleted {
				continue
			}
			fromName := m.From
			if a, ok := agents[m.From]; ok {
				fromName = a.Name
			}
			toName := m.To
			if a, ok := agents[m.To]; ok {
				toName = a.Name
			}
			// Direction
			dir := "←"
			if m.From == selfID {
				dir = "→"
			}
			// Status icon
			si := statusIcon(m.Status)
			if m.Status == "" {
				si = "📤"
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
			}
			// Short content
			content := m.Content
			if len(content) > 60 {
				content = content[:57] + "..."
			}
			content = strings.ReplaceAll(content, "\n", " ")
			// Short ID
			shortID := m.ID
			if len(m.ID) > 6 {
				shortID = m.ID[len(m.ID)-6:]
			}
			sb.WriteString(fmt.Sprintf("  %s %s %s→%s %s [%s]: %s\n", typeIcon, dir, fromName, toName, si, shortID, content))
		}
		sb.WriteString("\nUse hub_msg_status <message_id> <status> to update a message status (read/acked/replied/ignored).")
	}

	return tools.ToolResult{Output: sb.String()}
}

// HubMsgStatusTool changes the status of a message (read, acked, replied, ignored)
type HubMsgStatusTool struct {
	hub *Hub
}

func NewHubMsgStatusTool(hub *Hub) *HubMsgStatusTool {
	return &HubMsgStatusTool{hub: hub}
}

func (t *HubMsgStatusTool) Name() string { return "hub_msg_status" }

func (t *HubMsgStatusTool) Description() string {
	return `hub_msg_status — change the status of a message. Parameters: message_id (required) — ID of the message, status (required) — new status: "read" (I read it), "acked" (I took note), "replied" (I replied to it), "ignored" (I'm ignoring this). Use this to track message lifecycle and avoid re-reading old messages. Statuses: sent → delivered → read → acked/replied/ignored.`
}

func (t *HubMsgStatusTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message_id": map[string]any{
				"type":        "string",
				"description": "ID of the message to update (from hub_check or hub_history)",
			},
			"status": map[string]any{
				"type":        "string",
				"description": `New status: "read" (I read it), "acked" (I took note), "replied" (I replied), "ignored" (I'm ignoring this)`,
			},
			"note": map[string]any{
				"type":        "string",
				"description": "Optional note about the status change",
			},
		},
		"required": []string{"message_id", "status"},
	}
}

func (t *HubMsgStatusTool) Execute(params map[string]string) tools.ToolResult {
	messageID := params["message_id"]
	status := params["status"]
	note := params["note"]

	if messageID == "" {
		return tools.ToolResult{Error: "message_id is required"}
	}
	if status == "" {
		return tools.ToolResult{Error: "status is required"}
	}

	// Validate status
	validStatuses := map[string]bool{
		"read": true, "acked": true, "replied": true, "ignored": true, "deleted": true,
	}
	if !validStatuses[status] {
		return tools.ToolResult{Error: fmt.Sprintf("invalid status '%s'. Valid: read, acked, replied, ignored, deleted", status)}
	}

	if err := t.hub.UpdateMessageStatus(messageID, MessageStatus(status), note); err != nil {
		return tools.ToolResult{Error: err.Error()}
	}

	statusIcons := map[string]string{
		"read":    "📖",
		"acked":   "✅",
		"replied": "💬",
		"ignored": "🚫",
		"deleted": "🗑️",
	}
	icon := statusIcons[status]

	result := fmt.Sprintf("%s Message %s status changed to: %s", icon, messageID, status)
	if note != "" {
		result += fmt.Sprintf("\n   Note: %s", note)
	}
	return tools.ToolResult{Output: result}
}

// HubMsgCommentTool adds a comment to a message
type HubMsgCommentTool struct {
	hub *Hub
}

func NewHubMsgCommentTool(hub *Hub) *HubMsgCommentTool {
	return &HubMsgCommentTool{hub: hub}
}

func (t *HubMsgCommentTool) Name() string { return "hub_msg_comment" }

func (t *HubMsgCommentTool) Description() string {
	return `hub_msg_comment — add a comment to a message. Parameters: message_id (required) — ID of the message, content (required) — comment text. Use this to add context, notes, or follow-up information to messages. Both sender and recipient can comment.`
}

func (t *HubMsgCommentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message_id": map[string]any{
				"type":        "string",
				"description": "ID of the message to comment on (from hub_check or hub_history)",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Comment text to add",
			},
		},
		"required": []string{"message_id", "content"},
	}
}

func (t *HubMsgCommentTool) Execute(params map[string]string) tools.ToolResult {
	messageID := params["message_id"]
	content := params["content"]

	if messageID == "" {
		return tools.ToolResult{Error: "message_id is required"}
	}
	if content == "" {
		return tools.ToolResult{Error: "content is required"}
	}

	if err := t.hub.AddComment(messageID, content); err != nil {
		return tools.ToolResult{Error: err.Error()}
	}

	// Get self name for display
	selfName := "me"
	if profile, ok := t.hub.GetAgent(t.hub.SelfID()); ok {
		selfName = profile.Name
	}

	return tools.ToolResult{Output: fmt.Sprintf("💬 Comment added by %s to message %s\n   Content: %s", selfName, messageID, content)}
}

// HubMsgEditTool edits the content of a message (only sender can edit)
type HubMsgEditTool struct {
	hub *Hub
}

func NewHubMsgEditTool(hub *Hub) *HubMsgEditTool {
	return &HubMsgEditTool{hub: hub}
}

func (t *HubMsgEditTool) Name() string { return "hub_msg_edit" }

func (t *HubMsgEditTool) Description() string {
	return `hub_msg_edit — edit the content of a message you sent. Parameters: message_id (required) — ID of the message, content (required) — new content. Only the sender can edit. Original content is preserved in message history. Use this to fix typos, add details, or clarify your message.`
}

func (t *HubMsgEditTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message_id": map[string]any{
				"type":        "string",
				"description": "ID of the message to edit (only messages you sent)",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "New content for the message",
			},
		},
		"required": []string{"message_id", "content"},
	}
}

func (t *HubMsgEditTool) Execute(params map[string]string) tools.ToolResult {
	messageID := params["message_id"]
	content := params["content"]

	if messageID == "" {
		return tools.ToolResult{Error: "message_id is required"}
	}
	if content == "" {
		return tools.ToolResult{Error: "content is required"}
	}

	if err := t.hub.EditMessage(messageID, content); err != nil {
		return tools.ToolResult{Error: err.Error()}
	}

	return tools.ToolResult{Output: fmt.Sprintf("✏️ Message %s edited. Original content preserved in history.", messageID)}
}

// HubMsgDeleteTool soft-deletes a message (marks as deleted)
type HubMsgDeleteTool struct {
	hub *Hub
}

func NewHubMsgDeleteTool(hub *Hub) *HubMsgDeleteTool {
	return &HubMsgDeleteTool{hub: hub}
}

func (t *HubMsgDeleteTool) Name() string { return "hub_msg_delete" }

func (t *HubMsgDeleteTool) Description() string {
	return `hub_msg_delete — delete a message (soft delete, marks as deleted). Parameters: message_id (required) — ID of the message to delete. Both sender and recipient can delete. Deleted messages are marked but preserved in history.`
}

func (t *HubMsgDeleteTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message_id": map[string]any{
				"type":        "string",
				"description": "ID of the message to delete",
			},
		},
		"required": []string{"message_id"},
	}
}

func (t *HubMsgDeleteTool) Execute(params map[string]string) tools.ToolResult {
	messageID := params["message_id"]

	if messageID == "" {
		return tools.ToolResult{Error: "message_id is required"}
	}

	if err := t.hub.DeleteMessage(messageID); err != nil {
		return tools.ToolResult{Error: err.Error()}
	}

	return tools.ToolResult{Output: fmt.Sprintf("🗑️ Message %s deleted.", messageID)}
}

