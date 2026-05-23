package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestImportFromCline_SimpleFormat(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp.json")

	config := `{
		"filesystem": {
			"command": "npx",
			"args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
			"type": "stdio"
		},
		"remote": {
			"url": "http://localhost:3000/sse",
			"type": "sse"
		}
	}`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	configs, err := ImportFromCline(configPath)
	if err != nil {
		t.Fatalf("ImportFromCline: %v", err)
	}
	if len(configs) != 2 {
		t.Errorf("expected 2 configs, got %d", len(configs))
	}

	var foundFS, foundRemote bool
	for _, cfg := range configs {
		if cfg.Name == "filesystem" {
			foundFS = true
			if cfg.Type != "stdio" {
				t.Errorf("filesystem type = %q, want stdio", cfg.Type)
			}
			if cfg.Command != "npx" {
				t.Errorf("filesystem command = %q, want npx", cfg.Command)
			}
		}
		if cfg.Name == "remote" {
			foundRemote = true
			if cfg.Type != "sse" {
				t.Errorf("remote type = %q, want sse", cfg.Type)
			}
		}
	}
	if !foundFS {
		t.Error("filesystem config not found")
	}
	if !foundRemote {
		t.Error("remote config not found")
	}
}

func TestImportFromCline_DisabledServer(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp.json")

	config := `{
		"disabled-server": {
			"command": "echo",
			"disabled": true
		}
	}`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	configs, err := ImportFromCline(configPath)
	if err != nil {
		t.Fatalf("ImportFromCline: %v", err)
	}
	if len(configs) != 1 {
		t.Errorf("expected 1 config, got %d", len(configs))
	}
	if configs[0].Enabled {
		t.Error("disabled server should have Enabled=false")
	}
}

func TestImportFromCline_VSCodeFormat(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "settings.json")

	// VS Code format with mcpServers key
	config := `{
		"something.mcpServers": {
			"my-server": {
				"command": "node",
				"args": ["server.js"]
			}
		}
	}`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	configs, err := ImportFromCline(configPath)
	if err != nil {
		t.Fatalf("ImportFromCline: %v", err)
	}
	// ImportFromCline should parse VS Code format
	if len(configs) < 1 {
		t.Errorf("expected at least 1 config, got %d", len(configs))
	}
}

func TestImportFromCline_FileNotFound(t *testing.T) {
	_, err := ImportFromCline("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestImportFromCline_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp.json")

	if err := os.WriteFile(configPath, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := ImportFromCline(configPath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestRegisterHighLevelTool(t *testing.T) {
	server := NewBugBusterMCPServer(ServeConfig{Transport: "stdio"})

	server.RegisterHighLevelTool("my-tool", "My tool description", json.RawMessage(`{"type":"object"}`), nil)
	// No panic means success
}

func TestCallTool_NotConnected(t *testing.T) {
	client := NewMCPClient(ClientConfig{Name: "test-server", Type: "stdio", Command: "echo"})

	_, err := client.CallTool("some-tool", map[string]string{})
	if err == nil {
		t.Error("expected error for disconnected client")
	}
}

func TestMCPToolWrapper_Execute_NotConnected(t *testing.T) {
	tool := &MCPToolWrapper{
		serverName: "nonexistent",
		toolName:   "test-tool",
		desc:       "test",
		params:     nil,
		client:     nil,
	}

	result := tool.Execute(map[string]string{})
	if result.Error == "" {
		t.Error("expected error for disconnected tool")
	}
}
