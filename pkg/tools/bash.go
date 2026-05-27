package tools

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"bugbuster-code/pkg/i18n"
)

// isBackgroundCommand detects if a command ends with & (background execution).
// Catches: "command &", "cmd1; cmd2 &", "cmd &;"
// Does NOT catch: "cmd && cmd", "cmd &>file", "cmd 2>&1"
func isBackgroundCommand(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	if len(trimmed) == 0 || trimmed[len(trimmed)-1] != '&' {
		return false
	}
	// Check it's not "&&" (logical AND)
	if strings.HasSuffix(trimmed, "&&") {
		return false
	}
	// Check it's not "&>" (redirect stderr+stdout)
	if strings.Contains(trimmed, "&>") {
		// If the last & is part of &>, it's not background
		lastAmp := strings.LastIndex(trimmed, "&")
		if lastAmp > 0 && trimmed[lastAmp-1:lastAmp+1] == "&>" {
			return false
		}
	}
	// Check it's not "2>&1" (redirect stderr to stdout)
	if strings.HasSuffix(trimmed, "2>&1") {
		return false
	}
	return true
}

// BashTool is the shell command execution tool
type BashTool struct {
	AllowedDirs     []string
	DefaultDir      string
	Timeout         time.Duration
	BlockedCommands []string
	AllowNetwork    bool
	BgTool          *BackgroundTool
}

func NewBashTool() *BashTool {
	return &BashTool{
		Timeout: 30 * time.Second,
	}
}

func (t *BashTool) Name() string { return "bash" }

func (t *BashTool) Description() string {
	return i18n.T("tools.bash.description")
}

func (t *BashTool) Execute(params map[string]string) ToolResult {
	command, ok := params["command"]
	if !ok || command == "" {
		return Error("tools.bash.param_command")
	}

	timeout := t.Timeout
	if ts, ok := params["timeout"]; ok {
		if sec, err := strconv.Atoi(ts); err == nil && sec > 0 {
			timeout = time.Duration(sec) * time.Second
		}
	}

	workDir := t.DefaultDir
	if dir, ok := params["dir"]; ok && dir != "" {
		workDir = filepath.Clean(dir)
	}

	// Security checks
	blocked := t.BlockedCommands
	if len(blocked) == 0 {
		blocked = []string{"rm -rf /", "mkfs", "dd if=", "> /dev/sd", "format c:"}
	}
	lowerCmd := strings.ToLower(command)
	for _, d := range blocked {
		if strings.Contains(lowerCmd, d) {
			return Error("security.command_blocked", command)
		}
	}

	dangerousPatterns := []string{
		"$(rm", "$(rmdir", "$(mkfs", "$(dd ", "$(format",
		"`rm", "`rmdir", "`mkfs",
		"> /dev/sd", "> /dev/hd",
		":(){:|:&};:",
	}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerCmd, pattern) {
			return Error("security.command_blocked", command)
		}
	}

	if !t.AllowNetwork {
		networkCmds := []string{"curl ", "wget ", "nc ", "ncat ", "ssh ", "scp ", "rsync ", "ftp "}
		for _, nc := range networkCmds {
			if strings.Contains(lowerCmd, nc) {
				return Error("security.network_blocked", command)
			}
		}
	}

	// Auto-detect background commands (&)
	// Catch: "command &", "command &;", "command1; command2 &"
	// Don't catch: "command && command", "command &>file", "command 2>&1"
	trimmedCmd := strings.TrimSpace(command)
	if isBackgroundCommand(trimmedCmd) {
		bgCmd := strings.TrimRight(trimmedCmd, "& ;")
		bgCmd = strings.TrimSpace(bgCmd)
		return ToolResult{
			Output: fmt.Sprintf("⚠️ Background command detected. The `&` operator is NOT supported in the bash tool.\n\nUse the `background` tool instead:\n\nbackground(command=\"%s\")\n\nUse ps() to check status, logs(id=N) to view output, kill(id=N) to stop.", bgCmd),
		}
	}

	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	var cmd *exec.Cmd
	if strings.Contains(command, "&&") || strings.Contains(command, "||") || strings.Contains(command, "|") || strings.Contains(command, ";") {
		cmd = exec.Command("bash", "-c", command)
	} else {
		parts := strings.Fields(command)
		cmd = exec.Command(parts[0], parts[1:]...)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return Error("tools.bash.exec_error", err, "")
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		output := StripANSIAndTrim(stdout.String())
		if stderrStr := StripANSIAndTrim(stderr.String()); stderrStr != "" {
			if output != "" {
				output += "\n"
			}
			output += stderrStr
		}
		if err != nil {
			if output == "" {
				output = err.Error()
			}
			return Error("tools.bash.exec_error", err, output)
		}
		if output == "" {
			output = i18n.T("tools.bash.empty_output")
		}
		maxOutput := 50000
		if len(output) > maxOutput {
			output = output[:maxOutput] + fmt.Sprintf(i18n.T("tools.bash.truncated"), len(output))
		}
		return Success("%s", output)

	case <-time.After(timeout):
		// Timeout — move process to background instead of killing
		if t.BgTool != nil && cmd.Process != nil {
			bgID, bgErr := t.BgTool.MoveToBackground(cmd, stdout.String(), stderr.String(), time.Now())
			if bgErr == nil {
				partialOutput := StripANSIAndTrim(stdout.String())
				if partialOutput == "" {
					partialOutput = "(no output before timeout)"
				}
				return ToolResult{
					Output: i18n.T("tools.bash.timeout_moved_to_bg", fmt.Sprintf("%v", timeout), strconv.Itoa(bgID), strconv.Itoa(cmd.Process.Pid)) +
						"\n\nPartial output:\n" + partialOutput +
						"\n\nUse `ps()` to check status, `logs(id=\"" + strconv.Itoa(bgID) + "\")` to view output, `kill(id=\"" + strconv.Itoa(bgID) + "\")` to stop.",
				}
			}
		}
		// Fallback: kill process if MoveToBackground failed
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		}
		killTimer := time.NewTimer(5 * time.Second)
		select {
		case <-done:
			// Process exited after SIGTERM
		case <-killTimer.C:
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
			<-done
		}
		killTimer.Stop()
		partialOutput := StripANSIAndTrim(stdout.String())
		if partialOutput == "" {
			partialOutput = "(no output before timeout)"
		}
		return ToolResult{
			Error: fmt.Sprintf(i18n.T("tools.bash.timeout"), timeout) +
				"\n\nPartial output:\n" + partialOutput +
				"\n\nTip: Retry with a longer timeout, e.g. timeout=120",
		}
	}
}

func (t *BashTool) ExecuteAsync(params map[string]string) <-chan AsyncEvent {
	ch := make(chan AsyncEvent, 32)

	go func() {
		defer close(ch)

		command, ok := params["command"]
		if !ok || command == "" {
			ch <- AsyncEvent{Type: "result", Error: i18n.T("tools.bash.param_command"), Done: true}
			return
		}

		timeout := t.Timeout
		if ts, ok := params["timeout"]; ok {
			if sec, err := strconv.Atoi(ts); err == nil && sec > 0 {
				timeout = time.Duration(sec) * time.Second
			}
		}

		workDir := t.DefaultDir
		if dir, ok := params["dir"]; ok && dir != "" {
			workDir = filepath.Clean(dir)
		}

		// Security checks
		blocked := t.BlockedCommands
		if len(blocked) == 0 {
			blocked = []string{"rm -rf /", "mkfs", "dd if=", "> /dev/sd", "format c:"}
		}
		lowerCmd := strings.ToLower(command)
		for _, d := range blocked {
			if strings.Contains(lowerCmd, d) {
				ch <- AsyncEvent{Type: "result", Error: i18n.T("security.command_blocked", command), Done: true}
				return
			}
		}

		dangerousPatterns := []string{
			"$(rm", "$(rmdir", "$(mkfs", "$(dd ", "$(format",
			"`rm", "`rmdir", "`mkfs",
			"> /dev/sd", "> /dev/hd",
			":(){:|:&};:",
		}
		for _, pattern := range dangerousPatterns {
			if strings.Contains(lowerCmd, pattern) {
				ch <- AsyncEvent{Type: "result", Error: i18n.T("security.command_blocked", command), Done: true}
				return
			}
		}

		if !t.AllowNetwork {
			networkCmds := []string{"curl ", "wget ", "nc ", "ncat ", "ssh ", "scp ", "rsync ", "ftp "}
			for _, nc := range networkCmds {
				if strings.Contains(lowerCmd, nc) {
					ch <- AsyncEvent{Type: "result", Error: i18n.T("security.network_blocked", command), Done: true}
					return
				}
			}
		}

		// Auto-detect background commands (&)
		// Catch: "command &", "command &;", "command1; command2 &"
		// Don't catch: "command && command", "command &>file", "command 2>&1"
		trimmedCmd := strings.TrimSpace(command)
		if isBackgroundCommand(trimmedCmd) {
			bgCmd := strings.TrimRight(trimmedCmd, "& ;")
			bgCmd = strings.TrimSpace(bgCmd)
			ch <- AsyncEvent{
				Type:   "result",
				Output: fmt.Sprintf("⚠️ Background command detected. The `&` operator is NOT supported in the bash tool.\n\nUse the `background` tool instead:\n\nbackground(command=\"%s\")\n\nUse ps() to check status, logs(id=N) to view output, kill(id=N) to stop.", bgCmd),
				Done:   true,
			}
			return
		}

		var cmd *exec.Cmd
		if strings.Contains(command, "&&") || strings.Contains(command, "||") || strings.Contains(command, "|") || strings.Contains(command, ";") {
			cmd = exec.Command("bash", "-c", command)
		} else {
			parts := strings.Fields(command)
			cmd = exec.Command(parts[0], parts[1:]...)
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if workDir != "" {
			cmd.Dir = workDir
		}

		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			ch <- AsyncEvent{Type: "result", Error: err.Error(), Done: true}
			return
		}
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			ch <- AsyncEvent{Type: "result", Error: err.Error(), Done: true}
			return
		}

		if err := cmd.Start(); err != nil {
			ch <- AsyncEvent{Type: "result", Error: err.Error(), Done: true}
			return
		}

		var stdoutBuf, stderrBuf strings.Builder
		var scanWg sync.WaitGroup
		scanWg.Add(1)
		go func() {
			defer scanWg.Done()
			scanner := bufio.NewScanner(stdoutPipe)
			for scanner.Scan() {
				line := scanner.Text()
				stdoutBuf.WriteString(line)
				stdoutBuf.WriteString("\n")
				ch <- AsyncEvent{Type: "progress", Output: line}
			}
			scanner = bufio.NewScanner(stderrPipe)
			for scanner.Scan() {
				line := scanner.Text()
				stderrBuf.WriteString(line)
				stderrBuf.WriteString("\n")
				ch <- AsyncEvent{Type: "progress", Output: line}
			}
		}()

		waitCh := make(chan error, 1)
		go func() { waitCh <- cmd.Wait() }()

		timeoutCh := time.After(timeout)

		var cmdErr error
		select {
		case cmdErr = <-waitCh:
			// Command completed
		case <-timeoutCh:
			// Timeout — move process to background
			if t.BgTool != nil && cmd.Process != nil {
				bgID, bgErr := t.BgTool.MoveToBackground(cmd, stdoutBuf.String(), stderrBuf.String(), time.Now())
				if bgErr == nil {
					partialOutput := StripANSIAndTrim(stdoutBuf.String())
					if partialOutput == "" {
						partialOutput = "(no output before timeout)"
					}
					ch <- AsyncEvent{
						Type: "result",
						Output: i18n.T("tools.bash.timeout_moved_to_bg", fmt.Sprintf("%v", timeout), strconv.Itoa(bgID), strconv.Itoa(cmd.Process.Pid)) +
							"\n\nPartial output:\n" + partialOutput +
							"\n\nUse `ps()` to check status, `logs(id=\"" + strconv.Itoa(bgID) + "\")` to view output, `kill(id=\"" + strconv.Itoa(bgID) + "\")` to stop.",
						Done: true,
					}
					scanWg.Wait()
					return
				}
			}
			// Fallback: kill process if MoveToBackground failed
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			}
			killTimer := time.NewTimer(5 * time.Second)
			select {
			case <-waitCh:
				// Process exited after SIGTERM
			case <-killTimer.C:
				if cmd.Process != nil {
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				}
				<-waitCh
			}
			killTimer.Stop()
			partialOutput := StripANSIAndTrim(stdoutBuf.String())
			if partialOutput == "" {
				partialOutput = "(no output before timeout)"
			}
			ch <- AsyncEvent{
				Type:   "result",
				Output: partialOutput,
				Error: fmt.Sprintf(i18n.T("tools.bash.timeout"), timeout) +
					"\n\nPartial output:\n" + partialOutput +
					"\n\nTip: Retry with a longer timeout, e.g. timeout=120",
				Done: true,
			}
			scanWg.Wait()
			return
		}

		scanWg.Wait()

		output := StripANSIAndTrim(stdoutBuf.String())
		if stderrStr := StripANSIAndTrim(stderrBuf.String()); stderrStr != "" {
			if output != "" {
				output += "\n"
			}
			output += stderrStr
		}

		if cmdErr != nil {
			var errDetail string
			if exitErr, ok := cmdErr.(*exec.ExitError); ok {
				errDetail = exitErr.ProcessState.String()
			} else {
				errDetail = cmdErr.Error()
			}
			if output == "" {
				output = errDetail
			}
			ch <- AsyncEvent{Type: "result", Output: output, Error: fmt.Sprintf("command failed: %s", errDetail), Done: true}
			return
		}

		if output == "" {
			output = i18n.T("tools.bash.empty_output")
		}

		maxOutput := 50000
		if len(output) > maxOutput {
			output = output[:maxOutput] + fmt.Sprintf(i18n.T("tools.bash.truncated"), len(output))
		}

		ch <- AsyncEvent{Type: "result", Output: output, Done: true}
	}()

	return ch
}

func (t *BashTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.bash.param_command_desc"),
			},
			"timeout": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.bash.param_timeout_desc"),
			},
			"dir": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.bash.param_workdir_desc"),
			},
		},
		"required": []string{"command"},
	}
}