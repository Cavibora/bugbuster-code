package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
	"bugbuster-code/pkg/tools"
)

func TestRunSubagent_TextResponse(t *testing.T) {
	i18n.Init("en")

	mockProv := &MockStreamingProvider{
		events: []provider.StreamEvent{
			{Type: provider.EventTextDelta, Text: "Hello from subagent"},
			{Type: provider.EventDone, Text: "Hello from subagent"},
		},
	}

	tool := NewSubagentTool(SubagentConfig{
		MaxConcurrent: 1,
		Timeout:       30 * time.Second,
		MaxIterations: 5,
		MaxResultLen:  4000,
	}, mockProv, map[string]tools.Tool{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loop := newSubagentLoop(tool.tools, mockProv, tool.config, "test task", "test context")
	result, err := tool.runSubagent(ctx, loop, "test task", nil)

	if err != nil {
		t.Fatalf("runSubagent returned error: %v", err)
	}
	if !strings.Contains(result, "Hello from subagent") {
		t.Errorf("expected result to contain 'Hello from subagent', got: %s", result)
	}
}

func TestRunSubagent_WithProgress(t *testing.T) {
	i18n.Init("en")

	mockProv := &MockStreamingProvider{
		events: []provider.StreamEvent{
			{Type: provider.EventTextDelta, Text: "Working"},
			{Type: provider.EventDone, Text: "Working"},
		},
	}

	tool := NewSubagentTool(SubagentConfig{
		MaxConcurrent: 1,
		Timeout:       30 * time.Second,
		MaxIterations: 5,
		MaxResultLen:  4000,
	}, mockProv, map[string]tools.Tool{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	progressCh := make(chan tools.AsyncEvent, 50)
	loop := newSubagentLoop(tool.tools, mockProv, tool.config, "test task", "")
	result, err := tool.runSubagent(ctx, loop, "test task", progressCh)

	if err != nil {
		t.Fatalf("runSubagent returned error: %v", err)
	}
	if !strings.Contains(result, "Working") {
		t.Errorf("expected result to contain 'Working', got: %s", result)
	}
	close(progressCh)

	var progressCount int
	for event := range progressCh {
		if event.Type == "progress" {
			progressCount++
		}
	}
	_ = progressCount
}

func TestRunSubagent_ToolCallProgress(t *testing.T) {
	i18n.Init("en")

	mockProv := &MockStreamingProvider{
		events: []provider.StreamEvent{
			{Type: provider.EventToolCallStart, ToolName: "bash", ToolInput: map[string]any{"command": "ls"}},
			{Type: provider.EventToolCallEnd, ToolName: "bash", ToolOK: true, Duration: 100 * time.Millisecond},
			{Type: provider.EventTextDelta, Text: "Done"},
			{Type: provider.EventDone, Text: "Done"},
		},
	}

	tool := NewSubagentTool(SubagentConfig{
		MaxConcurrent: 1,
		Timeout:       30 * time.Second,
		MaxIterations: 5,
		MaxResultLen:  4000,
	}, mockProv, map[string]tools.Tool{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	progressCh := make(chan tools.AsyncEvent, 50)
	loop := newSubagentLoop(tool.tools, mockProv, tool.config, "test task", "")
	_, err := tool.runSubagent(ctx, loop, "test task", progressCh)

	if err != nil {
		t.Fatalf("runSubagent returned error: %v", err)
	}
	close(progressCh)

	var foundToolStart bool
	for event := range progressCh {
		if event.Type == "progress" {
			if strings.Contains(event.Output, "bash") && strings.Contains(event.Output, "⏺") {
				foundToolStart = true
			}
		}
	}

	if !foundToolStart {
		t.Error("expected tool start progress event")
	}
}

func TestRunSubagent_ThinkingProgress(t *testing.T) {
	i18n.Init("en")

	mockProv := &MockStreamingProvider{
		events: []provider.StreamEvent{
			{Type: provider.EventThinking, Text: "Let me think about this problem..."},
			{Type: provider.EventTextDelta, Text: "Result"},
			{Type: provider.EventDone, Text: "Result"},
		},
	}

	tool := NewSubagentTool(SubagentConfig{
		MaxConcurrent: 1,
		Timeout:       30 * time.Second,
		MaxIterations: 5,
		MaxResultLen:  4000,
	}, mockProv, map[string]tools.Tool{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	progressCh := make(chan tools.AsyncEvent, 50)
	loop := newSubagentLoop(tool.tools, mockProv, tool.config, "test task", "")
	_, err := tool.runSubagent(ctx, loop, "test task", progressCh)

	if err != nil {
		t.Fatalf("runSubagent returned error: %v", err)
	}
	close(progressCh)

	var foundThinking bool
	for event := range progressCh {
		if event.Type == "progress" && strings.Contains(strings.ToLower(event.Output), "thinking") {
			foundThinking = true
		}
	}

	if !foundThinking {
		t.Error("expected thinking progress event")
	}
}

func TestRunSubagent_Error(t *testing.T) {
	i18n.Init("en")

	mockProv := &MockStreamingProvider{
		events: []provider.StreamEvent{
			{Type: provider.EventError, Error: fmt.Errorf("provider unavailable")},
		},
	}

	tool := NewSubagentTool(SubagentConfig{
		MaxConcurrent: 1,
		Timeout:       30 * time.Second,
		MaxIterations: 5,
		MaxResultLen:  4000,
	}, mockProv, map[string]tools.Tool{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loop := newSubagentLoop(tool.tools, mockProv, tool.config, "test task", "")
	_, err := tool.runSubagent(ctx, loop, "test task", nil)

	if err == nil {
		t.Error("expected error from runSubagent")
	}
}

func TestRunSubagent_ContextCancelled(t *testing.T) {
	i18n.Init("en")

	mockProv := &MockStreamingProvider{
		events: []provider.StreamEvent{
			{Type: provider.EventTextDelta, Text: "Working..."},
			{Type: provider.EventDone, Text: "Working..."},
		},
	}

	tool := NewSubagentTool(SubagentConfig{
		MaxConcurrent: 1,
		Timeout:       30 * time.Second,
		MaxIterations: 5,
		MaxResultLen:  4000,
	}, mockProv, map[string]tools.Tool{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	loop := newSubagentLoop(tool.tools, mockProv, tool.config, "test task", "")
	_, err := tool.runSubagent(ctx, loop, "test task", nil)

	if err != nil && ctx.Err() != context.Canceled {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewSubagentLoop_FiltersDelegateTask(t *testing.T) {
	mockProv := &MockStreamingProvider{
		events: []provider.StreamEvent{
			{Type: provider.EventTextDelta, Text: "OK"},
			{Type: provider.EventDone, Text: "OK"},
		},
	}

	parentTools := map[string]tools.Tool{
		DelegateTaskToolName: nil,
		"bash":               nil,
		"read":               nil,
	}

	tool := NewSubagentTool(SubagentConfig{
		MaxConcurrent: 1,
		Timeout:       30 * time.Second,
		MaxIterations: 5,
		MaxResultLen:  4000,
	}, mockProv, parentTools)

	loop := newSubagentLoop(tool.tools, mockProv, tool.config, "test task", "")

	if _, ok := loop.Tools[DelegateTaskToolName]; ok {
		t.Error("delegate_task should be filtered out of subagent tools")
	}
	if _, ok := loop.Tools["bash"]; !ok {
		t.Error("bash should be present in subagent tools")
	}
}

func TestSummarizeResult(t *testing.T) {
	tests := []struct {
		name   string
		result string
		maxLen int
		want   string
	}{
		{
			name:   "short result",
			result: "Hello",
			maxLen: 100,
			want:   "Hello",
		},
		{
			name:   "long result truncated",
			result: strings.Repeat("a", 200),
			maxLen: 50,
			want:   "[Result truncated",
		},
		{
			name:   "result at max length",
			result: strings.Repeat("a", 100),
			maxLen: 100,
			want:   strings.Repeat("a", 100),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarizeResult(tt.result, tt.maxLen)
			if tt.maxLen >= len(tt.result) {
				if got != tt.want {
					t.Errorf("summarizeResult() = %q, want %q", got, tt.want)
				}
			} else {
				if !strings.Contains(got, tt.want) {
					t.Errorf("summarizeResult() = %q, want to contain %q", got, tt.want)
				}
			}
		})
	}
}

func TestFormatSubagentToolSummary(t *testing.T) {
	tests := []struct {
		name         string
		toolName     string
		params       map[string]string
		wantContains string
	}{
		{
			name:         "with path",
			toolName:     "read",
			params:       map[string]string{"path": "/tmp/file.txt"},
			wantContains: "read(/tmp/file.txt)",
		},
		{
			name:         "with command",
			toolName:     "bash",
			params:       map[string]string{"command": "ls -la"},
			wantContains: "bash(ls -la)",
		},
		{
			name:         "without display key",
			toolName:     "custom",
			params:       map[string]string{"foo": "bar"},
			wantContains: "custom",
		},
		{
			name:         "long path truncated",
			toolName:     "read",
			params:       map[string]string{"path": strings.Repeat("a", 100)},
			wantContains: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSubagentToolSummary(tt.toolName, tt.params)
			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("formatSubagentToolSummary() = %q, want to contain %q", got, tt.wantContains)
			}
		})
	}
}

func TestBuildSubagentSystemPrompt(t *testing.T) {
	prompt := buildSubagentSystemPrompt("fix the bug", "the bug is in main.go")
	if !strings.Contains(prompt, "fix the bug") {
		t.Error("prompt should contain task description")
	}
	if !strings.Contains(prompt, "main.go") {
		t.Error("prompt should contain task context")
	}
	if !strings.Contains(prompt, "subagent") {
		t.Error("prompt should mention subagent")
	}
}

func TestBuildSubagentSystemPrompt_NoContext(t *testing.T) {
	prompt := buildSubagentSystemPrompt("do something", "")
	if !strings.Contains(prompt, "do something") {
		t.Error("prompt should contain task description")
	}
	if strings.Contains(prompt, "Additional Context") {
		t.Error("prompt should not contain Additional Context section when context is empty")
	}
}

func TestDefaultSubagentConfig(t *testing.T) {
	cfg := DefaultSubagentConfig()
	if cfg.MaxConcurrent != 3 {
		t.Errorf("expected MaxConcurrent=3, got %d", cfg.MaxConcurrent)
	}
	if cfg.Timeout.Seconds() != 600 {
		t.Errorf("expected Timeout=10m, got %v", cfg.Timeout)
	}
	if cfg.MaxIterations != 15 {
		t.Errorf("expected MaxIterations=15, got %d", cfg.MaxIterations)
	}
	if cfg.MaxResultLen != 8000 {
		t.Errorf("expected MaxResultLen=8000, got %d", cfg.MaxResultLen)
	}
}

func TestSubagentTool_Name(t *testing.T) {
	tool := &SubagentTool{}
	if tool.Name() != "delegate_task" {
		t.Errorf("expected 'delegate_task', got '%s'", tool.Name())
	}
}

func TestSubagentTool_Parameters(t *testing.T) {
	tool := &SubagentTool{}
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("expected type=object, got %v", params["type"])
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be map[string]any")
	}
	if _, ok := props["task"]; !ok {
		t.Error("expected 'task' property")
	}
	if _, ok := props["context"]; !ok {
		t.Error("expected 'context' property")
	}
}

func TestSubagentTool_Execute_EmptyTask(t *testing.T) {
	tool := NewSubagentTool(DefaultSubagentConfig(), nil, nil)
	result := tool.Execute(map[string]string{"task": ""})
	if result.Error == "" {
		t.Error("expected error for empty task")
	}
}

func TestSubagentTool_ImplementsAsyncTool(t *testing.T) {
	var _ tools.AsyncTool = (*SubagentTool)(nil)
}

func TestSubagentTool_ExecuteAsync_EmptyTask(t *testing.T) {
	tool := NewSubagentTool(DefaultSubagentConfig(), nil, nil)
	ch := tool.ExecuteAsync(map[string]string{"task": ""})

	var foundError bool
	for evt := range ch {
		if evt.Done && evt.Error != "" {
			foundError = true
		}
	}
	if !foundError {
		t.Error("expected error event for empty task")
	}
}

func TestSubagentTool_SetCancelCh(t *testing.T) {
	tool := NewSubagentTool(DefaultSubagentConfig(), nil, nil)
	cancelCh := make(chan struct{})
	tool.SetCancelCh(cancelCh)
	if tool.cancelCh == nil {
		t.Error("expected cancelCh to be set")
	}
}

func TestSubagentTool_CreateContext_WithCancel(t *testing.T) {
	tool := NewSubagentTool(SubagentConfig{
		Timeout: 5 * time.Minute,
	}, nil, nil)

	cancelCh := make(chan struct{})
	tool.SetCancelCh(cancelCh)

	ctx, cancel := tool.createContext()
	defer cancel()

	if ctx == nil {
		t.Error("expected non-nil context")
	}

	close(cancelCh)
	time.Sleep(50 * time.Millisecond)

	select {
	case <-ctx.Done():
		// Expected
	default:
		t.Error("expected context to be cancelled after cancelCh closed")
	}
}

func TestSubagentTool_CreateContext_WithoutCancel(t *testing.T) {
	tool := NewSubagentTool(SubagentConfig{
		Timeout: 5 * time.Minute,
	}, nil, nil)

	ctx, cancel := tool.createContext()
	defer cancel()

	if ctx == nil {
		t.Error("expected non-nil context")
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Error("expected context to have deadline")
	}
	if deadline.Before(time.Now()) {
		t.Error("deadline should be in the future")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{time.Microsecond, "<1ms"},
		{time.Millisecond, "1ms"},
		{100 * time.Millisecond, "100ms"},
		{time.Second, "1.0s"},
		{5*time.Second + 500*time.Millisecond, "5.5s"},
		{time.Minute, "1.0m"},
		{5*time.Minute + 30*time.Second, "5.5m"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.d)
		if result != tt.expected {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, result, tt.expected)
		}
	}
}

func TestSummarizeThinking(t *testing.T) {
	result := summarizeThinking("")
	if result != "" {
		t.Errorf("expected empty for empty text, got %q", result)
	}

	result = summarizeThinking("I need to fix the bug")
	if result != "I need to fix the bug" {
		t.Errorf("expected single line, got %q", result)
	}

	result = summarizeThinking("line 1\nline 2\nline 3")
	if result != "line 3" {
		t.Errorf("expected last line, got %q", result)
	}

	longLine := strings.Repeat("x", 100)
	result = summarizeThinking(longLine)
	if len(result) > 80 {
		t.Errorf("expected truncated result, got %d chars", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Errorf("expected truncation suffix, got %q", result)
	}
}

func TestNewSubagentLoop_Timeouts(t *testing.T) {
	cfg := DefaultSubagentConfig()
	loop := newSubagentLoop(nil, nil, cfg, "test task", "")

	if loop.RequestTimeout != 10*time.Minute {
		t.Errorf("expected RequestTimeout=10m, got %v", loop.RequestTimeout)
	}
	if loop.ThinkingTimeout != 5*time.Minute {
		t.Errorf("expected ThinkingTimeout=5m, got %v", loop.ThinkingTimeout)
	}
	if loop.IdleTimeout != 3*time.Minute {
		t.Errorf("expected IdleTimeout=3m, got %v", loop.IdleTimeout)
	}
}

func TestNewSubagentLoop_MaxIterations(t *testing.T) {
	cfg := DefaultSubagentConfig()
	cfg.MaxIterations = 5
	loop := newSubagentLoop(nil, nil, cfg, "test task", "")

	if loop.maxIterations != 5 {
		t.Errorf("expected maxIterations=5, got %d", loop.maxIterations)
	}
}

func TestNewSubagentLoop_NonInteractive(t *testing.T) {
	cfg := DefaultSubagentConfig()
	loop := newSubagentLoop(nil, nil, cfg, "test task", "")

	if !loop.nonInteractive {
		t.Error("expected subagent to be non-interactive")
	}
}
