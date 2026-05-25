package tools

import (
	"bufio"
	"bytes"
	"context"
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

// BashTool is the shell command execution tool
type BashTool struct {
	AllowedDirs     []string // allowed directories for commands
	DefaultDir      string   // default working directory
	Timeout         time.Duration
	BlockedCommands []string // blocked commands (from config)
	AllowNetwork    bool     // whether network commands are allowed
}

// NewBashTool creates a tool for executing bash commands with optional timeout.
// NewBashTool creates a tool for executing bash commands with optional timeout.
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

	// Timeout
	timeout := t.Timeout
	if ts, ok := params["timeout"]; ok {
		if sec, err := strconv.Atoi(ts); err == nil && sec > 0 {
			timeout = time.Duration(sec) * time.Second
		}
	}

	// Working directory
	workDir := t.DefaultDir
	if dir, ok := params["dir"]; ok && dir != "" {
		workDir = filepath.Clean(dir)
	}

	// Security: blocked commands (from config + defaults)
	blocked := t.BlockedCommands
	if len(blocked) == 0 {
		// Default list if config is not set
		blocked = []string{"rm -rf /", "mkfs", "dd if=", "> /dev/sd", "format c:"}
	}
	lowerCmd := strings.ToLower(command)
	for _, d := range blocked {
		if strings.Contains(lowerCmd, d) {
			return Error("security.command_blocked", command)
		}
	}

	// Security: dangerous constructs (command substitution, pipe to shell, etc.)
	dangerousPatterns := []string{
		"$(rm", "$(rmdir", "$(mkfs", "$(dd ", "$(format",
		"`rm", "`rmdir", "`mkfs",
		"> /dev/sd", "> /dev/hd",
		":(){:|:&};:", // fork bomb
	}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerCmd, pattern) {
			return Error("security.command_blocked", command)
		}
	}

	// Security: network commands
	if !t.AllowNetwork {
		networkCmds := []string{"curl ", "wget ", "nc ", "ncat ", "ssh ", "scp ", "rsync ", "ftp "}
		for _, nc := range networkCmds {
			if strings.Contains(lowerCmd, nc) {
				return Error("security.network_blocked", command)
			}
		}
	}

	// Command execution with timeout via context
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd
	if strings.Contains(command, "&&") || strings.Contains(command, "||") || strings.Contains(command, "|") || strings.Contains(command, ";") {
		cmd = exec.CommandContext(ctx, "bash", "-c", command)
	} else {
		parts := strings.Fields(command)
		cmd = exec.CommandContext(ctx, parts[0], parts[1:]...)
	}
	// Put command in its own process group so we can kill all children
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderrStr := stderr.String(); stderrStr != "" {
		if output != "" {
			output += "\n"
		}
		output += stderrStr
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			// Kill entire process group
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
			return Error("tools.bash.timeout", timeout)
		}
		if output == "" {
			output = err.Error()
		}
		return Error("tools.bash.exec_error", err, output)
	}

	if output == "" {
		output = i18n.T("tools.bash.empty_output")
	}

	// Limit output
	maxOutput := 50000
	if len(output) > maxOutput {
		output = output[:maxOutput] + fmt.Sprintf(i18n.T("tools.bash.truncated"), len(output))
	}

	return Success("%s", output)
}

// ExecuteAsync executes a bash command asynchronously, returning a channel with progress events
func (t *BashTool) ExecuteAsync(params map[string]string) <-chan AsyncEvent {
	ch := make(chan AsyncEvent, 32)

	go func() {
		defer close(ch)

		command, ok := params["command"]
		if !ok || command == "" {
			ch <- AsyncEvent{Type: "result", Error: i18n.T("tools.bash.param_command"), Done: true}
			return
		}

		// Timeout
		timeout := t.Timeout
		if ts, ok := params["timeout"]; ok {
			if sec, err := strconv.Atoi(ts); err == nil && sec > 0 {
				timeout = time.Duration(sec) * time.Second
			}
		}

		// Working directory
		workDir := t.DefaultDir
		if dir, ok := params["dir"]; ok && dir != "" {
			workDir = filepath.Clean(dir)
		}

		// Security: blocked commands
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

		// Security: dangerous constructs
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

		// Security: network commands
		if !t.AllowNetwork {
			networkCmds := []string{"curl ", "wget ", "nc ", "ncat ", "ssh ", "scp ", "rsync ", "ftp "}
			for _, nc := range networkCmds {
				if strings.Contains(lowerCmd, nc) {
					ch <- AsyncEvent{Type: "result", Error: i18n.T("security.network_blocked", command), Done: true}
					return
				}
			}
		}

		// Create command with process group for clean kill
		var cmd *exec.Cmd
		if strings.Contains(command, "&&") || strings.Contains(command, "||") || strings.Contains(command, "|") || strings.Contains(command, ";") {
			cmd = exec.Command("bash", "-c", command)
		} else {
			parts := strings.Fields(command)
			cmd = exec.Command(parts[0], parts[1:]...)
		}
		// Put command in its own process group so we can kill all children
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

		// Timeout: graceful then forced kill of entire process group
		var killTimer *time.Timer
		if timeout > 0 {
			killTimer = time.AfterFunc(timeout, func() {
				if cmd.Process != nil {
					// First try SIGTERM (graceful)
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
					// After 3 seconds, force SIGKILL
					time.AfterFunc(3*time.Second, func() {
						_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					})
				}
				stdoutPipe.Close()
				stderrPipe.Close()
			})
		}

		// Read stdout and stderr in parallel
		var stdoutBuf, stderrBuf strings.Builder
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stdoutPipe)
			for scanner.Scan() {
				line := scanner.Text()
				stdoutBuf.WriteString(line)
				stdoutBuf.WriteString("\n")
				ch <- AsyncEvent{Type: "progress", Output: line}
			}
		}()

		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stderrPipe)
			for scanner.Scan() {
				line := scanner.Text()
				stderrBuf.WriteString(line)
				stderrBuf.WriteString("\n")
				ch <- AsyncEvent{Type: "progress", Output: line}
			}
		}()

		// Wait for stream reading to complete
		wg.Wait()

		// Wait for process to complete
		err = cmd.Wait()

		// Stop timeout if process completed on time
		if killTimer != nil {
			killTimer.Stop()
		}

		// Form result
		output := stdoutBuf.String()
		if stderrStr := stderrBuf.String(); stderrStr != "" {
			if output != "" {
				output += "\n"
			}
			output += stderrStr
		}

		if err != nil {
			var errDetail string
			if exitErr, ok := err.(*exec.ExitError); ok {
				errDetail = exitErr.ProcessState.String()
			} else {
				errDetail = err.Error()
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
