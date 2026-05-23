package agent

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Pre-compiled regular expressions for parsing tool calls.
// Stored as package-level variables to avoid re-compilation on each call.
var (
	toolTagRe      = regexp.MustCompile(`<tool\s+name="([^"]+)"\s*>([\s\S]*?)</tool>`)
	xmlTagRe       = regexp.MustCompile(`<([^>]+)>([\s\S]*?)</([^>]+)>`)
	nameAttrRe     = regexp.MustCompile(`name=["']([^"']+)["']`)
	boxDrawRe      = regexp.MustCompile("[│┌└┐┘─├┤┬┴┼]")
	toolTagCleanRe = regexp.MustCompile(`<tool\s+name="[^"]+"\s*>[\s\S]*?</tool>`)
)

// ToolCall — tool call extracted from model response
type ToolCall struct {
	Name   string
	Params map[string]string
}

// ParseToolCalls extracts tool calls from model response
// Supports two formats:
// 1. XML: <tool name="read"><path>main.go</path></tool>
// 2. JSON: {"tool": "read", "params": {"path": "main.go"}}
func ParseToolCalls(response string) []ToolCall {
	var calls []ToolCall

	// Try XML format first
	xmlCalls := parseXMLToolCalls(response)
	if len(xmlCalls) > 0 {
		return xmlCalls
	}

	// Then try JSON format
	jsonCalls := parseJSONToolCalls(response)
	if len(jsonCalls) > 0 {
		return jsonCalls
	}

	return calls
}

// parseXMLToolCalls parses <tool name="...">...</tool> format
func parseXMLToolCalls(response string) []ToolCall {
	var calls []ToolCall

	matches := toolTagRe.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		toolName := match[1]
		innerContent := strings.TrimSpace(match[2])

		params := parseXMLParams(innerContent)
		calls = append(calls, ToolCall{
			Name:   toolName,
			Params: params,
		})
	}

	return calls
}

// parseXMLParams parses nested <param>value</param> or <key>value</key>
// Supports formats:
// 1. Named tags: <path>main.go</path> — key = tag name
// 2. Alternating <param>: <param>path</param><param>main.go</param> — keys and values alternate
// 3. <param> with name attribute: <param name="path">main.go</param> — key from attribute name
// 4. Single <param>value</param> — if no other parameters, key = "param" (tool will return error)
func parseXMLParams(content string) map[string]string {
	params := make(map[string]string)

	matches := xmlTagRe.FindAllStringSubmatch(content, -1)

	// Collect all tag=value pairs
	type pair struct {
		tag      string // tag name (without attributes)
		attrName string // value attribute name, if any
		value    string
	}
	var pairs []pair
	for _, match := range matches {
		if len(match) >= 4 {
			openTag := strings.TrimSpace(match[1])
			value := strings.TrimSpace(match[2])
			closeTag := strings.TrimSpace(match[3])

			// Extract tag name and attribute name from opening tag
			// openTag can be "param" or "param name=\"path\"" or "param name='path'"
			tagName, attrName := parseOpenTag(openTag)

			// Closing tag must match tag name (without attributes)
			if tagName == closeTag {
				pairs = append(pairs, pair{tag: tagName, attrName: attrName, value: value})
			}
		}
	}

	// Check <param> format — alternating key-value
	paramValues := make([]string, 0)
	paramAttrNames := make([]string, 0) // name attributes for each <param>
	for _, p := range pairs {
		if p.tag == "param" {
			paramValues = append(paramValues, p.value)
			paramAttrNames = append(paramAttrNames, p.attrName)
		}
	}

	// If <param> tags have name attributes — use them
	if len(paramValues) > 0 {
		hasAttrName := false
		for _, a := range paramAttrNames {
			if a != "" {
				hasAttrName = true
				break
			}
		}

		if hasAttrName {
			// Format: <param name="key">value</param>
			// If <param> has name attribute — use it as key
			// If not — value can be a key (alternating format)
			allHaveAttr := true
			for _, a := range paramAttrNames {
				if a == "" {
					allHaveAttr = false
					break
				}
			}

			if allHaveAttr {
				// All <param> have name attribute — format <param name="key">value</param>
				for i, p := range pairs {
					if p.tag == "param" {
						params[paramAttrNames[i]] = p.value
					}
				}
				// Add non-param tags
				for _, p := range pairs {
					if p.tag != "param" {
						params[p.tag] = p.value
					}
				}
				return params
			}
		}

		// Alternating <param>key</param><param>value</param> — even count
		if len(paramValues)%2 == 0 {
			for i := 0; i+1 < len(paramValues); i += 2 {
				params[paramValues[i]] = paramValues[i+1]
			}
			// Add non-param tags
			for _, p := range pairs {
				if p.tag != "param" {
					params[p.tag] = p.value
				}
			}
			return params
		}

		// Odd count of <param> — try to interpret as
		// <param name="key">value</param> if any attributes,
		// or leave as is (key = "param")
		for i, p := range pairs {
			if p.tag == "param" {
				if paramAttrNames[i] != "" {
					params[paramAttrNames[i]] = p.value
				} else {
					params["param"] = p.value
				}
			} else {
				params[p.tag] = p.value
			}
		}
		return params
	}

	// Regular format: tag name = key
	for _, p := range pairs {
		params[p.tag] = p.value
	}

	return params
}

// parseOpenTag extracts tag name and attribute name from opening tag
// Examples: "param" → ("param", ""), "param name=\"path\"" → ("param", "path")
func parseOpenTag(openTag string) (tagName, attrName string) {
	if m := nameAttrRe.FindStringSubmatch(openTag); len(m) >= 2 {
		attrName = m[1]
	}
	// Tag name — first word before space
	if idx := strings.Index(openTag, " "); idx >= 0 {
		tagName = openTag[:idx]
	} else {
		tagName = openTag
	}
	return tagName, attrName
}

// parseJSONToolCalls parses JSON-format calls tools
// Supports:
// 1. Clean JSON: {"tool": "read", "params": {"path": "main.go"}}
// 2. Markdown wrappers: ```json\n{"tool": "read", ...}\n```
// 3. Box-drawing characters (│, ┌, └) — cleaned before parsing
func parseJSONToolCalls(response string) []ToolCall {
	var calls []ToolCall

	// Extract JSON from markdown blocks (```json ... ```)
	jsonBlocks := extractJSONBlocks(response)

	for _, block := range jsonBlocks {
		// Clean from box-drawing characters and other artifacts
		cleaned := cleanJSON(block)
		if call, ok := parseSingleJSONToolCall(cleaned); ok {
			calls = append(calls, call)
		}
	}

	// Also search for plain JSON in lines (without markdown wrappers)
	if len(calls) == 0 {
		lines := strings.Split(response, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Remove box-drawing characters
			line = cleanJSON(line)
			if !strings.HasPrefix(line, "{") {
				continue
			}
			if call, ok := parseSingleJSONToolCall(line); ok {
				calls = append(calls, call)
			}
		}
	}

	// If nothing found — try to clean the entire text from box-drawing
	// and parse multi-line JSON
	if len(calls) == 0 {
		cleaned := cleanJSON(response)
		// Search for all JSON objects with "tool" key
		calls = findJSONToolCallsInText(cleaned)
	}

	return calls
}

// extractJSONBlocks extracts JSON blocks from markdown wrappers
func extractJSONBlocks(response string) []string {
	var blocks []string

	// Search for ```json ... ``` or ``` ... ``` blocks
	// Use manual parsing instead of regex due to backticks
	i := 0
	for {
		// Search for block start: ```
		start := strings.Index(response[i:], "```")
		if start == -1 {
			break
		}
		start += i
		// Skip ``` and optional "json"
		contentStart := start + 3
		// Skip "json" and spaces
		rest := response[contentStart:]
		if strings.HasPrefix(rest, "json") {
			contentStart += 4
			rest = response[contentStart:]
		}
		// Skip spaces and line breaks
		for contentStart < len(response) && (response[contentStart] == ' ' || response[contentStart] == '\n' || response[contentStart] == '\r') {
			contentStart++
		}
		// Search for block end: ```
		end := strings.Index(response[contentStart:], "```")
		if end == -1 {
			break
		}
		end += contentStart
		block := response[contentStart:end]
		// Remove trailing whitespace/newlines
		block = strings.TrimSpace(block)
		if block != "" {
			blocks = append(blocks, block)
		}
		i = end + 3
	}

	return blocks
}

// cleanJSON removes box-drawing characters and other artifacts from JSON
func cleanJSON(s string) string {
	s = boxDrawRe.ReplaceAllString(s, "")

	// Remove extra spaces at the beginning of lines
	lines := strings.Split(s, "\n")
	var cleaned []string
	for _, line := range lines {
		cleaned = append(cleaned, strings.TrimSpace(line))
	}
	s = strings.Join(cleaned, "\n")

	return s
}

// parseSingleJSONToolCall parses a single JSON tool call
func parseSingleJSONToolCall(s string) (ToolCall, bool) {
	var call struct {
		Tool   string            `json:"tool"`
		Params map[string]string `json:"params"`
	}

	if err := json.Unmarshal([]byte(s), &call); err != nil {
		return ToolCall{}, false
	}

	if call.Tool == "" {
		return ToolCall{}, false
	}

	if call.Params == nil {
		call.Params = make(map[string]string)
	}

	return ToolCall{
		Name:   call.Tool,
		Params: call.Params,
	}, true
}

// findJSONToolCallsInText searches for JSON objects with "tool" key in text
func findJSONToolCallsInText(text string) []ToolCall {
	var calls []ToolCall
	// Search for all occurrences of { ... } and try to parse
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		// Search for end of JSON object
		depth := 0
		j := i
		for j < len(text) {
			if text[j] == '{' {
				depth++
			} else if text[j] == '}' {
				depth--
				if depth == 0 {
					// Try to parse
					candidate := text[i : j+1]
					if call, ok := parseSingleJSONToolCall(candidate); ok {
						calls = append(calls, call)
						i = j
					}
					break
				}
			}
			j++
		}
	}
	return calls
}

// StripToolCalls removes tool calls from response, leaving text
func StripToolCalls(response string) string {
	result := toolTagCleanRe.ReplaceAllString(response, "")

	// Remove markdown wrappers with JSON tool calls (manual parsing)
	result = stripMarkdownToolBlocks(result)

	// Remove JSON format (lines like {"tool": "..."})
	lines := strings.Split(result, "\n")
	var filtered []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, `{"tool"`) {
			continue
		}
		filtered = append(filtered, line)
	}

	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

// stripMarkdownToolBlocks removes markdown blocks with JSON tool calls
func stripMarkdownToolBlocks(response string) string {
	var result strings.Builder
	i := 0
	for {
		start := strings.Index(response[i:], "```")
		if start == -1 {
			result.WriteString(response[i:])
			break
		}
		start += i
		result.WriteString(response[i:start])
		// Skip ```
		contentStart := start + 3
		// Skip "json" and spaces
		rest := response[contentStart:]
		isJSON := strings.HasPrefix(rest, "json")
		// Search for block end
		end := strings.Index(response[contentStart:], "```")
		if end == -1 {
			// No closing ``` — leave as is
			result.WriteString(response[start:])
			break
		}
		end += contentStart
		block := response[contentStart:end]
		// If this is a JSON block with tool call — skip
		trimmedBlock := strings.TrimSpace(block)
		if isJSON && strings.Contains(trimmedBlock, `"tool"`) {
			// Skip this block
		} else {
			// Leave block as is
			result.WriteString("```")
			result.WriteString(block)
			result.WriteString("```")
		}
		i = end + 3
	}
	return result.String()
}
