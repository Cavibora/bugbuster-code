package agent

import (
	"encoding/json"
	"fmt"
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
	// Angle bracket key=value format: <tool_name=bash> <parameter=command> ls ...
	angleBracketKVRe = regexp.MustCompile(`<tool_name=(\w+)\s*>[\s\S]*?<parameter=(\w+)\s*>([\s\S]*?)(?:</parameter>|</tool_name>|$)`)
)

// ToolCall — tool call extracted from model response
type ToolCall struct {
	Name   string
	Params map[string]string
}

// toolAliases maps alternative tool names to canonical names.
// Models often use different naming conventions (PascalCase, snake_case, etc.)
var toolAliases = map[string]string{
	// Read tool aliases
	"Read": "read", "READ": "read", "file_read": "read", "fileread": "read",
	"cat": "read", "open": "read", "view": "read", "show": "read",
	"get_file": "read", "getfile": "read", "load_file": "read",
	"read_file": "read", "readfile": "read", "fread": "read",
	// Write tool aliases
	"Write": "write", "WRITE": "write", "file_write": "write", "filewrite": "write",
	"save": "write", "create_file": "write", "createfile": "write",
	"write_file": "write", "writefile": "write", "fwrite": "write",
	// Edit tool aliases
	"Edit": "edit", "EDIT": "edit", "file_edit": "edit", "fileedit": "edit",
	"replace": "edit", "sed": "edit", "patch": "edit", "modify": "edit",
	"edit_file": "edit", "editfile": "edit",
	// Bash tool aliases
	"Bash": "bash", "BASH": "bash", "shell": "bash", "exec": "bash",
	"execute": "bash", "run": "bash", "command": "bash", "cmd": "bash",
	"terminal": "bash", "cli": "bash",
	// Grep tool aliases
	"Grep": "grep", "GREP": "grep", "search": "grep", "find_text": "grep",
	"findtext": "grep", "file_search": "grep", "filesearch": "grep",
	"ripgrep": "grep", "rg": "grep",
	// Glob tool aliases
	"Glob": "glob", "GLOB": "glob", "find_files": "glob", "findfiles": "glob",
	"file_glob": "glob", "fileglob": "glob", "list_files": "glob",
	// Memory tool aliases
	"Memory": "memory", "MEMORY": "memory", "remember": "memory", "recall": "memory",
	"save_memory": "memory", "load_memory": "memory",
	// LSP tool aliases
	"LSP": "lsp", "lsp_tool": "lsp", "language_server": "lsp",
	"go_to_definition": "lsp", "find_references": "lsp",
	// Browse tool aliases
	"Browse": "browse", "BROWSE": "browse", "web_search": "browse", "websearch": "browse",
	"search_web": "browse", "internet_search": "browse", "web_browse": "browse",
	// WebFetch tool aliases
	"WebFetch": "web_fetch", "webfetch": "web_fetch", "fetch": "web_fetch",
	"curl": "web_fetch", "wget": "web_fetch", "http_get": "web_fetch",
	"http_request": "web_fetch", "url_fetch": "web_fetch",
	// AskUser tool aliases
	"AskUser": "ask_user", "askuser": "ask_user", "ask": "ask_user",
	"question": "ask_user", "prompt_user": "ask_user", "input": "ask_user",
	// TodoWrite tool aliases
	"TodoWrite": "todo_write", "todowrite": "todo_write", "todo": "todo_write",
	"add_todo": "todo_write", "set_todo": "todo_write",
	// TodoRead tool aliases
	"TodoRead": "todo_read", "todoread": "todo_read", "list_todo": "todo_read",
	"get_todo": "todo_read",
	// Learn tool aliases
	"Learn": "learn", "LEARN": "learn", "train": "learn", "teach": "learn",
	// DelegateTask tool aliases
	"DelegateTask": "delegate_task", "delegatetask": "delegate_task",
	"delegate": "delegate_task", "subagent": "delegate_task", "sub_task": "delegate_task",
	// Background tool aliases
	"Background": "background", "bg": "background", "background_run": "background",
	// PS tool aliases
	"PS": "ps", "Ps": "ps", "process_list": "ps", "processlist": "ps",
	"processes": "ps", "jobs": "ps",
	// Logs tool aliases
	"Logs": "logs", "LOGS": "logs", "log": "logs", "tail": "logs",
	"output": "logs", "stdout": "logs",
	// Kill tool aliases
	"Kill": "kill", "KILL": "kill", "kill_process": "kill", "killprocess": "kill",
	"terminate": "kill", "stop_process": "kill",
}

// normalizeToolName maps alternative tool names to canonical names.
// Handles: PascalCase, UPPER_CASE, snake_case, common aliases.
func normalizeToolName(name string) string {
	// Direct alias lookup
	if canonical, ok := toolAliases[name]; ok {
		return canonical
	}
	// Try lowercase
	lower := strings.ToLower(name)
	if lower != name {
		if canonical, ok := toolAliases[lower]; ok {
			return canonical
		}
		return lower // e.g. "Read" → "read"
	}
	// Try replacing hyphens with underscores (web-fetch → web_fetch)
	underscore := strings.ReplaceAll(name, "-", "_")
	if underscore != name {
		if canonical, ok := toolAliases[underscore]; ok {
			return canonical
		}
		return underscore
	}
	return name
}

// findClosestToolName finds the closest matching tool name using Levenshtein distance.
// Returns the canonical tool name and true if found, or empty string and false.
func findClosestToolName(name string, availableTools map[string]bool) (string, bool) {
	normalized := normalizeToolName(name)
	if availableTools[normalized] {
		return normalized, true
	}

	// Try Levenshtein distance (max 2 edits)
	bestDist := 3 // max allowed distance
	bestTool := ""
	for toolName := range availableTools {
		d := levenshteinDistance(normalized, toolName)
		if d < bestDist {
			bestDist = d
			bestTool = toolName
		}
	}
	if bestDist <= 2 && bestTool != "" {
		return bestTool, true
	}
	return "", false
}

// levenshteinDistance computes the Levenshtein distance between two strings.
func levenshteinDistance(a, b string) int {
	aLen := len(a)
	bLen := len(b)
	if aLen == 0 {
		return bLen
	}
	if bLen == 0 {
		return aLen
	}

	// Use a single row for DP
	row := make([]int, bLen+1)
	for j := 0; j <= bLen; j++ {
		row[j] = j
	}

	for i := 1; i <= aLen; i++ {
		prev := row[0]
		row[0] = i
		for j := 1; j <= bLen; j++ {
			temp := row[j]
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			row[j] = minOf3(
				row[j]+1,     // deletion
				row[j-1]+1,   // insertion
				prev+cost,    // substitution
			)
			prev = temp
		}
	}
	return row[bLen]
}

func minOf3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// ParseToolCalls extracts tool calls from model response
// Supports multiple formats:
// 1. XML: <tool name="read"><path>main.go</path></tool>
// 2. Angle bracket key=value: <tool_name=bash> <parameter=command> ls ...
// 3. JSON: {"tool": "read", "params": {"path": "main.go"}}
// 4. Function call: read(path="/tmp/main.go")
// 5. Backtick tool: `read("/tmp/main.go")`
// 6. Ollama format: ```tool\nread\npath=/tmp/main.go\n```
// 7. Auto-detect: JSON without "tool" key — detect by structure
// 8. Markdown code blocks: ```bash\nls /tmp/\n``` → bash(command="ls /tmp/")
// 9. YAML: tool: read\npath: main.go
func ParseToolCalls(response string) []ToolCall {
	var calls []ToolCall

	// Try XML format first
	xmlCalls := parseXMLToolCalls(response)
	if len(xmlCalls) > 0 {
		return xmlCalls
	}

	// Try angle bracket key=value format: <tool_name=bash> <parameter=command> ls ...
	angleCalls := parseAngleBracketKVToolCalls(response)
	if len(angleCalls) > 0 {
		return angleCalls
	}

	// Then try JSON format
	jsonCalls := parseJSONToolCalls(response)
	if len(jsonCalls) > 0 {
		return jsonCalls
	}

	// Try function call format: tool_name(param1="value1", param2="value2")
	funcCalls := parseFuncCallToolCalls(response)
	if len(funcCalls) > 0 {
		return funcCalls
	}

	// Try auto-detect: JSON without "tool" key — detect by structure
	autoCalls := parseAutoDetectToolCalls(response)
	if len(autoCalls) > 0 {
		return autoCalls
	}

	// Try Markdown code blocks: ```bash\nls /tmp/\n``` → bash(command="ls /tmp/")
	mdCalls := parseMarkdownCodeBlocks(response)
	if len(mdCalls) > 0 {
		return mdCalls
	}

	// Try YAML format: tool: read\npath: main.go
	yamlCalls := parseYAMLToolCalls(response)
	if len(yamlCalls) > 0 {
		return yamlCalls
	}

	return calls
}

// parseMarkdownCodeBlocks extracts tool calls from markdown code blocks
// e.g. ```bash\nls /tmp/\n``` or ```python\nprint("hello")\n```
func parseMarkdownCodeBlocks(response string) []ToolCall {
	var calls []ToolCall

	// Match ```lang\n...``` blocks
	re := regexp.MustCompile("(?s)```(\\w+)\\s*\n(.*?)```")
	matches := re.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		lang := strings.ToLower(strings.TrimSpace(match[1]))
		code := strings.TrimSpace(match[2])
		if code == "" {
			continue
		}

		switch lang {
		case "bash", "sh", "shell", "zsh", "fish":
			calls = append(calls, ToolCall{
				Name:   "bash",
				Params: map[string]string{"command": code},
			})
		case "python", "python3", "py":
			calls = append(calls, ToolCall{
				Name:   "bash",
				Params: map[string]string{"command": "python3 " + code},
			})
		case "javascript", "js", "node":
			calls = append(calls, ToolCall{
				Name:   "bash",
				Params: map[string]string{"command": "node -e " + code},
			})
		case "go":
			calls = append(calls, ToolCall{
				Name:   "bash",
				Params: map[string]string{"command": "go run " + code},
			})
		}
	}

	return calls
}

// parseYAMLToolCalls extracts tool calls from YAML format
// e.g. tool: read\npath: main.go
func parseYAMLToolCalls(response string) []ToolCall {
	var calls []ToolCall

	// Match YAML blocks with tool: field
	re := regexp.MustCompile(`(?m)tool:\s*(\w+)(?:\n(?:[ \t]+\w+:[^\n]*))*`)
	matches := re.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		toolName := strings.ToLower(strings.TrimSpace(match[1]))
		normalized := normalizeToolName(toolName)

		// Check if it's a known tool
		known := map[string]bool{
			"read": true, "write": true, "edit": true, "bash": true,
			"grep": true, "glob": true, "memory": true, "lsp": true,
			"browse": true, "web_fetch": true, "ask_user": true,
			"todo_write": true, "todo_read": true, "learn": true,
			"delegate_task": true, "kill": true, "ps": true, "logs": true,
			"background": true,
		}
		if !known[normalized] {
			continue
		}

		// Parse YAML-like key: value pairs
		block := match[0]
		params := make(map[string]string)
		lineRe := regexp.MustCompile(`(?m)^\s*(\w+):\s*(.+)$`)
		for _, lineMatch := range lineRe.FindAllStringSubmatch(block, -1) {
			key := strings.ToLower(strings.TrimSpace(lineMatch[1]))
			val := strings.TrimSpace(lineMatch[2])
			if key == "tool" {
				continue
			}
			params[key] = val
		}

		if len(params) > 0 {
			calls = append(calls, ToolCall{
				Name:   normalized,
				Params: params,
			})
		}
	}

	return calls
}

// parseAngleBracketKVToolCalls parses angle bracket key=value format:
// <tool_name=bash> <parameter=command> ls /tmp/
// <tool_name=read> <parameter=path> /tmp/main.go
// <tool_name=write> <parameter=path> /tmp/file.txt <parameter=content> hello world
// Also supports variations:
// <tool_name="bash"> <parameter="command"> ls /tmp/
// <tool_name=bash><parameter=command>ls /tmp/</parameter></tool_name>
func parseAngleBracketKVToolCalls(response string) []ToolCall {
	var calls []ToolCall

	// Pattern 1: <tool_name=X> <parameter=Y> value </parameter> </tool_name>
	// With optional quotes around values
	kvRe := regexp.MustCompile(`<tool_name=["']?(\w+)["']?\s*>([\s\S]*?)</tool_name>`)
	matches := kvRe.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		toolName := normalizeToolName(match[1])
		innerContent := strings.TrimSpace(match[2])

		// Extract parameters from inner content
		params := parseAngleBracketKVParams(innerContent)
		if len(params) > 0 {
			calls = append(calls, ToolCall{
				Name:   toolName,
				Params: params,
			})
		}
	}

	// Pattern 2: <tool_name=X> <parameter=Y> value (without closing tags)
	// This is the most common format models use
	if len(calls) == 0 {
		// Match: <tool_name=X> followed by <parameter=Y> value pairs
		simpleRe := regexp.MustCompile(`<tool_name=["']?(\w+)["']?\s*>`)
		simpleMatches := simpleRe.FindAllStringSubmatchIndex(response, -1)

		for i, loc := range simpleMatches {
			if len(loc) < 4 {
				continue
			}
			toolName := normalizeToolName(response[loc[2]:loc[3]])

			// Find the end of this tool call (next <tool_name> or end of text)
			var endIdx int
			if i+1 < len(simpleMatches) {
				endIdx = simpleMatches[i+1][0]
			} else {
				endIdx = len(response)
			}

			innerContent := response[loc[1]:endIdx]
			params := parseAngleBracketKVParams(innerContent)

			if len(params) > 0 {
				calls = append(calls, ToolCall{
					Name:   toolName,
					Params: params,
				})
			}
		}
	}

	// Pattern 3: Generic <X=Y> value </X> format (not tool_name/parameter specific)
	// <bash command="ls /tmp/"> or <bash> ls /tmp/ </bash>
	if len(calls) == 0 {
		// Match known tool names as tags: <bash>...</bash>, <read>...</read>, etc.
		knownTools := map[string]bool{
			"read": true, "write": true, "edit": true, "bash": true,
			"grep": true, "glob": true, "memory": true, "lsp": true,
			"browse": true, "web_fetch": true, "ask_user": true,
			"todo_write": true, "todo_read": true, "learn": true,
			"delegate_task": true, "kill": true, "ps": true, "logs": true,
			"background": true,
		}

		// Match <toolName ...>content</toolName> or <toolName attr="val">content</toolName>
		// We can't use backreferences in Go regex, so we match any closing tag
		genericRe := regexp.MustCompile(`<(\w+)((?:\s+[^>]*)?)\s*>([\s\S]*?)</\w+>`)
		genericMatches := genericRe.FindAllStringSubmatch(response, -1)

		for _, match := range genericMatches {
			if len(match) < 4 {
				continue
			}
			rawName := match[1]
			attrs := match[2]
			content := strings.TrimSpace(match[3])

			toolName := normalizeToolName(rawName)
			if !knownTools[toolName] {
				if closest, found := findClosestToolName(rawName, knownTools); found {
					toolName = closest
				} else {
					continue
				}
			}

			params := make(map[string]string)

			// Parse attributes: attr="value" or attr='value'
			if attrs != "" {
				attrRe := regexp.MustCompile(`(\w+)\s*=\s*["']([^"']*)["']`)
				attrMatches := attrRe.FindAllStringSubmatch(attrs, -1)
				for _, am := range attrMatches {
					if len(am) >= 3 {
						params[am[1]] = am[2]
					}
				}
			}

			// Parse inner content as <parameter=X> value pairs
			innerParams := parseAngleBracketKVParams(content)
			for k, v := range innerParams {
				params[k] = v
			}

			// If no params from attributes or inner tags, use content as default param
			if len(params) == 0 && content != "" {
				paramName := defaultParamName(toolName)
				params[paramName] = content
			}

			if len(params) > 0 {
				calls = append(calls, ToolCall{
					Name:   toolName,
					Params: params,
				})
			}
		}
	}

	return calls
}

// parseAngleBracketKVParams extracts key=value pairs from angle bracket format:
// <parameter=command> ls /tmp/ </parameter>
// <parameter=path> /tmp/main.go </parameter>
// <parameter="command"> ls /tmp/ </parameter>
func parseAngleBracketKVParams(content string) map[string]string {
	params := make(map[string]string)

	// Pattern 1: <parameter=X> value </parameter> (with closing tag)
	// Match: <parameter=X>...</parameter> or <param=X>...</param>
	closeTagRe := regexp.MustCompile(`<(?:parameter|param|arg|argument)=["']?(\w+)["']?\s*>([\s\S]*?)</(?:parameter|param|arg|argument)>`)
	matches := closeTagRe.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			paramName := match[1]
			value := strings.TrimSpace(match[2])
			if value != "" {
				params[paramName] = value
			}
		}
	}
	if len(params) > 0 {
		return params
	}

	// Pattern 2: <parameter=X> value (without closing tag, value until end of line)
	// This handles: <parameter=command> ls /tmp/
	simpleParamRe := regexp.MustCompile(`<(?:parameter|param|arg|argument)=["']?(\w+)["']?\s*>[\s]*([^\n<]+)`)
	simpleMatches := simpleParamRe.FindAllStringSubmatch(content, -1)
	for _, match := range simpleMatches {
		if len(match) >= 3 {
			paramName := match[1]
			value := strings.TrimSpace(match[2])
			if value != "" {
				params[paramName] = value
			}
		}
	}

	// Pattern 3: <X=Y> value (generic key=value tag)
	if len(params) == 0 {
		genericRe := regexp.MustCompile(`<(\w+)=["']?(\w+)["']?\s*>[\s]*([^\n<]+)`)
		genericMatches := genericRe.FindAllStringSubmatch(content, -1)
		for _, match := range genericMatches {
			if len(match) >= 4 {
				tagName := match[1]
				paramName := match[2]
				value := strings.TrimSpace(match[3])
				if value != "" {
					// If tag is "parameter"/"param"/"arg" — use paramName
					if tagName == "parameter" || tagName == "param" || tagName == "arg" || tagName == "argument" {
						params[paramName] = value
					} else {
						// Tag name is the parameter name
						params[tagName] = value
					}
				}
			}
		}
	}

	return params
}

// parseXMLToolCalls parses <tool name="...">...</tool> format
func parseXMLToolCalls(response string) []ToolCall {
	var calls []ToolCall

	matches := toolTagRe.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		// Normalize tool name (handles PascalCase, aliases, etc.)
		toolName := normalizeToolName(match[1])
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
// 4. Auto-detect: JSON without "tool" key — detect by structure
func parseJSONToolCalls(response string) []ToolCall {
	var calls []ToolCall

	// Extract JSON from markdown blocks (```json ... ``` or ``` ... ```)
	jsonBlocks := extractJSONBlocks(response)

	for _, block := range jsonBlocks {
		// Clean from box-drawing characters and other artifacts
		cleaned := cleanJSON(block)
		if call, ok := parseSingleJSONToolCall(cleaned); ok {
			calls = append(calls, call)
		} else {
			// Try auto-detect: JSON without "tool" key
			autoCalls := parseAutoDetectToolCalls(cleaned)
			if len(autoCalls) > 0 {
				calls = append(calls, autoCalls...)
			}
		}
	}

	// Also search for plain JSON in lines (without markdown wrappers)
	if len(calls) == 0 {
		lines := strings.Split(response, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Remove box-drawing characters
			line = cleanJSON(line)
			if !strings.HasPrefix(line, "{") && !strings.HasPrefix(line, "[") {
				continue
			}
			if call, ok := parseSingleJSONToolCall(line); ok {
				calls = append(calls, call)
			} else {
				// Try auto-detect
				autoCalls := parseAutoDetectToolCalls(line)
				if len(autoCalls) > 0 {
					calls = append(calls, autoCalls...)
				}
			}
		}
	}

	// If nothing found — try to clean the entire text from box-drawing
	// and parse multi-line JSON
	if len(calls) == 0 {
		cleaned := cleanJSON(response)
		// Search for all JSON objects with "tool" key
		calls = findJSONToolCallsInText(cleaned)

		// If still nothing — try auto-detect on the cleaned text
		if len(calls) == 0 {
			autoCalls := parseAutoDetectToolCalls(cleaned)
			if len(autoCalls) > 0 {
				calls = append(calls, autoCalls...)
			}
		}
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
		Name:   normalizeToolName(call.Tool),
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

// parseFuncCallToolCalls parses function call format: tool_name(param1="value1", param2="value2")
func parseFuncCallToolCalls(response string) []ToolCall {
	var calls []ToolCall

	// Match: tool_name(key="value", key2="value2")
	// Also match: tool_name("value") for single-arg tools like read
	funcRe := regexp.MustCompile(`(\w+)\(([^)]*)\)`)
	matches := funcRe.FindAllStringSubmatch(response, -1)

	// Known tools and their aliases (normalized)
	knownTools := map[string]bool{
		"read": true, "write": true, "edit": true, "bash": true,
		"grep": true, "glob": true, "memory": true, "lsp": true,
		"browse": true, "web_fetch": true, "ask_user": true,
		"todo_write": true, "todo_read": true, "learn": true,
		"delegate_task": true, "kill": true, "ps": true, "logs": true,
		"background": true, "ask": true,
	}

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		rawToolName := match[1]
		argsStr := match[2]

		// Normalize tool name (handles PascalCase, aliases, etc.)
		toolName := normalizeToolName(rawToolName)

		// Check if it's a known tool (exact match or fuzzy match)
		if !knownTools[toolName] {
			// Try fuzzy match
			if closest, found := findClosestToolName(rawToolName, knownTools); found {
				toolName = closest
			} else {
				continue
			}
		}

		params := make(map[string]string)

		// Parse key="value" pairs
		kvRe := regexp.MustCompile(`(\w+)\s*=\s*"([^"]*)"`)
		kvMatches := kvRe.FindAllStringSubmatch(argsStr, -1)
		for _, kv := range kvMatches {
			if len(kv) >= 3 {
				params[kv[1]] = kv[2]
			}
		}

		// Parse key='value' pairs
		kvSingleRe := regexp.MustCompile(`(\w+)\s*=\s*'([^']*)'`)
		kvSingleMatches := kvSingleRe.FindAllStringSubmatch(argsStr, -1)
		for _, kv := range kvSingleMatches {
			if len(kv) >= 3 {
				params[kv[1]] = kv[2]
			}
		}

		// If no key=value pairs found, try single quoted value: read("/path/to/file")
		if len(params) == 0 {
			singleRe := regexp.MustCompile(`"([^"]*)"`)
			singleMatch := singleRe.FindStringSubmatch(argsStr)
			if len(singleMatch) >= 2 {
				// Determine parameter name based on tool
				paramName := defaultParamName(toolName)
				params[paramName] = singleMatch[1]
			}
			// Also try single quotes
			singleRe2 := regexp.MustCompile(`'([^']*)'`)
			singleMatch2 := singleRe2.FindStringSubmatch(argsStr)
			if len(singleMatch2) >= 2 && len(params) == 0 {
				paramName := defaultParamName(toolName)
				params[paramName] = singleMatch2[1]
			}
			// Try bare path without quotes: read(/path/to/file)
			// Also handles broken XML: read(/path/to/file</param)
			if len(params) == 0 {
				// Strip any trailing XML-like tags (</param>, </param, </path>, etc.)
				cleaned := regexp.MustCompile(`</\w+>?`).ReplaceAllString(argsStr, "")
				// Strip trailing > if present (from broken XML like </param>)
				cleaned = strings.TrimRight(cleaned, ">")
				// Strip leading/trailing whitespace
				cleaned = strings.TrimSpace(cleaned)
				if len(cleaned) > 0 && !strings.Contains(cleaned, "=") && !strings.Contains(cleaned, `"`) {
					// Looks like a bare path/command
					paramName := defaultParamName(toolName)
					params[paramName] = cleaned
				}
			}
		}

		if len(params) > 0 {
			calls = append(calls, ToolCall{
				Name:   toolName,
				Params: params,
			})
		}
	}

	return calls
}

// defaultParamName returns the default parameter name for a tool
func defaultParamName(toolName string) string {
	defaults := map[string]string{
		"read":    "path",
		"write":   "path",
		"edit":    "path",
		"bash":    "command",
		"grep":    "pattern",
		"glob":    "pattern",
		"memory":  "action",
		"lsp":     "operation",
		"browse":  "action",
		"web_fetch": "url",
		"ask_user": "question",
	}
	if name, ok := defaults[toolName]; ok {
		return name
	}
	return "param"
}

// parseAutoDetectToolCalls tries to detect tool calls from JSON without "tool" key.
// Models often output JSON like:
//
//	output:[{"id": "1", "status": "in_progress", "subject": "Reading files"}]
//	{"path": "main.go", "content": "package main..."}
//	{"command": "ls -la"}
//
// Auto-detect maps JSON structure to tool:
//   - id + status + subject → todo_write
//   - path + content → write
//   - path + old + new → edit
//   - path → read
//   - command → bash
//   - pattern + path → grep
//   - pattern → glob
//   - url → web_fetch
//   - question → ask_user
//   - action + key + value → memory
func parseAutoDetectToolCalls(response string) []ToolCall {
	// Strip common prefixes like "output:", "result:", "tool_call:"
	cleaned := response
	for _, prefix := range []string{"output:", "result:", "tool_call:", "call:", "action:"} {
		if strings.HasPrefix(strings.TrimSpace(cleaned), prefix) {
			cleaned = strings.TrimSpace(cleaned)[len(prefix):]
			break
		}
	}
	cleaned = strings.TrimSpace(cleaned)

	// Try to parse as JSON array: [{...}, {...}]
	if strings.HasPrefix(cleaned, "[") {
		var arr []map[string]interface{}
		if err := json.Unmarshal([]byte(cleaned), &arr); err == nil {
			var calls []ToolCall
			for _, obj := range arr {
				if call, ok := detectToolFromJSON(obj); ok {
					calls = append(calls, call)
				}
			}
			if len(calls) > 0 {
				return calls
			}
		}
	}

	// Try to parse as single JSON object: {...}
	if strings.HasPrefix(cleaned, "{") {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(cleaned), &obj); err == nil {
			if call, ok := detectToolFromJSON(obj); ok {
				return []ToolCall{call}
			}
		}
	}

	// Try to find JSON arrays/objects anywhere in the text
	// Models often wrap JSON in text like: "Here's my plan:\n[{...}]"
	return findAutoDetectJSONInText(cleaned)
}

// detectToolFromJSON maps a JSON object to a tool call based on its structure
func detectToolFromJSON(obj map[string]interface{}) (ToolCall, bool) {
	// Check for todo_write: id + status + subject
	if hasKeys(obj, "id", "subject") || hasKeys(obj, "id", "status") {
		params := make(map[string]string)
		params["action"] = "save"
		if v, ok := obj["id"]; ok {
			params["id"] = fmtJSON(v)
		}
		if v, ok := obj["subject"]; ok {
			params["subject"] = fmtJSON(v)
		}
		if v, ok := obj["status"]; ok {
			params["status"] = fmtJSON(v)
		}
		if v, ok := obj["description"]; ok {
			params["description"] = fmtJSON(v)
		}
		// Build todos JSON
		todos := buildTodosJSON(obj)
		if todos != "" {
			params["todos"] = todos
		}
		return ToolCall{Name: "todo_write", Params: params}, true
	}

	// Check for write: path + content
	if hasKeys(obj, "path", "content") {
		return ToolCall{
			Name:   "write",
			Params: map[string]string{"path": fmtJSON(obj["path"]), "content": fmtJSON(obj["content"])},
		}, true
	}

	// Check for edit: path + old + new
	if hasKeys(obj, "path", "old", "new") {
		return ToolCall{
			Name:   "edit",
			Params: map[string]string{"path": fmtJSON(obj["path"]), "old": fmtJSON(obj["old"]), "new": fmtJSON(obj["new"])},
		}, true
	}

	// Check for bash: command
	if hasKeys(obj, "command") {
		return ToolCall{
			Name:   "bash",
			Params: map[string]string{"command": fmtJSON(obj["command"])},
		}, true
	}

	// Check for grep: pattern
	if hasKeys(obj, "pattern") && hasKeys(obj, "path") {
		return ToolCall{
			Name:   "grep",
			Params: map[string]string{"pattern": fmtJSON(obj["pattern"]), "path": fmtJSON(obj["path"])},
		}, true
	}

	// Check for glob: pattern (without path)
	if hasKeys(obj, "pattern") && !hasKeys(obj, "path") {
		return ToolCall{
			Name:   "glob",
			Params: map[string]string{"pattern": fmtJSON(obj["pattern"])},
		}, true
	}

	// Check for web_fetch: url
	if hasKeys(obj, "url") {
		return ToolCall{
			Name:   "web_fetch",
			Params: map[string]string{"url": fmtJSON(obj["url"])},
		}, true
	}

	// Check for ask_user: question
	if hasKeys(obj, "question") {
		return ToolCall{
			Name:   "ask_user",
			Params: map[string]string{"question": fmtJSON(obj["question"])},
		}, true
	}

	// Check for read: path only
	if hasKeys(obj, "path") && !hasKeys(obj, "content") && !hasKeys(obj, "old") {
		return ToolCall{
			Name:   "read",
			Params: map[string]string{"path": fmtJSON(obj["path"])},
		}, true
	}

	// Check for memory: action + key/value
	if hasKeys(obj, "action") && (hasKeys(obj, "key") || hasKeys(obj, "value")) {
		params := map[string]string{"action": fmtJSON(obj["action"])}
		if v, ok := obj["key"]; ok {
			params["key"] = fmtJSON(v)
		}
		if v, ok := obj["value"]; ok {
			params["value"] = fmtJSON(v)
		}
		return ToolCall{Name: "memory", Params: params}, true
	}

	return ToolCall{}, false
}

// hasKeys checks if a map has all the specified keys
func hasKeys(obj map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		if _, ok := obj[key]; !ok {
			return false
		}
	}
	return true
}

// fmtJSON formats a JSON value as a string
func fmtJSON(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%v", val)
	case int:
		return fmt.Sprintf("%d", val)
	case bool:
		return fmt.Sprintf("%v", val)
	case nil:
		return ""
	case []interface{}:
		// Convert array to JSON string
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	case map[string]interface{}:
		// Convert object to JSON string
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// buildTodosJSON builds a JSON array of todos from a todo_write-like object
func buildTodosJSON(obj map[string]interface{}) string {
	// If the object itself is a todo item, wrap it in an array
	todo := map[string]interface{}{}
	if v, ok := obj["id"]; ok {
		todo["id"] = v
	}
	if v, ok := obj["subject"]; ok {
		todo["subject"] = v
	}
	if v, ok := obj["status"]; ok {
		todo["status"] = v
	}
	if v, ok := obj["description"]; ok {
		todo["description"] = v
	}
	if len(todo) > 0 {
		b, err := json.Marshal([]map[string]interface{}{todo})
		if err == nil {
			return string(b)
		}
	}
	return ""
}

// findAutoDetectJSONInText searches for JSON arrays/objects in text
// that don't have a "tool" key but match known tool structures
func findAutoDetectJSONInText(text string) []ToolCall {
	var calls []ToolCall

	// Find all JSON arrays [{...}, ...]
	arrRe := regexp.MustCompile(`\[\s*\{[\s\S]*?\}\s*\]`)
	for _, match := range arrRe.FindAllString(text, -1) {
		var arr []map[string]interface{}
		if err := json.Unmarshal([]byte(match), &arr); err == nil {
			for _, obj := range arr {
				if call, ok := detectToolFromJSON(obj); ok {
					calls = append(calls, call)
				}
			}
		}
	}
	if len(calls) > 0 {
		return calls
	}

	// Find all JSON objects {...}
	objRe := regexp.MustCompile(`\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}`)
	for _, match := range objRe.FindAllString(text, -1) {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(match), &obj); err == nil {
			if call, ok := detectToolFromJSON(obj); ok {
				calls = append(calls, call)
			}
		}
	}

	return calls
}
