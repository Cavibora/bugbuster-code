package provider

import (
	"bugbuster-code/pkg/i18n"
)

// NewFromConfig creates provider from configuration
func NewFromConfig(name string, cfg ProviderConfig) (Provider, error) {
	switch cfg.Type {
	case "openai":
		return NewOpenAIProvider(name, cfg)
	case "anthropic":
		return NewAnthropicProvider(name, cfg)
	case "ollama":
		return NewOllamaProvider(name, cfg)
	case "cavibora":
		return NewCaviboraProvider(name, cfg)
	case "openai_compat":
		return NewOpenAICompatProvider(name, cfg)
	default:
		return nil, i18n.E("errors_provider.unknown_type", cfg.Type)
	}
}

// NewDefaultProvider creates default provider from BugBuster configuration
func NewDefaultProvider(providers map[string]ProviderConfig, defaultName string) (Provider, error) {
	if defaultName == "" {
		// Take first available provider
		for name, cfg := range providers {
			return NewFromConfig(name, cfg)
		}
		return nil, i18n.E("errors_provider.no_providers")
	}

	cfg, ok := providers[defaultName]
	if !ok {
		return nil, i18n.E("errors_provider.not_found", defaultName)
	}

	return NewFromConfig(defaultName, cfg)
}
