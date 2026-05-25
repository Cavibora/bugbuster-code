package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"bugbuster-code/pkg/i18n"
)

// MemoryTool stores important project facts that persist across sessions.
// Each session has its own memory file (.bugbuster/memory/<session-id>.md).
// Facts are automatically loaded into the system prompt at session start.
type MemoryTool struct {
	mu       sync.RWMutex
	filePath string
	facts    []MemoryFact
	loaded   bool
}

// MemoryFact represents a single stored fact
type MemoryFact struct {
	Key      string    // Fact identifier (e.g. "project_path", "mysql_host")
	Value    string    // Fact value
	Category string    // Group (e.g. "project", "database", "metrics")
	SavedAt  time.Time // When saved
}

// NewMemoryTool creates a new memory tool for a specific session
func NewMemoryTool(sessionID string) *MemoryTool {
	home, _ := os.UserHomeDir()
	memoryDir := filepath.Join(home, ".bugbuster", "memory")
	return &MemoryTool{
		filePath: filepath.Join(memoryDir, sessionID+".md"),
	}
}

// NewMemoryToolWithPath creates a memory tool with explicit path (for testing)
func NewMemoryToolWithPath(filePath string) *MemoryTool {
	return &MemoryTool{filePath: filePath}
}

// SetSessionID updates the session ID and file path.
// Call this after session creation to associate memory with the correct session.
func (t *MemoryTool) SetSessionID(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	home, _ := os.UserHomeDir()
	memoryDir := filepath.Join(home, ".bugbuster", "memory")
	newPath := filepath.Join(memoryDir, sessionID+".md")
	// If old file exists with facts, migrate them to new file
	if t.filePath != newPath {
		oldFacts := t.facts
		t.filePath = newPath
		t.loaded = false
		t.facts = nil
		// Save old facts to new file
		if len(oldFacts) > 0 {
			t.facts = oldFacts
			t.saveToFile()
		}
	}
}

func (t *MemoryTool) Name() string { return "memory" }

func (t *MemoryTool) Description() string {
	return i18n.T("tools.memory.description")
}

func (t *MemoryTool) Execute(params map[string]string) ToolResult {
	action := params["action"]
	switch action {
	case "save":
		return t.save(params)
	case "load":
		return t.load(params)
	case "list":
		return t.list()
	case "delete":
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

func (t *MemoryTool) save(params map[string]string) ToolResult {
	key := strings.TrimSpace(params["key"])
	value := strings.TrimSpace(params["value"])
	category := strings.TrimSpace(params["category"])

	if key == "" || value == "" {
		return Error("tools.memory.save_empty")
	}
	if category == "" {
		category = "general"
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.loadFromFile(); err != nil && !os.IsNotExist(err) {
		return Error("tools.memory.read_error", err)
	}

	// Update existing or add new
	found := false
	for i, f := range t.facts {
		if strings.EqualFold(f.Key, key) {
			t.facts[i].Value = value
			t.facts[i].Category = category
			t.facts[i].SavedAt = time.Now()
			found = true
			break
		}
	}
	if !found {
		t.facts = append(t.facts, MemoryFact{
			Key:      key,
			Value:    value,
			Category: category,
			SavedAt:  time.Now(),
		})
	}

	if err := t.saveToFile(); err != nil {
		return Error("tools.memory.write_error", err)
	}

	return ToolResult{Output: fmt.Sprintf("✓ Saved: %s = %s [%s]", key, value, category)}
}

func (t *MemoryTool) load(params map[string]string) ToolResult {
	key := strings.TrimSpace(params["key"])
	category := strings.TrimSpace(params["category"])

	t.mu.RLock()
	defer t.mu.RUnlock()

	if err := t.loadFromFile(); err != nil && !os.IsNotExist(err) {
		return Error("tools.memory.read_error", err)
	}

	var matched []MemoryFact
	for _, f := range t.facts {
		if key != "" && strings.Contains(strings.ToLower(f.Key), strings.ToLower(key)) {
			matched = append(matched, f)
		} else if category != "" && strings.EqualFold(f.Category, category) {
			matched = append(matched, f)
		}
	}

	if key == "" && category == "" {
		matched = t.facts
	}

	if len(matched) == 0 {
		return ToolResult{Output: "No matching facts found."}
	}

	return ToolResult{Output: formatFacts(matched)}
}

func (t *MemoryTool) list() ToolResult {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if err := t.loadFromFile(); err != nil && !os.IsNotExist(err) {
		return Error("tools.memory.read_error", err)
	}

	if len(t.facts) == 0 {
		return ToolResult{Output: "Memory is empty. Use 'save' to store facts."}
	}

	return ToolResult{Output: formatFacts(t.facts)}
}

func (t *MemoryTool) delete(params map[string]string) ToolResult {
	key := strings.TrimSpace(params["key"])
	if key == "" {
		return Error("tools.memory.delete_empty")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.loadFromFile(); err != nil && !os.IsNotExist(err) {
		return Error("tools.memory.read_error", err)
	}

	filtered := make([]MemoryFact, 0, len(t.facts))
	deleted := 0
	for _, f := range t.facts {
		if strings.EqualFold(f.Key, key) {
			deleted++
		} else {
			filtered = append(filtered, f)
		}
	}

	if deleted == 0 {
		return ToolResult{Output: fmt.Sprintf("Fact '%s' not found.", key)}
	}

	t.facts = filtered
	if err := t.saveToFile(); err != nil {
		return Error("tools.memory.write_error", err)
	}

	return ToolResult{Output: fmt.Sprintf("✓ Deleted %d fact(s) with key '%s'", deleted, key)}
}

// LoadAllFacts returns all stored facts as formatted text (for system prompt injection)
func (t *MemoryTool) LoadAllFacts() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if err := t.loadFromFile(); err != nil {
		return ""
	}
	if len(t.facts) == 0 {
		return ""
	}

	return formatFactsForPrompt(t.facts)
}

// MemoryFilePath returns the session-scoped memory file path.
func MemoryFilePath(sessionID string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".bugbuster", "memory", sessionID+".md")
}

// --- File I/O ---

func (t *MemoryTool) loadFromFile() error {
	if t.loaded {
		return nil
	}

	data, err := os.ReadFile(t.filePath)
	if err != nil {
		return err
	}

	t.facts = parseMemoryMD(string(data))
	t.loaded = true
	return nil
}

func (t *MemoryTool) saveToFile() error {
	dir := filepath.Dir(t.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	content := renderMemoryMD(t.facts)
	return os.WriteFile(t.filePath, []byte(content), 0644)
}

// --- Markdown parsing/rendering ---

func parseMemoryMD(content string) []MemoryFact {
	var facts []MemoryFact
	var currentCategory string

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Category header: ## category_name
		if strings.HasPrefix(line, "## ") {
			currentCategory = strings.TrimPrefix(line, "## ")
			currentCategory = strings.TrimSpace(currentCategory)
			continue
		}

		// Fact line: - **key**: value
		if strings.HasPrefix(line, "- **") {
			rest := strings.TrimPrefix(line, "- **")
			end := strings.Index(rest, "**")
			if end == -1 {
				continue
			}
			key := rest[:end]
			value := strings.TrimSpace(rest[end+2:])
			if strings.HasPrefix(value, ": ") {
				value = value[2:]
			}

			facts = append(facts, MemoryFact{
				Key:      key,
				Value:    value,
				Category: currentCategory,
			})
		}
	}

	return facts
}

func renderMemoryMD(facts []MemoryFact) string {
	// Group by category
	groups := make(map[string][]MemoryFact)
	var categories []string
	for _, f := range facts {
		if _, ok := groups[f.Category]; !ok {
			categories = append(categories, f.Category)
		}
		groups[f.Category] = append(groups[f.Category], f)
	}
	sort.Strings(categories)

	var sb strings.Builder
	sb.WriteString("# BugBuster Agent Memory\n\n")

	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("## %s\n", cat))
		for _, f := range groups[cat] {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", f.Key, f.Value))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatFacts(facts []MemoryFact) string {
	// Group by category
	groups := make(map[string][]MemoryFact)
	var categories []string
	for _, f := range facts {
		if _, ok := groups[f.Category]; !ok {
			categories = append(categories, f.Category)
		}
		groups[f.Category] = append(groups[f.Category], f)
	}
	sort.Strings(categories)

	var sb strings.Builder
	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("[%s]\n", cat))
		for _, f := range groups[cat] {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", f.Key, f.Value))
		}
	}
	return sb.String()
}

func formatFactsForPrompt(facts []MemoryFact) string {
	// Group by category
	groups := make(map[string][]MemoryFact)
	var categories []string
	for _, f := range facts {
		if _, ok := groups[f.Category]; !ok {
			categories = append(categories, f.Category)
		}
		groups[f.Category] = append(groups[f.Category], f)
	}
	sort.Strings(categories)

	var sb strings.Builder
	sb.WriteString("Important facts about this project (from agent memory):\n\n")
	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("[%s]\n", cat))
		for _, f := range groups[cat] {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", f.Key, f.Value))
		}
	}
	return sb.String()
}
