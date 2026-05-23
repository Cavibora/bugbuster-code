package tools

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"bugbuster-code/pkg/i18n"
)

// GrepTool is the content search tool for files
type GrepTool struct {
	AllowedDirs []string
	MaxResults  int
}

// NewGrepTool creates a tool for searching file contents using regular expressions.
// NewGrepTool creates a tool for searching file contents using regular expressions.
func NewGrepTool() *GrepTool {
	return &GrepTool{
		MaxResults: 50,
	}
}

func (t *GrepTool) Name() string { return "grep" }

func (t *GrepTool) Description() string {
	return i18n.T("tools.grep.description")
}

func (t *GrepTool) Execute(params map[string]string) ToolResult {
	pattern, ok := params["pattern"]
	if !ok || pattern == "" {
		return Error("tools.grep.param_pattern")
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

	// Compile regular expression
	flags := ""
	if params["case_insensitive"] == "true" {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + pattern)
	if err != nil {
		return Error("tools.grep.regex_error", err)
	}

	filePattern := params["file_pattern"]
	if filePattern == "" {
		filePattern = "*" // all files
	}

	var results []string
	count := 0

	err = filepath.WalkDir(searchPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Skip hidden directories and .git
			name := d.Name()
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Check file pattern
		if filePattern != "*" {
			matched, _ := filepath.Match(filePattern, d.Name())
			if !matched {
				return nil
			}
		}

		// Skip binary files
		if isBinary(path) {
			return nil
		}

		// Limit file size (10MB)
		if info.Size() > 10*1024*1024 {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if re.MatchString(line) {
				// Truncate long lines
				displayLine := line
				if len(displayLine) > 200 {
					displayLine = displayLine[:200] + "..."
				}
				results = append(results, fmt.Sprintf("%s:%d: %s", path, lineNum, displayLine))
				count++
				if count >= t.MaxResults {
					return fmt.Errorf("max results reached")
				}
			}
		}
		return nil
	})

	if count >= t.MaxResults {
		results = append(results, fmt.Sprintf(i18n.T("tools.grep.truncated"), t.MaxResults))
	}

	if len(results) == 0 {
		return Success("tools.grep.no_matches")
	}

	return Success("%s", strings.Join(results, "\n"))
}

// isBinary checks if a file is binary
func isBinary(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return true
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil {
		return true
	}

	// Check for null bytes
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}
	return false
}

func (t *GrepTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.grep.param_pattern_desc"),
			},
			"path": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.grep.param_path_desc"),
			},
			"file_pattern": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.grep.param_glob_desc"),
			},
			"case_insensitive": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.grep.param_ignore_case_desc"),
			},
		},
		"required": []string{"pattern"},
	}
}
