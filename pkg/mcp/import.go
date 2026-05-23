package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ImportFromClaudeCode imports MCP servers from .mcp.json (Claude Code format)
// Format: { "mcpServers": { "name": { "command": "...", "args": [...], "env": {...} } } }
func ImportFromClaudeCode(path string) ([]ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read .mcp.json: %w", err)
	}

	var raw struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			URL     string            `json:"url"`
			Type    string            `json:"type"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse .mcp.json: %w", err)
	}

	var configs []ClientConfig
	for name, server := range raw.MCPServers {
		serverType := "stdio"
		if server.Type != "" {
			serverType = server.Type
		} else if server.URL != "" {
			serverType = "sse"
		}

		configs = append(configs, ClientConfig{
			Name:    name,
			Type:    serverType,
			Command: server.Command,
			Args:    server.Args,
			URL:     server.URL,
			Env:     server.Env,
			Headers: server.Headers,
			Enabled: true,
		})
	}

	return configs, nil
}

// ImportFromCline imports MCP servers from VS Code settings (Cline format)
// Format: { "cline.mcpServers": { "name": { "command": "...", "args": [...], "env": {...} } } }
// Or from .cline/mcp.json
func ImportFromCline(path string) ([]ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cline config: %w", err)
	}

	// Try .cline/mcp.json format (simple)
	var simpleServers map[string]struct {
		Command  string            `json:"command"`
		Args     []string          `json:"args"`
		Env      map[string]string `json:"env"`
		URL      string            `json:"url"`
		Type     string            `json:"type"`
		Headers  map[string]string `json:"headers"`
		Disabled bool              `json:"disabled"`
	}

	if err := json.Unmarshal(data, &simpleServers); err == nil {
		var configs []ClientConfig
		for name, server := range simpleServers {
			serverType := "stdio"
			if server.Type != "" {
				serverType = server.Type
			} else if server.URL != "" {
				serverType = "sse"
			}

			configs = append(configs, ClientConfig{
				Name:    name,
				Type:    serverType,
				Command: server.Command,
				Args:    server.Args,
				URL:     server.URL,
				Env:     server.Env,
				Headers: server.Headers,
				Enabled: !server.Disabled,
			})
		}
		if len(configs) > 0 {
			return configs, nil
		}
	}

	// Try VS Code settings.json format
	var vsSettings map[string]json.RawMessage
	if err := json.Unmarshal(data, &vsSettings); err != nil {
		return nil, fmt.Errorf("parse cline config: %w", err)
	}

	// Look for cline.mcpServers
	for key, raw := range vsSettings {
		if strings.HasSuffix(key, "mcpServers") || strings.HasSuffix(key, "mcp_servers") {
			var servers map[string]struct {
				Command string            `json:"command"`
				Args    []string          `json:"args"`
				Env     map[string]string `json:"env"`
				URL     string            `json:"url"`
			}
			if err := json.Unmarshal(raw, &servers); err != nil {
				continue
			}

			var configs []ClientConfig
			for name, server := range servers {
				serverType := "stdio"
				if server.URL != "" {
					serverType = "sse"
				}
				configs = append(configs, ClientConfig{
					Name:    name,
					Type:    serverType,
					Command: server.Command,
					Args:    server.Args,
					URL:     server.URL,
					Env:     server.Env,
					Enabled: true,
				})
			}
			return configs, nil
		}
	}

	return nil, nil
}

// ImportFromContinue imports MCP servers from config.yaml (Continue.dev format)
// YAML format: experimental: { mcpServers: { name: { command: ..., args: [...] } } }
func ImportFromContinue(path string) ([]ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read continue config: %w", err)
	}

	// Continue uses YAML — parse as map[string]any
	// Try JSON parsing for compatibility
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		// If not JSON, try simple text YAML parsing
		return nil, fmt.Errorf("continue config format not supported yet")
	}

	// Look for mcpServers at any level
	servers := findMCPServers(raw)
	if servers == nil {
		return nil, nil
	}

	var configs []ClientConfig
	for name, s := range servers {
		server, ok := s.(map[string]any)
		if !ok {
			continue
		}
		config := ClientConfig{
			Name:    name,
			Enabled: true,
		}

		if cmd, ok := server["command"].(string); ok {
			config.Command = cmd
			config.Type = "stdio"
		}
		if url, ok := server["url"].(string); ok {
			config.URL = url
			config.Type = "sse"
		}
		if args, ok := server["args"].([]any); ok {
			for _, a := range args {
				if s, ok := a.(string); ok {
					config.Args = append(config.Args, s)
				}
			}
		}
		if env, ok := server["env"].(map[string]any); ok {
			config.Env = make(map[string]string, len(env))
			for k, v := range env {
				if s, ok := v.(string); ok {
					config.Env[k] = s
				}
			}
		}

		configs = append(configs, config)
	}

	return configs, nil
}

// ImportFromAider imports settings from .aider.conf.yml
// Aider has no MCP server format, but may contain model settings
func ImportFromAider(path string) ([]ClientConfig, error) {
	// Aider doesn't support MCP servers — return empty list
	// File may exist but MCP configuration is not provided in it
	return nil, nil
}

// AutoImport scans project and imports MCP configuration from all found sources
func AutoImport(projectDir string) ([]ClientConfig, error) {
	var allConfigs []ClientConfig
	var errs []string

	// Look for .mcp.json (Claude Code)
	mcpJSONPath := filepath.Join(projectDir, ".mcp.json")
	if _, err := os.Stat(mcpJSONPath); err == nil {
		configs, err := ImportFromClaudeCode(mcpJSONPath)
		if err != nil {
			errs = append(errs, fmt.Sprintf(".mcp.json: %v", err))
		} else {
			allConfigs = append(allConfigs, configs...)
		}
	}

	// Look for .claude/mcp.json (Claude Code project scope)
	claudeMCPPath := filepath.Join(projectDir, ".claude", "mcp.json")
	if _, err := os.Stat(claudeMCPPath); err == nil {
		configs, err := ImportFromClaudeCode(claudeMCPPath)
		if err != nil {
			errs = append(errs, fmt.Sprintf(".claude/mcp.json: %v", err))
		} else {
			allConfigs = append(allConfigs, configs...)
		}
	}

	// Look for .cline/mcp.json (Cline)
	clineMCPPath := filepath.Join(projectDir, ".cline", "mcp.json")
	if _, err := os.Stat(clineMCPPath); err == nil {
		configs, err := ImportFromCline(clineMCPPath)
		if err != nil {
			errs = append(errs, fmt.Sprintf(".cline/mcp.json: %v", err))
		} else {
			allConfigs = append(allConfigs, configs...)
		}
	}

	// Look for VS Code settings (Cline)
	vscodeSettingsPath := filepath.Join(projectDir, ".vscode", "settings.json")
	if _, err := os.Stat(vscodeSettingsPath); err == nil {
		configs, err := ImportFromCline(vscodeSettingsPath)
		if err != nil {
			errs = append(errs, fmt.Sprintf("VS Code settings: %v", err))
		} else {
			allConfigs = append(allConfigs, configs...)
		}
	}

	// Look for Continue config.yaml
	continuePath := filepath.Join(projectDir, ".continue", "config.yaml")
	if _, err := os.Stat(continuePath); err == nil {
		configs, err := ImportFromContinue(continuePath)
		if err != nil {
			errs = append(errs, fmt.Sprintf("Continue config: %v", err))
		} else {
			allConfigs = append(allConfigs, configs...)
		}
	}

	// Look for .aider.conf.yml (Aider — does not support MCP)
	// Skip, since ImportFromAider always returns nil

	if len(errs) > 0 && len(allConfigs) == 0 {
		return nil, fmt.Errorf("auto-import errors: %s", strings.Join(errs, "; "))
	}

	return allConfigs, nil
}

// findMCPServers recursively searches for mcpServers in JSON structure
func findMCPServers(data map[string]any) map[string]any {
	// Direct search
	if servers, ok := data["mcpServers"].(map[string]any); ok {
		return servers
	}
	if servers, ok := data["mcp_servers"].(map[string]any); ok {
		return servers
	}

	// Recursive search
	for _, v := range data {
		if nested, ok := v.(map[string]any); ok {
			if servers := findMCPServers(nested); servers != nil {
				return servers
			}
		}
	}

	return nil
}
