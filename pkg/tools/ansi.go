package tools

import (
	"regexp"
	"strings"
)

// ANSI escape sequence patterns
var (
	// Matches CSI sequences: \x1b[...m (SGR), \x1b[...H (cursor), etc.
	ansiCSI = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	// Matches OSC sequences: \x1b]...BEL or \x1b]...ST
	ansiOSC = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	// Matches other escape sequences: \x1b(, \x1b), etc.
	ansiOther = regexp.MustCompile(`\x1b[()][AB012]`)
	// Matches cursor position sequences
	ansiCursor = regexp.MustCompile(`\x1b\[(?:\d+;)?\d+[Hf]`)
	// Matches erase sequences: \x1b[2J, \x1b[K, \x1b[P, etc.
	ansiErase = regexp.MustCompile(`\x1b\[(?:\d*[ABCDEFGJKPSTX])`)
	// Matches scroll sequences: \x1b[S, \x1b[T, \x1b[L, \x1b[M
	ansiScroll = regexp.MustCompile(`\x1b\[\d*[STML]`)
	// Matches screen mode sequences: \x1b[?1049h, \x1b[?25l, etc.
	ansiScreen = regexp.MustCompile(`\x1b\[\?\d+[hl]`)
	// Matches title sequences: \x1b]0;...\x07 or \x1b]2;...\x07
	ansiTitle = regexp.MustCompile(`\x1b\]\d+;[^\x07]*\x07`)
	// Matches \r followed by content (carriage return — progress bar overwrite)
	// \r means "go to beginning of line", so everything before \r on the same line is overwritten
	carriageReturnOverwrite = regexp.MustCompile(`^[^\n]*\r|\n[^\n]*\r`)
)

// StripANSI removes all ANSI escape sequences and cleans up terminal output artifacts.
// This includes:
// - Color/style codes (SGR sequences)
// - Cursor movement codes
// - Screen clear/erase codes
// - Title/icon sequences
// - Carriage returns without newlines (progress bar overwrites)
// - OSC sequences (terminal title, hyperlinks)
func StripANSI(s string) string {
	// Remove OSC sequences first (they can contain semicolons that confuse other patterns)
	s = ansiTitle.ReplaceAllString(s, "")
	s = ansiOSC.ReplaceAllString(s, "")
	// Remove CSI sequences
	s = ansiCSI.ReplaceAllString(s, "")
	s = ansiCursor.ReplaceAllString(s, "")
	s = ansiErase.ReplaceAllString(s, "")
	s = ansiScroll.ReplaceAllString(s, "")
	s = ansiScreen.ReplaceAllString(s, "")
	s = ansiOther.ReplaceAllString(s, "")
	// Remove carriage return overwrites (progress bar pattern: "text\rnewtext" → "newtext")
	// \r means "go to beginning of line", so everything before \r on the same line is overwritten
	s = carriageReturnOverwrite.ReplaceAllStringFunc(s, func(match string) string {
		// Find the last \r in the match and return everything after it
		lastCR := strings.LastIndex(match, "\r")
		if lastCR >= 0 {
			// Preserve the newline prefix if present
			prefix := ""
			if len(match) > 0 && match[0] == '\n' {
				prefix = "\n"
			}
			return prefix + match[lastCR+1:]
		}
		return match
	})
	// Clean up: remove leftover \r that are followed by \n (turn \r\n into \n)
	// This is already handled by the carriage return pattern above
	// Remove multiple consecutive blank lines (common after stripping)
	for i := 0; i < 3; i++ {
		prev := s
		s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
		if s == prev {
			break
		}
	}
	return s
}

// StripANSIAndTrim removes ANSI sequences and trims trailing whitespace from each line.
// Useful for cleaning bash command output before sending to the model.
func StripANSIAndTrim(s string) string {
	s = StripANSI(s)
	lines := regexp.MustCompile(`\r?\n`).Split(s, -1)
	var trimmed []string
	for _, line := range lines {
		trimmed = append(trimmed, regexp.MustCompile(`\s+$`).ReplaceAllString(line, ""))
	}
	result := ""
	for i, line := range trimmed {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	// Remove trailing blank lines
	result = regexp.MustCompile(`\n+$`).ReplaceAllString(result, "\n")
	return result
}