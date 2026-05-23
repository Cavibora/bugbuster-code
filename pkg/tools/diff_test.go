package tools

import (
	"strings"
	"testing"
)

func TestUnifiedDiff_Identical(t *testing.T) {
	diff := UnifiedDiff("a.txt", "b.txt", "hello\nworld\n", "hello\nworld\n")
	if diff != "" {
		t.Errorf("Expected empty diff for identical texts, got:\n%s", diff)
	}
}

func TestUnifiedDiff_AddLine(t *testing.T) {
	oldText := "line1\nline2\n"
	newText := "line1\nline2\nline3\n"

	diff := UnifiedDiff("a.txt", "b.txt", oldText, newText)

	if diff == "" {
		t.Fatal("Expected non-empty diff")
	}
	if !strings.Contains(diff, "+line3") {
		t.Errorf("Expected '+line3' in diff, got:\n%s", diff)
	}
	if !strings.Contains(diff, "--- a/a.txt") {
		t.Errorf("Expected '--- a/a.txt' header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+++ b/b.txt") {
		t.Errorf("Expected '+++ b/b.txt' header, got:\n%s", diff)
	}
}

func TestUnifiedDiff_RemoveLine(t *testing.T) {
	oldText := "line1\nline2\nline3\n"
	newText := "line1\nline3\n"

	diff := UnifiedDiff("a.txt", "b.txt", oldText, newText)

	if diff == "" {
		t.Fatal("Expected non-empty diff")
	}
	if !strings.Contains(diff, "-line2") {
		t.Errorf("Expected '-line2' in diff, got:\n%s", diff)
	}
}

func TestUnifiedDiff_ModifyLine(t *testing.T) {
	oldText := "line1\nold_line\nline3\n"
	newText := "line1\nnew_line\nline3\n"

	diff := UnifiedDiff("a.txt", "b.txt", oldText, newText)

	if diff == "" {
		t.Fatal("Expected non-empty diff")
	}
	if !strings.Contains(diff, "-old_line") {
		t.Errorf("Expected '-old_line' in diff, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+new_line") {
		t.Errorf("Expected '+new_line' in diff, got:\n%s", diff)
	}
}

func TestUnifiedDiff_EmptyOld(t *testing.T) {
	diff := UnifiedDiff("a.txt", "b.txt", "", "hello\nworld\n")

	if diff == "" {
		t.Fatal("Expected non-empty diff for new file")
	}
	if !strings.Contains(diff, "+hello") {
		t.Errorf("Expected '+hello' in diff, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+world") {
		t.Errorf("Expected '+world' in diff, got:\n%s", diff)
	}
}

func TestUnifiedDiff_EmptyNew(t *testing.T) {
	diff := UnifiedDiff("a.txt", "b.txt", "hello\nworld\n", "")

	if diff == "" {
		t.Fatal("Expected non-empty diff for deleted file")
	}
	if !strings.Contains(diff, "-hello") {
		t.Errorf("Expected '-hello' in diff, got:\n%s", diff)
	}
	if !strings.Contains(diff, "-world") {
		t.Errorf("Expected '-world' in diff, got:\n%s", diff)
	}
}

func TestDiffStats(t *testing.T) {
	diff := "--- a/a.txt\n+++ b/b.txt\n@@ -1,3 +1,3 @@\n line1\n-old_line\n+new_line\n line3\n"

	added, removed := DiffStats(diff)
	if added != 1 {
		t.Errorf("Expected 1 added line, got %d", added)
	}
	if removed != 1 {
		t.Errorf("Expected 1 removed line, got %d", removed)
	}
}

func TestDiffStats_Empty(t *testing.T) {
	added, removed := DiffStats("")
	if added != 0 || removed != 0 {
		t.Errorf("Expected 0 added and 0 removed, got %d added, %d removed", added, removed)
	}
}

func TestDiffLines(t *testing.T) {
	diff := "--- a/a.txt\n+++ b/b.txt\n@@ -1,3 +1,3 @@\n line1\n-old_line\n+new_line\n line3\n"

	lines := DiffLines(diff, 10)
	if len(lines) != 4 {
		t.Errorf("Expected 4 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != " line1" {
		t.Errorf("Expected ' line1', got '%s'", lines[0])
	}
	if lines[1] != "-old_line" {
		t.Errorf("Expected '-old_line', got '%s'", lines[1])
	}
	if lines[2] != "+new_line" {
		t.Errorf("Expected '+new_line', got '%s'", lines[2])
	}
}

func TestDiffLines_MaxLines(t *testing.T) {
	diff := "--- a/a.txt\n+++ b/b.txt\n@@ -1,5 +1,5 @@\n line1\n-old1\n-old2\n+new1\n+new2\n line3\n"

	lines := DiffLines(diff, 3)
	if len(lines) != 4 { // 3 lines + "... (N more lines)"
		t.Errorf("Expected 4 lines (3 + overflow), got %d: %v", len(lines), lines)
	}
}

func TestDiffLines_Empty(t *testing.T) {
	lines := DiffLines("", 10)
	if lines != nil {
		t.Errorf("Expected nil for empty diff, got %v", lines)
	}
}

func TestUnifiedDiff_MultipleChanges(t *testing.T) {
	oldText := "line1\nline2\nline3\nline4\nline5\n"
	newText := "line1\nmodified2\nline3\nadded4\nline5\n"

	diff := UnifiedDiff("a.txt", "b.txt", oldText, newText)

	if diff == "" {
		t.Fatal("Expected non-empty diff")
	}

	added, removed := DiffStats(diff)
	if added < 1 {
		t.Errorf("Expected at least 1 added line, got %d", added)
	}
	if removed < 1 {
		t.Errorf("Expected at least 1 removed line, got %d", removed)
	}
}
