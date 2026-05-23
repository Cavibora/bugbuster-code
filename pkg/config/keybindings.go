package config

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Action is an action identifier for key bindings
type Action string

const (
	ActionSend        Action = "send"
	ActionNewline     Action = "newline"
	ActionCancel      Action = "cancel"
	ActionInterrupt   Action = "interrupt"
	ActionHistoryUp   Action = "history_up"
	ActionHistoryDown Action = "history_down"
	ActionScrollUp    Action = "scroll_up"
	ActionScrollDown  Action = "scroll_down"
)

// KeyBindings is configurable key bindings (YAML)
type KeyBindings struct {
	Send        []string `yaml:"send"`
	Newline     []string `yaml:"newline"`
	Cancel      []string `yaml:"cancel"`
	Interrupt   []string `yaml:"interrupt"`
	HistoryUp   []string `yaml:"history_up"`
	HistoryDown []string `yaml:"history_down"`
	ScrollUp    []string `yaml:"scroll_up"`
	ScrollDown  []string `yaml:"scroll_down"`
}

// DefaultKeyBindings returns default bindings
func DefaultKeyBindings() KeyBindings {
	return KeyBindings{
		Send:        []string{"enter"},
		Newline:     []string{"shift+enter", "alt+enter", "ctrl+j"},
		Cancel:      []string{"ctrl+c"},
		Interrupt:   []string{"esc"},
		HistoryUp:   []string{"up"},
		HistoryDown: []string{"down"},
		ScrollUp:    []string{"pgup", "ctrl+u"},
		ScrollDown:  []string{"pgdown", "ctrl+d"},
	}
}

// ResolvedKeyBindings is resolved bindings (defaults + user config)
type ResolvedKeyBindings struct {
	Send        map[string]bool
	Newline     map[string]bool
	Cancel      map[string]bool
	Interrupt   map[string]bool
	HistoryUp   map[string]bool
	HistoryDown map[string]bool
	ScrollUp    map[string]bool
	ScrollDown  map[string]bool
}

// Resolve merges default bindings with user bindings.
// Empty fields are replaced with defaults.
func (kb KeyBindings) Resolve() ResolvedKeyBindings {
	defaults := DefaultKeyBindings()
	if len(kb.Send) == 0 {
		kb.Send = defaults.Send
	}
	if len(kb.Newline) == 0 {
		kb.Newline = defaults.Newline
	}
	if len(kb.Cancel) == 0 {
		kb.Cancel = defaults.Cancel
	}
	if len(kb.Interrupt) == 0 {
		kb.Interrupt = defaults.Interrupt
	}
	if len(kb.HistoryUp) == 0 {
		kb.HistoryUp = defaults.HistoryUp
	}
	if len(kb.HistoryDown) == 0 {
		kb.HistoryDown = defaults.HistoryDown
	}
	if len(kb.ScrollUp) == 0 {
		kb.ScrollUp = defaults.ScrollUp
	}
	if len(kb.ScrollDown) == 0 {
		kb.ScrollDown = defaults.ScrollDown
	}

	return ResolvedKeyBindings{
		Send:        toSet(kb.Send),
		Newline:     toSet(kb.Newline),
		Cancel:      toSet(kb.Cancel),
		Interrupt:   toSet(kb.Interrupt),
		HistoryUp:   toSet(kb.HistoryUp),
		HistoryDown: toSet(kb.HistoryDown),
		ScrollUp:    toSet(kb.ScrollUp),
		ScrollDown:  toSet(kb.ScrollDown),
	}
}

// Matches checks if a pressed key matches an action.
// It checks both msg.String() and explicit modifier+code combinations
// to handle terminals that don't properly report Shift+Enter/Alt+Enter.
func (r ResolvedKeyBindings) Matches(msg tea.KeyPressMsg, action Action) bool {
	var set map[string]bool
	switch action {
	case ActionSend:
		set = r.Send
	case ActionNewline:
		set = r.Newline
	case ActionCancel:
		set = r.Cancel
	case ActionInterrupt:
		set = r.Interrupt
	case ActionHistoryUp:
		set = r.HistoryUp
	case ActionHistoryDown:
		set = r.HistoryDown
	case ActionScrollUp:
		set = r.ScrollUp
	case ActionScrollDown:
		set = r.ScrollDown
	default:
		return false
	}

	// Check string representation first (handles most cases)
	if set[msg.String()] {
		return true
	}

	// Fallback: check modifier+code combinations for keys that terminals
	// may not report correctly (Shift+Enter, Alt+Enter, Ctrl+J)
	if msg.Code == tea.KeyEnter {
		if msg.Mod.Contains(tea.ModShift) && set["shift+enter"] {
			return true
		}
		if msg.Mod.Contains(tea.ModAlt) && set["alt+enter"] {
			return true
		}
	}

	// Ctrl+J sends \n (0x0A) — treat as newline in terminals without CSI-u
	if msg.Code == 10 && set["ctrl+j"] {
		return true
	}

	return false
}

// PrimaryKey returns the primary (first) key for an action
func (r ResolvedKeyBindings) PrimaryKey(action Action) string {
	keys := sortedKeys(r.setForAction(action))
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

// AllKeys returns all keys for an action separated by "/"
func (r ResolvedKeyBindings) AllKeys(action Action) string {
	keys := sortedKeys(r.setForAction(action))
	return strings.Join(keys, "/")
}

// FormatHelp formats a help line with current bindings.
// mode: "idle" or "streaming"
// t is a function for translating action labels (i18n.T)
func (r ResolvedKeyBindings) FormatHelp(mode string, t func(string, ...any) string) string {
	type entry struct {
		key   string
		label string
	}

	var entries []entry

	if mode == "streaming" {
		entries = []entry{
			{r.PrimaryKey(ActionInterrupt), t("keys.interrupt")},
			{r.PrimaryKey(ActionSend), t("keys.comment")},
			{r.PrimaryKey(ActionCancel), t("keys.cancel")},
		}
	} else {
		entries = []entry{
			{r.PrimaryKey(ActionSend), t("keys.send")},
			{r.AllKeys(ActionNewline), t("keys.newline")},
			{r.PrimaryKey(ActionHistoryUp) + "/" + r.PrimaryKey(ActionHistoryDown), t("keys.history")},
			{r.PrimaryKey(ActionScrollUp) + "/" + r.PrimaryKey(ActionScrollDown), t("keys.scroll")},
			{r.PrimaryKey(ActionInterrupt), t("keys.quit")},
		}
	}

	var parts []string
	for _, e := range entries {
		if e.key != "" {
			parts = append(parts, fmt.Sprintf("%s — %s", e.key, e.label))
		}
	}
	return strings.Join(parts, " │ ")
}

// FormatKeybindingsList formats full list of bindings for /help
func (r ResolvedKeyBindings) FormatKeybindingsList(t func(string, ...any) string) string {
	type entry struct {
		action Action
		label  string
	}

	entries := []entry{
		{ActionSend, t("keys.send")},
		{ActionNewline, t("keys.newline")},
		{ActionCancel, t("keys.cancel")},
		{ActionInterrupt, t("keys.interrupt")},
		{ActionHistoryUp, t("keys.history_up")},
		{ActionHistoryDown, t("keys.history_down")},
		{ActionScrollUp, t("keys.scroll_up")},
		{ActionScrollDown, t("keys.scroll_down")},
	}

	var lines []string
	for _, e := range entries {
		keys := sortedKeys(r.setForAction(e.action))
		keyStr := strings.Join(keys, ", ")
		lines = append(lines, fmt.Sprintf("    %-20s %s", keyStr, e.label))
	}
	return strings.Join(lines, "\n")
}

func (r ResolvedKeyBindings) setForAction(action Action) map[string]bool {
	switch action {
	case ActionSend:
		return r.Send
	case ActionNewline:
		return r.Newline
	case ActionCancel:
		return r.Cancel
	case ActionInterrupt:
		return r.Interrupt
	case ActionHistoryUp:
		return r.HistoryUp
	case ActionHistoryDown:
		return r.HistoryDown
	case ActionScrollUp:
		return r.ScrollUp
	case ActionScrollDown:
		return r.ScrollDown
	default:
		return nil
	}
}

func toSet(keys []string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[strings.ToLower(k)] = true
	}
	return m
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
