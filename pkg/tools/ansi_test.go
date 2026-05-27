package tools

import (
	"strings"
	"testing"
)

func TestStripANSI_BasicColors(t *testing.T) {
	input := "\x1b[31mRed Text\x1b[0m normal"
	expected := "Red Text normal"
	result := StripANSI(input)
	if result != expected {
		t.Errorf("StripANSI basic colors: got %q, want %q", result, expected)
	}
}

func TestStripANSI_CursorMovement(t *testing.T) {
	input := "\x1b[2J\x1b[H\x1b[1;1HHello"
	expected := "Hello"
	result := StripANSI(input)
	if result != expected {
		t.Errorf("StripANSI cursor: got %q, want %q", result, expected)
	}
}

func TestStripANSI_ProgressBar(t *testing.T) {
	// Simulates: progress bar overwriting same line
	input := "  0%\r 50%\r100%\nDone"
	expected := "100%\nDone"
	result := StripANSI(input)
	if result != expected {
		t.Errorf("StripANSI progress bar: got %q, want %q", result, expected)
	}
}

func TestStripANSI_ComplicatedProgress(t *testing.T) {
	// Real-world cargo build output with spinner
	input := "\x1b[?25l\x1b[2K\r  Compiling grfn v0.1.0 (\x1b[36m\x1b[1m/\x1b[0m\x1b[?25h)\r\x1b[2K\r  Compiling grfn v0.1.0 (\x1b[32m\x1b[1mDONE\x1b[0m)\n"
	result := StripANSI(input)
	if strings.Contains(result, "\x1b") {
		t.Errorf("StripANSI still contains escape sequences: %q", result)
	}
	if !strings.Contains(result, "Compiling grfn") {
		t.Errorf("StripANSI removed meaningful content: %q", result)
	}
}

func TestStripANSI_OSCSequences(t *testing.T) {
	input := "\x1b]0;window title\x07Some text\x1b]2;another title\x07more text"
	expected := "Some textmore text"
	result := StripANSI(input)
	if result != expected {
		t.Errorf("StripANSI OSC: got %q, want %q", result, expected)
	}
}

func TestStripANSI_ScreenModes(t *testing.T) {
	input := "\x1b[?1049h\x1b[?25lHidden\x1b[?25h\x1b[?1049l"
	result := StripANSI(input)
	if strings.Contains(result, "\x1b") {
		t.Errorf("StripANSI still contains escape sequences: %q", result)
	}
	if !strings.Contains(result, "Hidden") {
		t.Errorf("StripANSI removed meaningful content: %q", result)
	}
}

func TestStripANSI_MultipleBlankLines(t *testing.T) {
	input := "line1\n\n\n\nline2"
	expected := "line1\n\nline2"
	result := StripANSI(input)
	if result != expected {
		t.Errorf("StripANSI blank lines: got %q, want %q", result, expected)
	}
}

func TestStripANSI_NoANSI(t *testing.T) {
	input := "Just plain text\nwith newlines"
	result := StripANSI(input)
	if result != input {
		t.Errorf("StripANSI modified plain text: got %q, want %q", result, input)
	}
}

func TestStripANSIAndTrim_TrailingSpaces(t *testing.T) {
	input := "  line with spaces   \n  indented line  \n"
	result := StripANSIAndTrim(input)
	if strings.Contains(result, "  \n") {
		t.Errorf("StripANSIAndTrim left trailing spaces: %q", result)
	}
}

func TestStripANSIAndTrim_ANSIAndSpaces(t *testing.T) {
	input := "\x1b[32m  Success  \x1b[0m \n\x1b[31m  Error  \x1b[0m\n"
	result := StripANSIAndTrim(input)
	if strings.Contains(result, "\x1b") {
		t.Errorf("StripANSIAndTrim left ANSI: %q", result)
	}
	if strings.Contains(result, "  \n") {
		t.Errorf("StripANSIAndTrim left trailing spaces: %q", result)
	}
}

func TestStripANSI_RealWorldCargoOutput(t *testing.T) {
	// Real cargo build output with ANSI
	input := "\x1b[0m\x1b[1m\x1b[32m Compiling\x1b[0m grfn-cli v0.1.0 (/Users/ss/ai/grfn/crates/grfn-cli)\n\x1b[0m\x1b[1m\x1b[32m Finished\x1b[0m `release` profile [optimized] target(s) in 4.32s\n"
	result := StripANSI(input)
	if strings.Contains(result, "\x1b") {
		t.Errorf("StripANSI left escape sequences: %q", result)
	}
	if !strings.Contains(result, "Compiling grfn-cli") {
		t.Errorf("StripANSI removed meaningful content: %q", result)
	}
	if !strings.Contains(result, "Finished") {
		t.Errorf("StripANSI removed meaningful content: %q", result)
	}
}

func TestStripANSI_NpmOutput(t *testing.T) {
	// npm progress output with carriage returns
	input := "\r  \u2581\u2582\u2583\u2584\u2585\u2586\u2587\u2588\u2587\u2586\u2585\u2584\u2583\u2582\u2581 | 42/100\r  ██████████████░░░░░░░░░░░░░░░░░░░░░ | 42% | ETA: 2s\n"
	result := StripANSI(input)
	if strings.Contains(result, "\x1b") {
		t.Errorf("StripANSI left escape sequences: %q", result)
	}
}

func TestStripANSI_PythonOutput(t *testing.T) {
	// Python tqdm progress bar
	input := "\x1b[2K\r  0%|          | 0/100 [00:00<?, ?it/s]\r 50%|█████     | 50/100 [00:01<00:01, 49.5it/s]\r100%|██████████| 100/100 [00:02<00:00, 49.0it/s]\n"
	result := StripANSI(input)
	if strings.Contains(result, "\x1b") {
		t.Errorf("StripANSI left escape sequences: %q", result)
	}
	// Should keep the last progress line (100%)
	if !strings.Contains(result, "100%") {
		t.Errorf("StripANSI removed meaningful content: %q", result)
	}
}