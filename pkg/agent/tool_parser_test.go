package agent

import (
	"testing"
)

func TestParseXMLToolCalls(t *testing.T) {
	response := `<tool name="read"><path>main.go</path></tool>`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "read" {
		t.Errorf("expected name read, got %s", calls[0].Name)
	}
	if calls[0].Params["path"] != "main.go" {
		t.Errorf("expected path=main.go, got %v", calls[0].Params)
	}
}

func TestParseXMLToolCallsParamFormat(t *testing.T) {
	// Формат <param>key</param><param>value</param> — чередующиеся пары
	response := `<tool name="read"><param>path</param><param>/Users/ss/project/main.go</param></tool>`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "read" {
		t.Errorf("expected name read, got %s", calls[0].Name)
	}
	if calls[0].Params["path"] != "/Users/ss/project/main.go" {
		t.Errorf("expected path=/Users/ss/project/main.go, got %v", calls[0].Params)
	}
}

func TestParseXMLToolCallsMultipleParams(t *testing.T) {
	// Несколько пар param
	response := `<tool name="edit"><param>path</param><param>main.go</param><param>old</param><param>func main()</param><param>new</param><param>func main() { fmt.Println("hello") }</param></tool>`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "edit" {
		t.Errorf("expected name edit, got %s", calls[0].Name)
	}
	if calls[0].Params["path"] != "main.go" {
		t.Errorf("expected path=main.go, got %v", calls[0].Params)
	}
	if calls[0].Params["old"] != "func main()" {
		t.Errorf("expected old=func main(), got %v", calls[0].Params)
	}
}

func TestParseXMLToolCallsBash(t *testing.T) {
	response := `<tool name="bash"><param>command</param><param>ls -la /tmp</param></tool>`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Errorf("expected name bash, got %s", calls[0].Name)
	}
	if calls[0].Params["command"] != "ls -la /tmp" {
		t.Errorf("expected command=ls -la /tmp, got %v", calls[0].Params)
	}
}

func TestParseJSONToolCalls(t *testing.T) {
	response := `{"tool": "bash", "params": {"command": "ls -la"}}`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Errorf("expected name bash, got %s", calls[0].Name)
	}
	if calls[0].Params["command"] != "ls -la" {
		t.Errorf("expected command=ls -la, got %v", calls[0].Params)
	}
}

func TestParseToolCallsNoCalls(t *testing.T) {
	response := "Just a regular text response without any tool calls."
	calls := ParseToolCalls(response)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestStripToolCalls(t *testing.T) {
	response := "Some text\n<tool name=\"read\"><path>main.go</path></tool>\nMore text"
	result := StripToolCalls(response)
	if containsStr(result, "<tool") {
		t.Errorf("expected tool calls to be stripped, got: %s", result)
	}
	if !containsStr(result, "Some text") {
		t.Errorf("expected text to remain, got: %s", result)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
func TestParseJSONToolCallsMarkdown(t *testing.T) {
	// JSON внутри markdown-блока
	response := "I'll read the file:\n```json\n{\"tool\": \"read\", \"params\": {\"path\": \"/Users/ss/main.go\"}}\n```\nDone."
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "read" {
		t.Errorf("expected name read, got %s", calls[0].Name)
	}
	if calls[0].Params["path"] != "/Users/ss/main.go" {
		t.Errorf("expected path=/Users/ss/main.go, got %v", calls[0].Params)
	}
}

func TestParseJSONToolCallsBoxDrawing(t *testing.T) {
	// JSON с box-drawing символами (артефакт рендеринга)
	response := "┌json\n│ {\n│   \"tool\": \"read\",\n│   \"params\": {\"path\": \"/tmp/test.go\"}\n│ }\n└"
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "read" {
		t.Errorf("expected name read, got %s", calls[0].Name)
	}
	if calls[0].Params["path"] != "/tmp/test.go" {
		t.Errorf("expected path=/tmp/test.go, got %v", calls[0].Params)
	}
}

func TestParseJSONToolCallsMarkdownWithBoxDrawing(t *testing.T) {
	// Markdown-блок с box-drawing символами
	response := "```json\n│ {\n│   \"tool\": \"bash\",\n│   \"params\": {\"command\": \"ls -la\"}\n│ }\n```"
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Errorf("expected name bash, got %s", calls[0].Name)
	}
	if calls[0].Params["command"] != "ls -la" {
		t.Errorf("expected command=ls -la, got %v", calls[0].Params)
	}
}

func TestStripToolCallsMarkdown(t *testing.T) {
	response := "Some text\n```json\n{\"tool\": \"read\", \"params\": {\"path\": \"main.go\"}}\n```\nMore text"
	result := StripToolCalls(response)
	if containsStr(result, "```json") {
		t.Errorf("expected markdown block to be stripped, got: %s", result)
	}
	if !containsStr(result, "Some text") {
		t.Errorf("expected text to remain, got: %s", result)
	}
}

func TestCleanJSON(t *testing.T) {
	input := "│ {\n│   \"tool\": \"read\"\n│ }"
	result := cleanJSON(input)
	if containsStr(result, "│") {
		t.Errorf("expected box-drawing chars to be removed, got: %s", result)
	}
}

func TestParseXMLParamsParamNameAttr(t *testing.T) {
	// Формат <param name="path">value</param> — атрибут name
	response := `<tool name="read"><param name="path">/Users/ss/project/main.go</param></tool>`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "read" {
		t.Errorf("expected name read, got %s", calls[0].Name)
	}
	if calls[0].Params["path"] != "/Users/ss/project/main.go" {
		t.Errorf("expected path=/Users/ss/project/main.go, got %v", calls[0].Params)
	}
}

func TestParseXMLParamsParamNameAttrMultiple(t *testing.T) {
	// Несколько <param name="...">value</param>
	response := `<tool name="edit"><param name="path">main.go</param><param name="old">func main()</param><param name="new">func main() { fmt.Println("hello") }</param></tool>`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "edit" {
		t.Errorf("expected name edit, got %s", calls[0].Name)
	}
	if calls[0].Params["path"] != "main.go" {
		t.Errorf("expected path=main.go, got %v", calls[0].Params)
	}
	if calls[0].Params["old"] != "func main()" {
		t.Errorf("expected old=func main(), got %v", calls[0].Params)
	}
	if calls[0].Params["new"] != "func main() { fmt.Println(\"hello\") }" {
		t.Errorf("expected new=func main()..., got %v", calls[0].Params)
	}
}

func TestParseXMLParamsParamNameAttrSingleQuotes(t *testing.T) {
	// Атрибут name с одинарными кавычками
	response := `<tool name="read"><param name='path'>/tmp/test.go</param></tool>`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Params["path"] != "/tmp/test.go" {
		t.Errorf("expected path=/tmp/test.go, got %v", calls[0].Params)
	}
}

func TestParseXMLParamsMixedNamedAndParam(t *testing.T) {
	// Смешанный формат: именованные теги + <param name="...">
	response := `<tool name="bash"><param name="command">ls -la</param><param name="timeout">30</param></tool>`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Params["command"] != "ls -la" {
		t.Errorf("expected command=ls -la, got %v", calls[0].Params)
	}
	if calls[0].Params["timeout"] != "30" {
		t.Errorf("expected timeout=30, got %v", calls[0].Params)
	}
}

func TestParseXMLParamsSingleParamOdd(t *testing.T) {
	// Одиночный <param>value</param> — нечётное количество, ключ = "param"
	response := `<tool name="read"><param>/Users/ss/project</param></tool>`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	// Ключ будет "param" — инструмент вернёт ошибку "параметр 'path' обязателен"
	if calls[0].Params["param"] != "/Users/ss/project" {
		t.Errorf("expected param=/Users/ss/project, got %v", calls[0].Params)
	}
}

func TestParseOpenTag(t *testing.T) {
	tests := []struct {
		input    string
		tagName  string
		attrName string
	}{
		{"param", "param", ""},
		{"path", "path", ""},
		{`param name="path"`, "param", "path"},
		{`param name='path'`, "param", "path"},
		{`param name="command"`, "param", "command"},
	}

	for _, tt := range tests {
		tagName, attrName := parseOpenTag(tt.input)
		if tagName != tt.tagName {
			t.Errorf("parseOpenTag(%q): tagName = %q, want %q", tt.input, tagName, tt.tagName)
		}
		if attrName != tt.attrName {
			t.Errorf("parseOpenTag(%q): attrName = %q, want %q", tt.input, attrName, tt.attrName)
		}
	}
}

func TestAutoDetectTodoWrite(t *testing.T) {
	// Model outputs: output:[{"id": "1", "status": "in_progress", "subject": "Reading files"}]
	response := `output:[{"id": "1", "status": "in_progress", "subject": "Читаю текущий план и todo."}]`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d: %+v", len(calls), calls)
	}
	if calls[0].Name != "todo_write" {
		t.Errorf("expected name todo_write, got %s", calls[0].Name)
	}
}

func TestAutoDetectBash(t *testing.T) {
	response := `{"command": "ls -la"}`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Errorf("expected name bash, got %s", calls[0].Name)
	}
	if calls[0].Params["command"] != "ls -la" {
		t.Errorf("expected command=ls -la, got %v", calls[0].Params)
	}
}

func TestAutoDetectRead(t *testing.T) {
	response := `{"path": "main.go"}`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "read" {
		t.Errorf("expected name read, got %s", calls[0].Name)
	}
	if calls[0].Params["path"] != "main.go" {
		t.Errorf("expected path=main.go, got %v", calls[0].Params)
	}
}

func TestAutoDetectWrite(t *testing.T) {
	response := `{"path": "main.go", "content": "package main\nfunc main() {}"}`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "write" {
		t.Errorf("expected name write, got %s", calls[0].Name)
	}
}

func TestAutoDetectEdit(t *testing.T) {
	response := `{"path": "main.go", "old": "hello", "new": "world"}`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "edit" {
		t.Errorf("expected name edit, got %s", calls[0].Name)
	}
}

func TestAutoDetectGrep(t *testing.T) {
	response := `{"pattern": "TODO", "path": "src/"}`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "grep" {
		t.Errorf("expected name grep, got %s", calls[0].Name)
	}
}

func TestAutoDetectWebFetch(t *testing.T) {
	response := `{"url": "https://example.com"}`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "web_fetch" {
		t.Errorf("expected name web_fetch, got %s", calls[0].Name)
	}
}

func TestAutoDetectWithPrefix(t *testing.T) {
	response := `result:{"command": "go test ./..."}`
	calls := ParseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Errorf("expected name bash, got %s", calls[0].Name)
	}
}

func TestAutoDetectTodoArray(t *testing.T) {
	response := `[{"id": "1", "subject": "Read files", "status": "in_progress"}, {"id": "2", "subject": "Write code", "status": "pending"}]`
	calls := ParseToolCalls(response)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Name != "todo_write" {
		t.Errorf("expected name todo_write, got %s", calls[0].Name)
	}
	if calls[1].Name != "todo_write" {
		t.Errorf("expected name todo_write, got %s", calls[1].Name)
	}
}

func TestAutoDetectRealExample(t *testing.T) {
	// Real example from user: model outputs raw JSON without "tool" key
	input := `output:[{"id": "1", "status": "in_progress", "subject": "Читаю текущий план и todo."}]`
	calls := ParseToolCalls(input)
	if len(calls) == 0 {
		t.Fatalf("Expected tool call from: %s", input)
	}
	if calls[0].Name != "todo_write" {
		t.Errorf("Expected todo_write, got %s", calls[0].Name)
	}
	t.Logf("✅ Parsed: tool=%s, params=%v", calls[0].Name, calls[0].Params)
}

func TestParseAngleBracketKVToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []ToolCall
	}{
		{
			name:  "tool_name=bash with parameter=command",
			input: `<tool_name=bash> <parameter=command> ls /Users/ss/ai-test/`,
			expected: []ToolCall{
				{Name: "bash", Params: map[string]string{"command": "ls /Users/ss/ai-test/"}},
			},
		},
		{
			name:  "tool_name=read with parameter=path",
			input: `<tool_name=read> <parameter=path> /tmp/main.go`,
			expected: []ToolCall{
				{Name: "read", Params: map[string]string{"path": "/tmp/main.go"}},
			},
		},
		{
			name:  "tool_name=bash with closing tags",
			input: `<tool_name=bash><parameter=command>ls -la /tmp/</parameter></tool_name>`,
			expected: []ToolCall{
				{Name: "bash", Params: map[string]string{"command": "ls -la /tmp/"}},
			},
		},
		{
			name:  "tool_name=write with multiple parameters",
			input: `<tool_name=write> <parameter=path> /tmp/test.go <parameter=content> package main`,
			expected: []ToolCall{
				{Name: "write", Params: map[string]string{"path": "/tmp/test.go", "content": "package main"}},
			},
		},
		{
			name:  "tool_name with quotes",
			input: `<tool_name="bash"> <parameter="command"> ls -la`,
			expected: []ToolCall{
				{Name: "bash", Params: map[string]string{"command": "ls -la"}},
			},
		},
		{
			name:  "Bash tag with command attribute",
			input: `<bash command="ls -la /tmp/">run</bash>`,
			expected: []ToolCall{
				{Name: "bash", Params: map[string]string{"command": "ls -la /tmp/"}},
			},
		},
		{
			name:  "Read tag with path attribute",
			input: `<read path="/tmp/main.go">read file</read>`,
			expected: []ToolCall{
				{Name: "read", Params: map[string]string{"path": "/tmp/main.go"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseToolCalls(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d calls, got %d", len(tt.expected), len(result))
				t.Logf("input: %s", tt.input)
				for _, call := range result {
					t.Logf("  got: %s %v", call.Name, call.Params)
				}
				return
			}
			for i, call := range result {
				if call.Name != tt.expected[i].Name {
					t.Errorf("expected name %q, got %q", tt.expected[i].Name, call.Name)
				}
				for k, v := range tt.expected[i].Params {
					if got, ok := call.Params[k]; !ok || got != v {
						t.Errorf("expected param %q=%q, got %q=%q", k, v, k, got)
					}
				}
			}
		})
	}
}
