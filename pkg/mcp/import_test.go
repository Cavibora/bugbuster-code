package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestImportFromContinue(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "continue.json")

	config := map[string]any{
		"mcpServers": map[string]any{
			"test-server": map[string]any{
				"command": "node",
				"args":    []any{"server.js"},
				"env": map[string]any{
					"API_KEY": "test123",
				},
			},
		},
	}
	data, _ := json.Marshal(config)
	os.WriteFile(configPath, data, 0644)

	configs, err := ImportFromContinue(configPath)
	if err != nil {
		t.Fatalf("ImportFromContinue() error: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("Expected 1 config, got %d", len(configs))
	}
	if configs[0].Name != "test-server" {
		t.Errorf("Expected name 'test-server', got '%s'", configs[0].Name)
	}
	if configs[0].Command != "node" {
		t.Errorf("Expected command 'node', got '%s'", configs[0].Command)
	}
	if configs[0].Type != "stdio" {
		t.Errorf("Expected type 'stdio', got '%s'", configs[0].Type)
	}
	if len(configs[0].Args) != 1 || configs[0].Args[0] != "server.js" {
		t.Errorf("Expected args ['server.js'], got %v", configs[0].Args)
	}
	if configs[0].Env["API_KEY"] != "test123" {
		t.Errorf("Expected env API_KEY=test123, got %v", configs[0].Env)
	}
}

func TestImportFromContinue_SSE(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "continue.json")

	config := map[string]any{
		"mcpServers": map[string]any{
			"remote-server": map[string]any{
				"url": "http://localhost:8080/mcp",
			},
		},
	}
	data, _ := json.Marshal(config)
	os.WriteFile(configPath, data, 0644)

	configs, err := ImportFromContinue(configPath)
	if err != nil {
		t.Fatalf("ImportFromContinue() error: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("Expected 1 config, got %d", len(configs))
	}
	if configs[0].Type != "sse" {
		t.Errorf("Expected type 'sse', got '%s'", configs[0].Type)
	}
	if configs[0].URL != "http://localhost:8080/mcp" {
		t.Errorf("Expected URL 'http://localhost:8080/mcp', got '%s'", configs[0].URL)
	}
}

func TestImportFromContinue_NoServers(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "continue.json")

	config := map[string]any{
		"other": "data",
	}
	data, _ := json.Marshal(config)
	os.WriteFile(configPath, data, 0644)

	configs, err := ImportFromContinue(configPath)
	if err != nil {
		t.Fatalf("ImportFromContinue() error: %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("Expected 0 configs, got %d", len(configs))
	}
}

func TestImportFromContinue_FileNotFound(t *testing.T) {
	_, err := ImportFromContinue("/nonexistent/path.json")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestImportFromContinue_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "continue.json")
	os.WriteFile(configPath, []byte("not json"), 0644)

	_, err := ImportFromContinue(configPath)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestImportFromAider(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".aider.conf.yml")
	os.WriteFile(configPath, []byte("model: gpt-4"), 0644)

	configs, err := ImportFromAider(configPath)
	if err != nil {
		t.Fatalf("ImportFromAider() error: %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("Expected 0 configs, got %d", len(configs))
	}
}

func TestAutoImport(t *testing.T) {
	dir := t.TempDir()

	mcpConfig := map[string]any{
		"mcpServers": map[string]any{
			"test-server": map[string]any{
				"command": "node",
				"args":    []any{"server.js"},
			},
		},
	}
	data, _ := json.Marshal(mcpConfig)
	os.WriteFile(filepath.Join(dir, ".mcp.json"), data, 0644)

	configs, err := AutoImport(dir)
	if err != nil {
		t.Fatalf("AutoImport() error: %v", err)
	}
	if len(configs) < 1 {
		t.Errorf("Expected at least 1 config, got %d", len(configs))
	}
}

func TestAutoImport_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	configs, err := AutoImport(dir)
	if err != nil {
		t.Fatalf("AutoImport() error: %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("Expected 0 configs, got %d", len(configs))
	}
}

func TestMCPToolWrapper_Interface(t *testing.T) {
	tool := &MCPToolWrapper{
		toolName: "test_tool",
		desc:     "A test tool",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{
					"type": "string",
				},
			},
		},
	}

	if tool.Name() != "test_tool" {
		t.Errorf("Expected name 'test_tool', got '%s'", tool.Name())
	}
	if tool.Description() != "A test tool" {
		t.Errorf("Expected description 'A test tool', got '%s'", tool.Description())
	}
	params := tool.Parameters()
	if params == nil {
		t.Error("Expected non-nil parameters")
	}
}

func TestGetServerInfo(t *testing.T) {
	server := NewBugBusterMCPServer(ServeConfig{Transport: "stdio"})

	name, version, transport := server.GetServerInfo()
	if name == "" {
		t.Error("Expected non-empty server name")
	}
	if version == "" {
		t.Error("Expected non-empty server version")
	}
	if transport != "stdio" {
		t.Errorf("Expected transport 'stdio', got '%s'", transport)
	}
}

func TestFindMCPServers(t *testing.T) {
	data := map[string]any{
		"mcpServers": map[string]any{
			"server1": map[string]any{
				"command": "test",
			},
		},
	}

	result := findMCPServers(data)
	if result == nil {
		t.Fatal("expected to find mcpServers")
	}
	if len(result) != 1 {
		t.Errorf("expected 1 server, got %d", len(result))
	}

	// Test nested
	data2 := map[string]any{
		"experimental": map[string]any{
			"mcpServers": map[string]any{
				"nested": map[string]any{
					"command": "nested-cmd",
				},
			},
		},
	}

	result2 := findMCPServers(data2)
	if result2 == nil {
		t.Fatal("expected to find nested mcpServers")
	}
}

func TestMCPClientConfigDefaults(t *testing.T) {
	cfg := ClientConfig{
		Name:    "test",
		Type:    "stdio",
		Command: "npx",
		Args:    []string{"-y", "test-server"},
		Enabled: true,
	}

	client := NewMCPClient(cfg)
	if client.IsConnected() {
		t.Error("expected client to be disconnected initially")
	}
	if client.config.Name != "test" {
		t.Errorf("expected name test, got %s", client.config.Name)
	}
}

func TestManagerBasic(t *testing.T) {
	mgr := NewManager()

	configs := map[string]ClientConfig{
		"test-server": {
			Name:    "test-server",
			Type:    "stdio",
			Command: "echo",
			Enabled: false,
		},
	}

	mgr.LoadFromConfig(configs)

	servers := mgr.GetConnectedServers()
	if len(servers) != 0 {
		t.Errorf("expected 0 connected servers, got %d", len(servers))
	}

	tools := mgr.GetAllTools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestParseMCPConfig(t *testing.T) {
	jsonData := `{
		"my-server": {
			"name": "my-server",
			"type": "stdio",
			"command": "npx",
			"args": ["-y", "server"],
			"enabled": true
		}
	}`

	configs, err := ParseMCPConfig(json.RawMessage(jsonData))
	if err != nil {
		t.Fatalf("ParseMCPConfig error: %v", err)
	}

	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}

	if configs["my-server"].Type != "stdio" {
		t.Errorf("expected type stdio, got %s", configs["my-server"].Type)
	}
}
