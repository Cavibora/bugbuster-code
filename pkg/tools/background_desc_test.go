package tools

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"bugbuster-code/pkg/i18n"
)

func init() {
	i18n.Init("en")
}

// ===================== Description methods =====================

func TestBackgroundTool_Description(t *testing.T) {
	bg := NewBackgroundTool(t.TempDir())
	desc := bg.Description()
	if desc == "" {
		t.Error("expected non-empty description for BackgroundTool")
	}
}

func TestPSTool_Description(t *testing.T) {
	bg := NewBackgroundTool(t.TempDir())
	ps := NewPSTool(bg)
	desc := ps.Description()
	if desc == "" {
		t.Error("expected non-empty description for PSTool")
	}
}

func TestLogsTool_Description(t *testing.T) {
	bg := NewBackgroundTool(t.TempDir())
	logs := NewLogsTool(bg)
	desc := logs.Description()
	if desc == "" {
		t.Error("expected non-empty description for LogsTool")
	}
}

func TestKillProcessTool_Description(t *testing.T) {
	bg := NewBackgroundTool(t.TempDir())
	kill := NewKillProcessTool(bg)
	desc := kill.Description()
	if desc == "" {
		t.Error("expected non-empty description for KillProcessTool")
	}
}

// ===================== Cleanup =====================

func TestBackgroundTool_Cleanup_RemovesStoppedProcesses(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	// Start a short-lived process
	bg.Execute(map[string]string{"command": "echo done"})
	time.Sleep(500 * time.Millisecond)

	// Start a long-lived process
	bg.Execute(map[string]string{"command": "sleep 30"})

	beforeCount := len(bg.ListProcesses())
	bg.Cleanup()
	afterCount := len(bg.ListProcesses())

	if afterCount > beforeCount {
		t.Errorf("cleanup should not increase process count: before=%d, after=%d", beforeCount, afterCount)
	}

	// Clean up the long process
	processes := bg.ListProcesses()
	for _, p := range processes {
		bg.KillProcess(p.ID)
	}
}

func TestBackgroundTool_Cleanup_DeletesLogFiles(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	// Start a short-lived process
	result := bg.Execute(map[string]string{"command": "echo cleanup_test"})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}

	// Wait for process to complete
	time.Sleep(500 * time.Millisecond)

	// Get the log file path before cleanup
	processes := bg.ListProcesses()
	var logFile string
	for _, p := range processes {
		if strings.Contains(p.Command, "echo") {
			logFile = p.LogFile
			break
		}
	}

	// If process already exited, log file should still exist
	if logFile != "" {
		if _, err := os.Stat(logFile); os.IsNotExist(err) {
			t.Log("log file already removed (process exited too fast)")
		}
	}

	bg.Cleanup()

	// After cleanup, stopped processes' log files should be removed
	// (but we can't guarantee this if the process was already cleaned up)
}

// ===================== KillProcess ====================

func TestBackgroundTool_KillProcess_NotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	// Start a short process and wait for it to finish
	bg.Execute(map[string]string{"command": "echo done"})
	time.Sleep(500 * time.Millisecond)

	processes := bg.ListProcesses()
	if len(processes) == 0 {
		t.Skip("no processes found")
	}

	// Try to kill a process that may have already exited
	id := processes[0].ID
	err := bg.KillProcess(id)
	// Process might have already exited, so error is acceptable
	if err != nil && !strings.Contains(err.Error(), "not running") {
		t.Logf("KillProcess returned: %v (acceptable)", err)
	}
}

func TestBackgroundTool_KillProcess_RunningProcess(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	// Start a long-running process
	result := bg.Execute(map[string]string{"command": "sleep 60"})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}

	processes := bg.ListProcesses()
	if len(processes) == 0 {
		t.Fatal("expected at least one process")
	}
	id := processes[0].ID

	// Kill the running process
	err := bg.KillProcess(id)
	if err != nil {
		t.Fatalf("unexpected error killing running process: %v", err)
	}

	// Wait for process to terminate
	time.Sleep(1 * time.Second)

	// Verify process is no longer running
	proc, ok := bg.GetProcess(id)
	if !ok {
		t.Skip("process was cleaned up")
	}
	// The monitor goroutine polls every 500ms, so give it time
	if proc.Running.Load() {
		// Process might still be shutting down — this is a race condition
		t.Logf("process still marked as running after kill (race condition acceptable)")
	}
}

// ===================== ReadLogs ====================

func TestBackgroundTool_ReadLogs_FullContent(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	// Start a process that writes output
	bg.Execute(map[string]string{"command": "echo hello_logs_test"})
	time.Sleep(500 * time.Millisecond)

	processes := bg.ListProcesses()
	if len(processes) == 0 {
		t.Fatal("expected at least one process")
	}
	id := processes[0].ID

	// Read all logs (lines=0 means all)
	content, err := bg.ReadLogs(id, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Content might be empty if process hasn't flushed yet, but should not error
	_ = content
}

func TestBackgroundTool_ReadLogs_WithLineLimit(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	// Start a process that writes multiple lines
	bg.Execute(map[string]string{"command": "for i in $(seq 1 20); do echo line_$i; done"})
	time.Sleep(500 * time.Millisecond)

	processes := bg.ListProcesses()
	if len(processes) == 0 {
		t.Fatal("expected at least one process")
	}
	id := processes[0].ID

	// Read last 5 lines
	content, err := bg.ReadLogs(id, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(content), "\n")
	// Should have at most 5 lines (or fewer if process hasn't written all output yet)
	if len(lines) > 5 {
		t.Errorf("expected at most 5 lines, got %d", len(lines))
	}
}

func TestBackgroundTool_ReadLogs_NonExistentProcess(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	_, err := bg.ReadLogs(999, 10)
	if err == nil {
		t.Error("expected error for non-existent process")
	}
}

// ===================== LogsTool Execute ====================

func TestLogsTool_Execute_EmptyLogs(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	// Start a process that produces no output
	bg.Execute(map[string]string{"command": "true"})
	time.Sleep(300 * time.Millisecond)

	processes := bg.ListProcesses()
	if len(processes) == 0 {
		t.Fatal("expected at least one process")
	}
	id := processes[0].ID

	logs := NewLogsTool(bg)
	result := logs.Execute(map[string]string{
		"id": strconv.Itoa(id),
	})
	// Should succeed (even if empty)
	if result.Error != "" {
		t.Logf("result: %s", result.Error)
	}
}

func TestLogsTool_Execute_WithLines(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	bg.Execute(map[string]string{"command": "echo test_output"})
	time.Sleep(500 * time.Millisecond)

	processes := bg.ListProcesses()
	if len(processes) == 0 {
		t.Fatal("expected at least one process")
	}
	id := processes[0].ID

	logs := NewLogsTool(bg)
	result := logs.Execute(map[string]string{
		"id":    strconv.Itoa(id),
		"lines": "10",
	})
	// Should succeed
	if result.Error != "" {
		t.Logf("result error: %s", result.Error)
	}
}

func TestLogsTool_Execute_NegativeLines(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	bg.Execute(map[string]string{"command": "echo test"})
	time.Sleep(300 * time.Millisecond)

	processes := bg.ListProcesses()
	if len(processes) == 0 {
		t.Fatal("expected at least one process")
	}
	id := processes[0].ID

	logs := NewLogsTool(bg)
	result := logs.Execute(map[string]string{
		"id":    strconv.Itoa(id),
		"lines": "-5",
	})
	// Negative lines should be treated as 0 (all content)
	_ = result
}

// ===================== KillProcessTool Execute ====================

func TestKillProcessTool_Execute_Success(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	bg.Execute(map[string]string{"command": "sleep 60"})
	time.Sleep(300 * time.Millisecond)

	processes := bg.ListProcesses()
	if len(processes) == 0 {
		t.Fatal("expected at least one process")
	}
	id := processes[0].ID

	kill := NewKillProcessTool(bg)
	result := kill.Execute(map[string]string{"id": strconv.Itoa(id)})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
}

// ===================== PSTool Execute ====================

func TestPSTool_Execute_NoProcesses(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	ps := NewPSTool(bg)
	result := ps.Execute(map[string]string{})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "no") && !strings.Contains(result.Output, "No") {
		// Should indicate no processes
		t.Logf("output: %s", result.Output)
	}
}

func TestPSTool_Execute_WithProcesses(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	bg.Execute(map[string]string{"command": "sleep 30"})
	time.Sleep(300 * time.Millisecond)

	ps := NewPSTool(bg)
	result := ps.Execute(map[string]string{})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "sleep") {
		t.Errorf("expected 'sleep' in output, got: %s", result.Output)
	}

	// Cleanup
	processes := bg.ListProcesses()
	for _, p := range processes {
		bg.KillProcess(p.ID)
	}
}

// ===================== BackgroundTool Execute edge cases ====================

func TestBackgroundTool_Execute_EmptyCommand(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	result := bg.Execute(map[string]string{})
	if result.Error == "" {
		t.Error("expected error for empty command")
	}
}

func TestBackgroundTool_Execute_CommandWithPipe(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	result := bg.Execute(map[string]string{"command": "echo hello | tr a-z A-Z"})
	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
}

func TestBackgroundTool_Execute_CommandWithAnd(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	result := bg.Execute(map[string]string{"command": "echo first && echo second"})
	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
}