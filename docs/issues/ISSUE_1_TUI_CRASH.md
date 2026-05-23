# GitHub Issues for BugBuster Code Alpha

## Issue 1: TUI crash — memory corruption in View()

**Labels:** `bug`, `priority: critical`, `area: tui`

**Title:** TUI crash: memory corruption (bad pointer in Go heap) during streaming

**Body:**

### Description

When using TUI mode during active streaming (especially with large outputs), BugBuster crashes with a Go runtime fatal error:

```
runtime: pointer 0x1400090def8 to unused region of span
fatal error: found bad pointer in Go heap (incorrect use of unsafe or cgo?)
```

### Stack Trace

```
main.TUI.View() →
  charm.land/bubbles/v2/viewport.Model.View() →
    charm.land/lipgloss/v2.Style.Render() →
      charm.land/lipgloss/v2.alignTextHorizontal() →
        strings.(*Builder).WriteString() →
          runtime.growslice() → GC finds bad pointer
```

### Root Cause

Data race on `m.output` (*strings.Builder). The `m.output` is modified from `Update()` (under mutex) but `View()` reads it concurrently from Bubble Tea's render goroutine. When `m.output` grows (via `WriteString`), `runtime.growslice` reallocates the internal buffer, and GC scans the old buffer finding stale pointers.

### Reproduction

1. Start BugBuster in TUI mode: `bugbuster --tui`
2. Ask a question that generates a long response with tool calls
3. Wait for streaming to produce ~100K+ characters of output
4. Crash occurs during GC mark phase

### Workaround

Use CLI mode (default) instead of TUI mode.

### Fix Plan

- Copy `m.output.String()` to a separate string before passing to `viewport.SetContent()`
- Add `sync.RWMutex` to protect `m.output` reads in `View()`
- Limit viewport content size to prevent huge allocations
