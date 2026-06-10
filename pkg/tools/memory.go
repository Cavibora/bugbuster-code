package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"bugbuster-code/pkg/i18n"
)

const (
	memoryMaxFacts    = 30
	memoryMaxValueLen = 2000
	memoryMaxBackups  = 3
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
func (t *MemoryTool) SetSessionID(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	newPath := MemoryFilePath(sessionID)
	if t.filePath != newPath {
		oldFacts := t.facts
		t.filePath = newPath
		t.loaded = false
		t.facts = nil
		if len(oldFacts) > 0 {
			t.facts = oldFacts
			t.saveToFileLocked()
		}
	}
}

// SetSessionIDForProject updates the session ID and file path using project directory.
func (t *MemoryTool) SetSessionIDForProject(sessionID, projectDir string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	newPath := MemoryFilePathForProject(sessionID, projectDir)
	if t.filePath != newPath {
		oldFacts := t.facts
		t.filePath = newPath
		t.loaded = false
		t.facts = nil
		if len(oldFacts) > 0 {
			t.facts = oldFacts
			t.saveToFileLocked()
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
	case "compress":
		return t.compress(params)
	case "restore":
		return t.restore()
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
				"enum":        []string{"save", "load", "list", "delete", "compress", "restore"},
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
			"max_tokens": map[string]any{
				"type":        "integer",
				"description": "Maximum tokens to keep after compression (for compress action)",
			},
		},
		"required": []string{"action"},
	}
}

// save stores a key-value fact with deduplication and limits
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

	// Truncate long values
	originalLen := len(value)
	if len(value) > memoryMaxValueLen {
		value = value[:memoryMaxValueLen] + fmt.Sprintf("... (truncated, original was %d chars)", originalLen)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.loadFromFile(); err != nil && !os.IsNotExist(err) {
		return Error("tools.memory.read_error", err)
	}

	// Check for exact duplicate value under different key
	for _, f := range t.facts {
		if f.Value == value && f.Key != key {
			// Update existing key instead of creating duplicate
			for i := range t.facts {
				if t.facts[i].Key == f.Key {
					t.facts[i].Value = value
					t.facts[i].Category = category
					t.facts[i].SavedAt = time.Now()
					break
				}
			}
			if err := t.saveToFileLocked(); err != nil {
				return Error("tools.memory.write_error", err)
			}
			return ToolResult{Output: fmt.Sprintf("✓ Same value already saved as '%s'. Updated that key instead of creating duplicate '%s'. Total: %d/%d facts", f.Key, key, len(t.facts), memoryMaxFacts)}
		}
	}

	// Warn about similar key (prefix match or Levenshtein ≤ 2)
	// Don't auto-update — let the model decide
	if similarKey := findSimilarMemoryKey(t.facts, key); similarKey != "" {
		// Just warn, don't auto-update
		// (Continue to save the new key below)
		_ = similarKey // suppress unused warning
	}

	// Update existing key or add new
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

	// Auto-compress if too many facts
	if len(t.facts) > memoryMaxFacts {
		t.compressInternal(memoryMaxFacts)
	}

	if err := t.saveToFileLocked(); err != nil {
		return Error("tools.memory.write_error", err)
	}

	return ToolResult{Output: fmt.Sprintf("✓ Saved '%s' (%d/%d facts). Category: %s", key, len(t.facts), memoryMaxFacts, category)}
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

	// Backup before delete
	t.backup()

	filtered := make([]MemoryFact, 0, len(t.facts))
	deleted := 0
	for _, f := range t.facts {
		if strings.EqualFold(f.Key, key) {
			deleted++
		} else {
			filtered = append(filtered, f)
		}
	}

	// Protect from mass deletion (but allow single key deletion)
	if len(t.facts) > 2 && deleted > len(t.facts)/2 {
		return ToolResult{Output: fmt.Sprintf("⚠️ Refusing to delete %d facts — that's more than half of all facts (%d total). Delete specific keys or use 'compress'.", deleted, len(t.facts))}
	}

	if deleted == 0 {
		return ToolResult{Output: fmt.Sprintf("Fact '%s' not found.", key)}
	}

	t.facts = filtered
	if err := t.saveToFileLocked(); err != nil {
		return Error("tools.memory.write_error", err)
	}

	return ToolResult{Output: fmt.Sprintf("✓ Deleted %d fact(s) with key '%s'. Remaining: %d/%d", deleted, key, len(t.facts), memoryMaxFacts)}
}

func (t *MemoryTool) compress(params map[string]string) ToolResult {
	maxTokens := 0
	if mt := strings.TrimSpace(params["max_tokens"]); mt != "" {
		if n, err := fmt.Sscanf(mt, "%d", &maxTokens); n != 1 || err != nil {
			maxTokens = 0
		}
	}
	return t.Compress(maxTokens)
}

// TokenCount estimates the number of tokens in the memory file.
func (t *MemoryTool) TokenCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if err := t.loadFromFile(); err != nil {
		return 0
	}

	content := renderMemoryMD(t.facts)
	return len(content) / 4
}

// Compress removes duplicate and low-priority facts.
func (t *MemoryTool) Compress(maxTokens int) ToolResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.loadFromFile(); err != nil && !os.IsNotExist(err) {
		return Error("tools.memory.read_error", err)
	}

	if len(t.facts) == 0 {
		return ToolResult{Output: "Memory is empty, nothing to compress."}
	}

	result := t.compressInternal(maxTokens)
	return ToolResult{Output: result}
}

// compressInternal removes low-priority facts (caller must hold t.mu)
func (t *MemoryTool) compressInternal(maxTokens int) string {
	originalCount := len(t.facts)

	// Backup before compress
	t.backup()

	// Step 1: Remove exact duplicates (same key + same value)
	seen := make(map[string]bool)
	deduped := make([]MemoryFact, 0, len(t.facts))
	for _, f := range t.facts {
		key := strings.ToLower(f.Key + "|" + f.Value)
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, f)
		}
	}

	// Step 2: Merge facts with same key (keep latest)
	merged := make(map[string]MemoryFact)
	var order []string
	for _, f := range deduped {
		key := strings.ToLower(f.Key)
		if existing, ok := merged[key]; ok {
			if f.SavedAt.After(existing.SavedAt) || len(f.Value) > len(existing.Value) {
				merged[key] = f
			}
		} else {
			merged[key] = f
			order = append(order, key)
		}
	}

	// Step 3: Remove duplicate values (keep first occurrence)
	valueSeen := make(map[string]bool)
	var unique []MemoryFact
	for _, key := range order {
		f := merged[key]
		h := hashMemoryValue(f.Value)
		if !valueSeen[h] {
			valueSeen[h] = true
			unique = append(unique, f)
		}
	}

	// Step 4: Sort by priority (critical > credentials > project > architecture > general)
	priorityCategories := map[string]int{
		"critical":     10,
		"credentials":  9,
		"project":      8,
		"architecture": 7,
		"permanent":    6,
	}

	sort.Slice(unique, func(i, j int) bool {
		pi, oki := priorityCategories[unique[i].Category]
		pj, okj := priorityCategories[unique[j].Category]
		if !oki {
			pi = 1
		}
		if !okj {
			pj = 1
		}
		if pi != pj {
			return pi > pj
		}
		return unique[i].SavedAt.After(unique[j].SavedAt)
	})

	// Step 5: If over maxTokens, keep only facts that fit
	if maxTokens > 0 {
		newTokens := len(renderMemoryMD(unique)) / 4
		if newTokens > maxTokens {
			var kept []MemoryFact
			currentTokens := 0
			for _, f := range unique {
				factTokens := (len(f.Key) + len(f.Value) + len(f.Category) + 10) / 4
				if currentTokens+factTokens <= maxTokens {
					kept = append(kept, f)
					currentTokens += factTokens
				}
			}
			unique = kept
		}
	}

	// Step 6: If still over maxFacts, keep only top maxFacts
	if len(unique) > memoryMaxFacts {
		unique = unique[:memoryMaxFacts]
	}

	t.facts = unique
	if err := t.saveToFileLocked(); err != nil {
		return fmt.Sprintf("Error saving compressed memory: %v", err)
	}

	removed := originalCount - len(t.facts)
	return fmt.Sprintf("✓ Compressed: %d → %d facts (removed %d duplicates/low-priority). Protected: critical, credentials, project, architecture.", originalCount, len(t.facts), removed)
}

// restore loads facts from the most recent backup
func (t *MemoryTool) restore() ToolResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	backupPath := t.filePath + ".bak.1"
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("No backup found at %s", backupPath)}
	}

	backupFacts := parseMemoryMD(string(data))

	// Merge: add backup facts that don't exist in current
	var added int
	for _, bf := range backupFacts {
		found := false
		for _, cf := range t.facts {
			if strings.EqualFold(cf.Key, bf.Key) {
				found = true
				break
			}
		}
		if !found {
			bf.Category = "restored"
			t.facts = append(t.facts, bf)
			added++
		}
	}

	if err := t.saveToFileLocked(); err != nil {
		return Error("tools.memory.write_error", err)
	}

	return ToolResult{Output: fmt.Sprintf("✓ Restored %d facts from backup. Total: %d facts", added, len(t.facts))}
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

// MemoryFilePath returns the path to the memory file for a session.
func MemoryFilePath(sessionID string) string {
	if cwd, err := os.Getwd(); err == nil {
		bbDir := filepath.Join(cwd, ".bugbuster")
		if info, err := os.Stat(bbDir); err == nil && info.IsDir() {
			return filepath.Join(cwd, ".bugbuster", "memory", sessionID+".md")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".bugbuster", "memory", sessionID+".md")
}

// MemoryFilePathForProject returns the path to the memory file for a session,
// using the given project directory as the first choice.
func MemoryFilePathForProject(sessionID, projectDir string) string {
	bbDir := filepath.Join(projectDir, ".bugbuster")
	if info, err := os.Stat(bbDir); err == nil && info.IsDir() {
		return filepath.Join(projectDir, ".bugbuster", "memory", sessionID+".md")
	}
	if cwd, err := os.Getwd(); err == nil {
		bbDir := filepath.Join(cwd, ".bugbuster")
		if info, err := os.Stat(bbDir); err == nil && info.IsDir() {
			return filepath.Join(cwd, ".bugbuster", "memory", sessionID+".md")
		}
	}
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

// saveToFileLocked saves facts to file (caller must hold t.mu)
func (t *MemoryTool) saveToFileLocked() error {
	dir := filepath.Dir(t.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	content := renderMemoryMD(t.facts)
	return os.WriteFile(t.filePath, []byte(content), 0644)
}

func (t *MemoryTool) saveToFile() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.saveToFileLocked()
}

// backup creates a rotating backup of the memory file (caller must hold t.mu)
func (t *MemoryTool) backup() {
	for i := memoryMaxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.bak.%d", t.filePath, i)
		newPath := fmt.Sprintf("%s.bak.%d", t.filePath, i+1)
		data, err := os.ReadFile(oldPath)
		if err == nil {
			os.WriteFile(newPath, data, 0644)
		}
	}
	data, err := os.ReadFile(t.filePath)
	if err == nil {
		os.WriteFile(t.filePath+".bak.1", data, 0644)
	}
}

// --- Helper functions ---

func findSimilarMemoryKey(facts []MemoryFact, key string) string {
	keyLower := strings.ToLower(key)
	for _, f := range facts {
		fLower := strings.ToLower(f.Key)
		if fLower == keyLower {
			continue // exact match, not similar
		}
		// Prefix match (e.g., "project_path" vs "project_root")
		if strings.HasPrefix(fLower, keyLower+"_") || strings.HasPrefix(keyLower, fLower+"_") {
			return f.Key
		}
		// Common prefix match (e.g., "project_path" vs "project_root" share "project_")
		commonLen := 0
		minLen := len(fLower)
		if len(keyLower) < minLen {
			minLen = len(keyLower)
		}
		for i := 0; i < minLen; i++ {
			if fLower[i] == keyLower[i] {
				commonLen++
			} else {
				break
			}
		}
		if commonLen >= 5 && commonLen >= len(fLower)/2 && commonLen >= len(keyLower)/2 {
			return f.Key
		}
		// Levenshtein distance ≤ 2 for short keys
		if len(key) <= 20 && levenshteinDist(fLower, keyLower) <= 2 {
			return f.Key
		}
	}
	return ""
}

func hashMemoryValue(v string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(v))))
	return hex.EncodeToString(h[:])
}

func levenshteinDist(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	d := make([][]int, la+1)
	for i := range d {
		d[i] = make([]int, lb+1)
		d[i][0] = i
	}
	for j := 1; j <= lb; j++ {
		d[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = minOf3(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
		}
	}
	return d[la][lb]
}

func minOf3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
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