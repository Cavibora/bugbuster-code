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
)

// MemoryTool stores important project facts that persist across sessions.
// Each session has its own memory file (.bugbuster/memory/<session-id>.md).
// Facts are automatically loaded into the system prompt at session start.
//
// IMPORTANT RULES:
// - Save ONLY essential facts: paths, credentials, config, critical rules
// - Do NOT save: temporary data, progress, test results, verbose descriptions
// - Max 30 facts, max 2000 chars per value
// - Use 'compress' to remove duplicates and outdated facts
// - Category "permanent" or key prefix "!" means CANNOT be deleted or overwritten
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

// IsPermanent returns true if this fact cannot be deleted or overwritten
func (f *MemoryFact) IsPermanent() bool {
	return isPermanentCategory(f.Category)
}

// isPermanentCategory returns true if the category marks a fact as permanent
func isPermanentCategory(category string) bool {
	return strings.EqualFold(category, "permanent") ||
		strings.EqualFold(category, "critical")
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
				"enum":        []string{"save", "load", "list", "delete", "compress"},
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

	// Normalize: strip "!" prefix from key for storage, but mark as permanent
	isPermanentKey := strings.HasPrefix(key, "!")
	if isPermanentKey {
		key = strings.TrimPrefix(key, "!")
		if category == "general" {
			category = "permanent"
		}
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

	// Check if trying to overwrite a permanent fact with a different value
	for _, f := range t.facts {
		if strings.EqualFold(f.Key, key) && f.IsPermanent() && f.Value != value {
			return ToolResult{Output: fmt.Sprintf("🔒 Cannot overwrite permanent fact '%s'. Current value: %s. To change it, first delete it manually from the memory file.", key, truncateStr(f.Value, 80))}
		}
	}

	// Check for exact duplicate value under different key
	for _, f := range t.facts {
		if f.Value == value && f.Key != key {
			// Don't overwrite permanent facts
			if f.IsPermanent() {
				return ToolResult{Output: fmt.Sprintf("🔒 Same value already saved as permanent fact '%s'. Cannot overwrite.", f.Key)}
			}
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
	if similarKey := findSimilarMemoryKey(t.facts, key); similarKey != "" {
		// Check if similar key is permanent
		for _, f := range t.facts {
			if strings.EqualFold(f.Key, similarKey) && f.IsPermanent() {
				return ToolResult{Output: fmt.Sprintf("🔒 Similar permanent fact '%s' already exists. Cannot create similar key '%s'.", similarKey, key)}
			}
		}
	}

	// Update existing key or add new
	found := false
	for i, f := range t.facts {
		if strings.EqualFold(f.Key, key) {
			// Never downgrade a permanent fact's category
			if f.IsPermanent() && !isPermanentCategory(category) {
				// Keep the permanent category, only update value if same (no-op)
				// Do not modify the fact at all
				found = true
				break
			}
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

	permMark := ""
	if category == "permanent" || category == "critical" || category == "credentials" || category == "project" || isPermanentKey {
		permMark = " 🔒"
	}
	return ToolResult{Output: fmt.Sprintf("✓ Saved '%s' (%d/%d facts). Category: %s%s", key, len(t.facts), memoryMaxFacts, category, permMark)}
}

func (t *MemoryTool) load(params map[string]string) ToolResult {
	key := strings.TrimSpace(params["key"])
	category := strings.TrimSpace(params["category"])

	// Normalize: strip "!" prefix from key
	if strings.HasPrefix(key, "!") {
		key = strings.TrimPrefix(key, "!")
	}

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

	// Normalize: strip "!" prefix from key
	isPermanentKey := strings.HasPrefix(key, "!")
	if isPermanentKey {
		key = strings.TrimPrefix(key, "!")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.loadFromFile(); err != nil && !os.IsNotExist(err) {
		return Error("tools.memory.read_error", err)
	}

	// Check if trying to delete a permanent fact
	for _, f := range t.facts {
		if strings.EqualFold(f.Key, key) && f.IsPermanent() {
			return ToolResult{Output: fmt.Sprintf("🔒 Cannot delete permanent fact '%s' (category: %s). Remove the 'permanent'/'critical' category or '!' prefix from the memory file manually if you really need to delete it.", key, f.Category)}
		}
	}

	// Backup before delete (in .bugbuster/backups/, not next to memory file)
	// ChangeTracker handles undo, no need for .bak files

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

	// Backup before compress (in .bugbuster/backups/, not next to memory file)
	// ChangeTracker handles undo, no need for .bak files

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

	// Step 2: Merge facts with same key (keep latest, but never overwrite permanent)
	merged := make(map[string]MemoryFact)
	var order []string
	for _, f := range deduped {
		key := strings.ToLower(f.Key)
		if existing, ok := merged[key]; ok {
			// Never replace permanent fact with non-permanent
			if existing.IsPermanent() && !f.IsPermanent() {
				continue
			}
			if f.SavedAt.After(existing.SavedAt) || len(f.Value) > len(existing.Value) {
				merged[key] = f
			}
		} else {
			merged[key] = f
			order = append(order, key)
		}
	}

	// Step 3: Remove duplicate values (keep first occurrence, prefer permanent)
	// Sort order first so permanent facts come first, then they win on duplicate values
	valueSeen := make(map[string]bool)
	var unique []MemoryFact
	for _, key := range order {
		f := merged[key]
		h := hashMemoryValue(f.Value)
		if !valueSeen[h] {
			valueSeen[h] = true
			unique = append(unique, f)
		} else {
			// Duplicate value found — if this fact is permanent and the kept one is not, replace it
			h := hashMemoryValue(f.Value)
			for i, u := range unique {
				if hashMemoryValue(u.Value) == h && u.Value == f.Value && !u.IsPermanent() && f.IsPermanent() {
					unique[i] = f // Replace non-permanent with permanent
					break
				}
			}
		}
	}

	// Step 4: Sort by priority (permanent > critical > credentials > project > architecture > general)
	priorityCategories := map[string]int{
		"permanent":    10,
		"critical":     10,
		"credentials":  9,
		"project":      8,
		"architecture": 7,
	}

	sort.Slice(unique, func(i, j int) bool {
		// Permanent facts always come first
		if unique[i].IsPermanent() != unique[j].IsPermanent() {
			return unique[i].IsPermanent()
		}
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

	// Step 5: If over maxTokens, keep only facts that fit — but NEVER drop permanent
	if maxTokens > 0 {
		newTokens := len(renderMemoryMD(unique)) / 4
		if newTokens > maxTokens {
			var kept []MemoryFact
			currentTokens := 0
			for _, f := range unique {
				factTokens := (len(f.Key) + len(f.Value) + len(f.Category) + 10) / 4
				// Always keep permanent facts regardless of token budget
				if f.IsPermanent() {
					kept = append(kept, f)
					currentTokens += factTokens
					continue
				}
				if currentTokens+factTokens <= maxTokens {
					kept = append(kept, f)
					currentTokens += factTokens
				}
			}
			unique = kept
		}
	}

	// Step 6: If still over maxFacts, keep only top maxFacts (but never remove permanent)
	if len(unique) > memoryMaxFacts {
		var kept []MemoryFact
		// Always keep permanent facts
		for _, f := range unique {
			if f.IsPermanent() {
				kept = append(kept, f)
			}
		}
		// Add non-permanent facts up to maxFacts
		for _, f := range unique {
			if !f.IsPermanent() && len(kept) < memoryMaxFacts {
				kept = append(kept, f)
			}
		}
		unique = kept
	}

	t.facts = unique
	if err := t.saveToFileLocked(); err != nil {
		return fmt.Sprintf("Error saving compressed memory: %v", err)
	}

	removed := originalCount - len(t.facts)
	permCount := 0
	for _, f := range t.facts {
		if f.IsPermanent() {
			permCount++
		}
	}
	return fmt.Sprintf("✓ Compressed: %d → %d facts (removed %d duplicates/low-priority). 🔒 %d permanent facts protected.", originalCount, len(t.facts), removed, permCount)
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

// --- Helper functions ---

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

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

		// Fact line: - **key**: value  or  - **!key**: value (permanent)
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

			// Handle ! prefix in key (permanent marker)
			if strings.HasPrefix(key, "!") {
				key = strings.TrimPrefix(key, "!")
				if currentCategory == "general" || currentCategory == "" {
					currentCategory = "permanent"
				}
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
	// Group by category, permanent/critical/credentials first
	groups := make(map[string][]MemoryFact)
	var categories []string
	for _, f := range facts {
		if _, ok := groups[f.Category]; !ok {
			categories = append(categories, f.Category)
		}
		groups[f.Category] = append(groups[f.Category], f)
	}

	// Sort: permanent, critical, credentials, project first
	priorityOrder := map[string]int{
		"permanent":    0,
		"critical":     1,
		"credentials":  2,
		"project":      3,
		"architecture": 4,
	}
	sort.Slice(categories, func(i, j int) bool {
		pi, oki := priorityOrder[categories[i]]
		pj, okj := priorityOrder[categories[j]]
		if !oki {
			pi = 99
		}
		if !okj {
			pj = 99
		}
		if pi != pj {
			return pi < pj
		}
		return categories[i] < categories[j]
	})

	var sb strings.Builder
	sb.WriteString("# BugBuster Agent Memory\n\n")

	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("## %s\n", cat))
		for _, f := range groups[cat] {
			key := f.Key
			// Add ! prefix for permanent facts in non-permanent categories
			if f.IsPermanent() && cat != "permanent" && cat != "critical" && cat != "credentials" && cat != "project" {
				key = "!" + key
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", key, f.Value))
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
			permMark := ""
			if f.IsPermanent() {
				permMark = " 🔒"
			}
			sb.WriteString(fmt.Sprintf("  %s: %s%s\n", f.Key, f.Value, permMark))
		}
	}
	return sb.String()
}

func formatFactsForPrompt(facts []MemoryFact) string {
	// Group by category, permanent first
	groups := make(map[string][]MemoryFact)
	var categories []string
	for _, f := range facts {
		if _, ok := groups[f.Category]; !ok {
			categories = append(categories, f.Category)
		}
		groups[f.Category] = append(groups[f.Category], f)
	}

	// Sort: permanent, critical, credentials, project first
	priorityOrder := map[string]int{
		"permanent":    0,
		"critical":     1,
		"credentials":  2,
		"project":      3,
		"architecture": 4,
	}
	sort.Slice(categories, func(i, j int) bool {
		pi, oki := priorityOrder[categories[i]]
		pj, okj := priorityOrder[categories[j]]
		if !oki {
			pi = 99
		}
		if !okj {
			pj = 99
		}
		if pi != pj {
			return pi < pj
		}
		return categories[i] < categories[j]
	})

	var sb strings.Builder
	sb.WriteString("Important facts about this project (from agent memory):\n\n")
	for _, cat := range categories {
		permMark := ""
		if cat == "permanent" || cat == "critical" || cat == "credentials" || cat == "project" {
			permMark = " 🔒"
		}
		sb.WriteString(fmt.Sprintf("[%s]%s\n", cat, permMark))
		for _, f := range groups[cat] {
			perm := ""
			if f.IsPermanent() {
				perm = " 🔒"
			}
			sb.WriteString(fmt.Sprintf("- %s: %s%s\n", f.Key, f.Value, perm))
		}
	}
	return sb.String()
}