package provider

import (
	"bugbuster-code/pkg/i18n"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaProvider — provider for Ollama (OpenAI-compatible API)
type OllamaProvider struct {
	name          string
	baseURL       string
	model         string
	maxTokens     int     // num_predict (0 = default 4096)
	contextWindow int     // num_ctx (0 = default 4096)
	temperature   float64 // sampling temperature (0 = not sent)
	topP          float64 // top-p sampling (0 = not sent)
	topK          int     // top-k sampling (0 = not sent)
	client        *http.Client
	// Delegate to OpenAI-compatible API
	delegate *OpenAIProvider
}

// NewOllamaProvider creates provider Ollama
func NewOllamaProvider(name string, cfg ProviderConfig) (*OllamaProvider, error) {
	baseURL := cfg.GetBaseURL()
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	if cfg.Model == "" {
		cfg.Model = "llama3"
	}

	// Ollama uses OpenAI-compatible API at /v1
	openaiCfg := ProviderConfig{
		Type:        "openai",
		BaseURL:     baseURL + "/v1",
		Model:       cfg.Model,
		MaxTokens:   cfg.MaxTokens,
		Temperature: cfg.Temperature,
		TopP:        cfg.TopP,
	}

	delegate, err := NewOpenAIProvider(name, openaiCfg)
	if err != nil {
		return nil, err
	}

	return &OllamaProvider{
		name:          name,
		baseURL:       baseURL,
		model:         cfg.Model,
		maxTokens:     cfg.MaxTokens,
		contextWindow: cfg.ContextWindow,
		temperature:   cfg.Temperature,
		topP:          cfg.TopP,
		topK:          cfg.TopK,
		client:        &http.Client{Timeout: 10 * time.Minute}, // Fallback timeout; for streaming we use context
		delegate:      delegate,
	}, nil
}

func (p *OllamaProvider) Name() string { return p.name }

// Complete sends synchronous request to Ollama
func (p *OllamaProvider) Complete(messages []Message, tools []ToolDef) (*CompletionResult, error) {
	// First try OpenAI-compatible API
	result, err := p.delegate.Complete(messages, tools)
	if err == nil {
		return result, nil
	}

	// Fallback to native Ollama API (/api/chat)
	return p.completeNative(messages, tools)
}

// Stream sends streaming request to Ollama
func (p *OllamaProvider) Stream(messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	// First try OpenAI-compatible API
	ch, err := p.delegate.Stream(messages, tools)
	if err == nil {
		return ch, nil
	}

	// Fallback to native Ollama API
	return p.streamNative(messages, tools)
}

// completeNative uses native Ollama API (/api/chat)
func (p *OllamaProvider) completeNative(messages []Message, tools []ToolDef) (*CompletionResult, error) {
	reqBody := p.buildNativeRequest(messages, tools, false)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, i18n.E("errors_provider.serialize", err)
	}

	req, err := http.NewRequest("POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, i18n.E("errors_provider.create_request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, i18n.E("errors_provider.request", "Ollama", err)
	}
	defer resp.Body.Close()

	respBody, err := ReadFullBody(resp.Body)
	if err != nil {
		return nil, i18n.E("errors_provider.read_response", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, FormatHTTPError(resp.StatusCode, respBody)
	}

	return p.parseNativeResponse(respBody)
}

// streamNative uses native Ollama API with streaming output
func (p *OllamaProvider) streamNative(messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	reqBody := p.buildNativeRequest(messages, tools, true)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, i18n.E("errors_provider.serialize", err)
	}

	req, err := http.NewRequest("POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, i18n.E("errors_provider.create_request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, i18n.E("errors_provider.request", "Ollama", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := ReadFullBody(resp.Body)
		resp.Body.Close()
		return nil, FormatHTTPError(resp.StatusCode, respBody)
	}

	ch := make(chan StreamEvent, 100)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		p.parseNativeStream(resp.Body, ch)
	}()

	return ch, nil
}

func (p *OllamaProvider) buildNativeRequest(messages []Message, tools []ToolDef, stream bool) map[string]any {
	var ollamaMsgs []map[string]any
	for _, msg := range messages {
		ollamaMsgs = append(ollamaMsgs, map[string]any{
			"role":    msg.Role,
			"content": msg.GetText(),
		})
	}

	numPredict := p.maxTokens
	if numPredict <= 0 {
		numPredict = 16384
	}

	options := map[string]any{
		"num_predict": numPredict,
	}
	if p.contextWindow > 0 {
		options["num_ctx"] = p.contextWindow
	}
	// Sampling parameters (only if set in config)
	if p.temperature > 0 {
		options["temperature"] = p.temperature
	}
	if p.topP > 0 {
		options["top_p"] = p.topP
	}
	if p.topK > 0 {
		options["top_k"] = p.topK
	}

	req := map[string]any{
		"model":    p.model,
		"messages": ollamaMsgs,
		"stream":   stream,
		"options":  options,
	}

	// Add tools (Ollama supports tools since version 0.2.x)
	if len(tools) > 0 {
		var ollamaTools []map[string]any
		for _, tool := range tools {
			ollamaTools = append(ollamaTools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        tool.Name,
					"description": tool.Description,
					"parameters":  tool.Parameters,
				},
			})
		}
		req["tools"] = ollamaTools
	}

	return req
}

func (p *OllamaProvider) parseNativeResponse(body []byte) (*CompletionResult, error) {
	var resp struct {
		Message struct {
			Role     string `json:"role"`
			Content  string `json:"content"`
			Thinking string `json:"thinking"`
		} `json:"message"`
		Done bool `json:"done"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, i18n.E("errors_provider.parse_response", "Ollama", err)
	}

	var content []ContentBlock
	if resp.Message.Thinking != "" {
		content = append(content, ContentBlock{Type: "thinking", Text: resp.Message.Thinking})
	}
	content = append(content, ContentBlock{Type: "text", Text: resp.Message.Content})

	return &CompletionResult{
		Message: Message{
			Role:    "assistant",
			Content: content,
		},
		StopReason: "end_turn",
	}, nil
}

func (p *OllamaProvider) parseNativeStream(body io.Reader, ch chan<- StreamEvent) {
	scanner := json.NewDecoder(body)

	for scanner.More() {
		var chunk struct {
			Message struct {
				Role     string `json:"role"`
				Content  string `json:"content"`
				Thinking string `json:"thinking"`
			} `json:"message"`
			Done bool `json:"done"`
		}

		if err := scanner.Decode(&chunk); err != nil {
			ch <- StreamEvent{Type: "error", Error: err}
			return
		}

		// Thinking content (reasoning models: qwen3, deepseek-r1 etc.)
		if chunk.Message.Thinking != "" {
			ch <- StreamEvent{Type: EventThinking, Text: chunk.Message.Thinking}
		}

		if chunk.Message.Content != "" {
			ch <- StreamEvent{Type: "text_delta", Text: chunk.Message.Content}
		}

		if chunk.Done {
			ch <- StreamEvent{Type: "done"}
			return
		}
	}
}

// CompleteWithCtx sends request with context
func (p *OllamaProvider) CompleteWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error) {
	// First try OpenAI-compatible API with context
	result, err := p.delegate.CompleteWithCtx(ctx, messages, tools)
	if err == nil {
		return result, nil
	}

	// Fallback to native Ollama API with context
	return p.completeNativeWithCtx(ctx, messages, tools)
}

// completeNativeWithCtx uses native Ollama API with context
func (p *OllamaProvider) completeNativeWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error) {
	reqBody := p.buildNativeRequest(messages, tools, false)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, i18n.E("errors_provider.serialize", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, i18n.E("errors_provider.create_request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, i18n.E("errors_provider.request", "Ollama", err)
	}
	defer resp.Body.Close()

	respBody, err := ReadFullBody(resp.Body)
	if err != nil {
		return nil, i18n.E("errors_provider.read_response", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, FormatHTTPError(resp.StatusCode, respBody)
	}

	return p.parseNativeResponse(respBody)
}

// StreamWithCtx sends streaming request with context
func (p *OllamaProvider) StreamWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	// First try OpenAI-compatible API with context
	ch, err := p.delegate.StreamWithCtx(ctx, messages, tools)
	if err == nil {
		return ch, nil
	}

	// Fallback to native Ollama API with context
	return p.streamNativeWithCtx(ctx, messages, tools)
}

// streamNativeWithCtx uses native Ollama API with streaming output and context
func (p *OllamaProvider) streamNativeWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	reqBody := p.buildNativeRequest(messages, tools, true)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, i18n.E("errors_provider.serialize", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, i18n.E("errors_provider.create_request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, i18n.E("errors_provider.request", "Ollama", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := ReadFullBody(resp.Body)
		resp.Body.Close()
		return nil, FormatHTTPError(resp.StatusCode, respBody)
	}

	ch := make(chan StreamEvent, 100)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		p.parseNativeStream(resp.Body, ch)
	}()

	return ch, nil
}
