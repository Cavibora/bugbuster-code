package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bugbuster-code/pkg/i18n"
)

// ReadTool is the file reading tool
type ReadTool struct {
	AllowedDirs      []string // allowed directories (empty = all)
	MaxSize          int64    // max. size file (bytes)
	AllowSecretFiles bool     // allow access to secret files (depends on permission_mode)
}

// NewReadTool creates a tool for reading file contents.
func NewReadTool() *ReadTool {
	return &ReadTool{
		MaxSize: 1024 * 1024, // 1MB default
	}
}

func (t *ReadTool) Name() string { return "read" }

func (t *ReadTool) Description() string {
	return i18n.T("tools.read.description")
}

func (t *ReadTool) Execute(params map[string]string) ToolResult {
	path, ok := params["path"]
	if !ok || path == "" {
		return Error("tools.read.param_required")
	}

	// Security: resolve and check path
	resolvedPath, err := ResolvePath(path)
	if err != nil {
		return Error("security.error", err)
	}
	path = resolvedPath

	// Security: check secret files
	var secretWarning string
	if isSecret, reason := SecretPathInfo(path); isSecret {
		if !t.AllowSecretFiles {
			return Error("security.secret_file", filepath.Base(path))
		}
		// Allow but add warning to result
		secretWarning = fmt.Sprintf("⚠️  Warning: %s — %s\n\n", filepath.Base(path), reason)
	}

	// Check allowed directories
	if !IsPathAllowed(path, t.AllowedDirs) {
		return Error("security.path_denied", path)
	}

	info, err := os.Stat(path)
	if err != nil {
		return Error("tools.read.file_not_found", path)
	}

	if info.IsDir() {
		// If directory — show file list
		entries, err := os.ReadDir(path)
		if err != nil {
			return Error("tools.read.dir_error", err)
		}
		var names []string
		for _, e := range entries {
			prefix := "  "
			if e.IsDir() {
				prefix = "📁 "
			}
			names = append(names, prefix+e.Name())
		}
		return Success("tools.read.dir_listing", path, strings.Join(names, "\n"))
	}

	if info.Size() > t.MaxSize {
		return Error("tools.read.file_too_large", info.Size(), t.MaxSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Error("tools.read.file_error", err)
	}

	content := string(data)
	if secretWarning != "" {
		content = secretWarning + content
	}

	return Success("%s", content)
}

func (t *ReadTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.read.param_path"),
			},
		},
		"required": []string{"path"},
	}
}
