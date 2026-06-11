package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"bugbuster-code/pkg/agent"
	"bugbuster-code/pkg/config"
	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
	"bugbuster-code/pkg/tools"

	"github.com/fatih/color"
)

var codeLinePatterns = []string{
	"pub ", "fn ", "fn<", "func ", "var ", "const ", "type ",
	"impl ", "mod ", "use ", "import ", "class ", "def ", "return ",
	"self.", "super.", "crate::", "std::", "fmt::", "io::",
	"::new(", "::from(", "::default()", "string::", "vec<", "option<",
	"result<", "box<", "arc<", "rc<", "hashmap<",
}

var codePrefixes = []string{
	"pub ", "fn ", "func ", "var ", "const ",
	"type ", "impl ", "mod ", "use ", "import ", "class ", "def ", "return ",
	"if ", "else ", "for ", "while ", "match ", "case ", "switch ",
	"//", "/*", "*/", "#[", "@", "self.", "super.", "crate::",
	"std::", "fmt::", "io::", "string::", "vec<", "option<", "result<",
}

// providerDisplayName returns a human-readable provider name.
// Shows "local" for providers running on localhost/LAN, "cloud" for remote providers.
func providerDisplayName(providerName string, provCfg provider.ProviderConfig) string {
	baseURL := provCfg.GetBaseURL()
	isLocal := strings.Contains(baseURL, "localhost") ||
		strings.Contains(baseURL, "127.0.0.1") ||
		strings.Contains(baseURL, "0.0.0.0") ||
		strings.HasPrefix(baseURL, "http://192.168.") ||
		strings.HasPrefix(baseURL, "http://10.") ||
		strings.HasPrefix(baseURL, "https://192.168.") ||
		strings.HasPrefix(baseURL, "https://10.")

	if isLocal {
		return "local"
	}
	// For cloud providers, show "cloud" instead of protocol type (anthropic, openai)
	return "cloud"
}

// readLineFromStdin reads a line from stdin for ask_user responses.
//
// The problem: readline leaves background goroutines (CancelableStdin.ioloop,
// Operation.ioloop) that read from os.Stdin. When ask_user fires, these goroutines
// compete with us for stdin data, causing hangs and lost input.
//
// Solution: Use /dev/tty directly instead of os.Stdin. /dev/tty is a separate
// file descriptor connected to the user's terminal, bypassing readline's
// interception of os.Stdin (fd 0).
func readLineFromStdin() string {
	// Restore terminal to cooked mode so user can see their input.
	// This is critical: readline leaves terminal in raw mode where:
	// - Enter sends \r (0x0D) instead of \n (0x0A)
	// - Characters are not echoed (user can't see what they type)
	// - Backspace doesn't work properly
	restoreTerminalToNormal()

	// Open /dev/tty — this gives us a fresh fd to the terminal,
	// completely independent of readline's grip on fd 0 (os.Stdin).
	// This avoids competition with readline goroutines for stdin data.
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		// Fallback: read from stdin directly (cooked mode already restored)
		var buf [1]byte
		var line []byte
		for {
			n, err := os.Stdin.Read(buf[:])
			if err != nil || n == 0 {
				break
			}
			if buf[0] == '\n' || buf[0] == '\r' {
				fmt.Println()
				break
			}
			line = append(line, buf[0])
		}
		return string(line)
	}
	defer tty.Close()

	// Ensure /dev/tty is in cooked mode (it should be, but just in case)
	// This is a separate fd from os.Stdin, so readline's raw mode
	// on fd 0 doesn't affect it.
	sttyCmd := exec.Command("stty", "sane")
	sttyCmd.Stdin = tty
	sttyCmd.Stdout = tty
	sttyCmd.Run()

	// Write prompt directly to tty
	tty.Write([]byte("> "))

	// Read line from /dev/tty with timeout protection.
	// If user doesn't respond within 5 minutes, return empty string
	// to prevent permanent hang.
	done := make(chan string, 1)
	go func() {
		var buf [1]byte
		var line []byte
		for {
			n, err := tty.Read(buf[:])
			if err != nil || n == 0 {
				break
			}
			if buf[0] == '\n' || buf[0] == '\r' {
				tty.Write([]byte("\n"))
				break
			}
			// Handle backspace in cooked mode
			if buf[0] == 127 || buf[0] == 8 {
				if len(line) > 0 {
					line = line[:len(line)-1]
				}
				continue
			}
			// Handle Ctrl+C
			if buf[0] == 3 {
				line = nil
				break
			}
			line = append(line, buf[0])
		}
		done <- string(line)
	}()

	select {
	case answer := <-done:
		return answer
	case <-time.After(5 * time.Minute):
		tty.Write([]byte("\n(timeout)\n"))
		return ""
	}
}

func isCodeLikeLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if utf8.RuneCountInString(trimmed) <= 3 {
		return true
	}
	codeSuffixes := []string{";", "}", "{", ")", "]", ",", ":"}
	for _, s := range codeSuffixes {
		if strings.HasSuffix(trimmed, s) {
			return true
		}
	}
	lower := strings.ToLower(trimmed)
	for _, p := range codePrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	if strings.HasPrefix(lower, "let ") {
		afterLet := lower[4:]
		if strings.HasPrefix(afterLet, "me ") || strings.HasPrefix(afterLet, "us ") ||
			strings.HasPrefix(afterLet, "him ") || strings.HasPrefix(afterLet, "her ") ||
			strings.HasPrefix(afterLet, "them ") || strings.HasPrefix(afterLet, "'s ") {
		} else {
			return true
		}
	}
	for _, p := range codeLinePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	specialCount := 0
	for _, ch := range trimmed {
		if ch == '{' || ch == '}' || ch == '(' || ch == ')' ||
			ch == '<' || ch == '>' || ch == ':' || ch == '.' ||
			ch == '=' || ch == '&' || ch == '|' || ch == '\\' {
			specialCount++
		}
	}
	runes := utf8.RuneCountInString(trimmed)
	if runes > 0 && float64(specialCount)/float64(runes) > 0.4 {
		return true
	}
	return false
}

func summarizeThinking(thinkingText string) string {
	lines := strings.Split(strings.TrimSpace(thinkingText), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if !isCodeLikeLine(line) {
			if len(line) > 80 {
				line = line[:77] + "..."
			}
			return line
		}
	}
	return ""
}

// runQueryWithLoop — request with existing loop (with streaming)
func runQueryWithLoop(loop *agent.AgentLoop, query string, cfg *config.BugBusterConfig, providerName string, ctx context.Context, cancel context.CancelFunc, askCh *tools.AskChannel, session *agent.Session, sessionMgr *agent.SessionManager, rlClose func(), rlRecreate func()) {
	ch, err := loop.StreamWithCancel(ctx, query)
	if err != nil {
		result, err := loop.Run(query)
		if err != nil {
			color.Red("%s", i18n.T("cli_error.general", err))
			return
		}
		fmt.Println(result)
		return
	}

	var (
		totalInTokens   int
		totalOutTokens  int
		totalDuration   time.Duration
		genStart        time.Time // when first output token was received
		genEnd          time.Time // when last output token was received
		totalGenDur     time.Duration // accumulated generation time
		spinner         *Spinner
		textReceived    bool
		thinkingStarted bool
		thinkingActive  bool
		thinkingBuf     strings.Builder
		thinkingSummary string
		toolInputBuf    strings.Builder
		currentToolName string
		mdRenderer      = NewGlamourRenderer()
	)

	fmt.Println(FormatSeparator())

	spinner = NewSpinner(i18n.T("cli.spinner_thinking"))
	provCfg := cfg.Providers[providerName]
	spinner.UpdateProvider(providerDisplayName(providerName, provCfg), provCfg.Model)
	spinner.Start()

	var askQuestion chan string
	var askAnswer chan string
	if askCh != nil {
		askQuestion = askCh.Question
		askAnswer = askCh.Answer
	}

	// Auto-save session every 30 seconds
	autoSaveTicker := time.NewTicker(30 * time.Second)
	defer autoSaveTicker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-autoSaveTicker.C:
				if session != nil && sessionMgr != nil {
					session.Messages = loop.Context.GetMessages()
					sessionMgr.SaveSessionMessages(session)
				}
			}
		}
	}()

streamLoop:
	for {
		select {
		case <-ctx.Done():
			spinner = stopActiveSpinner(spinner)
			fmt.Println()
			color.Yellow("%s", i18n.T("cli.cancel_request"))
			if askAnswer != nil {
				select {
				case askAnswer <- "":
				default:
				}
			}
			break streamLoop

		case question := <-askQuestion:
			oldSpinner := spinner
			spinner = stopActiveSpinner(spinner)
			fmt.Print(mdRenderer.Flush())
			fmt.Printf("\n❓ %s\n> ", question)
			if rlClose != nil {
				rlClose()
			}
			answer := readLineFromStdin()
			answer = strings.TrimSpace(answer)
			select {
			case askAnswer <- answer:
			default:
			}
			if rlRecreate != nil {
				rlRecreate()
			}
			spinner = NewSpinner(i18n.T("cli.spinner_thinking"))
			oldSpinner.CopyStatsTo(spinner)
			spinner.UpdateProvider(providerDisplayName(providerName, provCfg), provCfg.Model)
			spinner.Start()

		case event, ok := <-ch:
			if !ok {
				break streamLoop
			}

			switch event.Type {

		case provider.EventIterationStart:
			// Set genEnd to now so gap between iterations is not counted as generation time
			// (but don't reset genEnd to zero — that would lose accumulated time)
			if !genEnd.IsZero() {
				genEnd = time.Now()
			}
			if spinner != nil {
				// Don't pause gen time — just update genEnd in spinner
				// This preserves totalGenDur from previous iterations
				spinner.mu.Lock()
				if !spinner.genEnd.IsZero() {
					spinner.genEnd = time.Now()
				}
				// Update totalGenDur from local variable so speed is shown immediately
				// (spinner's totalGenDur may be stale if UpdateGenTime wasn't called recently)
				spinner.totalGenDur = totalGenDur
				spinner.mu.Unlock()
				// Update tokens from accumulated data so speed/tokens are shown
				// on first step (before any EventTextDelta/EventToolCallDelta)
				if totalInTokens > 0 || totalOutTokens > 0 {
					spinner.UpdateTokens(totalInTokens, totalOutTokens)
				}
			}
			if !textReceived && spinner != nil && spinner.IsActive() {
				spinner.UpdateMessage(fmt.Sprintf(i18n.T("cli.spinner_step"), event.Iteration))
			}

			case provider.EventThinking:
				// Reset genEnd so thinking time is not counted as generation time
				// Thinking tokens are separate from output tokens
				genEnd = time.Time{}
				if spinner != nil {
					spinner.PauseGenTime()
				}
				if !thinkingStarted {
					oldSpinner := spinner
					spinner = stopActiveSpinner(spinner)
					fmt.Printf("\n  %s∴ %s%s\n", appTheme.Dim.ANSICode(), i18n.T("cli.thinking"), ansiReset)
					thinkingStarted = true
					spinner = NewSpinner(i18n.T("cli.thinking"))
					oldSpinner.CopyStatsTo(spinner)
					spinner.UpdateProvider(providerDisplayName(providerName, provCfg), provCfg.Model)
					spinner.Start()
				}
				thinkingActive = true
				thinkingBuf.WriteString(event.Text)
				thinkingSummary = summarizeThinking(thinkingBuf.String())
				if spinner != nil && spinner.IsActive() {
					spinner.UpdateMessage(thinkingSummary)
				}

			case provider.EventTextDelta:
				// Track generation time for accurate speed calculation
				if genStart.IsZero() {
					genStart = time.Now()
				} else if !genEnd.IsZero() {
					// Accumulate generation time between tokens
					// Skip if genEnd was reset (after tool execution)
					totalGenDur += time.Since(genEnd)
				}
				genEnd = time.Now()
				if spinner != nil {
					spinner.UpdateGenTime()
					// Increment token count for accurate speed display
					// EventUsage may come infrequently, so we count text deltas
					totalOutTokens++
					spinner.UpdateTokens(totalInTokens, totalOutTokens)
				}
				if thinkingActive {
					spinner = stopActiveSpinner(spinner)
					thinkingText := thinkingBuf.String()
					thinkingBuf.Reset()
					if thinkingText != "" {
						wrapped := wrapText(strings.TrimSpace(thinkingText), 4, 80)
						fmt.Printf("%s%s%s\n", appTheme.Dim.ANSICode(), wrapped, ansiReset)
					}
					thinkingActive = false
					fmt.Println()
				}
				thinkingSummary = ""
				textReceived = true
				mdRenderer.Render(event.Text)

			case provider.EventToolCallStart:
				// Reset genEnd so gap before tool call is not counted as generation time
				// (model may have been thinking, which is not output generation)
				genEnd = time.Time{}
				if spinner != nil {
					spinner.PauseGenTime() // don't count tool execution time as generation time
				}
				oldSpinner := spinner
				spinner = stopActiveSpinner(spinner)
				if thinkingActive {
					thinkingText := thinkingBuf.String()
					thinkingBuf.Reset()
					if thinkingText != "" {
						wrapped := wrapText(strings.TrimSpace(thinkingText), 4, 80)
						fmt.Printf("%s%s%s\n", appTheme.Dim.ANSICode(), wrapped, ansiReset)
					}
					thinkingActive = false
				}
				thinkingSummary = ""
				fmt.Print(mdRenderer.Flush())
				textReceived = true
				toolInputBuf.Reset()
				currentToolName = event.ToolName
				spinner = NewSpinner(fmt.Sprintf("⏺ %s", event.ToolName))
				oldSpinner.CopyStatsTo(spinner)
				spinner.UpdateProvider(providerDisplayName(providerName, provCfg), provCfg.Model)
				spinner.Start()

			case provider.EventToolCallDelta:
				// Tool call tokens are also generation — count them and track time
				if genStart.IsZero() {
					genStart = time.Now()
				}
				if !genEnd.IsZero() {
					totalGenDur += time.Since(genEnd)
				}
				genEnd = time.Now()
				totalOutTokens++
				if spinner != nil {
					spinner.UpdateGenTime()
					spinner.UpdateTokens(totalInTokens, totalOutTokens)
				}
				toolInputBuf.WriteString(event.ToolDelta)
				if spinner != nil && spinner.IsActive() {
					params := parsePartialToolInput(toolInputBuf.String())
					if len(params) > 0 {
						summary := formatToolSummary(currentToolName, params)
						width := terminalWidth()
						maxLen := width - 10
						if maxLen < 40 {
							maxLen = 40
						}
						runes := []rune(summary)
						if len(runes) > maxLen {
							summary = string(runes[:maxLen-3]) + "..."
						}
						spinner.UpdateMessage(fmt.Sprintf("⏺ %s", summary))
					}
				}

			case provider.EventToolCallEnd:
				// Reset genEnd so tool execution time is not counted
				genEnd = time.Time{}
				if spinner != nil {
					spinner.PauseGenTime() // don't count tool execution time as generation time
				}
				oldSpinner := spinner
				spinner = stopActiveSpinner(spinner)
				fmt.Printf("\r%s\r", ansiClear)
				if thinkingActive {
					thinkingText := thinkingBuf.String()
					thinkingBuf.Reset()
					if thinkingText != "" {
						wrapped := wrapText(strings.TrimSpace(thinkingText), 4, 80)
						fmt.Printf("%s%s%s\n", appTheme.Dim.ANSICode(), wrapped, ansiReset)
					}
					thinkingActive = false
				}
				toolEndParams := make(map[string]string)
				if event.ToolInput != nil {
					for k, v := range event.ToolInput {
						toolEndParams[k] = fmt.Sprintf("%v", v)
					}
				}
				fmt.Println()
				fmt.Println(FormatToolCallStart(event.ToolName, toolEndParams))
				fmt.Println(FormatToolCallEnd(event.ToolName, event.ToolOK, event.ToolResult, event.ToolFullResult, event.Duration, toolEndParams))
				if event.ToolName == "todo_write" && event.ToolOK {
					fmt.Println(FormatTodoChecklist(event.ToolFullResult))
				}
				toolInputBuf.Reset()
				currentToolName = ""
				textReceived = false
				spinner = NewSpinner(fmt.Sprintf(i18n.T("cli.spinner_step"), event.Iteration))
				oldSpinner.CopyStatsTo(spinner)
				spinner.UpdateProvider(providerDisplayName(providerName, provCfg), provCfg.Model)
				spinner.Start()
				// Auto-save session after each tool call
				if session != nil && sessionMgr != nil {
					session.Messages = loop.Context.GetMessages()
					sessionMgr.SaveSessionMessages(session)
				}

			case provider.EventUsage:
				if event.InputTokens > totalInTokens {
					totalInTokens = event.InputTokens
				}
				if event.OutputTokens > totalOutTokens {
					totalOutTokens = event.OutputTokens
				}
				if spinner != nil && spinner.IsActive() {
					spinner.UpdateTokens(totalInTokens, totalOutTokens)
				}
				if event.ToolMessage != "" {
					msg := event.ToolMessage
					if idx := strings.Index(msg, "\n"); idx >= 0 {
						msg = msg[:idx]
					}
					width := terminalWidth()
					maxLen := width - 6
					if maxLen < 40 {
						maxLen = 40
					}
					runes := []rune(msg)
					if len(runes) > maxLen {
						msg = string(runes[:maxLen-3]) + "..."
					}
					if spinner != nil && spinner.IsActive() {
						spinner.UpdateMessage(fmt.Sprintf("⏺ %s │ %s", currentToolName, msg))
					}
				}

			case provider.EventUserInjected:
				color.HiYellow("  💬 %s", event.Text)
				color.Yellow("  ↳ comment added to context")

			case provider.EventCompaction:
				oldSpinner := spinner
				spinner = stopActiveSpinner(spinner)
				spinner = NewSpinner(i18n.T("cli.compacting"))
				oldSpinner.CopyStatsTo(spinner)
				spinner.UpdateProvider(providerDisplayName(providerName, provCfg), provCfg.Model)
				spinner.Start()

			case provider.EventCompactionDone:
				oldSpinner := spinner
				spinner = stopActiveSpinner(spinner)
				spinner = NewSpinner(i18n.T("cli.spinner_thinking"))
				oldSpinner.CopyStatsTo(spinner)
				spinner.UpdateProvider(providerDisplayName(providerName, provCfg), provCfg.Model)
				spinner.Start()
				// Auto-save session after compaction
				if session != nil && sessionMgr != nil {
					session.Messages = loop.Context.GetMessages()
					sessionMgr.SaveSessionMessages(session)
				}

			case provider.EventThinkingTimeout:
				spinner = stopActiveSpinner(spinner)
				mins := int(event.Duration.Minutes())
				if mins < 1 {
					mins = 1
				}
				color.Yellow("\n  %s", i18n.T("cli.thinking_timeout_warn", fmt.Sprintf("%d", mins)))



			case provider.EventIterationEnd:
				// iteration completed

			case provider.EventDone:
				spinner = stopActiveSpinner(spinner)
				if thinkingActive {
					thinkingText := thinkingBuf.String()
					thinkingBuf.Reset()
					if thinkingText != "" {
						wrapped := wrapText(strings.TrimSpace(thinkingText), 4, 80)
						fmt.Printf("%s%s%s\n", appTheme.Dim.ANSICode(), wrapped, ansiReset)
					}
					thinkingActive = false
				}
				flushed := mdRenderer.Flush()
				if flushed != "" {
					fmt.Print(flushed)
				}
				fmt.Println()
				fmt.Println()
				totalDuration = event.Duration
				provCfg := cfg.Providers[providerName]
				genDur := totalGenDur
				statusLine := FormatStatusLineEx(
					totalInTokens, totalOutTokens,
					totalDuration, genDur,
					loop.Context.TokenCount(), loop.Context.MaxTokens,
					providerDisplayName(providerName, provCfg), provCfg.Model,
				)
				if statusLine != "" {
					fmt.Println(statusLine)
				}
				// Incremental session save after each response
				if session != nil && sessionMgr != nil {
					session.Messages = loop.Context.GetMessages()
					if err := sessionMgr.SaveSessionMessages(session); err != nil {
						color.Red("%s", i18n.T("cli_error.session_save", err))
					}
				}

			case provider.EventError:
				spinner = stopActiveSpinner(spinner)
				color.Red("%s", i18n.T("cli_error.stream", event.Error))
				break streamLoop
			}
		}
	}
}
