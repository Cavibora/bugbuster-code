package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/logger"
	"bugbuster-code/pkg/provider"
)

// ArchiveBlock is an archive context block
type ArchiveBlock struct {
	ID          string             `json:"id"`
	SessionID   string             `json:"session_id"`
	Summary     string             `json:"summary"`
	Topics      []string           `json:"topics"`
	Messages    []provider.Message `json:"messages"`
	TokenCount  int                `json:"token_count"`
	CreatedAt   time.Time          `json:"created_at"`
	Optimized   bool               `json:"optimized"`
	SourcePhase string             `json:"source_phase"` // "compaction" | "optimizer"
}

// ArchiveIndex is search index for all archive blocks
type ArchiveIndex struct {
	Entries []ArchiveIndexEntry `json:"entries"`
	Updated time.Time           `json:"updated"`
}

// ArchiveIndexEntry is a record in search index
type ArchiveIndexEntry struct {
	ID         string    `json:"id"`
	Summary    string    `json:"summary"`
	Topics     []string  `json:"topics"`
	TokenCount int       `json:"token_count"`
	CreatedAt  time.Time `json:"created_at"`
	SessionID  string    `json:"session_id"`
}

// ArchiveStore is archive blocks manager
type ArchiveStore struct {
	dir       string // base directory (e.g., .bugbuster/context/)
	maxBlocks int
	mu        sync.RWMutex
}

// NewArchiveStore creates storage archives
func NewArchiveStore(dir string, maxBlocks int) *ArchiveStore {
	if maxBlocks <= 0 {
		maxBlocks = 50
	}
	return &ArchiveStore{
		dir:       dir,
		maxBlocks: maxBlocks,
	}
}

// sessionDir returns subdirectory for sessions.
// If sessionID is empty, uses "_default".
func (s *ArchiveStore) sessionDir(sessionID string) string {
	if sessionID == "" {
		sessionID = "_default"
	}
	return filepath.Join(s.dir, sessionID)
}

// ListSessions returns list of all session IDs with archives
func (s *ArchiveStore) ListSessions() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var sessions []string
	for _, entry := range entries {
		if entry.IsDir() {
			sessions = append(sessions, entry.Name())
		}
	}
	return sessions, nil
}

// Init creates base directory for archives
func (s *ArchiveStore) Init() error {
	return os.MkdirAll(s.dir, 0755)
}

// SaveBlock saves archive block to disk (atomic write)
func (s *ArchiveStore) SaveBlock(block *ArchiveBlock) error {
	dir := s.sessionDir(block.SessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("archive: create session dir: %w", err)
	}

	data, err := json.MarshalIndent(block, "", "  ")
	if err != nil {
		return fmt.Errorf("archive: marshal block: %w", err)
	}

	tmpFile := filepath.Join(dir, block.ID+".tmp")
	finalFile := filepath.Join(dir, block.ID+".json")

	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("archive: write temp: %w", err)
	}

	if err := os.Rename(tmpFile, finalFile); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("archive: rename: %w", err)
	}

	return nil
}

// LoadBlock loads block by ID from session subdirectories
func (s *ArchiveStore) LoadBlock(id string) (*ArchiveBlock, error) {
	// Search for block in all session subdirectories
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("archive: read dir: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name(), id+".json"))
		if err == nil {
			var block ArchiveBlock
			if err := json.Unmarshal(data, &block); err != nil {
				return nil, fmt.Errorf("archive: unmarshal block %s: %w", id, err)
			}
			return &block, nil
		}
	}
	return nil, fmt.Errorf("archive: block %s not found", id)
}

// LoadBlockInSession loads block by ID from specific session
func (s *ArchiveStore) LoadBlockInSession(id, sessionID string) (*ArchiveBlock, error) {
	dir := s.sessionDir(sessionID)
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return nil, fmt.Errorf("archive: read block %s: %w", id, err)
	}
	var block ArchiveBlock
	if err := json.Unmarshal(data, &block); err != nil {
		return nil, fmt.Errorf("archive: unmarshal block %s: %w", id, err)
	}
	return &block, nil
}

// LoadIndex loads search index for sessions
func (s *ArchiveStore) LoadIndex() (*ArchiveIndex, error) {
	// For backward compatibility — load global index
	// (used by ArchiveOptimizer)
	return s.LoadIndexForSession("")
}

// LoadIndexForSession loads search index for specific session
func (s *ArchiveStore) LoadIndexForSession(sessionID string) (*ArchiveIndex, error) {
	dir := s.sessionDir(sessionID)
	data, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return &ArchiveIndex{}, nil
		}
		return nil, fmt.Errorf("archive: read index: %w", err)
	}
	var idx ArchiveIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("archive: unmarshal index: %w", err)
	}
	return &idx, nil
}

// SaveIndex saves search index for global context (backward compatibility)
func (s *ArchiveStore) SaveIndex(idx *ArchiveIndex) error {
	return s.SaveIndexForSession(idx, "")
}

// SaveIndexForSession saves search index for specific session (atomic write)
func (s *ArchiveStore) SaveIndexForSession(idx *ArchiveIndex, sessionID string) error {
	dir := s.sessionDir(sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("archive: create session dir: %w", err)
	}

	idx.Updated = time.Now()
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("archive: marshal index: %w", err)
	}

	tmpFile := filepath.Join(dir, "index.tmp")
	finalFile := filepath.Join(dir, "index.json")

	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("archive: write index temp: %w", err)
	}

	if err := os.Rename(tmpFile, finalFile); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("archive: rename index: %w", err)
	}

	return nil
}

// ArchiveMessages archives messages before compaction.
// Filters: removes tool errors, tool_use, tool_result — archive only keeps meaningful context.
// Creates block from filtered messages, saves to disk, updates index.
func (s *ArchiveStore) ArchiveMessages(oldMsgs []provider.Message, sessionID string) (*ArchiveBlock, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.Init(); err != nil {
		return nil, err
	}

	if len(oldMsgs) == 0 {
		return nil, nil
	}

	// Filter: keep only meaningful context (user text, assistant text/thinking)
	// Tool errors, tool_use, tool_result — remove, they are not needed in archive
	filtered := filterMessagesForArchive(oldMsgs)
	if len(filtered) == 0 {
		return nil, nil
	}

	// Do not create blocks with less than 2 messages — single assistant-thinking at 30 tokens
	// does not carry meaningful context, and creates garbage in archive
	if len(filtered) < 2 {
		return nil, nil
	}

	id := generateBlockID()

	// Extract recap from messages (if any)
	recaps := extractRecaps(filtered)
	summary := ""
	if len(recaps) > 0 {
		summary = strings.Join(recaps, "; ")
	}

	// Extract topics (keywords from text and tool_use)
	topics := extractTopics(oldMsgs)

	// Count tokens of filtered messages
	tokenCount := EstimateMessagesTokens(filtered)

	// If summary is empty — generate via SimpleSummarize
	if summary == "" {
		summary = SimpleSummarize(filtered, 200)
	}

	block := &ArchiveBlock{
		ID:          id,
		SessionID:   sessionID,
		Summary:     summary,
		Topics:      topics,
		Messages:    filtered,
		TokenCount:  tokenCount,
		CreatedAt:   time.Now(),
		SourcePhase: "compaction",
	}

	if err := s.SaveBlock(block); err != nil {
		return nil, err
	}

	// Update index
	if err := s.updateIndex(block); err != nil {
		// Not critical — index can be recreated
		logger.Error("archive: failed to update index", "err", err)
	}

	// Remove old blocks if limit exceeded
	if err := s.PruneBlocksForSession(sessionID); err != nil {
		logger.Error("archive: failed to prune blocks", "err", err)
	}

	return block, nil
}

// DeleteBlock deletes block by ID (searches in all session subdirectories)
func (s *ArchiveStore) DeleteBlock(id string) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		blockFile := filepath.Join(s.dir, entry.Name(), id+".json")
		if err := os.Remove(blockFile); err == nil {
			return nil // Successfully deleted
		}
	}
	return nil // Block not found — not an error
}

// DeleteBlockInSession deletes block by ID from specific session
func (s *ArchiveStore) DeleteBlockInSession(id, sessionID string) error {
	dir := s.sessionDir(sessionID)
	return os.Remove(filepath.Join(dir, id+".json"))
}

// PruneBlocks deletes oldest blocks in sessions if count exceeds maxBlocks
func (s *ArchiveStore) PruneBlocks() error {
	// PruneBlocks now works with current session via global index
	// For backward compatibility we use LoadIndex/SaveIndex
	return nil // PruneBlocks is called inside ArchiveMessages which already uses sessionDir
}

// PruneBlocksForSession deletes oldest blocks in specific session
func (s *ArchiveStore) PruneBlocksForSession(sessionID string) error {
	idx, err := s.LoadIndexForSession(sessionID)
	if err != nil {
		return err
	}

	if len(idx.Entries) <= s.maxBlocks {
		return nil
	}

	// Sort by creation date (oldest first)
	entries := idx.Entries
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].CreatedAt.After(entries[j].CreatedAt) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Remove oldest blocks
	toDelete := len(entries) - s.maxBlocks
	dir := s.sessionDir(sessionID)
	for i := 0; i < toDelete; i++ {
		blockFile := filepath.Join(dir, entries[i].ID+".json")
		os.Remove(blockFile)
	}

	// Update index
	idx.Entries = entries[toDelete:]
	return s.SaveIndexForSession(idx, sessionID)
}

// updateIndex adds block record to session index
func (s *ArchiveStore) updateIndex(block *ArchiveBlock) error {
	sessionID := block.SessionID
	idx, err := s.LoadIndexForSession(sessionID)
	if err != nil {
		idx = &ArchiveIndex{}
	}

	entry := ArchiveIndexEntry{
		ID:         block.ID,
		Summary:    block.Summary,
		Topics:     block.Topics,
		TokenCount: block.TokenCount,
		CreatedAt:  block.CreatedAt,
		SessionID:  block.SessionID,
	}

	idx.Entries = append(idx.Entries, entry)
	return s.SaveIndexForSession(idx, sessionID)
}

// generateBlockID generates unique block ID
func generateBlockID() string {
	return fmt.Sprintf("blk_%s", time.Now().Format("20060102_150405"))
}

// filterMessagesForArchive filters messages for archiving.
// Removes tool errors, tool_use, tool_result — keeps only meaningful context:
// user text, assistant text/thinking.
func filterMessagesForArchive(messages []provider.Message) []provider.Message {
	// First remove messages with tool errors
	messages = RemoveToolErrors(messages)

	var result []provider.Message
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			// System messages are not archived — they are always in context
			continue
		case "user":
			// Check if there are tool_result blocks
			hasToolResult := false
			for _, block := range msg.Content {
				if block.Type == "tool_result" {
					hasToolResult = true
					break
				}
			}
			if hasToolResult {
				// Check if there are only tool_result blocks
				onlyToolResult := true
				for _, block := range msg.Content {
					if block.Type != "tool_result" {
						onlyToolResult = false
						break
					}
				}
				if onlyToolResult {
					// Only tool_result — remove entirely
					continue
				}
				// Has both text and tool_result — remove tool_result blocks
				var filtered []provider.ContentBlock
				for _, block := range msg.Content {
					if block.Type != "tool_result" {
						filtered = append(filtered, block)
					}
				}
				if len(filtered) > 0 {
					msg.Content = filtered
					result = append(result, msg)
				}
				continue
			}
			result = append(result, msg)
		case "assistant":
			// Assistant messages — leave text and thinking, delete tool_use
			var filtered []provider.ContentBlock
			for _, block := range msg.Content {
				switch block.Type {
				case "text", "thinking":
					filtered = append(filtered, block)
					// tool_use — delete
				}
			}
			if len(filtered) > 0 {
				msg.Content = filtered
				result = append(result, msg)
			}
		}
	}
	return result
}

// extractTopics extracts keywords from messages for search
func extractTopics(messages []provider.Message) []string {
	topicSet := make(map[string]bool)

	for _, msg := range messages {
		// Extract file names and patterns from tool_use
		for _, block := range msg.Content {
			if block.Type == "tool_use" {
				if path, ok := block.Input["path"]; ok {
					if s, ok := path.(string); ok {
						topicSet[filepath.Base(s)] = true
					}
				}
				if pattern, ok := block.Input["pattern"]; ok {
					if s, ok := pattern.(string); ok {
						topicSet[s] = true
					}
				}
				if command, ok := block.Input["command"]; ok {
					if s, ok := command.(string); ok {
						// Extract first word of commands
						parts := strings.Fields(s)
						if len(parts) > 0 {
							topicSet[parts[0]] = true
						}
					}
				}
			}
		}

		// Extract keywords from user messages
		if msg.Role == "user" {
			text := msg.GetText()
			words := strings.Fields(text)
			for _, w := range words {
				w = strings.ToLower(strings.Trim(w, ".,!?;:()[]{}\"'`"))
				if len(w) > 3 && !isStopWord(w) {
					topicSet[w] = true
				}
			}
		}
	}

	// Limit to 15 topics
	topics := make([]string, 0, len(topicSet))
	for t := range topicSet {
		topics = append(topics, t)
		if len(topics) >= 15 {
			break
		}
	}
	return topics
}

// isStopWord checks if word is a stop word (multilingual: en, ru, de, es, fr, pt, ja, zh)
func isStopWord(word string) bool {
	stopWords := map[string]bool{
		// English
		"this": true, "that": true, "with": true, "from": true,
		"have": true, "will": true, "been": true, "were": true,
		"they": true, "their": true, "about": true, "would": true,
		"could": true, "should": true, "which": true, "there": true,
		"these": true, "other": true, "after": true, "before": true,
		"the": true, "also": true, "very": true, "just": true,
		// Russian
		"этом": true, "этот": true, "эта": true, "это": true,
		"который": true, "которая": true, "которое": true, "которые": true,
		"чтобы": true, "потому": true, "может": true, "можно": true,
		"надо": true, "будет": true, "были": true,
		"и": true, "в": true, "на": true, "с": true, "по": true,
		"к": true, "у": true, "о": true, "а": true, "но": true,
		"не": true, "как": true, "то": true, "для": true, "от": true,
		"из": true, "за": true, "же": true, "бы": true, "ли": true,
		"да": true,
		// German
		"der": true, "die": true, "das": true, "ein": true, "eine": true,
		"ist": true, "sind": true, "war": true, "waren": true, "sein": true,
		"gewesen": true, "haben": true, "hat": true, "hatte": true,
		"werden": true, "wurde": true, "wurden": true, "können": true,
		"kann": true, "könnte": true, "sollte": true, "muss": true,
		"nicht": true, "auch": true, "aber": true, "oder": true, "und": true,
		"als": true, "wie": true, "von": true, "zu": true, "an": true,
		"auf": true, "mit": true, "für": true, "noch": true, "schon": true,
		"nur": true, "sehr": true, "ja": true, "nein": true,
		"dieser": true, "diese": true, "dieses": true, "was": true,
		// Spanish
		"el": true, "la": true, "los": true, "las": true, "un": true,
		"una": true, "unos": true, "unas": true, "son": true, "era": true,
		"eran": true, "ser": true, "estar": true, "puede": true,
		"pueden": true, "podría": true, "debería": true, "también": true,
		"pero": true, "como": true, "más": true, "muy": true, "ya": true,
		"este": true, "esta": true, "esto": true, "quien": true,
		"cual": true, "cuando": true, "donde": true, "por": true,
		"para": true, "con": true, "sin": true, "sobre": true,
		"entre": true, "todo": true, "todos": true, "cada": true,
		"otro": true, "otra": true, "sí": true, "bien": true,
		// French
		"les": true, "des": true, "du": true, "était": true,
		"étaient": true, "être": true, "avoir": true, "ont": true,
		"avait": true, "peut": true, "peuvent": true, "pourrait": true,
		"devrait": true, "pas": true, "aussi": true, "ou": true,
		"très": true, "déjà": true, "ce": true, "cette": true,
		"ces": true, "celui": true, "celle": true, "ceux": true,
		"qui": true, "quoi": true, "quand": true, "où": true,
		"par": true, "sans": true, "dans": true, "tout": true,
		"tous": true, "chaque": true, "autre": true, "autres": true,
		"oui": true, "non": true, "peu": true,
		// Portuguese
		"os": true, "umas": true, "uns": true, "uma": true,
		"são": true, "eram": true, "está": true,
		"estão": true, "tem": true, "têm": true, "pode": true,
		"podem": true, "poderia": true, "deveria": true, "não": true,
		"também": true, "muito": true, "já": true, "isto": true,
		"estes": true, "estas": true, "aquele": true, "aquela": true,
		"aquilo": true, "quem": true, "qual": true, "quando": true,
		"onde": true, "sem": true, "até": true, "desde": true,
		"outro": true, "outros": true,
		"sim": true, "bem": true, "pouco": true,
		// Japanese
		"の": true, "に": true, "は": true, "を": true, "が": true,
		"で": true, "と": true, "も": true, "から": true, "まで": true,
		"より": true, "へ": true, "や": true, "など": true, "だ": true,
		"である": true, "です": true, "ます": true, "れる": true,
		"られる": true, "しない": true, "ない": true, "ある": true,
		"いる": true, "する": true, "なる": true, "この": true,
		"その": true, "あの": true, "どの": true, "これ": true,
		"それ": true, "あれ": true, "何": true, "誰": true,
		"どこ": true, "いつ": true, "なぜ": true, "どう": true,
		"とても": true, "もう": true, "まだ": true, "また": true,
		"ただ": true, "しかし": true, "そして": true, "だから": true,
		"または": true,
		// Chinese
		"了": true, "在": true, "我": true, "有": true, "就": true,
		"人": true, "都": true, "一个": true, "上": true, "很": true,
		"到": true, "说": true, "要": true, "去": true, "你": true,
		"会": true, "着": true, "没有": true, "看": true, "好": true,
		"自己": true, "这": true, "他": true, "她": true, "它": true,
		"们": true, "那": true, "些": true, "什么": true, "哪": true,
		"谁": true, "怎么": true, "为什么": true, "哪里": true,
		"可以": true, "能": true, "应该": true, "但是": true,
		"因为": true, "所以": true, "如果": true, "虽然": true,
		"还是": true, "或者": true, "而且": true,
	}
	return stopWords[word]
}

// FormatArchiveBlock formats archive block for model display
func FormatArchiveBlock(block *ArchiveBlock, maxTokens int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s %s]\n", i18n.T("archive.block_label"), block.ID))
	sb.WriteString(fmt.Sprintf("%s: %s\n", i18n.T("archive.summary_label"), block.Summary))
	if len(block.Topics) > 0 {
		sb.WriteString(fmt.Sprintf("%s: %s\n", i18n.T("archive.topics_label"), strings.Join(block.Topics, ", ")))
	}
	sb.WriteString(fmt.Sprintf("%s: %s\n", i18n.T("archive.session_label"), block.SessionID))
	sb.WriteString(fmt.Sprintf("%s: ~%d\n", i18n.T("archive.tokens_label"), block.TokenCount))
	sb.WriteString("---\n")

	// Format messages with token budget
	budget := maxTokens
	for _, msg := range block.Messages {
		if budget <= 0 {
			sb.WriteString("...\n")
			break
		}
		text := msg.GetText()
		tokens := EstimateTokens(text)
		if tokens > budget {
			// Trim to budget
			maxChars := budget * charsPerToken
			if maxChars > len(text) {
				maxChars = len(text)
			}
			text = text[:maxChars] + "..."
			tokens = budget
		}
		sb.WriteString(fmt.Sprintf("[%s] %s\n", msg.Role, text))
		budget -= tokens
	}

	return sb.String()
}
