package tools

import (
	"testing"
)

func TestWebFetchTool_Name(t *testing.T) {
	tool := NewWebFetchTool()
	if tool.Name() != "web_fetch" {
		t.Errorf("Expected name 'web_fetch', got '%s'", tool.Name())
	}
}

func TestWebFetchTool_MissingURL(t *testing.T) {
	tool := NewWebFetchTool()
	result := tool.Execute(map[string]string{})
	if result.Error == "" {
		t.Error("Expected error for missing URL")
	}
}

func TestWebFetchTool_InvalidURL(t *testing.T) {
	tool := NewWebFetchTool()
	result := tool.Execute(map[string]string{"url": "ftp://example.com"})
	if result.Error == "" {
		t.Error("Expected error for invalid URL scheme")
	}
}

func TestWebFetchTool_NetworkBlocked(t *testing.T) {
	tool := NewWebFetchTool()
	tool.AllowNetwork = false
	result := tool.Execute(map[string]string{"url": "https://example.com"})
	if result.Error == "" {
		t.Error("Expected error when network is blocked")
	}
}

func TestWebFetchTool_Parameters(t *testing.T) {
	tool := NewWebFetchTool()
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("Expected type='object', got '%v'", params["type"])
	}
}

func TestAskUserTool_Name(t *testing.T) {
	tool := NewAskUserTool()
	if tool.Name() != "ask_user" {
		t.Errorf("Expected name 'ask_user', got '%s'", tool.Name())
	}
}

func TestAskUserTool_MissingQuestion(t *testing.T) {
	tool := NewAskUserTool()
	result := tool.Execute(map[string]string{})
	if result.Error == "" {
		t.Error("Expected error for missing question")
	}
}

func TestAskUserTool_Parameters(t *testing.T) {
	tool := NewAskUserTool()
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("Expected type='object', got '%v'", params["type"])
	}
}
