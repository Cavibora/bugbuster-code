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
	autoMode  bool            // autopilot: automatically continue after each response
	autoState *AutoPilotState  // state autopilot

	// Deferred readline result from paste detection.
	// On timeout goroutine is still blocked on rl.Readline() —
	// save channel to read result on next call
	// readMultilineInput. This ensures only one goroutine
	// calls rl.Readline() concurrently (no data race).
	pendingLine chan lineResult
}

// NewSplitTerminal creates a new split-terminal mode
func NewSplitTerminal(cfg *config.BugBusterConfig, loop *agent.AgentLoop, ct *ChangeTracker, providerName string) *SplitTerminal {
	return &SplitTerminal{
		cfg:           cfg,
		loop:          loop,
		changeTracker: ct,
		providerName:  providerName,
	}
}

// resetReadline recreates readline instance to ensure clean state.
// This is necessary because readLineFromStdin() (for ask_user) opens /dev/tty
// and puts terminal in cooked mode, which can leave readline goroutines
// (CancelableStdin.ioloop, Operation.ioloop) in a broken state.
// Recreating readline ensures all goroutines are fresh and terminal is in raw mode.
func (st *SplitTerminal) resetReadline() {
	if st.rl != nil {
		st.rl.Close()
	}
	historyFile := filepath.Join(getProjectDir(st.cfg), ".bugbuster", "history")
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
	// Clear pending line from previous readline instance
	st.pendingLine = nil
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

	// Readline with command history
	historyFile := filepath.Join(getProjectDir(st.cfg), ".bugbuster", "history")
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
		memTool.SetSessionID(currentSession.ID)
	}
	color.Yellow("%s", i18n.T("cli_session.info", currentSession.ID))

	// Ctrl+C processing
	var currentCancel context.CancelFunc
	var interrupted bool

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	go func() {
		for range sigCh {
			st.mu.Lock()
			if st.streaming && currentCancel != nil {
				color.Yellow("\n" + i18n.T("cli.cancel_request"))
				currentCancel()
				interrupted = true
				if st.askCh != nil {
					select {
					case st.askCh.Answer <- "":
					default:
					}
				}
			}
			st.mu.Unlock()
		}
	}()

	// Main input loop
	var ctrlCount int
	for {
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
		handled := handleCommand(input, st.loop, st.cfg, p, st.changeTracker, rl, sessionMgr, currentSession)
		if !handled {
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

			// Recreate readline after each request to ensure clean state.
			// readLineFromStdin() (for ask_user) opens /dev/tty and puts
			// terminal in cooked mode, which can leave readline goroutines
			// in a broken state. Recreating readline ensures all goroutines
			// are fresh and terminal is in raw mode.
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
		}
	}
}

// runStreamingQuery starts request to model with all context settings and AskChannel
func (st *SplitTerminal) runStreamingQuery(input string, currentCancel *context.CancelFunc, interrupted *bool) {
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

	runQueryWithLoop(st.loop, input, st.cfg, st.providerName, ctx, askCh, st.session, st.sessionMgr)

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
func (st *SplitTerminal) readMultilineInput(rl *readline.Instance) string {
	var lines []string
	prompt := color.HiGreenString("❯ ")
	continuation := color.HiGreenString("... ")

	rl.SetPrompt(prompt)

	for {
		var line string
		var err error

		if st.pendingLine != nil {
			select {
			case res := <-st.pendingLine:
				st.pendingLine = nil
				line, err = res.line, res.err
			case <-time.After(30 * time.Second):
				// Timeout — readline goroutine is stuck, discard it
				st.pendingLine = nil
				line, err = "", fmt.Errorf("readline timeout")
			}
		} else {
			line, err = rl.Readline()
		}

		if err != nil {
			if err == readline.ErrInterrupt {
				if len(lines) > 0 {
					lines = nil
					fmt.Println()
					continue
				}
				return ""
			}
			return ""
		}

		line = strings.TrimSpace(line)
		if line == "" && len(lines) == 0 {
			continue
		}

		lines = append(lines, line)

		if strings.HasSuffix(line, "\\") {
			lines[len(lines)-1] = strings.TrimSuffix(line, "\\")
			rl.SetPrompt(continuation)
			continue
		}

		pasting := false
		for {
			ch := make(chan lineResult, 1)
			go func() {
				l, e := rl.Readline()
				ch <- lineResult{l, e}
			}()
			select {
			case res := <-ch:
				if res.err != nil {
					goto done
				}
				pasting = true
				lines = append(lines, strings.TrimSpace(res.line))
			case <-time.After(100 * time.Millisecond):
				st.pendingLine = ch
				goto done
			}
		}

	done:
		if pasting {
			lineCount := len(lines)
			if lineCount > 1 {
				fmt.Printf("\n  📋 +%d %s\n", lineCount, i18n.T("cli.paste_lines"))
			}
		}

		result := strings.Join(lines, "\n")
		rl.SetPrompt(prompt)
		return result
	}
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
		changesFile := filepath.Join(getProjectDir(st.cfg), ".bugbuster", "changes.json")
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
		syscall.Dup2(int(devNull.Fd()), 0)
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
		syscall.Dup2(int(tty.Fd()), 0)
		tty.Close()
	}

	// Step 8: Drain any remaining data from stdin
	drainStdin()

	// Step 9: Final delay to let terminal settle before TUI takes over
	time.Sleep(100 * time.Millisecond)
}