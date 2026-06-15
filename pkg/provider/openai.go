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

// OpenAIProvider — provider for OpenAI Chat Completions API
type OpenAIProvider struct {
	name        string
	apiKey      string
	baseURL     string
	model       string
	maxTokens   int     // max_tokens for API request (0 = do not send)
	temperature float64 // sampling temperature (0 = not sent)
	topP        float64 // top-p sampling (0 = not sent)
	client      *http.Client
	retryPolicy RetryPolicy
}

// NewOpenAIProvider creates provider OpenAI
func NewOpenAIProvider(name string, cfg ProviderConfig) (*OpenAIProvider, error) {
	baseURL := cfg.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	// Remove trailing slash
	baseURL = strings.TrimRight(baseURL, "/")

	if cfg.Model == "" {
		cfg.Model = "gpt-4o"
	}

	return &OpenAIProvider{
		name:        name,
		apiKey:      cfg.APIKey,
		baseURL:     baseURL,
		model:       cfg.Model,
		maxTokens:   cfg.MaxTokens,
		temperature: cfg.Temperature,
		topP:        cfg.TopP,
		retryPolicy: DefaultRetryPolicy(),
		client:      &http.Client{
			// No timeout — for streaming use context with request_timeout
		},
	}, nil
}

// SetRetryPolicy sets retry policy
func (p *OpenAIProvider) SetRetryPolicy(policy RetryPolicy) {
	p.retryPolicy = policy
}

func (p *OpenAIProvider) Name() string { return p.name }

// Complete sends synchronous request to OpenAI with retry
func (p *OpenAIProvider) Complete(messages []Message, tools []ToolDef) (*CompletionResult, error) {
	reqBody := p.buildRequest(messages, tools, false)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, i18n.E("errors_provider.serialize", err)
	}

	var lastErr error
	for attempt := 0; attempt <= p.retryPolicy.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := p.retryPolicy.BackoffDuration(attempt - 1)
			time.Sleep(backoff)
		}

		result, statusCode, err := p.doRequest(body)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if we can retry the request
		if statusCode == 0 || !p.retryPolicy.IsRetryable(statusCode) {
			break
		}
	}

	return nil, lastErr
}

// CompleteWithCtx sends synchronous request with context (for cancellation/timeout)
func (p *OpenAIProvider) CompleteWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error) {
	reqBody := p.buildRequest(messages, tools, false)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, i18n.E("errors_provider.serialize", err)
	}

	var lastErr error
	for attempt := 0; attempt <= p.retryPolicy.MaxRetries; attempt++ {
		// Check context before attempt
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if attempt > 0 {
			backoff := p.retryPolicy.BackoffDuration(attempt - 1)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		result, statusCode, err := p.doRequestWithCtx(ctx, body)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check context after errors
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if statusCode == 0 || !p.retryPolicy.IsRetryable(statusCode) {
			break
		}
	}

	return nil, lastErr
}

// StreamWithCtx sends streaming request with context and retry
func (p *OpenAIProvider) StreamWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	reqBody := p.buildRequest(messages, tools, true)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, i18n.E("errors_provider.serialize", err)
	}

	var lastErr error
	for attempt := 0; attempt <= p.retryPolicy.MaxRetries; attempt++ {
		// Check context before attempt
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if attempt > 0 {
			backoff := p.retryPolicy.BackoffDuration(attempt - 1)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, i18n.E("errors_provider.create_request", err)
		}

		p.setHeaders(req)

		resp, err := p.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = i18n.E("errors_provider.request", "OpenAI", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := ReadFullBody(resp.Body)
			resp.Body.Close()
			statusCode := resp.StatusCode
			lastErr = FormatHTTPError(statusCode, respBody)

			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			if p.retryPolicy.IsRetryable(statusCode) {
				continue
			}
			return nil, lastErr
		}

		ch := make(chan StreamEvent, 100)

		go func() {
			defer close(ch)
			defer resp.Body.Close()

			p.parseStream(resp.Body, ch)
		}()

		return ch, nil
	}

	return nil, lastErr
}

// doRequestWithCtx executes HTTP-request with context
func (p *OpenAIProvider) doRequestWithCtx(ctx context.Context, body []byte) (*CompletionResult, int, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, 0, i18n.E("errors_provider.create_request", err)
	}

	p.setHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, i18n.E("errors_provider.request", "OpenAI", err)
	}
	defer resp.Body.Close()

	respBody, err := ReadFullBody(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, i18n.E("errors_provider.read_response", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, FormatHTTPError(resp.StatusCode, respBody)
	}

	result, err := p.parseResponse(respBody)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return result, resp.StatusCode, nil
}

// doRequest executes a single HTTP request to OpenAI
func (p *OpenAIProvider) doRequest(body []byte) (*CompletionResult, int, error) {
	req, err := http.NewRequest("POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, 0, i18n.E("errors_provider.create_request", err)
	}

	p.setHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, i18n.E("errors_provider.request", "OpenAI", err)
	}
	defer resp.Body.Close()

	respBody, err := ReadFullBody(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, i18n.E("errors_provider.read_response", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, FormatHTTPError(resp.StatusCode, respBody)
	}

	result, err := p.parseResponse(respBody)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return result, resp.StatusCode, nil
}

// Stream sends streaming request to OpenAI with retry
func (p *OpenAIProvider) Stream(messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	reqBody := p.buildRequest(messages, tools, true)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, i18n.E("errors_provider.serialize", err)
	}

	var lastErr error
	for attempt := 0; attempt <= p.retryPolicy.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := p.retryPolicy.BackoffDuration(attempt - 1)
			time.Sleep(backoff)
		}

		req, err := http.NewRequest("POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, i18n.E("errors_provider.create_request", err)
		}

		p.setHeaders(req)

		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = i18n.E("errors_provider.request", "OpenAI", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := ReadFullBody(resp.Body)
			resp.Body.Close()
			statusCode := resp.StatusCode
			lastErr = FormatHTTPError(statusCode, respBody)

			if p.retryPolicy.IsRetryable(statusCode) {
				continue
			}
			return nil, lastErr
		}

		ch := make(chan StreamEvent, 100)

		go func() {
			defer close(ch)
			defer resp.Body.Close()

			p.parseStream(resp.Body, ch)
		}()

		return ch, nil
	}

	return nil, lastErr
}

func (p *OpenAIProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
}

func (p *OpenAIProvider) buildRequest(messages []Message, tools []ToolDef, stream bool) map[string]any {
	// Convert messages to OpenAI format
	var openaiMsgs []map[string]any
	for _, msg := range messages {
		om := p.convertMessage(msg)
		openaiMsgs = append(openaiMsgs, om...)
	}

	req := map[string]any{
		"model":    p.model,
		"messages": openaiMsgs,
	}

	if p.maxTokens > 0 {
		req["max_tokens"] = p.maxTokens
	}

	// Sampling parameters (only if set in config)
	if p.temperature > 0 {
		req["temperature"] = p.temperature
	}
	if p.topP > 0 {
		req["top_p"] = p.topP
	}

	if stream {
		req["stream"] = true
		req["stream_options"] = map[string]any{
			"include_usage": true,
		}
	}

	// Add tools (function calling)
	if len(tools) > 0 {
		var openaiTools []map[string]any
		for _, tool := range tools {
			openaiTools = append(openaiTools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        tool.Name,
					"description": tool.Description,
					"parameters":  tool.Parameters,
				},
			})
		}
		req["tools"] = openaiTools
	}

	return req
}

// convertMessage converts Message to OpenAI format (may return multiple messages)
func (p *OpenAIProvider) convertMessage(msg Message) []map[string]any {
	var result []map[string]any

	if len(msg.Content) == 0 {
		// Plain text
		result = append(result, map[string]any{
			"role":    msg.Role,
			"content": msg.Text,
		})
		return result
	}

	// Check if there is tool_use
	var textParts []map[string]any
	var toolCalls []map[string]any

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, map[string]any{
				"type": "text",
				"text": block.Text,
			})
		case "tool_use":
			// Serialize Input to JSON string for OpenAI/Ollama compatibility
			argsJSON, err := json.Marshal(block.Input)
			if err != nil {
				argsJSON = []byte("{}")
			}
			toolCalls = append(toolCalls, map[string]any{
				"id":   block.ToolUseID,
				"type": "function",
				"function": map[string]any{
					"name":      block.ToolName,
					"arguments": string(argsJSON),
				},
			})
		case "tool_result":
			// OpenAI: role=tool, tool_call_id=...
			content := block.Output
			if block.IsError {
				content = "Error: " + content
			}
			result = append(result, map[string]any{
				"role":         "tool",
				"tool_call_id": block.ToolUseID,
				"content":      content,
			})
		}
	}

	if len(toolCalls) > 0 {
		// Assistant message with tool calls
		entry := map[string]any{
			"role":       "assistant",
			"tool_calls": toolCalls,
		}
		if len(textParts) > 0 {
			entry["content"] = textParts
		}
		result = append([]map[string]any{}, entry)
	} else if len(textParts) > 0 {
		result = append(result, map[string]any{
			"role":    msg.Role,
			"content": textParts,
		})
	}

	return result
}

func (p *OpenAIProvider) parseResponse(body []byte) (*CompletionResult, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Role             string `json:"role"`
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"` // nolint
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, i18n.E("errors_provider.parse_response", "OpenAI", err)
	}

	if len(resp.Choices) == 0 {
		return nil, i18n.E("errors_provider.empty_response", "OpenAI")
	}

	choice := resp.Choices[0]
	msg := choice.Message

	// Convert to Message
	var content []ContentBlock
	// Reasoning/thinking content (o1, o3 models)
	if msg.ReasoningContent != "" {
		content = append(content, ContentBlock{Type: "thinking", Text: msg.ReasoningContent})
	}
	if msg.Content != "" {
		content = append(content, ContentBlock{Type: "text", Text: msg.Content})
	}

	for _, tc := range msg.ToolCalls {
		// Arguments can be either a JSON object or a JSON string
		var input map[string]any
		if len(tc.Function.Arguments) > 0 {
			// Try parsing as JSON object first
			if err := json.Unmarshal(tc.Function.Arguments, &input); err != nil {
				// Try parsing as JSON string (Ollama/Qwen format)
				var argsStr string
				if err2 := json.Unmarshal(tc.Function.Arguments, &argsStr); err2 == nil {
					json.Unmarshal([]byte(argsStr), &input)
				}
			}
		}
		if input == nil {
			input = make(map[string]any)
		}
		content = append(content, ContentBlock{
			Type:      "tool_use",
			ToolUseID: tc.ID,
			ToolName:  tc.Function.Name,
			Input:     input,
		})
	}

	stopReason := "end_turn"
	if choice.FinishReason == "tool_calls" {
		stopReason = "tool_use"
	} else if choice.FinishReason == "length" {
		stopReason = "max_tokens"
	}

	return &CompletionResult{
		Message: Message{
			Role:    msg.Role,
			Content: content,
		},
		StopReason: stopReason,
		Usage: Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}, nil
}

func (p *OpenAIProvider) parseStream(body io.Reader, ch chan<- StreamEvent) {
	var currentToolID, currentToolName string
	var toolInputBuf strings.Builder

	_ = ExtractJSONFromSSE(body, func(jsonStr string) error {
		var chunk struct {
			Choices []struct {
				Delta struct {
					Role             string `json:"role"`
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						ID       string `json:"id"`
						Index    int    `json:"index"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}

		if err := json.Unmarshal([]byte(jsonStr), &chunk); err != nil {
			return nil // Skip invalid JSON
		}

		// Usage data (only present in the final chunk when stream_options.include_usage=true)
		if chunk.Usage != nil {
			ch <- StreamEvent{
				Type:         EventUsage,
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
		}

		if len(chunk.Choices) == 0 {
			return nil
		}

		delta := chunk.Choices[0].Delta

		// Reasoning/thinking content (o1, o3, DeepSeek etc.)
		if delta.ReasoningContent != "" {
			ch <- StreamEvent{Type: EventThinking, Text: delta.ReasoningContent}
		}

		// Text content
		if delta.Content != "" {
			ch <- StreamEvent{Type: "text_delta", Text: delta.Content}
		}

		// Tool calls
		for _, tc := range delta.ToolCalls {
			if tc.ID != "" {
				// New tool call
				if currentToolID != "" {
					// Close previous
					ch <- StreamEvent{
						Type:       "tool_call_end",
						ToolCallID: currentToolID,
						ToolName:   currentToolName,
					}
				}
				currentToolID = tc.ID
				currentToolName = tc.Function.Name
				toolInputBuf.Reset()
				ch <- StreamEvent{
					Type:       "tool_call_start",
					ToolCallID: tc.ID,
					ToolName:   tc.Function.Name,
				}
			}
			if tc.Function.Arguments != "" {
				toolInputBuf.WriteString(tc.Function.Arguments)
				ch <- StreamEvent{
					Type:      "tool_call_delta",
					ToolDelta: tc.Function.Arguments,
				}
			}
		}

		// End of stream
		if chunk.Choices[0].FinishReason != nil {
			reason := *chunk.Choices[0].FinishReason
			if reason == "tool_calls" {
				// Close last tool call
				if currentToolID != "" {
					ch <- StreamEvent{
						Type:       "tool_call_end",
						ToolCallID: currentToolID,
						ToolName:   currentToolName,
					}
				}
			}
			ch <- StreamEvent{Type: "done"}
		}

		return nil
	})
}
