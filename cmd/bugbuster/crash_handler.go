package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"bugbuster-code/pkg/agent"
	"bugbuster-code/pkg/i18n"
)

// crashDir returns the directory for crash logs
var crashDir = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".bugbuster", "crashes")
}

// exitFunc allows overriding os.Exit for tests
var exitFunc = os.Exit

// Global session references for crash recovery
var (
	globalSession    *agent.Session
	globalSessionMgr *agent.SessionManager
	globalLoop       *agent.AgentLoop
)

// crashCleanup is called on normal exit to restore stderr and close crash log
var crashCleanup func() = func() {}

// setupCrashHandler creates the crash directory, checks for previous crashes,
// and redirects stderr to a crash log file. Returns a cleanup function.
func setupCrashHandler() (cleanup func(), prevCrashPath string) {
	dir := crashDir()
	os.MkdirAll(dir, 0755)

	// Check for previous crash
	prevCrashPath = findLatestCrash(dir)

	// Create crash log file
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	crashPath := filepath.Join(dir, fmt.Sprintf("crash_%s.log", timestamp))

	f, err := os.Create(crashPath)
	if err != nil {
		// Can't create crash log — just check for previous crash
		return func() {}, prevCrashPath
	}

	// Write header
	fmt.Fprintf(f, "BugBuster v%s\n", Version)
	fmt.Fprintf(f, "Time: %s\n", time.Now().Format("2006-01-02 15:04:05 MST"))
	fmt.Fprintf(f, "OS: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(f, "Go: %s\n", runtime.Version())
	fmt.Fprintf(f, "\n--- Output ---\n\n")
	f.Sync()

	// Save original stderr fd
	origStderrFd, _ := syscall.Dup(syscall.Stderr)

	// Redirect stderr (fd 2) to crash log file
	syscall.Dup2(int(f.Fd()), syscall.Stderr)

	// Start goroutine to also copy stderr to terminal
	reader, writer, _ := os.Pipe()

	// Redirect fd 2 to writer end of pipe
	syscall.Dup2(int(writer.Fd()), syscall.Stderr)
	writer.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	done := make(chan struct{})
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				// Drain remaining data
				for {
					n, err := reader.Read(buf)
					if n > 0 {
						f.Write(buf[:n])
						if origStderrFd >= 0 {
							syscall.Write(origStderrFd, buf[:n])
						}
					}
					if err != nil {
						break
					}
				}
				return
			default:
				n, err := reader.Read(buf)
				if n > 0 {
					f.Write(buf[:n])
					if origStderrFd >= 0 {
						syscall.Write(origStderrFd, buf[:n])
					}
				}
				if err != nil {
					return
				}
			}
		}
	}()

	cleanup = func() {
		// Restore original stderr
		if origStderrFd >= 0 {
			syscall.Dup2(origStderrFd, syscall.Stderr)
			syscall.Close(origStderrFd)
		}

		// Signal goroutine to stop
		close(done)
		reader.Close()

		// Wait for goroutine to finish
		wg.Wait()

		// Close crash log file
		f.Close()

		// If crash log is small (no crash), delete it
		info, err := os.Stat(crashPath)
		if err == nil && info.Size() < 500 {
			os.Remove(crashPath)
		}
	}

	// Store cleanup function globally so writeCrashLog can call it
	crashCleanup = cleanup

	return cleanup, prevCrashPath
}

// writeCrashLog writes a crash report to a file and prints a user-friendly
// message instead of the full stack trace.
func writeCrashLog(r interface{}) {
	dir := crashDir()
	os.MkdirAll(dir, 0755)

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	crashPath := filepath.Join(dir, fmt.Sprintf("crash_%s.log", timestamp))

	// Collect stack trace
	buf := make([]byte, 1<<20) // 1MB
	n := runtime.Stack(buf, true)
	stack := string(buf[:n])

	// Build crash report
	var sb strings.Builder
	sb.WriteString("BugBuster Crash Report\n")
	sb.WriteString("======================\n")
	sb.WriteString(fmt.Sprintf("Time: %s\n", time.Now().Format("2006-01-02 15:04:05 MST")))
	sb.WriteString(fmt.Sprintf("Version: %s\n", Version))
	sb.WriteString(fmt.Sprintf("OS: %s/%s\n", runtime.GOOS, runtime.GOARCH))
	sb.WriteString(fmt.Sprintf("Go: %s\n", runtime.Version()))
	sb.WriteString(fmt.Sprintf("\nPanic: %v\n", r))
	sb.WriteString(fmt.Sprintf("\nStack Trace:\n%s\n", stack))

	// Write crash log
	if err := os.WriteFile(crashPath, []byte(sb.String()), 0644); err != nil {
		fmt.Fprintf(os.Stdout, "CRASH: %v\n%s\n", r, stack)
		fmt.Fprintf(os.Stdout, "Failed to write crash log: %v\n", err)
		exitFunc(1)
	}

	// Save session before exit (crash recovery)
	if globalSession != nil && globalSessionMgr != nil {
		if globalLoop != nil && globalLoop.Context != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Context access panicked — save without messages
					}
				}()
				globalSession.Messages = globalLoop.Context.GetMessages()
			}()
		}
		if err := globalSessionMgr.SaveSessionMessages(globalSession); err == nil {
			fmt.Fprintf(os.Stdout, "\n  ✅ %s\n", i18n.T("cli_success.session_saved", globalSession.ID))
		}
	}

	// Print user-friendly message to stdout (stderr is redirected)
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "╔══════════════════════════════════════════╗\n")
	fmt.Fprintf(os.Stdout, "║  %s          ║\n", i18n.T("cli.crash_title"))
	fmt.Fprintf(os.Stdout, "╚══════════════════════════════════════════╝\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "%s: %s\n", i18n.T("cli.crash_log_saved"), crashPath)
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "%s\n", i18n.T("cli.crash_report"))
	fmt.Fprintf(os.Stdout, "  https://github.com/bugbuster-code/bugbuster/issues\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "%s\n", i18n.T("cli.crash_restore"))
	fmt.Fprintf(os.Stdout, "  bugbuster --session <session-id>\n")
	fmt.Fprintf(os.Stdout, "\n")

	// Restore stderr before exit so the friendly message is visible
	crashCleanup()

	exitFunc(1)
}

// findLatestCrash finds the most recent crash log, if any
func findLatestCrash(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	var latest string
	var latestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "crash_") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latest = filepath.Join(dir, entry.Name())
		}
	}

	return latest
}

// showPreviousCrashNotification shows a notification about a previous crash
func showPreviousCrashNotification(crashPath string) {
	data, err := os.ReadFile(crashPath)
	if err != nil {
		return
	}

	content := string(data)

	// Extract panic message or first error line
	var panicMsg string
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "Panic:") {
			panicMsg = strings.TrimPrefix(line, "Panic:")
			panicMsg = strings.TrimSpace(panicMsg)
			break
		}
	}
	if panicMsg == "" {
		// Try to find "runtime error" or "fatal error" line
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "runtime error:") || strings.HasPrefix(line, "fatal error:") {
				panicMsg = line
				break
			}
		}
	}

	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "╔══════════════════════════════════════════╗\n")
	fmt.Fprintf(os.Stdout, "║  %s                 ║\n", i18n.T("cli.crash_previous"))
	fmt.Fprintf(os.Stdout, "╚══════════════════════════════════════════╝\n")
	fmt.Fprintf(os.Stdout, "\n")
	if panicMsg != "" {
		fmt.Fprintf(os.Stdout, "  %s: %s\n", i18n.T("cli.crash_error"), panicMsg)
	}
	fmt.Fprintf(os.Stdout, "  %s: %s\n", i18n.T("cli.crash_log"), crashPath)
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "  %s\n", i18n.T("cli.crash_dismiss"))
	fmt.Fprintf(os.Stdout, "\n")
}

// clearCrashLogs removes all crash logs
func clearCrashLogs() error {
	dir := crashDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "crash_") {
			os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
	return nil
}

// cleanupEmptyCrashLog removes crash log files if no crash occurred
func cleanupEmptyCrashLog() {
	dir := crashDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "crash_") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		// If crash log only contains the header (no actual crash), remove it
		if info.Size() < 500 { // Header is ~200 bytes, crash dump is much larger
			os.Remove(path)
		}
	}
}

// ensureNoUnusedImports
var _ = io.ReadFull