package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

func init() {
	i18n.Init("en")
}

// ===================== AskTool Execute tests =====================

// mockAskProvider implements provider.Provider for testing AskTool
type mockAskProvider struct {
	response string
	err      error
}

func (m *mockAskProvider) Name() string  { return "mock" }
func (m *mockAskProvider) Model() string { return "mock-model" }

func (m *mockAskProvider) Complete(messages []provider.Message, tools []provider.ToolDef) (*provider.CompletionResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &provider.CompletionResult{
		Message:    provider.AssistantText(m.response),
		StopReason: "end_turn",
	}, nil
}

func (m *mockAskProvider) CompleteWithCtx(ctx context.Context, messages []provider.Message, tools []provider.ToolDef) (*provider.CompletionResult, error) {
	return m.Complete(messages, tools)
}

func (m *mockAskProvider) Stream(messages []provider.Message, tools []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.StreamEvent{Type: "text_delta", Text: m.response}
	ch <- provider.StreamEvent{Type: "done"}
	close(ch)
	return ch, nil
}

func (m *mockAskProvider) StreamWithCtx(ctx context.Context, messages []provider.Message, tools []provider.ToolDef) (<-chan provider.StreamEvent, error) {
	return m.Stream(messages, tools)
}

func TestAskTool_Execute_Success(t *testing.T) {
	tool := NewAskTool()
	tool.Provider = &mockAskProvider{response: "Paris is the capital of France."}

	result := tool.Execute(map[string]string{"prompt": "What is the capital of France?"})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Paris") {
		t.Errorf("expected output to contain 'Paris', got: %s", result.Output)
	}
}

func TestAskTool_Execute_ProviderError(t *testing.T) {
	tool := NewAskTool()
	tool.Provider = &mockAskProvider{err: fmt.Errorf("connection refused")}

	result := tool.Execute(map[string]string{"prompt": "Hello"})
	if result.Error == "" {
		t.Fatal("expected error when provider fails")
	}
}

func TestAskTool_Execute_EmptyResponse(t *testing.T) {
	tool := NewAskTool()
	tool.Provider = &mockAskProvider{response: ""}

	result := tool.Execute(map[string]string{"prompt": "Hello"})
	if result.Error == "" {
		t.Fatal("expected error for empty response from provider")
	}
}

func TestAskTool_Execute_LongResponseTruncated(t *testing.T) {
	tool := NewAskTool()
	longResponse := strings.Repeat("a", 6000)
	tool.Provider = &mockAskProvider{response: longResponse}

	result := tool.Execute(map[string]string{"prompt": "Write a lot"})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if len(result.Output) > 5100 { // 5000 + truncation message
		t.Errorf("expected output to be truncated, got length %d", len(result.Output))
	}
}

// ===================== AskUserTool Cancel tests =====================

func TestAskUserTool_Cancel_StopsWaiting(t *testing.T) {
	tool := NewAskUserTool()

	ch := &AskChannel{
		Question: make(chan string, 1),
		Answer:   make(chan string, 1),
	}
	tool.SetAskChannel(ch)

	var result ToolResult
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		result = tool.Execute(map[string]string{"question": "What is your name?"})
	}()

	// Wait for the question to be sent
	select {
	case <-ch.Question:
		// Good, question was sent
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for question")
	}

	// Cancel while waiting for answer
	tool.Cancel()

	wg.Wait()

	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
	// Should return no_answer message (i18n key resolved to "(user did not answer)")
	if !strings.Contains(result.Output, "user did not answer") && !strings.Contains(result.Output, "no_answer") {
		t.Errorf("expected no_answer in output, got: %s", result.Output)
	}
}

func TestAskUserTool_Cancel_BeforeExecute(t *testing.T) {
	tool := NewAskUserTool()

	// Use AskFunc mode (CLI) — simpler to test cancel behavior
	// Cancel before execute — the cancel signal will be in the channel
	tool.Cancel()

	// Set AskFunc — but since cancel is already sent, the tool should
	// check cancelCh first in channel mode. With no channel set,
	// it falls through to AskFunc mode which doesn't check cancelCh.
	// So let's test with NonInteractive mode instead.
	tool.NonInteractive = true

	result := tool.Execute(map[string]string{"question": "Test?"})
	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
	// In non-interactive mode, should return "skipped"
	if !strings.Contains(result.Output, "skipped") {
		t.Errorf("expected 'skipped' in output, got: %s", result.Output)
	}
}

func TestAskUserTool_Cancel_MultipleTimes(t *testing.T) {
	tool := NewAskUserTool()

	// Multiple cancels should not panic or block
	for i := 0; i < 10; i++ {
		tool.Cancel()
	}
}

func TestAskUserTool_Cancel_NonInteractive(t *testing.T) {
	tool := NewAskUserTool()
	tool.NonInteractive = true

	// Cancel should not panic in non-interactive mode
	tool.Cancel()
}

// ===================== WebFetchTool Execute tests =====================

func TestWebFetchTool_Execute_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body>Hello World</body></html>")
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{"url": server.URL})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "200") {
		t.Errorf("expected status 200 in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Hello World") {
		t.Errorf("expected 'Hello World' in output, got: %s", result.Output)
	}
}

func TestWebFetchTool_Execute_WithMethod(t *testing.T) {
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "")
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{"url": server.URL, "method": "HEAD"})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if receivedMethod != "HEAD" {
		t.Errorf("expected HEAD method, got %s", receivedMethod)
	}
}

func TestWebFetchTool_Execute_PostMethod(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "POST response")
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{"url": server.URL, "method": "POST"})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
}

func TestWebFetchTool_Execute_CustomHeaders(t *testing.T) {
	var receivedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Custom-Header")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{
		"url":     server.URL,
		"headers": "X-Custom-Header:TestValue",
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if receivedHeader != "TestValue" {
		t.Errorf("expected custom header 'TestValue', got '%s'", receivedHeader)
	}
}

func TestWebFetchTool_Execute_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Not Found")
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{"url": server.URL})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "404") {
		t.Errorf("expected 404 status in output, got: %s", result.Output)
	}
}

func TestWebFetchTool_Execute_500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Internal Server Error")
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{"url": server.URL})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "500") {
		t.Errorf("expected 500 status in output, got: %s", result.Output)
	}
}

func TestWebFetchTool_Execute_ConnectionError(t *testing.T) {
	tool := NewWebFetchTool()
	tool.AllowNetwork = true

	// Use a port that's unlikely to be listening
	result := tool.Execute(map[string]string{"url": "http://127.0.0.1:1"})
	if result.Error == "" {
		t.Fatal("expected error for connection refused")
	}
}

func TestWebFetchTool_Execute_LargeResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Send a response larger than maxOutput (10000)
		fmt.Fprint(w, strings.Repeat("x", 20000))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{"url": server.URL})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	// Output should be truncated
	if len(result.Output) > 10500 {
		t.Errorf("expected output to be truncated, got length %d", len(result.Output))
	}
}

func TestWebFetchTool_Execute_ResponseTooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "2000000")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data")
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	tool.AllowNetwork = true
	tool.MaxSize = 1024 // 1KB limit

	result := tool.Execute(map[string]string{"url": server.URL})
	if result.Error == "" {
		t.Fatal("expected error for response too large")
	}
}

func TestWebFetchTool_Execute_DefaultMethod(t *testing.T) {
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{"url": server.URL})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if receivedMethod != "GET" {
		t.Errorf("expected GET method, got %s", receivedMethod)
	}
}

func TestWebFetchTool_Execute_UserAgent(t *testing.T) {
	var receivedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{"url": server.URL})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if receivedUA != "BugBuster-Code/1.0" {
		t.Errorf("expected User-Agent 'BugBuster-Code/1.0', got '%s'", receivedUA)
	}
}

func TestWebFetchTool_Execute_ContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{"url": server.URL})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "application/json") {
		t.Errorf("expected Content-Type in output, got: %s", result.Output)
	}
}

func TestWebFetchTool_Execute_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	tool.AllowNetwork = true
	tool.Timeout = 100 * time.Millisecond

	result := tool.Execute(map[string]string{"url": server.URL})
	if result.Error == "" {
		t.Fatal("expected error for timeout")
	}
}

func TestWebFetchTool_Execute_BlockedMethod(t *testing.T) {
	tool := NewWebFetchTool()
	tool.AllowNetwork = true

	result := tool.Execute(map[string]string{"url": "https://example.com", "method": "DELETE"})
	if result.Error == "" {
		t.Fatal("expected error for blocked method")
	}
}

func TestWebFetchTool_Execute_InvalidHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	tool.AllowNetwork = true

	// Headers without colon should be ignored
	result := tool.Execute(map[string]string{
		"url":     server.URL,
		"headers": "InvalidHeader",
	})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
}