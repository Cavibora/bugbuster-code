package config

import (
	"fmt"
	"strings"

	"bugbuster-code/pkg/provider"
)

// ValidationError represents a single validation issue
type ValidationError struct {
	Field   string // dot-separated field path, e.g. "providers.openai.api_key"
	Message string // human-readable error description
	Level   string // "error" (must fix) or "warning" (should fix)
}

func (e ValidationError) String() string {
	prefix := "⚠️"
	if e.Level == "error" {
		prefix = "❌"
	}
	return fmt.Sprintf("%s %s: %s", prefix, e.Field, e.Message)
}

// ValidationErrors is a collection of validation issues
type ValidationErrors struct {
	Errors   []ValidationError
	Warnings []ValidationError
}

func (ve *ValidationErrors) HasErrors() bool {
	return len(ve.Errors) > 0
}

func (ve *ValidationErrors) String() string {
	var sb strings.Builder
	for _, e := range ve.Errors {
		sb.WriteString(e.String() + "\n")
	}
	for _, w := range ve.Warnings {
		sb.WriteString(w.String() + "\n")
	}
	return sb.String()
}

// Known provider types
var knownProviderTypes = map[string]bool{
	"openai":           true,
	"anthropic":        true,
	"ollama":           true,
	"cavibora":         true,
	"openai_compatible": true,
}

// Providers that require an API key
var providersRequiringAPIKey = map[string]bool{
	"openai":           true,
	"anthropic":        true,
	"cavibora":         true,
	"openai_compatible": true,
}

// ValidateConfig validates the BugBuster configuration and returns errors and warnings.
func ValidateConfig(cfg *BugBusterConfig) *ValidationErrors {
	result := &ValidationErrors{}

	// 1. Check default_provider
	if cfg.DefaultProvider == "" {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "default_provider",
			Message: "is empty — no default provider specified. BugBuster will use 'ollama' (local model).",
		})
	} else if len(cfg.Providers) == 0 {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "default_provider",
			Message: fmt.Sprintf("provider '%s' specified but no providers defined at all.", cfg.DefaultProvider),
		})
	} else if _, ok := cfg.Providers[cfg.DefaultProvider]; !ok {
		available := make([]string, 0, len(cfg.Providers))
		for name := range cfg.Providers {
			available = append(available, name)
		}
		result.Errors = append(result.Errors, ValidationError{
			Field:   "default_provider",
			Message: fmt.Sprintf("provider '%s' not found in providers section. Available: %s", cfg.DefaultProvider, strings.Join(available, ", ")),
		})
	}

	// 2. Validate each provider
	for name, prov := range cfg.Providers {
		validateProvider(name, prov, result)
	}

	// 3. Validate agent settings
	if cfg.Agent.MaxTokens > 0 && cfg.Agent.MaxTokens < 1000 {
		result.Warnings = append(result.Warnings, ValidationError{
			Field:   "agent.max_tokens",
			Message: fmt.Sprintf("value %d is very small — context window may be too limited for normal operation. Recommended: 8000+", cfg.Agent.MaxTokens),
		})
	}
	if cfg.Agent.MaxTokens > 1000000 {
		result.Warnings = append(result.Warnings, ValidationError{
			Field:   "agent.max_tokens",
			Message: fmt.Sprintf("value %d is very large — this is the context window size, not the output token limit. Make sure your model supports this.", cfg.Agent.MaxTokens),
		})
	}
	if cfg.Agent.KeepRecent > 0 && cfg.Agent.KeepRecent < 2 {
		result.Warnings = append(result.Warnings, ValidationError{
			Field:   "agent.keep_recent",
			Message: "value < 2 may cause context compaction to lose important messages.",
		})
	}
	if cfg.Agent.RequestTimeout > 0 && cfg.Agent.RequestTimeout < 60 {
		result.Warnings = append(result.Warnings, ValidationError{
			Field:   "agent.request_timeout",
			Message: fmt.Sprintf("value %d seconds may be too short for complex model requests. Recommended: 600+", cfg.Agent.RequestTimeout),
		})
	}

	// 4. Validate agent_providers references
	for taskType, provName := range cfg.AgentProviders {
		if _, ok := cfg.Providers[provName]; !ok {
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("agent_providers.%s", taskType),
				Message: fmt.Sprintf("references provider '%s' which does not exist in providers section.", provName),
			})
		}
	}

	// 5. Validate subagent provider reference
	if cfg.Agent.Subagent.Provider != "" {
		if _, ok := cfg.Providers[cfg.Agent.Subagent.Provider]; !ok {
			result.Errors = append(result.Errors, ValidationError{
				Field:   "agent.subagent.provider",
				Message: fmt.Sprintf("references provider '%s' which does not exist in providers section.", cfg.Agent.Subagent.Provider),
			})
		}
	}

	// 6. Validate MCP servers
	for name, srv := range cfg.MCP.Servers {
		if srv.Type == "" {
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("mcp.servers.%s.type", name),
				Message: "MCP server type is required (stdio, sse, or streamable-http).",
			})
		} else if srv.Type != "stdio" && srv.Type != "sse" && srv.Type != "streamable-http" {
			result.Warnings = append(result.Warnings, ValidationError{
				Field:   fmt.Sprintf("mcp.servers.%s.type", name),
				Message: fmt.Sprintf("unknown MCP server type '%s'. Expected: stdio, sse, or streamable-http.", srv.Type),
			})
		}
		if srv.Type == "stdio" && srv.Command == "" {
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("mcp.servers.%s.command", name),
				Message: "stdio MCP server requires a command.",
			})
		}
		if (srv.Type == "sse" || srv.Type == "streamable-http") && srv.URL == "" {
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("mcp.servers.%s.url", name),
				Message: fmt.Sprintf("%s MCP server requires a URL.", srv.Type),
			})
		}
	}

	// 7. Validate LSP servers
	for lang, srv := range cfg.LSP.Servers {
		if srv.Command == "" {
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("lsp.servers.%s.command", lang),
				Message: "LSP server command is required.",
			})
		}
	}

	// 8. Validate security
	for _, cmd := range cfg.Security.BlockedCommands {
		if cmd == "" {
			result.Warnings = append(result.Warnings, ValidationError{
				Field:   "security.blocked_commands",
				Message: "contains empty string — this will not block anything.",
			})
			break
		}
	}

	// 9. Validate tools
	if cfg.Tools.BashTimeout > 0 && cfg.Tools.BashTimeout < 5 {
		result.Warnings = append(result.Warnings, ValidationError{
			Field:   "tools.bash_timeout",
			Message: fmt.Sprintf("value %d seconds may be too short for most bash commands. Recommended: 30+.", cfg.Tools.BashTimeout),
		})
	}

	return result
}

// validateProvider validates a single provider configuration
func validateProvider(name string, prov provider.ProviderConfig, result *ValidationErrors) {
	prefix := fmt.Sprintf("providers.%s", name)

	// Check type
	if prov.Type == "" {
		result.Errors = append(result.Errors, ValidationError{
			Field:   prefix + ".type",
			Message: "provider type is required. Valid types: openai, anthropic, ollama, cavibora, openai_compatible.",
		})
	} else if !knownProviderTypes[prov.Type] {
		result.Errors = append(result.Errors, ValidationError{
			Field:   prefix + ".type",
			Message: fmt.Sprintf("unknown provider type '%s'. Valid types: openai, anthropic, ollama, cavibora, openai_compatible.", prov.Type),
		})
	}

	// Check model
	if prov.Model == "" {
		result.Errors = append(result.Errors, ValidationError{
			Field:   prefix + ".model",
			Message: "model is required. Example: gpt-4o, claude-3-5-sonnet-20241022, qwen2.5-coder:7b.",
		})
	}

	// Check API key for providers that require it
	if providersRequiringAPIKey[prov.Type] && prov.APIKey == "" {
		result.Errors = append(result.Errors, ValidationError{
			Field:   prefix + ".api_key",
			Message: fmt.Sprintf("API key is required for provider type '%s'. Set it directly or use ${ENV_VAR} syntax.", prov.Type),
		})
	}

	// Check API key with ${} syntax
	if prov.APIKey != "" && strings.HasPrefix(prov.APIKey, "${") && strings.HasSuffix(prov.APIKey, "}") {
		envVar := prov.APIKey[2 : len(prov.APIKey)-1]
		// We can't check if the env var is set at validation time (it might be set later),
		// but we can warn if it looks like a typo
		if strings.Contains(envVar, " ") {
			result.Warnings = append(result.Warnings, ValidationError{
				Field:   prefix + ".api_key",
				Message: fmt.Sprintf("environment variable name '%s' contains spaces — this is likely a mistake.", envVar),
			})
		}
	}

	// Check base_url for ollama (default is localhost:11434)
	if prov.Type == "ollama" && prov.BaseURL == "" && prov.Model != "" {
		// This is fine — default URL will be used
	}

	// Check max_tokens
	if prov.MaxTokens > 0 && prov.MaxTokens < 100 {
		result.Warnings = append(result.Warnings, ValidationError{
			Field:   prefix + ".max_tokens",
			Message: fmt.Sprintf("value %d is very small — model output will be extremely limited. Recommended: 4096+.", prov.MaxTokens),
		})
	}

	// Check context_window
	if prov.ContextWindow > 0 && prov.ContextWindow < 1000 {
		result.Warnings = append(result.Warnings, ValidationError{
			Field:   prefix + ".context_window",
			Message: fmt.Sprintf("value %d is very small — context window may be too limited. Recommended: 8000+.", prov.ContextWindow),
		})
	}

	// Check openai_compatible requires base_url
	if prov.Type == "openai_compatible" && prov.BaseURL == "" {
		result.Errors = append(result.Errors, ValidationError{
			Field:   prefix + ".base_url",
			Message: "base_url is required for openai_compatible provider type.",
		})
	}

	// Check base_url format
	if prov.BaseURL != "" {
		if !strings.HasPrefix(prov.BaseURL, "http://") && !strings.HasPrefix(prov.BaseURL, "https://") {
			result.Errors = append(result.Errors, ValidationError{
				Field:   prefix + ".base_url",
				Message: fmt.Sprintf("base_url '%s' must start with http:// or https://.", prov.BaseURL),
			})
		}
	}

	// Check for common mistakes: wrong field names that YAML would silently ignore
	// (These are caught by yaml.Unmarshal, but we can check for known gotchas)
}

// FormatValidationReport formats validation results as a readable string
func FormatValidationReport(result *ValidationErrors, configPath string) string {
	if !result.HasErrors() && len(result.Warnings) == 0 {
		return ""
	}

	var sb strings.Builder
	if configPath != "" {
		sb.WriteString(fmt.Sprintf("📋 Configuration validation: %s\n", configPath))
	} else {
		sb.WriteString("📋 Configuration validation:\n")
	}

	if len(result.Errors) > 0 {
		sb.WriteString("\n❌ Errors (must fix):\n")
		for _, e := range result.Errors {
			sb.WriteString("  " + e.String() + "\n")
		}
	}

	if len(result.Warnings) > 0 {
		sb.WriteString("\n⚠️  Warnings (should fix):\n")
		for _, w := range result.Warnings {
			sb.WriteString("  " + w.String() + "\n")
		}
	}

	return sb.String()
}