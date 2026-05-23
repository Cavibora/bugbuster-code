package tools

import (
	"testing"
)

func TestSimpleLCS(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want int // expected length of LCS
	}{
		{
			name: "identical",
			a:    []string{"a", "b", "c"},
			b:    []string{"a", "b", "c"},
			want: 3,
		},
		{
			name: "common subsequence",
			a:    []string{"a", "b", "c", "d"},
			b:    []string{"a", "c", "d"},
			want: 3,
		},
		{
			name: "no common",
			a:    []string{"x", "y"},
			b:    []string{"a", "b"},
			want: 0,
		},
		{
			name: "empty a",
			a:    []string{},
			b:    []string{"a", "b"},
			want: 0,
		},
		{
			name: "empty b",
			a:    []string{"a", "b"},
			b:    []string{},
			want: 0,
		},
		{
			name: "repeated lines",
			a:    []string{"a", "a", "b"},
			b:    []string{"a", "b", "a"},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := simpleLCS(tt.a, tt.b)
			if len(result) != tt.want {
				t.Errorf("simpleLCS() returned %d entries, want %d", len(result), tt.want)
			}
		})
	}
}

func TestToolDescriptions(t *testing.T) {
	// Test that all tools return non-empty descriptions
	tools := []struct {
		name string
		tool Tool
	}{
		{"bash", NewBashTool()},
		{"read", NewReadTool()},
		{"write", NewWriteTool()},
		{"edit", NewEditTool()},
		{"grep", NewGrepTool()},
		{"glob", NewGlobTool()},
		{"ask", NewAskTool()},
		{"ask_user", NewAskUserTool()},
		{"learn", NewLearnTool()},
		{"lsp", NewLSPTool()},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			if tt.tool.Name() == "" {
				t.Error("Expected non-empty name")
			}
			if tt.tool.Description() == "" {
				t.Error("Expected non-empty description")
			}
			if tt.tool.Parameters() == nil {
				t.Error("Expected non-nil parameters")
			}
		})
	}
}

func TestLSPTool_SetRootDir(t *testing.T) {
	tool := NewLSPTool()
	tool.SetRootDir("/tmp/test")
	// Should not panic
}
