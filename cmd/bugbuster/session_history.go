package main

import (
	"fmt"
	"strings"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"

	"charm.land/lipgloss/v2"
)

// isToolResultOnly checks if message contains only tool_result blocks
func isToolResultOnly(msg provider.Message) bool {
	if len(msg.Content) == 0 {
		return false
	}
	for _, block := range msg.Content {
		if block.Type != "tool_result" {
			return false
		}
	}
	return true
}

// maxHistoryMessages — maximum message count for rendering during session restore
const maxHistoryMessages = 30

// renderSessionHistory renders session messages into strings.Builder for TUI.
// Shows only last maxHistoryMessages messages to avoid overflowing viewport.
func renderSessionHistory(messages []provider.Message, output *strings.Builder, mdRenderer *GlamourRenderer) {
	total := len(messages)
	start := 0
	if total > maxHistoryMessages {
		start = total - maxHistoryMessages
		skipped := total - maxHistoryMessages
		output.WriteString(helpStyle.Render(fmt.Sprintf("  ↑ ... %d earlier messages skipped ...", skipped)) + "\n")
		output.WriteString(helpStyle.Render(fmt.Sprintf("  ↑ ... %d messages total, showing last %d ...", total, maxHistoryMessages)) + "\n\n")
	}

	for i := start; i < total; i++ {
		msg := messages[i]
		switch msg.Role {
		case "user":
			if isToolResultOnly(msg) {
				for _, block := range msg.Content {
					if block.Type == "tool_result" {
						output.WriteString(FormatToolCallEnd(block.ToolName, !block.IsError, block.Output, block.Output, 0, nil) + "\n")
					}
				}
				continue
			}
			text := msg.GetResponseText()
			if text != "" {
				output.WriteString(userMsgStyle.Render("  ❯ "+text) + "\n")
				output.WriteString(separatorStyle.Render("  ──────────────────────────────────────────────────") + "\n")
			}
		case "assistant":
			for _, block := range msg.Content {
				switch block.Type {
				case "thinking":
					if block.Text != "" {
						wrapped := wrapText(strings.TrimSpace(block.Text), 4, 80)
						output.WriteString(lipgloss.NewStyle().Foreground(appTheme.Thinking.LipglossColor()).Render("  ∴ "+i18n.T("cli.thinking")) + "\n")
						output.WriteString(lipgloss.NewStyle().Foreground(appTheme.Thinking.LipglossColor()).Render(wrapped) + "\n\n")
					}
				case "text":
					if block.Text != "" {
						mdRenderer.Render(block.Text + "\n")
						if rendered := mdRenderer.Flush(); rendered != "" {
							output.WriteString(rendered)
						}
					}
				case "tool_use":
					if flushed := mdRenderer.Flush(); flushed != "" {
						output.WriteString(flushed)
					}
					params := make(map[string]string)
					for k, v := range block.Input {
						params[k] = fmt.Sprintf("%v", v)
					}
					output.WriteString("\n" + FormatToolCallStart(block.ToolName, params) + "\n")
				}
			}
			if flushed := mdRenderer.Flush(); flushed != "" {
				output.WriteString(flushed)
			}
		}
	}
}

// renderSessionHistoryCLI renders session messages to stdout.
// Shows only last maxHistoryMessages messages to avoid overflowing terminal.
func renderSessionHistoryCLI(messages []provider.Message) {
	total := len(messages)
	start := 0
	if total > maxHistoryMessages {
		start = total - maxHistoryMessages
		// Show header about skipped messages
		skipped := total - maxHistoryMessages
		fmt.Printf("%s  ↑ ... %d earlier messages skipped ...%s\n", appTheme.Dim.ANSICode(), skipped, ansiReset)
		fmt.Printf("%s  ↑ ... %d messages total, showing last %d ...%s\n\n", appTheme.Dim.ANSICode(), total, maxHistoryMessages, ansiReset)
	}

	for i := start; i < total; i++ {
		msg := messages[i]
		switch msg.Role {
		case "user":
			if isToolResultOnly(msg) {
				renderToolResultsCLI(msg)
				continue
			}
			renderUserMessageCLI(msg)
		case "assistant":
			renderAssistantMessageCLI(msg)
		}
	}
}

// renderToolResultsCLI renders tool results
func renderToolResultsCLI(msg provider.Message) {
	for _, block := range msg.Content {
		if block.Type == "tool_result" {
			fmt.Println(FormatToolCallEnd(block.ToolName, !block.IsError, block.Output, block.Output, 0, nil))
		}
	}
}

func renderUserMessageCLI(msg provider.Message) {
	text := msg.GetResponseText()
	if text == "" {
		return
	}
	fmt.Printf("%s%s  ❯ %s%s\n", appTheme.Success.ANSICode(), ansiBold, text, ansiReset)
	fmt.Printf("%s  ──────────────────────────────────────────────────%s\n", appTheme.Dim.ANSICode(), ansiReset)
}

func renderAssistantMessageCLI(msg provider.Message) {
	mdRenderer := NewGlamourRenderer()
	for _, block := range msg.Content {
		switch block.Type {
		case "thinking":
			if block.Text != "" {
				wrapped := wrapTextCLI(strings.TrimSpace(block.Text), 4, 80)
				fmt.Printf("%s%s  ∴ %s%s\n", appTheme.Dim.ANSICode(), appTheme.Dim.ANSICode(), i18n.T("cli.thinking"), ansiReset)
				fmt.Printf("%s%s%s\n\n", appTheme.Dim.ANSICode(), wrapped, ansiReset)
			}
		case "text":
			if block.Text != "" {
				mdRenderer.Render(block.Text + "\n")
				rendered := mdRenderer.Flush()
				if rendered != "" {
					fmt.Print(rendered)
				}
			}
		case "tool_use":
			// Before tool_use, reset markdown buffer
			if flushed := mdRenderer.Flush(); flushed != "" {
				fmt.Print(flushed)
			}
			params := make(map[string]string)
			for k, v := range block.Input {
				params[k] = fmt.Sprintf("%v", v)
			}
			fmt.Println()
			fmt.Println(FormatToolCallStart(block.ToolName, params))
		}
	}
	// Reset remaining buffer
	if flushed := mdRenderer.Flush(); flushed != "" {
		fmt.Print(flushed)
	}
	fmt.Println()
}

// wrapTextCLI wraps text with indentation (ANSI version)
func wrapTextCLI(text string, indent int, maxCols int) string {
	if maxCols <= indent {
		maxCols = 80
	}
	maxContent := maxCols - indent
	indentStr := strings.Repeat(" ", indent)

	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if len(line) <= maxContent {
			lines = append(lines, indentStr+line)
		} else {
			// Split long lines
			for len(line) > maxContent {
				lines = append(lines, indentStr+line[:maxContent])
				line = line[maxContent:]
			}
			if line != "" {
				lines = append(lines, indentStr+line)
			}
		}
	}
	return strings.Join(lines, "\n")
}