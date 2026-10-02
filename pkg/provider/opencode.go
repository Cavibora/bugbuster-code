package provider

import (
	"context"
	"strings"
)

// OpenCodeProvider — provider for OpenCode Go (Zen Go) API.
//
// OpenCode Go is an OpenAI-compatible endpoint at https://opencode.ai/zen/go/v1
// that provides access to open coding models (GLM, Kimi, DeepSeek, Qwen, MiMo, etc.).
//
// Requirements from https://opencode.ai/docs/go/#where-can-i-use-it:
//   - Send typical coding agent traffic
//   - Identify with a coding-agent User-Agent (not a generic SDK name)
//   - Send a stable session ID in the x-opencode-session header
//
// Model ID format: opencode-go/<model-id>
//   e.g. opencode-go/kimi-k3, opencode-go/glm-5.3-flash
type OpenCodeProvider struct {
	delegate  *OpenAIProvider
	sessionID string
}

// NewOpenCodeProvider creates a provider for OpenCode Go.
//
// cfg.BaseURL is optional — defaults to https://opencode.ai/zen/go/v1.
// cfg.Model must use the opencode-go/<model-id> format.
func NewOpenCodeProvider(name string, cfg ProviderConfig) (*OpenCodeProvider, error) {
	// Default base URL for OpenCode Go
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://opencode.ai/zen/go/v1"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	if cfg.Model == "" {
		cfg.Model = "opencode-go/glm-5.3-flash"
	}

	delegate, err := NewOpenAIProvider(name, cfg)
	if err != nil {
		return nil, err
	}

	return &OpenCodeProvider{
		delegate:  delegate,
		sessionID: "", // set later via SetSessionID
	}, nil
}

// SetSessionID sets the stable session ID sent in the x-opencode-session header.
// OpenCode Go uses this for routing optimization and prompt caching.
func (p *OpenCodeProvider) SetSessionID(id string) {
	p.sessionID = id
	if p.delegate != nil {
		p.delegate.SetSessionID(id)
	}
}

func (p *OpenCodeProvider) Name() string { return p.delegate.Name() }
func (p *OpenCodeProvider) Model() string { return p.delegate.Model() }

func (p *OpenCodeProvider) Complete(messages []Message, tools []ToolDef) (*CompletionResult, error) {
	return p.delegate.Complete(messages, tools)
}

func (p *OpenCodeProvider) Stream(messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	return p.delegate.Stream(messages, tools)
}

func (p *OpenCodeProvider) CompleteWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error) {
	return p.delegate.CompleteWithCtx(ctx, messages, tools)
}

func (p *OpenCodeProvider) StreamWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	return p.delegate.StreamWithCtx(ctx, messages, tools)
}
