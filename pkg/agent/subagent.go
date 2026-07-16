package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
	"bugbuster-code/pkg/tools"
)

// DelegateTaskToolName is task delegation tool name
const DelegateTaskToolName = "delegate_task"

// SubagentConfig configures behavior subagents.
type SubagentConfig struct {
	MaxConcurrent      int                          // max concurrent subagents (default: 3)
	Timeout            time.Duration                // timeout for subagent (default: 10m)
	MaxIterations      int                          // max loop iterations for subagent (default: 15)
	MaxResultLen       int                          // max result length in characters (default: 8000)
	Compactor          Compactor                    // LLM compactor from parent (can be nil)
	ContextTokens      int                          // context window size (0 = inherit from parent)
	ContextKeepRecent  int                          // keep recent messages on compaction (0 = auto)
	ProviderName       string                       // provider name for subagent (empty = inherit from parent)
	ModelName          string                       // model name override (empty = use provider default)
	Providers          map[string]provider.ProviderConfig // available providers for creating subagent provider
}

// DefaultSubagentConfig returns configuration default.
func DefaultSubagentConfig() SubagentConfig {
	return SubagentConfig{
		MaxConcurrent:     3,
		Timeout:          10 * time.Minute,
		MaxIterations:    15,
		MaxResultLen:     8000,
		ContextTokens:    0, // 0 = inherit from parent
		ContextKeepRecent: 0,
	}
}

// SubagentTool implements tools.Tool and tools.AsyncTool — spawns subagent on call.
// Subagent runs in isolated context with limited set of tools.
// On AsyncTool call forwards subagent progress to parent stream.
type SubagentTool struct {
	config   SubagentConfig
	provider provider.Provider
	tools    map[string]tools.Tool // parent tools registry (for filtering)
	sem      chan struct{}         // semaphore limiting concurrency
	cancelCh <-chan struct{}       // parent cancellation channel (closed on cancellation)
}

// NewSubagentTool creates tool for delegating tasks to subagents.
func NewSubagentTool(config SubagentConfig, prov provider.Provider, parentTools map[string]tools.Tool) *SubagentTool {
	sem := make(chan struct{}, config.MaxConcurrent)
	for i := 0; i < config.MaxConcurrent; i++ {
		sem <- struct{}{}
	}
	return &SubagentTool{
		config:   config,
		provider: prov,
		tools:    parentTools,
		sem:      sem,
	}
}

// SetCancelCh sets parent cancellation channel.
// When channel is closed, subagent must stop working.
func (t *SubagentTool) SetCancelCh(ch <-chan struct{}) {
	t.cancelCh = ch
}

func (t *SubagentTool) Name() string { return DelegateTaskToolName }

func (t *SubagentTool) Description() string {
	return i18n.T("tools.delegate_task.description")
}

func (t *SubagentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.delegate_task.param_task_desc"),
			},
			"context": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.delegate_task.param_context_desc"),
			},
		},
		"required": []string{"task"},
	}
}

// createContext creates context with timeout, considering parent cancellation.
func (t *SubagentTool) createContext() (context.Context, context.CancelFunc) {
	if t.cancelCh != nil {
		ctx, cancel := context.WithTimeout(context.Background(), t.config.Timeout)
		go func() {
			select {
			case <-t.cancelCh:
				cancel()
			case <-ctx.Done():
			}
		}()
		return ctx, cancel
	}
	return context.WithTimeout(context.Background(), t.config.Timeout)
}

// Execute spawns subagent synchronously (for non-AsyncTool calls).
func (t *SubagentTool) Execute(params map[string]string) tools.ToolResult {
	taskDesc := params["task"]
	taskCtx := params["context"]

	if taskDesc == "" {
		return tools.Error("tools.delegate_task.empty_task")
	}

	<-t.sem
	defer func() { t.sem <- struct{}{} }()

	subLoop := newSubagentLoop(t.tools, t.provider, t.config, taskDesc, taskCtx)

	ctx, cancel := t.createContext()
	defer cancel()

	result, err := t.runSubagent(ctx, subLoop, taskDesc, nil)
	if err != nil {
		errMsg := fmt.Sprintf("subagent failed: %v", err)
		if ctx.Err() == context.DeadlineExceeded {
			errMsg = i18n.T("errors.subagent_timeout")
		}
		return tools.ToolResult{Error: errMsg}
	}

	result = summarizeResult(result, t.config.MaxResultLen)

	return tools.ToolResult{Output: result}
}

// ExecuteAsync spawns subagent asynchronously, passing progress to parent stream.
func (t *SubagentTool) ExecuteAsync(params map[string]string) <-chan tools.AsyncEvent {
	ch := make(chan tools.AsyncEvent, 50)

	go func() {
		defer close(ch)

		taskDesc := params["task"]
		taskCtx := params["context"]

		if taskDesc == "" {
			ch <- tools.AsyncEvent{Type: "result", Error: "task description is required", Done: true}
			return
		}

		ch <- tools.AsyncEvent{Type: "progress", Output: i18n.T("cli.subagent_starting")}
		<-t.sem
		defer func() { t.sem <- struct{}{} }()

		subLoop := newSubagentLoop(t.tools, t.provider, t.config, taskDesc, taskCtx)

		ctx, cancel := t.createContext()
		defer cancel()

		result, err := t.runSubagent(ctx, subLoop, taskDesc, ch)
		if err != nil {
			errMsg := fmt.Sprintf("subagent failed: %v", err)
			if ctx.Err() == context.DeadlineExceeded {
				errMsg = i18n.T("errors.subagent_timeout")
			}
			ch <- tools.AsyncEvent{Type: "result", Error: errMsg, Done: true}
			return
		}

		result = summarizeResult(result, t.config.MaxResultLen)

		ch <- tools.AsyncEvent{Type: "result", Output: result, Done: true}
	}()

	return ch
}

// runSubagent starts subagent and returns result.
// If progressCh != nil, sends subagent progress events to channel.
func (t *SubagentTool) runSubagent(ctx context.Context, loop *AgentLoop, taskDesc string, progressCh chan<- tools.AsyncEvent) (string, error) {
	eventCh, err := loop.StreamWithCancel(ctx, taskDesc)
	if err != nil {
		return loop.Run(taskDesc)
	}

	var result strings.Builder
	var toolResults []string // collect tool results for summary
	var compactionCount int  // count compactions to detect infinite loops
	iteration := 0

	for event := range eventCh {
		switch event.Type {
		case provider.EventTextDelta:
			result.WriteString(event.Text)
		case provider.EventDone:
			if result.Len() == 0 && event.Text != "" {
				result.WriteString(event.Text)
			}
		case provider.EventError:
			if ctx.Err() == context.DeadlineExceeded && result.Len() > 0 {
				warning := fmt.Sprintf("\n\n⚠️ %s", i18n.T("errors.subagent_timeout_partial"))
				result.WriteString(warning)
				return result.String(), nil
			}
			if result.Len() > 0 {
				return result.String(), event.Error
			}
			return "", event.Error

		case provider.EventToolCallStart:
			if progressCh != nil {
				var toolParams map[string]string
				if event.ToolInput != nil && len(event.ToolInput) > 0 {
					toolParams = make(map[string]string)
					for k, v := range event.ToolInput {
						toolParams[k] = fmt.Sprintf("%v", v)
					}
				}
				summary := formatSubagentToolSummary(event.ToolName, toolParams)
				progressCh <- tools.AsyncEvent{
					Type:   "progress",
					Output: summary,
				}
			}
		case provider.EventToolCallEnd:
			// Collect tool result for summary
			if event.ToolFullResult != "" {
				toolResults = append(toolResults, event.ToolFullResult)
			}
			if progressCh != nil {
				status := "✓"
				if !event.ToolOK {
					status = "✗"
				}
				durStr := ""
				if event.Duration > 0 {
					durStr = fmt.Sprintf(" %s", formatDuration(event.Duration))
				}
				progressCh <- tools.AsyncEvent{
					Type:   "progress",
					Output: fmt.Sprintf("  ⎿ %s %s%s", status, event.ToolName, durStr),
				}
			}
		case provider.EventToolProgress:
			if progressCh != nil && event.ToolMessage != "" {
				progressCh <- tools.AsyncEvent{
					Type:   "progress",
					Output: event.ToolMessage,
				}
			}
		case provider.EventThinking:
			if progressCh != nil {
				summary := summarizeThinking(event.Text)
				if summary != "" {
					progressCh <- tools.AsyncEvent{
						Type:   "progress",
						Output: i18n.T("cli.subagent_thinking", summary),
					}
				}
			}
		case provider.EventIterationEnd:
			iteration++
			if progressCh != nil {
				progressCh <- tools.AsyncEvent{
					Type:   "progress",
					Output: i18n.T("cli.subagent_iteration", iteration),
				}
			}
		case provider.EventCompaction:
			compactionCount++
			if compactionCount >= 3 {
				// Too many compactions — subagent is stuck in a loop
				// Return what we have so far with a warning
				result.WriteString(fmt.Sprintf("\n\n⚠️ %s", i18n.T("errors.subagent_compaction_loop")))
				return result.String(), nil
			}
			if compactionCount == 2 {
				// Second compaction — subagent is likely stuck, force finish on next iteration
				if progressCh != nil {
					progressCh <- tools.AsyncEvent{
						Type:   "progress",
						Output: "⚠️ Context compacted twice, finishing up...",
					}
				}
			}
			if progressCh != nil {
				progressCh <- tools.AsyncEvent{
					Type:   "progress",
					Output: i18n.T("cli.compacting"),
				}
			}
		}
	}

	// If subagent produced no text but has tool results,
	// build summary from tool results
	if result.Len() == 0 && len(toolResults) > 0 {
		result.WriteString(buildToolResultSummary(toolResults))
	}

	return result.String(), nil
}

// buildToolResultSummary creates a summary from collected tool results.
func buildToolResultSummary(toolResults []string) string {
	var b strings.Builder
	b.WriteString("Subagent completed tool operations. Results:\n\n")
	for i, tr := range toolResults {
		if i >= 10 { // limit to 10 results
			b.WriteString(fmt.Sprintf("... and %d more results\n", len(toolResults)-10))
			break
		}
		// Take first 200 chars of each result
		line := tr
		if len(line) > 200 {
			line = line[:197] + "..."
		}
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, line))
	}
	return b.String()
}

// formatDuration formats duration in human-readable form.
func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return "<1ms"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
}

// formatSubagentToolSummary formats brief summary of subagent tool call.
// bash commands are shown in full for security — user must see what's being executed.
func formatSubagentToolSummary(toolName string, params map[string]string) string {
	displayKeys := []string{"path", "command", "pattern", "query", "prompt", "url", "file", "dir"}
	noTruncate := toolName == "bash" || toolName == "write" || toolName == "edit"
	for _, key := range displayKeys {
		if v, ok := params[key]; ok {
			display := v
			if noTruncate {
				// Show full command/path for security
				display = strings.ReplaceAll(display, "\n", " ⏎ ")
				display = strings.ReplaceAll(display, "\r", "")
			} else if len(display) > 60 {
				display = display[:57] + "..."
			}
			return fmt.Sprintf("  ⏺ %s(%s)", toolName, display)
		}
	}
	return fmt.Sprintf("  ⏺ %s", toolName)
}

// summarizeThinking extracts meaningful phrase from thinking text.
func summarizeThinking(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			if len(line) > 80 {
				return line[:77] + "..."
			}
			return line
		}
	}
	return ""
}

// newSubagentLoop creates isolated AgentLoop for subagent.
func newSubagentLoop(
	parentTools map[string]tools.Tool,
	prov provider.Provider,
	config SubagentConfig,
	taskDesc string,
	taskContext string,
) *AgentLoop {
	filteredTools := make(map[string]tools.Tool, len(parentTools))
	for name, t := range parentTools {
		if name != DelegateTaskToolName {
			filteredTools[name] = t
		}
	}

	// Determine provider for subagent
	subProvider := prov // default: inherit from parent
	if config.ProviderName != "" && config.Providers != nil {
		if provCfg, ok := config.Providers[config.ProviderName]; ok {
			// Override model if specified
			if config.ModelName != "" {
				provCfg.Model = config.ModelName
			}
			if p, err := provider.NewFromConfig(config.ProviderName, provCfg); err == nil {
				subProvider = p
			}
		}
	} else if config.ModelName != "" && config.Providers != nil {
		// No provider specified, but model specified — find provider with this model
		for provName, provCfg := range config.Providers {
			if provCfg.Model == config.ModelName {
				if p, err := provider.NewFromConfig(provName, provCfg); err == nil {
					subProvider = p
				}
				break
			}
		}
	}

	// Subagents have stricter loop detection than main agent
	loopDetector := NewLoopDetector()
	loopDetector.SetToolRepeatThreshold(3)       // 3 repeats of same tool = loop (vs 8 for main)
	loopDetector.SetRepeatThreshold(2)           // 2 identical iterations = loop (vs 6 for main)
	loopDetector.SetTextSimilarityThreshold(0.5)  // More aggressive text similarity detection

	// Context window: inherit from parent if ContextTokens is 0
	contextTokens := config.ContextTokens
	if contextTokens == 0 {
		contextTokens = 24000 // default fallback
	}
	contextKeepRecent := config.ContextKeepRecent
	if contextKeepRecent == 0 {
		// Keep 2/3 of context as recent messages
		contextKeepRecent = contextTokens * 2 / 3 / 500 // ~2/3 of context in messages
		if contextKeepRecent < 8 {
			contextKeepRecent = 8
		}
	}

	loop := &AgentLoop{
		Tools:           filteredTools,
		Context:         NewConversationContextWithTokens(contextTokens, contextKeepRecent),
		provider:        subProvider,
		LoopDetector:   loopDetector,
		maxIterations:   config.MaxIterations,
		nonInteractive:  true,
		userInject:      make(chan string, 16),
		RequestTimeout:  10 * time.Minute,
		ThinkingTimeout: 5 * time.Minute,
		IdleTimeout:     3 * time.Minute,
	}

	if config.Compactor != nil {
		loop.Context.Compactor = config.Compactor
	}

	loop.SetSystemPrompt(buildSubagentSystemPrompt(taskDesc, taskContext))

	return loop
}

// buildSubagentSystemPrompt creates system prompt for subagent.
func buildSubagentSystemPrompt(taskDesc, taskContext string) string {
	var b strings.Builder
	b.WriteString("You are a specialized subagent. Your job is to complete the assigned task and return a clear result.\n\n")
	b.WriteString("RULES:\n")
	b.WriteString("1. Complete the task yourself — do NOT delegate to other agents.\n")
	b.WriteString("2. After completing all tool calls, you MUST write a final text summary of what you did and what you found.\n")
	b.WriteString("3. Your final message must contain the answer or result — not just tool calls.\n")
	b.WriteString("4. Be concise but thorough. Include specific findings, file paths, code changes, etc.\n")
	b.WriteString("5. If you cannot complete the task, explain why and what you tried.\n\n")
	b.WriteString("## Task\n")
	b.WriteString(taskDesc)
	if taskContext != "" {
		b.WriteString("\n\n## Additional Context\n")
		b.WriteString(taskContext)
	}
	return b.String()
}

// summarizeResult trims result if longer than maxLen.
func summarizeResult(result string, maxLen int) string {
	if len(result) <= maxLen {
		return result
	}
	truncated := result[:maxLen]
	if idx := strings.LastIndex(truncated, "\n"); idx > maxLen/2 {
		truncated = truncated[:idx]
	}
	return truncated + fmt.Sprintf("\n\n[Result truncated. Full length: %d characters]", len(result))
}