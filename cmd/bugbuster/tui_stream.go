package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bugbuster-code/pkg/provider"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// runStream starts streaming in goroutine and sends events via program.Send
// program is passed as parameter to avoid data race —
// goroutine must not read m.program without mutex.
func (m *TUI) runStream(query string, ctx context.Context, program *tea.Program) {
	defer func() {
		if r := recover(); r != nil {
			// Recover from panic — send error event and done signal
			if program != nil {
				program.Send(streamEventMsg{event: provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("internal error: %v", r)}})
				program.Send(streamDoneMsg{})
			}
		}
	}()

	ch, err := m.loop.StreamWithCancel(ctx, query)
	if err != nil {
		if program != nil {
			program.Send(streamEventMsg{event: provider.StreamEvent{Type: provider.EventError, Error: err}})
			program.Send(streamDoneMsg{})
		}
		return
	}

	for event := range ch {
		select {
		case <-ctx.Done():
			if program != nil {
				program.Send(streamDoneMsg{})
			}
			return
		default:
		}
		if program != nil {
			program.Send(streamEventMsg{event: event})
		}
	}
	if program != nil {
		program.Send(streamDoneMsg{})
	}
}

// syncViewport synchronizes viewport with output.
// Creates an independent copy of the output string so the viewport
// does not share the strings.Builder buffer — this eliminates
// data corruption when strings.Builder.String() shares its internal
// buffer and the builder is later modified (WriteString/Reset/growslice).
// Also limits viewport content to prevent memory issues with very large outputs.
func (m *TUI) syncViewport() {
	if m.ready {
		// Trim output if too large — prevent memory corruption
		// from strings.Builder growing too big and causing GC issues
		const maxOutputLines = 10000
		content := m.output.String()
		lines := strings.Count(content, "\n")
		if lines > maxOutputLines {
			// Keep last maxOutputLines lines by truncating output builder
			// This prevents strings.Builder from growing unbounded
			idx := strings.LastIndex(content, "\n")
			count := 0
			lastIdx := len(content)
			for idx >= 0 && count < maxOutputLines {
				lastIdx = idx
				idx = strings.LastIndex(content[:idx], "\n")
				count++
			}
			// Rebuild output with only last maxOutputLines lines
			trimmed := content[lastIdx+1:]
			m.output.Reset()
			m.output.WriteString(trimmed)
			content = trimmed
		}
		// Force independent copy — strings.Builder.String() shares
		// its internal buffer with the builder. If the builder is later
		// modified (WriteString, Reset, growslice), the shared buffer
		// can be reallocated, leaving the viewport string pointing to
		// freed memory. This causes "found bad pointer in Go heap"
		// crashes in the GC when viewport.View() triggers growslice
		// on a string that still references the old buffer.
		//
		// strings.Clone() creates a truly independent copy that does
		// not share memory with the builder.
		safeCopy := strings.Clone(content)
		m.viewport.SetContent(safeCopy)
		m.viewport.GotoBottom()
	}
}

// spinnerCmd returns command for spinner animation
func (m TUI) spinnerCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// autoContinueCmd returns command for autopilot continuation after delay
func autoContinueCmd(input string) tea.Cmd {
	return tea.Tick(autoDelayBetweenIterations, func(t time.Time) tea.Msg {
		return autoContinueMsg{input: input}
	})
}

// toolTickCmd returns command for tool timer update
func (m TUI) toolTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return toolTickMsg{}
	})
}

// addToHistory adds request to history
func (m *TUI) addToHistory(input string) {
	// Do not add duplicates
	if len(m.history) > 0 && m.history[len(m.history)-1] == input {
		return
	}
	// Limit history to 100 elements
	if len(m.history) >= 100 {
		m.history = m.history[1:]
	}
	m.history = append(m.history, input)
}

// updateTextareaHeight updates textarea height and viewport depending on line count
func (m *TUI) updateTextareaHeight() {
	if !m.textarea.DynamicHeight {
		return
	}
	lines := m.textarea.LineCount()
	h := lines + 1 // +1 for prompt/placeholder
	if h < 3 {
		h = 3
	}
	if h > m.textarea.MaxHeight {
		h = m.textarea.MaxHeight
	}
	m.textarea.SetHeight(h)

	// Update viewport height considering new textarea height
	if m.ready && m.height > 0 {
		textareaHeight := h
		if textareaHeight < 3 {
			textareaHeight = 3
		}
		// 5 lines: header (1) + status bar (1) + hints (1) + padding (2)
		viewportHeight := m.height - textareaHeight - 5
		if viewportHeight < 5 {
			viewportHeight = 5
		}
		m.viewport.SetHeight(viewportHeight)
	}
}
// safeViewportView safely renders viewport.View() with panic recovery.
// Returns the rendered string and any error (including panics).
func safeViewportView(v viewport.Model) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("viewport render panic: %v", r)
		}
	}()
	return v.View(), nil
}
