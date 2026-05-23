package provider

import (
	"bugbuster-code/pkg/i18n"
	"context"
	"strings"
)

// OpenAICompatProvider is a provider for OpenAI-compatible APIs (vLLM, LM Studio, LocalAI, etc.)
// Delegates to OpenAIProvider with custom base_url
type OpenAICompatProvider struct {
	delegate *OpenAIProvider
}

// NewOpenAICompatProvider creates a provider for OpenAI-compatible API
func NewOpenAICompatProvider(name string, cfg ProviderConfig) (*OpenAICompatProvider, error) {
	if cfg.BaseURL == "" {
		return nil, i18n.E("errors_provider.base_url_required")
	}

	// Remove trailing slash
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	if cfg.Model == "" {
		cfg.Model = "default"
	}

	delegate, err := NewOpenAIProvider(name, cfg)
	if err != nil {
		return nil, err
	}

	return &OpenAICompatProvider{delegate: delegate}, nil
}

func (p *OpenAICompatProvider) Name() string { return p.delegate.Name() }

func (p *OpenAICompatProvider) Complete(messages []Message, tools []ToolDef) (*CompletionResult, error) {
	return p.delegate.Complete(messages, tools)
}

func (p *OpenAICompatProvider) Stream(messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	return p.delegate.Stream(messages, tools)
}

// CompleteWithCtx sends a request with context
func (p *OpenAICompatProvider) CompleteWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error) {
	return CompleteWithCtxDefault(p, ctx, messages, tools)
}

// StreamWithCtx sends a streaming request with context
func (p *OpenAICompatProvider) StreamWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	return StreamWithCtxDefault(p, ctx, messages, tools)
}
