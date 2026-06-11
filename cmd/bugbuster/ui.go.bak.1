package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/theme"
	"bugbuster-code/pkg/tools"

	"charm.land/glamour/v2"
	"github.com/alecthomas/chroma/v2/quick"
	"github.com/fatih/color"
	"golang.org/x/term"
)

// ANSI escape codes
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiItalic = "\033[3m"
	ansiClear  = "\033[2K"
)

// appTheme — global app theme (initialized at startup)
var appTheme *theme.ResolvedTheme

// ─── Spinner ────────────────────────────────────────────────────────────────

// Spinner — animated spinner with synchronous stop and buffered output.
// While spinner is active, use spinner.Println/Printf to buffer output.
// Do NOT use fmt.Println/fmt.Printf directly — they will corrupt the display.
type Spinner struct {
	frames       []rune
	stopCh       chan struct{}
	doneCh       chan struct{}
	message      string
	mu           sync.Mutex
	active       bool
	startTime    time.Time
	tokensIn     int
	tokensOut    int
	providerName string
	modelName    string
	genStart     time.Time // when first output token was received
	genEnd       time.Time // when last output token was received
	totalGenDur  time.Duration // accumulated generation time

	// Buffered output: accumulated while spinner is active.
	// Printed atomically when spinner stops.
	pendingLines []string
	outputMu     sync.Mutex
}

var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

func terminalWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

func truncateMessage(msg string, maxWidth int) string {
	// Reserve space for: duration + tokens + speed + provider/model
	// "1.2m · ⬆ 99999 ⬇ 99999 Σ 99999 · ⚡ 999tok/s · local · model-name"
	reserve := 80
	available := maxWidth - reserve
	if available < 20 {
		available = 20
	}
	if len(msg) <= available {
		return msg
	}
	runes := []rune(msg)
	if len(runes) > available {
		if available > 3 {
			return string(runes[:available-3]) + "..."
		}
		return string(runes[:available])
	}
	return msg
}

func NewSpinner(msg string) *Spinner {
	return &Spinner{
		frames:    spinnerFrames,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
		message:   msg,
		startTime: time.Now(),
	}
}

// Start starts spinner in goroutine.
// Uses ONLY \r\033[2K to clear and rewrite current line.
// Does NOT use \033[A or \n — this prevents line multiplication.
func (s *Spinner) Start() {
	s.mu.Lock()
	s.active = true
	s.startTime = time.Now()
	s.pendingLines = nil
	s.mu.Unlock()

	go func() {
		defer close(s.doneCh)
		i := 0
		for {
			select {
			case <-s.stopCh:
				return
			default:
				s.mu.Lock()
				msg := s.message
				elapsed := time.Since(s.startTime)
				tokensIn := s.tokensIn
				tokensOut := s.tokensOut
				s.mu.Unlock()

				width := terminalWidth()
				msg = truncateMessage(msg, width)

				parts := []string{msg}
				parts = append(parts, FormatDuration(elapsed))
				// Always show tokens if available (even if only input tokens)
				if tokensIn > 0 || tokensOut > 0 {
					parts = append(parts, FormatTokens(tokensIn, tokensOut))
				}
				// Speed: use totalGenDur (accumulated generation time)
				// Show speed as soon as we have any generation time
				s.mu.Lock()
				totalGD := s.totalGenDur
				providerName := s.providerName
				modelName := s.modelName
				s.mu.Unlock()
				if tokensOut > 0 && totalGD.Seconds() > 0 {
					speed := float64(tokensOut) / totalGD.Seconds()
					// Realistic range: 0.5-500 tok/s
					if speed >= 0.5 && speed <= 500 {
						parts = append(parts, fmt.Sprintf("%s⚡%s %s%s%s", appTheme.Success.ANSICode(), ansiReset, appTheme.Info.ANSICode(), formatSpeed(speed), ansiReset))
					}
				}
				// Provider and model
				if providerName != "" || modelName != "" {
					var info []string
					if providerName != "" {
						info = append(info, providerName)
					}
					if modelName != "" {
						info = append(info, modelName)
					}
					parts = append(parts, fmt.Sprintf("%s%s%s", appTheme.Dim.ANSICode(), strings.Join(info, " · "), ansiReset))
				}
				display := strings.Join(parts, appTheme.Dim.ANSICode()+" · "+ansiReset)

				// Clear current line and write spinner frame
				fmt.Fprintf(cmdOutput, "\r\033[2K  %s%s%s %s", appTheme.Primary.ANSICode(), string(s.frames[i%len(s.frames)]), ansiReset, display)
				os.Stdout.Sync()
				i++
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
}

// Stop synchronously stops spinner, clears line, and flushes buffered output.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	s.mu.Unlock()

	close(s.stopCh)
	<-s.doneCh // wait for goroutine to finish

	// Clear spinner line
	fmt.Fprintf(cmdOutput, "\r\033[2K\033[?25h")

	// Flush buffered output
	s.outputMu.Lock()
	lines := s.pendingLines
	s.pendingLines = nil
	s.outputMu.Unlock()

	for _, line := range lines {
		fmt.Fprintln(cmdOutput, line)
	}
}

// Println buffers a line to be printed when spinner stops.
func (s *Spinner) Println(line string) {
	s.outputMu.Lock()
	s.pendingLines = append(s.pendingLines, line)
	s.outputMu.Unlock()
}

// Printf buffers a formatted line to be printed when spinner stops.
func (s *Spinner) Printf(format string, args ...any) {
	s.Println(fmt.Sprintf(format, args...))
}

func (s *Spinner) UpdateMessage(msg string) {
	s.mu.Lock()
	s.message = msg
	s.mu.Unlock()
}

func (s *Spinner) UpdateTokens(tokensIn, tokensOut int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokensIn = tokensIn
	s.tokensOut = tokensOut
	// Track generation time for accurate speed calculation
	if tokensOut > 0 {
		if s.genStart.IsZero() {
			s.genStart = time.Now()
		}
		s.genEnd = time.Now()
	}
}

func (s *Spinner) UpdateProvider(providerName, modelName string) {
	s.mu.Lock()
	// Avoid duplication like "qwen-fast-35b · qwen-fast-35b"
	// If provider name equals model name, show only model name
	if providerName == modelName && modelName != "" {
		s.providerName = ""
		s.modelName = modelName
	} else if providerName == "" && modelName != "" {
		// No provider name, show only model name
		s.providerName = ""
		s.modelName = modelName
	} else {
		s.providerName = providerName
		s.modelName = modelName
	}
	s.mu.Unlock()
}

func (s *Spinner) UpdateGenTime() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.genStart.IsZero() {
		s.genStart = time.Now()
	} else if !s.genEnd.IsZero() {
		// Accumulate generation time between tokens (skip if genEnd was reset by PauseGenTime)
		s.totalGenDur += time.Since(s.genEnd)
	}
	s.genEnd = time.Now()
}

// PauseGenTime resets genEnd so that time spent on tool execution
// is not counted as generation time. Call this when a tool starts executing.
func (s *Spinner) PauseGenTime() {
	s.mu.Lock()
	s.genEnd = time.Time{} // reset genEnd so next UpdateGenTime doesn't add tool execution time
	s.mu.Unlock()
}

// CopyStatsTo copies tokens and gen time to another spinner.
// Used when creating a new spinner to preserve stats.
func (s *Spinner) CopyStatsTo(dst *Spinner) {
	if s == nil || dst == nil {
		return
	}
	s.mu.Lock()
	srcIn := s.tokensIn
	srcOut := s.tokensOut
	srcGenStart := s.genStart
	srcGenEnd := s.genEnd
	srcTotalGenDur := s.totalGenDur
	srcProviderName := s.providerName
	srcModelName := s.modelName
	s.mu.Unlock()

	dst.mu.Lock()
	dst.tokensIn = srcIn
	dst.tokensOut = srcOut
	dst.totalGenDur = srcTotalGenDur
	dst.providerName = srcProviderName
	dst.modelName = srcModelName
	if !srcGenStart.IsZero() {
		dst.genStart = srcGenStart
	}
	if !srcGenEnd.IsZero() {
		dst.genEnd = srcGenEnd
	}
	dst.mu.Unlock()
}

func (s *Spinner) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func stopActiveSpinner(s *Spinner) *Spinner {
	if s != nil {
		s.Stop()
	}
	return nil
}

// ─── Duration / Token formatting ────────────────────────────────────────────

func FormatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return "<1ms"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
}

func FormatTokens(input, output int) string {
	total := input + output
	return fmt.Sprintf("%s⬆ %d %s⬇ %d %sΣ %d%s",
		appTheme.Info.ANSICode(), input, appTheme.Success.ANSICode(), output, ansiBold, total, ansiReset)
}

func FormatIteration(current int) string {
	return fmt.Sprintf("%sStep %d%s", appTheme.Primary.ANSICode(), current, ansiReset)
}

// ─── Context bar ────────────────────────────────────────────────────────────

func FormatContextBar(used, max int) string {
	if max == 0 {
		return fmt.Sprintf("%s%d%s", appTheme.Dim.ANSICode(), used, ansiReset)
	}
	pct := float64(used) / float64(max)
	barWidth := 20
	filled := int(pct * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	var clr string
	switch {
	case pct < 0.5:
		clr = appTheme.Success.ANSICode()
	case pct < 0.8:
		clr = appTheme.Warning.ANSICode()
	default:
		clr = appTheme.Error.ANSICode()
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	return fmt.Sprintf("%s%s%s %d/%d", clr, bar, ansiReset, used, max)
}

// ─── Status line ────────────────────────────────────────────────────────────

func FormatStatusLine(tokensIn, tokensOut int, dur time.Duration, ctxUsed, ctxMax int, providerName, modelName string, taskType ...string) string {
	return FormatStatusLineEx(tokensIn, tokensOut, dur, 0, ctxUsed, ctxMax, providerName, modelName, taskType...)
}

func FormatStatusLineEx(tokensIn, tokensOut int, dur, genDur time.Duration, ctxUsed, ctxMax int, providerName, modelName string, taskType ...string) string {
	var parts []string

	// Task type badge (first)
	if len(taskType) > 0 && taskType[0] != "" {
		parts = append(parts, fmt.Sprintf("%s%s%s", appTheme.Primary.ANSICode(), taskType[0], ansiReset))
	}
	if dur > 0 {
		parts = append(parts, fmt.Sprintf("%s⏱%s %s", appTheme.StatusTime.ANSICode(), ansiReset, FormatDuration(dur)))
	}
	if tokensIn > 0 || tokensOut > 0 {
		parts = append(parts, FormatTokens(tokensIn, tokensOut))
	}
	// Speed: use ONLY genDur (accumulated generation time)
	// Never fallback to total duration — it includes tool execution time
	if tokensOut > 0 && genDur > 0 {
		speed := float64(tokensOut) / genDur.Seconds()
		// Realistic range: 0.5-500 tok/s
		if speed >= 0.5 && speed <= 500 {
			speedStr := fmt.Sprintf("%s⚡%s %s%s%s", appTheme.Success.ANSICode(), ansiReset, appTheme.Info.ANSICode(), formatSpeed(speed), ansiReset)
			parts = append(parts, speedStr)
		}
	}
	if providerName != "" || modelName != "" {
		var info []string
		// Avoid duplication like "qwen-fast-35b · qwen-fast-35b"
		if providerName == modelName && modelName != "" {
			info = append(info, modelName)
		} else {
			if providerName != "" {
				info = append(info, providerName)
			}
			if modelName != "" {
				info = append(info, modelName)
			}
		}
		parts = append(parts, fmt.Sprintf("%s%s%s", appTheme.Dim.ANSICode(), strings.Join(info, " · "), ansiReset))
	}

	if len(parts) == 0 {
		return ""
	}

	separator := fmt.Sprintf(" %s│%s ", appTheme.StatusSep.ANSICode(), ansiReset)
	line := "  " + strings.Join(parts, separator)

	if ctxMax > 0 {
		line += "\n  " + FormatContextBar(ctxUsed, ctxMax)
	}
	return line
}

// formatSpeed formats tokens per second in human-readable form
func formatSpeed(speed float64) string {
	if speed < 10 {
		return fmt.Sprintf("%.1ftok/s", speed)
	}
	return fmt.Sprintf("%.0ftok/s", speed)
}

func FormatContextInfo(msgCount, tokensUsed, maxTokens int) string {
	var parts []string
	if msgCount >= 0 {
		parts = append(parts, fmt.Sprintf("%s: %d", i18n.T("cli.context_messages"), msgCount))
	}
	if maxTokens > 0 {
		parts = append(parts, fmt.Sprintf("%s: ~%d/%d", i18n.T("cli.context_tokens"), tokensUsed, maxTokens))
		parts = append(parts, FormatContextBar(tokensUsed, maxTokens))
	} else if tokensUsed > 0 {
		parts = append(parts, fmt.Sprintf("%s: ~%d", i18n.T("cli.context_tokens"), tokensUsed))
	}
	return strings.Join(parts, " │ ")
}

func FormatSeparator() string {
	return fmt.Sprintf("\n%s%s%s", appTheme.Separator.ANSICode(), "──────────────────────────────────────────────────", ansiReset)
}

func FormatProgressBar(current, total int, width int) string {
	if total <= 0 || width <= 0 {
		return ""
	}
	pct := float64(current) / float64(total)
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled
	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	return fmt.Sprintf("%s%s%s %d/%d", appTheme.Primary.ANSICode(), bar, ansiReset, current, total)
}

// ─── Tool call formatting ───────────────────────────────────────────────────

func FormatToolCallStart(name string, params map[string]string) string {
	var parts []string
	displayKeys := []string{"path", "command", "pattern", "query", "prompt", "url", "file", "dir", "lines", "task"}
	shown := make(map[string]bool)

	for _, key := range displayKeys {
		if v, ok := params[key]; ok {
			display := v
			if key == "command" {
				if idx := strings.Index(v, "\n"); idx >= 0 {
					display = v[:idx]
				}
				if utf8.RuneCountInString(display) > 80 {
					runes := []rune(display)
					display = string(runes[:77]) + "..."
				}
			} else if key == "task" {
				// Tasks can be very long — truncate aggressively
				if idx := strings.Index(v, "\n"); idx >= 0 {
					display = v[:idx]
				}
				if utf8.RuneCountInString(display) > 60 {
					runes := []rune(display)
					display = string(runes[:57]) + "..."
				}
			} else if utf8.RuneCountInString(display) > 120 {
				runes := []rune(display)
				display = string(runes[:117]) + "..."
			}
			parts = append(parts, display)
			shown[key] = true
		}
	}

	for k, v := range params {
		if shown[k] {
			continue
		}
		display := v
		if utf8.RuneCountInString(display) > 60 {
			runes := []rune(display)
			display = string(runes[:57]) + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, display))
	}

	paramStr := strings.Join(parts, appTheme.ToolParams.ANSICode()+" · "+ansiReset)
	if paramStr != "" {
		return fmt.Sprintf("  ⏺ %s(%s%s%s)", name, appTheme.ToolParams.ANSICode(), paramStr, ansiReset)
	}
	return fmt.Sprintf("  ⏺ %s", name)
}

func FormatToolCallEnd(name string, ok bool, result string, fullResult string, dur time.Duration, params map[string]string) string {
	status := appTheme.Success.ANSICode() + "✓" + ansiReset
	if !ok {
		status = appTheme.Error.ANSICode() + "✗" + ansiReset
	}

	durStr := ""
	if dur > 0 {
		durStr = " " + appTheme.StatusTime.ANSICode() + FormatDuration(dur) + ansiReset
	}

	summary := formatToolResultSummary(name, ok, result, fullResult, params)
	firstLine := fmt.Sprintf("  ⎿ %s %s%s", status, summary, durStr)

	extraLines := formatToolResultExtra(name, ok, fullResult)
	if len(extraLines) > 0 {
		return firstLine + "\n" + strings.Join(extraLines, "\n")
	}
	return firstLine
}

func formatToolResultSummary(name string, ok bool, result string, fullResult string, params map[string]string) string {
	if !ok {
		err := result
		if len(err) > 200 {
			err = err[:197] + "..."
		}
		return appTheme.Error.ANSICode() + err + ansiReset
	}

	switch name {
	case "read":
		return formatReadSummary(fullResult, params)
	case "bash":
		return formatBashSummary(result, params)
	case "write":
		return formatWriteSummary(fullResult)
	case "edit":
		return formatEditSummary(fullResult)
	case "todo_write":
		return formatTodoWriteSummary(fullResult)
	case "todo_read":
		return formatTodoReadSummary(fullResult)
	case "glob":
		return formatGlobSummary(fullResult)
	case "grep":
		return formatGrepSummary(fullResult)
	case "delegate_task":
		return formatDelegateTaskSummary(fullResult)
	default:
		return formatGenericSummary(result)
	}
}

func formatReadSummary(fullResult string, params map[string]string) string {
	if fullResult == "" {
		if path, ok := params["path"]; ok {
			return appTheme.ToolSummary.ANSICode() + path + ansiReset
		}
		return appTheme.ToolSummary.ANSICode() + "0 lines" + ansiReset
	}
	if strings.HasPrefix(fullResult, "Directory ") {
		firstLine := fullResult
		if idx := strings.Index(fullResult, "\n"); idx >= 0 {
			firstLine = fullResult[:idx]
		}
		firstLine = strings.TrimSuffix(firstLine, ":")
		return appTheme.ToolSummary.ANSICode() + firstLine + ansiReset
	}
	lines := strings.Count(fullResult, "\n") + 1
	return appTheme.ToolSummary.ANSICode() + fmt.Sprintf("%d lines", lines) + ansiReset
}

func formatBashSummary(result string, params map[string]string) string {
	if result == "" || result == "(command executed successfully, empty output)" {
		return appTheme.ToolSummary.ANSICode() + "empty output" + ansiReset
	}
	if cmd, ok := params["command"]; ok {
		display := cmd
		if len(display) > 80 {
			display = display[:77] + "..."
		}
		return appTheme.ToolSummary.ANSICode() + display + ansiReset
	}
	display := result
	if len(display) > 120 {
		display = display[:117] + "..."
	}
	return appTheme.ToolSummary.ANSICode() + display + ansiReset
}

func formatWriteSummary(fullResult string) string {
	if fullResult == "" {
		return appTheme.ToolSummary.ANSICode() + "written" + ansiReset
	}
	firstLine := fullResult
	if idx := strings.Index(fullResult, "\n"); idx >= 0 {
		firstLine = fullResult[:idx]
	}
	path := ""
	if strings.HasPrefix(firstLine, "file ") && strings.Contains(firstLine, " written ") {
		parts := strings.SplitN(firstLine, " ", 3)
		if len(parts) >= 2 {
			path = parts[1]
		}
	}
	rest := ""
	if idx := strings.Index(fullResult, "\n"); idx >= 0 {
		rest = fullResult[idx+1:]
	}
	if strings.HasPrefix(rest, "---") {
		added, removed := tools.DiffStats(rest)
		if added > 0 && removed > 0 {
			return appTheme.ToolSummary.ANSICode() + fmt.Sprintf("Added %d, Removed %d lines in %s", added, removed, path) + ansiReset
		} else if added > 0 {
			return appTheme.ToolSummary.ANSICode() + fmt.Sprintf("Added %d lines in %s", added, path) + ansiReset
		} else if removed > 0 {
			return appTheme.ToolSummary.ANSICode() + fmt.Sprintf("Removed %d lines in %s", removed, path) + ansiReset
		}
		return appTheme.ToolSummary.ANSICode() + "Edited " + path + ansiReset
	}
	if rest != "" {
		lines := strings.Count(rest, "\n") + 1
		if strings.HasSuffix(rest, "\n") {
			lines--
		}
		if path != "" {
			return appTheme.ToolSummary.ANSICode() + fmt.Sprintf("Wrote %d lines to %s", lines, path) + ansiReset
		}
		return appTheme.ToolSummary.ANSICode() + fmt.Sprintf("Wrote %d lines", lines) + ansiReset
	}
	if path != "" {
		return appTheme.ToolSummary.ANSICode() + "Wrote " + path + ansiReset
	}
	return appTheme.ToolSummary.ANSICode() + "written" + ansiReset
}

func formatEditSummary(fullResult string) string {
	if fullResult == "" {
		return appTheme.ToolSummary.ANSICode() + "edited" + ansiReset
	}
	firstLine := fullResult
	if idx := strings.Index(fullResult, "\n"); idx >= 0 {
		firstLine = fullResult[:idx]
	}
	path := ""
	if strings.HasPrefix(firstLine, "file ") && strings.Contains(firstLine, " edited") {
		parts := strings.SplitN(firstLine, " ", 3)
		if len(parts) >= 2 {
			path = parts[1]
		}
	}
	rest := ""
	if idx := strings.Index(fullResult, "\n"); idx >= 0 {
		rest = fullResult[idx+1:]
	}
	if strings.HasPrefix(rest, "---") {
		added, removed := tools.DiffStats(rest)
		if added > 0 && removed > 0 {
			return appTheme.ToolSummary.ANSICode() + fmt.Sprintf("Added %d, Removed %d lines in %s", added, removed, path) + ansiReset
		} else if added > 0 {
			return appTheme.ToolSummary.ANSICode() + fmt.Sprintf("Added %d lines in %s", added, path) + ansiReset
		} else if removed > 0 {
			return appTheme.ToolSummary.ANSICode() + fmt.Sprintf("Removed %d lines in %s", removed, path) + ansiReset
		}
	}
	if path != "" {
		return appTheme.ToolSummary.ANSICode() + "Edited " + path + ansiReset
	}
	return appTheme.ToolSummary.ANSICode() + "edited" + ansiReset
}

func formatGlobSummary(fullResult string) string {
	if fullResult == "no files found" || fullResult == "" {
		return appTheme.ToolSummary.ANSICode() + "no files found" + ansiReset
	}
	count := strings.Count(fullResult, "\n") + 1
	return appTheme.ToolSummary.ANSICode() + fmt.Sprintf("%d files", count) + ansiReset
}

func formatGrepSummary(fullResult string) string {
	if fullResult == "no matches found" || fullResult == "" {
		return appTheme.ToolSummary.ANSICode() + "no matches found" + ansiReset
	}
	count := strings.Count(fullResult, "\n") + 1
	return appTheme.ToolSummary.ANSICode() + fmt.Sprintf("%d matches", count) + ansiReset
}

func formatGenericSummary(result string) string {
	if result == "" {
		return appTheme.ToolSummary.ANSICode() + "done" + ansiReset
	}
	display := result
	if idx := strings.Index(display, "\n"); idx >= 0 {
		display = display[:idx]
	}
	if len(display) > 80 {
		display = display[:77] + "..."
	}
	return appTheme.ToolSummary.ANSICode() + display + ansiReset
}

func formatDelegateTaskSummary(fullResult string) string {
	if fullResult == "" {
		return appTheme.ToolSummary.ANSICode() + "subagent completed" + ansiReset
	}
	lines := strings.Split(fullResult, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 120 {
				return appTheme.ToolSummary.ANSICode() + line[:117] + "..." + ansiReset
			}
			return appTheme.ToolSummary.ANSICode() + line + ansiReset
		}
	}
	return appTheme.ToolSummary.ANSICode() + "subagent completed" + ansiReset
}

// ─── Tool result extra lines ────────────────────────────────────────────────

func formatToolResultExtra(name string, ok bool, fullResult string) []string {
	if fullResult == "" || !ok {
		return nil
	}
	switch name {
	case "bash":
		return formatBashExtra(fullResult)
	case "edit":
		return formatDiffExtra(fullResult)
	case "write":
		return formatDiffExtra(fullResult)
	default:
		return nil
	}
}

func formatBashExtra(fullResult string) []string {
	lines := strings.Split(fullResult, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	lang := detectLanguageFromContent(fullResult)
	highlighted := highlightCodeBlock(fullResult, lang)
	highlightedLines := strings.Split(highlighted, "\n")
	if len(highlightedLines) > 0 && highlightedLines[len(highlightedLines)-1] == "" {
		highlightedLines = highlightedLines[:len(highlightedLines)-1]
	}

	maxLines := 200
	var result []string
	for i, line := range highlightedLines {
		if i >= maxLines {
			remaining := len(highlightedLines) - maxLines
			result = append(result, fmt.Sprintf("     %s... (%d more lines)%s", appTheme.ToolSummary.ANSICode(), remaining, ansiReset))
			break
		}
		result = append(result, "     "+line)
	}
	return result
}

func formatDiffExtra(fullResult string) []string {
	if fullResult == "" {
		return nil
	}

	filePath := ""
	firstLine := fullResult
	if idx := strings.Index(fullResult, "\n"); idx >= 0 {
		firstLine = fullResult[:idx]
	}
	if strings.HasPrefix(firstLine, "file ") {
		parts := strings.SplitN(firstLine, " ", 3)
		if len(parts) >= 2 {
			filePath = parts[1]
		}
	}

	rest := ""
	if idx := strings.Index(fullResult, "\n"); idx >= 0 {
		rest = fullResult[idx+1:]
	}

	if rest == "" || !strings.HasPrefix(rest, "---") {
		var codeLines []string
		lines := strings.Split(rest, "\n")
		for _, line := range lines {
			codeLine := stripLineNumber(line)
			codeLines = append(codeLines, codeLine)
		}
		if len(codeLines) > 0 && codeLines[len(codeLines)-1] == "" {
			codeLines = codeLines[:len(codeLines)-1]
		}

		lang := languageFromPath(filePath)
		highlighted := highlightCodeBlock(strings.Join(codeLines, "\n"), lang)
		highlightedLines := strings.Split(highlighted, "\n")
		if len(highlightedLines) > 0 && highlightedLines[len(highlightedLines)-1] == "" {
			highlightedLines = highlightedLines[:len(highlightedLines)-1]
		}

		maxLines := 50
		var result []string
		for i, line := range highlightedLines {
			if i >= maxLines {
				remaining := len(highlightedLines) - maxLines
				result = append(result, fmt.Sprintf("     %s... (%d more lines)%s", appTheme.ToolSummary.ANSICode(), remaining, ansiReset))
				break
			}
			lineNum := appTheme.Dim.ANSICode() + fmt.Sprintf("%4d", i+1) + ansiReset
			result = append(result, "  "+lineNum+"  "+line)
		}
		return result
	}

	diffLines := tools.DiffLines(rest, 50)
	if len(diffLines) == 0 {
		return nil
	}

	var result []string
	for _, line := range diffLines {
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case '+':
			result = append(result, appTheme.Success.ANSICode()+"     "+line+ansiReset)
		case '-':
			result = append(result, appTheme.Error.ANSICode()+"     "+line+ansiReset)
		default:
			result = append(result, appTheme.Dim.ANSICode()+"     "+line+ansiReset)
		}
	}
	return result
}

func stripLineNumber(line string) string {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	for i < len(line) && line[i] == ' ' {
		i++
	}
	if i < len(line) {
		return line[i:]
	}
	return line
}

func detectLanguageFromContent(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "bash"
	}
	if strings.Contains(trimmed, "pub fn ") || (strings.Contains(trimmed, "fn ") && strings.Contains(trimmed, "->")) ||
		strings.Contains(trimmed, "let mut ") || strings.Contains(trimmed, "impl ") {
		return "rust"
	}
	if (strings.Contains(trimmed, "func ") && strings.Contains(trimmed, "package ")) ||
		(strings.Contains(trimmed, "func (") && strings.Contains(trimmed, "error")) {
		return "go"
	}
	if (strings.Contains(trimmed, "def ") && strings.Contains(trimmed, ":")) ||
		(strings.Contains(trimmed, "import ") && strings.Contains(trimmed, "self")) {
		return "python"
	}
	if (strings.Contains(trimmed, "const ") && strings.Contains(trimmed, "=>")) ||
		(strings.Contains(trimmed, "function ") && strings.Contains(trimmed, "{")) {
		return "javascript"
	}
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return "json"
	}
	if strings.Contains(trimmed, ": ") && !strings.Contains(trimmed, "->") &&
		(strings.HasPrefix(trimmed, "- ") || strings.Contains(trimmed, "\n- ")) {
		return "yaml"
	}
	return "bash"
}

func languageFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	langMap := map[string]string{
		".go": "go", ".py": "python", ".js": "javascript", ".ts": "typescript",
		".tsx": "typescript", ".jsx": "javascript", ".rs": "rust", ".java": "java",
		".kt": "kotlin", ".rb": "ruby", ".php": "php", ".c": "c", ".cpp": "cpp",
		".h": "c", ".hpp": "cpp", ".cs": "csharp", ".swift": "swift",
		".sh": "bash", ".bash": "bash", ".zsh": "bash", ".sql": "sql",
		".html": "html", ".css": "css", ".scss": "scss", ".json": "json",
		".yaml": "yaml", ".yml": "yaml", ".toml": "toml", ".xml": "xml",
		".md": "markdown", ".lua": "lua",
	}
	if lang, ok := langMap[ext]; ok {
		return lang
	}
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "dockerfile":
		return "docker"
	case "makefile":
		return "make"
	case "go.mod", "go.sum":
		return "go"
	}
	return ""
}

func highlightCodeBlock(code, lang string) string {
	if lang == "" {
		return code
	}
	var buf strings.Builder
	style := "monokai"
	if appTheme != nil && appTheme.Mode == "light" {
		style = "emacs"
	}
	err := quick.Highlight(&buf, code, lang, "terminal256", style)
	if err != nil {
		return code
	}
	return buf.String()
}

// ─── Todo formatting ────────────────────────────────────────────────────────

type todoItem struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Status  string `json:"status"`
}

func FormatTodoChecklist(jsonStr string) string {
	var items []todoItem
	if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
		return appTheme.ToolSummary.ANSICode() + jsonStr + ansiReset
	}
	if len(items) == 0 {
		return appTheme.ToolSummary.ANSICode() + "no tasks" + ansiReset
	}
	var sb strings.Builder
	for _, item := range items {
		var icon string
		var clr string
		switch item.Status {
		case "completed":
			icon = "✔"
			clr = appTheme.Success.ANSICode()
		case "in_progress":
			icon = "◉"
			clr = appTheme.Primary.ANSICode()
		default:
			icon = "☐"
			clr = appTheme.Dim.ANSICode()
		}
		sb.WriteString(fmt.Sprintf("  %s%s%s %s\n", clr, icon, ansiReset, item.Subject))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatTodoWriteSummary(fullResult string) string {
	var items []todoItem
	if err := json.Unmarshal([]byte(fullResult), &items); err != nil {
		return appTheme.ToolSummary.ANSICode() + "updated" + ansiReset
	}
	total := len(items)
	done := 0
	for _, item := range items {
		if item.Status == "completed" {
			done++
		}
	}
	return appTheme.ToolSummary.ANSICode() + fmt.Sprintf("%d tasks (%d done)", total, done) + ansiReset
}

func formatTodoReadSummary(fullResult string) string {
	var items []todoItem
	if err := json.Unmarshal([]byte(fullResult), &items); err != nil {
		return appTheme.ToolSummary.ANSICode() + fullResult + ansiReset
	}
	return appTheme.ToolSummary.ANSICode() + fmt.Sprintf("%d tasks", len(items)) + ansiReset
}

// ─── Markdown rendering ─────────────────────────────────────────────────────

type GlamourRenderer struct {
	buf strings.Builder
}

func NewGlamourRenderer() *GlamourRenderer {
	return &GlamourRenderer{}
}

func (gr *GlamourRenderer) Render(text string) string {
	gr.buf.WriteString(text)
	return ""
}

func (gr *GlamourRenderer) Flush() string {
	if gr.buf.Len() == 0 {
		return ""
	}
	raw := gr.buf.String()
	gr.buf.Reset()
	rendered, err := RenderMarkdownGlamour(raw)
	if err != nil {
		return raw
	}
	return rendered
}

func RenderMarkdownGlamour(text string) (string, error) {
	mode := "dark"
	wordWrap := 80
	if appTheme != nil {
		mode = appTheme.Mode
		if appTheme.WordWrap > 0 {
			wordWrap = appTheme.WordWrap
		}
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(mode),
		glamour.WithWordWrap(wordWrap),
	)
	if err != nil {
		return text, err
	}
	out, err := r.Render(text)
	if err != nil {
		return text, err
	}
	return out, nil
}

// ─── Utility ────────────────────────────────────────────────────────────────

func formatToolSummary(toolName string, params map[string]string) string {
	if len(params) == 0 {
		return toolName
	}
	var parts []string
	displayKeys := []string{"path", "command", "pattern", "query", "prompt", "url", "file", "dir", "lines", "task"}
	shown := make(map[string]bool)
	for _, key := range displayKeys {
		if v, ok := params[key]; ok {
			display := v
			if key == "task" {
				// Tasks can be very long — truncate aggressively
				if idx := strings.Index(v, "\n"); idx >= 0 {
					display = v[:idx]
				}
				if utf8.RuneCountInString(display) > 60 {
					runes := []rune(display)
					display = string(runes[:57]) + "..."
				}
			} else if key == "command" {
				if idx := strings.Index(v, "\n"); idx >= 0 {
					display = v[:idx]
				}
				if len(display) > 80 {
					display = display[:77] + "..."
				}
			} else if len(display) > 120 {
				display = display[:117] + "..."
			}
			parts = append(parts, display)
			shown[key] = true
		}
	}
	for k, v := range params {
		if shown[k] {
			continue
		}
		display := v
		if len(display) > 60 {
			display = display[:57] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, display))
	}
	paramStr := strings.Join(parts, " · ")
	if paramStr != "" {
		return fmt.Sprintf("%s(%s)", toolName, paramStr)
	}
	return toolName
}

func parsePartialToolInput(jsonStr string) map[string]string {
	result := make(map[string]string)
	if jsonStr == "" {
		return result
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &raw); err == nil {
		for k, v := range raw {
			result[k] = fmt.Sprintf("%v", v)
		}
		return result
	}
	// Try to fix partial JSON:
	// 1. Count unescaped quotes to detect unclosed strings
	quoteCount := 0
	for i := 0; i < len(jsonStr); i++ {
		if jsonStr[i] == '"' && (i == 0 || jsonStr[i-1] != '\\') {
			quoteCount++
		}
	}
	fixed := jsonStr
	// If odd number of quotes, last string is unclosed — close it
	if quoteCount%2 != 0 {
		fixed += `"`
	}
	// Close unclosed braces and brackets
	openBraces := strings.Count(fixed, "{") - strings.Count(fixed, "}")
	for i := 0; i < openBraces; i++ {
		fixed += "}"
	}
	openBrackets := strings.Count(fixed, "[") - strings.Count(fixed, "]")
	for i := 0; i < openBrackets; i++ {
		fixed += "]"
	}
	if err := json.Unmarshal([]byte(fixed), &raw); err == nil {
		for k, v := range raw {
			result[k] = fmt.Sprintf("%v", v)
		}
	}
	return result
}

func wrapText(text string, indent, width int) string {
	if text == "" {
		return ""
	}
	if width <= 0 {
		width = 80
	}
	prefix := strings.Repeat(" ", indent)
	available := width - indent
	if available < 20 {
		available = 20
	}
	var result strings.Builder
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}
		line = strings.TrimSpace(line)
		if len(line) <= available {
			result.WriteString(prefix + line)
			continue
		}
		runes := []rune(line)
		for len(runes) > 0 {
			if len(runes) <= available {
				result.WriteString(prefix + string(runes))
				break
			}
			spaceIdx := -1
			for j := available; j >= 0; j-- {
				if j < len(runes) && runes[j] == ' ' {
					spaceIdx = j
					break
				}
			}
			if spaceIdx <= 0 {
				spaceIdx = available
			}
			result.WriteString(prefix + string(runes[:spaceIdx]) + "\n")
			runes = runes[spaceIdx+1:]
		}
	}
	return result.String()
}

func showSpinner(msg string) *Spinner {
	s := NewSpinner(msg)
	s.Start()
	return s
}

func printColored(c *color.Color, format string, args ...any) {
	c.Printf(format, args...)
}

func printWarning(format string, args ...any) {
	color.Yellow(format, args...)
}

func printError(format string, args ...any) {
	color.Red(format, args...)
}

func printSuccess(format string, args ...any) {
	color.Green(format, args...)
}

func printInfo(format string, args ...any) {
	color.Cyan(format, args...)
}

func showHelp() {
	fmt.Fprintln(cmdOutput, )
	color.Cyan("  Commands:")
	fmt.Fprintln(cmdOutput, "    /help          — " + i18n.T("cli.help_help"))
	fmt.Fprintln(cmdOutput, "    /tui           — " + i18n.T("cli.help_tui"))
	fmt.Fprintln(cmdOutput, "    /cli           — " + i18n.T("cli.help_cli"))
	fmt.Fprintln(cmdOutput, "    /auto          — " + i18n.T("cli.help_auto"))
	fmt.Fprintln(cmdOutput, "    /compact       — " + i18n.T("cli.help_compact"))
	fmt.Fprintln(cmdOutput, "    /clear         — " + i18n.T("cli.help_clear"))
	fmt.Fprintln(cmdOutput, "    /model <name>  — " + i18n.T("cli.help_model"))
	fmt.Fprintln(cmdOutput, "    /provider <n>  — " + i18n.T("cli.help_provider"))
	fmt.Fprintln(cmdOutput, "    /session <id>  — " + i18n.T("cli.help_session"))
	fmt.Fprintln(cmdOutput, "    /sessions      — " + i18n.T("cli.help_sessions"))
	fmt.Fprintln(cmdOutput, "    /config        — " + i18n.T("cli.help_config"))
	fmt.Fprintln(cmdOutput, "    /exit          — " + i18n.T("cli.help_exit"))
	fmt.Fprintln(cmdOutput, )
	color.Cyan("  Shortcuts:")
	fmt.Fprintln(cmdOutput, "    Enter          — " + i18n.T("cli.help_send"))
	fmt.Fprintln(cmdOutput, "    Shift+Enter    — " + i18n.T("cli.help_newline"))
	fmt.Fprintln(cmdOutput, "    Ctrl+C         — " + i18n.T("cli.help_interrupt"))
	fmt.Fprintln(cmdOutput, "    Esc            — " + i18n.T("cli.help_cancel"))
	fmt.Fprintln(cmdOutput, )
}
