package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrashDir(t *testing.T) {
	dir := crashDir()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".bugbuster", "crashes")
	if dir != expected {
		t.Errorf("crashDir() = %s, want %s", dir, expected)
	}
}

func TestWriteCrashLog(t *testing.T) {
	tmpDir := t.TempDir()

	origCrashDir := crashDir
	defer func() { crashDir = origCrashDir }()
	crashDir = func() string { return tmpDir }

	var exited bool
	origExitFunc := exitFunc
	defer func() { exitFunc = origExitFunc }()
	exitFunc = func(code int) { exited = true }

	writeCrashLog("test panic: something went wrong")

	if !exited {
		t.Error("exitFunc was not called")
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read crash dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("No crash log created")
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("Failed to read crash log: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "BugBuster Crash Report") {
		t.Error("Crash log missing header")
	}
	if !strings.Contains(content, "test panic: something went wrong") {
		t.Error("Crash log missing panic message")
	}
	if !strings.Contains(content, "Stack Trace:") {
		t.Error("Crash log missing stack trace")
	}
}

func TestFindLatestCrash(t *testing.T) {
	tmpDir := t.TempDir()

	result := findLatestCrash(tmpDir)
	if result != "" {
		t.Errorf("Expected empty result, got %s", result)
	}

	crashFile := filepath.Join(tmpDir, "crash_2024-01-01_12-00-00.log")
	os.WriteFile(crashFile, []byte("test crash"), 0644)

	result = findLatestCrash(tmpDir)
	if result != crashFile {
		t.Errorf("Expected %s, got %s", crashFile, result)
	}
}

func TestClearCrashLogs(t *testing.T) {
	tmpDir := t.TempDir()

	origCrashDir := crashDir
	defer func() { crashDir = origCrashDir }()
	crashDir = func() string { return tmpDir }

	crashFile1 := filepath.Join(tmpDir, "crash_2024-01-01_12-00-00.log")
	crashFile2 := filepath.Join(tmpDir, "crash_2024-01-02_12-00-00.log")
	nonCrashFile := filepath.Join(tmpDir, "other_file.txt")

	os.WriteFile(crashFile1, []byte("crash1"), 0644)
	os.WriteFile(crashFile2, []byte("crash2"), 0644)
	os.WriteFile(nonCrashFile, []byte("other"), 0644)

	err := clearCrashLogs()
	if err != nil {
		t.Fatalf("clearCrashLogs() error: %v", err)
	}

	entries, _ := os.ReadDir(tmpDir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "crash_") {
			t.Errorf("Crash log not deleted: %s", entry.Name())
		}
	}

	if _, err := os.Stat(nonCrashFile); os.IsNotExist(err) {
		t.Error("Non-crash file was deleted")
	}
}

func TestSetupCrashHandler(t *testing.T) {
	tmpDir := t.TempDir()

	origCrashDir := crashDir
	defer func() { crashDir = origCrashDir }()
	crashDir = func() string { return tmpDir }

	// No previous crash
	cleanup, prevCrash := setupCrashHandler()
	if prevCrash != "" {
		t.Errorf("Expected empty result, got %s", prevCrash)
	}
	cleanup()

	// Create a crash log (must be > 200 bytes to survive cleanupEmptyCrashLog)
	crashFile := filepath.Join(tmpDir, "crash_2024-01-01_12-00-00.log")
	largeContent := "Panic: test crash\nStack Trace:\n" + strings.Repeat("test line\n", 30)
	os.WriteFile(crashFile, []byte(largeContent), 0644)

	// Should find the crash log
	cleanup, prevCrash = setupCrashHandler()
	if prevCrash != crashFile {
		t.Errorf("Expected %s, got %s", crashFile, prevCrash)
	}
	cleanup()
}

func TestCleanupEmptyCrashLog(t *testing.T) {
	tmpDir := t.TempDir()

	origCrashDir := crashDir
	defer func() { crashDir = origCrashDir }()
	crashDir = func() string { return tmpDir }

	emptyFile := filepath.Join(tmpDir, "crash_2024-01-01_12-00-00.log")
	os.WriteFile(emptyFile, []byte("small"), 0644)

	realFile := filepath.Join(tmpDir, "crash_2024-01-02_12-00-00.log")
	largeContent := strings.Repeat("x", 1000)
	os.WriteFile(realFile, []byte(largeContent), 0644)

	cleanupEmptyCrashLog()

	if _, err := os.Stat(emptyFile); !os.IsNotExist(err) {
		t.Error("Empty crash log was not deleted")
	}

	if _, err := os.Stat(realFile); os.IsNotExist(err) {
		t.Error("Real crash log was deleted")
	}
}