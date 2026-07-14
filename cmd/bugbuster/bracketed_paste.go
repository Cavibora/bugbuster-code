package main

import (
	"bytes"
	"io"
	"os"
	"sync"

	"github.com/charmbracelet/x/term"
)

// bracketedPasteReader wraps an io.Reader to handle terminal bracketed paste sequences.
//
// When the terminal is in bracketed paste mode (\x1b[?2004h), pasted text is
// wrapped in \x1b[200~ ... \x1b[201~ escape sequences. This reader detects those
// sequences and replaces the embedded newlines with \r (carriage return) so that
// readline treats the entire paste as a single logical line.
//
// This is critical for CLI mode: without it, pasting multiline code results in
// only the first line being processed, with remaining lines treated as separate
// queries — which corrupts the model's context.
type bracketedPasteReader struct {
	src io.Reader
	mu  sync.Mutex
	// leftover from previous read that wasn't consumed
	leftover []byte
	// whether we're inside a paste sequence
	inPaste bool
	// partial escape sequence from previous read (e.g. "\x1b[20" if "\x1b[200~" was split)
	partialEscape []byte
}

// Paste escape sequences
var (
	pasteStartSeq = []byte("\x1b[200~")
	pasteEndSeq   = []byte("\x1b[201~")
	escapeStart   = []byte("\x1b[")
)

// newBracketedPasteReader creates a new reader that handles bracketed paste sequences.
func newBracketedPasteReader(src io.Reader) *bracketedPasteReader {
	return &bracketedPasteReader{
		src: src,
	}
}

// Read implements io.Reader. It reads from the underlying source and processes
// bracketed paste sequences: removes \x1b[200~ and \x1b[201~ markers, and
// replaces \n with \r inside pastes so readline doesn't split them.
func (r *bracketedPasteReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	if len(r.leftover) > 0 {
		n := copy(p, r.leftover)
		r.leftover = r.leftover[n:]
		r.mu.Unlock()
		return n, nil
	}
	r.mu.Unlock()

	// Read from source into a large buffer
	buf := make([]byte, 4096)
	n, err := r.src.Read(buf)
	if n == 0 {
		return 0, err
	}

	data := buf[:n]

	// Prepend any partial escape sequence from previous read
	if len(r.partialEscape) > 0 {
		data = append(r.partialEscape, data...)
		r.partialEscape = nil
	}

	processed := r.processPaste(data)

	// If processed data fits in p, return it directly
	if len(processed) <= len(p) {
		n := copy(p, processed)
		return n, err
	}

	// Otherwise, copy what fits and buffer the rest
	n = copy(p, processed)
	r.mu.Lock()
	r.leftover = append(r.leftover, processed[n:]...)
	r.mu.Unlock()
	return n, err
}

// processPaste processes bracketed paste sequences in the data.
// It handles partial escape sequences that may be split across reads.
func (r *bracketedPasteReader) processPaste(data []byte) []byte {
	// Fast path: no escape sequences
	if !bytes.Contains(data, []byte("\x1b[")) && !r.inPaste {
		return data
	}

	var result bytes.Buffer
	result.Grow(len(data))
	i := 0

	for i < len(data) {
		// Check for paste start sequence \x1b[200~
		if bytes.HasPrefix(data[i:], pasteStartSeq) {
			r.inPaste = true
			i += len(pasteStartSeq)
			continue
		}

		// Check for paste end sequence \x1b[201~
		if bytes.HasPrefix(data[i:], pasteEndSeq) {
			r.inPaste = false
			i += len(pasteEndSeq)
			continue
		}

		// Check for partial escape sequence at end of data
		// This handles the case where \x1b[200~ is split across reads
		if data[i] == '\x1b' && i < len(data)-1 && data[i+1] == '[' {
			// Find the end of this escape sequence (up to ~ or a letter)
			end := i + 2
			for end < len(data) && data[end] >= '0' && data[end] <= '9' {
				end++
			}
			if end < len(data) && (data[end] == '~' || (data[end] >= 'A' && data[end] <= 'Z') || (data[end] >= 'a' && data[end] <= 'z')) {
				// Complete escape sequence — check if it's a paste marker
				seq := data[i : end+1]
				if bytes.Equal(seq, pasteStartSeq) {
					r.inPaste = true
					i = end + 1
					continue
				} else if bytes.Equal(seq, pasteEndSeq) {
					r.inPaste = false
					i = end + 1
					continue
				}
				// Not a paste marker — pass through as-is
				result.Write(data[i : end+1])
				i = end + 1
				continue
			}
			// Incomplete escape sequence at end of data — save for next read
			if end == len(data) || (end < len(data) && data[end] >= '0' && data[end] <= '9') {
				r.partialEscape = make([]byte, len(data[i:]))
				copy(r.partialEscape, data[i:])
				break
			}
			// Unknown escape — pass through
			result.WriteByte(data[i])
			i++
			continue
		}

		if r.inPaste {
			// Inside paste — replace \n with \r so readline treats it as one line
			if data[i] == '\n' {
				result.WriteByte('\r')
				i++
			} else if data[i] == '\r' {
				// Skip standalone \r in paste (terminal may send \r\n)
				i++
			} else {
				result.WriteByte(data[i])
				i++
			}
		} else {
			result.WriteByte(data[i])
			i++
		}
	}

	return result.Bytes()
}

// Close implements io.Closer. It's a no-op since we don't own the underlying reader.
func (r *bracketedPasteReader) Close() error {
	return nil
}

// enableBracketedPaste enables terminal bracketed paste mode.
// When enabled, the terminal wraps pasted text in \x1b[200~ ... \x1b[201~ sequences.
func enableBracketedPaste() {
	if term.IsTerminal(0) {
		os.Stdout.Write([]byte("\x1b[?2004h"))
		os.Stdout.Sync()
	}
}

// disableBracketedPaste disables terminal bracketed paste mode.
func disableBracketedPaste() {
	if term.IsTerminal(0) {
		os.Stdout.Write([]byte("\x1b[?2004l"))
		os.Stdout.Sync()
	}
}