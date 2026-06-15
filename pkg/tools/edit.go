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
	AllowSecretFiles bool
}

func NewEditTool() *EditTool {
	return &EditTool{}
}

func (t *EditTool) Name() string { return "edit" }

func (t *EditTool) Description() string {
	return i18n.T("tools.edit.description")
}

// backupFile creates a .bak copy of the file before editing.
func backupFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for i := 3; i >= 2; i-- {
		oldBak := fmt.Sprintf("%s.bak.%d", path, i-1)
		newBak := fmt.Sprintf("%s.bak.%d", path, i)
		if _, err := os.Stat(oldBak); err == nil {
			os.Rename(oldBak, newBak)
		}
	}
	return os.WriteFile(path+".bak.1", data, 0644)
}

// normalizeWhitespace normalizes whitespace for fuzzy matching.
func normalizeWhitespace(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\t", "    ")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n")
}

// fuzzyFindOld tries to find oldText in content with fuzzy matching.
// Returns the actual text found in content (with original whitespace).
func fuzzyFindOld(content, oldText string) (string, bool) {
	// 1. Exact match
	if strings.Contains(content, oldText) {
		return oldText, true
	}

	// 2. Normalized match (whitespace differences)
	normalizedContent := normalizeWhitespace(content)
	normalizedOld := normalizeWhitespace(oldText)

	if strings.Contains(normalizedContent, normalizedOld) {
		idx := strings.Index(normalizedContent, normalizedOld)
		if idx < 0 {
			return "", false
		}

		// Map back to original content
		origIdx := 0
		normIdx := 0
		for origIdx < len(content) && normIdx < idx {
			if content[origIdx] == normalizedContent[normIdx] {
				origIdx++
				normIdx++
			} else if content[origIdx] == '\r' {
				origIdx++
			} else if content[origIdx] == '\t' {
				origIdx++
				normIdx += 4
			} else {
				origIdx++
				normIdx++
			}
		}

		endNormIdx := idx + len(normalizedOld)
		origEndIdx := origIdx
		for origEndIdx < len(content) && normIdx < endNormIdx {
			if content[origEndIdx] == normalizedContent[normIdx] {
				origEndIdx++
				normIdx++
			} else if content[origEndIdx] == '\r' {
				origEndIdx++
			} else if content[origEndIdx] == '\t' {
				origEndIdx++
				normIdx += 4
			} else {
				origEndIdx++
				normIdx++
			}
		}

		if origIdx < len(content) && origEndIdx <= len(content) {
			return content[origIdx:origEndIdx], true
		}
	}

	// 3. Line-by-line fuzzy match
	contentLines := strings.Split(content, "\n")
	oldLines := strings.Split(oldText, "\n")

	if len(oldLines) == 0 || len(contentLines) == 0 {
		return "", false
	}

	firstOld := strings.TrimSpace(oldLines[0])
	lastOld := strings.TrimSpace(oldLines[len(oldLines)-1])

	bestStart := -1
	bestScore := 0

	for i := 0; i <= len(contentLines)-len(oldLines); i++ {
		score := 0
		if strings.TrimSpace(contentLines[i]) == firstOld {
			score += 2
		} else if strings.Contains(strings.TrimSpace(contentLines[i]), firstOld) || strings.Contains(firstOld, strings.TrimSpace(contentLines[i])) {
			score += 1
		}
		lastIdx := i + len(oldLines) - 1
		if lastIdx < len(contentLines) {
			if strings.TrimSpace(contentLines[lastIdx]) == lastOld {
				score += 2
			} else if strings.Contains(strings.TrimSpace(contentLines[lastIdx]), lastOld) || strings.Contains(lastOld, strings.TrimSpace(contentLines[lastIdx])) {
				score += 1
			}
		}
		matches := 0
		for j := 0; j < len(oldLines) && i+j < len(contentLines); j++ {
			if strings.TrimSpace(contentLines[i+j]) == strings.TrimSpace(oldLines[j]) {
				matches++
			}
		}
		score += matches

		if score > bestScore {
			bestScore = score
			bestStart = i
		}
	}

	if bestStart >= 0 && bestScore >= len(oldLines)/2 {
		endIdx := bestStart + len(oldLines)
		if endIdx > len(contentLines) {
			endIdx = len(contentLines)
		}
		return strings.Join(contentLines[bestStart:endIdx], "\n"), true
	}

	return "", false
}

// detectDuplicateBlocks checks if the text has suspicious duplicate blocks.
func detectDuplicateBlocks(text string) bool {
	lines := strings.Split(text, "\n")
	if len(lines) < 6 {
		return false
	}

	// Check for 4+ consecutive identical non-empty lines
	consecutive := 1
	for i := 1; i < len(lines); i++ {
		if lines[i] == lines[i-1] && strings.TrimSpace(lines[i]) != "" {
			consecutive++
			if consecutive >= 4 {
				return true
			}
		} else {
			consecutive = 1
		}
	}

	// Check for duplicate blocks (3+ lines appearing twice in a row)
	for i := 0; i < len(lines)-5; i++ {
		blockSize := 3
		if i+blockSize*2 > len(lines) {
			break
		}
		block1 := strings.Join(lines[i:i+blockSize], "\n")
		block2 := strings.Join(lines[i+blockSize:i+blockSize*2], "\n")
		if block1 == block2 && strings.TrimSpace(block1) != "" {
			return true
		}
	}

	return false
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

	// Try exact match first, then fuzzy matching
	actualOld := oldText
	found := false

	if strings.Contains(content, oldText) {
		found = true
	} else {
		actualOld, found = fuzzyFindOld(content, oldText)
	}

	if !found {
		return ToolResult{
			Output: fmt.Sprintf("❌ Text not found in %s\n\nHint: The text to replace was not found exactly. This can happen due to:\n- Whitespace differences (tabs vs spaces)\n- Line ending differences (\\r\\n vs \\n)\n- The text was already edited in a previous step\n\nUse the 'read' tool to see the current file content.", path),
			Error:  "text not found",
		}
	}

	// Safety: don't delete more than 50% of the file
	oldLines := len(strings.Split(actualOld, "\n"))
	newLines := len(strings.Split(newText, "\n"))
	totalLines := len(strings.Split(content, "\n"))

	if totalLines > 10 && oldLines > totalLines/2 && newLines < oldLines/2 {
		return ToolResult{
			Output: fmt.Sprintf("⚠️ Safety: refusing to delete %d lines (>%d%% of file). Use 'write' tool if you really want to replace most of the file.", oldLines, (oldLines*100)/totalLines),
			Error:  "safety: too many lines deleted",
		}
	}

	// Create backup before editing
	backupFile(path)

	// Replace first occurrence of actual matched text
	newContent := strings.Replace(content, actualOld, newText, 1)

	// Validate: file should not become empty
	if len(newContent) == 0 && len(content) > 0 {
		return ToolResult{
			Output: "⚠️ Safety: refusing to make file empty. Use 'write' tool if you really want to clear the file.",
			Error:  "safety: file would be empty",
		}
	}

	// Validate: no duplicate blocks (sign of broken edit)
	if detectDuplicateBlocks(newContent) {
		return ToolResult{
			Output: "⚠️ Safety: edit would create duplicate blocks in the file. The 'old' text may match multiple locations. Please use 'read' to check the file and provide a more specific 'old' text.",
			Error:  "safety: duplicate blocks detected",
		}
	}

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return Error("tools.edit.write_error", path, err)
	}

	// Generate diff to show changes
	diff := UnifiedDiff(path, path, content, newContent)
	added, removed := DiffStats(diff)

	// Build model-friendly result: show what changed clearly
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✅ %s edited (+%d/-%d lines)\n\n", path, added, removed))

	if added == 0 && removed == 0 {
		sb.WriteString("(no visible changes)\n")
	} else {
		sb.WriteString("Changes applied successfully. The file now contains the new content.\n")
	}

	// Show compact diff for reference (this is a CHANGE LOG, not file content)
	if diff != "" {
		diffLines := DiffLines(diff, 15)
		if len(diffLines) > 0 {
			sb.WriteString("\nChange log (NOT file content — only shows what changed):\n")
			for _, line := range diffLines {
				sb.WriteString(line + "\n")
			}
		}
	}

	return ToolResult{Output: sb.String()}
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
