package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"bugbuster-code/pkg/tools"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// ClientConfig is MCP server configuration
type ClientConfig struct {
	Name    string            `yaml:"name"`    // unique name
	Type    string            `yaml:"type"`    // "stdio", "sse", "streamable-http"
	Command string            `yaml:"command"` // command for stdio (e.g. "npx")
	Args    []string          `yaml:"args"`    // arguments commands
	URL     string            `yaml:"url"`     // URL for SSE/HTTP
	Env     map[string]string `yaml:"env"`     // environment variables
	Headers map[string]string `yaml:"headers"` // HTTP headers (Authorization etc.)
	Enabled bool              `yaml:"enabled"` // whether server is enabled
}

// MCPServerInfo — information about MCP server
type MCPServerInfo struct {
	Name        string
	Version     string
	Description string
	Tools       []MCPToolInfo
}

// MCPToolInfo — information about MCP server tool
type MCPToolInfo struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema
}

// MCPToolWrapper — MCP tool wrapper for tools.Tool interface
type MCPToolWrapper struct {
	serverName string
	toolName   string
	desc       string
	params     map[string]any
	client     *MCPClient
}

func (t *MCPToolWrapper) Name() string               { return t.toolName }
func (t *MCPToolWrapper) Description() string        { return t.desc }
func (t *MCPToolWrapper) Parameters() map[string]any { return t.params }

func (t *MCPToolWrapper) Execute(params map[string]string) tools.ToolResult {
	if t.client == nil || !t.client.IsConnected() {
		return tools.ToolResult{Error: "MCP client not connected"}
	}

	result, err := t.client.CallTool(t.toolName, params)
	if err != nil {
		return tools.ToolResult{Error: fmt.Sprintf("MCP error: %v", err)}
	}

	return tools.ToolResult{Output: result}
}

// MCPClient — client for interacting with MCP server
type MCPClient struct {
	config     ClientConfig
	mu         sync.Mutex
	connected  bool
	serverInfo *MCPServerInfo
	tools      []*MCPToolWrapper
	mcpClient  *mcpclient.Client
}

// NewMCPClient creates a new MCP-client
func NewMCPClient(config ClientConfig) *MCPClient {
	return &MCPClient{
		config: config,
	}
}

// Connect connects to MCP server and receives tool list
func (c *MCPClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	var client *mcpclient.Client
	var err error

	switch c.config.Type {
	case "stdio":
		// Prepare environment variables
		env := make([]string, 0, len(c.config.Env))
		for k, v := range c.config.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}

		client, err = mcpclient.NewStdioMCPClient(
			c.config.Command,
			env,
			c.config.Args...,
		)
		if err != nil {
			return fmt.Errorf("failed to create stdio MCP client for '%s': %w", c.config.Name, err)
		}

	case "sse":
		if c.config.URL == "" {
			return fmt.Errorf("MCP server '%s': URL required for SSE type", c.config.Name)
		}

		var opts []transport.ClientOption
		if len(c.config.Headers) > 0 {
			opts = append(opts, transport.WithHeaders(c.config.Headers))
		}
		client, err = mcpclient.NewSSEMCPClient(c.config.URL, opts...)
		if err != nil {
			return fmt.Errorf("failed to create SSE MCP client for '%s': %w", c.config.Name, err)
		}

	case "streamable-http":
		if c.config.URL == "" {
			return fmt.Errorf("MCP server '%s': URL required for streamable-http type", c.config.Name)
		}

		var opts []transport.StreamableHTTPCOption
		if len(c.config.Headers) > 0 {
			opts = append(opts, transport.WithHTTPHeaders(c.config.Headers))
		}
		client, err = mcpclient.NewStreamableHttpClient(c.config.URL, opts...)
		if err != nil {
			return fmt.Errorf("failed to create streamable-http MCP client for '%s': %w", c.config.Name, err)
		}

	default:
		return fmt.Errorf("MCP server '%s': unknown type '%s'", c.config.Name, c.config.Type)
	}

	// Initialize connection
	initRequest := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "bugbuster-code",
				Version: "1.0.0",
			},
		},
	}

	_, err = client.Initialize(ctx, initRequest)
	if err != nil {
		// Close client on initialization error
		_ = client.Close()
		return fmt.Errorf("failed to initialize MCP client for '%s': %w", c.config.Name, err)
	}

	// Get tool list
	toolsResult, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		// Server may not support tools — this is not an error
		toolsResult = &mcp.ListToolsResult{}
	}

	// Create wrappers for tools
	var wrappers []*MCPToolWrapper
	for _, tool := range toolsResult.Tools {
		// Convert ToolInputSchema to map[string]any
		var params map[string]any
		if tool.InputSchema.Type != "" || tool.InputSchema.Properties != nil {
			params = map[string]any{
				"type":       tool.InputSchema.Type,
				"properties": tool.InputSchema.Properties,
			}
			if len(tool.InputSchema.Required) > 0 {
				params["required"] = tool.InputSchema.Required
			}
		}

		wrapper := &MCPToolWrapper{
			serverName: c.config.Name,
			toolName:   tool.Name,
			desc:       tool.Description,
			params:     params,
			client:     c,
		}
		wrappers = append(wrappers, wrapper)
	}

	c.mcpClient = client
	c.tools = wrappers
	c.connected = true

	// Fill server information
	c.serverInfo = &MCPServerInfo{
		Name:  c.config.Name,
		Tools: make([]MCPToolInfo, len(wrappers)),
	}
	for i, w := range wrappers {
		c.serverInfo.Tools[i] = MCPToolInfo{
			Name:        w.toolName,
			Description: w.desc,
			Parameters:  w.params,
		}
	}

	return nil
}

// Disconnect disconnects from MCP server
func (c *MCPClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.mcpClient != nil {
		_ = c.mcpClient.Close()
	}
	c.connected = false
	c.tools = nil
	c.mcpClient = nil
	return nil
}

// GetTools returns MCP server tool list as tools.Tool
func (c *MCPClient) GetTools() []tools.Tool {
	c.mu.Lock()
	defer c.mu.Unlock()

	var result []tools.Tool
	for _, t := range c.tools {
		result = append(result, t)
	}
	return result
}

// CallTool calls tool on MCP server
func (c *MCPClient) CallTool(name string, params map[string]string) (string, error) {
	c.mu.Lock()
	client := c.mcpClient
	c.mu.Unlock()

	if client == nil {
		return "", fmt.Errorf("MCP server '%s' not connected", c.config.Name)
	}

	// Convert map[string]string to map[string]any for mcp-go
	args := make(map[string]any, len(params))
	for k, v := range params {
		args[k] = v
	}

	// 5 minute timeout for MCP call
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	})
	if err != nil {
		return "", fmt.Errorf("MCP call '%s' failed: %w", name, err)
	}

	// Collect text result
	var output strings.Builder
	for _, content := range result.Content {
		if text, ok := content.(mcp.TextContent); ok {
			output.WriteString(text.Text)
		}
	}

	return output.String(), nil
}

// IsConnected returns connection status
func (c *MCPClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// GetServerInfo returns server information
func (c *MCPClient) GetServerInfo() *MCPServerInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.serverInfo
}

// Manager — MCP server manager
type Manager struct {
	mu      sync.RWMutex
	clients map[string]*MCPClient
	configs map[string]ClientConfig
}

// NewManager creates a new MCP server manager
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*MCPClient),
		configs: make(map[string]ClientConfig),
	}
}

// LoadFromConfig loads MCP servers from configuration
func (m *Manager) LoadFromConfig(configs map[string]ClientConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, cfg := range configs {
		m.configs[name] = cfg
	}
}

// ConnectAll connects to all enabled MCP servers
func (m *Manager) ConnectAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []string
	for name, cfg := range m.configs {
		if !cfg.Enabled {
			continue
		}
		client := NewMCPClient(cfg)
		if err := client.Connect(ctx); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		m.clients[name] = client
	}

	if len(errs) > 0 {
		return fmt.Errorf("MCP connection errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// GetAllTools returns tools from all connected MCP servers
func (m *Manager) GetAllTools() []tools.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allTools []tools.Tool
	for _, client := range m.clients {
		allTools = append(allTools, client.GetTools()...)
	}
	return allTools
}

// DisconnectAll disconnects from all MCP servers
func (m *Manager) DisconnectAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, client := range m.clients {
		_ = client.Disconnect()
	}
	m.clients = make(map[string]*MCPClient)
}

// GetConnectedServers returns names of connected servers
func (m *Manager) GetConnectedServers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var names []string
	for name := range m.clients {
		names = append(names, name)
	}
	return names
}

// ParseMCPConfig parses MCP configuration from JSON
func ParseMCPConfig(data json.RawMessage) (map[string]ClientConfig, error) {
	var configs map[string]ClientConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("invalid MCP config: %w", err)
	}
	return configs, nil
}
