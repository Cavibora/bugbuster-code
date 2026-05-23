package provider

import (
	"fmt"
	"strings"
)

// ContentBlock is a structured content block in message
type ContentBlock struct {
	Type      string         // "text", "thinking", "tool_use", "tool_result"
	Text      string         // for type="text" and type="thinking"
	ToolUseID string         // for tool_use and tool_result
	ToolName  string         // for tool_use
	Input     map[string]any // parameters tool_use
	Output    string         // result tool_result
	IsError   bool           // for tool_result
}

// Message — message in conversation context
type Message struct {
	Role    string         // "system", "user", "assistant"
	Content []ContentBlock // structured content
	Text    string         // fallback: plain text (if Content is empty)
}

// SystemMsg creates a system message
func SystemMsg(text string) Message {
	return Message{
		Role: "system",
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

// UserMsg creates a user message
func UserMsg(text string) Message {
	return Message{
		Role: "user",
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

// AssistantText creates assistant response with text
func AssistantText(text string) Message {
	return Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

// AssistantToolCalls creates assistant response with tool calls
func AssistantToolCalls(blocks []ContentBlock) Message {
	return Message{
		Role:    "assistant",
		Content: blocks,
	}
}

// ToolResultMsg creates message with tool result
func ToolResultMsg(toolUseID, toolName, output string, isError bool) Message {
	return Message{
		Role: "user", // Anthropic requires role=user for tool_result
		Content: []ContentBlock{
			{
				Type:      "tool_result",
				ToolUseID: toolUseID,
				ToolName:  toolName,
				Output:    output,
				IsError:   isError,
			},
		},
	}
}

// GetText returns text from messages (from Content or Text), including thinking-blocks and tool_result
func (m Message) GetText() string {
	if len(m.Content) > 0 {
		var parts []string
		for _, block := range m.Content {
			switch block.Type {
			case "text", "thinking":
				parts = append(parts, block.Text)
			case "tool_result":
				prefix := "Result"
				if block.IsError {
					prefix = "Error"
				}
				if block.ToolName != "" {
					parts = append(parts, fmt.Sprintf("[%s of %s]\n%s", prefix, block.ToolName, block.Output))
				} else {
					parts = append(parts, fmt.Sprintf("[%s]\n%s", prefix, block.Output))
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return m.Text
}

// GetThinking returns thinking content from messages (model reasoning)
func (m Message) GetThinking() string {
	if len(m.Content) == 0 {
		return ""
	}
	var parts []string
	for _, block := range m.Content {
		if block.Type == "thinking" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// GetResponseText returns only text response (without thinking)
func (m Message) GetResponseText() string {
	if len(m.Content) > 0 {
		var parts []string
		for _, block := range m.Content {
			if block.Type == "text" {
				parts = append(parts, block.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return m.Text
}

// GetToolCalls extracts all tool calls from messages
func (m Message) GetToolCalls() []ContentBlock {
	var calls []ContentBlock
	for _, block := range m.Content {
		if block.Type == "tool_use" {
			calls = append(calls, block)
		}
	}
	return calls
}

// HasToolCalls checks if there are tool calls in message
func (m Message) HasToolCalls() bool {
	for _, block := range m.Content {
		if block.Type == "tool_use" {
			return true
		}
	}
	return false
}

// ToPlainText converts message to plain text (for XML parsing)
func (m Message) ToPlainText() string {
	text := m.GetText()
	if text != "" {
		return text
	}
	return m.Text
}

// MessagesToText converts message list to text prompt
func MessagesToText(messages []Message) string {
	var parts []string
	for _, m := range messages {
		role := m.Role
		text := m.GetText()
		if text == "" {
			text = m.Text
		}
		if text != "" {
			parts = append(parts, "["+role+"]\n"+text)
		}
	}
	return strings.Join(parts, "\n\n")
}
