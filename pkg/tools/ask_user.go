package tools

import (
	"sync"
	"time"

	"bugbuster-code/pkg/i18n"
)

// AskFunc is a function for requesting a response from the user.
// question is the question text, returns the user's response or empty string.
type AskFunc func(question string) string

// AskUserTool is a tool for requesting information from the user.
// Supports three modes:
// 1. AskChannel (TUI) — question/response via Go channels
// 2. AskFunc (CLI) — question via callback (readline)
// 3. Fallback — non-interactive mode (auto-skip)
type AskUserTool struct {
	NonInteractive bool // in non-interactive mode — auto-skip

	// AskChannel is a channel for TUI mode.
	mu         sync.Mutex
	askChannel *AskChannel

	// AskFunc is a callback for CLI mode (readline).
	// Set by SplitTerminal during initialization.
	askFunc AskFunc

	// Cancel channel — signals ask_user to stop waiting
	cancelCh chan struct{}
}

// AskChannel is a question/answer exchange mechanism between ask_user and TUI.
type AskChannel struct {
	Question chan string
	Answer   chan string
}

// NewAskUserTool creates a tool that prompts the user for input during agent execution.
func NewAskUserTool() *AskUserTool {
	return &AskUserTool{
		cancelCh: make(chan struct{}),
	}
}

// SetAskChannel sets the channel for TUI mode.
func (t *AskUserTool) SetAskChannel(ch *AskChannel) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.askChannel = ch
}

// SetAskFunc sets the callback for CLI mode.
func (t *AskUserTool) SetAskFunc(fn AskFunc) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.askFunc = fn
}

// Cancel stops any pending ask_user call.
func (t *AskUserTool) Cancel() {
	select {
	case t.cancelCh <- struct{}{}:
	default:
	}
}

func (t *AskUserTool) Name() string { return "ask_user" }

func (t *AskUserTool) Description() string {
	return i18n.T("tools.ask_user.description")
}

func (t *AskUserTool) Execute(params map[string]string) ToolResult {
	question, ok := params["question"]
	if !ok || question == "" {
		return Error("tools.ask_user.param_question")
	}

	// In non-interactive mode — skip
	if t.NonInteractive {
		return Success("tools.ask_user.skipped")
	}

	// TUI mode: send question via channel and wait for response
	t.mu.Lock()
	ch := t.askChannel
	fn := t.askFunc
	t.mu.Unlock()

	if ch != nil {
		// Non-blocking send of question with timeout
		select {
		case ch.Question <- question:
		case <-t.cancelCh:
			return Success("tools.ask_user.no_answer")
		case <-time.After(30 * time.Second):
			return Success("tools.ask_user.no_answer")
		}

		// Wait for answer with timeout and cancel support
		select {
		case answer := <-ch.Answer:
			if answer == "" {
				return Success("tools.ask_user.no_answer")
			}
			return Success("%s", answer)
		case <-t.cancelCh:
			return Success("tools.ask_user.no_answer")
		case <-time.After(10 * time.Minute):
			return Success("tools.ask_user.no_answer")
		}
	}

	// CLI mode: call callback (readline)
	if fn != nil {
		answer := fn(question)
		if answer == "" {
			return Success("tools.ask_user.no_answer")
		}
		return Success("%s", answer)
	}

	// No channel or callback — cannot request
	return Success("tools.ask_user.no_answer")
}

// ExecuteAsync implements AsyncTool for TUI mode.
func (t *AskUserTool) ExecuteAsync(params map[string]string) <-chan AsyncEvent {
	ch := make(chan AsyncEvent, 1)

	go func() {
		defer close(ch)
		result := t.Execute(params)
		if result.Error != "" {
			ch <- AsyncEvent{Type: "result", Error: result.Error, Done: true}
		} else {
			ch <- AsyncEvent{Type: "result", Output: result.Output, Done: true}
		}
	}()

	return ch
}

func (t *AskUserTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.ask_user.param_question_desc"),
			},
		},
		"required": []string{"question"},
	}
}
