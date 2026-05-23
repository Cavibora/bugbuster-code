package agent

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

// TokenEstimate — approximate token estimation
// English: ~4 chars/token, Russian: ~2 chars/token
// Using average: ~3 chars/token
const charsPerToken = 3

// EstimateTokens estimates token count in text
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}

	// Count characters, excluding extra spaces
	runes := []rune(text)
	totalChars := 0
	prevSpace := false

	for _, r := range runes {
		if unicode.IsSpace(r) {
			if !prevSpace {
				totalChars++
				prevSpace = true
			}
		} else {
			totalChars++
			prevSpace = false
		}
	}

	// For Cyrillic — more tokens (fewer chars per token)
	cyrillicCount := 0
	for _, r := range runes {
		if unicode.Is(unicode.Cyrillic, r) {
			cyrillicCount++
		}
	}

	// If more than 30% Cyrillic — use 2 chars/token
	if totalChars > 0 && float64(cyrillicCount)/float64(len(runes)) > 0.3 {
		return totalChars / 2
	}

	return totalChars / charsPerToken
}

// EstimateMessagesTokens estimates tokens in message list
func EstimateMessagesTokens(messages []provider.Message) int {
	total := 0
	for _, msg := range messages {
		// Message role ~4 tokens
		total += 4
		// Message text
		total += EstimateTokens(msg.GetText())
		// Tool calls — each ~10 tokens overhead + input
		for _, block := range msg.Content {
			switch block.Type {
			case "tool_use":
				total += 10
				for _, v := range block.Input {
					total += EstimateTokens(fmt.Sprintf("%v", v))
				}
			case "tool_result":
				total += 5
				total += EstimateTokens(block.Output)
			}
		}
	}
	return total
}

// extractRecaps extracts ※ Recap: blocks from assistant messages
func extractRecaps(messages []provider.Message) []string {
	var recaps []string
	for _, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}
		text := msg.GetText()
		if text == "" {
			continue
		}
		for {
			idx := strings.Index(text, "※ Recap:")
			if idx == -1 {
				break
			}
			after := text[idx+len("※ Recap:"):]
			end := strings.Index(after, "\n")
			recapText := after
			if end >= 0 {
				recapText = after[:end]
			}
			recapText = strings.TrimSpace(recapText)
			if recapText != "" {
				recaps = append(recaps, recapText)
			}
			if end >= 0 {
				text = after[end:]
			} else {
				break
			}
		}
	}
	return recaps
}

// buildRecapMessage creates system message with saved recaps
func buildRecapMessage(recaps []string) provider.Message {
	var sb strings.Builder
	sb.WriteString(i18n.T("compaction.recap_header"))
	for i, r := range recaps {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, r))
	}
	return provider.Message{
		Role: "system",
		Content: []provider.ContentBlock{
			{Type: "text", Text: sb.String()},
		},
	}
}

// RemoveToolErrors deletes tool_result with IsError=true and corresponding tool_use.
// Returns cleaned message list.

// CompactContext reduces the context size by removing tool errors, duplicates,
// orphan tool pairs, and truncating tool outputs. It preserves the most recent
// keepRecent messages and ensures the total token count stays within maxTokens.
func CompactContext(messages []provider.Message, maxTokens int, keepRecent int) []provider.Message {
	// Always clean tool errors, duplicates, orphan pairs
	messages = RemoveToolErrors(messages)
	messages = RemoveDuplicates(messages)
	messages = ensureToolPairIntegrity(messages)

	currentTokens := EstimateMessagesTokens(messages)
	if currentTokens <= maxTokens {
		return messages
	}

	// Split into system and rest
	var systemMsgs []provider.Message
	var otherMsgs []provider.Message
	for _, m := range messages {
		if m.Role == "system" {
			systemMsgs = append(systemMsgs, m)
		} else {
			otherMsgs = append(otherMsgs, m)
		}
	}

	systemTokens := EstimateMessagesTokens(systemMsgs)
	availableTokens := maxTokens - systemTokens

	if availableTokens <= 0 {
		return systemMsgs
	}

	// Extract recaps from old messages before compaction
	recaps := extractRecaps(otherMsgs)

	// Keep last keepRecent messages
	if keepRecent > len(otherMsgs) {
		keepRecent = len(otherMsgs)
	}

	// Dynamic keepRecent: if last keepRecent messages
	// take more than 30% of maxTokens, reduce keepRecent
	maxRecentTokens := availableTokens / 4
	for keepRecent > 2 && keepRecent <= len(otherMsgs) {
		recentTest := otherMsgs[len(otherMsgs)-keepRecent:]
		if EstimateMessagesTokens(recentTest) <= maxRecentTokens {
			break
		}
		keepRecent--
	}
	if keepRecent < 2 {
		keepRecent = 2
	}

	recentMsgs := otherMsgs[len(otherMsgs)-keepRecent:]
	oldMsgs := otherMsgs[:len(otherMsgs)-keepRecent]

	recentTokens := EstimateMessagesTokens(recentMsgs)

	// Helper function: adds recap to result if it fits in limit
	tryAddRecap := func(result []provider.Message, tail []provider.Message) []provider.Message {
		if len(recaps) == 0 {
			return append(result, tail...)
		}
		recapMsg := buildRecapMessage(recaps)
		recapTokens := EstimateMessagesTokens([]provider.Message{recapMsg})
		currentTokens := EstimateMessagesTokens(result)
		tailTokens := EstimateMessagesTokens(tail)
		if currentTokens+recapTokens+tailTokens <= maxTokens {
			result = append(result, recapMsg)
		}
		return append(result, tail...)
	}

	// If last messages fit — compact old ones by priority
	if recentTokens <= availableTokens {
		remainingTokens := availableTokens - recentTokens

		if len(oldMsgs) > 0 && remainingTokens > 50 {
			// Compact old messages by priority: tool_result → tool_use → text
			compactedOld := compactByPriority(oldMsgs, remainingTokens)
			// If compaction produced result — use it
			if EstimateMessagesTokens(compactedOld)+systemTokens <= maxTokens {
				result := make([]provider.Message, 0, len(systemMsgs)+len(compactedOld)+len(recentMsgs)+1)
				result = append(result, systemMsgs...)
				result = append(result, compactedOld...)
				return tryAddRecap(result, recentMsgs)
			}

			// Otherwise — summarize old messages
			summary := SimpleSummarize(oldMsgs, remainingTokens)
			summaryMsg := provider.Message{
				Role: "system",
				Content: []provider.ContentBlock{
					{Type: "text", Text: summary},
				},
			}
			result := make([]provider.Message, 0, len(systemMsgs)+1+len(recentMsgs)+1)
			result = append(result, systemMsgs...)
			result = append(result, summaryMsg)
			return tryAddRecap(result, recentMsgs)
		}

		result := make([]provider.Message, 0, len(systemMsgs)+len(recentMsgs)+1)
		result = append(result, systemMsgs...)
		return tryAddRecap(result, recentMsgs)
	}

	// Last messages do not fit — trim them by priority
	// First try to truncate tool_result in recent messages
	compactedRecent := make([]provider.Message, len(recentMsgs))
	copy(compactedRecent, recentMsgs)
	for i, msg := range compactedRecent {
		compactedRecent[i] = truncateToolOutputs(msg)
	}
	compactedRecent = ensureToolPairIntegrity(compactedRecent)
	if EstimateMessagesTokens(compactedRecent)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(compactedRecent))
		result = append(result, systemMsgs...)
		result = append(result, compactedRecent...)
		return result
	}

	// Truncate tool_use arguments in recent messages
	compactedRecent1a := make([]provider.Message, len(compactedRecent))
	copy(compactedRecent1a, compactedRecent)
	for i, msg := range compactedRecent1a {
		compactedRecent1a[i] = truncateToolArgs(msg, MaxToolArgChars)
	}
	compactedRecent1a = ensureToolPairIntegrity(compactedRecent1a)
	if EstimateMessagesTokens(compactedRecent1a)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(compactedRecent1a))
		result = append(result, systemMsgs...)
		result = append(result, compactedRecent1a...)
		return result
	}

	// Truncate thinking blocks in recent messages
	compactedRecent1b := make([]provider.Message, len(compactedRecent1a))
	copy(compactedRecent1b, compactedRecent1a)
	for i, msg := range compactedRecent1b {
		compactedRecent1b[i] = truncateThinking(msg)
	}
	compactedRecent1b = ensureToolPairIntegrity(compactedRecent1b)
	if EstimateMessagesTokens(compactedRecent1b)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(compactedRecent1b))
		result = append(result, systemMsgs...)
		result = append(result, compactedRecent1b...)
		return result
	}

	// Phase 1c: Truncate long assistant messages in recent messages
	compactedRecent1c := make([]provider.Message, len(compactedRecent1b))
	copy(compactedRecent1c, compactedRecent1b)
	for i, msg := range compactedRecent1c {
		compactedRecent1c[i] = truncateAssistantText(msg)
	}
	compactedRecent1c = ensureToolPairIntegrity(compactedRecent1c)
	if EstimateMessagesTokens(compactedRecent1c)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(compactedRecent1c))
		result = append(result, systemMsgs...)
		result = append(result, compactedRecent1c...)
		return result
	}

	// Remove tool_use and tool_result from recent messages
	compactedRecent2 := make([]provider.Message, 0, len(recentMsgs))
	for _, msg := range recentMsgs {
		stripped := stripToolCalls(msg)
		if !isEmptyMessage(stripped) {
			compactedRecent2 = append(compactedRecent2, stripped)
		}
	}
	if EstimateMessagesTokens(compactedRecent2)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(compactedRecent2))
		result = append(result, systemMsgs...)
		result = append(result, compactedRecent2...)
		return result
	}

	// Fallback — truncate individual messages that don't fit
	// First try to add messages from the end, truncating those that do not fit
	result := make([]provider.Message, 0, len(systemMsgs)+len(recentMsgs))
	result = append(result, systemMsgs...)
	tokensUsed := systemTokens

	for i := len(recentMsgs) - 1; i >= 0; i-- {
		msgTokens := EstimateMessagesTokens([]provider.Message{recentMsgs[i]})
		if tokensUsed+msgTokens <= maxTokens {
			// Message fits entirely
			result = append(result, recentMsgs[i])
			tokensUsed += msgTokens
		} else {
			// Message does not fit — truncate to remaining budget
			remainingTokens := maxTokens - tokensUsed
			if remainingTokens > 100 {
				truncated := truncateMessageToFit(recentMsgs[i], remainingTokens)
				if EstimateMessagesTokens([]provider.Message{truncated}) > 0 {
					result = append(result, truncated)
				}
			}
			break
		}
	}

	// Reverse order (we added from the end)
	var reversed []provider.Message
	for i := len(result) - 1; i >= len(systemMsgs); i-- {
		reversed = append(reversed, result[i])
	}

	result = make([]provider.Message, 0, len(systemMsgs)+len(reversed)+1)
	result = append(result, systemMsgs...)
	return tryAddRecap(result, reversed)
}

// CompactContextWithCompactor compacts context using a Compactor for LLM summarization.
// Compaction priority: tool errors and duplicates → tool_result → tool_use → old messages → thinking/text.
// If the compactor implements SummarizeWithCtx, it uses context-aware summarization.
func CompactContextWithCompactor(messages []provider.Message, maxTokens int, keepRecent int, compactor Compactor, ctx context.Context) []provider.Message {
	// Always clean tool errors, duplicates, orphan pairs
	messages = RemoveToolErrors(messages)
	messages = RemoveDuplicates(messages)
	messages = ensureToolPairIntegrity(messages)

	currentTokens := EstimateMessagesTokens(messages)
	if currentTokens <= maxTokens {
		return messages
	}

	var systemMsgs []provider.Message
	var otherMsgs []provider.Message
	for _, m := range messages {
		if m.Role == "system" {
			systemMsgs = append(systemMsgs, m)
		} else {
			otherMsgs = append(otherMsgs, m)
		}
	}

	systemTokens := EstimateMessagesTokens(systemMsgs)
	availableTokens := maxTokens - systemTokens
	if availableTokens <= 0 {
		return systemMsgs
	}

	// Extract recaps from old messages before compaction
	recaps := extractRecaps(otherMsgs)

	if keepRecent > len(otherMsgs) {
		keepRecent = len(otherMsgs)
	}

	// Dynamic keepRecent: if last keepRecent messages
	// take more than 30% of maxTokens, reduce keepRecent
	maxRecentTokens := availableTokens / 4
	for keepRecent > 2 && keepRecent <= len(otherMsgs) {
		recentTest := otherMsgs[len(otherMsgs)-keepRecent:]
		if EstimateMessagesTokens(recentTest) <= maxRecentTokens {
			break
		}
		keepRecent--
	}
	if keepRecent < 2 {
		keepRecent = 2
	}

	recentMsgs := otherMsgs[len(otherMsgs)-keepRecent:]
	oldMsgs := otherMsgs[:len(otherMsgs)-keepRecent]

	recentTokens := EstimateMessagesTokens(recentMsgs)

	// Helper function: adds recap to result if it fits in limit
	tryAddRecap := func(result []provider.Message, tail []provider.Message) []provider.Message {
		if len(recaps) == 0 {
			return append(result, tail...)
		}
		recapMsg := buildRecapMessage(recaps)
		recapTokens := EstimateMessagesTokens([]provider.Message{recapMsg})
		currentTokens := EstimateMessagesTokens(result)
		tailTokens := EstimateMessagesTokens(tail)
		if currentTokens+recapTokens+tailTokens <= maxTokens {
			result = append(result, recapMsg)
		}
		return append(result, tail...)
	}

	if recentTokens <= availableTokens {
		remainingTokens := availableTokens - recentTokens

		if len(oldMsgs) > 0 && remainingTokens > 50 {
			// Compact old messages by priority
			compactedOld := compactByPriority(oldMsgs, remainingTokens)
			if EstimateMessagesTokens(compactedOld)+systemTokens <= maxTokens {
				result := make([]provider.Message, 0, len(systemMsgs)+len(compactedOld)+len(recentMsgs)+1)
				result = append(result, systemMsgs...)
				result = append(result, compactedOld...)
				return tryAddRecap(result, recentMsgs)
			}

			// LLM summary of old messages
			// Add previous summary for incremental update
			summaryInput := oldMsgs
			existingSummary := extractExistingSummary(systemMsgs)
			if existingSummary != "" {
				prevSummaryMsg := provider.Message{
					Role: "system",
					Content: []provider.ContentBlock{
						{Type: "text", Text: i18n.T("compaction.summary_header") + existingSummary},
					},
				}
				summaryInput = append([]provider.Message{prevSummaryMsg}, oldMsgs...)
			}
			var summary string
			if ctxCompactor, ok := compactor.(interface {
				SummarizeWithCtx([]provider.Message, int, context.Context) string
			}); ok {
				summary = ctxCompactor.SummarizeWithCtx(summaryInput, remainingTokens, ctx)
			} else {
				summary = compactor.Summarize(summaryInput, remainingTokens)
			}
			summaryMsg := provider.Message{
				Role: "system",
				Content: []provider.ContentBlock{
					{Type: "text", Text: summary},
				},
			}
			result := make([]provider.Message, 0, len(systemMsgs)+1+len(recentMsgs)+1)
			result = append(result, systemMsgs...)
			result = append(result, summaryMsg)
			return tryAddRecap(result, recentMsgs)
		}

		result := make([]provider.Message, 0, len(systemMsgs)+len(recentMsgs)+1)
		result = append(result, systemMsgs...)
		return tryAddRecap(result, recentMsgs)
	}

	// Last messages do not fit — trim by priority
	compactedRecent := make([]provider.Message, len(recentMsgs))
	copy(compactedRecent, recentMsgs)
	for i, msg := range compactedRecent {
		compactedRecent[i] = truncateToolOutputs(msg)
	}
	compactedRecent = ensureToolPairIntegrity(compactedRecent)
	if EstimateMessagesTokens(compactedRecent)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(compactedRecent))
		result = append(result, systemMsgs...)
		result = append(result, compactedRecent...)
		return result
	}

	// Truncate tool_use arguments in recent messages
	compactedRecent1a := make([]provider.Message, len(compactedRecent))
	copy(compactedRecent1a, compactedRecent)
	for i, msg := range compactedRecent1a {
		compactedRecent1a[i] = truncateToolArgs(msg, MaxToolArgChars)
	}
	compactedRecent1a = ensureToolPairIntegrity(compactedRecent1a)
	if EstimateMessagesTokens(compactedRecent1a)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(compactedRecent1a))
		result = append(result, systemMsgs...)
		result = append(result, compactedRecent1a...)
		return result
	}

	// Truncate thinking blocks in recent messages
	compactedRecent1b := make([]provider.Message, len(compactedRecent1a))
	copy(compactedRecent1b, compactedRecent1a)
	for i, msg := range compactedRecent1b {
		compactedRecent1b[i] = truncateThinking(msg)
	}
	compactedRecent1b = ensureToolPairIntegrity(compactedRecent1b)
	if EstimateMessagesTokens(compactedRecent1b)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(compactedRecent1b))
		result = append(result, systemMsgs...)
		result = append(result, compactedRecent1b...)
		return result
	}

	// Phase 1c: Truncate long assistant messages in recent messages
	compactedRecent1c := make([]provider.Message, len(compactedRecent1b))
	copy(compactedRecent1c, compactedRecent1b)
	for i, msg := range compactedRecent1c {
		compactedRecent1c[i] = truncateAssistantText(msg)
	}
	compactedRecent1c = ensureToolPairIntegrity(compactedRecent1c)
	if EstimateMessagesTokens(compactedRecent1c)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(compactedRecent1c))
		result = append(result, systemMsgs...)
		result = append(result, compactedRecent1c...)
		return result
	}

	compactedRecent2 := make([]provider.Message, 0, len(recentMsgs))
	for _, msg := range recentMsgs {
		stripped := stripToolCalls(msg)
		if !isEmptyMessage(stripped) {
			compactedRecent2 = append(compactedRecent2, stripped)
		}
	}
	if EstimateMessagesTokens(compactedRecent2)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(compactedRecent2))
		result = append(result, systemMsgs...)
		result = append(result, compactedRecent2...)
		return result
	}

	// Fallback — truncate individual messages that don't fit
	result := make([]provider.Message, 0, len(systemMsgs)+len(recentMsgs))
	result = append(result, systemMsgs...)
	tokensUsed := systemTokens
	for i := len(recentMsgs) - 1; i >= 0; i-- {
		msgTokens := EstimateMessagesTokens([]provider.Message{recentMsgs[i]})
		if tokensUsed+msgTokens <= maxTokens {
			result = append(result, recentMsgs[i])
			tokensUsed += msgTokens
		} else {
			remainingTokens := maxTokens - tokensUsed
			if remainingTokens > 100 {
				truncated := truncateMessageToFit(recentMsgs[i], remainingTokens)
				if EstimateMessagesTokens([]provider.Message{truncated}) > 0 {
					result = append(result, truncated)
				}
			}
			break
		}
	}
	var rev []provider.Message
	for i := len(result) - 1; i >= len(systemMsgs); i-- {
		rev = append(rev, result[i])
	}
	result = make([]provider.Message, 0, len(systemMsgs)+len(rev)+1)
	result = append(result, systemMsgs...)
	return tryAddRecap(result, rev)
}

// Compactor is an interface for context compaction
