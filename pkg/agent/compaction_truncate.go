package agent

import (
	"fmt"
	"strings"

	"bugbuster-code/pkg/provider"
)

// MaxToolOutputLines is the maximum number of lines to keep from tool output
// when truncating large tool results.
const MaxToolOutputLines = 10

// MaxThinkingChars — max character count in thinking block during compaction
const MaxThinkingChars = 500

// MaxToolArgChars — max character count for each string value
// in tool_use Input block during truncation
const MaxToolArgChars = 200

// MaxAssistantTextChars — max text length of assistant messages during compaction.
// Messages longer than this threshold are truncated: first 500 and last 500 characters are kept.
const MaxAssistantTextChars = 2000

// AssistantTextHead — how many characters to keep at start of truncated assistant messages
const AssistantTextHead = 500

// AssistantTextTail — how many characters to keep at end of truncated assistant messages
const AssistantTextTail = 500

// truncateStringLines truncates multi-line text, keeping first headLines
// and last tailLines lines with marker "[...N lines truncated...]" between them.
func truncateStringLines(text string, headLines, tailLines int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= headLines+tailLines+1 {
		return text // no need to truncate
	}
	truncated := len(lines) - headLines - tailLines
	head := lines[:headLines]
	tail := lines[len(lines)-tailLines:]
	return strings.Join(head, "\n") + "\n[..." + fmt.Sprintf("%d lines truncated...", truncated) + "]\n" + strings.Join(tail, "\n")
}

// truncateToolOutputs truncates tool output, keeping ToolUseID, ToolName,
// first and last output lines. Replaces deprecated stripToolResults.
func truncateToolOutputs(msg provider.Message) provider.Message {
	var filtered []provider.ContentBlock
	for _, block := range msg.Content {
		if block.Type == "tool_result" {
			truncated := truncateStringLines(block.Output, MaxToolOutputLines/2, MaxToolOutputLines/2)
			filtered = append(filtered, provider.ContentBlock{
				Type:      "tool_result",
				ToolUseID: block.ToolUseID,
				ToolName:  block.ToolName,
				Output:    truncated,
				IsError:   block.IsError,
			})
		} else {
			filtered = append(filtered, block)
		}
	}
	if len(filtered) == 0 {
		return msg
	}
	msg.Content = filtered
	return msg
}

// truncateToolOutputsToBudget aggressively truncates tool_result to token budget.
// Unlike truncateToolOutputs (which saves start and end by lines),
// this function cuts Output to maxTokens*charsPerToken characters.
func truncateToolOutputsToBudget(msg provider.Message, maxTokens int) provider.Message {
	maxChars := maxTokens * charsPerToken
	if maxChars < 100 {
		maxChars = 100
	}
	modified := false
	var filtered []provider.ContentBlock
	for _, block := range msg.Content {
		if block.Type == "tool_result" && len(block.Output) > maxChars {
			filtered = append(filtered, provider.ContentBlock{
				Type:      "tool_result",
				ToolUseID: block.ToolUseID,
				ToolName:  block.ToolName,
				Output:    block.Output[:maxChars] + "\n...[truncated]",
				IsError:   block.IsError,
			})
			modified = true
		} else {
			filtered = append(filtered, block)
		}
	}
	if !modified || len(filtered) == 0 {
		return msg
	}
	msg.Content = filtered
	return msg
}

// truncateThinking truncates thinking blocks in message.
// Long thinking blocks are truncated to MaxThinkingChars characters
// with truncation marker at the end.
func truncateThinking(msg provider.Message) provider.Message {
	modified := false
	var filtered []provider.ContentBlock
	for _, block := range msg.Content {
		if block.Type == "thinking" && len(block.Text) > MaxThinkingChars {
			filtered = append(filtered, provider.ContentBlock{
				Type: block.Type,
				Text: block.Text[:MaxThinkingChars] + "\n...[thinking truncated]",
			})
			modified = true
		} else {
			filtered = append(filtered, block)
		}
	}
	if !modified {
		return msg
	}
	msg.Content = filtered
	return msg
}

// truncateAssistantText truncates long assistant messages.
// Keeps first AssistantTextHead and last AssistantTextTail characters
// with truncation marker between them.
func truncateAssistantText(msg provider.Message) provider.Message {
	if msg.Role != "assistant" {
		return msg
	}
	modified := false
	var filtered []provider.ContentBlock
	for _, block := range msg.Content {
		if block.Type == "text" && len(block.Text) > MaxAssistantTextChars {
			head := AssistantTextHead
			tail := AssistantTextTail
			text := block.Text
			truncated := len(text) - head - tail
			if truncated < 0 {
				truncated = 0
			}
			var newText string
			if tail > 0 && len(text) > head+tail {
				newText = text[:head] + fmt.Sprintf("\n...[%d chars truncated]...\n", truncated) + text[len(text)-tail:]
			} else {
				newText = text[:head] + fmt.Sprintf("\n...[%d chars truncated]...", truncated)
			}
			filtered = append(filtered, provider.ContentBlock{
				Type: block.Type,
				Text: newText,
			})
			modified = true
		} else {
			filtered = append(filtered, block)
		}
	}
	if !modified {
		return msg
	}
	if len(filtered) == 0 {
		return msg
	}
	msg.Content = filtered
	return msg
}

// truncateToolArgs truncates string values in tool_use Input map,
// keeping first maxChars characters with "...[truncated]" marker.
// Preserves valid JSON structure of Input.
func truncateToolArgs(msg provider.Message, maxChars int) provider.Message {
	modified := false
	var filtered []provider.ContentBlock
	for _, block := range msg.Content {
		if block.Type == "tool_use" && len(block.Input) > 0 {
			newInput := make(map[string]any, len(block.Input))
			for k, v := range block.Input {
				if s, ok := v.(string); ok && len(s) > maxChars {
					newInput[k] = s[:maxChars] + "...[truncated]"
					modified = true
				} else {
					newInput[k] = v
				}
			}
			filtered = append(filtered, provider.ContentBlock{
				Type:      block.Type,
				Text:      block.Text,
				ToolUseID: block.ToolUseID,
				ToolName:  block.ToolName,
				Input:     newInput,
			})
		} else {
			filtered = append(filtered, block)
		}
	}
	if !modified {
		return msg
	}
	if len(filtered) == 0 {
		return msg
	}
	msg.Content = filtered
	return msg
}

// EnsureToolPairIntegrity deletes orphan tool_use (without tool_result)
// and orphan tool_result (without tool_use) from message list.
// Anthropic API requires strict pairing: every tool_use must have tool_result
// with matching ToolUseID, and vice versa.
func EnsureToolPairIntegrity(messages []provider.Message) []provider.Message {
	// Collect all ToolUseID from tool_use and tool_result blocks
	useIDs := make(map[string]bool)
	resultIDs := make(map[string]bool)
	for _, msg := range messages {
		for _, block := range msg.Content {
			switch block.Type {
			case "tool_use":
				if block.ToolUseID != "" {
					useIDs[block.ToolUseID] = true
				}
			case "tool_result":
				if block.ToolUseID != "" {
					resultIDs[block.ToolUseID] = true
				}
			}
		}
	}

	// Check if all pairs are intact
	allPaired := true
	for id := range useIDs {
		if !resultIDs[id] {
			allPaired = false
			break
		}
	}
	if allPaired {
		for id := range resultIDs {
			if !useIDs[id] {
				allPaired = false
				break
			}
		}
	}
	if allPaired {
		return messages // no orphans — nothing to do
	}

	// Collect orphan ToolUseID
	orphanIDs := make(map[string]bool)
	for id := range useIDs {
		if !resultIDs[id] {
			orphanIDs[id] = true
		}
	}
	for id := range resultIDs {
		if !useIDs[id] {
			orphanIDs[id] = true
		}
	}

	// Remove orphan blocks, but preserve tool_result in user messages
	// (they contain important context even without paired tool_use)
	result := make([]provider.Message, 0, len(messages))
	for _, msg := range messages {
		var filtered []provider.ContentBlock
		hasContent := false
		for _, block := range msg.Content {
			if block.Type == "tool_use" && orphanIDs[block.ToolUseID] {
				// Always remove orphan tool_use — it's useless without tool_result
				continue
			}
			if block.Type == "tool_result" && orphanIDs[block.ToolUseID] {
				if msg.Role == "user" {
					// Convert orphan tool_result to text block — preserve the output
					text := block.Output
					if text != "" {
						filtered = append(filtered, provider.ContentBlock{
							Type: "text",
							Text: "[Tool output]\n" + text,
						})
						hasContent = true
					}
					continue
				}
				// Remove orphan tool_result from non-user messages
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

// stripToolResults — DEPRECATED: use truncateToolOutputs.
// Truncates tool_result but loses ToolUseID/ToolName.
func stripToolResults(msg provider.Message) provider.Message {
	var filtered []provider.ContentBlock
	for _, block := range msg.Content {
		if block.Type == "tool_result" {
			filtered = append(filtered, provider.ContentBlock{
				Type:   "tool_result",
				Text:   "",
				Output: "[output truncated]",
			})
		} else {
			filtered = append(filtered, block)
		}
	}
	if len(filtered) == 0 {
		return msg
	}
	msg.Content = filtered
	return msg
}

// stripToolCalls deletes tool_use and tool_result blocks from messages.
// Returns message with only thinking and text blocks.
// Preserves tool_result blocks in user messages — they contain important context.
func stripToolCalls(msg provider.Message) provider.Message {
	var filtered []provider.ContentBlock
	for _, block := range msg.Content {
		if msg.Role == "user" && block.Type == "tool_result" {
			// Keep tool_result in user messages — they contain important context
			filtered = append(filtered, block)
			continue
		}
		if block.Type == "tool_use" || block.Type == "tool_result" {
			continue // fully delete
		}
		filtered = append(filtered, block)
	}
	if len(filtered) == 0 {
		return provider.Message{} // empty message
	}
	msg.Content = filtered
	return msg
}

// isEmptyMessage checks if message is empty (no meaningful content)
func isEmptyMessage(msg provider.Message) bool {
	if msg.Role == "system" {
		return false // system messages are always important
	}
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				return false
			}
		case "thinking":
			if strings.TrimSpace(block.Text) != "" {
				return false
			}
		default:
			return false // has tool_use/tool_result — not empty
		}
	}
	return true
}

// truncateMessageToFit truncates message to given token budget.
// Applies all truncation phases: tool_result → tool_use args → thinking → assistant text → strip tool calls
func truncateMessageToFit(msg provider.Message, maxTokens int) provider.Message {
	result := msg
	tokens := EstimateMessagesTokens([]provider.Message{result})

	// Phase 1: Truncate tool_result (standard truncation by lines)
	result = truncateToolOutputs(result)
	tokens = EstimateMessagesTokens([]provider.Message{result})
	if tokens <= maxTokens {
		return result
	}

	// Phase 1a: Aggressive tool_result truncation by token budget
	result = truncateToolOutputsToBudget(result, maxTokens)
	tokens = EstimateMessagesTokens([]provider.Message{result})
	if tokens <= maxTokens {
		return result
	}

	// Phase 2: Truncate tool_use arguments
	result = truncateToolArgs(result, MaxToolArgChars)
	tokens = EstimateMessagesTokens([]provider.Message{result})
	if tokens <= maxTokens {
		return result
	}

	// Phase 3: Truncate thinking
	result = truncateThinking(result)
	tokens = EstimateMessagesTokens([]provider.Message{result})
	if tokens <= maxTokens {
		return result
	}

	// Phase 4: Truncate assistant text
	result = truncateAssistantText(result)
	tokens = EstimateMessagesTokens([]provider.Message{result})
	if tokens <= maxTokens {
		return result
	}

	// Phase 5: Remove tool_use and tool_result
	result = stripToolCalls(result)
	if !isEmptyMessage(result) {
		tokens = EstimateMessagesTokens([]provider.Message{result})
		if tokens <= maxTokens {
			return result
		}
	}

	// Phase 6: Cut text and tool_result to budget (hard truncation)
	maxChars := maxTokens * charsPerToken
	if maxChars < 100 {
		maxChars = 100
	}
	var filtered []provider.ContentBlock
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			if len(block.Text) > maxChars {
				block.Text = block.Text[:maxChars] + "\n...[truncated]"
			}
			filtered = append(filtered, block)
		case "tool_result":
			if len(block.Output) > maxChars {
				block.Output = block.Output[:maxChars] + "\n...[truncated]"
			}
			filtered = append(filtered, block)
		default:
			filtered = append(filtered, block)
		}
	}
	if len(filtered) > 0 {
		result.Content = filtered
	}

	return result
}

// compactByPriority compacts messages by priority:
// 0. Removes tool_results with errors, duplicates, orphan pairs
// 1. Truncates tool_result (command outputs) — keeping start and end
// 1a. Truncates tool_use arguments — long lines are trimmed
// 2. Removes tool_use and tool_result — leaves only thinking and text
// 3. Removes old messages entirely
// 4. Only system prompt
// Keeps: system, thinking, text — most important for context
