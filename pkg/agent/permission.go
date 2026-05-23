package agent

import (
	"fmt"
	"strings"

	"bugbuster-code/pkg/i18n"
)

// PermissionMode — permission level for tools
type PermissionMode string

const (
	// PermissionAutoApprove — automatically approve all actions
	PermissionAutoApprove PermissionMode = "auto-approve"
	// PermissionAsk — ask before dangerous actions
	PermissionAsk PermissionMode = "ask"
	// PermissionDeny — deny dangerous actions
	PermissionDeny PermissionMode = "deny"
)

// ToolPermission — access level required by a tool
type ToolPermission string

const (
	// PermReadOnly — read-only (read, grep, glob)
	PermReadOnly ToolPermission = "read-only"
	// PermWorkspaceWrite — write to workspace directory (write, edit)
	PermWorkspaceWrite ToolPermission = "workspace-write"
	// PermDangerFullAccess — full access (bash, ask external LLM)
	PermDangerFullAccess ToolPermission = "danger-full-access"
)

// PermissionRequest — request to perform a dangerous action
type PermissionRequest struct {
	ToolName string
	Params   map[string]string
	Reason   string
	Level    ToolPermission
}

// PermissionResult — result request permissions
type PermissionResult string

const (
	PermApproved PermissionResult = "approved"
	PermDenied   PermissionResult = "denied"
)

// PermissionChecker — interface for permission checking
type PermissionChecker interface {
	// CheckPermission checks permission for tool execution
	CheckPermission(req PermissionRequest) PermissionResult
}

// AskFunc is a function for interactive permission requests from user.
// Returns true if user approved, false if declined.
type AskFunc func(req PermissionRequest) bool

// DefaultPermissionChecker checks permissions by configuration
type DefaultPermissionChecker struct {
	Mode         PermissionMode
	WorkspaceDir string
	AskUser      AskFunc // interactive request to user (nil = fallback)
}

// NewDefaultPermissionChecker creates a permission checker
func NewDefaultPermissionChecker(mode PermissionMode, workspaceDir string) *DefaultPermissionChecker {
	return &DefaultPermissionChecker{
		Mode:         mode,
		WorkspaceDir: workspaceDir,
	}
}

// SetAskFunc sets function for interactive permission request
func (c *DefaultPermissionChecker) SetAskFunc(fn AskFunc) {
	c.AskUser = fn
}

// CheckPermission checks permission for tool execution
func (c *DefaultPermissionChecker) CheckPermission(req PermissionRequest) PermissionResult {
	switch c.Mode {
	case PermissionAutoApprove:
		return PermApproved
	case PermissionDeny:
		if req.Level == PermDangerFullAccess {
			return PermDenied
		}
		return PermApproved
	case PermissionAsk:
		// ReadOnly always approved
		if req.Level == PermReadOnly {
			return PermApproved
		}
		// If interactive request function exists — ask
		if c.AskUser != nil {
			if c.AskUser(req) {
				return PermApproved
			}
			return PermDenied
		}
		// Without request function — approve WorkspaceWrite, deny DangerFullAccess
		if req.Level == PermWorkspaceWrite {
			return PermApproved
		}
		return PermDenied
	default:
		return PermApproved
	}
}

// FormatPermissionRequest formats permission request for display
func FormatPermissionRequest(req PermissionRequest) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚠️  %s: %s\n", i18n.T("agent.permission_reason"), req.ToolName))
	sb.WriteString(fmt.Sprintf("   Level: %s\n", req.Level))
	sb.WriteString(fmt.Sprintf("   Reason: %s\n", req.Reason))

	// Show key parameters
	for k, v := range req.Params {
		if k == "command" || k == "path" || k == "content" {
			// Truncate long values
			display := v
			if len(display) > 100 {
				display = display[:100] + "..."
			}
			sb.WriteString(fmt.Sprintf("   %s: %s\n", k, display))
		}
	}

	return sb.String()
}

// ToolPermissionLevel returns permission level for tool by name
func ToolPermissionLevel(toolName string) ToolPermission {
	switch toolName {
	case "read", "grep", "glob", "lsp":
		return PermReadOnly
	case "write", "edit":
		return PermWorkspaceWrite
	case "bash", "ask", "web_fetch":
		return PermDangerFullAccess
	case "learn", "ask_user":
		return PermWorkspaceWrite
	default:
		return PermDangerFullAccess // unknown tools — maximum level
	}
}

// IsReadOnlyTool checks that tool only reads
func IsReadOnlyTool(toolName string) bool {
	return ToolPermissionLevel(toolName) == PermReadOnly
}

// IsDangerousTool checks that tool requires full access
func IsDangerousTool(toolName string) bool {
	return ToolPermissionLevel(toolName) == PermDangerFullAccess
}
