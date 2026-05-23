package mcp

import (
	"context"
	"testing"
	"time"
)

func TestMCPClientConnect_UnknownType(t *testing.T) {
	cfg := ClientConfig{
		Name:    "bad-type",
		Type:    "websocket",
		Enabled: true,
	}
	client := NewMCPClient(cfg)
	err := client.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if err.Error() != "MCP server 'bad-type': unknown type 'websocket'" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMCPClientConnect_SSE_NoURL(t *testing.T) {
	cfg := ClientConfig{
		Name:    "sse-no-url",
		Type:    "sse",
		Enabled: true,
	}
	client := NewMCPClient(cfg)
	err := client.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error for SSE without URL")
	}
	if err.Error() != "MCP server 'sse-no-url': URL required for SSE type" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMCPClientConnect_StreamableHTTP_NoURL(t *testing.T) {
	cfg := ClientConfig{
		Name:    "http-no-url",
		Type:    "streamable-http",
		Enabled: true,
	}
	client := NewMCPClient(cfg)
	err := client.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error for streamable-http without URL")
	}
	if err.Error() != "MCP server 'http-no-url': URL required for streamable-http type" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMCPClientConnect_SSE_InvalidURL(t *testing.T) {
	cfg := ClientConfig{
		Name:    "sse-bad-url",
		Type:    "sse",
		URL:     "://invalid",
		Enabled: true,
	}
	client := NewMCPClient(cfg)
	err := client.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid SSE URL")
	}
	// Должен содержать "failed to create SSE MCP client"
	if err.Error()[:30] != "failed to create SSE MCP clien" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMCPClientConnect_StreamableHTTP_InvalidURL(t *testing.T) {
	cfg := ClientConfig{
		Name:    "http-bad-url",
		Type:    "streamable-http",
		URL:     "://invalid",
		Enabled: true,
	}
	client := NewMCPClient(cfg)
	err := client.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid streamable-http URL")
	}
	if err.Error()[:30] != "failed to create streamable-ht" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMCPClientConnect_SSE_Unreachable(t *testing.T) {
	cfg := ClientConfig{
		Name:    "sse-unreachable",
		Type:    "sse",
		URL:     "http://127.0.0.1:1/mcp/sse",
		Headers: map[string]string{"Authorization": "Bearer test-token"},
		Enabled: true,
	}
	client := NewMCPClient(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	if err == nil {
		t.Fatal("expected error for unreachable SSE server")
	}
	// Должен содержать "failed to initialize"
	if err.Error()[:20] != "failed to initialize" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMCPClientConnect_StreamableHTTP_Unreachable(t *testing.T) {
	cfg := ClientConfig{
		Name:    "http-unreachable",
		Type:    "streamable-http",
		URL:     "http://127.0.0.1:1/mcp",
		Headers: map[string]string{"X-Custom": "value"},
		Enabled: true,
	}
	client := NewMCPClient(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	if err == nil {
		t.Fatal("expected error for unreachable streamable-http server")
	}
	if err.Error()[:20] != "failed to initialize" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMCPClientConnect_AlreadyConnected(t *testing.T) {
	cfg := ClientConfig{
		Name:    "stdio-echo",
		Type:    "stdio",
		Command: "echo",
		Args:    []string{"hello"},
		Enabled: true,
	}
	client := NewMCPClient(cfg)

	// Первое подключение может пройти или нет (зависит от echo),
	// но если подключились — повторный вызов должен вернуть nil
	// Мы используем хитрость: вручную ставим connected=true
	client.mu.Lock()
	client.connected = true
	client.mu.Unlock()

	err := client.Connect(context.Background())
	if err != nil {
		t.Errorf("expected nil for already connected, got: %v", err)
	}
}

func TestMCPClientDisconnect_NotConnected(t *testing.T) {
	cfg := ClientConfig{Name: "test"}
	client := NewMCPClient(cfg)

	err := client.Disconnect()
	if err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

func TestMCPClientCallTool_NotConnected(t *testing.T) {
	cfg := ClientConfig{Name: "test"}
	client := NewMCPClient(cfg)

	_, err := client.CallTool("my-tool", map[string]string{"key": "val"})
	if err == nil {
		t.Fatal("expected error for not connected client")
	}
	if err.Error() != "MCP server 'test' not connected" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMCPToolWrapper_NotConnected(t *testing.T) {
	wrapper := &MCPToolWrapper{
		serverName: "test",
		toolName:   "my-tool",
		desc:       "desc",
		client:     nil,
	}

	result := wrapper.Execute(map[string]string{"key": "val"})
	if result.Error != "MCP client not connected" {
		t.Errorf("expected 'MCP client not connected', got: %s", result.Error)
	}
}

func TestMCPToolWrapper_NilClient(t *testing.T) {
	wrapper := &MCPToolWrapper{
		serverName: "test",
		toolName:   "my-tool",
		desc:       "desc",
		client:     &MCPClient{},
	}

	result := wrapper.Execute(map[string]string{"key": "val"})
	if result.Error != "MCP client not connected" {
		t.Errorf("expected 'MCP client not connected', got: %s", result.Error)
	}
}

func TestMCPClientGetTools_Empty(t *testing.T) {
	client := NewMCPClient(ClientConfig{Name: "test"})
	tools := client.GetTools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestMCPClientGetServerInfo_NotConnected(t *testing.T) {
	client := NewMCPClient(ClientConfig{Name: "test"})
	info := client.GetServerInfo()
	if info != nil {
		t.Errorf("expected nil, got %+v", info)
	}
}

func TestManagerConnectAll_Disabled(t *testing.T) {
	mgr := NewManager()
	mgr.LoadFromConfig(map[string]ClientConfig{
		"disabled": {
			Name:    "disabled",
			Type:    "stdio",
			Command: "echo",
			Enabled: false,
		},
	})

	err := mgr.ConnectAll(context.Background())
	if err != nil {
		t.Errorf("expected nil for disabled server, got: %v", err)
	}

	servers := mgr.GetConnectedServers()
	if len(servers) != 0 {
		t.Errorf("expected 0 connected, got %d", len(servers))
	}
}

func TestManagerConnectAll_Mixed(t *testing.T) {
	mgr := NewManager()
	mgr.LoadFromConfig(map[string]ClientConfig{
		"disabled": {
			Name:    "disabled",
			Type:    "stdio",
			Command: "echo",
			Enabled: false,
		},
		"bad-type": {
			Name:    "bad-type",
			Type:    "unknown",
			Enabled: true,
		},
	})

	err := mgr.ConnectAll(context.Background())
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	// disabled не пытается подключаться, ошибка только от bad-type
	if err.Error()[:20] != "MCP connection error" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestManagerDisconnectAll(t *testing.T) {
	mgr := NewManager()
	mgr.LoadFromConfig(map[string]ClientConfig{})
	mgr.DisconnectAll()

	servers := mgr.GetConnectedServers()
	if len(servers) != 0 {
		t.Errorf("expected 0 after disconnect, got %d", len(servers))
	}
}

func TestManagerGetAllTools_Empty(t *testing.T) {
	mgr := NewManager()
	tools := mgr.GetAllTools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestClientConfigWithHeaders(t *testing.T) {
	cfg := ClientConfig{
		Name: "sse-with-headers",
		Type: "sse",
		URL:  "http://127.0.0.1:1/mcp/sse",
		Headers: map[string]string{
			"Authorization": "Bearer secret-token",
			"X-API-Key":     "key123",
		},
		Enabled: true,
	}

	client := NewMCPClient(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Подключение не удастся (нет сервера), но проверяем что конфигурация принята
	err := client.Connect(ctx)
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	// Проверяем что это ошибка инициализации, а не "not yet implemented"
	if err.Error()[:20] != "failed to initialize" {
		t.Errorf("expected init failure (not 'not implemented'), got: %v", err)
	}
}

func TestParseMCPConfigWithHeaders(t *testing.T) {
	jsonData := `{
		"remote": {
			"name": "remote",
			"type": "sse",
			"url": "https://mcp.example.com/sse",
			"headers": {
				"Authorization": "Bearer token123"
			},
			"enabled": true
		}
	}`

	configs, err := ParseMCPConfig([]byte(jsonData))
	if err != nil {
		t.Fatalf("ParseMCPConfig error: %v", err)
	}

	cfg, ok := configs["remote"]
	if !ok {
		t.Fatal("expected 'remote' config")
	}
	if cfg.Type != "sse" {
		t.Errorf("expected type sse, got %s", cfg.Type)
	}
	if cfg.Headers["Authorization"] != "Bearer token123" {
		t.Errorf("expected Authorization header, got %v", cfg.Headers)
	}
	if cfg.URL != "https://mcp.example.com/sse" {
		t.Errorf("expected URL, got %s", cfg.URL)
	}
}
