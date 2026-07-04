package config

import (
	"strings"
	"testing"

	"bugbuster-code/pkg/provider"
)

func TestValidateConfig_EmptyDefaultProvider(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "",
		Providers:       map[string]provider.ProviderConfig{},
	}
	result := ValidateConfig(cfg)
	if !result.HasErrors() {
		t.Error("Expected errors for empty default_provider")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "default_provider") {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected error about default_provider, got: %v", result.Errors)
	}
}

func TestValidateConfig_UnknownDefaultProvider(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "nonexistent",
		Providers: map[string]provider.ProviderConfig{
			"ollama": {Type: "ollama", Model: "qwen2.5-coder:7b"},
		},
	}
	result := ValidateConfig(cfg)
	if !result.HasErrors() {
		t.Error("Expected errors for unknown default_provider")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "default_provider") && strings.Contains(e.Message, "nonexistent") {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected error about nonexistent provider, got: %v", result.Errors)
	}
}

func TestValidateConfig_MissingProviderType(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "myprovider",
		Providers: map[string]provider.ProviderConfig{
			"myprovider": {Model: "gpt-4o", APIKey: "sk-xxx"},
		},
	}
	result := ValidateConfig(cfg)
	if !result.HasErrors() {
		t.Error("Expected errors for missing provider type")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "type") {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected error about missing type, got: %v", result.Errors)
	}
}

func TestValidateConfig_UnknownProviderType(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "myprovider",
		Providers: map[string]provider.ProviderConfig{
			"myprovider": {Type: "unknown_type", Model: "gpt-4o"},
		},
	}
	result := ValidateConfig(cfg)
	if !result.HasErrors() {
		t.Error("Expected errors for unknown provider type")
	}
}

func TestValidateConfig_MissingModel(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "myprovider",
		Providers: map[string]provider.ProviderConfig{
			"myprovider": {Type: "openai", APIKey: "sk-xxx"},
		},
	}
	result := ValidateConfig(cfg)
	if !result.HasErrors() {
		t.Error("Expected errors for missing model")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "model") {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected error about missing model, got: %v", result.Errors)
	}
}

func TestValidateConfig_MissingAPIKey(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "myprovider",
		Providers: map[string]provider.ProviderConfig{
			"myprovider": {Type: "openai", Model: "gpt-4o"},
		},
	}
	result := ValidateConfig(cfg)
	if !result.HasErrors() {
		t.Error("Expected errors for missing API key")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "api_key") {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected error about missing api_key, got: %v", result.Errors)
	}
}

func TestValidateConfig_OpenAICompatibleMissingBaseURL(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "myprovider",
		Providers: map[string]provider.ProviderConfig{
			"myprovider": {Type: "openai_compatible", Model: "my-model", APIKey: "xxx"},
		},
	}
	result := ValidateConfig(cfg)
	if !result.HasErrors() {
		t.Error("Expected errors for missing base_url on openai_compatible")
	}
}

func TestValidateConfig_InvalidBaseURL(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "myprovider",
		Providers: map[string]provider.ProviderConfig{
			"myprovider": {Type: "openai", Model: "gpt-4o", APIKey: "sk-xxx", BaseURL: "ftp://bad.url"},
		},
	}
	result := ValidateConfig(cfg)
	if !result.HasErrors() {
		t.Error("Expected errors for invalid base_url")
	}
}

func TestValidateConfig_OllamaNoAPIKey(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "ollama",
		Providers: map[string]provider.ProviderConfig{
			"ollama": {Type: "ollama", Model: "qwen2.5-coder:7b"},
		},
	}
	result := ValidateConfig(cfg)
	// Ollama doesn't require API key — should have no errors about api_key
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "api_key") {
			t.Errorf("Ollama should not require API key, got: %s", e)
		}
	}
}

func TestValidateConfig_ValidConfig(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "openai",
		Providers: map[string]provider.ProviderConfig{
			"openai": {Type: "openai", Model: "gpt-4o", APIKey: "sk-xxx"},
		},
	}
	result := ValidateConfig(cfg)
	if result.HasErrors() {
		t.Errorf("Valid config should have no errors, got: %v", result.Errors)
	}
}

func TestValidateConfig_AgentProviderReference(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "openai",
		Providers: map[string]provider.ProviderConfig{
			"openai": {Type: "openai", Model: "gpt-4o", APIKey: "sk-xxx"},
		},
		AgentProviders: map[string]string{
			"code_review": "nonexistent",
		},
	}
	result := ValidateConfig(cfg)
	if !result.HasErrors() {
		t.Error("Expected errors for nonexistent agent_provider reference")
	}
}

func TestValidateConfig_SmallMaxTokens(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "ollama",
		Providers: map[string]provider.ProviderConfig{
			"ollama": {Type: "ollama", Model: "qwen2.5-coder:7b"},
		},
		Agent: AgentConfig{MaxTokens: 100},
	}
	result := ValidateConfig(cfg)
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w.Field, "agent.max_tokens") {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected warning about small max_tokens, got: %v", result.Warnings)
	}
}

func TestValidateConfig_LargeMaxTokens(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "ollama",
		Providers: map[string]provider.ProviderConfig{
			"ollama": {Type: "ollama", Model: "qwen2.5-coder:7b"},
		},
		Agent: AgentConfig{MaxTokens: 2000000},
	}
	result := ValidateConfig(cfg)
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w.Field, "agent.max_tokens") {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected warning about large max_tokens, got: %v", result.Warnings)
	}
}

func TestValidateConfig_MCPServerValidation(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "ollama",
		Providers: map[string]provider.ProviderConfig{
			"ollama": {Type: "ollama", Model: "qwen2.5-coder:7b"},
		},
		MCP: MCPConfigSection{
			Servers: map[string]MCPServerConfig{
				"bad": {Type: "stdio", Command: ""}, // missing command
			},
		},
	}
	result := ValidateConfig(cfg)
	if !result.HasErrors() {
		t.Error("Expected errors for MCP server without command")
	}
}

func TestFormatValidationReport(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "nonexistent",
		Providers:       map[string]provider.ProviderConfig{},
	}
	result := ValidateConfig(cfg)
	report := FormatValidationReport(result, ".bugbuster.yaml")
	if !strings.Contains(report, "❌") {
		t.Errorf("Report should contain error markers, got: %s", report)
	}
	if !strings.Contains(report, ".bugbuster.yaml") {
		t.Errorf("Report should contain config path, got: %s", report)
	}
}

func TestValidateConfig_NoProviders(t *testing.T) {
	cfg := &BugBusterConfig{
		DefaultProvider: "myprovider",
		Providers:       map[string]provider.ProviderConfig{},
	}
	result := ValidateConfig(cfg)
	if !result.HasErrors() {
		t.Error("Expected errors when default_provider references nonexistent provider with empty providers map")
	}
}