package config

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDefaultKeyBindings(t *testing.T) {
	kb := DefaultKeyBindings()
	if len(kb.Send) == 0 || kb.Send[0] != "enter" {
		t.Errorf("Default Send = %v, want [enter]", kb.Send)
	}
	if len(kb.Newline) < 1 || kb.Newline[0] != "shift+enter" {
		t.Errorf("Default Newline = %v, want [shift+enter, alt+enter, ctrl+j]", kb.Newline)
	}
	if len(kb.Newline) < 2 || kb.Newline[1] != "alt+enter" {
		t.Errorf("Default Newline = %v, want [shift+enter, alt+enter, ctrl+j]", kb.Newline)
	}
	if len(kb.Newline) < 3 || kb.Newline[2] != "ctrl+j" {
		t.Errorf("Default Newline = %v, want [shift+enter, alt+enter, ctrl+j]", kb.Newline)
	}
	if len(kb.Cancel) == 0 || kb.Cancel[0] != "ctrl+c" {
		t.Errorf("Default Cancel = %v, want [ctrl+c]", kb.Cancel)
	}
	if len(kb.Interrupt) == 0 || kb.Interrupt[0] != "esc" {
		t.Errorf("Default Interrupt = %v, want [esc]", kb.Interrupt)
	}
}

func TestResolveDefaults(t *testing.T) {
	// Пустой конфиг должен резолвиться в дефолты
	kb := KeyBindings{}
	resolved := kb.Resolve()

	if !resolved.Send["enter"] {
		t.Error("Resolved Send should contain 'enter'")
	}
	if !resolved.Newline["alt+enter"] {
		t.Error("Resolved Newline should contain 'alt+enter'")
	}
	if !resolved.Newline["ctrl+j"] {
		t.Error("Resolved Newline should contain 'ctrl+j'")
	}
	if !resolved.Cancel["ctrl+c"] {
		t.Error("Resolved Cancel should contain 'ctrl+c'")
	}
	if !resolved.Interrupt["esc"] {
		t.Error("Resolved Interrupt should contain 'esc'")
	}
	if !resolved.ScrollUp["pgup"] {
		t.Error("Resolved ScrollUp should contain 'pgup'")
	}
	if !resolved.ScrollUp["ctrl+u"] {
		t.Error("Resolved ScrollUp should contain 'ctrl+u'")
	}
}

func TestResolveCustom(t *testing.T) {
	// Кастомный конфиг должен перекрывать дефолты
	kb := KeyBindings{
		Send:    []string{"enter"},
		Newline: []string{"ctrl+j"}, // кастомный перенос строки
		Cancel:  []string{"ctrl+c", "ctrl+q"},
	}
	resolved := kb.Resolve()

	if !resolved.Send["enter"] {
		t.Error("Resolved Send should contain 'enter'")
	}
	if !resolved.Newline["ctrl+j"] {
		t.Error("Resolved Newline should contain 'ctrl+j'")
	}
	if resolved.Newline["alt+enter"] {
		t.Error("Resolved Newline should NOT contain 'alt+enter' (overridden)")
	}
	if !resolved.Cancel["ctrl+c"] {
		t.Error("Resolved Cancel should contain 'ctrl+c'")
	}
	if !resolved.Cancel["ctrl+q"] {
		t.Error("Resolved Cancel should contain 'ctrl+q'")
	}
	// Неуказанные поля — дефолтные
	if !resolved.Interrupt["esc"] {
		t.Error("Resolved Interrupt should contain 'esc' (default)")
	}
	if !resolved.ScrollUp["pgup"] {
		t.Error("Resolved ScrollUp should contain 'pgup' (default)")
	}
}

func TestMatchesEnter(t *testing.T) {
	resolved := DefaultKeyBindings().Resolve()

	// Enter должен матчить Send
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	if !resolved.Matches(msg, ActionSend) {
		t.Error("Enter should match ActionSend")
	}
	if resolved.Matches(msg, ActionNewline) {
		t.Error("Enter should NOT match ActionNewline")
	}
}

func TestMatchesAltEnter(t *testing.T) {
	resolved := DefaultKeyBindings().Resolve()

	// Alt+Enter должен матчить Newline
	msg := tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt}
	if !resolved.Matches(msg, ActionNewline) {
		t.Error("Alt+Enter should match ActionNewline")
	}
	if resolved.Matches(msg, ActionSend) {
		t.Error("Alt+Enter should NOT match ActionSend")
	}
}

func TestMatchesCtrlC(t *testing.T) {
	resolved := DefaultKeyBindings().Resolve()

	msg := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	if !resolved.Matches(msg, ActionCancel) {
		t.Error("Ctrl+C should match ActionCancel")
	}
}

func TestMatchesEscape(t *testing.T) {
	resolved := DefaultKeyBindings().Resolve()

	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	if !resolved.Matches(msg, ActionInterrupt) {
		t.Error("Escape should match ActionInterrupt")
	}
}

func TestMatchesScrollKeys(t *testing.T) {
	resolved := DefaultKeyBindings().Resolve()

	// PgUp
	msg := tea.KeyPressMsg{Code: tea.KeyPgUp}
	if !resolved.Matches(msg, ActionScrollUp) {
		t.Error("PgUp should match ActionScrollUp")
	}

	// Ctrl+U
	msg = tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	if !resolved.Matches(msg, ActionScrollUp) {
		t.Error("Ctrl+U should match ActionScrollUp")
	}

	// PgDown
	msg = tea.KeyPressMsg{Code: tea.KeyPgDown}
	if !resolved.Matches(msg, ActionScrollDown) {
		t.Error("PgDown should match ActionScrollDown")
	}

	// Ctrl+D
	msg = tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
	if !resolved.Matches(msg, ActionScrollDown) {
		t.Error("Ctrl+D should match ActionScrollDown")
	}
}

func TestPrimaryKey(t *testing.T) {
	resolved := DefaultKeyBindings().Resolve()

	if resolved.PrimaryKey(ActionSend) != "enter" {
		t.Errorf("PrimaryKey(ActionSend) = %q, want 'enter'", resolved.PrimaryKey(ActionSend))
	}
	if resolved.PrimaryKey(ActionNewline) != "alt+enter" {
		t.Errorf("PrimaryKey(ActionNewline) = %q, want 'alt+enter'", resolved.PrimaryKey(ActionNewline))
	}
	if resolved.PrimaryKey(ActionCancel) != "ctrl+c" {
		t.Errorf("PrimaryKey(ActionCancel) = %q, want 'ctrl+c'", resolved.PrimaryKey(ActionCancel))
	}
}

func TestAllKeys(t *testing.T) {
	resolved := DefaultKeyBindings().Resolve()

	newlineKeys := resolved.AllKeys(ActionNewline)
	if newlineKeys != "alt+enter/ctrl+j/shift+enter" {
		t.Errorf("AllKeys(ActionNewline) = %q, want 'alt+enter/ctrl+j/shift+enter'", newlineKeys)
	}

	scrollUpKeys := resolved.AllKeys(ActionScrollUp)
	if scrollUpKeys != "ctrl+u/pgup" {
		t.Errorf("AllKeys(ActionScrollUp) = %q, want 'ctrl+u/pgup'", scrollUpKeys)
	}
}

func TestMatchesCtrlJ(t *testing.T) {
	resolved := DefaultKeyBindings().Resolve()

	// Ctrl+J как Ctrl+J (модификатор + клавиша)
	msg := tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}
	if !resolved.Matches(msg, ActionNewline) {
		t.Error("Ctrl+J should match ActionNewline")
	}
	if resolved.Matches(msg, ActionSend) {
		t.Error("Ctrl+J should NOT match ActionSend")
	}

	// Ctrl+J как \n (код 10) — fallback для терминалов без CSI-u
	msg2 := tea.KeyPressMsg{Code: 10}
	if !resolved.Matches(msg2, ActionNewline) {
		t.Error("Ctrl+J (code 10) should match ActionNewline")
	}
	if resolved.Matches(msg2, ActionSend) {
		t.Error("Ctrl+J (code 10) should NOT match ActionSend")
	}
}

func TestMatchesShiftEnter(t *testing.T) {
	resolved := DefaultKeyBindings().Resolve()

	// Shift+Enter должен матчить Newline (дефолт теперь включает shift+enter)
	msg := tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
	if !resolved.Matches(msg, ActionNewline) {
		t.Error("Shift+Enter should match ActionNewline")
	}
	if resolved.Matches(msg, ActionSend) {
		t.Error("Shift+Enter should NOT match ActionSend")
	}
}

func TestFormatHelp(t *testing.T) {
	resolved := DefaultKeyBindings().Resolve()

	// Простая функция перевода для теста
	translate := func(key string, _ ...interface{}) string {
		m := map[string]string{
			"keys.send":      "отправить",
			"keys.newline":   "перенос",
			"keys.cancel":    "отмена",
			"keys.interrupt": "прервать",
			"keys.quit":      "выход",
			"keys.history":   "история",
			"keys.scroll":    "скролл",
			"keys.comment":   "комментарий",
		}
		if v, ok := m[key]; ok {
			return v
		}
		return key
	}

	help := resolved.FormatHelp("idle", translate)
	if help == "" {
		t.Error("FormatHelp should return non-empty string")
	}
	// Должен содержать "enter" и "отправить"
	if !containsAll(help, "enter", "отправить") {
		t.Errorf("FormatHelp idle = %q, should contain 'enter' and 'отправить'", help)
	}

	helpStreaming := resolved.FormatHelp("streaming", translate)
	if !containsAll(helpStreaming, "esc", "прервать") {
		t.Errorf("FormatHelp streaming = %q, should contain 'esc' and 'прервать'", helpStreaming)
	}
}

func TestFormatKeybindingsList(t *testing.T) {
	resolved := DefaultKeyBindings().Resolve()

	translate := func(key string, _ ...interface{}) string { return key }
	list := resolved.FormatKeybindingsList(translate)
	if list == "" {
		t.Error("FormatKeybindingsList should return non-empty string")
	}
	if !containsAll(list, "enter", "alt+enter", "ctrl+c", "esc") {
		t.Errorf("FormatKeybindingsList = %q, should contain key names", list)
	}
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || sub == "" ||
		(s != "" && containsStr(s, sub)))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
