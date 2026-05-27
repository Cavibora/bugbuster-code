package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"bugbuster-code/pkg/i18n"
)

// BackgroundProcess represents a running background process
type BackgroundProcess struct {
	ID        int
	PID       int
	Command   string
	Dir       string
	LogFile   string
	StartTime time.Time
	Running   atomic.Bool
	ExitCode  atomic.Int32
}

// BackgroundTool manages background processes
type BackgroundTool struct {
	mu        sync.Mutex
	processes map[int]*BackgroundProcess
	nextID    atomic.Int64
	logDir    string
	wg        sync.WaitGroup
}

// NewBackgroundTool creates a new background process manager
func NewBackgroundTool(logDir string) *BackgroundTool {
	if logDir == "" {
		home, _ := os.UserHomeDir()
		logDir = filepath.Join(home, ".bugbuster", "bg_logs")
	}
	bt := &BackgroundTool{
		processes: make(map[int]*BackgroundProcess),
		logDir:    logDir,
	}
	bt.nextID.Store(0)
	os.MkdirAll(logDir, 0755)
	return bt
}

func (t *BackgroundTool) Name() string { return "background" }

func (t *BackgroundTool) Description() string {
	return i18n.T("tools.background.description")
}

// monitorProcess polls the process PID to detect when it exits.
// This avoids calling cmd.Wait() which would race with the caller's goroutine.
func (t *BackgroundTool) monitorProcess(proc *BackgroundProcess, f *os.File) {
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		defer func() {
			if f != nil {
				f.Close()
			}
		}()
		for {
			if err := syscall.Kill(proc.PID, 0); err != nil {
				proc.Running.Store(false)
				proc.ExitCode.Store(-1)
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()
}

func (t *BackgroundTool) Execute(params map[string]string) ToolResult {
	command, ok := params["command"]
	if !ok || command == "" {
		return Error("tools.background.param_command")
	}

	workDir := params["dir"]
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	id := int(t.nextID.Add(1))
	logFile := filepath.Join(t.logDir, fmt.Sprintf("bg_%d.log", id))

	var cmd *exec.Cmd
	if strings.Contains(command, "&&") || strings.Contains(command, "||") || strings.Contains(command, "|") || strings.Contains(command, ";") {
		cmd = exec.Command("bash", "-c", command)
	} else {
		parts := strings.Fields(command)
		cmd = exec.Command(parts[0], parts[1:]...)
	}
	cmd.Dir = workDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	f, err := os.Create(logFile)
	if err != nil {
		return Error("tools.background.log_create_error", err)
	}

	cmd.Stdout = f
	cmd.Stderr = f

	if err := cmd.Start(); err != nil {
		f.Close()
		return Error("tools.background.start_error", err)
	}

	proc := &BackgroundProcess{
		ID:        id,
		PID:       cmd.Process.Pid,
		Command:   command,
		Dir:       workDir,
		LogFile:   logFile,
		StartTime: time.Now(),
	}
	proc.Running.Store(true)

	t.mu.Lock()
	t.processes[id] = proc
	t.mu.Unlock()

	// Monitor process — poll PID instead of cmd.Wait() to avoid race
	t.monitorProcess(proc, f)

	return Success(i18n.T("tools.background.started", id, cmd.Process.Pid, logFile))
}

// GetProcess returns a process by ID
func (t *BackgroundTool) GetProcess(id int) (*BackgroundProcess, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p, ok := t.processes[id]
	return p, ok
}

// ListProcesses returns all background processes
func (t *BackgroundTool) ListProcesses() []*BackgroundProcess {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]*BackgroundProcess, 0, len(t.processes))
	for _, p := range t.processes {
		result = append(result, p)
	}
	return result
}

// KillProcess kills a background process
func (t *BackgroundTool) KillProcess(id int) error {
	t.mu.Lock()
	p, ok := t.processes[id]
	t.mu.Unlock()

	if !ok {
		return fmt.Errorf("process %d not found", id)
	}

	if !p.Running.Load() {
		return fmt.Errorf("process %d is not running", id)
	}

	// Kill entire process group
	if err := syscall.Kill(-p.PID, syscall.SIGTERM); err != nil {
		syscall.Kill(-p.PID, syscall.SIGKILL)
	}

	return nil
}

// ReadLogs reads the last N lines of a process log file
func (t *BackgroundTool) ReadLogs(id int, lines int) (string, error) {
	t.mu.Lock()
	p, ok := t.processes[id]
	t.mu.Unlock()

	if !ok {
		return "", fmt.Errorf("process %d not found", id)
	}

	data, err := os.ReadFile(p.LogFile)
	if err != nil {
		return "", err
	}

	content := string(data)
	if lines <= 0 {
		return content, nil
	}

	allLines := strings.Split(content, "\n")
	if len(allLines) > lines {
		allLines = allLines[len(allLines)-lines:]
	}
	return strings.Join(allLines, "\n"), nil
}

// MoveToBackground moves a running command to background management.
// Used when bash command times out — the process continues running in background.
// IMPORTANT: This does NOT call cmd.Wait() — the caller already has a goroutine doing that.
func (t *BackgroundTool) MoveToBackground(cmd *exec.Cmd, stdout, stderr string, startTime time.Time) (int, error) {
	if cmd == nil || cmd.Process == nil {
		return 0, fmt.Errorf("no process to move to background")
	}

	id := int(t.nextID.Add(1))
	logFile := filepath.Join(t.logDir, fmt.Sprintf("bg_%d.log", id))

	// Write existing output to log file
	if stdout != "" || stderr != "" {
		f, err := os.Create(logFile)
		if err != nil {
			return 0, fmt.Errorf("failed to create log file: %w", err)
		}
		if stdout != "" {
			f.WriteString("=== STDOUT (before background) ===\n")
			f.WriteString(stdout)
			if !strings.HasSuffix(stdout, "\n") {
				f.WriteString("\n")
			}
		}
		if stderr != "" {
			f.WriteString("=== STDERR (before background) ===\n")
			f.WriteString(stderr)
			if !strings.HasSuffix(stderr, "\n") {
				f.WriteString("\n")
			}
		}
		f.Close()
	}

	// Open log file for appending (further output will be written by the caller's goroutine)
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, fmt.Errorf("failed to open log file: %w", err)
	}

	// Replace stdout/stderr pipes with file — the caller's goroutine will write to this file
	cmd.Stdout = f
	cmd.Stderr = f

	proc := &BackgroundProcess{
		ID:        id,
		PID:       cmd.Process.Pid,
		Command:   strings.Join(cmd.Args, " "),
		Dir:       cmd.Dir,
		LogFile:   logFile,
		StartTime: startTime,
	}
	proc.Running.Store(true)

	t.mu.Lock()
	t.processes[id] = proc
	t.mu.Unlock()

	// Monitor process — poll PID instead of cmd.Wait() to avoid race with caller's goroutine
	t.monitorProcess(proc, f)

	return id, nil
}

// Cleanup removes log files for stopped processes
func (t *BackgroundTool) Cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for id, p := range t.processes {
		if !p.Running.Load() {
			os.Remove(p.LogFile)
			delete(t.processes, id)
		}
	}
}

// WaitAll waits for all monitor goroutines to finish
func (t *BackgroundTool) WaitAll() {
	t.wg.Wait()
}

// --- PS Tool ---

type PSTool struct {
	bg *BackgroundTool
}

func NewPSTool(bg *BackgroundTool) *PSTool {
	return &PSTool{bg: bg}
}

func (t *PSTool) Name() string { return "ps" }

func (t *PSTool) Description() string {
	return i18n.T("tools.ps.description")
}

func (t *PSTool) Execute(params map[string]string) ToolResult {
	processes := t.bg.ListProcesses()
	if len(processes) == 0 {
		return Success(i18n.T("tools.ps.no_processes"))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-4s %-8s %-8s %-10s %s\n", "ID", "PID", "STATUS", "UPTIME", "COMMAND"))
	for _, p := range processes {
		status := "running"
		if !p.Running.Load() {
			status = fmt.Sprintf("exit(%d)", p.ExitCode.Load())
		}
		uptime := time.Since(p.StartTime).Truncate(time.Second)
		cmd := p.Command
		if len(cmd) > 60 {
			cmd = cmd[:57] + "..."
		}
		sb.WriteString(fmt.Sprintf("%-4d %-8d %-8s %-10s %s\n", p.ID, p.PID, status, uptime, cmd))
	}

	return Success(sb.String())
}

// --- Logs Tool ---

type LogsTool struct {
	bg *BackgroundTool
}

func NewLogsTool(bg *BackgroundTool) *LogsTool {
	return &LogsTool{bg: bg}
}

func (t *LogsTool) Name() string { return "logs" }

func (t *LogsTool) Description() string {
	return i18n.T("tools.logs.description")
}

func (t *LogsTool) Execute(params map[string]string) ToolResult {
	idStr, ok := params["id"]
	if !ok || idStr == "" {
		return Error("tools.logs.param_id")
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return Error("tools.logs.invalid_id", idStr)
	}

	lines := 50
	if l, ok := params["lines"]; ok {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			lines = n
		}
	}

	content, err := t.bg.ReadLogs(id, lines)
	if err != nil {
		return Error("tools.logs.read_error", err)
	}

	if content == "" {
		return Success(i18n.T("tools.logs.empty", id))
	}

	return Success(content)
}

// --- KillProcess Tool ---

type KillProcessTool struct {
	bg *BackgroundTool
}

func NewKillProcessTool(bg *BackgroundTool) *KillProcessTool {
	return &KillProcessTool{bg: bg}
}

func (t *KillProcessTool) Name() string { return "kill" }

func (t *KillProcessTool) Description() string {
	return i18n.T("tools.kill.description")
}

func (t *KillProcessTool) Execute(params map[string]string) ToolResult {
	idStr, ok := params["id"]
	if !ok || idStr == "" {
		return Error("tools.kill.param_id")
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return Error("tools.kill.invalid_id", idStr)
	}

	if err := t.bg.KillProcess(id); err != nil {
		return Error("tools.kill.error", err)
	}

	return Success(i18n.T("tools.kill.success", id))
}

// Parameters for each tool

func (t *BackgroundTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The command to run in the background (non-blocking)",
			},
			"dir": map[string]any{
				"type":        "string",
				"description": "Working directory for the command",
			},
		},
		"required": []string{"command"},
	}
}

func (t *PSTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *LogsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "Background process ID",
			},
			"lines": map[string]any{
				"type":        "string",
				"description": "Number of last lines to show (default: 50)",
			},
		},
		"required": []string{"id"},
	}
}

func (t *KillProcessTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "Background process ID to kill",
			},
		},
		"required": []string{"id"},
	}
}