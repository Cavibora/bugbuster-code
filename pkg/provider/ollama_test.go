package provider

import (
	"strings"
	"testing"
)

func TestOllamaNativeRequestIncludesTools(t *testing.T) {
	p := &OllamaProvider{
		model: "glm-5.1",
	}

	tools := []ToolDef{
		{
			Name:        "read",
			Description: "read a file",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
			},
		},
	}

	req := p.buildNativeRequest([]Message{}, tools, false)

	toolsField, ok := req["tools"]
	if !ok {
		t.Fatal("buildNativeRequest should include 'tools' field")
	}

	toolsList, ok := toolsField.([]map[string]any)
	if !ok {
		t.Fatalf("tools should be []map[string]any, got %T", toolsField)
	}

	if len(toolsList) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(toolsList))
	}

	tool := toolsList[0]
	fn, ok := tool["function"].(map[string]any)
	if !ok {
		t.Fatalf("tool should have 'function' field, got %T", tool["function"])
	}

	if fn["name"] != "read" {
		t.Errorf("tool name = %v, want 'read'", fn["name"])
	}
}

func TestOllamaNativeRequestNoTools(t *testing.T) {
	p := &OllamaProvider{
		model: "glm-5.1",
	}

	req := p.buildNativeRequest([]Message{}, nil, false)

	if _, ok := req["tools"]; ok {
		t.Error("buildNativeRequest should not include 'tools' when no tools provided")
	}
}

func TestOllamaNativeRequestMessages(t *testing.T) {
	p := &OllamaProvider{
		model: "glm-5.1",
	}

	msgs := []Message{
		UserMsg("hello"),
		ToolResultMsg("call_1", "read", "file content", false),
	}

	req := p.buildNativeRequest(msgs, nil, false)

	msgsField, ok := req["messages"]
	if !ok {
		t.Fatal("buildNativeRequest should include 'messages' field")
	}

	msgList, ok := msgsField.([]map[string]any)
	if !ok {
		t.Fatalf("messages should be []map[string]any, got %T", msgsField)
	}

	if len(msgList) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgList))
	}

	// First message should be user
	if msgList[0]["role"] != "user" {
		t.Errorf("first message role = %v, want 'user'", msgList[0]["role"])
	}

	// Second message should include tool result in content
	content, ok := msgList[1]["content"].(string)
	if !ok {
		t.Fatalf("second message content should be string, got %T", msgList[1]["content"])
	}
	if !strings.Contains(content, "file content") {
		t.Errorf("content should include tool result, got: %q", content)
	}
}
