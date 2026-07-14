package config

import (
	"os"
	"path/filepath"
	"strings"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
	"gopkg.in/yaml.v3"
)

// BugBusterConfig — configuration BugBuster Code
type BugBusterConfig struct {
	DefaultProvider string                             `yaml:"default_provider"`
	AgentProviders  map[string]string                  `yaml:"agent_providers"`
	Agent           AgentConfig                        `yaml:"agent"`
	Providers       map[string]provider.ProviderConfig `yaml:"providers"`
	Tools           ToolsConfig                        `yaml:"tools"`
	Security        SecurityConfig                     `yaml:"security"`
	MCP             MCPConfigSection                   `yaml:"mcp"`
	Plugins         PluginsConfigSection               `yaml:"plugins"`
	Theme           ThemeConfig                        `yaml:"theme"`
	Keys            KeyBindings                        `yaml:"keys"`
	MCPServe        MCPServeConfig                     `yaml:"mcp_serve"`
	LSP             LSPConfig                          `yaml:"lsp"`
	ContextArchive  ContextArchiveConfig               `yaml:"context_archive"`
	UI              string                             `yaml:"ui"` // "auto", "tui", "cli" (default: "auto")
}

// ContextArchiveConfig is context archiving settings
type ContextArchiveConfig struct {
	Enabled      bool `yaml:"enabled"`       // enable archiving (default: true)
	MaxBlocks    int  `yaml:"max_blocks"`    // max blocks in archive (default: 50)
	AutoOptimize bool `yaml:"auto_optimize"` // background optimization during compaction (default: true)
}

// ThemeConfig — theme configuration (colors, markdown rendering)
type ThemeConfig struct {
	Mode     string      `yaml:"mode"`      // "dark" | "light"
	WordWrap int         `yaml:"word_wrap"` // word wrap in markdown (0 = no wrap)
	Colors   ThemeColors `yaml:"colors"`    // colors
}

// ThemeColors — theme colors
type ThemeColors struct {
	Primary     string `yaml:"primary"`          // spinner, tool call headers
	Success     string `yaml:"success"`          // ✓, create, diff additions
	Error       string `yaml:"error"`            // ✗, errors, diff deletions
	Warning     string `yaml:"warning"`          // warnings, diff modifications
	Info        string `yaml:"info"`             // input tokens
	Dim         string `yaml:"dim"`              // dimmed text
	Thinking    string `yaml:"thinking"`         // thinking block
	ToolParams  string `yaml:"tool_params"`      // tool call parameters
	ToolSummary string `yaml:"tool_summary"`     // tool call result summary
	StatusTime  string `yaml:"status_time"`      // ⏱ time in status
	StatusSep   string `yaml:"status_separator"` // │ separator in status
	CtxGood     string `yaml:"context_bar_good"` // context < 50%
	CtxWarn     string `yaml:"context_bar_warn"` // context 50-80%
	CtxBad      string `yaml:"context_bar_bad"`  // context > 80%
	UserMsg     string `yaml:"user_message"`     // ❯ user input (TUI)
	Assistant   string `yaml:"assistant"`        // assistant spinner/status (TUI)
	Separator   string `yaml:"separator"`        // ─── separator
}

// MCPConfigSection — MCP servers configuration
type MCPConfigSection struct {
	Servers map[string]MCPServerConfig `yaml:"servers"`
}

// MCPServerConfig — individual MCP server configuration
type MCPServerConfig struct {
	Type    string            `yaml:"type"`    // "stdio", "sse", "streamable-http"
	Command string            `yaml:"command"` // command for stdio
	Args    []string          `yaml:"args"`    // arguments
	URL     string            `yaml:"url"`     // URL for SSE/HTTP
	Env     map[string]string `yaml:"env"`     // environment variables
	Headers map[string]string `yaml:"headers"` // HTTP headers (Authorization etc.)
	Enabled bool              `yaml:"enabled"` // whether enabled
}

// MCPServeConfig — BugBuster MCP server configuration (when BugBuster acts as server)
type MCPServeConfig struct {
	Transport string `yaml:"transport"` // "stdio", "sse", "streamable-http"
	Host      string `yaml:"host"`      // host for SSE/HTTP
	Port      int    `yaml:"port"`      // port for SSE/HTTP
	Prefix    string `yaml:"prefix"`    // tools prefix (default "bugbuster_")
	Enabled   bool   `yaml:"enabled"`   // enable server on startup
}

// LSPConfig — LSP servers configuration (Language Server Protocol)
type LSPConfig struct {
	Servers map[string]LSPServerConfig `yaml:"servers"` // language → server configuration
	Timeout int                        `yaml:"timeout"` // request timeout in seconds (default 10)
}

// LSPServerConfig — individual LSP server configuration
type LSPServerConfig struct {
	Command string   `yaml:"command"` // launch command (e.g. "gopls")
	Args    []string `yaml:"args"`    // command arguments (e.g. ["serve"])
}

// PluginsConfigSection — plugins configuration
type PluginsConfigSection struct {
	Builtins  []string                   `yaml:"builtins"` // built-in plugins (filesystem, bash, web)
	GoPlugins []GoPluginConfig           `yaml:"go"`       // external Go plugins (.so)
	MCP       map[string]MCPServerConfig `yaml:"mcp"`      // MCP servers (universal)
	Enabled   []string                   `yaml:"enabled"`  // backward compatibility: list of enabled built-in plugins
	Config    map[string]map[string]any  `yaml:"config"`   // backward compatibility: plugins configuration
}

// GoPluginConfig is external Go plugin configuration (.so)
type GoPluginConfig struct {
	Name   string         `yaml:"name"`   // unique name plugin
	Path   string         `yaml:"path"`   // path to .so file
	Config map[string]any `yaml:"config"` // plugin configuration
}

// AgentConfig is agent settings
type AgentConfig struct {
	MaxTokens       int                 `yaml:"max_tokens"`  // max tokens in context (0 = default 8000)
	KeepRecent      int                 `yaml:"keep_recent"` // how many recent messages to keep during compaction
	Verbose         bool                `yaml:"verbose"`
	PermissionMode  string              `yaml:"permission_mode"`  // auto-approve, ask, deny
	Permissions     PermissionsConfig    `yaml:"permissions"`      // per-tool permission overrides
	Language        string              `yaml:"language"`         // interface language (en, ru, es, fr, de, ja, zh, pt)
	RequestTimeout  int                 `yaml:"request_timeout"`  // max time for a single LLM request in seconds (0 = default 2400 = 40 min)
	ThinkingTimeout int                 `yaml:"thinking_timeout"` // max time without tokens from model in seconds (0 = default 600 = 10 min)
	IdleTimeout     int                 `yaml:"idle_timeout"`     // streaming timeout without events in seconds (0 = default 120 = 2 min)
	AutoContinue    bool                `yaml:"auto_continue"`    // auto-continue when model responds with text only (default: true)
	LoopDetection   LoopDetectionConfig `yaml:"loop_detection"`   // loop detection settings
	Subagent        SubagentYAMLConfig  `yaml:"subagent"`         // subagent configuration
	Fallback        FallbackConfig      `yaml:"fallback"`         // fallback provider configuration
}

// FallbackConfig — fallback provider settings
// When the primary provider fails (network error, rate limit, etc.),
// the agent automatically switches to the fallback provider.
type FallbackConfig struct {
	Provider       string `yaml:"provider"`        // fallback provider name (from providers map)
	MaxRetries     int    `yaml:"max_retries"`     // max retries on primary before fallback (default: 2)
	RetryDelayMs   int    `yaml:"retry_delay_ms"`  // delay between retries in ms (default: 1000)
	AutoSwitchBack bool   `yaml:"auto_switch_back"` // switch back to primary when it recovers (default: true)
}

// SubagentYAMLConfig — subagent configuration in YAML
type SubagentYAMLConfig struct {
	Provider       string `yaml:"provider"`         // provider name for subagent (empty = inherit from parent)
	Model          string `yaml:"model"`            // model name override (empty = use provider default)
	MaxConcurrent  int    `yaml:"max_concurrent"`   // max concurrent subagents (default: 3)
	MaxIterations  int    `yaml:"max_iterations"`   // max loop iterations for subagent (default: 15)
	Timeout        int    `yaml:"timeout"`           // timeout for subagent in seconds (default: 600 = 10m)
	ContextTokens  int    `yaml:"context_tokens"`   // context window size (0 = inherit from parent)
	ContextKeepRecent int `yaml:"context_keep_recent"` // keep recent messages on compaction (0 = auto)
}

// LoopDetectionConfig — loop detection settings
type LoopDetectionConfig struct {
	RepeatThreshold         int     `yaml:"repeat_threshold"`          // how many identical consecutive calls = loop (default: 6)
	ToolRepeatThreshold     int     `yaml:"tool_repeat_threshold"`     // how many calls of same tool with same parameters = loop (default: 8)
	WindowSize              int     `yaml:"window_size"`               // sliding window size (default: 30)
	TextSimilarityThreshold float64 `yaml:"text_similarity_threshold"` // text similarity threshold 0.0-1.0 (default: 0.65)
	TextSimilarityWindow    int     `yaml:"text_similarity_window"`    // how many text responses to check (default: 4)
}

// ToolsConfig — settings tools
type ToolsConfig struct {
	AllowedDirs    []string     `yaml:"allowed_dirs"`
	MaxFileSize    int64        `yaml:"max_file_size"`
	BashTimeout    int          `yaml:"bash_timeout"`
	MaxGrepResults int          `yaml:"max_grep_results"`
	MaxGlobResults int          `yaml:"max_glob_results"`
	Browse         BrowseConfig `yaml:"browse"`
	Screenshot     ScreenshotConfig `yaml:"screenshot"`
	TTS            TTSConfig    `yaml:"tts"`
	STT            STTConfig    `yaml:"stt"`
}

// ScreenshotConfig — screenshot capture settings
type ScreenshotConfig struct {
	Enabled bool   `yaml:"enabled"`  // enable screenshot tool (default true)
	Format  string `yaml:"format"`   // image format: "png" (default) or "jpeg"
	Quality int    `yaml:"quality"`  // JPEG quality 1-100 (default 85, PNG only)
}

// TTSConfig — text-to-speech settings
type TTSConfig struct {
	Enabled bool   `yaml:"enabled"` // enable TTS tool (default true)
	Voice   string `yaml:"voice"`   // voice: "alloy", "echo", "fable", "onyx", "nova", "shimmer" (default "alloy")
	Model   string `yaml:"model"`   // TTS model: "tts-1" (default) or "tts-1-hd"
}

// STTConfig — speech-to-text settings
type STTConfig struct {
	Enabled  bool   `yaml:"enabled"`   // enable STT tool (default true)
	Language string `yaml:"language"`   // default language hint: "en", "ru", etc. (empty = auto-detect)
	Model    string `yaml:"model"`      // STT model: "whisper-1" (default)
}

// BrowseConfig — headless browser and search configuration
type BrowseConfig struct {
	Engine      string `yaml:"engine"`       // "chromedp" (default), "rod", "playwright", "http" (fallback)
	SearchEngine string `yaml:"search_engine"` // "duckduckgo" (default), "google", "yandex", "bing"
	Timeout     int    `yaml:"timeout"`       // page load timeout in seconds (default 30)
	MaxResults  int    `yaml:"max_results"`   // max search results (default 10)
	UserAgent   string `yaml:"user_agent"`    // custom user agent
	Headless    bool   `yaml:"headless"`      // run browser in headless mode (default true)
	ChromePath  string `yaml:"chrome_path"`   // path to Chrome/Chromium binary (auto-detect if empty)
}

// SecurityConfig — settings security
type SecurityConfig struct {
	AllowNetwork    bool     `yaml:"allow_network"`
	BlockedCommands []string `yaml:"blocked_commands"`
	SandboxDir      string   `yaml:"sandbox_dir"`
}

// PermissionsConfig — granular per-tool permissions
// Each tool can have its own permission mode: "auto-approve", "ask", "deny"
// If not set, the global agent.permission_mode is used
type PermissionsConfig struct {
	Bash       string `yaml:"bash"`        // bash command execution
	Write      string `yaml:"write"`      // file write
	Edit       string `yaml:"edit"`       // file edit
	Read       string `yaml:"read"`       // file read (outside allowed_dirs)
	Grep       string `yaml:"grep"`       // search in files
	Glob       string `yaml:"glob"`       // file search by pattern
	WebFetch   string `yaml:"web_fetch"`  // HTTP requests
	Browse     string `yaml:"browse"`      // web browsing/search
	Ask        string `yaml:"ask"`        // ask external LLM
	Learn      string `yaml:"learn"`       // teach/train model
	Memory     string `yaml:"memory"`     // persistent memory
	Background string `yaml:"background"`  // background processes
	Kill       string `yaml:"kill"`        // kill processes
	Screenshot string `yaml:"screenshot"` // screenshot capture
	SendFile   string `yaml:"send_file"`   // send files to model
	TTS        string `yaml:"tts"`         // text-to-speech
	STT        string `yaml:"stt"`         // speech-to-text
	LSP        string `yaml:"lsp"`         // language server
	MCP        string `yaml:"mcp"`         // MCP tools
	Delegate   string `yaml:"delegate"`    // subagent delegation
}

// EffectivePermission returns the effective permission mode for a tool.
// If per-tool override is set, it's used; otherwise falls back to global mode.
func (p *PermissionsConfig) EffectivePermission(toolName, globalMode string) string {
	toolPerm := ""
	switch toolName {
	case "bash":
		toolPerm = p.Bash
	case "write":
		toolPerm = p.Write
	case "edit":
		toolPerm = p.Edit
	case "read":
		toolPerm = p.Read
	case "grep":
		toolPerm = p.Grep
	case "glob":
		toolPerm = p.Glob
	case "web_fetch":
		toolPerm = p.WebFetch
	case "browse":
		toolPerm = p.Browse
	case "ask":
		toolPerm = p.Ask
	case "learn":
		toolPerm = p.Learn
	case "memory":
		toolPerm = p.Memory
	case "background":
		toolPerm = p.Background
	case "ps":
		toolPerm = p.Background // ps is part of background process management
	case "logs":
		toolPerm = p.Background // logs is part of background process management
	case "kill":
		toolPerm = p.Kill
	case "screenshot":
		toolPerm = p.Screenshot
	case "send_file":
		toolPerm = p.SendFile
	case "tts":
		toolPerm = p.TTS
	case "stt":
		toolPerm = p.STT
	case "lsp":
		toolPerm = p.LSP
	case "mcp":
		toolPerm = p.MCP
	case "delegate_task":
		toolPerm = p.Delegate
	}
	if toolPerm != "" {
		return toolPerm
	}
	return globalMode
}

// EffectiveMap returns a map of tool name → permission mode for all non-empty overrides.
// This is used to populate the DefaultPermissionChecker.
func (p *PermissionsConfig) EffectiveMap() map[string]string {
	result := make(map[string]string)
	if p.Bash != "" {
		result["bash"] = p.Bash
	}
	if p.Write != "" {
		result["write"] = p.Write
	}
	if p.Edit != "" {
		result["edit"] = p.Edit
	}
	if p.Read != "" {
		result["read"] = p.Read
	}
	if p.Grep != "" {
		result["grep"] = p.Grep
	}
	if p.Glob != "" {
		result["glob"] = p.Glob
	}
	if p.WebFetch != "" {
		result["web_fetch"] = p.WebFetch
	}
	if p.Browse != "" {
		result["browse"] = p.Browse
	}
	if p.Ask != "" {
		result["ask"] = p.Ask
	}
	if p.Learn != "" {
		result["learn"] = p.Learn
	}
	if p.Memory != "" {
		result["memory"] = p.Memory
	}
	if p.Background != "" {
		result["background"] = p.Background
		result["ps"] = p.Background
		result["logs"] = p.Background
	}
	if p.Kill != "" {
		result["kill"] = p.Kill
	}
	if p.Screenshot != "" {
		result["screenshot"] = p.Screenshot
	}
	if p.SendFile != "" {
		result["send_file"] = p.SendFile
	}
	if p.TTS != "" {
		result["tts"] = p.TTS
	}
	if p.STT != "" {
		result["stt"] = p.STT
	}
	if p.LSP != "" {
		result["lsp"] = p.LSP
	}
	if p.MCP != "" {
		result["mcp"] = p.MCP
	}
	if p.Delegate != "" {
		result["delegate_task"] = p.Delegate
	}
	return result
}

// DefaultConfig returns configuration default
func DefaultConfig() *BugBusterConfig {
	return &BugBusterConfig{
		DefaultProvider: "ollama",
		Agent: AgentConfig{
			MaxTokens:       32768,
			KeepRecent:      20,
			Verbose:         false,
			PermissionMode:  "auto-approve",
			Language:        "en",
			RequestTimeout:  2400, // 40 minutes
			ThinkingTimeout: 600,  // 10 minutes
			IdleTimeout:     300,  // 5 minutes
			AutoContinue:    true,
			Subagent: SubagentYAMLConfig{
				MaxConcurrent:  3,
				MaxIterations:  15,
				Timeout:        600,
			},
			Fallback: FallbackConfig{
				MaxRetries:     2,
				RetryDelayMs:   1000,
				AutoSwitchBack: true,
			},
			LoopDetection: LoopDetectionConfig{
				RepeatThreshold:         6,
				ToolRepeatThreshold:     8,
				WindowSize:              30,
				TextSimilarityThreshold: 0.65,
				TextSimilarityWindow:    4,
			},
		},
		Providers: map[string]provider.ProviderConfig{
			"ollama": {
				Type:    "ollama",
				BaseURL: "http://localhost:11434",
				Model:   "qwen-fast-27b",
			},
		},
		Tools: ToolsConfig{
			MaxFileSize:    1024 * 1024,
			BashTimeout:    30,
			MaxGrepResults: 50,
			MaxGlobResults: 100,
			Screenshot: ScreenshotConfig{
				Enabled: true,
				Format:  "png",
			},
			TTS: TTSConfig{
				Enabled: true,
				Voice:   "alloy",
				Model:   "tts-1",
			},
			STT: STTConfig{
				Enabled:  true,
				Language: "",
				Model:    "whisper-1",
			},
		},
		Security: SecurityConfig{
			AllowNetwork:    false,
			BlockedCommands: []string{"rm -rf /", "mkfs", "dd if=", "format c:"},
		},
		Theme: ThemeConfig{
			Mode:     "dark",
			WordWrap: 80,
		},
		Keys: DefaultKeyBindings(),
		ContextArchive: ContextArchiveConfig{
			Enabled:      true,
			MaxBlocks:    50,
			AutoOptimize: true,
		},
		UI: "auto",
		LSP: LSPConfig{
			Timeout: 10,
			Servers: map[string]LSPServerConfig{
				"go":         {Command: "gopls", Args: []string{"serve"}},
				"typescript": {Command: "typescript-language-server", Args: []string{"--stdio"}},
				"python":     {Command: "pylsp"},
			},
		},
	}
}

// LoadConfig loads configuration from YAML file
func LoadConfig(path string) (*BugBusterConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, i18n.E("errors_config.read", err)
	}

	config := DefaultConfig()
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, i18n.E("errors_config.parse", err)
	}

	// Resolve environment variables in api_key
	for name := range config.Providers {
		prov := config.Providers[name]
		prov.APIKey = resolveEnvVars(prov.APIKey)
		config.Providers[name] = prov
	}

	return config, nil
}

// SaveConfig saves configuration to YAML file
func (c *BugBusterConfig) SaveConfig(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return i18n.E("errors_config.create_dir", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return i18n.E("errors_config.serialize", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return i18n.E("errors_config.write", err)
	}

	return nil
}

// ConfigFileNames is the list of config file names to search for, in priority order.
// Hidden file (.bugbuster.yaml) takes priority over visible file (bugbuster.yaml).
var ConfigFileNames = []string{".bugbuster.yaml", "bugbuster.yaml"}

// FindConfigFile searches for config file
// Priority: --config flag > .bugbuster.yaml > bugbuster.yaml (walk up) > ~/.bugbuster/config.yaml
func FindConfigFile() (string, error) {
	// Search for config files in current directory and above
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for i := 0; i < 10; i++ {
		for _, name := range ConfigFileNames {
			configPath := filepath.Join(dir, name)
			if _, err := os.Stat(configPath); err == nil {
				return configPath, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Check home directory
	home, err := os.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(home, ".bugbuster", "config.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}
	}

	return "", i18n.E("errors_config.not_found")
}

// MergeConfigs merges multiple configurations (later = higher priority)
func MergeConfigs(configs ...*BugBusterConfig) *BugBusterConfig {
	result := DefaultConfig()

	for _, cfg := range configs {
		if cfg == nil {
			continue
		}

		// DefaultProvider
		if cfg.DefaultProvider != "" {
			result.DefaultProvider = cfg.DefaultProvider
		}

		// Agent
		if cfg.Agent.Verbose {
			result.Agent.Verbose = cfg.Agent.Verbose
		}
		if cfg.Agent.PermissionMode != "" {
			result.Agent.PermissionMode = cfg.Agent.PermissionMode
		}
		if cfg.Agent.MaxTokens > 0 {
			result.Agent.MaxTokens = cfg.Agent.MaxTokens
		}
		if cfg.Agent.KeepRecent > 0 {
			result.Agent.KeepRecent = cfg.Agent.KeepRecent
		}
		if cfg.Agent.Language != "" {
			result.Agent.Language = cfg.Agent.Language
		}

		if cfg.Agent.RequestTimeout > 0 {
			result.Agent.RequestTimeout = cfg.Agent.RequestTimeout
		}
		if cfg.Agent.ThinkingTimeout > 0 {
			result.Agent.ThinkingTimeout = cfg.Agent.ThinkingTimeout
		}
		if cfg.Agent.IdleTimeout > 0 {
			result.Agent.IdleTimeout = cfg.Agent.IdleTimeout
		}
		// AutoContinue
		if cfg.Agent.AutoContinue {
			result.Agent.AutoContinue = cfg.Agent.AutoContinue
		}
		// Subagent
		if cfg.Agent.Subagent.Provider != "" {
			result.Agent.Subagent.Provider = cfg.Agent.Subagent.Provider
		}
		if cfg.Agent.Subagent.Model != "" {
			result.Agent.Subagent.Model = cfg.Agent.Subagent.Model
		}
		if cfg.Agent.Subagent.MaxConcurrent > 0 {
			result.Agent.Subagent.MaxConcurrent = cfg.Agent.Subagent.MaxConcurrent
		}
		if cfg.Agent.Subagent.MaxIterations > 0 {
			result.Agent.Subagent.MaxIterations = cfg.Agent.Subagent.MaxIterations
		}
		if cfg.Agent.Subagent.Timeout > 0 {
			result.Agent.Subagent.Timeout = cfg.Agent.Subagent.Timeout
		}
		if cfg.Agent.Subagent.ContextTokens > 0 {
			result.Agent.Subagent.ContextTokens = cfg.Agent.Subagent.ContextTokens
		}
		if cfg.Agent.Subagent.ContextKeepRecent > 0 {
			result.Agent.Subagent.ContextKeepRecent = cfg.Agent.Subagent.ContextKeepRecent
		}
		// Fallback
		if cfg.Agent.Fallback.Provider != "" {
			result.Agent.Fallback.Provider = cfg.Agent.Fallback.Provider
		}
		if cfg.Agent.Fallback.MaxRetries > 0 {
			result.Agent.Fallback.MaxRetries = cfg.Agent.Fallback.MaxRetries
		}
		if cfg.Agent.Fallback.RetryDelayMs > 0 {
			result.Agent.Fallback.RetryDelayMs = cfg.Agent.Fallback.RetryDelayMs
		}
		// AutoSwitchBack: true always wins (if any config enables — enable)
		if cfg.Agent.Fallback.AutoSwitchBack {
			result.Agent.Fallback.AutoSwitchBack = true
		}
		// LoopDetection
		if cfg.Agent.LoopDetection.RepeatThreshold > 0 {
			result.Agent.LoopDetection.RepeatThreshold = cfg.Agent.LoopDetection.RepeatThreshold
		}
		if cfg.Agent.LoopDetection.ToolRepeatThreshold > 0 {
			result.Agent.LoopDetection.ToolRepeatThreshold = cfg.Agent.LoopDetection.ToolRepeatThreshold
		}
		if cfg.Agent.LoopDetection.WindowSize > 0 {
			result.Agent.LoopDetection.WindowSize = cfg.Agent.LoopDetection.WindowSize
		}
		if cfg.Agent.LoopDetection.TextSimilarityThreshold > 0 {
			result.Agent.LoopDetection.TextSimilarityThreshold = cfg.Agent.LoopDetection.TextSimilarityThreshold
		}
		if cfg.Agent.LoopDetection.TextSimilarityWindow > 0 {
			result.Agent.LoopDetection.TextSimilarityWindow = cfg.Agent.LoopDetection.TextSimilarityWindow
		}
		// Providers (merge)
		if result.Providers == nil {
			result.Providers = make(map[string]provider.ProviderConfig)
		}
		for name := range cfg.Providers {
			result.Providers[name] = cfg.Providers[name]
		}

		// AgentProviders (merge)
		if cfg.AgentProviders != nil {
			if result.AgentProviders == nil {
				result.AgentProviders = make(map[string]string)
			}
			for k, v := range cfg.AgentProviders {
				result.AgentProviders[k] = v
			}
		}

		// Tools
		if len(cfg.Tools.AllowedDirs) > 0 {
			result.Tools.AllowedDirs = cfg.Tools.AllowedDirs
		}
		if cfg.Tools.MaxFileSize > 0 {
			result.Tools.MaxFileSize = cfg.Tools.MaxFileSize
		}
		if cfg.Tools.BashTimeout > 0 {
			result.Tools.BashTimeout = cfg.Tools.BashTimeout
		}
		if cfg.Tools.MaxGrepResults > 0 {
			result.Tools.MaxGrepResults = cfg.Tools.MaxGrepResults
		}
		if cfg.Tools.MaxGlobResults > 0 {
			result.Tools.MaxGlobResults = cfg.Tools.MaxGlobResults
		}
		// Browse config
		if cfg.Tools.Browse.Engine != "" {
			result.Tools.Browse.Engine = cfg.Tools.Browse.Engine
		}
		if cfg.Tools.Browse.SearchEngine != "" {
			result.Tools.Browse.SearchEngine = cfg.Tools.Browse.SearchEngine
		}
		if cfg.Tools.Browse.Timeout > 0 {
			result.Tools.Browse.Timeout = cfg.Tools.Browse.Timeout
		}
		if cfg.Tools.Browse.MaxResults > 0 {
			result.Tools.Browse.MaxResults = cfg.Tools.Browse.MaxResults
		}
		if cfg.Tools.Browse.UserAgent != "" {
			result.Tools.Browse.UserAgent = cfg.Tools.Browse.UserAgent
		}
		if cfg.Tools.Browse.ChromePath != "" {
			result.Tools.Browse.ChromePath = cfg.Tools.Browse.ChromePath
		}

		// Security
		if len(cfg.Security.BlockedCommands) > 0 {
			result.Security.BlockedCommands = cfg.Security.BlockedCommands
		}
		// AllowNetwork: true always wins (if any config allows — allow)
		if cfg.Security.AllowNetwork {
			result.Security.AllowNetwork = true
		}

		// Plugins (merge)
		if len(cfg.Plugins.Builtins) > 0 {
			result.Plugins.Builtins = cfg.Plugins.Builtins
		}
		if len(cfg.Plugins.GoPlugins) > 0 {
			result.Plugins.GoPlugins = cfg.Plugins.GoPlugins
		}
		if len(cfg.Plugins.MCP) > 0 {
			if result.Plugins.MCP == nil {
				result.Plugins.MCP = make(map[string]MCPServerConfig)
			}
			for name, srv := range cfg.Plugins.MCP {
				result.Plugins.MCP[name] = srv
			}
		}
		// Backward compatibility: enabled + config
		if len(cfg.Plugins.Enabled) > 0 {
			result.Plugins.Enabled = cfg.Plugins.Enabled
		}
		if len(cfg.Plugins.Config) > 0 {
			if result.Plugins.Config == nil {
				result.Plugins.Config = make(map[string]map[string]any)
			}
			for k, v := range cfg.Plugins.Config {
				result.Plugins.Config[k] = v
			}
		}

		// Theme
		if cfg.Theme.Mode != "" {
			result.Theme.Mode = cfg.Theme.Mode
		}
		if cfg.Theme.WordWrap > 0 {
			result.Theme.WordWrap = cfg.Theme.WordWrap
		}
		// Theme colors — merge non-empty fields
		mergeThemeColor := func(def, override string) string {
			if override != "" {
				return override
			}
			return def
		}
		result.Theme.Colors.Primary = mergeThemeColor(result.Theme.Colors.Primary, cfg.Theme.Colors.Primary)
		result.Theme.Colors.Success = mergeThemeColor(result.Theme.Colors.Success, cfg.Theme.Colors.Success)
		result.Theme.Colors.Error = mergeThemeColor(result.Theme.Colors.Error, cfg.Theme.Colors.Error)
		result.Theme.Colors.Warning = mergeThemeColor(result.Theme.Colors.Warning, cfg.Theme.Colors.Warning)
		result.Theme.Colors.Info = mergeThemeColor(result.Theme.Colors.Info, cfg.Theme.Colors.Info)
		result.Theme.Colors.Dim = mergeThemeColor(result.Theme.Colors.Dim, cfg.Theme.Colors.Dim)
		result.Theme.Colors.Thinking = mergeThemeColor(result.Theme.Colors.Thinking, cfg.Theme.Colors.Thinking)
		result.Theme.Colors.ToolParams = mergeThemeColor(result.Theme.Colors.ToolParams, cfg.Theme.Colors.ToolParams)
		result.Theme.Colors.ToolSummary = mergeThemeColor(result.Theme.Colors.ToolSummary, cfg.Theme.Colors.ToolSummary)
		result.Theme.Colors.StatusTime = mergeThemeColor(result.Theme.Colors.StatusTime, cfg.Theme.Colors.StatusTime)
		result.Theme.Colors.StatusSep = mergeThemeColor(result.Theme.Colors.StatusSep, cfg.Theme.Colors.StatusSep)
		result.Theme.Colors.CtxGood = mergeThemeColor(result.Theme.Colors.CtxGood, cfg.Theme.Colors.CtxGood)
		result.Theme.Colors.CtxWarn = mergeThemeColor(result.Theme.Colors.CtxWarn, cfg.Theme.Colors.CtxWarn)
		result.Theme.Colors.CtxBad = mergeThemeColor(result.Theme.Colors.CtxBad, cfg.Theme.Colors.CtxBad)
		result.Theme.Colors.UserMsg = mergeThemeColor(result.Theme.Colors.UserMsg, cfg.Theme.Colors.UserMsg)
		result.Theme.Colors.Assistant = mergeThemeColor(result.Theme.Colors.Assistant, cfg.Theme.Colors.Assistant)
		result.Theme.Colors.Separator = mergeThemeColor(result.Theme.Colors.Separator, cfg.Theme.Colors.Separator)

		// UI mode
		if cfg.UI != "" {
			result.UI = cfg.UI
		}

		// Keys (merge: non-empty fields override defaults)
		if len(cfg.Keys.Send) > 0 {
			result.Keys.Send = cfg.Keys.Send
		}
		if len(cfg.Keys.Newline) > 0 {
			result.Keys.Newline = cfg.Keys.Newline
		}
		if len(cfg.Keys.Cancel) > 0 {
			result.Keys.Cancel = cfg.Keys.Cancel
		}
		if len(cfg.Keys.Interrupt) > 0 {
			result.Keys.Interrupt = cfg.Keys.Interrupt
		}
		if len(cfg.Keys.HistoryUp) > 0 {
			result.Keys.HistoryUp = cfg.Keys.HistoryUp
		}
		if len(cfg.Keys.HistoryDown) > 0 {
			result.Keys.HistoryDown = cfg.Keys.HistoryDown
		}
		if len(cfg.Keys.ScrollUp) > 0 {
			result.Keys.ScrollUp = cfg.Keys.ScrollUp
		}
		if len(cfg.Keys.ScrollDown) > 0 {
			result.Keys.ScrollDown = cfg.Keys.ScrollDown
		}
	}

	// LSP (after merging all configs)
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		if cfg.LSP.Timeout > 0 {
			result.LSP.Timeout = cfg.LSP.Timeout
		}
		if len(cfg.LSP.Servers) > 0 {
			if result.LSP.Servers == nil {
				result.LSP.Servers = make(map[string]LSPServerConfig)
			}
			for lang, srv := range cfg.LSP.Servers {
				result.LSP.Servers[lang] = srv
			}
		}
	}

	return result
}

// EffectiveSecurity returns security settings for specific provider.
// Provider security has priority over global.
// If a provider field is not set (nil/zero), the global value is used.
func (c *BugBusterConfig) EffectiveSecurity(provCfg *provider.ProviderConfig) SecurityConfig {
	result := c.Security

	// AllowNetwork: provider has priority if set explicitly
	// Zero-value bool = false, so we need to distinguish "not set" from "set to false"
	// Use a separate flag: if provider has at least one security setting — use provider's
	if provCfg.Security.BlockedCommands != nil {
		result.BlockedCommands = provCfg.Security.BlockedCommands
	}
	// For bool, cannot distinguish "not set" from "set to false" via YAML,
	// so AllowNetwork in provider wins only if true
	if provCfg.Security.AllowNetwork {
		result.AllowNetwork = true
	}

	return result
}

// EffectiveContextWindow returns context window size for provider.
// Priority: provCfg.ContextWindow > cfg.Agent.MaxTokens > default 8000
func (c *BugBusterConfig) EffectiveContextWindow(provCfg *provider.ProviderConfig) int {
	if provCfg.ContextWindow > 0 {
		return provCfg.ContextWindow
	}
	if c.Agent.MaxTokens > 0 {
		return c.Agent.MaxTokens
	}
	return 8000
}

// resolveEnvVars replaces ${ENV_VAR} with environment variable value
func resolveEnvVars(s string) string {
	if !strings.Contains(s, "${") {
		return s
	}

	for {
		start := strings.Index(s, "${")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], "}")
		if end == -1 {
			break
		}
		end += start

		envName := s[start+2 : end]
		envValue := os.Getenv(envName)
		s = s[:start] + envValue + s[end+1:]
	}

	return s
}

// GetProviderForTask returns the provider name for a given task type.
// If agent_providers is configured and the task type matches, returns the mapped provider.
// Otherwise returns the default provider.
func (c *BugBusterConfig) GetProviderForTask(taskType string) string {
	if c.AgentProviders != nil {
		if provider, ok := c.AgentProviders[taskType]; ok {
			return provider
		}
	}
	return c.DefaultProvider
}

// GetTaskTypeForProvider returns the task type for a given provider name.
// If the provider is mapped in agent_providers, returns the task type.
// Otherwise returns empty string.
func (c *BugBusterConfig) GetTaskTypeForProvider(providerName string) string {
	if c.AgentProviders != nil {
		for taskType, prov := range c.AgentProviders {
			if prov == providerName {
				return taskType
			}
		}
	}
	return ""
}

// TaskTypes returns all configured task types
func (c *BugBusterConfig) TaskTypes() []string {
	if c.AgentProviders == nil {
		return nil
	}
	types := make([]string, 0, len(c.AgentProviders))
	for k := range c.AgentProviders {
		types = append(types, k)
	}
	return types
}
