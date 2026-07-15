package tools

import (
	"strings"

	"bugbuster-code/pkg/i18n"
)

// LearnTool is the learning tool (optional, Cavibora only)
type LearnTool struct {
	// TeachURL — URL for learning (Cavibora API)
	TeachURL string
	// APIKey — key for Cavibora API
	APIKey string
}

// NewLearnTool creates a tool for teaching the model new knowledge (placeholder implementation).
func NewLearnTool() *LearnTool {
	return &LearnTool{}
}

func (t *LearnTool) Name() string { return "learn" }

func (t *LearnTool) Description() string {
	return i18n.T("tools.learn.description")
}

func (t *LearnTool) Execute(params map[string]string) ToolResult {
	input, ok := params["input"]
	if !ok || input == "" {
		return Error("tools.learn.param_input")
	}

	output, ok := params["output"]
	if !ok || output == "" {
		return Error("tools.learn.param_output")
	}

	if t.TeachURL == "" {
		return Error("tools.learn.not_available")
	}

	learnType := params["type"]
	if learnType == "" {
		learnType = "text"
	}

	// LearnTool — optional learning tool (placeholder).
	// Real data submission to Cavibora /v1/teach is not implemented.
	// Tool returns "scheduled" confirmation without real HTTP request.
	switch strings.ToLower(learnType) {
	case "code":
		truncated := input
		if len(truncated) > 50 {
			truncated = truncated[:50]
		}
		return Success("tools.learn.code_scheduled", truncated, len(output))
	default:
		truncated := input
		if len(truncated) > 50 {
			truncated = truncated[:50]
		}
		return Success("tools.learn.scheduled", truncated, len(output))
	}
}

func (t *LearnTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.learn.param_input_desc"),
			},
			"output": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.learn.param_output_desc"),
			},
			"type": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.learn.param_type_desc"),
			},
		},
		"required": []string{"input", "output"},
	}
}
