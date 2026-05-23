package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bugbuster-code/pkg/i18n"
)

// EditTool — tool for precise file editing (fragment replacement)
type EditTool struct {
	AllowedDirs      []string
	AllowSecretFiles bool // allow access to secret files (depends on permission_mode)
}

// NewEditTool creates a tool for editing files using find-and-replace operations.
// NewEditTool creates a tool for editing files using find-and-replace operations.
func NewEditTool() *EditTool {
	return &EditTool{}
}

func (t *EditTool) Name() string { return "edit" }

func (t *EditTool) Description() string {
	return i18n.T("tools.edit.description")
}

func (t *EditTool) Execute(params map[string]string) ToolResult {
	path, ok := params["path"]
	if !ok || path == "" {
		return Error("tools.edit.param_path")
	}

	oldText, ok := params["old"]
	if !ok {
		return Error("tools.edit.param_old")
	}

	newText, ok := params["new"]
	if !ok {
		return Error("tools.edit.param_new")
	}

	// Security: resolve and check path
	resolvedPath, err := ResolvePath(path)
	if err != nil {
		return Error("security.error", err)
	}
	path = resolvedPath

	// Security: check secret files
	if isSecret, reason := SecretPathInfo(path); isSecret {
		if !t.AllowSecretFiles {
			return Error("security.secret_file", filepath.Base(path))
		}
		// Allow but log warning
		_ = reason
	}

	// Security: block system paths
	if IsSystemPath(path) {
		return Error("security.system_path", path)
	}

	// Check allowed directories
	if !IsPathAllowed(path, t.AllowedDirs) {
		return Error("security.path_denied", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Error("tools.edit.read_error", path, err)
	}

	content := string(data)

	// Check that old exists in file
	if !strings.Contains(content, oldText) {
		return Error("tools.edit.text_not_found", path)
	}

	// Replace first occurrence
	newContent := strings.Replace(content, oldText, newText, 1)

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return Error("tools.edit.write_error", path, err)
	}

	// Generate diff to show changes
	diff := UnifiedDiff(path, path, content, newContent)
	result := fmt.Sprintf("file %s edited\n%s", path, diff)

	return ToolResult{Output: result}
}

func (t *EditTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.edit.param_path_desc"),
			},
			"old": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.edit.param_old_desc"),
			},
			"new": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.edit.param_new_desc"),
			},
		},
		"required": []string{"path", "old", "new"},
	}
}
