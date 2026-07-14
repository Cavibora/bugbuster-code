package provider

import (
	"bugbuster-code/pkg/i18n"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// CaviboraProvider is a provider for Cavibora (OpenAI-compatible API)
type CaviboraProvider struct {
	name     string
	baseURL  string
	apiKey   string
	model    string
	client   *http.Client
	delegate *OpenAIProvider
}

// NewCaviboraProvider creates provider Cavibora
func NewCaviboraProvider(name string, cfg ProviderConfig) (*CaviboraProvider, error) {
	baseURL := cfg.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.cavibora.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	if cfg.Model == "" {
		cfg.Model = "cavibora-v1"
	}

	// Cavibora uses OpenAI-compatible API
	openaiCfg := ProviderConfig{
		Type:        "openai",
		BaseURL:     baseURL + "/v1",
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		MaxTokens:   cfg.MaxTokens,
		Temperature: cfg.Temperature,
		TopP:        cfg.TopP,
	}

	delegate, err := NewOpenAIProvider(name, openaiCfg)
	if err != nil {
		return nil, err
	}

	return &CaviboraProvider{
		name:     name,
		baseURL:  baseURL,
		apiKey:   cfg.APIKey,
		model:    cfg.Model,
		client:   &http.Client{Timeout: 10 * time.Minute}, // Fallback timeout; for streaming we use context
		delegate: delegate,
	}, nil
}

func (p *CaviboraProvider) Name() string { return p.name }
func (p *CaviboraProvider) Model() string { return p.model }

// Complete delegates to OpenAI-compatible API
func (p *CaviboraProvider) Complete(messages []Message, tools []ToolDef) (*CompletionResult, error) {
	return p.delegate.Complete(messages, tools)
}

// Stream delegates to OpenAI-compatible API
func (p *CaviboraProvider) Stream(messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	return p.delegate.Stream(messages, tools)
}

// Teach sends a training request (Cavibora-specific)
func (p *CaviboraProvider) Teach(input, output string) error {
	reqBody := map[string]any{
		"input":  input,
		"output": output,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return i18n.E("errors_provider.serialize", err)
	}

	req, err := http.NewRequest("POST", p.baseURL+"/v1/teach", bytes.NewReader(body))
	if err != nil {
		return i18n.E("errors_provider.create_request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return i18n.E("errors_provider.request", "Cavibora", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := ReadFullBody(resp.Body)
		return FormatHTTPError(resp.StatusCode, respBody)
	}

	return nil
}

// DreamResult contains the result of a dream (memory consolidation) request
type DreamResult struct {
	Seed        string `json:"seed"`
	Thoughts    []string `json:"thoughts"`
	NewBindings int    `json:"new_bindings"`
	Duration    string `json:"duration"`
}

// EmotionResult contains the emotional state of the model
type EmotionResult struct {
	Emotion    string  `json:"emotion"`
	Emoji      string  `json:"emoji"`
	Intensity  float64 `json:"intensity"`
	Bar        string  `json:"bar"`
	Detail     string  `json:"detail"`
}

// MeshStatsResult contains the mesh network statistics
type MeshStatsResult struct {
	Cells       int     `json:"cells"`
	Bindings    int     `json:"bindings"`
	Learnings   int     `json:"learnings"`
	Uptime      string  `json:"uptime"`
	ModelName   string  `json:"model_name"`
	Version     string  `json:"version"`
	Temperature float64 `json:"temperature"`
}

// Dream triggers memory consolidation (Cavibora-specific)
// Sends a POST request to /dream endpoint with an optional seed
func (p *CaviboraProvider) Dream(ctx context.Context, seed string) (*DreamResult, error) {
	reqBody := map[string]any{}
	if seed != "" {
		reqBody["seed"] = seed
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, i18n.E("errors_provider.serialize", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/dream", bytes.NewReader(body))
	if err != nil {
		return nil, i18n.E("errors_provider.create_request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, i18n.E("errors_provider.request", "Cavibora", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := ReadFullBody(resp.Body)
		return nil, FormatHTTPError(resp.StatusCode, respBody)
	}

	var result DreamResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, i18n.E("errors_provider.parse_response", err)
	}

	return &result, nil
}

// Emotions returns the current emotional state of the model (Cavibora-specific)
func (p *CaviboraProvider) Emotions(ctx context.Context) (*EmotionResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/stats", nil)
	if err != nil {
		return nil, i18n.E("errors_provider.create_request", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, i18n.E("errors_provider.request", "Cavibora", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := ReadFullBody(resp.Body)
		return nil, FormatHTTPError(resp.StatusCode, respBody)
	}

	var result EmotionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, i18n.E("errors_provider.parse_response", err)
	}

	return &result, nil
}

// MeshStats returns the mesh network statistics (Cavibora-specific)
func (p *CaviboraProvider) MeshStats(ctx context.Context) (*MeshStatsResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/stats", nil)
	if err != nil {
		return nil, i18n.E("errors_provider.create_request", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, i18n.E("errors_provider.request", "Cavibora", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := ReadFullBody(resp.Body)
		return nil, FormatHTTPError(resp.StatusCode, respBody)
	}

	var result MeshStatsResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, i18n.E("errors_provider.parse_response", err)
	}

	return &result, nil
}

// CompleteWithCtx sends request with context
func (p *CaviboraProvider) CompleteWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error) {
	return CompleteWithCtxDefault(p, ctx, messages, tools)
}

// StreamWithCtx sends streaming request with context
func (p *CaviboraProvider) StreamWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	return StreamWithCtxDefault(p, ctx, messages, tools)
}
