package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"bugbuster-code/pkg/agent"
	"bugbuster-code/pkg/config"
	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/theme"
	"bugbuster-code/pkg/tools"

	"github.com/charmbracelet/x/term"
	"github.com/chzyer/readline"
	"github.com/fatih/color"
)

// lineResult — result of reading lines from readline
type lineResult struct {
	line string
	err  error
}

// SplitTerminal — interactive mode with split-terminal approach:
// - Upper scope: stdout (chat history, agent output)
// - Lower scope: readline (user input)
// - Native terminal scroll
// - No AltScreen/Bubble Tea
type SplitTerminal struct {
	cfg           *config.BugBusterConfig
	loop          *agent.AgentLoop
	changeTracker *ChangeTracker
	providerName  string

	// Session
	session    *agent.Session
	sessionMgr *agent.SessionManager

	// Readline
	rl *readline.Instance

	// Streaming
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex

	// ask_user channel
	askCh *tools.AskChannel

	// State
	streaming bool
	verbose   bool
	debug     bool
	autoMode  bool           // autopilot: automatically continue after each response
	autoState *AutoPilotState // state autopilot

	// Deferred readline result from paste detection.
	// On timeout goroutine is still blocked on rl.Readline() —
	// save channel to read result on next call
	// readMultilineInput. This ensures only one goroutine
	// calls rl.Readline() concurrently (no data race).
	pendingLine chan lineResult

	// Background process manager
	bgTool *tools.BackgroundTool
}

// ensureHistoryDir ensures .bugbuster/history is a directory.
// Old versions used .bugbuster/history as a single file —
// if it's a file, rename to .old and create directory.
func ensureHistoryDir(projectDir string) {
	historyPath := filepath.Join(projectDir, ".bugbuster", "history")
	info, err := os.Stat(historyPath)
	if err != nil {
		// Doesn't exist — create directory
		os.MkdirAll(historyPath, 0755)
		return
	}
	if info.IsDir() {
		return // Already a directory
	}
	// It's a file — rename to .old and create directory
	os.Rename(historyPath, historyPath+".old")
	os.MkdirAll(historyPath, 0755)
}

// NewSplitTerminal creates a new split-terminal mode
func NewSplitTerminal(cfg *config.BugBusterConfig, loop *agent.AgentLoop, ct *ChangeTracker, providerName string) *SplitTerminal {
	projectDir := getProjectDir(cfg)
	bgTool := tools.NewBackgroundTool(filepath.Join(projectDir, ".bugbuster", "bg_logs"))
	return &SplitTerminal{
		cfg:           cfg,
		loop:          loop,
		changeTracker: ct,
		providerName:  providerName,
		bgTool:        bgTool,
	}
}

// resetReadline recreates readline instance ONLY if it was closed
// (st.rl == nil). This happens after ask_user closes readline via rlClose.
// If readline is still active (st.rl != nil), we skip recreation to avoid
// "first Enter absorbed" issue — new readline instance reads one byte
// from stdin during initialization, which can eat the user's first Enter.
func (st *SplitTerminal) resetReadline() {
	if st.rl != nil {
		// Readline is still active — no need to recreate
		return
	}
	// Drain any pending readline result from previous instance
	if st.pendingLine != nil {
		select {
		case <-st.pendingLine:
		default:
		}
		st.pendingLine = nil
	}
	// Readline was closed (by rlClose in ask_user) — recreate it
	restoreTerminalToNormal()
	ensureHistoryDir(getProjectDir(st.cfg))
	historyFile := filepath.Join(getProjectDir(st.cfg), ".bugbuster", "history", st.loop.Context.SessionID)
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          color.HiGreenString("❯ "),
		HistoryFile:     historyFile,
		HistoryLimit:    1000,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		color.Red("Failed to reinitialize readline: %v", err)
		restoreTerminalToNormal()
		return
	}
	st.rl = rl
}

// Run starts interactive split-terminal mode.
// Returns true if need to switch to TUI mode.
func (st *SplitTerminal) Run() bool {
	// Initialize i18n
	lang := langFlag
	if lang == "" {
		lang = st.cfg.Agent.Language
	}
	if lang == "" {
		lang = "en"
	}
	if err := i18n.Init(lang); err != nil {
		i18n.Init("en")
	}

	// Initialize theme
	appTheme = theme.ResolveTheme(st.cfg.Theme)

	// Create provider
	p, err := createProvider(st.cfg)
	if err != nil {
		color.Red("%s", i18n.T("cli_error.provider_create", err))
		color.Yellow("%s", i18n.T("cli_success.config_init_hint"))
		os.Exit(1)
	}
	_ = p // provider is already in loop

	// Restore terminal to normal mode after TUI (bubbletea).
	restoreTerminalToNormal()

	// Banner
	printBanner(st.cfg, p)

	// Ensure history directory exists (old versions used file instead of dir)
	projectDir := getProjectDir(st.cfg)
	ensureHistoryDir(projectDir)

	// Readline with command history
	historyFile := filepath.Join(projectDir, ".bugbuster", "history", st.loop.Context.SessionID)
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          color.HiGreenString("❯ "),
		HistoryFile:     historyFile,
		HistoryLimit:    1000,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		color.Red("%s", i18n.T("cli_error.general", err))
		color.Yellow("Attempting terminal reset...")
		restoreTerminalToNormal()
		rl, err = readline.NewEx(&readline.Config{
			Prompt:          color.HiGreenString("❯ "),
			HistoryFile:     historyFile,
			HistoryLimit:    1000,
			InterruptPrompt: "^C",
			EOFPrompt:       "exit",
		})
		if err != nil {
			color.Red("Failed to initialize readline: %v", err)
			color.Yellow("Try running 'stty sane' in your terminal and restart.")
			return false
		}
	}
	st.rl = rl

	// Sessions
	sessionsDir := filepath.Join(getProjectDir(st.cfg), ".bugbuster", "sessions")
	sessionMgr := agent.NewSessionManager(sessionsDir)

	currentSession := restoreOrNewSession(sessionMgr, rl, st.loop, st.cfg)
	st.session = currentSession
	st.sessionMgr = sessionMgr
	st.loop.Context.SessionID = currentSession.ID

	// Set global session references for crash recovery
	globalSession = currentSession
	globalSessionMgr = sessionMgr
	globalLoop = st.loop
	if searchTool, ok := st.loop.Tools["search_context"].(*agent.SearchContextTool); ok {
		searchTool.SessionID = currentSession.ID
	}
	// Update SessionID in MemoryTool
	if memTool, ok := st.loop.Tools["memory"].(*tools.MemoryTool); ok {
		memTool.SetSessionIDForProject(currentSession.ID, getProjectDir(st.cfg))
	}
	color.Yellow("%s", i18n.T("cli_session.info", currentSession.ID))

	// Ctrl+C processing
	var currentCancel context.CancelFunc
	var interrupted bool
	var ctrlCCount int
	var lastCtrlC time.Time

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	go func() {
		for range sigCh {
			now := time.Now()
			st.mu.Lock()
			if st.streaming && currentCancel != nil {
				// Cancel streaming request
				color.Yellow("\n" + i18n.T("cli.cancel_request"))
				currentCancel()
				interrupted = true
				ctrlCCount = 0
				if st.askCh != nil {
					select {
					case st.askCh.Answer <- "":
					default:
					}
				}
			} else {
				// Not streaming — count Ctrl+C presses
				if now.Sub(lastCtrlC) < 2*time.Second {
					ctrlCCount++
				} else {
					ctrlCCount = 1
				}
				lastCtrlC = now
				if ctrlCCount >= 2 {
					// Double Ctrl+C — exit
					st.mu.Unlock()
					st.saveAndExit()
					color.Cyan("\n%s", i18n.T("cli.goodbye"))
					os.Exit(0)
				} else {
					// Single Ctrl+C — show hint
					color.Yellow("\n%s", i18n.T("cli.ctrl_c_hint"))
				}
			}
			st.mu.Unlock()
		}
	}()

	// Main input loop
	var ctrlCount int
	for {
		if rl == nil {
			// Readline failed to initialize — wait and retry
			st.resetReadline()
			rl = st.rl
			if rl == nil {
				color.Red("Failed to initialize readline. Press Enter to retry...")
				time.Sleep(1 * time.Second)
				continue
			}
		}
		input := st.readMultilineInput(rl)
		if input == "" {
			ctrlCount++
			if ctrlCount >= 2 {
				st.saveAndExit()
				rl.Close()
				color.Cyan("%s", i18n.T("cli.goodbye"))
				return false
			}
			continue
		}
		ctrlCount = 0

		// Slash command handling
		// Commands write to os.Stdout directly (fmt.Println, color.XXX).
		handled, _, newProvName := handleCommand(input, st.loop, st.cfg, p, st.changeTracker, st.rl, sessionMgr, currentSession, st.bgTool)
		if handled {
			// Update provider name if model/provider was switched
			if newProvName != "" {
				st.providerName = newProvName
			}
			continue
		}

		if input == "/exit" || input == "/quit" {
			st.saveAndExit()
			rl.Close()
			color.Cyan("%s", i18n.T("cli.goodbye"))
			return false
		}
		if input == "/tui" {
			st.autoMode = false
			st.saveAndExit()
			switchToTUICleanup(st.rl, st.pendingLine)
			st.rl = nil
			st.pendingLine = nil
			color.Cyan("Switching to TUI mode...")
			return true
		}
		if input == "/auto" || strings.HasPrefix(input, "/auto ") {
			// Parse /auto [N] — iteration limit
			maxIter := 0 // 0 = default
			if parts := strings.Fields(input); len(parts) == 2 {
				if _, err := fmt.Sscanf(parts[1], "%d", &maxIter); err != nil {
					maxIter = 0
				}
			}
			st.autoMode = !st.autoMode
			if st.autoMode {
				st.autoState = NewAutoPilotState(maxIter)
				st.autoState.Iteration = 0
				color.HiCyan("🤖 %s", i18n.T("cli.auto_enabled"))
				if st.autoState.MaxIterations != autoMaxIterations {
					color.HiCyan("   Max iterations: %d", st.autoState.MaxIterations)
				}
			} else {
				st.autoState = nil
				color.Yellow("🤖 %s", i18n.T("cli.auto_disabled"))
			}
			continue
		}
		// Regular request to model
		st.runStreamingQuery(input, &currentCancel, &interrupted)
		st.resetReadline()
		rl = st.rl

		// Autopilot: automatically continue after each response,
		// until plan is completed, iteration limit is reached,
		// or user interrupts (Ctrl+C)
		if st.autoMode {
			st.autoState.Iteration = 1 // first request already completed
		}
		for st.autoMode && !interrupted {
			// Check iteration limit
			if st.autoState.Iteration >= st.autoState.MaxIterations {
				st.autoMode = false
				color.Yellow("🤖 %s", i18n.T("cli.auto_max_iterations", st.autoState.MaxIterations))
				break
			}
			// Check plan completion
			lastMsg := getLastAssistantMessage(st.loop)
			if isPlanCompleted(lastMsg) {
				st.autoMode = false
				color.Green("✅ %s", i18n.T("cli.auto_plan_completed"))
				break
			}
			// Delay between iterations for rate limiting
			time.Sleep(autoDelayBetweenIterations)
			// Generate encouraging phrase and start
			phrase := randomContinuePhrase()
			st.autoState.Iteration++
			color.HiCyan("%s", formatAutoIteration(st.autoState.Iteration, st.autoState.MaxIterations, phrase))
			st.runStreamingQuery(phrase, &currentCancel, &interrupted)
		}
		if interrupted && st.autoMode {
			st.autoMode = false
			color.Yellow("🤖 %s", i18n.T("cli.auto_interrupted"))
		}
		// Ensure readline is recreated after autopilot loop
		// (runStreamingQuery closes readline each time)
		st.resetReadline()
		rl = st.rl
	}
}

// runStreamingQuery starts request to model with all context settings and AskChannel
func (st *SplitTerminal) runStreamingQuery(input string, currentCancel *context.CancelFunc, interrupted *bool) {
	// Use context with cancel for Ctrl+C support.
	// No timeout — idle timeout and thinking timeout in agent_stream.go
	// handle stalled connections. Hard timeout was removed because it
	// interrupted active streaming, burning provider tokens.
	ctx, cancel := context.WithCancel(context.Background())
	*currentCancel = cancel
	*interrupted = false

	askCh := &tools.AskChannel{
		Question: make(chan string, 1),
		Answer:   make(chan string, 1),
	}
	if askUserTool, ok := st.loop.Tools["ask_user"].(*tools.AskUserTool); ok {
		askUserTool.SetAskChannel(askCh)
	}
	st.askCh = askCh

	st.mu.Lock()
	st.streaming = true
	st.ctx = ctx
	st.cancel = cancel
	st.mu.Unlock()

	// CRITICAL: Close readline before streaming starts.
	// Readline leaves goroutines (CancelableStdin.ioloop, Operation.ioloop,
	// Terminal.ioloop) that intercept SIGINT (Ctrl+C). If readline is active
	// during streaming, Ctrl+C is swallowed by readline goroutines and the
	// signal handler never receives it — user cannot cancel the request.
	// Closing readline kills these goroutines, allowing SIGINT to reach
	// our signal handler which cancels the streaming context.
	if st.rl != nil {
		st.rl.Close()
		st.rl = nil
	}

	rlClose := func() {
		// Readline already closed above — nothing to do
	}
	rlRecreate := func() {
		st.resetReadline()
	}
	runQueryWithLoop(st.loop, input, st.cfg, st.providerName, ctx, cancel, askCh, st.session, st.sessionMgr, rlClose, rlRecreate)

	if askUserTool, ok := st.loop.Tools["ask_user"].(*tools.AskUserTool); ok {
		askUserTool.SetAskChannel(nil)
	}
	st.askCh = nil

	st.mu.Lock()
	st.streaming = false
	st.cancel = nil
	st.mu.Unlock()
	*currentCancel = nil

	if *interrupted {
		color.Yellow("%s", i18n.T("cli.context_reset"))
	}
}

// readMultilineInput reads input with multiline support.
//
// CRITICAL: When pasting multiline text, ALL lines must be collected into
// a SINGLE query. If lines are split into separate queries, the model
// processes them independently — it may try to "fix" things that were
// already fixed in earlier lines, corrupting the code.
//
// Paste detection strategy:
// 1. After reading first line, wait up to 200ms for more data
// 2. If more data arrives → it's a paste → collect ALL lines until gap > 200ms
// 3. Join all lines with \n and send as single query
// 4. Ctrl+C during paste: cancel paste and return empty string
func (st *SplitTerminal) readMultilineInput(rl *readline.Instance) string {
	prompt := color.HiGreenString("❯ ")
	continuation := color.HiGreenString("... ")

	rl.SetPrompt(prompt)

	var lines []string

	// Check if we have a pending line from previous paste detection
	if st.pendingLine != nil {
		select {
		case res := <-st.pendingLine:
			st.pendingLine = nil
			if res.err != nil {
				return ""
			}
			line := strings.TrimSpace(res.line)
			if line != "" {
				lines = append(lines, line)
			}
		default:
			// No pending line, clear it
			st.pendingLine = nil
		}
	}

	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				if len(lines) > 0 {
					// Ctrl+C during input — cancel everything
					fmt.Printf("\n  %s\n", i18n.T("cli.paste_cancelled"))
					lines = nil
					return ""
				}
				return ""
			}
			// EOF — return what we have
			result := strings.Join(lines, "\n")
			rl.SetPrompt(prompt)
			return result
		}

		line = strings.TrimSpace(line)
		if line == "" && len(lines) == 0 {
			continue
		}

		lines = append(lines, line)

		// Paste detection: after first line, wait for more data
		// Use longer timeout (200ms) to reliably detect paste operations
		if len(lines) == 1 {
			time.Sleep(100 * time.Millisecond)
			if !stdinHasData() {
				// Single line — not a paste
				// Check for continuation backslash
				if strings.HasSuffix(line, "\\") {
					lines[len(lines)-1] = strings.TrimSuffix(line, "\\")
					rl.SetPrompt(continuation)
					continue
				}
				result := strings.Join(lines, "\n")
				rl.SetPrompt(prompt)
				return result
			}
			// It's a paste — show indicator and collect all lines
			fmt.Printf("  %s\n", i18n.T("cli.paste_detected"))
		}

		// Collecting paste — read all remaining lines until gap > 200ms
		// This ensures ALL pasted lines are collected into ONE query
		for {
			time.Sleep(50 * time.Millisecond)
			if !stdinHasData() {
				// Wait a bit longer — large pastes may have small gaps
				time.Sleep(150 * time.Millisecond)
				if !stdinHasData() {
					break // No more data — paste complete
				}
			}
			nextLine, nextErr := rl.Readline()
			if nextErr != nil {
				if nextErr == readline.ErrInterrupt {
					// Ctrl+C during paste — cancel everything
					fmt.Printf("\n  %s\n", i18n.T("cli.paste_cancelled"))
					lines = nil
					return ""
				}
				break // EOF or other error
			}
			nextLine = strings.TrimSpace(nextLine)
			if nextLine != "" {
				lines = append(lines, nextLine)
			}
		}

		// Paste complete — join ALL lines into single query
		result := strings.Join(lines, "\n")
		rl.SetPrompt(prompt)
		fmt.Printf("  %s (%d %s)\n", i18n.T("cli.paste_complete"), len(lines), i18n.T("cli.paste_lines"))
		return result
	}
}

// stdinHasData checks if there is data available on stdin.
// This is used for paste detection — when multiple lines are pasted,
// they arrive very quickly and stdin will have more data available.
func stdinHasData() bool {
	fd := os.Stdin.Fd()
	if !term.IsTerminal(fd) {
		return false
	}
	// Use select() with zero timeout to check if stdin has data
	// This is non-blocking — it returns immediately
	// Platform-specific: macOS returns (error), Linux returns (int, error)
	var tv syscall.Timeval
	tv.Usec = 0 // zero timeout = poll
	var readFds syscall.FdSet
	readFds.Bits[0] = 1 << 0 // fd 0 = stdin
	ok, _ := stdinSelect(&tv, &readFds)
	return ok && readFds.Bits[0] != 0
}

// saveAndExit saves session and exits
func (st *SplitTerminal) saveAndExit() {
	if st.session != nil && st.sessionMgr != nil {
		st.session.Messages = st.loop.Context.GetMessages()
		if err := st.sessionMgr.SaveSessionMessages(st.session); err != nil {
			color.Red("%s", i18n.T("cli_error.session_save", err))
		} else {
			color.Green("%s", i18n.T("cli_success.session_saved", st.session.ID))
		}
	}
	if st.changeTracker != nil {
		changesFile := filepath.Join(getProjectDir(st.cfg), ".bugbuster", "changes", st.loop.Context.SessionID+".json")
		st.changeTracker.SaveToFile(changesFile)
	}
}

// restoreTerminalToNormal restores terminal to normal (cooked) mode.
func restoreTerminalToNormal() {
	fd := os.Stdin.Fd()
	if !term.IsTerminal(fd) {
		return
	}
	cmd := exec.Command("stty", "sane")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Run()
}

// drainStdin drains any pending data from stdin to prevent
// stale input from being read by the next mode (e.g. TUI).
// This is critical for CLI→TUI switching: readline leaves stdin
// in raw mode with pending data, which causes TUI to freeze
// until user presses Enter.
func drainStdin() {
	fd := os.Stdin.Fd()
	if !term.IsTerminal(fd) {
		return
	}

	// Step 1: Restore terminal to cooked mode first.
	// Readline leaves terminal in raw mode, which prevents
	// Bubble Tea from correctly initializing.
	restoreTerminalToNormal()

	// Step 2: Set stdin to non-blocking mode with minimal timeout.
	// This allows us to drain any pending bytes without blocking.
	cmd := exec.Command("stty", "-echo", "raw", "min", "0", "time", "1")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Run()

	// Step 3: Read and discard all pending data in a loop.
	// Readline may have left multiple bytes in the buffer.
	// We read in a loop until we get zero bytes or an error.
	buf := make([]byte, 4096)
	totalDrained := 0
	for {
		os.Stdin.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, err := os.Stdin.Read(buf)
		os.Stdin.SetReadDeadline(time.Time{})
		totalDrained += n
		if n == 0 || err != nil {
			break
		}
		// Safety limit: don't drain more than 1MB
		if totalDrained > 1024*1024 {
			break
		}
	}

	// Step 4: Restore terminal to normal mode again.
	// This ensures the terminal is in a clean state for TUI.
	restoreTerminalToNormal()

	// Step 5: Small delay to let terminal settle
	time.Sleep(50 * time.Millisecond)
}

// InjectUserComment injects user comment during streaming
func (st *SplitTerminal) InjectUserComment(comment string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.streaming && st.loop != nil {
		st.loop.InjectUserMessage(comment)
	}
}

// switchToTUICleanup performs all necessary cleanup when switching from CLI to TUI mode.
// The key problem: readline leaves goroutines (CancelableStdin.ioloop, Operation.ioloop,
// Terminal.ioloop) that hold stdin and prevent Bubble Tea from reading it.
// rl.Close() alone is not enough — these goroutines block on reads from os.Stdin.
//
// Solution:
// 1. Write a byte to StdinWriter to unblock Terminal.ioloop
// 2. Close readline
// 3. Replace os.Stdin fd 0 with /dev/null to kill all readline goroutines
// 4. Wait for goroutines to die
// 5. Restore os.Stdin fd 0 from /dev/tty for Bubble Tea
// 6. Drain any remaining data from stdin
func switchToTUICleanup(rl *readline.Instance, pendingLine chan lineResult) {
	if rl != nil {
		// Step 1: Write a newline to StdinWriter to unblock Terminal.ioloop.
		if rl.Config != nil && rl.Config.StdinWriter != nil {
			rl.Config.StdinWriter.Write([]byte("\n"))
		}

		// Step 2: Close readline
		rl.Close()
	}

	// Step 3: Drain any pending readline result
	if pendingLine != nil {
		select {
		case <-pendingLine:
		default:
		}
	}

	// Step 4: Replace fd 0 (stdin) with /dev/null.
	// This kills all goroutines that are blocking on reads from os.Stdin,
	// because /dev/null always returns EOF immediately.
	devNull, err := os.OpenFile("/dev/null", os.O_RDONLY, 0)
	if err == nil {
		dupFd(int(devNull.Fd()), 0)
		devNull.Close()
	}

	// Step 5: Wait for readline goroutines to die.
	// They will get EOF from /dev/null and exit.
	time.Sleep(200 * time.Millisecond)

	// Step 6: Restore terminal to normal mode
	restoreTerminalToNormal()

	// Step 7: Restore fd 0 (stdin) from /dev/tty.
	// This gives Bubble Tea a fresh stdin connected to the real terminal.
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err == nil {
		dupFd(int(tty.Fd()), 0)
		tty.Close()
	}

	// Step 8: Drain any remaining data from stdin
	drainStdin()

	// Step 9: Final delay to let terminal settle before TUI takes over
	time.Sleep(100 * time.Millisecond)
}