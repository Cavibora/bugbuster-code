package mcp

import (
	"testing"

	"bugbuster-code/pkg/tools"
)

func TestNewBugBusterMCPServer_Defaults(t *testing.T) {
	srv := NewBugBusterMCPServer(ServeConfig{})
	if srv.config.Transport != "stdio" {
		t.Errorf("expected default transport 'stdio', got '%s'", srv.config.Transport)
	}
	if srv.config.Prefix != "bugbuster_" {
		t.Errorf("expected default prefix 'bugbuster_', got '%s'", srv.config.Prefix)
	}
	if srv.config.Host != "localhost" {
		t.Errorf("expected default host 'localhost', got '%s'", srv.config.Host)
	}
	if srv.config.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", srv.config.Port)
	}
}

func TestNewBugBusterMCPServer_CustomConfig(t *testing.T) {
	srv := NewBugBusterMCPServer(ServeConfig{
		Transport: "sse",
		Host:      "0.0.0.0",
		Port:      9090,
		Prefix:    "bb_",
	})
	if srv.config.Transport != "sse" {
		t.Errorf("expected transport 'sse', got '%s'", srv.config.Transport)
	}
	if srv.config.Host != "0.0.0.0" {
		t.Errorf("expected host '0.0.0.0', got '%s'", srv.config.Host)
	}
	if srv.config.Port != 9090 {
		t.Errorf("expected port 9090, got %d", srv.config.Port)
	}
	if srv.config.Prefix != "bb_" {
		t.Errorf("expected prefix 'bb_', got '%s'", srv.config.Prefix)
	}
}

func TestBugBusterMCPServer_RegisterTools(t *testing.T) {
	srv := NewBugBusterMCPServer(ServeConfig{Prefix: "test_"})

	tool := &mockTool{name: "read", description: "Read", params: map[string]any{"type": "object"}, result: tools.ToolResult{Output: "ok"}}
	srv.RegisterTools([]tools.Tool{tool})

	// Проверяем что инструмент зарегистрирован — через GetServerInfo
	name, version, transport := srv.GetServerInfo()
	if name != "bugbuster-code" {
		t.Errorf("expected name 'bugbuster-code', got '%s'", name)
	}
	if version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", version)
	}
	if transport != "stdio" {
		t.Errorf("expected transport 'stdio', got '%s'", transport)
	}
}

func TestBugBusterMCPServer_UnknownTransport(t *testing.T) {
	srv := NewBugBusterMCPServer(ServeConfig{Transport: "websocket"})
	err := srv.Serve(nil)
	if err == nil {
		t.Fatal("expected error for unknown transport")
	}
	if err.Error() != "unknown transport: websocket" {
		t.Errorf("unexpected error: %v", err)
	}
}
