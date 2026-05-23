package tools

import (
	"fmt"
	"strings"
)

// UnifiedDiff generates unified diff between two texts.
// Returns line in format:
//
//	--- a/oldName
//	+++ b/newName
//	@@ -start,count +start,count @@
//	-removed line
//	+added line
//	 context line
//
// If texts are identical — returns empty line.
func UnifiedDiff(oldName, newName, oldText, newText string) string {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	// If texts are identical — no diff
	if len(oldLines) == len(newLines) {
		identical := true
		for i := range oldLines {
			if oldLines[i] != newLines[i] {
				identical = false
				break
			}
		}
		if identical {
			return ""
		}
	}

	// Find common lines via LCS
	lcs := longestCommonSubsequence(oldLines, newLines)

	// Build hunks (change blocks)
	hunks := buildHunks(oldLines, newLines, lcs)

	if len(hunks) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- a/%s\n", oldName))
	sb.WriteString(fmt.Sprintf("+++ b/%s\n", newName))

	for _, hunk := range hunks {
		sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n",
			hunk.oldStart, hunk.oldCount,
			hunk.newStart, hunk.newCount))

		// Context before changes (up to 3 lines)
		for _, line := range hunk.prefix {
			sb.WriteString(" " + line + "\n")
		}

		// Removed lines
		for _, line := range hunk.removed {
			sb.WriteString("-" + line + "\n")
		}

		// Added lines
		for _, line := range hunk.added {
			sb.WriteString("+" + line + "\n")
		}

		// Context after changes (up to 3 lines)
		for _, line := range hunk.suffix {
			sb.WriteString(" " + line + "\n")
		}
	}

	return sb.String()
}

// DiffStats returns count of added and removed lines from diff.
func DiffStats(diff string) (added, removed int) {
	for _, line := range strings.Split(diff, "\n") {
		if len(line) == 0 {
			continue
		}
		if line[0] == '+' && !strings.HasPrefix(line, "+++") {
			added++
		} else if line[0] == '-' && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	return
}

// DiffLines returns diff lines for display (without --- and +++ headers).
// Limits line count to maxLines.
func DiffLines(diff string, maxLines int) []string {
	if diff == "" {
		return nil
	}

	lines := strings.Split(diff, "\n")
	var result []string
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		// Skip --- and +++ headers
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}
		// Skip @@ ... @@ headers (they will be in summary)
		if strings.HasPrefix(line, "@@") {
			continue
		}
		result = append(result, line)
		if maxLines > 0 && len(result) >= maxLines {
			remaining := len(lines) - len(result)
			if remaining > 0 {
				result = append(result, fmt.Sprintf("... (%d more lines)", remaining))
			}
			break
		}
	}
	return result
}

// hunk represents a change block in unified diff
type hunk struct {
	oldStart int      // starting line in old file (1-based)
	oldCount int      // line count in old file
	newStart int      // starting line in new file (1-based)
	newCount int      // line count in new file
	prefix   []string // context before changes
	removed  []string // removed lines
	added    []string // added lines
	suffix   []string // context after changes
}

// buildHunks builds change blocks from LCS
func buildHunks(oldLines, newLines []string, lcs []lcsEntry) []hunk {
	if len(lcs) == 0 && len(oldLines) == 0 && len(newLines) == 0 {
		return nil
	}

	// Build map of common lines
	commonOld := make(map[int]bool) // indexes in oldLines that are in LCS
	commonNew := make(map[int]bool) // indexes in newLines that are in LCS
	oldIdx, newIdx := 0, 0
	for _, entry := range lcs {
		for oldIdx < len(oldLines) && oldLines[oldIdx] != entry.line {
			oldIdx++
		}
		for newIdx < len(newLines) && newLines[newIdx] != entry.line {
			newIdx++
		}
		if oldIdx < len(oldLines) {
			commonOld[oldIdx] = true
			oldIdx++
		}
		if newIdx < len(newLines) {
			commonNew[newIdx] = true
			newIdx++
		}
	}

	// Group changes into hunks
	var hunks []hunk
	i, j := 0, 0

	for i < len(oldLines) || j < len(newLines) {
		// Skip common lines until next change
		for i < len(oldLines) && commonOld[i] && j < len(newLines) && commonNew[j] {
			i++
			j++
		}

		// Collect removed lines
		var removed []string
		removedStart := i + 1 // 1-based
		for i < len(oldLines) && !commonOld[i] {
			removed = append(removed, oldLines[i])
			i++
		}

		// Collect added lines
		var added []string
		addedStart := j + 1 // 1-based
		for j < len(newLines) && !commonNew[j] {
			added = append(added, newLines[j])
			j++
		}

		if len(removed) == 0 && len(added) == 0 {
			break
		}

		// Context before changes (up to 3 lines)
		var prefix []string
		prefixStart := removedStart - 1
		if prefixStart > 3 {
			prefixStart -= 3
		} else {
			prefixStart = 0
		}
		// Collect context from common lines before change
		contextBefore := removedStart - 1
		startContext := contextBefore - 3
		if startContext < 0 {
			startContext = 0
		}
		for k := startContext; k < contextBefore; k++ {
			if k < len(oldLines) && commonOld[k] {
				prefix = append(prefix, oldLines[k])
			}
		}

		// Context after changes (up to 3 lines)
		var suffix []string
		for k := 0; k < 3 && i+k < len(oldLines) && commonOld[i+k]; k++ {
			suffix = append(suffix, oldLines[i+k])
		}

		h := hunk{
			oldStart: removedStart - len(prefix),
			oldCount: len(prefix) + len(removed) + len(suffix),
			newStart: addedStart - len(prefix),
			newCount: len(prefix) + len(added) + len(suffix),
			prefix:   prefix,
			removed:  removed,
			added:    added,
			suffix:   suffix,
		}

		// Minimum values
		if h.oldCount == 0 {
			h.oldCount = len(added) // for pure additions
		}
		if h.newCount == 0 {
			h.newCount = len(removed) // for pure deletions
		}

		hunks = append(hunks, h)

		// Advance i and j through context after changes
		for k := 0; k < 3 && i < len(oldLines) && commonOld[i]; k++ {
			i++
			if j < len(newLines) && commonNew[j] {
				j++
			}
		}
	}

	return hunks
}

// lcsEntry represents a line in the longest common subsequence
type lcsEntry struct {
	line string
}

// longestCommonSubsequence finds LCS of two string arrays
func longestCommonSubsequence(a, b []string) []lcsEntry {
	m, n := len(a), len(b)

	// For large files use simplified algorithm
	// (full LCS requires O(m*n) memory, which can be a problem for large files)
	if m > 5000 || n > 5000 {
		return simpleLCS(a, b)
	}

	// DP table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	// Fill DP table
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Reconstruct LCS
	var result []lcsEntry
	i, j := m, n
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			result = append([]lcsEntry{{line: a[i-1]}}, result...)
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	return result
}

// simpleLCS — simplified algorithm for large files.
// Finds common lines by hash, skipping unique ones.
func simpleLCS(a, b []string) []lcsEntry {
	// Build map of b strings
	bMap := make(map[string][]int)
	for i, line := range b {
		bMap[line] = append(bMap[line], i)
	}

	var result []lcsEntry
	lastB := -1

	for _, line := range a {
		indices, ok := bMap[line]
		if !ok {
			continue
		}
		// Find index in b after lastB
		for _, idx := range indices {
			if idx > lastB {
				result = append(result, lcsEntry{line: line})
				lastB = idx
				break
			}
		}
	}

	return result
}

// splitLines splits text into lines, keeping \n at end of each line
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	// Remove empty last line if text ends with \n
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
