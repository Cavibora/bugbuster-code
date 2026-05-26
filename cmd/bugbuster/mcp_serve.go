package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"bugbuster-code/pkg/config"
	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/mcp"
	"bugbuster-code/pkg/theme"
	"bugbuster-code/pkg/tools"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"
)

var (
	mcpServeTransport string // --transport: stdio, sse, streamable-http
	mcpServeHost      string // --host
	mcpServePort      int    // --port
)

// mcpServeCmd creates subcommand mcp-serve
func mcpServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp-serve",
		Short: i18n.T("cli_subcommands.mcp_serve_short"),
		Long:  i18n.T("cli_subcommands.mcp_serve_long"),
		Run:   runMCPServe,
	}
	cmd.Flags().StringVar(&mcpServeTransport, "transport", "stdio", i18n.T("cli_flag.mcp_serve_transport"))
	cmd.Flags().StringVar(&mcpServeHost, "host", "localhost", i18n.T("cli_flag.mcp_serve_host"))
	cmd.Flags().IntVar(&mcpServePort, "port", 8080, i18n.T("cli_flag.mcp_serve_port"))
	return cmd
}

// runMCPServe starts BugBuster as MCP server
func runMCPServe(cmd *cobra.Command, args []string) {
	cfg := loadConfig()

	// Initialize i18n
	lang := langFlag
	if lang == "" {
		lang = cfg.Agent.Language
	}
	if lang == "" {
		lang = "en"
	}
	i18n.Init(lang)

	// Initialize theme
	appTheme = theme.ResolveTheme(cfg.Theme)

	// Create tools for MCP server
	toolsList := createMCPServerTools(cfg)

	// Create MCP server
	mcpServer := mcp.NewBugBusterMCPServer(mcp.ServeConfig{
		Transport: mcpServeTransport,
		Host:      mcpServeHost,
		Port:      mcpServePort,
	})
	mcpServer.RegisterTools(toolsList)

	// Register high-level tools (scan/fix)
	registerHighLevelTools(mcpServer, cfg)

	// Start
	ctx := context.Background()
	if err := mcpServer.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}

// createMCPServerTools creates tools for MCP server.
// Only tools suitable for external agents.
func createMCPServerTools(cfg *config.BugBusterConfig) []tools.Tool {
	provCfg := cfg.Providers[cfg.DefaultProvider]
	effectiveSecurity := cfg.EffectiveSecurity(&provCfg)

	readTool := tools.NewReadTool()
	readTool.AllowedDirs = cfg.Tools.AllowedDirs
	readTool.MaxSize = cfg.Tools.MaxFileSize

	writeTool := tools.NewWriteTool()
	writeTool.AllowedDirs = cfg.Tools.AllowedDirs

	editTool := tools.NewEditTool()
	editTool.AllowedDirs = cfg.Tools.AllowedDirs

	bashTool := tools.NewBashTool()
	bashTool.AllowedDirs = cfg.Tools.AllowedDirs
	bashTool.BlockedCommands = effectiveSecurity.BlockedCommands
	bashTool.AllowNetwork = effectiveSecurity.AllowNetwork
	if cfg.Tools.BashTimeout > 0 {
		bashTool.Timeout = time.Duration(cfg.Tools.BashTimeout) * time.Second
	}
	if projectDir != "" {
		bashTool.DefaultDir = projectDir
	}

	grepTool := tools.NewGrepTool()
	grepTool.AllowedDirs = cfg.Tools.AllowedDirs
	grepTool.MaxResults = cfg.Tools.MaxGrepResults

	globTool := tools.NewGlobTool()
	globTool.AllowedDirs = cfg.Tools.AllowedDirs
	globTool.MaxResults = cfg.Tools.MaxGlobResults

	// Only tools suitable for external agents
	return []tools.Tool{
		readTool,
		writeTool,
		editTool,
		bashTool,
		grepTool,
		globTool,
	}
}

// registerHighLevelTools registers high-level tools (scan/fix).
// These tools start agent inside handler.
func registerHighLevelTools(mcpServer *mcp.BugBusterMCPServer, cfg *config.BugBusterConfig) {
	// bugbuster_scan — project bug scanning
	scanSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Path to scan (default: current directory)"
			},
			"focus": {
				"type": "string",
				"description": "Focus area: security, performance, bugs, all"
			}
		}
	}`)

	mcpServer.RegisterHighLevelTool("scan", "Scan project for bugs and issues", scanSchema,
		func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			path := req.GetString("path", ".")
			focus := req.GetString("focus", "all")
			prompt := fmt.Sprintf("Scan the project at %s for issues. Focus: %s. List all bugs found with file paths and descriptions.", path, focus)
			result, err := runAgentForMCP(ctx, cfg, prompt)
			if err != nil {
				return mcpgo.NewToolResultError(fmt.Sprintf("scan failed: %v", err)), nil
			}
			return mcpgo.NewToolResultText(result), nil
		},
	)

	// bugbuster_fix — auto bug fixing
	fixSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"description": {
				"type": "string",
				"description": "Description of the bug to fix"
			},
			"path": {
				"type": "string",
				"description": "Path to the file or directory to fix"
			}
		},
		"required": ["description"]
	}`)

	mcpServer.RegisterHighLevelTool("fix", "Automatically fix bugs in the project", fixSchema,
		func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			description := req.GetString("description", "")
			path := req.GetString("path", ".")
			if description == "" {
				return mcpgo.NewToolResultError("description is required"), nil
			}
			prompt := fmt.Sprintf("Fix the following issue in %s: %s", path, description)
			result, err := runAgentForMCP(ctx, cfg, prompt)
			if err != nil {
				return mcpgo.NewToolResultError(fmt.Sprintf("fix failed: %v", err)), nil
			}
			return mcpgo.NewToolResultText(result), nil
		},
	)
}

// runAgentForMCP starts agent for high-level MCP tools.
func runAgentForMCP(ctx context.Context, cfg *config.BugBusterConfig, prompt string) (string, error) {
	p, err := createProvider(cfg)
	if err != nil {
		return "", fmt.Errorf("provider error: %w", err)
	}

	loop := createAgentLoop(cfg, p, nil, "")
	loop.SetNonInteractive(true)

	if askUser, ok := loop.Tools["ask_user"].(*tools.AskUserTool); ok {
		askUser.NonInteractive = true
	}

	eventCh, err := loop.StreamWithCancel(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("agent error: %w", err)
	}

	var result string
	for event := range eventCh {
		switch event.Type {
		case "text_delta":
			result += event.Text
		case "done":
			return result, nil
		case "error":
			return result, fmt.Errorf("%s", event.Error)
		}
	}

	return result, nil
}