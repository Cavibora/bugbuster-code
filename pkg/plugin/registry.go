package plugin

import (
	"bugbuster-code/pkg/config"
)

// KnownPlugin — description of a known plugin in registry
type KnownPlugin struct {
	Name        string                 // unique name
	Description string                 // description
	Type        string                 // "mcp" or "go"
	InstallCmd  string                 // install command (npm, pip, go)
	Config      config.MCPServerConfig // configuration for MCP plugins
}

// KnownPlugins — registry of known plugins
// Can be installed via /plugin install <name>
var KnownPlugins = map[string]KnownPlugin{
	// MCP servers (stdio)
	"github": {
		Name:        "github",
		Description: "GitHub API — issues, PRs, repos, search",
		Type:        "mcp",
		InstallCmd:  "npx -y @modelcontextprotocol/server-github",
		Config: config.MCPServerConfig{
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-github"},
			Enabled: true,
		},
	},
	"filesystem": {
		Name:        "filesystem",
		Description: "Filesystem access — read, write, search files",
		Type:        "mcp",
		InstallCmd:  "npx -y @modelcontextprotocol/server-filesystem",
		Config: config.MCPServerConfig{
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-filesystem"},
			Enabled: true,
		},
	},
	"postgres": {
		Name:        "postgres",
		Description: "PostgreSQL database — query, schema, insert",
		Type:        "mcp",
		InstallCmd:  "npx -y @modelcontextprotocol/server-postgres",
		Config: config.MCPServerConfig{
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-postgres"},
			Enabled: true,
		},
	},
	"brave-search": {
		Name:        "brave-search",
		Description: "Brave Search API — web search",
		Type:        "mcp",
		InstallCmd:  "npx -y @modelcontextprotocol/server-brave-search",
		Config: config.MCPServerConfig{
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-brave-search"},
			Enabled: true,
		},
	},
	"sqlite": {
		Name:        "sqlite",
		Description: "SQLite database — query, create tables",
		Type:        "mcp",
		InstallCmd:  "npx -y @modelcontextprotocol/server-sqlite",
		Config: config.MCPServerConfig{
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-sqlite"},
			Enabled: true,
		},
	},
	"memory": {
		Name:        "memory",
		Description: "Persistent memory — store and retrieve knowledge",
		Type:        "mcp",
		InstallCmd:  "npx -y @modelcontextprotocol/server-memory",
		Config: config.MCPServerConfig{
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-memory"},
			Enabled: true,
		},
	},
	"puppeteer": {
		Name:        "puppeteer",
		Description: "Browser automation — navigate, screenshot, click",
		Type:        "mcp",
		InstallCmd:  "npx -y @modelcontextprotocol/server-puppeteer",
		Config: config.MCPServerConfig{
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-puppeteer"},
			Enabled: true,
		},
	},
	"sequential-thinking": {
		Name:        "sequential-thinking",
		Description: "Sequential thinking — step-by-step reasoning",
		Type:        "mcp",
		InstallCmd:  "npx -y @modelcontextprotocol/server-sequential-thinking",
		Config: config.MCPServerConfig{
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-sequential-thinking"},
			Enabled: true,
		},
	},
	"fetch": {
		Name:        "fetch",
		Description: "Web fetch — retrieve and process web content",
		Type:        "mcp",
		InstallCmd:  "npx -y @modelcontextprotocol/server-fetch",
		Config: config.MCPServerConfig{
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-fetch"},
			Enabled: true,
		},
	},
	"everything": {
		Name:        "everything",
		Description: "MCP test server — all tool types and features",
		Type:        "mcp",
		InstallCmd:  "npx -y @modelcontextprotocol/server-everything",
		Config: config.MCPServerConfig{
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-everything"},
			Enabled: true,
		},
	},
}

// ListKnownPlugins returns list of all known plugins
func ListKnownPlugins() []KnownPlugin {
	var result []KnownPlugin
	for i := range KnownPlugins {
		result = append(result, KnownPlugins[i])
	}
	return result
}

// GetKnownPlugin returns known plugin info by name
func GetKnownPlugin(name string) (KnownPlugin, bool) {
	p, ok := KnownPlugins[name]
	return p, ok
}
