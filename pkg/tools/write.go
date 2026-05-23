package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bugbuster-code/pkg/i18n"
)

// WriteTool is a tool for writing files
type WriteTool struct {
	AllowedDirs      []string // allowed directories (empty = all)
	AllowSecretFiles bool     // allow access to secret files (depends on permission_mode)
}

// NewWriteTool creates a tool for writing content to files.
func NewWriteTool() *WriteTool {
	return &WriteTool{}
}

func (t *WriteTool) Name() string { return "write" }

func (t *WriteTool) Description() string {
	return i18n.T("tools.write.description")
}

func (t *WriteTool) Execute(params map[string]string) ToolResult {
	path, ok := params["path"]
	if !ok || path == "" {
		return Error("tools.write.param_path")
	}

	content, ok := params["content"]
	if !ok {
		return Error("tools.write.param_content")
	}

	// Security: resolve and validate path
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

	// Create intermediate directories
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Error("tools.write.dir_error", dir, err)
	}

	// Check if file exists (for diff or new file content)
	oldData, readErr := os.ReadFile(path)
	isNewFile := readErr != nil

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return Error("tools.write.file_error", path, err)
	}

	if isNewFile {
		// New file — show content (up to 50 lines)
		lines := strings.Split(content, "\n")
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		maxLines := 50
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("file %s written (%d bytes)\n", path, len(content)))
		for i, line := range lines {
			if i >= maxLines {
				sb.WriteString(fmt.Sprintf("... (%d more lines)", len(lines)-maxLines))
				break
			}
			sb.WriteString(fmt.Sprintf("%4d  %s\n", i+1, line))
		}
		return ToolResult{Output: sb.String()}
	}

	// Existing file — show diff
	oldContent := string(oldData)
	diff := UnifiedDiff(path, path, oldContent, content)
	result := fmt.Sprintf("file %s written (%d bytes)\n%s", path, len(content), diff)
	return ToolResult{Output: result}
}

func (t *WriteTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.write.param_path_desc"),
			},
			"content": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.write.param_content_desc"),
			},
		},
		"required": []string{"path", "content"},
	}
}
