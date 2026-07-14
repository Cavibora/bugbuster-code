package agent

import (
	"testing"
)

func TestPermissionModes(t *testing.T) {
	tests := []struct {
		name     string
		mode     PermissionMode
		req      PermissionRequest
		expected PermissionResult
		askUser  AskFunc
	}{
		{
			name: "auto-approve read-only",
			mode: PermissionAutoApprove,
			req: PermissionRequest{
				ToolName: "read",
				Level:    PermReadOnly,
			},
			expected: PermApproved,
		},
		{
			name: "auto-approve danger-full-access",
			mode: PermissionAutoApprove,
			req: PermissionRequest{
				ToolName: "bash",
				Level:    PermDangerFullAccess,
			},
			expected: PermApproved,
		},
		{
			name: "deny blocks danger-full-access",
			mode: PermissionDeny,
			req: PermissionRequest{
				ToolName: "bash",
				Level:    PermDangerFullAccess,
			},
			expected: PermDenied,
		},
		{
			name: "deny allows workspace-write",
			mode: PermissionDeny,
			req: PermissionRequest{
				ToolName: "write",
				Level:    PermWorkspaceWrite,
			},
			expected: PermApproved,
		},
		{
			name: "deny allows read-only",
			mode: PermissionDeny,
			req: PermissionRequest{
				ToolName: "read",
				Level:    PermReadOnly,
			},
			expected: PermApproved,
		},
		{
			name: "ask allows read-only",
			mode: PermissionAsk,
			req: PermissionRequest{
				ToolName: "read",
				Level:    PermReadOnly,
			},
			expected: PermApproved,
		},
		{
			name: "ask approves workspace-write (non-interactive)",
			mode: PermissionAsk,
			req: PermissionRequest{
				ToolName: "write",
				Level:    PermWorkspaceWrite,
			},
			expected: PermApproved,
		},
		{
			name: "ask denies danger-full-access (non-interactive, no AskUser)",
			mode: PermissionAsk,
			req: PermissionRequest{
				ToolName: "bash",
				Level:    PermDangerFullAccess,
			},
			expected: PermDenied,
		},
		{
			name: "ask approves danger-full-access with AskUser=true",
			mode: PermissionAsk,
			req: PermissionRequest{
				ToolName: "bash",
				Level:    PermDangerFullAccess,
			},
			expected: PermApproved,
			askUser:  func(req PermissionRequest) bool { return true },
		},
		{
			name: "ask denies danger-full-access with AskUser=false",
			mode: PermissionAsk,
			req: PermissionRequest{
				ToolName: "bash",
				Level:    PermDangerFullAccess,
			},
			expected: PermDenied,
			askUser:  func(req PermissionRequest) bool { return false },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewDefaultPermissionChecker(tt.mode, "/tmp")
			if tt.askUser != nil {
				checker.SetAskFunc(tt.askUser)
			}
			result := checker.CheckPermission(tt.req)
			if result != tt.expected {
				t.Errorf("CheckPermission(mode=%s, tool=%s, level=%s) = %s, want %s",
					tt.mode, tt.req.ToolName, tt.req.Level, result, tt.expected)
			}
		})
	}
}

func TestToolPermissionLevel(t *testing.T) {
	tests := []struct {
		toolName string
		expected ToolPermission
	}{
		{"read", PermReadOnly},
		{"grep", PermReadOnly},
		{"glob", PermReadOnly},
		{"write", PermWorkspaceWrite},
		{"edit", PermWorkspaceWrite},
		{"learn", PermWorkspaceWrite},
		{"bash", PermDangerFullAccess},
		{"ask", PermDangerFullAccess},
		{"unknown", PermDangerFullAccess},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			result := ToolPermissionLevel(tt.toolName)
			if result != tt.expected {
				t.Errorf("ToolPermissionLevel(%q) = %s, want %s",
					tt.toolName, result, tt.expected)
			}
		})
	}
}

func TestIsReadOnlyTool(t *testing.T) {
	if !IsReadOnlyTool("read") {
		t.Error("read should be read-only")
	}
	if !IsReadOnlyTool("grep") {
		t.Error("grep should be read-only")
	}
	if IsReadOnlyTool("bash") {
		t.Error("bash should not be read-only")
	}
	if IsReadOnlyTool("write") {
		t.Error("write should not be read-only")
	}
}

func TestIsDangerousTool(t *testing.T) {
	if !IsDangerousTool("bash") {
		t.Error("bash should be dangerous")
	}
	if !IsDangerousTool("ask") {
		t.Error("ask should be dangerous")
	}
	if IsDangerousTool("read") {
		t.Error("read should not be dangerous")
	}
	if IsDangerousTool("write") {
		t.Error("write should not be dangerous (it's workspace-write)")
	}
}

func TestPerToolPermissionOverrides(t *testing.T) {
	// Global mode: auto-approve, but bash is denied
	checker := NewDefaultPermissionChecker(PermissionAutoApprove, "/tmp")
	checker.SetToolPermission("bash", PermissionDeny)

	// bash should be denied (per-tool override)
	result := checker.CheckPermission(PermissionRequest{
		ToolName: "bash",
		Level:    PermDangerFullAccess,
	})
	if result != PermDenied {
		t.Errorf("bash with per-tool deny should be denied, got %s", result)
	}

	// write should be approved (global auto-approve)
	result = checker.CheckPermission(PermissionRequest{
		ToolName: "write",
		Level:    PermWorkspaceWrite,
	})
	if result != PermApproved {
		t.Errorf("write with global auto-approve should be approved, got %s", result)
	}
}

func TestPerToolPermissionOverridesFromConfig(t *testing.T) {
	// Global mode: deny, but read and write are auto-approve
	checker := NewDefaultPermissionChecker(PermissionDeny, "/tmp")
	checker.SetPermissionsFromConfig(map[string]string{
		"read":  "auto-approve",
		"write": "auto-approve",
	})

	// bash should be denied (global deny, no override)
	result := checker.CheckPermission(PermissionRequest{
		ToolName: "bash",
		Level:    PermDangerFullAccess,
	})
	if result != PermDenied {
		t.Errorf("bash with global deny should be denied, got %s", result)
	}

	// read should be approved (per-tool override)
	result = checker.CheckPermission(PermissionRequest{
		ToolName: "read",
		Level:    PermReadOnly,
	})
	if result != PermApproved {
		t.Errorf("read with per-tool auto-approve should be approved, got %s", result)
	}

	// write should be approved (per-tool override)
	result = checker.CheckPermission(PermissionRequest{
		ToolName: "write",
		Level:    PermWorkspaceWrite,
	})
	if result != PermApproved {
		t.Errorf("write with per-tool auto-approve should be approved, got %s", result)
	}
}

func TestEffectiveMode(t *testing.T) {
	checker := NewDefaultPermissionChecker(PermissionAsk, "/tmp")

	// No overrides — should return global mode
	if mode := checker.effectiveMode("bash"); mode != PermissionAsk {
		t.Errorf("effectiveMode(bash) with no overrides = %s, want %s", mode, PermissionAsk)
	}

	// Set override for bash
	checker.SetToolPermission("bash", PermissionDeny)
	if mode := checker.effectiveMode("bash"); mode != PermissionDeny {
		t.Errorf("effectiveMode(bash) with override = %s, want %s", mode, PermissionDeny)
	}

	// Other tools still use global mode
	if mode := checker.effectiveMode("write"); mode != PermissionAsk {
		t.Errorf("effectiveMode(write) without override = %s, want %s", mode, PermissionAsk)
	}
}

func TestFormatPermissionRequest(t *testing.T) {
	req := PermissionRequest{
		ToolName: "bash",
		Params:   map[string]string{"command": "rm -rf /tmp/test"},
		Reason:   "Удаление временных файлов",
		Level:    PermDangerFullAccess,
	}

	result := FormatPermissionRequest(req)
	if result == "" {
		t.Error("FormatPermissionRequest returned empty string")
	}
	// Проверяем что ключевые элементы присутствуют
	if !contains(result, "bash") {
		t.Error("FormatPermissionRequest should contain tool name")
	}
	if !contains(result, "danger-full-access") {
		t.Error("FormatPermissionRequest should contain level")
	}
	if !contains(result, "rm -rf /tmp/test") {
		t.Error("FormatPermissionRequest should contain command")
	}
}

func TestFormatPermissionRequest_LongValue(t *testing.T) {
	longContent := ""
	for i := 0; i < 200; i++ {
		longContent += "x"
	}

	req := PermissionRequest{
		ToolName: "write",
		Params:   map[string]string{"content": longContent},
		Reason:   "Запись файла",
		Level:    PermWorkspaceWrite,
	}

	result := FormatPermissionRequest(req)
	if !contains(result, "...") {
		t.Error("FormatPermissionRequest should truncate long values")
	}
}

func TestDefaultPermissionChecker_WorkspaceDir(t *testing.T) {
	checker := NewDefaultPermissionChecker(PermissionAutoApprove, "/home/user/project")
	if checker.WorkspaceDir != "/home/user/project" {
		t.Errorf("WorkspaceDir = %s, want /home/user/project", checker.WorkspaceDir)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
