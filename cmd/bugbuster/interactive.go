package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"bugbuster-code/pkg/agent"
	"bugbuster-code/pkg/config"
	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/logger"
	"bugbuster-code/pkg/mcp"
	"bugbuster-code/pkg/plugin"
	"bugbuster-code/pkg/provider"
	"bugbuster-code/pkg/theme"

	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// cmdOutput is the writer for command output. When readline is active,
// this is set to rl.Stdout() so readline can manage cursor position.
// Otherwise it defaults to os.Stdout.
var cmdOutput io.Writer = os.Stdout

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// isYesAnswer checks if the answer is a "yes" in any supported language.
// Supports: en (y/yes), ru (д/да), de (j/ja), es (s/sí), fr (o/oui), pt (s/sim), ja (はい/hai), zh (是/shi)
func isYesAnswer(answer string) bool {
	answer = strings.ToLower(answer)
	switch answer {
	case "y", "yes", // English
		"д", "да", // Russian
		"j", "ja", // German
		"s", "sí", // Spanish
		"o", "oui", // French
		"sim", // Portuguese
		"はい", "hai", // Japanese
		"是", "shi": // Chinese
		return true
	default:
		return false
	}
}

// runInteractive — interactive mode (split-terminal)
func runInteractive(cmd *cobra.Command, args []string) {
	cfg := loadConfig()

	// Initialize i18n
	lang := langFlag
	if lang == "" {
		lang = cfg.Agent.Language
	}
	if lang == "" {
		lang = "en"
	}
	if err := i18n.Init(lang); err != nil {
		i18n.Init("en")
	}

	// Initialize theme
	appTheme = theme.ResolveTheme(cfg.Theme)

	// If request is provided — execute and exit
	if len(args) > 0 {
		query := strings.Join(args, " ")
		runQuery(cfg, query)
		return
	}

	// If not terminal — simple non-interactive mode
	if !isTerminal() {
		fmt.Fprintln(os.Stderr, "Error: interactive mode requires a terminal")
		os.Exit(1)
	}

	// Create provider
	p, err := createProvider(cfg)
	if err != nil {
		color.Red("%s", i18n.T("cli_error.provider_create", err))
		color.Yellow("%s", i18n.T("cli_success.config_init_hint"))
		os.Exit(1)
	}

	// Change tracker for undo (create BEFORE agent so hooks work)
	changeTracker := NewChangeTracker()
	changesFile := filepath.Join(getProjectDir(cfg), ".bugbuster", "changes", sessionID+".json")
	changeTracker.LoadFromFile(changesFile)

	// Create agent with connected tracker
	loop := createAgentLoop(cfg, p, changeTracker, sessionID)

	// If --tui flag is specified — start TUI mode
	if tuiMode != "" {
		// TUI↔CLI switching loop
		startTUI := true
		for startTUI {
			startTUI = false
			initTUIStyles()
			switchToCLI := runTUI(cfg, loop, changeTracker, p.Name(), tuiMode)
			if switchToCLI {
				sessionID = loop.Context.SessionID
				func() {
					defer func() {
						if r := recover(); r != nil {
							fmt.Fprintf(os.Stderr, "Recovered from panic during TUI→CLI switch: %v\n", r)
							fmt.Fprintf(os.Stderr, "Run 'stty sane' to restore your terminal.\n")
						}
					}()
					st := NewSplitTerminal(cfg, loop, changeTracker, p.Name())
					if st.Run() {
						// User wants TUI again
						startTUI = true
					}
				}()
			}
		}
		return
	}

	// Determine UI mode: from environment variable or config
	// Default is CLI. TUI starts only with explicit specification.
	uiMode := os.Getenv("BUGBUSTER_UI_MODE")
	if uiMode == "" {
		uiMode = cfg.UI
	}

	switch uiMode {
	case "tui", "inline":
		// TUI↔CLI switching loop
		startTUI := true
		for startTUI {
			startTUI = false
			initTUIStyles()
			switchToCLI := runTUI(cfg, loop, changeTracker, p.Name(), uiMode)
			if switchToCLI {
				sessionID = loop.Context.SessionID
				func() {
					defer func() {
						if r := recover(); r != nil {
							fmt.Fprintf(os.Stderr, "Recovered from panic during TUI→CLI switch: %v\n", r)
							fmt.Fprintf(os.Stderr, "Run 'stty sane' to restore your terminal.\n")
						}
					}()
					st := NewSplitTerminal(cfg, loop, changeTracker, p.Name())
					if st.Run() {
						startTUI = true
					}
				}()
			}
		}
	default:
		// CLI (split-terminal) mode — default
		// CLI↔TUI switching loop: each mode can switch to another
		startCLI := true
		for startCLI {
			startCLI = false
			st := NewSplitTerminal(cfg, loop, changeTracker, p.Name())
			switchToTUI := st.Run()
			if switchToTUI {
				sessionID = loop.Context.SessionID
				initTUIStyles()
				switchBackToCLI := runTUI(cfg, loop, changeTracker, p.Name(), "auto")
				if switchBackToCLI {
					sessionID = loop.Context.SessionID
					func() {
						defer func() {
							if r := recover(); r != nil {
								fmt.Fprintf(os.Stderr, "Recovered from panic during TUI→CLI switch: %v\n", r)
								fmt.Fprintf(os.Stderr, "Run 'stty sane' to restore your terminal.\n")
							}
						}()
						startCLI = true // continue loop — return to CLI
					}()
				}
			}
		}
	}
}

// handleCommand handles slash commands. Returns true if command is recognized.
// When readline is active (rl != nil), output is written through rl.Stdout()
// so readline can properly manage cursor position and refresh the prompt (❯).
// When rl is nil (readline closed for command output), output goes to os.Stdout.
func handleCommand(input string, loop *agent.AgentLoop, cfg *config.BugBusterConfig, p provider.Provider, ct *ChangeTracker, rl *readline.Instance, sessionMgr *agent.SessionManager, currentSession *agent.Session) bool {
	switch {
	case input == "/exit", input == "/quit":
		// exit is handled in calling code — return false to exit main
		return false
	case input == "/sessions":
		printSessions(sessionMgr, currentSession)
		return true
	case input == "/skills":
		printSkills(loop)
		return true
	case strings.HasPrefix(input, "/skill "):
		name := strings.TrimSpace(strings.TrimPrefix(input, "/skill"))
		activateSkill(loop, name)
		return true
	case strings.HasPrefix(input, "/rename "):
		name := strings.TrimSpace(strings.TrimPrefix(input, "/rename"))
		if name == "" {
			color.Red("Usage: /rename <name>")
			return true
		}
		if currentSession == nil {
			color.Red("No active session")
			return true
		}
		if err := sessionMgr.RenameSession(currentSession.ID, name); err != nil {
			color.Red("Error renaming session: %v", err)
		} else {
			currentSession.Name = name
			color.Green("Session renamed to: %s", name)
		}
		return true
	case input == "/help":
		printHelp()
		return true
	case input == "/reset":
		loop.Context.Reset()
		if loop.LoopDetector != nil {
			loop.LoopDetector.Reset()
		}
		color.Yellow("%s", i18n.T("cli.context_reset"))
		return true
	case input == "/context":
		tokensUsed := loop.Context.TokenCount()
		maxTokens := loop.Context.MaxTokens
		msgCount := len(loop.Context.GetMessages())
		fmt.Fprintln(cmdOutput, FormatContextInfo(msgCount, tokensUsed, maxTokens))
		return true
	case input == "/compact":
		tokensBefore := loop.Context.TokenCount()
		maxTokens := loop.Context.MaxTokens
		if tokensBefore <= maxTokens {
			color.Green("%s (%d/%d)", i18n.T("cli.compaction_not_needed"), tokensBefore, maxTokens)
		} else {
			color.Yellow("%s (%d/%d)...", i18n.T("cli.compacting"), tokensBefore, maxTokens)
			loop.Context.Compact()
			tokensAfter := loop.Context.TokenCount()
			saved := tokensBefore - tokensAfter
			color.Green("%s %d → %d (%s: %d)", i18n.T("cli.compaction_done"), tokensBefore, tokensAfter, i18n.T("cli.compaction_saved"), saved)
		}
		return true
	case input == "/tools":
		printTools(loop)
		return true
	case input == "/mcp":
		printMCPServers(cfg)
		return true
	case input == "/dream", strings.HasPrefix(input, "/dream "):
		seed := strings.TrimPrefix(input, "/dream ")
		seed = strings.TrimSpace(seed)
		handleDreamCommand(loop, seed)
		return true
	case input == "/emotions":
		handleEmotionsCommand(loop)
		return true
	case input == "/mesh-stats":
		handleMeshStatsCommand(loop)
		return true
	case input == "/undo":
		undoResult, err := ct.Undo()
		if err != nil {
			color.Red("%v", err)
		} else {
			color.Green("%s", i18n.T("cli.undo_success", undoResult))
		}
		return true
	case input == "/undoall":
		count := ct.Count()
		if count == 0 {
			color.Yellow("%s", i18n.T("cli.undo_none"))
		} else {
			fmt.Fprint(cmdOutput, i18n.T("cli.undo_all_confirm", count)+" ")
			var answer string
			if rl != nil {
				a, _ := rl.Readline()
				answer = a
			}
			answer = strings.TrimSpace(strings.ToLower(answer))
			if isYesAnswer(answer) {
				n, err := ct.UndoAll()
				if err != nil {
					color.Red("%v", err)
				}
				color.Green("%s", i18n.T("cli.undo_all_done", n))
			}
		}
		return true
	case input == "/diff":
		changes := ct.Diff()
		fmt.Fprintln(cmdOutput, FormatDiff(changes))
		return true
	case strings.HasPrefix(input, "/model "):
		newModel := strings.TrimPrefix(input, "/model ")
		switchModel(loop, cfg, newModel)
		return true
	case strings.HasPrefix(input, "/provider "):
		newProvider := strings.TrimPrefix(input, "/provider ")
		switchProvider(loop, cfg, newProvider)
		return true
	case strings.HasPrefix(input, "/lang "):
		newLang := strings.TrimPrefix(input, "/lang ")
		if i18n.HasLanguage(newLang) {
			i18n.SetLanguage(newLang)
			color.Green("Language: %s (%s)", newLang, i18n.LanguageName(newLang))
		} else {
			available := i18n.AvailableLanguages()
			color.Red("Language '%s' not available. Available: %v", newLang, available)
		}
		return true
	case input == "/debug":
		debug = !debug
		if debug {
			logger.Init("debug", false, "")
			color.Yellow("Debug logging: ON")
		} else {
			logger.Init("info", false, "")
			color.Yellow("Debug logging: OFF")
		}
		return true
	case input == "/tui":
		// Do not handle here — handled in main Run() loop
		return false
	case input == "/cli":
		color.Yellow("%s", i18n.T("cli.already_in_cli"))
		return true
	case input == "/auto", strings.HasPrefix(input, "/auto "):
		// /auto is handled in SplitTerminal.Run() and TUI — return false to pass through
		return false
	case input == "/plugin", input == "/plugins":
		printPlugins(cfg)
		return true
	case strings.HasPrefix(input, "/plugin install "):
		pluginName := strings.TrimPrefix(input, "/plugin install ")
		installPlugin(cfg, pluginName)
		return true
	case strings.HasPrefix(input, "/plugin remove "):
		pluginName := strings.TrimPrefix(input, "/plugin remove ")
		removePlugin(cfg, pluginName)
		return true
	default:
		if strings.HasPrefix(input, "/") {
			color.Red("%s", i18n.T("cli.unknown_command", input))
			return true
		}
		return false
	}
}

// runQuery — one-time request (without interactive mode)
func runQuery(cfg *config.BugBusterConfig, query string) {
	p, err := createProvider(cfg)
	if err != nil {
		color.Red("%s", i18n.T("cli_error.general", err))
		os.Exit(1)
	}

	loop := createAgentLoop(cfg, p, nil, "")
	result, err := loop.Run(query)
	if err != nil {
		color.Red("%s", i18n.T("cli_error.general", err))
		os.Exit(1)
	}
	fmt.Fprintln(cmdOutput, result)
}

// printBanner prints banner
func printBanner(cfg *config.BugBusterConfig, p provider.Provider) {
	color.Cyan("╔══════════════════════════════════════════╗")
	color.Cyan("%s", i18n.T("cli_banner.title"))
	provCfg := cfg.Providers[cfg.DefaultProvider]
	providerInfo := p.Name()
	if provCfg.Model != "" {
		providerInfo = fmt.Sprintf("%s · %s", p.Name(), provCfg.Model)
	}
	color.Cyan("║  %s", providerInfo)
	color.Cyan("╚══════════════════════════════════════════╝")
	color.Yellow("%s", i18n.T("cli.prompt"))
}

// restoreOrNewSession restores or creates a new session
func restoreOrNewSession(sessionMgr *agent.SessionManager, rl *readline.Instance, loop *agent.AgentLoop, cfg *config.BugBusterConfig) *agent.Session {
	var currentSession *agent.Session
	if sessionID != "" {
		loaded, err := sessionMgr.LoadSession(sessionID)
		if err != nil {
			color.Red("%s", i18n.T("cli_error.session_load", sessionID, err))
			currentSession = sessionMgr.NewSession()
		} else {
			currentSession = loaded
			restoreSessionMessages(loop, currentSession.Messages)
			color.Green("%s", i18n.T("cli_success.session_restored", sessionID, len(currentSession.Messages)))
			// Restore chat on screen
			if len(currentSession.Messages) > 0 {
				renderSessionHistoryCLI(currentSession.Messages)
			}
		}
	} else {
		currentSession = restoreOrCreateSession(sessionMgr, rl)
		if currentSession.Messages != nil {
			restoreSessionMessages(loop, currentSession.Messages)
			// Restore chat on screen, if session is loaded with messages
			if len(currentSession.Messages) > 0 {
				renderSessionHistoryCLI(currentSession.Messages)
			}
		}
	}
	// Set session name if provided via --session-name flag
	if sessionName != "" && currentSession != nil {
		currentSession.Name = sessionName
		sessionMgr.RenameSession(currentSession.ID, sessionName)
	}
	return currentSession
}

// restoreSessionMessages loads session messages into agent context,
// preserving current system prompt and preventing immediate compaction.
func restoreSessionMessages(loop *agent.AgentLoop, messages []provider.Message) {
	// Save current system prompt (with tool descriptions etc.)
	currentSystemPrompt := loop.Context.GetSystemPrompt()

	// Preliminary cleanup: delete tool errors and duplicates
	messages = agent.RemoveToolErrors(messages)
	messages = agent.RemoveDuplicates(messages)

	// Remove old system messages from loaded session
	var filtered []provider.Message
	for _, m := range messages {
		if m.Role != "system" {
			filtered = append(filtered, m)
		}
	}

	// Clear context and add system prompt + session messages
	loop.Context.Reset()
	if currentSystemPrompt != "" {
		loop.Context.Add(provider.SystemMsg(currentSystemPrompt))
	}
	for _, msg := range filtered {
		loop.Context.Add(msg)
	}

	// Debug output: show context state after restore
	tokenCount := loop.Context.TokenCount()
	msgCount := len(loop.Context.GetMessages())
	maxTokens := loop.Context.MaxTokens
	color.HiBlack("  [session] %d messages, ~%d tokens (max: %d)", msgCount, tokenCount, maxTokens)

	// If context exceeds limit — do compaction with increased KeepRecent,
	// to preserve more recent messages
	if tokenCount > maxTokens && maxTokens > 0 {
		// Increase KeepRecent for compaction during restore
		origKeepRecent := loop.Context.KeepRecent
		loop.Context.KeepRecent = origKeepRecent * 4 // save 4x more messages
		if loop.Context.KeepRecent < 20 {
			loop.Context.KeepRecent = 20 // minimum 20 recent messages
		}
		color.HiBlack("  [session] Compacting: keepRecent=%d (was %d)", loop.Context.KeepRecent, origKeepRecent)
		loop.Context.Compact()
		loop.Context.KeepRecent = origKeepRecent // restore original value

		newTokenCount := loop.Context.TokenCount()
		newMsgCount := len(loop.Context.GetMessages())
		color.HiBlack("  [session] After compaction: %d messages, ~%d tokens", newMsgCount, newTokenCount)
	}
}

// saveSessionAndExit saves session, change tracker and exits
func saveSessionAndExit(session *agent.Session, loop *agent.AgentLoop, sessionMgr *agent.SessionManager, ct *ChangeTracker, changesFile string) {
	// Save session
	session.Messages = loop.Context.GetMessages()
	if err := sessionMgr.SaveSessionMessages(session); err != nil {
		color.Red("%s", i18n.T("cli_error.session_save", err))
	} else {
		color.Green("%s", i18n.T("cli_success.session_saved", session.ID))
		color.Cyan("  Restore: bugbuster --session %s", session.ID)
	}
	// Save change tracker
	if ct != nil && changesFile != "" {
		ct.SaveToFile(changesFile)
	}
	color.Cyan("%s", i18n.T("cli.goodbye"))
}

// restoreOrCreateSession offers to restore last session or create a new one
func restoreOrCreateSession(sessionMgr *agent.SessionManager, rl *readline.Instance) *agent.Session {
	sessions, err := sessionMgr.ListSessions()
	if err != nil || len(sessions) == 0 {
		return sessionMgr.NewSession()
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	if len(sessions) == 1 {
		s := sessions[0]
		fmt.Fprint(cmdOutput, color.HiCyanString("%s", i18n.T("cli_session.restore_prompt", s.ID, s.UpdatedAt.Format("2006-01-02 15:04"), len(s.Messages))) + " ")
		answer, _ := rl.Readline()
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer == "" || isYesAnswer(answer) {
			return s
		}
		color.Yellow("%s", i18n.T("cli_session.new_session"))
		return sessionMgr.NewSession()
	}

	color.Cyan("%s", i18n.T("cli_session.restore_list", len(sessions)))
	fmt.Fprintln(cmdOutput, i18n.T("cli_session.restore_new"))
	maxShow := 5
	if len(sessions) < maxShow {
		maxShow = len(sessions)
	}
	for i, s := range sessions[:maxShow] {
		fmt.Fprintln(cmdOutput, i18n.T("cli_session.restore_entry", i+1, s.ID, s.UpdatedAt.Format("2006-01-02 15:04"), len(s.Messages)))
	}

	fmt.Fprint(cmdOutput, i18n.T("cli_session.restore_choice"))
	answer, _ := rl.Readline()
	answer = strings.TrimSpace(answer)
	choice := 0
	fmt.Sscanf(answer, "%d", &choice)

	if choice >= 1 && choice <= maxShow {
		return sessions[choice-1]
	}

	color.Yellow("%s", i18n.T("cli_session.new_session"))
	return sessionMgr.NewSession()
}

// printMCPServers shows MCP server info and auto-scanning
func printMCPServers(cfg *config.BugBusterConfig) {
	color.Cyan("MCP Servers:")

	if len(cfg.MCP.Servers) == 0 {
		color.Yellow("  No MCP servers configured")
	} else {
		for name, server := range cfg.MCP.Servers {
			status := "disabled"
			if server.Enabled {
				status = "enabled"
			}
			fmt.Fprintf(cmdOutput, "  %-20s %-15s %s\n", name, server.Type, status)
			if server.Command != "" {
				fmt.Fprintf(cmdOutput, "    command: %s %v\n", server.Command, server.Args)
			}
			if server.URL != "" {
				fmt.Fprintf(cmdOutput, "    url: %s\n", server.URL)
			}
		}
	}

	// Project auto-scanning
	projectDir := getProjectDir(cfg)
	color.Cyan("\nAuto-import scan:")
	configs, err := mcp.AutoImport(projectDir)
	if err != nil {
		color.Red("  Error: %v", err)
	} else if len(configs) == 0 {
		color.Yellow("  No MCP configurations found in project")
	} else {
		color.Green("  Found %d MCP server(s):", len(configs))
		for _, c := range configs {
			fmt.Fprintf(cmdOutput, "  %-20s %-15s %s\n", c.Name, c.Type, c.Command)
		}
	}
}

// printSessions shows session list
func printSessions(sessionMgr *agent.SessionManager, currentSession *agent.Session) {
	color.Cyan("Sessions:")

	if currentSession != nil {
		name := currentSession.Name
		if name == "" {
			name = "(unnamed)"
		}
		color.Green("  Current: %s [%s] (%d messages)", currentSession.ID, name, len(currentSession.Messages))
	}

	sessions, err := sessionMgr.ListSessions()
	if err != nil {
		color.Red("  Error: %v", err)
		return
	}
	if len(sessions) == 0 {
		color.Yellow("  No saved sessions")
	} else {
		color.Yellow("  Available: %d session(s)", len(sessions))
		maxShow := 10
		if len(sessions) < maxShow {
			maxShow = len(sessions)
		}
		for i, s := range sessions[:maxShow] {
			name := s.Name
			if name == "" {
				name = "(unnamed)"
			}
			fmt.Fprintf(cmdOutput, "  %d. %s [%s] (%d msg, %s)\n", i+1, s.ID, name, len(s.Messages), s.UpdatedAt.Format("2006-01-02 15:04"))
		}
	}
	color.Cyan("  Restore: bugbuster --session <id>")
	color.Cyan("  Rename:  /rename <name>")
}


// printPlugins shows plugin list (built-in, Go, MCP)
func printPlugins(cfg *config.BugBusterConfig) {
	color.Cyan("Plugins:")

	// Built-in plugins
	builtins := cfg.Plugins.Builtins
	if len(builtins) == 0 {
		builtins = cfg.Plugins.Enabled // backward compatibility
	}
	if len(builtins) > 0 {
		color.Cyan("  Built-in:")
		for _, name := range builtins {
			fmt.Fprintf(cmdOutput, "    %-20s [builtin]\n", name)
		}
	}

	// Go-plugins (.so)
	if len(cfg.Plugins.GoPlugins) > 0 {
		color.Cyan("  Go plugins:")
		for _, gp := range cfg.Plugins.GoPlugins {
			fmt.Fprintf(cmdOutput, "    %-20s %s\n", gp.Name, gp.Path)
		}
	}

	// MCP servers (from plugins.mcp and mcp.servers)
	mcpServers := cfg.MCP.Servers
	if cfg.Plugins.MCP != nil {
		if mcpServers == nil {
			mcpServers = make(map[string]config.MCPServerConfig)
		}
		for name, srv := range cfg.Plugins.MCP {
			mcpServers[name] = srv
		}
	}
	if len(mcpServers) > 0 {
		color.Cyan("  MCP servers:")
		for name, srv := range mcpServers {
			status := "disabled"
			if srv.Enabled {
				status = "enabled"
			}
			fmt.Fprintf(cmdOutput, "    %-20s %-10s %s\n", name, srv.Type, status)
		}
	}

	// Available plugins (can be installed)
	color.Cyan("\nAvailable plugins (/plugin install <name>):")
	for _, p := range plugin.ListKnownPlugins() {
		fmt.Fprintf(cmdOutput, "    %-20s %s\n", p.Name, p.Description)
	}
}

// installPlugin sets MCP plugin from registry
func installPlugin(cfg *config.BugBusterConfig, name string) {
	p, ok := plugin.GetKnownPlugin(name)
	if !ok {
		color.Red("Plugin '%s' not found in registry", name)
		color.Yellow("Use /plugin to see available plugins")
		return
	}

	if p.Type != "mcp" {
		color.Red("Only MCP plugins can be installed via /plugin install")
		return
	}

	// Add to configuration
	if cfg.Plugins.MCP == nil {
		cfg.Plugins.MCP = make(map[string]config.MCPServerConfig)
	}
	cfg.Plugins.MCP[name] = p.Config

	color.Green("Plugin '%s' added to configuration", name)
	color.Yellow("Restart BugBuster to apply changes")

	// Dependency installation hint
	if p.InstallCmd != "" {
		color.Cyan("Install dependencies: %s", p.InstallCmd)
	}
}

// removePlugin deletes plugin from configuration
func removePlugin(cfg *config.BugBusterConfig, name string) {
	removed := false

	// Remove from MCP
	if cfg.Plugins.MCP != nil {
		if _, ok := cfg.Plugins.MCP[name]; ok {
			delete(cfg.Plugins.MCP, name)
			removed = true
		}
	}

	// Remove from MCP.Servers
	if _, ok := cfg.MCP.Servers[name]; ok {
		delete(cfg.MCP.Servers, name)
		removed = true
	}

	// Remove from builtins/enabled
	for i, n := range cfg.Plugins.Builtins {
		if n == name {
			cfg.Plugins.Builtins = append(cfg.Plugins.Builtins[:i], cfg.Plugins.Builtins[i+1:]...)
			removed = true
			break
		}
	}
	for i, n := range cfg.Plugins.Enabled {
		if n == name {
			cfg.Plugins.Enabled = append(cfg.Plugins.Enabled[:i], cfg.Plugins.Enabled[i+1:]...)
			removed = true
			break
		}
	}

	// Remove from Go-plugins
	for i, gp := range cfg.Plugins.GoPlugins {
		if gp.Name == name {
			cfg.Plugins.GoPlugins = append(cfg.Plugins.GoPlugins[:i], cfg.Plugins.GoPlugins[i+1:]...)
			removed = true
			break
		}
	}

	if removed {
		color.Green("Plugin '%s' removed from configuration", name)
		color.Yellow("Restart BugBuster to apply changes")
	} else {
		color.Red("Plugin '%s' not found in configuration", name)
	}
}

// handleDreamCommand handles /dream command (Cavibora-specific)
func handleDreamCommand(loop *agent.AgentLoop, seed string) {
	cavibora, ok := loop.GetProvider().(*provider.CaviboraProvider)
	if !ok {
		color.Red("%s", i18n.T("cli.cavibora_only"))
		return
	}

	color.Cyan("🌙 %s...", i18n.T("cli.cmd_dream"))
	result, err := cavibora.Dream(context.Background(), seed)
	if err != nil {
		color.Red("%s", i18n.T("cli.dream_error", err.Error()))
		return
	}

	color.Green("🌙 %s", i18n.T("cli.dream_result", result.Seed, len(result.Thoughts), result.NewBindings, result.Duration))
	if len(result.Thoughts) > 0 {
		color.Yellow("  💭 Thoughts:")
		for i, thought := range result.Thoughts {
			fmt.Fprintf(cmdOutput, "    %d. %s\n", i+1, thought)
		}
	}
}

// handleEmotionsCommand handles /emotions command (Cavibora-specific)
func handleEmotionsCommand(loop *agent.AgentLoop) {
	cavibora, ok := loop.GetProvider().(*provider.CaviboraProvider)
	if !ok {
		color.Red("%s", i18n.T("cli.cavibora_only"))
		return
	}

	result, err := cavibora.Emotions(context.Background())
	if err != nil {
		color.Red("%s", i18n.T("cli.emotions_error", err.Error()))
		return
	}

	fmt.Fprintf(cmdOutput, "💭 %s %s %s\n  %s\n", result.Emoji, result.Emotion, result.Bar, result.Detail)
}

// handleMeshStatsCommand handles /mesh-stats command (Cavibora-specific)
func handleMeshStatsCommand(loop *agent.AgentLoop) {
	cavibora, ok := loop.GetProvider().(*provider.CaviboraProvider)
	if !ok {
		color.Red("%s", i18n.T("cli.cavibora_only"))
		return
	}

	result, err := cavibora.MeshStats(context.Background())
	if err != nil {
		color.Red("%s", i18n.T("cli.mesh_stats_error", err.Error()))
		return
	}

	color.Green("🧠 %s", i18n.T("cli.mesh_stats_result",
		result.Cells, result.Bindings, result.Learnings,
		result.ModelName, result.Version, result.Uptime, result.Temperature))
}

func printSkills(loop *agent.AgentLoop) {
	if loop.SkillManager == nil {
		color.Yellow("Skills not available")
		return
	}
	skills := loop.SkillManager.List()
	if len(skills) == 0 {
		color.Yellow("No skills available")
		return
	}
	color.Cyan("Available skills:")
	fmt.Fprintln(cmdOutput, )
	for _, s := range skills {
		sourceTag := ""
		switch s.Source {
		case "builtin":
			sourceTag = color.HiBlackString("[builtin]")
		case "project":
			sourceTag = color.GreenString("[project]")
		case "global":
			sourceTag = color.CyanString("[global]")
		}
		fmt.Fprintf(cmdOutput, "  %-12s %s %s\n", color.HiWhiteString(s.Name), sourceTag, color.HiBlackString(s.Description))
	}
	fmt.Fprintln(cmdOutput, )
	color.HiBlack("Activate: /skill <name>")
}

func activateSkill(loop *agent.AgentLoop, name string) {
	if loop.SkillManager == nil {
		color.Red("Skills not available")
		return
	}
	content, err := loop.SkillManager.Activate(name)
	if err != nil {
		color.Red("%v", err)
		return
	}
	// Inject skill into system prompt
	msgs := loop.Context.GetMessages()
	for i, msg := range msgs {
		if msg.Role == "system" {
			updated := msg.GetText() + "\n\n" + content
			msgs[i] = provider.SystemMsg(updated)
			break
		}
	}
	loop.Context.Messages = msgs
	color.Green("✓ Skill '%s' activated", name)
	color.HiBlack("  Instructions injected into system prompt")
}


