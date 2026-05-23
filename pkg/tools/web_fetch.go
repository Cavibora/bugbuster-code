package tools

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bugbuster-code/pkg/i18n"
)

// WebFetchTool — tool for HTTP requests
type WebFetchTool struct {
	AllowNetwork bool          // whether network requests are allowed
	Timeout      time.Duration // request timeout
	MaxSize      int64         // max response size (bytes)
}

// NewWebFetchTool creates a tool for fetching content from URLs.
func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{
		AllowNetwork: true,
		Timeout:      30 * time.Second,
		MaxSize:      1024 * 1024, // 1MB
	}
}

func (t *WebFetchTool) Name() string { return "web_fetch" }

func (t *WebFetchTool) Description() string {
	return i18n.T("tools.web_fetch.description")
}

func (t *WebFetchTool) Execute(params map[string]string) ToolResult {
	url, ok := params["url"]
	if !ok || url == "" {
		return Error("tools.web_fetch.param_url")
	}

	if !t.AllowNetwork {
		return Error("security.network_blocked_general")
	}

	// URL validation
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return Error("tools.web_fetch.invalid_url")
	}

	method := params["method"]
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)

	// Allow only safe methods
	allowedMethods := map[string]bool{"GET": true, "HEAD": true, "POST": true}
	if !allowedMethods[method] {
		return Error("tools.web_fetch.method_not_allowed", method)
	}

	client := &http.Client{
		Timeout: t.Timeout,
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return Error("tools.web_fetch.create_error", err)
	}

	// User-Agent
	req.Header.Set("User-Agent", "BugBuster-Code/1.0")

	// Custom headers
	if headers, ok := params["headers"]; ok && headers != "" {
		for _, h := range strings.Split(headers, ",") {
			parts := strings.SplitN(h, ":", 2)
			if len(parts) == 2 {
				req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
			}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return Error("tools.web_fetch.request_error", err)
	}
	defer resp.Body.Close()

	// Check size
	if resp.ContentLength > t.MaxSize {
		return Error("tools.web_fetch.response_too_large", resp.ContentLength, t.MaxSize)
	}

	// Read response body with size limit
	limitedReader := io.LimitReader(resp.Body, t.MaxSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return Error("tools.web_fetch.read_error", err)
	}

	// Limit output
	maxOutput := 10000
	result := fmt.Sprintf("HTTP %d %s\n", resp.StatusCode, resp.Status)
	result += fmt.Sprintf("Content-Type: %s\n", resp.Header.Get("Content-Type"))
	result += fmt.Sprintf("Content-Length: %d\n\n", len(body))
	result += string(body)

	if len(result) > maxOutput {
		result = result[:maxOutput] + fmt.Sprintf(i18n.T("tools.web_fetch.truncated"), len(result))
	}

	return Success("%s", result)
}

func (t *WebFetchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.web_fetch.param_url_desc"),
			},
			"method": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.web_fetch.param_method_desc"),
			},
			"headers": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.web_fetch.param_headers_desc"),
			},
		},
		"required": []string{"url"},
	}
}
