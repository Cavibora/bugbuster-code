package provider

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
)

// ParseSSE parses Server-Sent Events from HTTP response
// Calls callback for each event
func ParseSSE(body io.Reader, callback func(event, data string)) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var event, data strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// Empty line = end of event
			if data.Len() > 0 {
				callback(event.String(), data.String())
				event.Reset()
				data.Reset()
			}
			continue
		}

		if strings.HasPrefix(line, "event:") {
			event.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "event:")))
		} else if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteString("\n")
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		} else if strings.HasPrefix(line, "id:") || strings.HasPrefix(line, "retry:") || strings.HasPrefix(line, ":") {
			// Ignore id, retry and comments
			continue
		} else if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			// Unknown line — may be data without prefix
			if data.Len() > 0 {
				data.WriteString("\n")
			}
			data.WriteString(line)
		}
	}

	// Last event (if not empty lines at the end)
	if data.Len() > 0 {
		callback(event.String(), data.String())
	}

	return scanner.Err()
}

// ParseSSELines parses SSE and returns all data-lines as a slice
func ParseSSELines(body io.Reader) ([]string, error) {
	var lines []string
	err := ParseSSE(body, func(event, data string) {
		lines = append(lines, data)
	})
	return lines, err
}

// ExtractJSONFromSSE extracts JSON objects from SSE data lines
// Format: data: {"key": "value"} or data: [DONE]
func ExtractJSONFromSSE(body io.Reader, callback func(jsonStr string) error) error {
	return ParseSSE(body, func(event, data string) {
		// Skip [DONE] marker
		if data == "[DONE]" {
			return
		}

		// There may be multiple JSON objects in one data
		for _, chunk := range splitJSON(data) {
			chunk = strings.TrimSpace(chunk)
			if chunk == "" || chunk == "[DONE]" {
				continue
			}
			if err := callback(chunk); err != nil {
				return
			}
		}
	})
}

// splitJSON splits a line into separate JSON objects.
// Handles escaped strings to not count { and } inside strings.
func splitJSON(s string) []string {
	var result []string
	depth := 0
	start := -1
	inString := false
	escape := false

	for i, ch := range s {
		if escape {
			escape = false
			continue
		}
		if ch == '\\' && inString {
			escape = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 && start >= 0 {
				result = append(result, s[start:i+1])
				start = -1
			}
		}
	}

	return result
}

// ReadFullBody reads the entire response body (for non-streaming requests)
func ReadFullBody(body io.Reader) ([]byte, error) {
	// Limit reading to 10MB
	return io.ReadAll(io.LimitReader(body, 10*1024*1024))
}

// FormatHTTPError formats HTTP error with response body
func FormatHTTPError(statusCode int, body []byte) error {
	// Truncate long error body
	bodyStr := string(body)
	if len(bodyStr) > 500 {
		bodyStr = bodyStr[:500] + "..."
	}
	return fmt.Errorf("HTTP %d: %s", statusCode, bodyStr)
}

// IsJSON checks if a line looks like JSON
func IsJSON(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")
}

// ConcatDataChunks merges SSE data chunks into one JSON
func ConcatDataChunks(chunks []string) string {
	var buf bytes.Buffer
	for _, chunk := range chunks {
		buf.WriteString(chunk)
	}
	return buf.String()
}
