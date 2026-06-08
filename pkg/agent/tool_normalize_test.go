package agent

import (
	"testing"
)

func TestNormalizeToolName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Exact matches (already canonical)
		{"read", "read"},
		{"write", "write"},
		{"edit", "edit"},
		{"bash", "bash"},
		{"grep", "grep"},
		{"glob", "glob"},
		{"memory", "memory"},
		{"lsp", "lsp"},
		{"browse", "browse"},
		{"web_fetch", "web_fetch"},
		{"ask_user", "ask_user"},
		{"todo_write", "todo_write"},
		{"todo_read", "todo_read"},
		{"learn", "learn"},
		{"delegate_task", "delegate_task"},
		{"background", "background"},
		{"ps", "ps"},
		{"logs", "logs"},
		{"kill", "kill"},

		// PascalCase aliases
		{"Read", "read"},
		{"Write", "write"},
		{"Edit", "edit"},
		{"Bash", "bash"},
		{"Grep", "grep"},
		{"Glob", "glob"},
		{"Memory", "memory"},
		{"LSP", "lsp"},
		{"Browse", "browse"},
		{"WebFetch", "web_fetch"},
		{"AskUser", "ask_user"},
		{"TodoWrite", "todo_write"},
		{"TodoRead", "todo_read"},
		{"Learn", "learn"},
		{"DelegateTask", "delegate_task"},
		{"Background", "background"},
		{"PS", "ps"},
		{"Logs", "logs"},
		{"Kill", "kill"},

		// UPPER_CASE aliases
		{"READ", "read"},
		{"WRITE", "write"},
		{"EDIT", "edit"},
		{"BASH", "bash"},
		{"GREP", "grep"},
		{"GLOB", "glob"},
		{"MEMORY", "memory"},
		{"LEARN", "learn"},
		{"KILL", "kill"},

		// Common aliases
		{"cat", "read"},
		{"open", "read"},
		{"view", "read"},
		{"show", "read"},
		{"file_read", "read"},
		{"read_file", "read"},
		{"save", "write"},
		{"create_file", "write"},
		{"replace", "edit"},
		{"sed", "edit"},
		{"patch", "edit"},
		{"shell", "bash"},
		{"exec", "bash"},
		{"run", "bash"},
		{"command", "bash"},
		{"cmd", "bash"},
		{"terminal", "bash"},
		{"search", "grep"},
		{"ripgrep", "grep"},
		{"rg", "grep"},
		{"find_files", "glob"},
		{"list_files", "glob"},
		{"remember", "memory"},
		{"recall", "memory"},
		{"web_search", "browse"},
		{"websearch", "browse"},
		{"curl", "web_fetch"},
		{"wget", "web_fetch"},
		{"fetch", "web_fetch"},
		{"ask", "ask_user"},
		{"question", "ask_user"},
		{"todo", "todo_write"},
		{"delegate", "delegate_task"},
		{"subagent", "delegate_task"},
		{"bg", "background"},
		{"jobs", "ps"},
		{"processes", "ps"},
		{"tail", "logs"},
		{"terminate", "kill"},

		// Hyphenated names (web-fetch → web_fetch)
		{"web-fetch", "web_fetch"},
		{"ask-user", "ask_user"},
		{"todo-write", "todo_write"},
		{"todo-read", "todo_read"},
		{"delegate-task", "delegate_task"},
		{"file-read", "read"},
		{"find-files", "glob"},

		// Unknown names — lowercase
		{"UnknownTool", "unknowntool"},
		{"SOMETHING", "something"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeToolName(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeToolName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFindClosestToolName(t *testing.T) {
	availableTools := map[string]bool{
		"read": true, "write": true, "edit": true, "bash": true,
		"grep": true, "glob": true, "memory": true, "lsp": true,
		"browse": true, "web_fetch": true, "ask_user": true,
		"todo_write": true, "todo_read": true, "learn": true,
		"delegate_task": true, "kill": true, "ps": true, "logs": true,
		"background": true,
	}

	tests := []struct {
		input    string
		found    bool
		expected string
	}{
		// Exact matches
		{"read", true, "read"},
		{"write", true, "write"},
		{"bash", true, "bash"},

		// Aliases (normalized first)
		{"Read", true, "read"},
		{"cat", true, "read"},
		{"shell", true, "bash"},

		// Typos (Levenshtein distance ≤ 2)
		{"rad", true, "read"},      // 1 edit
		{"rea", true, "read"},      // 1 edit
		{"reed", true, "read"},     // 1 edit
		{"writ", true, "write"},    // 1 edit
		{"rite", true, "write"},    // 1 edit
		{"bsh", true, "bash"},      // 1 edit
		{"bas", true, "bash"},      // 1 edit
		{"edt", true, "edit"},      // 1 edit
		{"grpe", true, "grep"},    // 1 edit
		{"glb", true, "glob"},      // 1 edit
		{"memry", true, "memory"},  // 1 edit

		// Too far (distance > 2)
		{"xyz", false, ""},
		{"unknown", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, found := findClosestToolName(tt.input, availableTools)
			if found != tt.found {
				t.Errorf("findClosestToolName(%q) found = %v, want %v", tt.input, found, tt.found)
			}
			if found && result != tt.expected {
				t.Errorf("findClosestToolName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"read", "read", 0},
		{"read", "reed", 1},
		{"read", "rad", 1},
		{"read", "write", 4},
		{"bash", "bas", 1},
		{"bash", "bsh", 1},
		{"memory", "memry", 1},
		{"", "abc", 3},
		{"abc", "", 3},
		{"", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			result := levenshteinDistance(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestParseToolCalls_WithAliases(t *testing.T) {
	// Test that tool aliases are normalized in parsed calls
	tests := []struct {
		name     string
		input    string
		expected []ToolCall
	}{
		{
			name:  "PascalCase_XML",
			input: `<tool name="Read"><path>main.go</path></tool>`,
			expected: []ToolCall{
				{Name: "read", Params: map[string]string{"path": "main.go"}},
			},
		},
		{
			name:  "alias_cat_XML",
			input: `<tool name="cat"><path>main.go</path></tool>`,
			expected: []ToolCall{
				{Name: "read", Params: map[string]string{"path": "main.go"}},
			},
		},
		{
			name:  "PascalCase_JSON",
			input: `{"tool": "Read", "params": {"path": "main.go"}}`,
			expected: []ToolCall{
				{Name: "read", Params: map[string]string{"path": "main.go"}},
			},
		},
		{
			name:  "alias_shell_JSON",
			input: `{"tool": "shell", "params": {"command": "ls"}}`,
			expected: []ToolCall{
				{Name: "bash", Params: map[string]string{"command": "ls"}},
			},
		},
		{
			name:  "hyphenated_JSON",
			input: `{"tool": "web-fetch", "params": {"url": "https://example.com"}}`,
			expected: []ToolCall{
				{Name: "web_fetch", Params: map[string]string{"url": "https://example.com"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := ParseToolCalls(tt.input)
			if len(calls) != len(tt.expected) {
				t.Fatalf("ParseToolCalls(%q) returned %d calls, want %d", tt.input, len(calls), len(tt.expected))
			}
			for i, call := range calls {
				if call.Name != tt.expected[i].Name {
					t.Errorf("call[%d].Name = %q, want %q", i, call.Name, tt.expected[i].Name)
				}
				for k, v := range tt.expected[i].Params {
					if call.Params[k] != v {
						t.Errorf("call[%d].Params[%q] = %q, want %q", i, k, call.Params[k], v)
					}
				}
			}
		})
	}
}