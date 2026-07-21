package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"bugbuster-code/pkg/agent"
	"bugbuster-code/pkg/config"
	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
	"bugbuster-code/pkg/tools"

	"github.com/fatih/color"

	"encoding/json"

	"unicode/utf8"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// TUI — Tea model for interactive TUI mode
type TUI struct {
	cfg           *config.BugBusterConfig
	loop          *agent.AgentLoop
	changeTracker *ChangeTracker
	providerName  string
	program       *tea.Program              // Reference to tea.Program for Send()
	keys          config.ResolvedKeyBindings // Resolved key bindings
	mu            *sync.Mutex                // Protection from data race between Update() and View()

	// Display mode
	inline bool // true = no AltScreen, history stays in terminal

	// Session
	session    *agent.Session
	sessionMgr *agent.SessionManager

	// UI components
	viewport    viewport.Model
	textarea    textarea.Model
	progressBar progress.Model

	// State
	output       *strings.Builder
	err          error
	ready        bool
	streaming    bool
	spinnerFrame int

	// Thinking-buffer
	thinkingStarted bool
	thinkingBuf     *strings.Builder
	thinkingSummary string // last meaningful phrase from thinking (1 line)

	// Pending action — last line of model text that looks like an action announcement
	// Shown in spinner until tool starts executing
	pendingAction string

	// Compaction state
	compacting bool

	// Markdown-renderer
	mdRenderer *GlamourRenderer

	// Streaming statistics
	totalInTokens  int
	totalOutTokens int
	totalDuration  time.Duration
	genStart       time.Time // when first output token was received
	genEnd         time.Time // when last output token was received
	totalGenDur    time.Duration // accumulated generation time

	// Context tokens cache (updated in Update, used in View)
	// Do NOT read directly from m.loop.Context during streaming — data race!
	ctxTokens    int
	ctxMaxTokens int

	// Progress bar for tools
	toolInProgress   string          // name or summary of executing tool (with parameters)
	toolStartTime    time.Time       // tool execution start time
	showProgress     bool            // whether to show progress bar
	toolPercent      float64         // progress 0.0-1.0
	toolInputBuf     strings.Builder // buffer for accumulating tool parameters from delta events
	currentToolName  string          // current tool name (for formatToolSummary)
	toolOutputLines  []string        // last tool output lines (max 3)
	toolOutputCount  int             // total count of output lines

	// Streaming
	ctx    context.Context
	cancel context.CancelFunc

	// Input history
	history     []string // history of entered requests
	historyIdx  int      // current position in history (0 = newest)
	historySave string   // saved input during history navigation

	// Dimensions
	width  int
	height int

	// AskUser — mode waiting for user response
	askUserQuestion string   // current question from model (empty = not in ask_user mode)
	askUserChannel  *tools.AskChannel // channel for question/response exchange with ask_user tool

	// Reference to AskUserTool for channel setup
	askUserTool *tools.AskUserTool

	// Mode switch flag — if true, restart in CLI after TUI exit
	switchToCLI bool

	// Keyboard enhancement support
	hasCSIu bool // true if terminal supports CSI-u (extended keyboard protocol)

	// Autopilot — automatically continue after each response
	autoMode  bool
	autoState *AutoPilotState

	// Background process manager
	bgTool *tools.BackgroundTool

	// Task type for status line
	taskType string

	// Auto-save
	lastSaveTime time.Time
}

// streamEventMsg — streaming event sent via tea.Program.Send
type streamEventMsg struct {
	event provider.StreamEvent
}

// streamDoneMsg — streaming completion signal
type streamDoneMsg struct{}

// autoContinueMsg — autopilot command for automatic continuation
type autoContinueMsg struct {
	input string
}

// askUserMsg — question from model to user (via ask_user tool)
type askUserMsg struct {
	question string
}

// spinnerTickMsg — spinner tick
type spinnerTickMsg struct{}

// toolTickMsg — timer tick for tool execution time update
type toolTickMsg struct{}

// autoSaveTickMsg — timer tick for auto-saving session
type autoSaveTickMsg struct{}

// NewTUI creates new TUI model
func NewTUI(cfg *config.BugBusterConfig, loop *agent.AgentLoop, ct *ChangeTracker, providerName string, inline bool) TUI {
	ta := textarea.New()
	ta.Placeholder = "Enter request..."
	ta.Focus()
	ta.CharLimit = 10000
	ta.SetHeight(3)
	ta.DynamicHeight = true
	ta.MaxHeight = 10

	pb := progress.New(progress.WithDefaultBlend())

	// Find AskUserTool for communication channel setup
	var askUserTool *tools.AskUserTool
	if t, ok := loop.Tools["ask_user"]; ok {
		if aut, ok2 := t.(*tools.AskUserTool); ok2 {
			askUserTool = aut
		}
	}

	return TUI{
		cfg:           cfg,
		loop:          loop,
		changeTracker: ct,
		providerName:  providerName,
		keys:          cfg.Keys.Resolve(),
		inline:        inline,
		textarea:      ta,
		progressBar:   pb,
		output:        &strings.Builder{},
		thinkingBuf:   &strings.Builder{},
		mdRenderer:    NewGlamourRenderer(),
		mu:            &sync.Mutex{},
		ctxTokens:     loop.Context.TokenCount(),
		ctxMaxTokens:  loop.Context.MaxTokens,
		askUserTool:   askUserTool,
		bgTool:        tools.NewBackgroundTool(filepath.Join(getProjectDir(cfg), ".bugbuster", "bg_logs")),
	}
}

// Init initializes TUI
func (m TUI) Init() tea.Cmd {
	return tea.Batch(textarea.Blink)
}

// Update handles events
func (m TUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport = viewport.New()
		m.viewport.SetWidth(msg.Width)
		m.viewport.Style = lipgloss.NewStyle().
			Padding(0, 1)
		m.textarea.SetWidth(msg.Width - 4)
		m.progressBar.SetWidth(msg.Width - 4)
		m.ready = true
		m.updateTextareaHeight()
		m.syncViewport()
		return m, nil

	case tea.KeyboardEnhancementsMsg:
		m.hasCSIu = true
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)

	case tea.PasteMsg:
		text := msg.Content
		lineCount := strings.Count(text, "\n") + 1
		if lineCount > 1 && !m.streaming {
			// Multiline insertion — show compact block
			m.output.WriteString(pasteBlockStyle.Render(
				fmt.Sprintf("  📋 %s +%d %s", i18n.T("cli.paste_block"), lineCount-1, i18n.T("cli.paste_lines"))) + "\n")
			m.syncViewport()
		}
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
		m.updateTextareaHeight()
		return m, tea.Batch(cmds...)

	case streamEventMsg:
		return m.handleStreamEvent(msg)

	case askUserMsg:
		// Model asks user — show question and wait for response
		m.askUserQuestion = msg.question
		m.output.WriteString(
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Bold(true).
				Render("  ❓ "+msg.question) + "\n",
		)
		m.output.WriteString(
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("244")).
				Italic(true).
				Render("  ↳ enter response and press Enter") + "\n",
		)
		m.syncViewport()
		m.textarea.Reset()
		m.textarea.Focus()
		return m, textarea.Blink

	case streamDoneMsg:
		m.streaming = false
		m.showProgress = false
		m.pendingAction = ""
		m.askUserQuestion = "" // Reset ask_user mode on streaming completion
		// Clear AskChannel — streaming completed, ask_user no longer needed
		if m.askUserTool != nil {
			m.askUserTool.SetAskChannel(nil)
		}
		m.askUserChannel = nil

		// Autopilot: if enabled, check plan completion and continue
		if m.autoMode {
			// Check iteration limit
			if m.autoState != nil {
				m.autoState.Iteration++
				if m.autoState.Iteration >= m.autoState.MaxIterations {
					maxIter := m.autoState.MaxIterations
					m.autoMode = false
					m.autoState = nil
					m.output.WriteString(color.YellowString("🤖 %s", i18n.T("cli.auto_max_iterations", maxIter)) + "\n")
					m.syncViewport()
					return m, nil
				}
			}
			lastMsg := getLastAssistantMessage(m.loop)
			if isPlanCompleted(lastMsg) {
				m.autoMode = false
				m.autoState = nil
				m.output.WriteString(color.GreenString("✅ %s", i18n.T("cli.auto_plan_completed")) + "\n")
				m.syncViewport()
				return m, nil
			}
			// Automatically start next request
			phrase := randomContinuePhrase()
			if m.autoState != nil {
				m.output.WriteString(color.HiCyanString("%s", formatAutoIteration(m.autoState.Iteration, m.autoState.MaxIterations, phrase)) + "\n")
			} else {
				m.output.WriteString(color.HiCyanString("🤖 Auto: %s", phrase) + "\n")
			}
			m.syncViewport()
			// Start new stream after delay
			return m, autoContinueCmd(phrase)
		}

		m.syncViewport()
		return m, nil

	case autoContinueMsg:
		// Autopilot: start next request automatically
		if m.autoMode && !m.streaming {
			m.textarea.Reset()
			m.updateTextareaHeight()
			m.historyIdx = 0
			m.historySave = ""

			// Reset statistics
			m.totalInTokens = 0
			m.totalOutTokens = 0
			m.totalDuration = 0
			m.thinkingStarted = false
			m.thinkingSummary = ""

			m.streaming = true

			// Create AskChannel for ask_user
			askCh := &tools.AskChannel{
				Question: make(chan string, 1),
				Answer:   make(chan string, 1),
			}
			if m.askUserTool != nil {
				m.askUserTool.SetAskChannel(askCh)
			}
			m.askUserChannel = askCh

			ctx, cancel := context.WithCancel(context.Background())
			m.ctx = ctx
			m.cancel = cancel

			go m.runStream(msg.input, ctx, m.program)
			return m, m.spinnerCmd()
		}

	case spinnerTickMsg:
		if m.streaming {
			m.spinnerFrame++
			m.syncViewport()
			return m, m.spinnerCmd()
		}
		if m.compacting {
			m.spinnerFrame++
			m.syncViewport()
			return m, m.spinnerCmd()
		}

	case toolTickMsg:
		if m.showProgress {
			m.syncViewport()
			return m, m.toolTickCmd()
		}

	case autoSaveTickMsg:
		// Auto-save session every 30 seconds during streaming
		if m.streaming && m.session != nil && m.sessionMgr != nil {
			m.session.Messages = m.loop.Context.GetMessages()
			m.session.InputHistory = m.history
			if err := m.sessionMgr.SaveSessionMessages(m.session); err == nil {
				m.lastSaveTime = time.Now()
			}
		}
		return m, m.autoSaveCmd()

	case progress.FrameMsg:
		// Progress bar animation
		var cmd tea.Cmd
		m.progressBar, cmd = m.progressBar.Update(msg)
		return m, cmd
	}

	// Update textarea
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	// Update viewport
	if m.ready {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// handleKeyPress handles key presses
func (m TUI) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	// Debug: log key press for troubleshooting Shift+Enter/Alt+Enter
	if os.Getenv("BUGBUSTER_DEBUG_KEYS") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] key: string=%q code=%d mod=%v text=%q hasCSIu=%v\n",
			msg.String(), msg.Code, msg.Mod, msg.Text, m.hasCSIu)
	}

	// Newline (Shift+Enter/Alt+Enter/Ctrl+J) — line break in textarea
	if m.keys.Matches(msg, config.ActionNewline) {
		m.textarea, cmd = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	// Cancel (Ctrl+C) — streaming cancellation or exit
	if m.keys.Matches(msg, config.ActionCancel) {
		// If waiting for ask_user response — send empty response and cancel
		if m.askUserQuestion != "" && m.askUserChannel != nil {
			select {
			case m.askUserChannel.Answer <- "":
			default:
			}
			m.askUserQuestion = ""
		}
		// If in autopilot mode — disable autopilot
		if m.autoMode {
			m.autoMode = false
			m.autoState = nil
			m.output.WriteString(color.YellowString("🤖 %s", i18n.T("cli.auto_disabled")) + "\n")
			m.syncViewport()
		}
		if m.streaming && m.cancel != nil {
			m.cancel()
			m.streaming = false
			m.output.WriteString("\n" + errorStyle.Render("  Request cancelled.") + "\n")
			m.syncViewport()
			return m, nil
		}
		return m, tea.Quit
	}

	// Interrupt (Esc) — interrupt streaming or exit
	if m.keys.Matches(msg, config.ActionInterrupt) {
		// If waiting for ask_user response — send empty response
		if m.askUserQuestion != "" && m.askUserChannel != nil {
			select {
			case m.askUserChannel.Answer <- "":
			default:
			}
			m.askUserQuestion = ""
		}
		if m.streaming {
			if m.cancel != nil {
				m.cancel()
			}
			m.streaming = false
			return m, nil
		}
		return m, tea.Quit
	}

	// Send (Enter) — send request
	if m.keys.Matches(msg, config.ActionSend) {
		return m.handleSend()
	}

	// HistoryUp (↑) — history navigation, only if cursor is on first line
	// First pass Up to textarea — if cursor moved, this is navigation within text.
	// If cursor did not move (was on line 0) — switch history.
	if m.keys.Matches(msg, config.ActionHistoryUp) {
		lineBefore := m.textarea.Line()
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
		if m.textarea.Line() == lineBefore && lineBefore == 0 && len(m.history) > 0 {
			if m.historyIdx == 0 {
				m.historySave = m.textarea.Value()
			}
			if m.historyIdx < len(m.history) {
				m.historyIdx++
				m.textarea.SetValue(m.history[len(m.history)-m.historyIdx])
			}
		}
		m.updateTextareaHeight()
		return m, tea.Batch(cmds...)
	}

	// HistoryDown (↓) — history navigation, only if cursor is on last line
	// First pass Down to textarea — if cursor moved, this is navigation within text.
	// If cursor did not move (was on last line) — switch history.
	if m.keys.Matches(msg, config.ActionHistoryDown) {
		lineBefore := m.textarea.Line()
		lineCountBefore := m.textarea.LineCount()
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
		if m.textarea.Line() == lineBefore && lineBefore == lineCountBefore-1 && m.historyIdx > 0 {
			m.historyIdx--
			if m.historyIdx == 0 {
				m.textarea.SetValue(m.historySave)
			} else {
				m.textarea.SetValue(m.history[len(m.history)-m.historyIdx])
			}
		}
		m.updateTextareaHeight()
		return m, tea.Batch(cmds...)
	}

	// ScrollUp (PgUp/Ctrl+U) — scroll viewport up
	if m.keys.Matches(msg, config.ActionScrollUp) {
		m.viewport.HalfPageUp()
		return m, nil
	}

	// ScrollDown (PgDown/Ctrl+D) — scroll viewport down
	if m.keys.Matches(msg, config.ActionScrollDown) {
		m.viewport.HalfPageDown()
		return m, nil
	}

	// Unhandled keys — pass to textarea
	m.textarea, cmd = m.textarea.Update(msg)
	m.updateTextareaHeight()
	return m, cmd
}

// handleSend handles request sending
func (m TUI) handleSend() (retModel tea.Model, retCmd tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			// Recover from panic — log error and continue
			m.output.WriteString(errorStyle.Render(fmt.Sprintf("  ⚠ Recovered: %v", r)) + "\n")
			m.syncViewport()
			m.textarea.Reset()
			retModel = m
			retCmd = nil
		}
	}()

	input := strings.TrimSpace(m.textarea.Value())
	if input == "" {
		return m, nil
	}

	// If model is waiting for ask_user response — send response via channel
	if m.askUserQuestion != "" && m.askUserChannel != nil {
		// Non-blocking send with timeout — don't hang if tool is no longer waiting
		select {
		case m.askUserChannel.Answer <- input:
		case <-time.After(5 * time.Second):
			// Tool no longer waiting — discard answer
		}
		m.askUserQuestion = ""
		m.textarea.Reset()
		m.updateTextareaHeight()
		m.output.WriteString(
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Render("  ↳ "+input) + "\n",
		)
		m.syncViewport()
		return m, nil
	}

	// Save to history
	m.addToHistory(input)

	// Commands work in any mode
	switch input {
	case "/exit", "/quit":
		return m, tea.Quit
	case "/help":
		m.output.WriteString(helpStyle.Render(printHelpString()) + "\n")
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/mcp":
		m.output.WriteString(mcpInfoString(m.cfg) + "\n")
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/sessions":
		m.output.WriteString(sessionsInfoString(m.sessionMgr, m.session) + "\n")
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/ps":
		processes := m.bgTool.ListProcesses()
		if len(processes) == 0 {
			m.output.WriteString("  No background processes\n")
		} else {
			m.output.WriteString("  Background Processes:\n")
			for _, p := range processes {
				status := "running"
				if !p.Running.Load() {
					status = fmt.Sprintf("exit(%d)", p.ExitCode.Load())
				}
				uptime := time.Since(p.StartTime).Truncate(time.Second)
				m.output.WriteString(fmt.Sprintf("  #%d PID:%d %s %s %s\n", p.ID, p.PID, status, uptime, p.Command))
			}
		}
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/kill ":
		idStr := strings.TrimPrefix(input, "/kill ")
		id, err := strconv.Atoi(strings.TrimSpace(idStr))
		if err != nil {
			m.output.WriteString(fmt.Sprintf("  ✗ Invalid process ID: %s\n", idStr))
		} else if err := m.bgTool.KillProcess(id); err != nil {
			m.output.WriteString(fmt.Sprintf("  ✗ %v\n", err))
		} else {
			m.output.WriteString(fmt.Sprintf("  ✓ Process #%d killed\n", id))
		}
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/logs ":
		idStr := strings.TrimPrefix(input, "/logs ")
		id, err := strconv.Atoi(strings.TrimSpace(idStr))
		if err != nil {
			m.output.WriteString(fmt.Sprintf("  ✗ Invalid process ID: %s\n", idStr))
		} else {
			content, err := m.bgTool.ReadLogs(id, 50)
			if err != nil {
				m.output.WriteString(fmt.Sprintf("  ✗ %v\n", err))
			} else if content == "" {
				m.output.WriteString(fmt.Sprintf("  Process #%d has no output yet\n", id))
			} else {
				m.output.WriteString(fmt.Sprintf("  Logs for process #%d:\n", id))
				m.output.WriteString(content + "\n")
			}
		}
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/compact":
		if m.streaming {
			m.output.WriteString(errorStyle.Render("  ✗ "+i18n.T("cli.compaction_during_stream")) + "\n")
			m.syncViewport()
			m.textarea.Reset()
			return m, nil
		}
		tokensBefore := m.loop.Context.TokenCount()
		maxTokens := m.loop.Context.MaxTokens
		if tokensBefore <= maxTokens {
			m.output.WriteString(color.GreenString("  ✓ "+i18n.T("cli.compaction_not_needed")) + fmt.Sprintf(" (%d/%d)\n", tokensBefore, maxTokens))
		} else {
			m.output.WriteString(color.YellowString("  🔄 "+i18n.T("cli.compacting")) + fmt.Sprintf(" (%d/%d)...\n", tokensBefore, maxTokens))
			m.loop.Context.Compact()
			tokensAfter := m.loop.Context.TokenCount()
			saved := tokensBefore - tokensAfter
			m.output.WriteString(color.GreenString("  ✓ "+i18n.T("cli.compaction_done")) + fmt.Sprintf(" %d → %d (%s: %d)\n", tokensBefore, tokensAfter, i18n.T("cli.compaction_saved"), saved))
			m.ctxTokens = tokensAfter
		}
		m.ctxMaxTokens = maxTokens
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/compact!":
		if m.streaming {
			m.output.WriteString(errorStyle.Render("  ✗ "+i18n.T("cli.compaction_during_stream")) + "\n")
			m.syncViewport()
			m.textarea.Reset()
			return m, nil
		}
		tokensBefore := m.loop.Context.TokenCount()
		m.output.WriteString(color.YellowString("  💥 Force compacting...") + fmt.Sprintf(" (%d tokens)...\n", tokensBefore))
		m.loop.Context.CompactForce()
		m.loop.ResetSpeedTracking()
		m.loop.SetLastAutoCompactAt()
		tokensAfter := m.loop.Context.TokenCount()
		saved := tokensBefore - tokensAfter
		m.output.WriteString(color.GreenString("  ✓ Force compacted") + fmt.Sprintf(" %d → %d (saved: %d)\n", tokensBefore, tokensAfter, saved))
		m.ctxTokens = tokensAfter
		m.ctxMaxTokens = m.loop.Context.MaxTokens
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/context":
		if m.streaming {
			// During streaming use cached values (no data race)
			m.output.WriteString(FormatContextInfo(-1, m.ctxTokens, m.ctxMaxTokens) + "\n")
		} else {
			tokensUsed := m.loop.Context.TokenCount()
			maxTokens := m.loop.Context.MaxTokens
			msgCount := len(m.loop.Context.GetMessages())
			m.ctxTokens = tokensUsed
			m.ctxMaxTokens = maxTokens
			m.output.WriteString(FormatContextInfo(msgCount, tokensUsed, maxTokens) + "\n")
		}
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/cli":
		// Switch to CLI mode: save session, exit TUI, restart
		saveSessionTUI(m)
		m.switchToCLI = true
		return m, tea.Quit
	case "/tui":
		// Already in TUI mode — show hint
		m.output.WriteString(helpStyle.Render("  ℹ "+i18n.T("cli.already_in_tui")) + "\n")
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/auto":
		m.autoMode = !m.autoMode
		if m.autoMode {
			m.autoState = NewAutoPilotState(0)
			m.output.WriteString(color.HiCyanString("🤖 %s", i18n.T("cli.auto_enabled")) + "\n")
		} else {
			m.autoState = nil
			m.output.WriteString(color.YellowString("🤖 %s", i18n.T("cli.auto_disabled")) + "\n")
		}
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/dream":
		m.output.WriteString(handleDreamCommandTUI(m.loop) + "\n")
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/emotions":
		m.output.WriteString(handleEmotionsCommandTUI(m.loop) + "\n")
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/mesh-stats":
		m.output.WriteString(handleMeshStatsCommandTUI(m.loop) + "\n")
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/provider":
		// List available providers
		m.output.WriteString(listProvidersTUI(m.cfg, m.providerName) + "\n")
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	case "/rename":
		m.output.WriteString(errorStyle.Render("  ✗ Usage: /rename <new_name>") + "\n")
		m.syncViewport()
		m.textarea.Reset()
		return m, nil
	default:
		// Check /auto N
		if strings.HasPrefix(input, "/auto ") {
			maxIter := 0
			if parts := strings.Fields(input); len(parts) == 2 {
				if _, err := fmt.Sscanf(parts[1], "%d", &maxIter); err != nil {
					maxIter = 0
				}
			}
			m.autoMode = true
			m.autoState = NewAutoPilotState(maxIter)
			m.output.WriteString(color.HiCyanString("🤖 %s", i18n.T("cli.auto_enabled")) + "\n")
			if m.autoState.MaxIterations != autoMaxIterations {
				m.output.WriteString(color.HiCyanString("   Max iterations: %d", m.autoState.MaxIterations) + "\n")
			}
			m.syncViewport()
			m.textarea.Reset()
			return m, nil
		}
		// Check /provider <name>
		if strings.HasPrefix(input, "/provider ") {
			provName := strings.TrimSpace(strings.TrimPrefix(input, "/provider "))
			if provName == "" {
				m.output.WriteString(listProvidersTUI(m.cfg, m.providerName) + "\n")
			} else {
				newProvider, err := switchProviderTUI(m.cfg, provName, m.loop, m.providerName)
				if err != nil {
					m.output.WriteString(errorStyle.Render(fmt.Sprintf("  ✗ %v", err)) + "\n")
				} else {
					m.providerName = provName
					// Save provider to session
					if m.session != nil {
						m.session.ProviderName = provName
						if m.sessionMgr != nil {
							m.sessionMgr.SaveSession(m.session)
						}
					}
					m.output.WriteString(color.GreenString("  ✓ Provider switched to: %s", provName) + "\n")
					_ = newProvider
				}
			}
			m.syncViewport()
			m.textarea.Reset()
			return m, nil
		}
		// Check /rename <name>
		if strings.HasPrefix(input, "/rename ") {
			newName := strings.TrimSpace(strings.TrimPrefix(input, "/rename "))
			if newName == "" {
				m.output.WriteString(errorStyle.Render("  ✗ Usage: /rename <new_name>") + "\n")
			} else if m.session != nil && m.sessionMgr != nil {
				if err := m.sessionMgr.RenameSession(m.session.ID, newName); err != nil {
					m.output.WriteString(errorStyle.Render(fmt.Sprintf("  ✗ %v", err)) + "\n")
				} else {
					m.session.Name = newName
					m.output.WriteString(color.GreenString("  ✓ Session renamed to: %s", newName) + "\n")
				}
			} else {
				m.output.WriteString(errorStyle.Render("  ✗ No active session") + "\n")
			}
			m.syncViewport()
			m.textarea.Reset()
			return m, nil
		}
		// Check /task <type> — auto-switch provider based on task type
		if strings.HasPrefix(input, "/task ") {
			taskType := strings.TrimSpace(strings.TrimPrefix(input, "/task "))
			if taskType == "" {
				// List available task types
				m.output.WriteString(color.CyanString("🎯 Available task types:\n"))
				for _, t := range m.cfg.TaskTypes() {
					provName := m.cfg.GetProviderForTask(t)
					prefix := "  "
					if provName == m.providerName {
						prefix = "→ "
					}
					m.output.WriteString(fmt.Sprintf("%s%s → %s\n", prefix, color.HiYellowString(t), provName))
				}
				m.output.WriteString(fmt.Sprintf("\n%s /task <type>\n", color.HiBlackString("Switch:")))
			} else {
				provName := m.cfg.GetProviderForTask(taskType)
				if provName == m.providerName {
					m.output.WriteString(color.HiYellowString("  ⚠ Already using %s for task '%s'\n", provName, taskType))
				} else {
					newProvider, err := switchProviderTUI(m.cfg, provName, m.loop, m.providerName)
					if err != nil {
						m.output.WriteString(errorStyle.Render(fmt.Sprintf("  ✗ %v", err)) + "\n")
					} else {
						m.providerName = provName
						// Save provider to session
						if m.session != nil {
							m.session.ProviderName = provName
							if m.sessionMgr != nil {
								m.sessionMgr.SaveSession(m.session)
							}
						}
						m.taskType = taskType
						m.output.WriteString(color.GreenString("  ✓ Task '%s' → provider %s\n", taskType, provName))
						_ = newProvider
					}
				}
			}
			m.syncViewport()
			m.textarea.Reset()
			return m, nil
		}
	}

	m.textarea.Reset()
	m.updateTextareaHeight()
	m.historyIdx = 0
	m.historySave = ""

	if m.streaming {
		// During streaming — inject comment into agent
		m.output.WriteString(
			lipgloss.NewStyle().
				Foreground(appTheme.Warning.LipglossColor()).
				Bold(true).
				Render("  💬 "+input) + "\n",
		)
		m.syncViewport()
		m.loop.InjectUserMessage(input)
		return m, nil
	}

	// Not streaming — start new request
	m.mdRenderer = NewGlamourRenderer()
	m.totalInTokens = 0
	m.totalOutTokens = 0
	m.totalDuration = 0
	m.pendingAction = ""

	// Auto-switch provider based on task content
	if m.cfg.AgentProviders != nil && m.taskType == "" {
		detectedTask := detectTaskType(input)
		if detectedTask != "" {
			provName := m.cfg.GetProviderForTask(detectedTask)
			if provName != "" && provName != m.providerName {
				newProvider, err := switchProviderTUI(m.cfg, provName, m.loop, m.providerName)
				if err == nil {
					m.providerName = provName
					m.taskType = detectedTask
					if m.session != nil {
						m.session.ProviderName = provName
						if m.sessionMgr != nil {
							m.sessionMgr.SaveSession(m.session)
						}
					}
					m.output.WriteString(color.HiCyanString("  🔄 Auto-switched to %s (task: %s)\n", provName, detectedTask))
					_ = newProvider
				}
			}
		}
	}

	m.output.WriteString(userMsgStyle.Render("  ❯ "+input) + "\n")
	m.output.WriteString(separatorStyle.Render("  ──────────────────────────────────────────────────") + "\n")
	m.streaming = true
	m.syncViewport()

	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel

	// Set AskChannel for ask_user tool (TUI mode)
	if m.askUserTool != nil {
		ch := &tools.AskChannel{
			Question: make(chan string, 1),
			Answer:   make(chan string, 1),
		}
		m.askUserChannel = ch
		m.askUserTool.SetAskChannel(ch)
		// Start goroutine for reading ask_user questions and sending to TUI
		// Capture program in local variable to avoid data race
		askProgram := m.program
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case question, ok := <-ch.Question:
					if !ok {
						return
					}
					if askProgram != nil {
						askProgram.Send(askUserMsg{question: question})
					}
				}
			}
		}()
	}

	go m.runStream(input, ctx, m.program)

	return m, tea.Batch(m.spinnerCmd(), m.autoSaveCmd())
}

// handleStreamEvent handles streaming events
func (m TUI) handleStreamEvent(msg streamEventMsg) (tea.Model, tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			// Recover from panic — log error and continue
			m.output.WriteString(errorStyle.Render(fmt.Sprintf("  ⚠ Recovered: %v", r)) + "\n")
			m.syncViewport()
		}
	}()

	var cmds []tea.Cmd

	switch msg.event.Type {
	case provider.EventTextDelta:
		// Track generation time for accurate speed calculation
		m.totalOutTokens++
		if m.genStart.IsZero() {
			m.genStart = time.Now()
		} else if !m.genEnd.IsZero() {
			// Accumulate generation time between tokens (skip if genEnd was reset)
			m.totalGenDur += time.Since(m.genEnd)
		}
		m.genEnd = time.Now()

		// Track last line as pending action
		text := msg.event.Text
		if len(text) > 0 {
			lastNewline := strings.LastIndex(text, "\n")
			lastLine := text
			if lastNewline >= 0 && lastNewline+1 < len(text) {
				lastLine = text[lastNewline+1:]
			}
			lastLine = strings.TrimSpace(lastLine)
			if len(lastLine) > 0 && len(lastLine) < 120 {
				m.pendingAction = lastLine
			}
		}
		m.syncViewport()
	case provider.EventThinking:
		// Reset genEnd so thinking time is not counted as generation time
		m.genEnd = time.Time{}
		if !m.thinkingStarted {
			m.output.WriteString(lipgloss.NewStyle().Foreground(appTheme.Thinking.LipglossColor()).Italic(true).Render("  ∴ "+i18n.T("cli.thinking")) + "\n")
			m.thinkingStarted = true
		}
		m.thinkingBuf.WriteString(msg.event.Text)
		// Update thinking summary — filter code-like lines
		m.thinkingSummary = summarizeThinking(m.thinkingBuf.String())
		m.syncViewport()
	case provider.EventToolCallStart:
		// Accumulate generation time for tool call tokens before resetting
		if !m.genEnd.IsZero() {
			m.totalGenDur += time.Since(m.genEnd)
		}
		m.genEnd = time.Time{} // reset genEnd so tool execution time is not counted as generation time
		m.flushThinking()
		m.pendingAction = ""
		m.output.WriteString(m.mdRenderer.Flush())
		// Convert ToolInput to map[string]string for FormatToolCallStart
		var toolParams map[string]string
		if msg.event.ToolInput != nil && len(msg.event.ToolInput) > 0 {
			toolParams = make(map[string]string)
			for k, v := range msg.event.ToolInput {
				toolParams[k] = fmt.Sprintf("%v", v)
			}
		}
		// Don't write tool start to output — show in progress bar instead.
		// Full formatting will be written on EventToolCallEnd (like CLI mode).
		// Start progress bar for tool
		m.currentToolName = msg.event.ToolName
		m.toolInProgress = formatToolSummary(msg.event.ToolName, toolParams)
		m.toolStartTime = time.Now()
		m.showProgress = true
		m.toolPercent = 0
		m.toolInputBuf.Reset()
		m.toolOutputLines = nil
		m.toolOutputCount = 0
		m.syncViewport()
		cmds = append(cmds, m.toolTickCmd())
	case provider.EventToolCallDelta:
		// Tool call tokens are also generation — count them and track time
		if m.genStart.IsZero() {
			m.genStart = time.Now()
		}
		if !m.genEnd.IsZero() {
			m.totalGenDur += time.Since(m.genEnd)
		}
		m.genEnd = time.Now()
		m.totalOutTokens++
		m.toolInputBuf.WriteString(msg.event.ToolDelta)
		// Try to parse partial JSON for progress lines update
		params := parsePartialToolInput(m.toolInputBuf.String())
		if len(params) > 0 {
			m.toolInProgress = formatToolSummary(m.currentToolName, params)
		}
		m.syncViewport()
	case provider.EventToolProgress:
		m.toolPercent = msg.event.ToolProgress
		if msg.event.ToolMessage != "" {
			// Add line to output tail (max 3 lines)
			m.toolOutputLines = append(m.toolOutputLines, msg.event.ToolMessage)
			if len(m.toolOutputLines) > 3 {
				m.toolOutputLines = m.toolOutputLines[len(m.toolOutputLines)-3:]
			}
			m.toolOutputCount++
		}
		m.syncViewport()
	case provider.EventToolCallEnd:
		m.genEnd = time.Time{} // reset genEnd so tool execution time is not counted as generation time
		m.showProgress = false
		m.toolInProgress = ""
		m.toolPercent = 0
		m.currentToolName = ""
		m.toolOutputLines = nil
		m.toolOutputCount = 0
		// Convert ToolInput for FormatToolCallStart and FormatToolCallEnd
		toolEndParams := make(map[string]string)
		if msg.event.ToolInput != nil {
			for k, v := range msg.event.ToolInput {
				toolEndParams[k] = fmt.Sprintf("%v", v)
			}
		}
		// Show tool call start + end together (like CLI mode)
		m.output.WriteString("\n" + FormatToolCallStart(msg.event.ToolName, toolEndParams) + "\n")
		m.output.WriteString(FormatToolCallEnd(msg.event.ToolName, msg.event.ToolOK, msg.event.ToolResult, msg.event.ToolFullResult, msg.event.Duration, toolEndParams) + "\n")
		// Specific checklist rendering for todo_write
		if msg.event.ToolName == "todo_write" && msg.event.ToolOK {
			m.output.WriteString(FormatTodoChecklist(msg.event.ToolFullResult) + "\n")
		}
		m.syncViewport()
		// Auto-save session after each tool call
		saveSessionTUI(m)
	case provider.EventUserInjected:
		m.output.WriteString(
			lipgloss.NewStyle().
				Foreground(appTheme.Warning.LipglossColor()).
				Italic(true).
				Render("  ↳ comment added to context") + "\n",
		)
		m.syncViewport()
	case provider.EventAutoContinue:
		// Show as dim system message (not as model text)
		m.output.WriteString(
			lipgloss.NewStyle().
				Foreground(appTheme.Dim.LipglossColor()).
				Italic(true).
				Render("  ↻ auto-continue") + "\n",
		)
		m.syncViewport()
	case provider.EventCompaction:
		m.compacting = true
		m.output.WriteString(
			lipgloss.NewStyle().
				Foreground(appTheme.Dim.LipglossColor()).
				Italic(true).
				Render("  🔄 "+i18n.T("cli.compacting")) + "\n",
		)
		m.syncViewport()
		// Start spinner
		return m, m.spinnerCmd()
	case provider.EventCompactionDone:
		m.compacting = false
		m.syncViewport()
		// Auto-save session after compaction
		saveSessionTUI(m)
	case provider.EventThinkingTimeout:
		mins := int(msg.event.Duration.Minutes())
		if mins < 1 {
			mins = 1
		}
		m.output.WriteString(
			lipgloss.NewStyle().
				Foreground(lipgloss.Color("11")).
				Render(fmt.Sprintf("\n  ⚠️  %s", i18n.T("cli.thinking_timeout_warn", fmt.Sprintf("%d", mins)))) + "\n",
		)
		m.syncViewport()
	case provider.EventUsage:
		m.totalInTokens = max(m.totalInTokens, msg.event.InputTokens)
		m.totalOutTokens = max(m.totalOutTokens, msg.event.OutputTokens)
		// Note: do NOT update genStart/genEnd here — EventUsage is not a generation event.
		// Only EventTextDelta/EventToolCallDelta should update generation time.
		// Update ctxTokens from Usage data (safely, no data race)
		if msg.event.InputTokens > 0 {
			m.ctxTokens = msg.event.InputTokens + msg.event.OutputTokens
		}
	case provider.EventDone:
		m.flushThinking()
		m.totalDuration = msg.event.Duration
		// Update ctxTokens from event data (safely, no data race)
		if msg.event.InputTokens > 0 {
			m.ctxTokens = msg.event.InputTokens + msg.event.OutputTokens
		}
		m.showProgress = false
		m.output.WriteString(m.mdRenderer.Flush())
		m.output.WriteString("\n")
		m.syncViewport()
		// Incremental session save after each response
		saveSessionTUI(m)
	case provider.EventError:
		m.showProgress = false
		m.output.WriteString(errorStyle.Render(i18n.T("cli_error.stream", msg.event.Error)) + "\n")
		m.syncViewport()
	}

	return m, tea.Batch(cmds...)
}

// flushThinking outputs accumulated thinking buffer
func (m *TUI) flushThinking() {
	if !m.thinkingStarted {
		return
	}
	thinkingText := m.thinkingBuf.String()
	m.thinkingBuf.Reset()
	m.thinkingStarted = false
	if thinkingText != "" {
		wrapped := wrapText(strings.TrimSpace(thinkingText), 4, 80)
		m.output.WriteString(lipgloss.NewStyle().Foreground(appTheme.Thinking.LipglossColor()).Render(wrapped) + "\n\n")
	}
}

// View renders TUI
func (m TUI) View() tea.View {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.ready {
		return tea.NewView("  BugBuster Code\n  Loading...")
	}

	// Header — name and provider/model
	provCfg := m.cfg.Providers[m.cfg.DefaultProvider]
	var headerInfo string
	displayProvider := providerDisplayName(m.providerName, provCfg)
	if provCfg.Model != "" {
		// Avoid duplication like "qwen-fast-35b · qwen-fast-35b"
		if displayProvider == provCfg.Model {
			headerInfo = provCfg.Model
		} else {
			headerInfo = fmt.Sprintf("%s · %s", displayProvider, provCfg.Model)
		}
	} else {
		headerInfo = displayProvider
	}
	header := lipgloss.NewStyle().
		Foreground(appTheme.Dim.LipglossColor()).
		Bold(true).
		Render("  BugBuster") + lipgloss.NewStyle().
		Foreground(appTheme.Dim.LipglossColor()).
		Render(" "+headerInfo)

	// Main content — safely render viewport
	var content string
	viewResult, viewErr := safeViewportView(m.viewport)
	if viewErr != nil {
		content = errorStyle.Render(fmt.Sprintf("  ⚠ Render error: %v", viewErr))
	} else if m.streaming {
		spinner := tuiSpinnerFrames[m.spinnerFrame%len(tuiSpinnerFrames)]
		if m.compacting {
			content = viewResult + "\n" + assistantStyle.Render("  "+spinner+" "+i18n.T("cli.compacting"))
		} else if m.thinkingStarted && m.thinkingSummary != "" {
			content = viewResult + "\n" + assistantStyle.Render("  "+spinner+" Thinking: "+m.thinkingSummary)
		} else if m.thinkingStarted {
			content = viewResult + "\n" + assistantStyle.Render("  "+spinner+" Thinking...")
		} else if m.pendingAction != "" {
			// Show last model text line as expected action indicator
			actionText := m.pendingAction
			if len(actionText) > 80 {
				actionText = actionText[:77] + "..."
			}
			content = viewResult + "\n" + assistantStyle.Render("  "+spinner+" "+actionText)
		} else {
			content = viewResult + "\n" + assistantStyle.Render("  "+spinner+" Thinking...")
		}
	} else {
		content = viewResult
	}

		// Progress bar for tool
		if m.showProgress && m.toolInProgress != "" {
			spinner := tuiSpinnerFrames[m.spinnerFrame%len(tuiSpinnerFrames)]
			elapsed := time.Since(m.toolStartTime).Round(100 * time.Millisecond)
			// Don't truncate bash/write/edit commands — user must see full command for security
			toolText := m.toolInProgress
			noTruncateTool := m.currentToolName == "bash" || m.currentToolName == "write" || m.currentToolName == "edit" || m.currentToolName == "delegate_task"
			if !noTruncateTool {
				maxToolLen := m.width - 20
				if maxToolLen < 40 {
					maxToolLen = 40
				}
				if utf8.RuneCountInString(toolText) > maxToolLen {
					runes := []rune(toolText)
					toolText = string(runes[:maxToolLen-3]) + "..."
				}
			}
			// Header: spinner + tool name + time + line count
			toolHeader := fmt.Sprintf("  %s ⏺ %s  %s", spinner, toolText, elapsed)
			if m.toolOutputCount > 0 {
				toolHeader += fmt.Sprintf(" [%d lines]", m.toolOutputCount)
			}
			toolLine := toolProgressStyle.Render(toolHeader)
			if m.toolPercent > 0 {
				pbar := m.progressBar.ViewAs(m.toolPercent)
				content += "\n" + toolLine + "\n" + "  " + pbar
			} else {
				content += "\n" + toolLine
			}
			// Output tail (last 3 lines)
			for _, line := range m.toolOutputLines {
				if len(line) > 120 {
					line = line[:117] + "..."
				}
				content += "\n" + helpStyle.Render("  │ "+line)
			}
		}


	// Status bar with tokens, time, context bar
	genDur := m.totalGenDur
	statusBar := FormatStatusLineEx(
		m.totalInTokens, m.totalOutTokens,
		m.totalDuration, genDur,
		m.ctxTokens, m.ctxMaxTokens,
		providerDisplayName(m.providerName, provCfg), provCfg.Model,
		m.taskType,
	)

	// Input field — change placeholder if waiting for ask_user response
	if m.askUserQuestion != "" {
		m.textarea.Placeholder = "↳ Enter response..."
	} else {
		m.textarea.Placeholder = "Enter request..."
	}
	input := m.textarea.View()

	// Hints — dynamic, based on key binding configuration
	var help string
	if m.streaming {
		help = helpStyle.Render("  " + m.keys.FormatHelp("streaming", i18n.T))
	} else {
		help = helpStyle.Render("  " + m.keys.FormatHelp("idle", i18n.T))
	}

	result := header + "\n" + content
	if statusBar != "" {
		result += "\n" + statusBar
	}
	result += "\n" + input + "\n" + help

	// If terminal doesn't support CSI-u, show Ctrl+J hint for newline
	if !m.hasCSIu && m.ready {
		help += helpStyle.Render("  │ ctrl+j — " + i18n.T("keys.newline"))
	}

	v := tea.NewView(result)
	if !m.inline {
		v.AltScreen = true
	}
	// Enable extended keyboard protocol for Shift+Enter/Alt+Enter detection
	v.KeyboardEnhancements.ReportEventTypes = true
	v.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
	v.KeyboardEnhancements.ReportAssociatedText = true
	return v
}

// runTUI starts TUI mode. Returns true if need to switch to CLI.
func runTUI(cfg *config.BugBusterConfig, loop *agent.AgentLoop, ct *ChangeTracker, providerName string, mode string) (retBool bool) {
	// Set TUI active flag — crash handler should not write stderr to terminal
	tuiActive = true
	defer func() { tuiActive = false }()

	// Recover from panic — write crash log to file instead of terminal
	defer func() {
		if r := recover(); r != nil {
			restoreTerminalToNormal()
			writeCrashLog(r)
			retBool = false
		}
	}()

	inline := mode == "inline"
	m := NewTUI(cfg, loop, ct, providerName, inline)

	// Create session
	sessionsDir := filepath.Join(getProjectDir(cfg), ".bugbuster", "sessions")
	sessionMgr := agent.NewSessionManager(sessionsDir)

	var currentSession *agent.Session
	if sessionID != "" {
		loaded, err := sessionMgr.LoadSession(sessionID)
		if err != nil {
			currentSession = sessionMgr.NewSession()
		} else {
			currentSession = loaded
			if currentSession.Messages != nil {
				loop.Context.Messages = currentSession.Messages // safe: not streaming yet
			}
		}
	} else {
		currentSession = sessionMgr.NewSession()
	}

	m.session = currentSession
	m.sessionMgr = sessionMgr
	loop.Context.SessionID = currentSession.ID

	// Restore provider from session if available
	if currentSession.ProviderName != "" {
		if provCfg, ok := cfg.Providers[currentSession.ProviderName]; ok {
			p, err := provider.NewFromConfig(currentSession.ProviderName, provCfg)
			if err == nil {
				loop.SetProvider(p)
				providerName = currentSession.ProviderName
			m.taskType = cfg.GetTaskTypeForProvider(currentSession.ProviderName)
			}
		}
	}

	m.providerName = providerName

	// Set global session references for crash recovery
	globalSession = currentSession
	globalSessionMgr = sessionMgr
	globalLoop = loop
	globalTUI = &m
	// Update SessionID in SearchContextTool
	if searchTool, ok := loop.Tools["search_context"].(*agent.SearchContextTool); ok {
		searchTool.SessionID = currentSession.ID
	}
	// Update SessionID in MemoryTool
	if memTool, ok := loop.Tools["memory"].(*tools.MemoryTool); ok {
		memTool.SetSessionIDForProject(currentSession.ID, getProjectDir(m.cfg))
	}

	// Restore chat history in TUI
	if currentSession.Messages != nil && len(currentSession.Messages) > 0 {
		// Skip system message (usually first)
		renderSessionHistory(currentSession.Messages, m.output, m.mdRenderer)
	}

	// Restore input history from session
	if currentSession.InputHistory != nil {
		m.history = currentSession.InputHistory
	}

	// Open /dev/tty directly for Bubble Tea input.
	// This avoids conflicts with readline goroutines that may still
	// hold os.Stdin when switching from CLI to TUI mode.
	ttyInput, ttyErr := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	var p *tea.Program
	if ttyErr != nil {
		// Fallback to os.Stdin if /dev/tty is not available
		p = tea.NewProgram(&m)
	} else {
		p = tea.NewProgram(&m, tea.WithInput(ttyInput))
	}
	m.program = p

	finalModel, err := p.Run()
	if ttyInput != nil {
		ttyInput.Close()
	}
	if err != nil {
		restoreTerminalToNormal()
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		// Save session even on error
		saveSessionTUI(m)
		os.Exit(1)
	}

	// Get final model state
	switchToCLI := false
	if finalTUI, ok := finalModel.(TUI); ok {
		switchToCLI = finalTUI.switchToCLI
	} else if finalTUIPtr, ok := finalModel.(*TUI); ok {
		switchToCLI = finalTUIPtr.switchToCLI
	}

	// Save session on normal exit (except switching to CLI —
	// session already saved in handleCommand when processing /cli)
	if !switchToCLI {
		saveSessionTUI(m)
	}

	// Return true if need to switch to CLI
	// (on switch do not output history to terminal — CLI will show it)
	if switchToCLI {
		return true
	}

	// In auto-mode (AltScreen) — output chat history to terminal
	if !m.inline {
		renderSessionHistoryCLI(m.loop.Context.GetMessages())
	}

	return false
}

// handleDreamCommandTUI handles /dream command in TUI mode
func handleDreamCommandTUI(loop *agent.AgentLoop) string {
	cavibora, ok := loop.GetProvider().(*provider.CaviboraProvider)
	if !ok {
		return color.RedString("%s", i18n.T("cli.cavibora_only"))
	}

	result, err := cavibora.Dream(context.Background(), "")
	if err != nil {
		return color.RedString("🌙 %s", i18n.T("cli.dream_error", err.Error()))
	}

	output := color.GreenString("🌙 %s", i18n.T("cli.dream_result", result.Seed, len(result.Thoughts), result.NewBindings, result.Duration))
	if len(result.Thoughts) > 0 {
		output += "\n  💭 Thoughts:"
		for i, thought := range result.Thoughts {
			output += fmt.Sprintf("\n    %d. %s", i+1, thought)
		}
	}
	return output
}

// handleEmotionsCommandTUI handles /emotions command in TUI mode
func handleEmotionsCommandTUI(loop *agent.AgentLoop) string {
	cavibora, ok := loop.GetProvider().(*provider.CaviboraProvider)
	if !ok {
		return color.RedString("%s", i18n.T("cli.cavibora_only"))
	}

	result, err := cavibora.Emotions(context.Background())
	if err != nil {
		return color.RedString("💭 %s", i18n.T("cli.emotions_error", err.Error()))
	}

	return fmt.Sprintf("💭 %s %s %s\n  %s", result.Emoji, result.Emotion, result.Bar, result.Detail)
}

// listProvidersTUI lists available providers in TUI mode
func listProvidersTUI(cfg *config.BugBusterConfig, currentProvider string) string {
	var sb strings.Builder
	sb.WriteString(color.CyanString("📡 Available providers:\n"))
	for name, prov := range cfg.Providers {
		prefix := "  "
		if name == currentProvider {
			prefix = "→ "
		}
		sb.WriteString(fmt.Sprintf("%s%s (%s, %s)\n", prefix, color.HiYellowString(name), prov.Type, prov.Model))
	}
	sb.WriteString(fmt.Sprintf("\n%s /provider <name>\n", color.HiBlackString("Switch:")))
	return sb.String()
}

// switchProviderTUI switches provider in TUI mode
func switchProviderTUI(cfg *config.BugBusterConfig, providerName string, loop *agent.AgentLoop, currentProvider string) (provider.Provider, error) {
	provCfg, ok := cfg.Providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider not found: %s\nAvailable: %s", providerName, strings.Join(getProviderNames(cfg.Providers), ", "))
	}

	p, err := provider.NewFromConfig(providerName, provCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %v", err)
	}

	loop.SetProvider(p)
	return p, nil
}

// handleMeshStatsCommandTUI handles /mesh-stats command in TUI mode
func handleMeshStatsCommandTUI(loop *agent.AgentLoop) string {
	cavibora, ok := loop.GetProvider().(*provider.CaviboraProvider)
	if !ok {
		return color.RedString("%s", i18n.T("cli.cavibora_only"))
	}

	result, err := cavibora.MeshStats(context.Background())
	if err != nil {
		return color.RedString("🧠 %s", i18n.T("cli.mesh_stats_error", err.Error()))
	}

	return color.GreenString("🧠 %s", i18n.T("cli.mesh_stats_result",
		result.Cells, result.Bindings, result.Learnings,
		result.ModelName, result.Version, result.Uptime, result.Temperature))
}
// taskKeywordsCache caches loaded keywords from JSON
var taskKeywordsCache struct {
	once      sync.Once
	keywords  map[string]string // keyword -> task type
	typos     map[string]string // typo -> correction
	loadErr   error
}

// loadTaskKeywords loads keywords from JSON file with fallback to hardcoded defaults
func loadTaskKeywords() (map[string]string, map[string]string, error) {
	taskKeywordsCache.once.Do(func() {
		// Try to find keywords file in multiple locations
		locations := []string{
			filepath.Join(".", "configs", "task_keywords.json"),
			filepath.Join(".", "task_keywords.json"),
			filepath.Join("/etc/bugbuster", "task_keywords.json"),
		}

		// Also check executable directory
		if exe, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exe)
			locations = append(locations,
				filepath.Join(exeDir, "configs", "task_keywords.json"),
				filepath.Join(exeDir, "task_keywords.json"),
			)
		}

		// Also check home directory
		if home, err := os.UserHomeDir(); err == nil {
			locations = append(locations,
				filepath.Join(home, ".bugbuster", "task_keywords.json"),
			)
		}

		var data []byte
		var foundPath string
		for _, loc := range locations {
			if d, err := os.ReadFile(loc); err == nil {
				data = d
				foundPath = loc
				break
			}
		}

		if data == nil {
			// Fallback to hardcoded defaults
			taskKeywordsCache.keywords = getDefaultTaskKeywords()
			taskKeywordsCache.typos = getDefaultTypos()
			return
		}

		// Parse JSON
		var cfg struct {
			Thinking map[string]struct {
				Keywords []string `json:"keywords"`
				Task     string   `json:"task"`
			} `json:"thinking"`
			Fast map[string]struct {
				Keywords []string `json:"keywords"`
				Task     string   `json:"task"`
			} `json:"fast"`
			Typos map[string]string `json:"typos"`
		}

		if err := json.Unmarshal(data, &cfg); err != nil {
			taskKeywordsCache.keywords = getDefaultTaskKeywords()
			taskKeywordsCache.typos = getDefaultTypos()
			taskKeywordsCache.loadErr = fmt.Errorf("failed to parse %s: %v", foundPath, err)
			return
		}

		keywords := make(map[string]string)
		for _, cat := range cfg.Thinking {
			for _, kw := range cat.Keywords {
				keywords[kw] = cat.Task
			}
		}
		for _, cat := range cfg.Fast {
			for _, kw := range cat.Keywords {
				keywords[kw] = cat.Task
			}
		}

		taskKeywordsCache.keywords = keywords
		taskKeywordsCache.typos = cfg.Typos
	})

	return taskKeywordsCache.keywords, taskKeywordsCache.typos, taskKeywordsCache.loadErr
}

// normalizeInput normalizes user input: lowercase, fix typos, transliterate
func normalizeInput(input string) string {
	lower := strings.ToLower(input)

	// Load typos map
	_, typos, _ := loadTaskKeywords()

	// Fix common typos
	for typo, correction := range typos {
		if strings.Contains(lower, typo) {
			lower = strings.ReplaceAll(lower, typo, correction)
		}
	}

	// Transliterate common patterns: z -> ж, zh -> ж, sh -> ш, ch -> ч, etc.
	translit := map[string]string{
		"zhit": "жить", "zh": "ж", "sh": "ш", "ch": "ч", "sch": "щ",
		"ya": "я", "yu": "ю", "ts": "ц",
	}
	for from, to := range translit {
		lower = strings.ReplaceAll(lower, from, to)
	}

	return lower
}

// detectTaskType analyzes user input and returns the matching task type
// for auto-switching to the appropriate provider (thinking vs fast)
// Loads keywords from JSON file with fallback to hardcoded defaults
func detectTaskType(input string) string {
	lower := normalizeInput(input)

	keywords, _, _ := loadTaskKeywords()

	// Check keywords (thinking first, then fast — thinking has higher priority)
	// Keywords are already ordered by priority in the JSON file
	for kw, taskType := range keywords {
		if strings.Contains(lower, kw) {
			return taskType
		}
	}

	return "" // No auto-switch — use current provider
}

// getDefaultTaskKeywords returns hardcoded fallback keywords
func getDefaultTaskKeywords() map[string]string {
	return map[string]string{
		// Thinking keywords (analysis, architecture, debugging)
		"analyz": "analyze", "анализ": "analyze", "analiza": "analyze", "analyse": "analyze",
		"оцени": "review", "review": "review", "аудит": "review", "audit": "review",
		"архитект": "architect", "architect": "architect", "план": "architect", "plan": "architect",
		"дизайн": "design", "design": "design", "проектиров": "design",
		"дебаг": "debug", "debug": "debug", "баг": "debug", "bug": "debug", "ошибк": "debug", "error": "debug",
		"исследуй": "analyze", "investigat": "analyze", "изучи": "analyze",
		"оптимизи": "analyze", "optimiz": "analyze", "улучш": "analyze", "improv": "analyze",
		"рефактор": "analyze", "refactor": "analyze", "перепиши": "analyze", "rewrit": "analyze",
		"сравни": "analyze", "compar": "analyze",
		"уязвим": "review", "vulnerabilit": "review", "security": "review", "безопасн": "review",
		"сложн": "analyze", "complex": "analyze", "deep": "analyze", "глубок": "analyze",
		"подробн": "analyze", "detail": "analyze", "thorough": "analyze", "детальн": "analyze",
		"почему": "analyze", "why": "analyze", "как работает": "analyze", "how does": "analyze",
		"объясни": "analyze", "explain": "analyze", "расскажи": "analyze",
		"стратег": "architect", "strateg": "architect", "roadmap": "architect",
		"паттерн": "architect", "pattern": "architect", "микросервис": "architect",
		"производительн": "analyze", "performance": "analyze", "benchmark": "analyze",
		"миграц": "architect", "migrat": "architect",
		// Fast keywords (coding, writing, simple tasks)
		"напиши": "code", "write": "code", "создай": "code", "creat": "code", "добавь": "code", "add": "code",
		"исправь": "code", "fix": "code", "поменяй": "code", "chang": "code", "обнов": "code", "updat": "code",
		"удали": "code", "delet": "code", "remove": "code", "переименуй": "code", "renam": "code",
		"запусти": "code", "run": "code", "тест": "code", "test": "code",
		"покажи": "code", "show": "code", "выведи": "code", "print": "code", "список": "code", "list": "code",
		"коммент": "code", "comment": "code", "документ": "code", "doc": "code",
		"implement": "code", "реализуй": "code", "код": "code", "code": "code", "функци": "code", "function": "code",
		"метод": "code", "method": "code", "класс": "code", "class": "code", "module": "code", "модуль": "code",
		"переведи": "code", "translat": "code", "скопируй": "code", "copy": "code", "вставь": "code", "paste": "code",
		"форматируй": "code", "format": "code", "отступ": "code", "indent": "code",
	}
}

// getDefaultTypos returns hardcoded fallback typos
func getDefaultTypos() map[string]string {
	return map[string]string{
		"аналз": "анализ", "бэг": "баг", "дебуг": "дебаг",
		"оптимизац": "оптимизи", "рефакторинг": "рефактор",
		"архитектур": "архитект", "обясни": "объясни",
		"напишиь": "напиши", "исправть": "исправь",
	}
}
