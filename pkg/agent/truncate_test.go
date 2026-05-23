package agent

import (
	"testing"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},         // short string, no truncation
		{"hello world", 5, "hello..."}, // truncated
		{"hello", 5, "hello"},          // exact length, no truncation
		{"", 10, ""},                   // empty string
		{"abc", 0, "..."},              // maxLen=0
	}

	for _, tt := range tests {
		got := truncate(tt.s, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
		}
	}
}

func TestDebugLog_NoDebugDir(t *testing.T) {
	a := NewAgentLoop(nil)
	// Should not panic when debugDir is empty
	a.debugLog("test", map[string]any{"key": "value"})
}
