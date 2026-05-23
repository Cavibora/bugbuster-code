package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"bugbuster-code/pkg/tools"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ToolAdapter converts tools.Tool to server.ServerTool with name prefix.
// Prefix is added to tool name to avoid collisions
// (e.g. "read" → "bugbuster_read").
func ToolAdapter(tool tools.Tool, prefix string) server.ServerTool {
	name := prefix + tool.Name()
	description := tool.Description()

	// Marshal Parameters() to json.RawMessage for NewToolWithRawSchema
	schemaBytes, err := json.Marshal(tool.Parameters())
	if err != nil {
		schemaBytes = []byte(`{"type":"object"}`)
	}

	mcpTool := mcpgo.NewToolWithRawSchema(name, description, json.RawMessage(schemaBytes))

	handler := func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		// Convert Arguments (map[string]any) → map[string]string
		args := req.GetArguments()
		params := make(map[string]string, len(args))
		for k, v := range args {
			params[k] = fmt.Sprintf("%v", v)
		}

		result := tool.Execute(params)
		if result.Error != "" {
			return mcpgo.NewToolResultError(result.Error), nil
		}
		return mcpgo.NewToolResultText(result.Output), nil
	}

	return server.ServerTool{Tool: mcpTool, Handler: handler}
}

// AdaptTools converts a set of tools.Tool with prefix.
func AdaptTools(toolsList []tools.Tool, prefix string) []server.ServerTool {
	result := make([]server.ServerTool, 0, len(toolsList))
	for _, t := range toolsList {
		result = append(result, ToolAdapter(t, prefix))
	}
	return result
}
