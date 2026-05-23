package main

import (
	"strings"
	"testing"
	"time"
)

func TestFormatStatusLineWithDetails(t *testing.T) {
	result := FormatStatusLine(5000, 7000, 3*time.Minute+15*time.Second, 50000, 200000, "zai", "glm-5.1")

	if !strings.Contains(result, "⏱") {
		t.Errorf("Expected ⏱ timer symbol, got: %q", result)
	}
	if !strings.Contains(result, "⬆") {
		t.Errorf("Expected input tokens arrow, got: %q", result)
	}
	if !strings.Contains(result, "zai") {
		t.Errorf("Expected provider name, got: %q", result)
	}
	if !strings.Contains(result, "glm-5.1") {
		t.Errorf("Expected model name, got: %q", result)
	}
	// Проверяем что контекст-бар на отдельной строке
	lines := strings.Split(result, "\n")
	if len(lines) < 2 {
		t.Errorf("Expected at least 2 lines (status + context bar), got %d", len(lines))
	}
}