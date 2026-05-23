package agent

import (
	"bugbuster-code/pkg/provider"
)

// RemoveToolErrors deletes tool_result with IsError=true and corresponding tool_use.
// Returns cleaned message list.
func RemoveToolErrors(messages []provider.Message) []provider.Message {
	// Collect ToolUseID of erroneous tool_result
	errorIDs := make(map[string]bool)
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.Type == "tool_result" && block.IsError {
				if block.ToolUseID != "" {
					errorIDs[block.ToolUseID] = true
				}
			}
		}
	}

	if len(errorIDs) == 0 {
		return messages
	}

	// Remove erroneous blocks from each message
	result := make([]provider.Message, 0, len(messages))
	for _, msg := range messages {
		var filtered []provider.ContentBlock
		hasContent := false
		for _, block := range msg.Content {
			// Remove tool_result with error
			if block.Type == "tool_result" && block.IsError {
				continue
			}
			// Remove tool_use corresponding to erroneous tool_result
			if block.Type == "tool_use" && errorIDs[block.ToolUseID] {
				continue
			}
			filtered = append(filtered, block)
			hasContent = true
		}
		if hasContent {
			msg.Content = filtered
			result = append(result, msg)
		}
	}
	return result
}

// RemoveDuplicates deletes duplicate and similar messages.
// Keeps last occurrence of each duplicate.
// Uses prefix key (first 200 characters) for detection
// messages with same beginning (model thinking loops).
func RemoveDuplicates(messages []provider.Message) []provider.Message {
	seen := make(map[string]bool)
	seenPrefix := make(map[string]bool) // prefix key for similar messages

	// Semantic deduplication: if 3+ assistant messages start the same
	// (first 50 characters), keep only last 2
	prefix50Count := make(map[string]int)
	for _, msg := range messages {
		if msg.Role == "assistant" {
			text := msg.GetResponseText()
			if len(text) >= 50 {
				prefix50Count[msg.Role+":"+text[:min(len(text), 50)]]++
			}
		}
	}
	duplicatePrefixes := make(map[string]bool)
	for prefix, count := range prefix50Count {
		if count >= 3 {
			duplicatePrefixes[prefix] = true
		}
	}
	prefix50Seen := make(map[string]int) // how many times we've seen this prefix

	// Iterate from end to keep last occurrences
	result := make([]provider.Message, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		text := msg.GetResponseText()
		key := msg.Role + ":" + text
		if key == "system:" || key == "user:" || key == "assistant:" {
			// Empty messages — skip
			continue
		}
		if seen[key] {
			continue
		}
		// For assistant messages check prefix key
		// (catches thinking loops: "Let me start with analysis..." → "Let me start with analysis of detokenization issues...")
		if msg.Role == "assistant" && len(text) >= 50 {
			prefixKey := msg.Role + ":" + text[:min(len(text), 200)]
			if seenPrefix[prefixKey] {
				// Similar message already exists — skip duplicate
				continue
			}
			seenPrefix[prefixKey] = true

			// Semantic deduplication: if 3+ messages with same beginning,
			// keep only last 2
			if len(text) >= 100 {
				shortPrefix := msg.Role + ":" + text[:min(len(text), 50)]
				if duplicatePrefixes[shortPrefix] {
					prefix50Seen[shortPrefix]++
					if prefix50Seen[shortPrefix] > 2 {
						// Already kept 2 messages with this prefix — skip
						continue
					}
				}
			}
		}
		seen[key] = true
		result = append(result, msg)
	}
	// Reverse back
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// MaxToolOutputLines — how many lines to keep when truncating tool output
// Half at the beginning, half at the end

func compactByPriority(messages []provider.Message, maxTokens int) []provider.Message {
	// Preliminary cleanup: delete tool errors, duplicates, orphan pairs (always)
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

	// Phase 1: Truncate tool_result (command outputs) — keeping start and end
	phase1 := make([]provider.Message, len(otherMsgs))
	copy(phase1, otherMsgs)
	for i, msg := range phase1 {
		phase1[i] = truncateToolOutputs(msg)
	}
	phase1 = ensureToolPairIntegrity(phase1)
	if EstimateMessagesTokens(phase1)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(phase1))
		result = append(result, systemMsgs...)
		result = append(result, phase1...)
		return result
	}

	// Phase 1a: Truncate tool_use arguments — long lines are cut
	phase1a := make([]provider.Message, len(phase1))
	copy(phase1a, phase1)
	for i, msg := range phase1a {
		phase1a[i] = truncateToolArgs(msg, MaxToolArgChars)
	}
	phase1a = ensureToolPairIntegrity(phase1a)
	if EstimateMessagesTokens(phase1a)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(phase1a))
		result = append(result, systemMsgs...)
		result = append(result, phase1a...)
		return result
	}

	// Phase 1b: Truncate thinking blocks — long thoughts are cut
	phase1b := make([]provider.Message, len(phase1a))
	copy(phase1b, phase1a)
	for i, msg := range phase1b {
		phase1b[i] = truncateThinking(msg)
	}
	phase1b = ensureToolPairIntegrity(phase1b)
	if EstimateMessagesTokens(phase1b)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(phase1b))
		result = append(result, systemMsgs...)
		result = append(result, phase1b...)
		return result
	}

	// Phase 1c: Truncate long assistant messages — save start and end
	phase1c := make([]provider.Message, len(phase1b))
	copy(phase1c, phase1b)
	for i, msg := range phase1c {
		phase1c[i] = truncateAssistantText(msg)
	}
	phase1c = ensureToolPairIntegrity(phase1c)
	if EstimateMessagesTokens(phase1c)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(phase1c))
		result = append(result, systemMsgs...)
		result = append(result, phase1c...)
		return result
	}

	// Phase 2: Remove tool_use and tool_result fully — keep only thinking and text
	phase2 := make([]provider.Message, 0, len(otherMsgs))
	for _, msg := range otherMsgs {
		stripped := stripToolCalls(msg)
		if !isEmptyMessage(stripped) {
			phase2 = append(phase2, stripped)
		}
	}
	if EstimateMessagesTokens(phase2)+systemTokens <= maxTokens {
		result := make([]provider.Message, 0, len(systemMsgs)+len(phase2))
		result = append(result, systemMsgs...)
		result = append(result, phase2...)
		return result
	}

	// Phase 3: Remove old messages from the end, keeping recent ones
	// Start removing from the beginning (oldest)
	for i := 0; i < len(phase2); i++ {
		remaining := phase2[i+1:]
		if EstimateMessagesTokens(remaining)+systemTokens <= maxTokens {
			result := make([]provider.Message, 0, len(systemMsgs)+len(remaining))
			result = append(result, systemMsgs...)
			result = append(result, remaining...)
			return result
		}
	}

	// Phase 4: Only system prompt
	return systemMsgs
}

// CompactContext compacts context by tokens
// Compaction priority: tool errors and duplicates → tool_result → tool_use → old messages → thinking/text
