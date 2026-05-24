package main

import (
	"context"
	"fmt"
	"os"
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

// readLineFromStdin reads a line from stdin, handling both \n and \r line endings.
// This is necessary because readline leaves stdin in raw mode where Enter sends \r (0x0D),
// not \n (0x0A). bufio.Reader.ReadString('\n') would hang forever in raw mode.
func readLineFromStdin() string {
	var buf [1]byte
	var line []byte
	for {
		n, err := os.Stdin.Read(buf[:])
		if err != nil || n == 0 {
			break
		}
		switch buf[0] {
		case '\n', '\r':
			if len(line) > 0 {
				return string(line)
			}
			return ""
		default:
			line = append(line, buf[0])
		}
	}
	return string(line)
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
func runQueryWithLoop(loop *agent.AgentLoop, query string, cfg *config.BugBusterConfig, providerName string, ctx context.Context, askCh *tools.AskChannel, session *agent.Session, sessionMgr *agent.SessionManager) {
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
	spinner.Start()

	var askQuestion chan string
	var askAnswer chan string
	if askCh != nil {
		askQuestion = askCh.Question
		askAnswer = askCh.Answer
	}

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
			spinner = stopActiveSpinner(spinner)
			fmt.Print(mdRenderer.Flush())
			fmt.Printf("\n❓ %s\n> ", question)
			answer := readLineFromStdin()
			answer = strings.TrimSpace(answer)
			// Non-blocking send: if tool is no longer waiting, don't hang
			select {
			case askAnswer <- answer:
			default:
			}
			spinner = NewSpinner(i18n.T("cli.spinner_thinking"))
			spinner.Start()

		case event, ok := <-ch:
			if !ok {
				break streamLoop
			}

			switch event.Type {

			case provider.EventIterationStart:
				if !textReceived && spinner != nil && spinner.IsActive() {
					spinner.UpdateMessage(fmt.Sprintf(i18n.T("cli.spinner_step"), event.Iteration))
				}

			case provider.EventThinking:
				if !thinkingStarted {
					spinner = stopActiveSpinner(spinner)
					fmt.Printf("\n  %s∴ %s%s\n", appTheme.Dim.ANSICode(), i18n.T("cli.thinking"), ansiReset)
					thinkingStarted = true
					spinner = NewSpinner(i18n.T("cli.thinking"))
					spinner.Start()
				}
				thinkingActive = true
				thinkingBuf.WriteString(event.Text)
				thinkingSummary = summarizeThinking(thinkingBuf.String())
				if spinner != nil && spinner.IsActive() {
					spinner.UpdateMessage(thinkingSummary)
				}

			case provider.EventTextDelta:
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
				spinner.Start()

			case provider.EventToolCallDelta:
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
				spinner.Start()

			case provider.EventUsage:
				totalInTokens = event.InputTokens
				totalOutTokens = event.OutputTokens
				if spinner != nil && spinner.IsActive() {
					spinner.UpdateTokens(totalInTokens, totalOutTokens)
				}

			case provider.EventToolProgress:
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
				spinner = stopActiveSpinner(spinner)
				spinner = NewSpinner(i18n.T("cli.compacting"))
				spinner.Start()

			case provider.EventCompactionDone:
				spinner = stopActiveSpinner(spinner)
				spinner = NewSpinner(i18n.T("cli.spinner_thinking"))
				spinner.Start()

			case provider.EventThinkingTimeout:
				spinner = stopActiveSpinner(spinner)
				minutes := event.Duration.Minutes()
				color.Yellow("\n  %s", i18n.T("cli.thinking_timeout_warn", minutes))

			case provider.EventRequestTimeout:
				spinner = stopActiveSpinner(spinner)
				minutes := event.Duration.Minutes()
				color.Red("\n  %s", i18n.T("cli.request_timeout_warn", minutes))

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
				provCfg := cfg.Providers[cfg.DefaultProvider]
				statusLine := FormatStatusLine(
					totalInTokens, totalOutTokens,
					totalDuration,
					loop.Context.TokenCount(), loop.Context.MaxTokens,
					providerName, provCfg.Model,
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
			}
		}
	}
}
