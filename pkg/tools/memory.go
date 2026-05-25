package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"bugbuster-code/pkg/i18n"
)

// MemoryTool stores and retrieves important facts about the project
// that the model should remember across the session.
// Data is persisted in .bugbuster/memory.md
type MemoryTool struct {
	mu       sync.Mutex
	filePath string
}

// NewMemoryTool creates a new memory tool.
// filePath is the path to the memory file (typically .bugbuster/memory.md).
func NewMemoryTool(filePath string) *MemoryTool {
	if filePath == "" {
		filePath = ".bugbuster/memory.md"
	}
	return &MemoryTool{filePath: filePath}
}

func (t *MemoryTool) Name() string { return "memory" }

func (t *MemoryTool) Description() string {
	return i18n.T("tools.memory.description")
}

func (t *MemoryTool) Execute(params map[string]string) ToolResult {
	action := params["action"]
	switch action {
	case "save", "remember", "store":
		return t.save(params)
	case "load", "recall", "search":
		return t.load(params)
	case "list":
		return t.list()
	case "delete", "remove":
		return t.delete(params)
	default:
		return Error("tools.memory.unknown_action", action)
	}
}

func (t *MemoryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"save", "load", "list", "delete"},
				"description": i18n.T("tools.memory.param_action_desc"),
			},
			"key": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.memory.param_key_desc"),
			},
			"value": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.memory.param_value_desc"),
			},
			"category": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.memory.param_category_desc"),
			},
		},
		"required": []string{"action"},
	}
}

// memoryEntry represents a single memory entry
type memoryEntry struct {
	Category string
	Key      string
	Value    string
}

// save stores a key-value pair in the memory file
func (t *MemoryTool) save(params map[string]string) ToolResult {
	key := strings.TrimSpace(params["key"])
	value := strings.TrimSpace(params["value"])
	category := strings.TrimSpace(params["category"])

	if key == "" {
		return Error("tools.memory.param_key")
	}
	if value == "" {
		return Error("tools.memory.param_value")
	}
	if category == "" {
		category = "general"
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Load existing entries
	entries := t.loadEntries()

	// Update or add entry
	found := false
	for i, e := range entries {
		if strings.EqualFold(e.Key, key) {
			entries[i] = memoryEntry{Category: category, Key: key, Value: value}
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, memoryEntry{Category: category, Key: key, Value: value})
	}

	// Save to file
	if err := t.saveEntries(entries); err != nil {
		return Error("tools.memory.save_error", err)
	}

	return Success("tools.memory.saved", key, category)
}

// load retrieves entries from the memory file
func (t *MemoryTool) load(params map[string]string) ToolResult {
	key := strings.TrimSpace(params["key"])
	category := strings.TrimSpace(params["category"])

	t.mu.Lock()
	defer t.mu.Unlock()

	entries := t.loadEntries()
	if len(entries) == 0 {
		return Success("tools.memory.empty")
	}

	var results []memoryEntry
	for _, e := range entries {
		if key != "" && !strings.Contains(strings.ToLower(e.Key), strings.ToLower(key)) &&
			!strings.Contains(strings.ToLower(e.Value), strings.ToLower(key)) {
			continue
		}
		if category != "" && !strings.EqualFold(e.Category, category) {
			continue
		}
		results = append(results, e)
	}

	if len(results) == 0 {
		return Success("tools.memory.not_found", key)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d entries:\n\n", len(results)))
	for _, e := range results {
		sb.WriteString(fmt.Sprintf("**[%s]** %s: %s\n", e.Category, e.Key, e.Value))
	}

	return ToolResult{Output: sb.String()}
}

// list returns all memory entries grouped by category
func (t *MemoryTool) list() ToolResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	entries := t.loadEntries()
	if len(entries) == 0 {
		return Success("tools.memory.empty")
	}

	// Group by category
	groups := make(map[string][]memoryEntry)
	var categories []string
	for _, e := range entries {
		if _, ok := groups[e.Category]; !ok {
			categories = append(categories, e.Category)
		}
		groups[e.Category] = append(groups[e.Category], e)
	}
	sort.Strings(categories)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Memory: %d entries in %d categories\n\n", len(entries), len(categories)))
	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("## %s\n", cat))
		for _, e := range groups[cat] {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", e.Key, e.Value))
		}
		sb.WriteString("\n")
	}

	return ToolResult{Output: sb.String()}
}

// delete removes an entry from the memory file
func (t *MemoryTool) delete(params map[string]string) ToolResult {
	key := strings.TrimSpace(params["key"])
	if key == "" {
		return Error("tools.memory.param_key")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	entries := t.loadEntries()
	var filtered []memoryEntry
	deleted := 0
	for _, e := range entries {
		if strings.EqualFold(e.Key, key) {
			deleted++
			continue
		}
		filtered = append(filtered, e)
	}

	if deleted == 0 {
		return Success("tools.memory.not_found", key)
	}

	if err := t.saveEntries(filtered); err != nil {
		return Error("tools.memory.save_error", err)
	}

	return Success("tools.memory.deleted", key, deleted)
}

// loadEntries reads all entries from the memory file
func (t *MemoryTool) loadEntries() []memoryEntry {
	data, err := os.ReadFile(t.filePath)
	if err != nil {
		return nil
	}

	var entries []memoryEntry
	var currentCategory string
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Category header: ## Category Name
		if strings.HasPrefix(line, "## ") {
			currentCategory = strings.TrimPrefix(line, "## ")
			currentCategory = strings.TrimSpace(currentCategory)
			continue
		}

		// Entry: - **key**: value
		if strings.HasPrefix(line, "- **") {
			// Parse: - **key**: value
			rest := strings.TrimPrefix(line, "- **")
			idx := strings.Index(rest, "**")
			if idx < 0 {
				continue
			}
			key := strings.TrimSpace(rest[:idx])
			value := strings.TrimSpace(rest[idx+2:])
			if strings.HasPrefix(value, ":") {
				value = strings.TrimSpace(strings.TrimPrefix(value, ":"))
			}
			if key != "" {
				entries = append(entries, memoryEntry{
					Category: currentCategory,
					Key:      key,
					Value:    value,
				})
			}
		}
	}

	return entries
}

// saveEntries writes all entries to the memory file
func (t *MemoryTool) saveEntries(entries []memoryEntry) error {
	// Ensure directory exists
	dir := filepath.Dir(t.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Group by category
	groups := make(map[string][]memoryEntry)
	var categories []string
	for _, e := range entries {
		if _, ok := groups[e.Category]; !ok {
			categories = append(categories, e.Category)
		}
		groups[e.Category] = append(groups[e.Category], e)
	}
	sort.Strings(categories)

	var sb strings.Builder
	sb.WriteString("# BugBuster Agent Memory\n")
	sb.WriteString("# This file stores important facts about the project.\n")
	sb.WriteString("# The agent reads this file at startup and updates it via the memory tool.\n")
	sb.WriteString("# DO NOT edit manually unless you know what you're doing.\n\n")

	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("## %s\n", cat))
		for _, e := range groups[cat] {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", e.Key, e.Value))
		}
		sb.WriteString("\n")
	}

	return os.WriteFile(t.filePath, []byte(sb.String()), 0644)
}

// LoadMemoryForPrompt loads all memory entries and returns them as a string
// suitable for inclusion in the system prompt.
// This is called at session startup to inject known facts.
func LoadMemoryForPrompt(filePath string) string {
	tool := &MemoryTool{filePath: filePath}
	entries := tool.loadEntries()
	if len(entries) == 0 {
		return ""
	}

	// Group by category
	groups := make(map[string][]memoryEntry)
	var categories []string
	for _, e := range entries {
		if _, ok := groups[e.Category]; !ok {
			categories = append(categories, e.Category)
		}
		groups[e.Category] = append(groups[e.Category], e)
	}
	sort.Strings(categories)

	var sb strings.Builder
	sb.WriteString("## Important Project Facts (from memory)\n")
	sb.WriteString("These facts were previously saved as important. Use them and keep them updated.\n\n")

	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("### %s\n", cat))
		for _, e := range groups[cat] {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", e.Key, e.Value))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Use the `memory` tool with action `save` to update these facts when they change.\n")

	return sb.String()
}
