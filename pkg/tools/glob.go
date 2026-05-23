package tools

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"bugbuster-code/pkg/i18n"
)

// GlobTool is the file pattern search tool
type GlobTool struct {
	AllowedDirs []string
	MaxResults  int
}

// NewGlobTool creates a tool for finding files matching glob patterns.
// NewGlobTool creates a tool for finding files matching glob patterns.
func NewGlobTool() *GlobTool {
	return &GlobTool{
		MaxResults: 100,
	}
}

func (t *GlobTool) Name() string { return "glob" }

func (t *GlobTool) Description() string {
	return i18n.T("tools.glob.description")
}

func (t *GlobTool) Execute(params map[string]string) ToolResult {
	pattern, ok := params["pattern"]
	if !ok || pattern == "" {
		return Error("tools.glob.param_pattern")
	}

	searchPath := "."
	if p, ok := params["path"]; ok && p != "" {
		searchPath = filepath.Clean(p)
	}

	// Security
	if strings.HasPrefix(searchPath, "..") {
		return Error("security.path_traversal")
	}

	if len(t.AllowedDirs) > 0 {
		allowed := false
		for _, dir := range t.AllowedDirs {
			if strings.HasPrefix(searchPath, dir) {
				allowed = true
				break
			}
		}
		if !allowed {
			return Error("security.dir_denied", searchPath)
		}
	}

	// If pattern contains **, use recursive traversal
	var matches []string
	var err error

	if strings.Contains(pattern, "**") {
		// Recursive search
		err = filepath.WalkDir(searchPath, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				// Skip hidden directories
				if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
					return filepath.SkipDir
				}
				return nil
			}

			// Check pattern match
			matched, mErr := filepath.Match(strings.ReplaceAll(pattern, "**/", ""), filepath.Base(path))
			if mErr == nil && matched {
				matches = append(matches, path)
				if len(matches) >= t.MaxResults {
					return fmt.Errorf("max results reached")
				}
			}
			// Also check extension match
			ext := filepath.Ext(path)
			extPattern := filepath.Ext(pattern)
			if extPattern != "" && ext == extPattern && !strings.Contains(pattern, "/") {
				matches = append(matches, path)
				if len(matches) >= t.MaxResults {
					return fmt.Errorf("max results reached")
				}
			}
			return nil
		})
	} else {
		// Simple glob
		fullPattern := filepath.Join(searchPath, pattern)
		matches, err = filepath.Glob(fullPattern)
	}

	if err != nil && len(matches) < t.MaxResults {
		return Error("tools.glob.search_error", err)
	}

	if len(matches) == 0 {
		return Success("tools.glob.no_files")
	}

	// Remove duplicates
	seen := make(map[string]bool)
	var unique []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}

	if len(unique) > t.MaxResults {
		unique = unique[:t.MaxResults]
		unique = append(unique, fmt.Sprintf(i18n.T("tools.glob.truncated"), len(matches)))
	}

	return Success("%s", strings.Join(unique, "\n"))
}

func (t *GlobTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.glob.param_pattern_desc"),
			},
			"path": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.glob.param_path_desc"),
			},
		},
		"required": []string{"pattern"},
	}
}
