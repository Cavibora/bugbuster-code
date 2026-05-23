package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"bugbuster-code/pkg/logger"
	"bugbuster-code/pkg/tools"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ServeConfig — BugBuster MCP server configuration
type ServeConfig struct {
	Transport string // "stdio" (default), "sse", "streamable-http"
	Host      string // host for SSE/HTTP (default "localhost")
	Port      int    // port for SSE/HTTP (default 8080)
	Prefix    string // tool prefix (default "bugbuster_")
}

// BugBusterMCPServer — BugBuster MCP server.
// Allows other agents to use BugBuster tools via MCP.
type BugBusterMCPServer struct {
	config ServeConfig
	server *server.MCPServer
}

// NewBugBusterMCPServer creates BugBuster MCP server.
func NewBugBusterMCPServer(cfg ServeConfig) *BugBusterMCPServer {
	if cfg.Transport == "" {
		cfg.Transport = "stdio"
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "bugbuster_"
	}
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}

	mcpServer := server.NewMCPServer(
		"bugbuster-code",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	return &BugBusterMCPServer{
		config: cfg,
		server: mcpServer,
	}
}

// RegisterTools registers tools (tools.Tool → ServerTool with prefix).
func (s *BugBusterMCPServer) RegisterTools(toolsList []tools.Tool) {
	serverTools := AdaptTools(toolsList, s.config.Prefix)
	s.server.AddTools(serverTools...)
}

// RegisterHighLevelTool registers a high-level tool (scan/fix)
// with custom handler.
func (s *BugBusterMCPServer) RegisterHighLevelTool(name, description string, schema json.RawMessage, handler server.ToolHandlerFunc) {
	tool := mcpgo.NewToolWithRawSchema(s.config.Prefix+name, description, schema)
	s.server.AddTool(tool, handler)
}

// Serve starts server with selected transport.
func (s *BugBusterMCPServer) Serve(ctx context.Context) error {
	switch s.config.Transport {
	case "stdio":
		return s.serveStdio(ctx)
	case "sse":
		return s.serveSSE(ctx)
	case "streamable-http":
		return s.serveStreamableHTTP(ctx)
	default:
		return fmt.Errorf("unknown transport: %s", s.config.Transport)
	}
}

func (s *BugBusterMCPServer) serveStdio(ctx context.Context) error {
	return server.ServeStdio(s.server)
}

func (s *BugBusterMCPServer) serveSSE(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	sseServer := server.NewSSEServer(s.server)
	logger.Info("MCP server (SSE) listening", "addr", addr)
	return sseServer.Start(addr)
}

func (s *BugBusterMCPServer) serveStreamableHTTP(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	httpServer := server.NewStreamableHTTPServer(s.server)
	logger.Info("MCP server (Streamable HTTP) listening", "addr", addr)
	return httpServer.Start(addr)
}

// GetServerInfo returns server info (for debugging).
func (s *BugBusterMCPServer) GetServerInfo() (name, version, transport string) {
	return "bugbuster-code", "1.0.0", s.config.Transport
}
