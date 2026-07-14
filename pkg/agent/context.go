package agent

import (
	"context"
	"sync"

	"bugbuster-code/pkg/provider"
)

// ConversationContext is agent conversation context.
// Thread-safe: all methods are protected by RWMutex for safe concurrent access
// from streaming goroutines and UI goroutines.
type ConversationContext struct {
	mu                  sync.RWMutex
	Messages            []provider.Message
	MaxTokens           int               // maximum token count (0 = default 8000)
	KeepRecent          int               // how many recent messages to keep during compaction
	SkipCompaction      bool              // skip compaction (for providers managing context themselves)
	Compactor           Compactor         // context compactor (nil = simple truncation)
	AutoCompact         bool              // automatically compact on Add() (default: true)
	OnCompact           func()            // callback before compaction (for UI notification)
	AfterCompact        func()            // callback after compaction (for memory injection)
	Ctx                 context.Context   // cancellation context for compaction (nil = context.Background())
	Archive             *ArchiveStore     // context archive (nil = archiving disabled)
	Optimizer           *ArchiveOptimizer // archive optimizer (nil = optimization disabled)
	SessionID           string            // ID of current session for linking with archive blocks
	lastCompactionRatio float64           // savings ratio at last compaction
	lowSaveCount        int               // how many consecutive times savings < 10%
	OriginalTask        string            // original user request — survives compaction
}

// NewConversationContext creates a new context with message limit
// Deprecated: use NewConversationContextWithTokens
func NewConversationContext(maxMessages int) *ConversationContext {
	return NewConversationContextWithTokens(8000, 6)
}

// NewConversationContextWithTokens creates context with token limit
func NewConversationContextWithTokens(maxTokens, keepRecent int) *ConversationContext {
	if maxTokens <= 0 {
		maxTokens = 8000 // default limit
	}
	if keepRecent <= 0 {
		keepRecent = 20
	}
	return &ConversationContext{
		MaxTokens:   maxTokens,
		KeepRecent:  keepRecent,
		AutoCompact: true,
	}
}

// Add adds message to context, trimming old ones when exceeding limit.
// Thread-safe: acquires write lock.
func (c *ConversationContext) Add(msg provider.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Messages = append(c.Messages, msg)
	c.trim()
	// Note: lowSaveCount is NOT reset here — it's managed inside compact()
	// and resets only when compaction is effective (>10% savings).
	// This allows anti-thrashing to work correctly.
}

// GetSystemPrompt returns system prompt (first message with role=system).
// Thread-safe: acquires read lock.
func (c *ConversationContext) GetSystemPrompt() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, m := range c.Messages {
		if m.Role == "system" {
			return m.GetText()
		}
	}
	return ""
}

// Reset clears context keeping system prompt.
// Thread-safe: acquires write lock.
func (c *ConversationContext) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	var systemMsgs []provider.Message
	for _, m := range c.Messages {
		if m.Role == "system" {
			systemMsgs = append(systemMsgs, m)
		}
	}
	c.Messages = systemMsgs
}

// BuildPrompt assembles weighted context into text prompt (for models without function calling).
// Thread-safe: acquires read lock.
func (c *ConversationContext) BuildPrompt() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return provider.MessagesToText(c.Messages)
}

// TokenCount returns approximate token count in context.
// Thread-safe: acquires read lock.
func (c *ConversationContext) TokenCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return EstimateMessagesTokens(c.Messages)
}

// MaxTokensValue returns the maximum token count for the context.
func (c *ConversationContext) MaxTokensValue() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.MaxTokens
}

// MessageCount returns the number of messages in context.
func (c *ConversationContext) MessageCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.Messages)
}

// GetMessages returns a copy of messages.
// Thread-safe: acquires read lock.
func (c *ConversationContext) GetMessages() []provider.Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]provider.Message, len(c.Messages))
	copy(result, c.Messages)
	return result
}

// trim trims old messages by tokens.
// Caller must hold write lock.
func (c *ConversationContext) trim() {
	// Skip compaction for providers managing context themselves
	if c.SkipCompaction {
		return
	}

	// If AutoCompact is disabled — skip (compaction is done explicitly via Compact())
	if !c.AutoCompact {
		return
	}

	c.compact()
}

// compact executes context compaction.
// Caller must hold write lock.
func (c *ConversationContext) compact() {
	// Anti-thrashing: if last 2 compactions saved <10%, skip
	// But only if context does NOT exceed limit — if it exceeds, compaction is mandatory
	currentTokens := EstimateMessagesTokens(c.Messages)
	if c.lowSaveCount >= 2 && c.MaxTokens > 0 && currentTokens <= c.MaxTokens {
		return
	}

	if c.OnCompact != nil {
		c.OnCompact()
	}
	ctx := c.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	tokensBefore := EstimateMessagesTokens(c.Messages)

	// Target compaction size: 2/3 of MaxTokens
	// This frees 1/3 of context for new messages and prevents
	// frequent re-compaction (when compaction removes 5%, and context
	// overflows again after 2-3 messages)
	targetTokens := c.MaxTokens * 2 / 3
	if targetTokens < 1 {
		targetTokens = 1
	}

	// Save copy of messages before compaction for archiving
	var msgsBeforeCompaction []provider.Message
	if c.Archive != nil {
		msgsBeforeCompaction = make([]provider.Message, len(c.Messages))
		copy(msgsBeforeCompaction, c.Messages)
	}

	if c.Compactor != nil {
		c.Messages = CompactContextWithCompactor(c.Messages, targetTokens, c.KeepRecent, c.Compactor, ctx)
	} else {
		c.Messages = CompactContext(c.Messages, targetTokens, c.KeepRecent)
	}

	tokensAfter := EstimateMessagesTokens(c.Messages)

	// Archive removed messages
	if c.Archive != nil && len(msgsBeforeCompaction) > 0 && tokensBefore > tokensAfter {
		removed := findRemovedMessages(msgsBeforeCompaction, c.Messages)
		if len(removed) > 0 {
			// Archive in background — do not block main thread
			archive := c.Archive
			sessionID := c.SessionID
			go func() {
				_, _ = archive.ArchiveMessages(removed, sessionID)
			}()
		}
	}

	// Run archive optimizer in background (if enabled)
	if c.Optimizer != nil {
		optimizer := c.Optimizer
		go func() {
			_ = optimizer.Optimize(context.Background())
		}()
	}

	// Calculate savings ratio
	if tokensBefore > 0 {
		c.lastCompactionRatio = float64(tokensBefore-tokensAfter) / float64(tokensBefore)
	}

	// Update counter of ineffective compactions
	if c.lastCompactionRatio < 0.1 {
		c.lowSaveCount++
	} else {
		c.lowSaveCount = 0
	}
}

// Compact executes explicit context compaction (when AutoCompact = false).
// On manual compaction reset anti-thrashing — user explicitly requested compaction.
// Thread-safe: acquires write lock.
func (c *ConversationContext) Compact() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lowSaveCount = 0
	c.compact()
}

// CompactForce performs aggressive compaction — strips all tool calls/results,
// thinking blocks, errors, and low-value data. Keeps only system messages,
// last 2 user/assistant exchanges, and OriginalTask.
// Thread-safe: acquires write lock.
func (c *ConversationContext) CompactForce() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lowSaveCount = 0

	if c.OnCompact != nil {
		c.OnCompact()
	}

	tokensBefore := EstimateMessagesTokens(c.Messages)

	// Save copy for archiving
	var msgsBeforeCompaction []provider.Message
	if c.Archive != nil {
		msgsBeforeCompaction = make([]provider.Message, len(c.Messages))
		copy(msgsBeforeCompaction, c.Messages)
	}

	// 1. Remove tool errors and duplicates
	msgs := RemoveToolErrors(c.Messages)
	msgs = RemoveDuplicates(msgs)
	msgs = EnsureToolPairIntegrity(msgs)

	// 2. Split system / other
	var systemMsgs []provider.Message
	var otherMsgs []provider.Message
	for _, m := range msgs {
		if m.Role == "system" {
			systemMsgs = append(systemMsgs, m)
		} else {
			otherMsgs = append(otherMsgs, m)
		}
	}

	// 3. Strip ALL tool calls and thinking from ALL messages
	cleaned := make([]provider.Message, 0, len(otherMsgs))
	for _, msg := range otherMsgs {
		stripped := stripToolCalls(msg)
		stripped = truncateThinking(stripped)
		stripped = truncateAssistantText(stripped)
		if !isEmptyMessage(stripped) {
			cleaned = append(cleaned, stripped)
		}
	}

	// 4. Keep only last 4 messages (2 exchanges)
	keepCount := 4
	if keepCount > len(cleaned) {
		keepCount = len(cleaned)
	}
	recent := cleaned[len(cleaned)-keepCount:]

	// 5. Summarize old messages into one compact system message
	old := cleaned[:len(cleaned)-keepCount]
	result := make([]provider.Message, 0, len(systemMsgs)+1+len(recent)+1)
	result = append(result, systemMsgs...)

	if len(old) > 0 {
		summary := SimpleSummarize(old, 200)
		if summary != "" {
			result = append(result, provider.Message{
				Role: "system",
				Content: []provider.ContentBlock{
					{Type: "text", Text: summary},
				},
			})
		}
	}

	// 6. Add OriginalTask if set
	if c.OriginalTask != "" {
		result = append(result, provider.Message{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "text", Text: "Original task: " + c.OriginalTask},
			},
		})
	}

	result = append(result, recent...)
	c.Messages = result

	tokensAfter := EstimateMessagesTokens(c.Messages)

	// Archive removed messages
	if c.Archive != nil && len(msgsBeforeCompaction) > 0 && tokensBefore > tokensAfter {
		removed := findRemovedMessages(msgsBeforeCompaction, c.Messages)
		if len(removed) > 0 {
			archive := c.Archive
			sessionID := c.SessionID
			go func() {
				_, _ = archive.ArchiveMessages(removed, sessionID)
			}()
		}
	}

	if c.Optimizer != nil {
		optimizer := c.Optimizer
		go func() {
			_ = optimizer.Optimize(context.Background())
		}()
	}

	if tokensBefore > 0 {
		c.lastCompactionRatio = float64(tokensBefore-tokensAfter) / float64(tokensBefore)
	}
	c.lowSaveCount = 0
}

// findRemovedMessages returns messages that were in before but not in after
func findRemovedMessages(before, after []provider.Message) []provider.Message {
	afterSet := make(map[string]bool)
	for _, m := range after {
		// Use text+role as key for identification
		key := m.Role + ":" + m.GetText()
		if len(key) > 200 {
			key = key[:200]
		}
		afterSet[key] = true
	}

	var removed []provider.Message
	for _, m := range before {
		key := m.Role + ":" + m.GetText()
		if len(key) > 200 {
			key = key[:200]
		}
		if !afterSet[key] {
			removed = append(removed, m)
		}
	}
	return removed
}