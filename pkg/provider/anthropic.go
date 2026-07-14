package provider

import (
	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/logger"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AnthropicProvider is a provider for Anthropic Messages API
type AnthropicProvider struct {
	name         string
	apiKey       string
	baseURL      string
	model        string
	maxTokens    int     // max_tokens for API request (0 = default 4096)
	budgetTokens int     // budget_tokens for thinking (0 = default 4096)
	temperature  float64 // sampling temperature (0 = not sent)
	topP         float64 // top-p sampling (0 = not sent)
	topK         int     // top-k sampling (0 = not sent)
	client       *http.Client
	retryPolicy  RetryPolicy
}

// NewAnthropicProvider creates provider Anthropic
func NewAnthropicProvider(name string, cfg ProviderConfig) (*AnthropicProvider, error) {
	baseURL := cfg.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	if cfg.Model == "" {
		cfg.Model = "claude-sonnet-4-20250514"
	}

	return &AnthropicProvider{
		name:         name,
		apiKey:       cfg.APIKey,
		baseURL:      baseURL,
		model:        cfg.Model,
		maxTokens:    cfg.MaxTokens,
		budgetTokens: cfg.BudgetTokens,
		temperature:  cfg.Temperature,
		topP:         cfg.TopP,
		topK:         cfg.TopK,
		retryPolicy:  DefaultRetryPolicy(),
		client:       &http.Client{
			// No timeout — for streaming use context with request_timeout
			// (1200s default). HTTP timeout would break long streams.
		},
	}, nil
}

func (p *AnthropicProvider) Name() string { return p.name }
func (p *AnthropicProvider) Model() string { return p.model }

// Complete sends synchronous request to Anthropic with retry
func (p *AnthropicProvider) Complete(messages []Message, tools []ToolDef) (*CompletionResult, error) {
	reqBody := p.buildRequest(messages, tools, false)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, i18n.E("errors_provider.serialize", err)
	}

	var lastErr error
	for attempt := 0; attempt <= p.retryPolicy.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := p.retryPolicy.BackoffDuration(attempt - 1)
			logger.Debug("retry", "provider", "anthropic", "attempt", attempt, "backoff", backoff)
			time.Sleep(backoff)
		}

		result, statusCode, err := p.doCompleteRequest(body)
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

// doCompleteRequest executes a single HTTP request to Anthropic
func (p *AnthropicProvider) doCompleteRequest(body []byte) (*CompletionResult, int, error) {
	req, err := http.NewRequest("POST", p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, 0, i18n.E("errors_provider.create_request", err)
	}

	p.setHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, i18n.E("errors_provider.request", "Anthropic", err)
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

// Stream sends streaming request to Anthropic with retry
func (p *AnthropicProvider) Stream(messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	reqBody := p.buildRequest(messages, tools, true)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, i18n.E("errors_provider.serialize", err)
	}

	var lastErr error
	for attempt := 0; attempt <= p.retryPolicy.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := p.retryPolicy.BackoffDuration(attempt - 1)
			logger.Debug("retry stream", "provider", "anthropic", "attempt", attempt, "backoff", backoff)
			time.Sleep(backoff)
		}

		// Create a new reader for each attempt
		req, err := http.NewRequest("POST", p.baseURL+"/v1/messages", bytes.NewReader(body))
		if err != nil {
			return nil, i18n.E("errors_provider.create_request", err)
		}

		p.setHeaders(req)

		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = i18n.E("errors_provider.request", "Anthropic", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := ReadFullBody(resp.Body)
			resp.Body.Close()
			statusCode := resp.StatusCode
			lastErr = FormatHTTPError(statusCode, respBody)

			// Check if we can retry the request
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

// StreamWithCtx sends streaming request with context (for cancellation/timeout)
func (p *AnthropicProvider) StreamWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
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

		req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/messages", bytes.NewReader(body))
		if err != nil {
			return nil, i18n.E("errors_provider.create_request", err)
		}

		p.setHeaders(req)

		resp, err := p.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = i18n.E("errors_provider.request", "Anthropic", err)
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

func (p *AnthropicProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("User-Agent", UserAgent)
	if p.apiKey != "" {
		req.Header.Set("x-api-key", p.apiKey)
	}
}

func (p *AnthropicProvider) buildRequest(messages []Message, tools []ToolDef, stream bool) map[string]any {
	// Anthropic: system message — separate top-level parameter
	var systemText string
	var chatMsgs []map[string]any

	for _, msg := range messages {
		if msg.Role == "system" {
			systemText += msg.GetText() + "\n"
			continue
		}
		chatMsgs = append(chatMsgs, p.convertMessage(msg)...)
	}

	maxTokens := p.maxTokens
	if maxTokens <= 0 {
		maxTokens = 16384
	}

	req := map[string]any{
		"model":      p.model,
		"messages":   chatMsgs,
		"max_tokens": maxTokens,
	}

	if systemText != "" {
		req["system"] = strings.TrimSpace(systemText)
	}

	if stream {
		req["stream"] = true
	}

	// Tools
	if len(tools) > 0 {
		var anthropicTools []map[string]any
		for _, tool := range tools {
			anthropicTools = append(anthropicTools, map[string]any{
				"name":         tool.Name,
				"description":  tool.Description,
				"input_schema": tool.Parameters,
			})
		}
		req["tools"] = anthropicTools
	}

	// Extended thinking — request thinking blocks
	// Anthropic API requires budget_tokens to enable thinking
	// Note: some proxies (z.ai etc.) may not support this parameter
	// and return an error. In that case, remove the parameter from config.
	budgetTokens := p.budgetTokens
	if budgetTokens <= 0 {
		budgetTokens = 4096
	}
	req["thinking"] = map[string]any{
		"type":          "enabled",
		"budget_tokens": budgetTokens,
	}

	// Sampling parameters (only if set in config)
	if p.temperature > 0 {
		req["temperature"] = p.temperature
	}
	if p.topP > 0 {
		req["top_p"] = p.topP
	}
	if p.topK > 0 {
		req["top_k"] = p.topK
	}

	return req
}

func (p *AnthropicProvider) convertMessage(msg Message) []map[string]any {
	var result []map[string]any

	if len(msg.Content) == 0 {
		result = append(result, map[string]any{
			"role":    msg.Role,
			"content": msg.Text,
		})
		return result
	}

	var content []map[string]any

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			content = append(content, map[string]any{
				"type": "text",
				"text": block.Text,
			})
		case "thinking":
			content = append(content, map[string]any{
				"type": "thinking",
				"text": block.Text,
			})
		case "image":
			// Anthropic image format: {type: "image", source: {type: "base64", media_type: "...", data: "..."}}
			mediaType := "image/png"
			switch block.ImageFormat {
			case "jpeg", "jpg":
				mediaType = "image/jpeg"
			case "gif":
				mediaType = "image/gif"
			case "webp":
				mediaType = "image/webp"
			}
			content = append(content, map[string]any{
				"type": "image",
				"source": map[string]any{
					"type":       "base64",
					"media_type": mediaType,
					"data":       block.ImageSource,
				},
			})
		case "tool_use":
			content = append(content, map[string]any{
				"type":  "tool_use",
				"id":    block.ToolUseID,
				"name":  block.ToolName,
				"input": block.Input,
			})
		case "tool_result":
			entry := map[string]any{
				"type":        "tool_result",
				"tool_use_id": block.ToolUseID,
			}
			if block.IsError {
				entry["is_error"] = true
			}
			entry["content"] = block.Output
			content = append(content, entry)
		}
	}

	role := msg.Role
	// Anthropic: tool_result goes in role=user
	if msg.Role == "tool" {
		role = "user"
	}

	result = append(result, map[string]any{
		"role":    role,
		"content": content,
	})

	return result
}

func (p *AnthropicProvider) parseResponse(body []byte) (*CompletionResult, error) {
	var resp struct {
		Content []struct {
			Type             string          `json:"type"`
			Text             string          `json:"text"`
			ID               string          `json:"id"`
			Name             string          `json:"name"`
			Input            json.RawMessage `json:"input"`
			ReasoningContent string          `json:"reasoning_content"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, i18n.E("errors_provider.parse_response", "Anthropic", err)
	}

	var content []ContentBlock
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			content = append(content, ContentBlock{Type: "text", Text: block.Text})
		case "thinking":
			content = append(content, ContentBlock{Type: "thinking", Text: block.Text})
		case "tool_use":
			var input map[string]any
			if err := json.Unmarshal(block.Input, &input); err != nil {
				// Instead of empty map — add parsing error info,
				// so the agent loop can return a clear error to the model
				input = map[string]any{
					"_parse_error": fmt.Sprintf("tool call parameters could not be parsed as JSON: %v", err),
					"_raw_input":   string(block.Input),
				}
			}
			content = append(content, ContentBlock{
				Type:      "tool_use",
				ToolUseID: block.ID,
				ToolName:  block.Name,
				Input:     input,
			})
		}
	}

	stopReason := "end_turn"
	if resp.StopReason == "tool_use" {
		stopReason = "tool_use"
	} else if resp.StopReason == "max_tokens" {
		stopReason = "max_tokens"
	}

	return &CompletionResult{
		Message: Message{
			Role:    "assistant",
			Content: content,
		},
		StopReason: stopReason,
		Usage: Usage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		},
	}, nil
}

func (p *AnthropicProvider) parseStream(body io.Reader, ch chan<- StreamEvent) {
	var currentToolID, currentToolName string
	var inThinkingBlock bool
	var toolInputBuf strings.Builder

	// In debug mode, dump raw SSE to file for analysis
	var dumpWriter io.Writer = io.Discard
	if logger.IsDebug() {
		homeDir, _ := os.UserHomeDir()
		dumpPath := filepath.Join(homeDir, ".bugbuster", "sse_dump.txt")
		_ = os.MkdirAll(filepath.Dir(dumpPath), 0755)
		if f, err := os.Create(dumpPath); err == nil {
			_, _ = f.WriteString(fmt.Sprintf("=== SSE dump from %s (%s) at %s ===\n",
				p.name, p.model, time.Now().Format("2006-01-02 15:04:05")))
			dumpWriter = f
			defer f.Close()
		}
	}
	teeReader := io.TeeReader(body, dumpWriter)

	_ = ParseSSE(teeReader, func(event, data string) {
		if data == "" || data == "[DONE]" {
			return
		}

		// Log raw event for debugging (in debug mode)
		logger.Debug("anthropic SSE raw",
			"event_type", event,
			"data", data,
		)

		// First parse into raw map to understand what is actually received
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(data), &raw); err != nil {
			return
		}

		// Determine event type
		var chunkType string
		if t, ok := raw["type"]; ok {
			_ = json.Unmarshal(t, &chunkType)
		}

		// Looking for thinking data in raw JSON — check all possible formats
		// Format 1: Anthropic standard — content_block_start with type=thinking
		// Format 2: OpenAI reasoning_content in delta
		// Format 3: Separate "thinking" field at top level
		if thinkingRaw, ok := raw["thinking"]; ok {
			var thinkingText string
			if err := json.Unmarshal(thinkingRaw, &thinkingText); err == nil && thinkingText != "" {
				ch <- StreamEvent{Type: EventThinking, Text: thinkingText}
			}
		}

		logger.Debug("anthropic chunk",
			"type", chunkType,
			"raw_keys", func() []string {
				var keys []string
				for k := range raw {
					keys = append(keys, k)
				}
				return keys
			}(),
		)

		var chunk struct {
			Type         string `json:"type"`
			ContentBlock struct {
				Type  string          `json:"type"`
				ID    string          `json:"id"`
				Name  string          `json:"name"`
				Text  string          `json:"text"`
				Input json.RawMessage `json:"input"`
			} `json:"content_block"`
			Delta struct {
				Type             string `json:"type"`
				Text             string `json:"text"`
				Thinking         string `json:"thinking"` // Anthropic thinking delta (thinking_delta)
				PartialJSON      string `json:"partial_json"`
				ReasoningContent string `json:"reasoning_content"` // OpenAI reasoning format
			} `json:"delta"`
			Index int `json:"index"`
		}

		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			logger.Warn("anthropic parse error", "error", err, "data", data)
			return
		}

		switch chunkType {
		case "content_block_start":
			if chunk.ContentBlock.Type == "tool_use" {
				currentToolID = chunk.ContentBlock.ID
				currentToolName = chunk.ContentBlock.Name
				ch <- StreamEvent{
					Type:       EventToolCallStart,
					ToolCallID: currentToolID,
					ToolName:   currentToolName,
				}
			} else if chunk.ContentBlock.Type == "thinking" {
				inThinkingBlock = true
				logger.Debug("anthropic thinking block started")
			}

		case "content_block_delta":
			deltaType := chunk.Delta.Type
			logger.Debug("anthropic delta",
				"delta_type", deltaType,
				"has_thinking", chunk.Delta.Thinking != "",
				"has_reasoning", chunk.Delta.ReasoningContent != "",
				"has_text", chunk.Delta.Text != "",
				"in_thinking", inThinkingBlock,
			)

			// Thinking delta — standard Anthropic format
			// z.ai (GLM-5.1) sends thinking in delta.thinking, NOT in delta.text
			if deltaType == "thinking_delta" {
				thinkingText := chunk.Delta.Thinking
				// Fallback: some implementations may use delta.text
				if thinkingText == "" {
					thinkingText = chunk.Delta.Text
				}
				if thinkingText != "" {
					ch <- StreamEvent{Type: EventThinking, Text: thinkingText}
				}
			} else if deltaType == "text_delta" {
				if inThinkingBlock {
					ch <- StreamEvent{Type: EventThinking, Text: chunk.Delta.Text}
				} else {
					ch <- StreamEvent{Type: EventTextDelta, Text: chunk.Delta.Text}
				}
			} else if deltaType == "input_json_delta" {
				toolInputBuf.WriteString(chunk.Delta.PartialJSON)
				ch <- StreamEvent{Type: EventToolCallDelta, ToolDelta: chunk.Delta.PartialJSON}
			}

			// Reasoning content — alternative format (OpenAI-compatible, z.ai etc.)
			// Check SEPARATELY from deltaType, because reasoning_content can
			// come inside text_delta or even without deltaType
			if chunk.Delta.ReasoningContent != "" {
				ch <- StreamEvent{Type: EventThinking, Text: chunk.Delta.ReasoningContent}
			}

			// If deltaType is empty but text exists — try to determine what it is
			if deltaType == "" && chunk.Delta.Text != "" {
				if inThinkingBlock {
					ch <- StreamEvent{Type: EventThinking, Text: chunk.Delta.Text}
				} else {
					ch <- StreamEvent{Type: EventTextDelta, Text: chunk.Delta.Text}
				}
			}

		case "content_block_stop":
			if currentToolID != "" {
				ch <- StreamEvent{
					Type:       EventToolCallEnd,
					ToolCallID: currentToolID,
					ToolName:   currentToolName,
				}
				currentToolID = ""
				currentToolName = ""
			}
			inThinkingBlock = false

		case "message_start":
			// message_start — start of message, may contain metadata
			logger.Debug("anthropic message_start")

		case "message_delta":
			// message_delta — contains stop_reason and usage
			var msgDelta struct {
				Delta struct {
					StopReason   string  `json:"stop_reason"`
					StopSequence *string `json:"stop_sequence"`
				} `json:"delta"`
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &msgDelta); err == nil {
				if msgDelta.Delta.StopReason != "" {
					ch <- StreamEvent{Type: "stop_reason", StopReason: msgDelta.Delta.StopReason}
				}
				if msgDelta.Usage.InputTokens > 0 || msgDelta.Usage.OutputTokens > 0 {
					ch <- StreamEvent{Type: EventUsage, InputTokens: msgDelta.Usage.InputTokens, OutputTokens: msgDelta.Usage.OutputTokens}
				}
			}

		case "message_stop":
			ch <- StreamEvent{Type: EventDone}

		case "ping":
			// heartbeat — ignored

		default:
			logger.Debug("anthropic unknown event type", "type", chunkType)
		}
	})
}

// CompleteWithCtx sends request with context
func (p *AnthropicProvider) CompleteWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error) {
	return CompleteWithCtxDefault(p, ctx, messages, tools)
}
