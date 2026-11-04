package agent

import (
	"context"
	"strings"
	"sync"

	"bugbuster-code/pkg/provider"
)

// ConversationContext is agent conversation context.
// Thread-safe: all methods are protected by RWMutex for safe concurrent access
// from streaming goroutines and UI goroutines.
type ConversationContext struct {
	mu sync.RWMutex
	// messages is the conversation history. It is unexported on purpose: every
	// read or write must go through a locking accessor, because the streaming
	// loop goroutine and HTTP/UI goroutines touch it concurrently. Use
	// GetMessages/AppendMessages/SetMessages/MessageCount.
	messages            []provider.Message
	MaxTokens           int               // maximum token count (0 = default 8000)
	KeepRecent          int               // how many recent messages to keep during compaction
	SkipCompaction      bool              // skip compaction (for providers managing context themselves)
	Compactor           Compactor         // context compactor (nil = simple truncation)
	OnCompact           func()            // callback before compaction (for UI notification)
	CompactionNotifier  func(tokensBefore, tokensAfter, msgsBefore, msgsAfter int, compType string) // callback after any compaction with details
	OnCompactForce      func()            // callback after CompactForce (to reset speed tracking)
	AfterCompact        func()            // callback after compaction (for memory injection)
	Ctx                 context.Context   // cancellation context for compaction (nil = context.Background())
	Archive             *ArchiveStore     // context archive (nil = archiving disabled)
	Optimizer           *ArchiveOptimizer // archive optimizer (nil = optimization disabled)
	SessionID           string            // ID of current session for linking with archive
	lastCompactionRatio float64           // savings ratio at last compaction
	lowSaveCount        int               // how many consecutive times savings < 10%
	// origTask and autoCompact are guarded by mu: they are written by the agent
	// goroutine and read by HTTP/UI goroutines (pkg/serve State polling). Use
	// SetOriginalTask/OriginalTaskValue and SetAutoCompact/AutoCompactEnabled.
	origTask    string // original user request — survives compaction
	autoCompact bool   // automatically compact on Add() (default: true)
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
		autoCompact: true,
	}
}

// Add adds message to context, trimming old ones when exceeding limit.
// Thread-safe: acquires write lock.
func (c *ConversationContext) Add(msg provider.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, msg)
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
	for _, m := range c.messages {
		if m.Role == "system" {
			return m.GetText()
		}
	}
	return ""
}

// SetSystemPrompt replaces the system prompt in the context.
// If no system message exists, it prepends one.
// Thread-safe: acquires write lock.
func (c *ConversationContext) SetSystemPrompt(prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, m := range c.messages {
		if m.Role == "system" {
			c.messages[i] = provider.SystemMsg(prompt)
			return
		}
	}
	// No system message found — prepend
	c.messages = append([]provider.Message{provider.SystemMsg(prompt)}, c.messages...)
}

// Reset clears context keeping system prompt.
// Thread-safe: acquires write lock.
func (c *ConversationContext) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	var systemMsgs []provider.Message
	for _, m := range c.messages {
		if m.Role == "system" {
			systemMsgs = append(systemMsgs, m)
		}
	}
	c.messages = systemMsgs
}

// BuildPrompt assembles weighted context into text prompt (for models without function calling).
// Thread-safe: acquires read lock.
func (c *ConversationContext) BuildPrompt() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return provider.MessagesToText(c.messages)
}

// TokenCount returns approximate token count in context.
// Thread-safe: acquires read lock.
func (c *ConversationContext) TokenCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return EstimateMessagesTokens(c.messages)
}

// MaxTokensValue returns the maximum token count for the context.
func (c *ConversationContext) MaxTokensValue() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.MaxTokens
}

// OriginalTaskValue returns the original user request kept across compactions.
// Thread-safe: acquires read lock.
func (c *ConversationContext) OriginalTaskValue() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.origTask
}

// SetOriginalTask records the original user request. Thread-safe.
func (c *ConversationContext) SetOriginalTask(task string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.origTask = task
}

// InitOriginalTask sets the original task only if none is recorded yet — the
// first user message of a session defines it, later ones must not overwrite it.
// Thread-safe.
func (c *ConversationContext) InitOriginalTask(task string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.origTask == "" {
		c.origTask = task
	}
}

// AutoCompactEnabled reports whether Add() may compact automatically.
// Thread-safe: acquires read lock.
func (c *ConversationContext) AutoCompactEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.autoCompact
}

// SetAutoCompact toggles automatic compaction inside Add(). The streaming loop
// disables it while it runs so compaction happens explicitly and can be
// announced as an event. Thread-safe.
func (c *ConversationContext) SetAutoCompact(on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.autoCompact = on
}

// MessageCount returns the number of messages in context.
func (c *ConversationContext) MessageCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.messages)
}

// GetMessages returns a copy of messages.
// Thread-safe: acquires read lock.
func (c *ConversationContext) GetMessages() []provider.Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]provider.Message, len(c.messages))
	copy(result, c.messages)
	return result
}

// AppendMessages adds messages to the history. Thread-safe: acquires write
// lock. Unlike Add it does not trigger auto-compaction — the streaming loop
// calls maybeCompact explicitly so it can announce the event.
func (c *ConversationContext) AppendMessages(msgs ...provider.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, msgs...)
}

// SetMessages replaces the whole history. Thread-safe: acquires write lock.
// Used when restoring a session or rewriting the history after filtering. The
// input slice is copied so the caller keeps ownership of its own data.
func (c *ConversationContext) SetMessages(msgs []provider.Message) {
	copied := make([]provider.Message, len(msgs))
	copy(copied, msgs)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = copied
}

// MessagesFunc rewrites the history through fn under the write lock. Use it
// for filter-transforms (RemoveCompactionReminders and friends) so the read,
// the transform and the swap stay atomic with respect to concurrent readers.
func (c *ConversationContext) MessagesFunc(fn func([]provider.Message) []provider.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = fn(c.messages)
}

// HasImages reports whether any message in the history carries image content.
func (c *ConversationContext) HasImages() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, msg := range c.messages {
		for _, block := range msg.Content {
			if block.Type == "image" {
				return true
			}
		}
	}
	return false
}

// StripImages removes every image content block from the conversation history
// and returns the number of blocks removed. Text, thinking, tool_use and
// tool_result blocks are preserved; a message left with no content at all is
// dropped. Used to self-heal a session that a text-only endpoint refuses to
// serve because stale image blocks in the history keep triggering
// "model does not support image input" on every subsequent request.
// Thread-safe: acquires write lock.
func (c *ConversationContext) StripImages() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	var kept []provider.Message
	for _, msg := range c.messages {
		hasImage := false
		for _, block := range msg.Content {
			if block.Type == "image" {
				hasImage = true
				removed++
			}
		}
		if !hasImage {
			kept = append(kept, msg)
			continue
		}

		var blocks []provider.ContentBlock
		for _, block := range msg.Content {
			if block.Type == "image" {
				continue
			}
			blocks = append(blocks, block)
		}
		if len(blocks) == 0 && strings.TrimSpace(msg.Text) == "" {
			continue // nothing left but the image — drop the message
		}
		msg.Content = blocks
		kept = append(kept, msg)
	}

	if removed > 0 {
		c.messages = kept
	}
	return removed
}

// trim trims old messages by tokens.
// Caller must hold write lock.
func (c *ConversationContext) trim() {
	// Skip compaction for providers managing context themselves
	if c.SkipCompaction {
		return
	}

	// If AutoCompact is disabled — skip (compaction is done explicitly via Compact())
	if !c.autoCompact {
		return
	}

	c.compact()
}

// compact executes context compaction.
// Caller must hold write lock.
func (c *ConversationContext) compact() {
	// Anti-thrashing: if last 2 compactions saved <10%, skip
	// But only if context does NOT exceed limit — if it exceeds, compaction is mandatory
	currentTokens := EstimateMessagesTokens(c.messages)
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

	tokensBefore := EstimateMessagesTokens(c.messages)
	msgsBefore := len(c.messages)

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
		msgsBeforeCompaction = make([]provider.Message, len(c.messages))
		copy(msgsBeforeCompaction, c.messages)
	}

	if c.Compactor != nil {
		c.messages = CompactContextWithCompactor(c.messages, targetTokens, c.KeepRecent, c.Compactor, ctx)
	} else {
		c.messages = CompactContext(c.messages, targetTokens, c.KeepRecent)
	}

	tokensAfter := EstimateMessagesTokens(c.messages)
	msgsAfter := len(c.messages)

	// Notify about compaction details
	if c.CompactionNotifier != nil {
		c.CompactionNotifier(tokensBefore, tokensAfter, msgsBefore, msgsAfter, "auto")
	}

	// Archive removed messages
	if c.Archive != nil && len(msgsBeforeCompaction) > 0 && tokensBefore > tokensAfter {
		removed := findRemovedMessages(msgsBeforeCompaction, c.messages)
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

	tokensBefore := EstimateMessagesTokens(c.messages)
	msgsBefore := len(c.messages)

	// Save copy for archiving
	var msgsBeforeCompaction []provider.Message
	if c.Archive != nil {
		msgsBeforeCompaction = make([]provider.Message, len(c.messages))
		copy(msgsBeforeCompaction, c.messages)
	}

	// 1. Remove tool errors, duplicates, and repeated user messages
	msgs := RemoveToolErrors(c.messages)
	msgs = RemoveDuplicates(msgs)
	msgs = DeduplicateRepeats(msgs)
	msgs = EnsureToolPairIntegrity(msgs)
	// Remove stale compaction reminders (system "[Auto-compacted]" notices
	// and user "[Context was compacted...]") except the latest one.
	msgs = RemoveCompactionReminders(msgs)

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
	// stripToolCalls removes tool_use but keeps tool_result in user messages,
	// creating orphans. Convert them to text blocks so the OpenAI format
	// doesn't produce invalid "role: tool" messages.
	cleaned = EnsureToolPairIntegrity(cleaned)

	// 4. Keep last 8 messages (4 exchanges) — enough to maintain working context
	// Too few (4) causes model to lose track and re-analyze, triggering repeated compaction
	keepCount := 8
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
	if c.origTask != "" {
		result = append(result, provider.Message{
			Role: "user",
			Content: []provider.ContentBlock{
				{Type: "text", Text: "Original task: " + c.origTask},
			},
		})
	}

	result = append(result, recent...)
	c.messages = result

	tokensAfter := EstimateMessagesTokens(c.messages)
	msgsAfter := len(c.messages)

	// Notify about compaction details
	if c.CompactionNotifier != nil {
		c.CompactionNotifier(tokensBefore, tokensAfter, msgsBefore, msgsAfter, "force")
	}

	// Archive removed messages
	if c.Archive != nil && len(msgsBeforeCompaction) > 0 && tokensBefore > tokensAfter {
		removed := findRemovedMessages(msgsBeforeCompaction, c.messages)
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

	// Notify agent loop that CompactForce was called (to reset speed tracking)
	if c.OnCompactForce != nil {
		c.OnCompactForce()
	}
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

// isAutoContinueMessage checks if a user message is an auto-continue prompt.
// These are transient messages that should be cleaned up from context,
// keeping only the most recent one.
func isAutoContinueMessage(msg provider.Message) bool {
	if msg.Role != "user" {
		return false
	}
	text := strings.TrimSpace(msg.GetResponseText())
	if text == "" {
		return false
	}
	// "Continue." — max_tokens continuation
	if text == "Continue." || text == "Continue" {
		return true
	}
	// Auto-continue prompts
	autoContinuePrefixes := []string{
		"Continue. Use tools.",
		"Continue working.",
		"Text-only reply, task not done.",
		"You responded with text only, but your task is not done yet.",
	}
	for _, prefix := range autoContinuePrefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

// isTransientErrorMessage checks if a user message is a transient error/hint
// that should NOT be kept in context (idle timeout hints, loop detection hints).
// These messages pollute context with stack traces and error details.
func isTransientErrorMessage(msg provider.Message) bool {
	if msg.Role != "user" {
		return false
	}
	text := strings.TrimSpace(msg.GetResponseText())
	if text == "" {
		return false
	}
	// Idle timeout hints
	if strings.Contains(text, "stream_idle") || strings.Contains(text, "idle_timeout") {
		return true
	}
	// Loop detection hints
	loopHints := []string{
		"loop_detector.hint_thinking_loop",
		"loop_detector.strategy_hint",
	}
	for _, hint := range loopHints {
		if strings.Contains(text, hint) {
			return true
		}
	}
	return false
}

// CleanupTransients removes transient messages from context:
// - Auto-continue prompts ("Continue.", "Continue working...", "You responded with text only...")
//   kept only the most recent one (so the model knows what to continue).
// - Transient error/hint messages (idle timeout, loop detection) removed entirely.
// - "Continue." (max_tokens) messages — keep only the last one.
// - Compaction reminders ("[Context was compacted...]", "[Auto-compacted]") — keep only the last one.
// - Performance mirror system messages — keep only the last one.
// This prevents context pollution from repeated auto-continue, error, compaction,
// and performance mirror messages.
// Thread-safe: acquires write lock.
func (c *ConversationContext) CleanupTransients() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = cleanupTransientsLocked(c.messages)
}

// cleanupTransientsLocked performs the actual transient cleanup without locking.
// Caller must hold the write lock.
func cleanupTransientsLocked(messages []provider.Message) []provider.Message {
	if len(messages) == 0 {
		return messages
	}

	// Find indices of auto-continue messages and transient error messages
	var lastAutoContinueIdx int = -1
	var transientIndices []int

	for i, msg := range messages {
		if isAutoContinueMessage(msg) {
			lastAutoContinueIdx = i // keep track of the last one
		}
		if isTransientErrorMessage(msg) {
			transientIndices = append(transientIndices, i)
		}
	}

	// Build set of indices to remove
	removeSet := make(map[int]bool)
	for _, idx := range transientIndices {
		removeSet[idx] = true
	}
	// Remove all auto-continue messages except the last one
	for i, msg := range messages {
		if isAutoContinueMessage(msg) && i != lastAutoContinueIdx {
			removeSet[i] = true
		}
	}

	// Remove all compaction reminders except the latest of each role
	// (user reminders carry the task; system notices carry auto-compact stats)
	messages = RemoveCompactionReminders(messages)

	// Remove all performance mirrors except the last one
	lastMirrorIdx := -1
	for i, msg := range messages {
		if isPerformanceMirror(msg) {
			lastMirrorIdx = i
		}
	}
	for i, msg := range messages {
		if isPerformanceMirror(msg) && i != lastMirrorIdx {
			removeSet[i] = true
		}
	}

	if len(removeSet) == 0 {
		return messages
	}

	// Build filtered list
	result := make([]provider.Message, 0, len(messages)-len(removeSet))
	for i, msg := range messages {
		if !removeSet[i] {
			result = append(result, msg)
		}
	}
	return result
}

// isCompactionReminder checks if a message is a "[Context was compacted...]"
// or "[Auto-compacted]" reminder injected after compaction.
// User-role reminders are injected after auto/manual compaction;
// system-role "[Auto-compacted]" messages are injected after speed-triggered
// auto-compact (injectSpeedMirror). These accumulate over time and pollute
// context — only the latest should be kept.
func isCompactionReminder(msg provider.Message) bool {
	text := strings.TrimSpace(msg.GetResponseText())
	if text == "" {
		return false
	}
	switch msg.Role {
	case "user":
		return strings.Contains(text, "[Context was compacted") ||
			strings.Contains(text, "[Auto-compacted]") ||
			strings.Contains(text, "[Compacted. Original task:")
	case "system":
		// System-role auto-compaction notices start with the marker.
		// Use HasPrefix to avoid false positives on summaries that merely
		// mention the marker in the middle of the text.
		return strings.HasPrefix(text, "[Auto-compacted]")
	default:
		return false
	}
}

// isPerformanceMirror checks if a message is a "[Performance mirror]" system message.
// These are injected every 5 iterations and accumulate — only the latest should be kept.
func isPerformanceMirror(msg provider.Message) bool {
	if msg.Role != "system" {
		return false
	}
	text := strings.TrimSpace(msg.GetResponseText())
	if text == "" {
		return false
	}
	return strings.HasPrefix(text, "[Performance mirror]")
}

// RemoveCompactionReminders removes all compaction reminder messages
// except the most recent one of each role (user and system).
// User-role reminders carry the original task; system-role notices carry
// auto-compaction stats — both are kept, but only in their latest form.
func RemoveCompactionReminders(messages []provider.Message) []provider.Message {
	lastUserIdx, lastSystemIdx := -1, -1
	for i, msg := range messages {
		if !isCompactionReminder(msg) {
			continue
		}
		if msg.Role == "user" {
			lastUserIdx = i
		} else {
			lastSystemIdx = i
		}
	}
	if lastUserIdx == -1 && lastSystemIdx == -1 {
		return messages
	}
	removeSet := make(map[int]bool)
	for i, msg := range messages {
		if !isCompactionReminder(msg) {
			continue
		}
		if (msg.Role == "user" && i != lastUserIdx) ||
			(msg.Role != "user" && i != lastSystemIdx) {
			removeSet[i] = true
		}
	}
	if len(removeSet) == 0 {
		return messages
	}
	result := make([]provider.Message, 0, len(messages)-len(removeSet))
	for i, msg := range messages {
		if !removeSet[i] {
			result = append(result, msg)
		}
	}
	return result
}

// RemovePerformanceMirrors removes all performance mirror system messages
// except the most recent one. Returns a new slice.
func RemovePerformanceMirrors(messages []provider.Message) []provider.Message {
	lastIdx := -1
	for i, msg := range messages {
		if isPerformanceMirror(msg) {
			lastIdx = i
		}
	}
	if lastIdx == -1 {
		return messages
	}
	removeSet := make(map[int]bool)
	for i, msg := range messages {
		if isPerformanceMirror(msg) && i != lastIdx {
			removeSet[i] = true
		}
	}
	if len(removeSet) == 0 {
		return messages
	}
	result := make([]provider.Message, 0, len(messages)-len(removeSet))
	for i, msg := range messages {
		if !removeSet[i] {
			result = append(result, msg)
		}
	}
	return result
}