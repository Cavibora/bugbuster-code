package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"bugbuster-code/pkg/agent"
	"bugbuster-code/pkg/config"
	"bugbuster-code/pkg/mcp"

	"charm.land/lipgloss/v2"
)

// TUI styles (initialized via initTUIStyles)
var (
	separatorStyle   lipgloss.Style
	userMsgStyle     lipgloss.Style
	assistantStyle    lipgloss.Style
	errorStyle       lipgloss.Style
	helpStyle        lipgloss.Style
	toolProgressStyle lipgloss.Style
	pasteBlockStyle  lipgloss.Style

	tuiSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
)

// initTUIStyles initializes styles from theme
func initTUIStyles() {
	separatorStyle = lipgloss.NewStyle().
		Foreground(appTheme.Separator.LipglossColor())

	userMsgStyle = lipgloss.NewStyle().
		Foreground(appTheme.UserMsg.LipglossColor()).
		Bold(true)

	assistantStyle = lipgloss.NewStyle().
		Foreground(appTheme.Assistant.LipglossColor())

	errorStyle = lipgloss.NewStyle().
		Foreground(appTheme.Error.LipglossColor())

	helpStyle = lipgloss.NewStyle().
		Foreground(appTheme.Dim.LipglossColor())

	toolProgressStyle = lipgloss.NewStyle().
		Foreground(appTheme.Warning.LipglossColor())

	pasteBlockStyle = lipgloss.NewStyle().
		Foreground(appTheme.Dim.LipglossColor()).
		Italic(true)
}

// printHelpString returns help line for TUI
func printHelpString() string {
	return `  Commands:
			    /help     — Show help
			    /exit     — Exit
			    /reset    — Reset context
			    /undo     — Undo last change
			    /undoall  — Undo all changes
			    /diff     — Show changes
			    /context  — Context info
			    /tools    — List tools
		    /mcp      — MCP servers and auto-scanning
		    /sessions — List sessions
			    /model    — Switch model
			    /provider — Switch provider
			    /task     — Switch provider by task type (code/analyze/debug...)
			    /rename   — Rename session
			    /lang     — Change language
			    /auto     — Autopilot: auto-continue after each response
			    /auto N   — Autopilot with N iterations limit
			    /cli      — Switch to CLI mode`
}

// mcpInfoString returns MCP server info for TUI
func mcpInfoString(cfg *config.BugBusterConfig) string {
	var sb strings.Builder

	sb.WriteString(lipgloss.NewStyle().Foreground(appTheme.Primary.LipglossColor()).Bold(true).Render("MCP Servers:"))
	sb.WriteString("\n")

	if len(cfg.MCP.Servers) == 0 {
		sb.WriteString("  No MCP servers configured\n")
	} else {
		for name, server := range cfg.MCP.Servers {
			status := "disabled"
			if server.Enabled {
				status = "enabled"
			}
			sb.WriteString(fmt.Sprintf("  %-20s %-15s %s\n", name, server.Type, status))
			if server.Command != "" {
				sb.WriteString(fmt.Sprintf("    command: %s %v\n", server.Command, server.Args))
			}
			if server.URL != "" {
				sb.WriteString(fmt.Sprintf("    url: %s\n", server.URL))
			}
		}
	}

	// Project auto-scanning
	projectDir := getProjectDir(cfg)
	sb.WriteString("\n" + lipgloss.NewStyle().Foreground(appTheme.Primary.LipglossColor()).Bold(true).Render("Auto-import scan:"))
	sb.WriteString("\n")
	configs, err := mcp.AutoImport(projectDir)
	if err != nil {
		sb.WriteString(lipgloss.NewStyle().Foreground(appTheme.Error.LipglossColor()).Render(fmt.Sprintf("  Error: %v", err)) + "\n")
	} else if len(configs) == 0 {
		sb.WriteString("  No MCP configurations found in project\n")
	} else {
		sb.WriteString(lipgloss.NewStyle().Foreground(appTheme.UserMsg.LipglossColor()).Render(fmt.Sprintf("  Found %d MCP server(s):", len(configs))) + "\n")
		for _, c := range configs {
			sb.WriteString(fmt.Sprintf("  %-20s %-15s %s\n", c.Name, c.Type, c.Command))
		}
	}

	return sb.String()
}

// sessionsInfoString returns session info for TUI
func sessionsInfoString(mgr *agent.SessionManager, current *agent.Session) string {
	var sb strings.Builder

	sb.WriteString(lipgloss.NewStyle().Foreground(appTheme.Primary.LipglossColor()).Bold(true).Render("Sessions:"))
	sb.WriteString("\n")

	if current != nil {
		sb.WriteString(lipgloss.NewStyle().Foreground(appTheme.UserMsg.LipglossColor()).Render(fmt.Sprintf("  Current: %s", current.ID)))
		sb.WriteString(fmt.Sprintf(" (%d messages)\n", len(current.Messages)))
	}

	if mgr != nil {
		sessions, err := mgr.ListSessions()
		if err != nil {
			sb.WriteString(lipgloss.NewStyle().Foreground(appTheme.Error.LipglossColor()).Render(fmt.Sprintf("  Error: %v", err)) + "\n")
		} else if len(sessions) == 0 {
			sb.WriteString("  No saved sessions\n")
		} else {
			sb.WriteString(fmt.Sprintf("  Available: %d session(s)\n", len(sessions)))
			maxShow := 5
			if len(sessions) < maxShow {
				maxShow = len(sessions)
			}
			for _, s := range sessions[:maxShow] {
				sb.WriteString(fmt.Sprintf("    %s (%d msg, %s)\n", s.ID, len(s.Messages), s.UpdatedAt.Format("2006-01-02 15:04")))
			}
		}
	}

	sb.WriteString("\n" + lipgloss.NewStyle().Foreground(appTheme.Dim.LipglossColor()).Render("  Restore: bugbuster --session <id>"))

	return sb.String()
}

// saveSessionTUI saves session and change tracker
func saveSessionTUI(m TUI) {
	if m.session != nil && m.sessionMgr != nil {
		m.session.Messages = m.loop.Context.GetMessages()
		m.session.InputHistory = m.history
		if err := m.sessionMgr.SaveSessionMessages(m.session); err != nil {
			// Only log errors, don't spam on successful incremental saves
		}
	}
	if m.changeTracker != nil {
		changesFile := filepath.Join(getProjectDir(m.cfg), ".bugbuster", "changes", m.loop.Context.SessionID+".json")
		m.changeTracker.SaveToFile(changesFile)
	}
}