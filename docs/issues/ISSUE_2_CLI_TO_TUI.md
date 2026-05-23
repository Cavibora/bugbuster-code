## Issue 2: CLI→TUI switch hangs until Enter pressed

**Labels:** `bug`, `priority: medium`, `area: tui`, `area: cli`

**Title:** Switching from CLI to TUI mode (/tui) hangs until Enter is pressed

**Body:**

### Description

When switching from CLI mode to TUI mode using the `/tui` command, the application hangs and waits for the user to press Enter before the TUI interface appears.

### Steps to Reproduce

1. Start BugBuster in CLI mode: `bugbuster`
2. Type `/tui` and press Enter
3. Observe: application appears frozen
4. Press Enter again
5. TUI mode starts

### Root Cause

The `readline` library spawns several goroutines that hold `os.Stdin`:
- `CancelableStdin.ioloop` — reads from stdin
- `Operation.ioloop` — processes input events
- `DefaultOnWidthChanged` — monitors terminal width

When `rl.Close()` is called, these goroutines are NOT terminated. They continue holding `os.Stdin`, preventing Bubble Tea from reading input.

Current workaround attempts:
- `drainStdin()` — reads pending bytes from stdin
- `restoreTerminalToNormal()` — runs `stty sane`
- `syscall.Dup2` — replaces fd 0 with `/dev/null`, then restores from `/dev/tty`
- `tea.WithInput(ttyInput)` — passes `/dev/tty` directly to Bubble Tea

None of these fully solve the problem because readline goroutines remain active.

### Fix Plan

- Replace `github.com/chzyer/readline` with a library that properly cleans up goroutines
- Or: implement custom input handling without readline
- Or: use `exec.Command("stty")` to fully reset terminal state before Bubble Tea init
