package tools

import (
	"fmt"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

// AskTool is a tool that requests an external LLM
type AskTool struct {
	Provider provider.Provider
	Timeout  time.Duration
}

// NewAskTool creates a tool that asks the user a question and returns their response.
func NewAskTool() *AskTool {
	return &AskTool{
		Timeout: 60 * time.Second,
	}
}

func (t *AskTool) Name() string { return "ask" }

func (t *AskTool) Description() string {
	return i18n.T("tools.ask.description")
}

func (t *AskTool) Execute(params map[string]string) ToolResult {
	prompt, ok := params["prompt"]
	if !ok || prompt == "" {
		return Error("tools.ask.param_prompt")
	}

	if t.Provider == nil {
		return Error("errors.not_connected")
	}

	messages := []provider.Message{
		provider.SystemMsg(i18n.T("tools.ask.system_prompt")),
		provider.UserMsg(prompt),
	}

	result, err := t.Provider.Complete(messages, nil)
	if err != nil {
		return Error("tools.ask.request_error", err)
	}

	text := result.Message.GetText()
	if text == "" {
		return Error("errors.empty_response")
	}

	// Limit output
	maxLen := 5000
	if len(text) > maxLen {
		text = text[:maxLen] + fmt.Sprintf(i18n.T("tools.ask.truncated"), len(text))
	}

	return Success("%s", text)
}

func (t *AskTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.ask.param_prompt_desc"),
			},
		},
		"required": []string{"prompt"},
	}
}
