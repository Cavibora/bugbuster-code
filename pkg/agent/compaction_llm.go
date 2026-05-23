package agent

// Compactor defines the interface for context compaction strategies.
// Implementations can use LLM summarization, simple truncation, or other approaches
// to reduce the context size while preserving important information.
import (
	"context"
	"fmt"
	"strings"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

type Compactor interface {
	// Summarize creates a summary of messages
	Summarize(messages []provider.Message, maxTokens int) string
}

// LLMCompactor is an LLM-based compactor (uses provider to generate summary)
type LLMCompactor struct {
	provider provider.Provider
}

// NewLLMCompactor creates LLM-compactor
func NewLLMCompactor(p provider.Provider) *LLMCompactor {
	return &LLMCompactor{provider: p}
}

// Summarize generates a summary via LLM (with 30 second timeout)
func (c *LLMCompactor) Summarize(messages []provider.Message, maxTokens int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return c.SummarizeWithCtx(messages, maxTokens, ctx)
}

// extractExistingSummary finds a previous summary in system messages
func extractExistingSummary(messages []provider.Message) string {
	header := i18n.T("compaction.summary_header")
	for _, msg := range messages {
		if msg.Role != "system" {
			continue
		}
		text := msg.GetText()
		if strings.HasPrefix(text, header) {
			return strings.TrimPrefix(text, header)
		}
	}
	return ""
}

// SummarizeWithCtx generates a summary via LLM with cancellation context
// Supports incremental update: if there is a previous summary in messages,
// passes it as <previous-summary> for updating instead of generating from scratch.
func (c *LLMCompactor) SummarizeWithCtx(messages []provider.Message, maxTokens int, ctx context.Context) string {
	if c.provider == nil {
		return SimpleSummarize(messages, maxTokens)
	}

	// Check context cancellation before starting
	select {
	case <-ctx.Done():
		return SimpleSummarize(messages, maxTokens)
	default:
	}

	// Check if there is a previous summary
	previousSummary := extractExistingSummary(messages)

	// Collect text from old messages (without tool_result — they are the most voluminous)
	var sb strings.Builder
	sb.WriteString("Previous conversation:\n\n")
	for _, msg := range messages {
		text := msg.GetText()
		if text == "" {
			continue
		}
		switch msg.Role {
		case "user":
			sb.WriteString("User: ")
		case "assistant":
			sb.WriteString("Assistant: ")
		default:
			continue
		}
		// Limit the length
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}

	var prompt string
	if previousSummary != "" {
		prompt = fmt.Sprintf(
			"You are updating an existing conversation summary with new information.\n\n"+
				"<previous-summary>\n%s\n</previous-summary>\n\n"+
				"<new-conversation>\n%s\n</new-conversation>\n\n"+
				"Update the summary to incorporate the new information. "+
				"Keep the summary under %d tokens. Write in the same language as the conversation.",
			previousSummary, sb.String(), maxTokens,
		)
	} else {
		prompt = fmt.Sprintf(
			"Summarize the following conversation in a concise way, preserving key decisions, code changes, and important context. "+
				"Keep the summary under %d tokens. Write in the same language as the conversation.\n\n%s",
			maxTokens, sb.String(),
		)
	}

	summaryMsg := []provider.Message{
		provider.SystemMsg("You are a conversation summarizer. Provide concise, accurate summaries preserving all important context."),
		provider.UserMsg(prompt),
	}

	// Run LLM request in a goroutine with a hard timeout.
	type summarizeResult struct {
		res *provider.CompletionResult
		err error
	}
	done := make(chan summarizeResult, 1)
	go func() {
		compactionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		res, err := c.provider.CompleteWithCtx(compactionCtx, summaryMsg, nil)
		done <- summarizeResult{res, err}
	}()

	select {
	case r := <-done:
		if r.err != nil || r.res == nil {
			return SimpleSummarize(messages, maxTokens)
		}
		summary := r.res.Message.GetText()
		if summary == "" {
			return SimpleSummarize(messages, maxTokens)
		}
		return i18n.T("compaction.summary_header") + summary + "\n"
	case <-time.After(10 * time.Second):
		return SimpleSummarize(messages, maxTokens)
	case <-ctx.Done():
		return SimpleSummarize(messages, maxTokens)
	}
}

// SimpleSummarize is simple truncation without LLM (fallback)
func SimpleSummarize(messages []provider.Message, maxTokens int) string {
	var sb strings.Builder
	sb.WriteString(i18n.T("compaction.summary_header"))

	for _, msg := range messages {
		text := msg.GetText()
		if text == "" {
			continue
		}

		// Truncate long messages
		maxLen := maxTokens * charsPerToken / len(messages)
		if maxLen < 50 {
			maxLen = 50
		}
		if maxLen > 500 {
			maxLen = 500
		}

		if len(text) > maxLen {
			text = text[:maxLen] + "..."
		}

		switch msg.Role {
		case "user":
			sb.WriteString(i18n.T("compaction.user_label"))
		case "assistant":
			sb.WriteString(i18n.T("compaction.assistant_label"))
		default:
			continue
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}

	return sb.String()
}
