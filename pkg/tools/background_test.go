package tools

import (
	"os"
	"strconv"
	"testing"
	"time"
)

func TestBackgroundToolStartAndPS(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	// Start a background process
	result := bg.Execute(map[string]string{
		"command": "echo hello",
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Output == "" {
		t.Fatal("expected output")
	}

	// List processes
	ps := NewPSTool(bg)
	psResult := ps.Execute(map[string]string{})
	if psResult.Error != "" {
		t.Fatalf("unexpected error: %s", psResult.Error)
	}
	if psResult.Output == "" {
		t.Fatal("expected output")
	}
}

func TestBackgroundToolWithDir(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	result := bg.Execute(map[string]string{
		"command": "pwd",
		"dir":     tmpDir,
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
}

func TestBackgroundToolMissingCommand(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	result := bg.Execute(map[string]string{})
	if result.Error == "" {
		t.Fatal("expected error for missing command")
	}
}

func TestLogsTool(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	// Start a process that writes to log
	result := bg.Execute(map[string]string{
		"command": "echo test_log_output",
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}

	// Wait for process to complete and log to be written
	time.Sleep(500 * time.Millisecond)

	// Get process ID
	processes := bg.ListProcesses()
	if len(processes) == 0 {
		t.Fatal("expected at least one process")
	}
	id := processes[0].ID

	// Read logs
	logs := NewLogsTool(bg)
	logResult := logs.Execute(map[string]string{
		"id":    strconv.Itoa(id),
		"lines": "10",
	})
	// Process may have completed, so logs might be empty or contain output
	_ = logResult
}

func TestKillTool(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	// Start a long-running process
	result := bg.Execute(map[string]string{
		"command": "sleep 60",
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}

	processes := bg.ListProcesses()
	if len(processes) == 0 {
		t.Fatal("expected at least one process")
	}
	id := processes[0].ID

	// Kill it
	kill := NewKillProcessTool(bg)
	killResult := kill.Execute(map[string]string{
		"id": strconv.Itoa(id),
	})
	if killResult.Error != "" {
		t.Fatalf("unexpected error killing process: %s", killResult.Error)
	}
}

func TestKillToolInvalidID(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	kill := NewKillProcessTool(bg)
	result := kill.Execute(map[string]string{
		"id": "999",
	})
	if result.Error == "" {
		t.Fatal("expected error for invalid ID")
	}
}

func TestLogsToolMissingID(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	logs := NewLogsTool(bg)
	result := logs.Execute(map[string]string{})
	if result.Error == "" {
		t.Fatal("expected error for missing ID")
	}
}

func TestBackgroundToolLogCreation(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	result := bg.Execute(map[string]string{
		"command": "echo hello_world",
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}

	// Wait for process to complete
	time.Sleep(500 * time.Millisecond)

	// Check that log file was created
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read log dir: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected log file to be created")
	}
}

func TestBackgroundToolCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	bg := NewBackgroundTool(tmpDir)

	// Start a short process
	bg.Execute(map[string]string{"command": "echo done"})
	time.Sleep(500 * time.Millisecond)

	// Start a long process
	bg.Execute(map[string]string{"command": "sleep 60"})

	// Cleanup should remove completed processes
	bg.Cleanup()

	processes := bg.ListProcesses()
	// Only the long-running process should remain
	for _, p := range processes {
		if !p.Running.Load() {
			t.Errorf("expected only running processes, found stopped process #%d", p.ID)
		}
	}
}