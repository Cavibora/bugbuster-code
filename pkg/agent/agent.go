package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/logger"
	"bugbuster-code/pkg/provider"
	"bugbuster-code/pkg/skills"
	"bugbuster-code/pkg/tools"
)

// AgentLoop is the main agent loop for BugBuster Code
type AgentLoop struct {
	Tools             map[string]tools.Tool
	Context           *ConversationContext
	provider          provider.Provider
	verbose           bool
	debug             bool   // debug mode: log request/response JSON
	debugDir          string // directory for debug logs (.bugbuster)
	nonInteractive    bool   // exec mode: auto-skip ask_user
	PermissionChecker PermissionChecker
	LoopDetector      *LoopDetector // loop detector
	SkillManager      *skills.Manager // skill system

	// LLM request timeouts
	RequestTimeout  time.Duration // max time for a single LLM request (0 = 20 min)
	ThinkingTimeout time.Duration // max time without tokens from model (0 = 10 min)
	IdleTimeout     time.Duration // streaming timeout without events (0 = 2 min)

	// idleTimeoutCount is the number of consecutive idle timeouts
	// After 2 consecutive — fatal error, before that — warning
	idleTimeoutCount int

	// userInject is a channel for injecting user comments
	// during the agent loop. The user can clarify,
	// correct, or guide the agent in real time.
	userInject chan string

	// maxIterations limits loop iterations (0 = unlimited, for subagents)
	maxIterations int
}

// NewAgentLoop creates a new agent loop
func NewAgentLoop(p provider.Provider) *AgentLoop {
	a := &AgentLoop{
		Tools:        make(map[string]tools.Tool),
		Context:      NewConversationContextWithTokens(8000, 20),
		provider:     p,
		verbose:      false,
		LoopDetector: NewLoopDetector(),
		userInject:   make(chan string, 16),
	}

	// Register standard tools
	a.RegisterTool(tools.NewReadTool())
	a.RegisterTool(tools.NewWriteTool())
	a.RegisterTool(tools.NewEditTool())
	a.RegisterTool(tools.NewBashTool())
	a.RegisterTool(tools.NewGrepTool())
	a.RegisterTool(tools.NewGlobTool())

	return a
}

// RegisterTool registers a tool in the agent
func (a *AgentLoop) RegisterTool(tool tools.Tool) {
	a.Tools[tool.Name()] = tool
}

// SetSystemPrompt sets the system prompt
func (a *AgentLoop) SetSystemPrompt(prompt string) {
	// Remove old system prompt
	var filtered []provider.Message
	for _, m := range a.Context.Messages {
		if m.Role != "system" {
			filtered = append(filtered, m)
		}
	}
	a.Context.Messages = filtered
	a.Context.Add(provider.SystemMsg(prompt))
}

// SetVerbose sets verbose mode
func (a *AgentLoop) SetVerbose(v bool) {
	a.verbose = v
}

// SetDebug sets debug mode (log request/response JSON)
func (a *AgentLoop) SetDebug(d bool) {
	a.debug = d
}

// SetDebugDir sets the directory for debug logs
func (a *AgentLoop) SetDebugDir(dir string) {
	a.debugDir = dir
}

// SetMaxTokens sets the maximum token count in context
func (a *AgentLoop) SetMaxTokens(maxTokens int) {
	a.Context.MaxTokens = maxTokens
}

// SetKeepRecent sets the count of recent messages to keep during compaction
func (a *AgentLoop) SetKeepRecent(n int) {
	a.Context.KeepRecent = n
}

// SetProvider sets the provider
func (a *AgentLoop) SetProvider(p provider.Provider) {
	a.provider = p
}

// GetProvider returns the current provider
func (a *AgentLoop) GetProvider() provider.Provider {
	return a.provider
}

// SetNonInteractive sets non-interactive mode (exec)
func (a *AgentLoop) SetNonInteractive(v bool) {
	a.nonInteractive = v
}

// SetPermissionChecker sets the permission checker
func (a *AgentLoop) SetPermissionChecker(pc PermissionChecker) {
	a.PermissionChecker = pc
}

// SetMaxIterations sets the loop iteration limit (0 = unlimited).
// Used for subagents to limit execution time.
func (a *AgentLoop) SetMaxIterations(n int) {
	a.maxIterations = n
}

// SetRequestTimeout sets the maximum time for a single LLM request.
// 0 = use default (20 minutes).
func (a *AgentLoop) SetRequestTimeout(d time.Duration) {
	a.RequestTimeout = d
}

// SetThinkingTimeout sets the maximum time without tokens from the model.
// 0 = use default (10 minutes).
func (a *AgentLoop) SetThinkingTimeout(d time.Duration) {
	a.ThinkingTimeout = d
}

// SetIdleTimeout sets the streaming timeout without events.
// 0 = use default (2 minutes).
func (a *AgentLoop) SetIdleTimeout(d time.Duration) {
	a.IdleTimeout = d
}

// SetLoopRepeatThreshold sets the repetition threshold for loop detection.
func (a *AgentLoop) SetLoopRepeatThreshold(n int) {
	if a.LoopDetector != nil {
		a.LoopDetector.SetRepeatThreshold(n)
	}
}

// SetLoopToolRepeatThreshold sets the repetition threshold for single tool calls.
func (a *AgentLoop) SetLoopToolRepeatThreshold(n int) {
	if a.LoopDetector != nil {
		a.LoopDetector.SetToolRepeatThreshold(n)
	}
}

// SetLoopWindowSize sets the sliding window size for loop detection.
func (a *AgentLoop) SetLoopWindowSize(n int) {
	if a.LoopDetector != nil {
		a.LoopDetector.SetWindowSize(n)
	}
}

// SetLoopTextSimilarityThreshold sets the text response similarity threshold.
func (a *AgentLoop) SetLoopTextSimilarityThreshold(t float64) {
	if a.LoopDetector != nil {
		a.LoopDetector.SetTextSimilarityThreshold(t)
	}
}

// SetLoopTextSimilarityWindow sets the text response similarity check window.
func (a *AgentLoop) SetLoopTextSimilarityWindow(n int) {
	if a.LoopDetector != nil {
		a.LoopDetector.SetTextSimilarityWindow(n)
	}
}

// effectiveRequestTimeout returns the effective request timeout.
func (a *AgentLoop) effectiveRequestTimeout() time.Duration {
	if a.RequestTimeout > 0 {
		return a.RequestTimeout
	}
	return 20 * time.Minute
}

// effectiveThinkingTimeout returns the effective thinking timeout.
func (a *AgentLoop) effectiveThinkingTimeout() time.Duration {
	if a.ThinkingTimeout > 0 {
		return a.ThinkingTimeout
	}
	return 10 * time.Minute
}

// effectiveIdleTimeout returns the effective idle timeout.
func (a *AgentLoop) effectiveIdleTimeout() time.Duration {
	if a.IdleTimeout > 0 {
		return a.IdleTimeout
	}
	return 5 * time.Minute
}

// effectiveMaxTokens returns the effective max_tokens limit.
func (a *AgentLoop) effectiveMaxTokens() int {
	if a.Context.MaxTokens > 0 {
		return a.Context.MaxTokens
	}
	return 16384
}

// EnableSubagents registers the delegate_task tool and configures
// subagent infrastructure for this AgentLoop.
func (a *AgentLoop) EnableSubagents(config SubagentConfig) {
	tool := NewSubagentTool(config, a.provider, a.Tools)
	a.Tools[DelegateTaskToolName] = tool
}

// InjectUserMessage adds a user comment to the context
// during the agent loop. The comment will be processed
// between iterations — the model will receive it as a clarification/correction.
// Returns false if the channel is full (agent can't keep up).
func (a *AgentLoop) InjectUserMessage(text string) bool {
	select {
	case a.userInject <- text:
		return true
	default:
		return false // channel is full
	}
}

// Run is the main loop: think → parse tools → execute → repeat
func (a *AgentLoop) Run(input string) (string, error) {
	a.Context.Add(provider.UserMsg(input))
	return a.runLoop()
}

// RunWithMessages starts with ready messages
func (a *AgentLoop) RunWithMessages(messages []provider.Message) (string, error) {
	a.Context.Messages = append(a.Context.Messages, messages...)
	return a.runLoop()
}

// runLoop is the internal agent loop
func (a *AgentLoop) runLoop() (string, error) {
	if a.provider == nil {
		return "", i18n.E("errors.not_connected")
	}

	// Build tool list for function calling
	toolDefs := a.buildToolDefs()

	for iteration := 1; ; iteration++ {
		// Check iteration limit (for subagents)
		if a.maxIterations > 0 && iteration > a.maxIterations {
			lastText := ""
			if len(a.Context.Messages) > 0 {
				lastText = a.Context.Messages[len(a.Context.Messages)-1].GetText()
			}
			return lastText, nil
		}

		// Debug mode: log request
		if a.debug {
			a.debugLog("→ Request", map[string]any{
				"iteration": iteration,
				"messages":  a.Context.Messages,
				"tools":     toolDefs,
			})
		}

		// Get response from provider with retry
		var result *provider.CompletionResult
		var err error
		for retry := 0; retry < 3; retry++ {
			result, err = a.provider.Complete(a.Context.Messages, toolDefs)
			if err == nil {
				break
			}
			if a.verbose {
				logger.Debug("retry", "attempt", retry+1, "provider", a.provider.Name(), "error", err)
			}
			// Exponential backoff: 200ms -> 400ms -> 800ms + jitter
			backoff := time.Duration(200*(1<<retry)) * time.Millisecond
			jitter := time.Duration(retry*100) * time.Millisecond
			time.Sleep(backoff + jitter)
		}
		if err != nil {
			return "", i18n.E("errors_provider.request", a.provider.Name(), err)
		}

		// Debug mode: log response
		if a.debug {
			a.debugLog("← Response", map[string]any{
				"iteration":   iteration,
				"stop_reason": result.StopReason,
				"message":     result.Message,
				"usage":       result.Usage,
			})
		}

		// Add assistant response to context
		a.Context.Messages = append(a.Context.Messages, result.Message)

		// Check thinking loop
		thinkingText := result.Message.GetThinking()
		if a.LoopDetector != nil && thinkingText != "" {
			if isLoop, msg := a.LoopDetector.RecordThinking(thinkingText); isLoop {
				// In synchronous mode — abort with error
				return result.Message.GetText(), errors.New(msg)
			}
		}

		if a.verbose {
			logger.Debug("iteration", "num", iteration, "stop_reason", result.StopReason)
			logger.Debug("response", "text", result.Message.GetText())
		}

		// Check if there are tool calls
		toolCalls := result.Message.GetToolCalls()

		// Check: no toolCalls OR toolCalls with empty parameters (Ollama bug)
		needFallback := len(toolCalls) == 0
		if !needFallback {
			for _, tc := range toolCalls {
				if len(tc.Input) == 0 {
					needFallback = true
					break
				}
			}
		}
		if needFallback {
			// Check XML/JSON tool calls in text (fallback)
			text := result.Message.GetText()
			parsedCalls := ParseToolCalls(text)
			if len(parsedCalls) == 0 {
				if len(toolCalls) == 0 {
					// Check text response loop
					if a.LoopDetector != nil {
						if isLoop, msg := a.LoopDetector.RecordTextResponse(text); isLoop {
							return text, errors.New(msg)
						}
					}
					return text, nil
				}
				// toolCalls exist but with empty parameters — and no XML fallback
				// Leave as is, tools will return an error
			} else {
				// Have XML/JSON tool calls — convert and execute
				toolCalls = convertParsedToContentBlocks(parsedCalls)
			}
		}

		// Execute all tool calls
		for _, tc := range toolCalls {
			tool, ok := a.Tools[tc.ToolName]
			if !ok {
				errMsg := i18n.T("errors.unknown_tool", tc.ToolName)
				logger.Error("unknown tool requested", "tool", tc.ToolName)
				a.Context.Messages = append(a.Context.Messages, provider.ToolResultMsg(
					tc.ToolUseID, tc.ToolName, "ERROR: "+errMsg, true,
				))
				continue
			}

			// Check parameter parsing error (from providers like Anthropic/z.ai)
			if parseErr, hasErr := tc.Input["_parse_error"]; hasErr {
				errMsg := fmt.Sprintf("%v", parseErr)
				if rawInput, hasRaw := tc.Input["_raw_input"]; hasRaw {
					errMsg += fmt.Sprintf("\nRaw input: %v", rawInput)
				}
				logger.Error("tool parse error", "tool", tc.ToolName, "error", errMsg)
				a.Context.Messages = append(a.Context.Messages, provider.ToolResultMsg(
					tc.ToolUseID, tc.ToolName, "ERROR: "+errMsg, true,
				))
				continue
			}
			// Convert parameters
			params := convertInputToParams(tc.Input)
			if a.verbose {
				logger.Debug("execute_tool", "name", tc.ToolName, "input", tc.Input, "params", params)
			}

			// Check permissions
			if a.PermissionChecker != nil {
				req := PermissionRequest{
					ToolName: tc.ToolName,
					Params:   params,
					Reason:   i18n.T("agent.permission_reason", tc.ToolName),
					Level:    ToolPermissionLevel(tc.ToolName),
				}
				result := a.PermissionChecker.CheckPermission(req)
				if result == PermDenied {
					logger.Warn("tool forbidden", "tool", tc.ToolName, "level", req.Level)
					a.Context.Messages = append(a.Context.Messages, provider.ToolResultMsg(
						tc.ToolUseID, tc.ToolName,
						i18n.T("errors.forbidden", req.Level), true,
					))
					continue
				}
			}

			if a.verbose {
				logger.Debug("call_tool", "tool", tc.ToolName, "params", params)
			}

			start := time.Now()
			toolResult := tool.Execute(params)
			elapsed := time.Since(start)

			logger.Debug("tool executed",
				"tool", tc.ToolName,
				"duration", elapsed.String(),
				"error", toolResult.Error != "",
			)

			toolOK := toolResult.Error == ""
			if toolResult.Error != "" {
				logger.Error("tool error", "tool", tc.ToolName, "error", toolResult.Error, "duration", elapsed)
				a.Context.Messages = append(a.Context.Messages, provider.ToolResultMsg(
					tc.ToolUseID, tc.ToolName, "ERROR: "+toolResult.Error, true,
				))
			} else {
				if a.verbose {
					logger.Debug("tool_result", "tool", tc.ToolName, "duration", elapsed, "output", truncate(toolResult.Output, 500))
				}
				a.Context.Messages = append(a.Context.Messages, provider.ToolResultMsg(
					tc.ToolUseID, tc.ToolName, toolResult.Output, false,
				))
			}

			// Check for loop
			if a.LoopDetector != nil {
				if isLoop, msg := a.LoopDetector.RecordToolCall(tc.ToolName, params, toolOK); isLoop {
					// Return current response text + loop message
					return result.Message.GetText(), errors.New(msg)
				}
			}
		}
	}
}

// Stream is the streaming agent loop
func (a *AgentLoop) Stream(input string) (<-chan provider.StreamEvent, error) {
	a.Context.Add(provider.UserMsg(input))
	return a.streamLoopWithCtx(context.Background())
}

// StreamWithCancel is the streaming loop with context cancellation support
func (a *AgentLoop) StreamWithCancel(ctx context.Context, input string) (<-chan provider.StreamEvent, error) {
	a.Context.Add(provider.UserMsg(input))
	return a.streamLoopWithCtx(ctx)
}

func (a *AgentLoop) streamLoopWithCtx(ctx context.Context) (<-chan provider.StreamEvent, error) {
	if a.provider == nil {
		return nil, i18n.E("errors.not_connected")
	}

	eventCh := make(chan provider.StreamEvent, 100)

	// Disable auto-compaction in Add() — will do it explicitly
	// to send compaction event to UI
	a.Context.AutoCompact = false

	go func() {
		defer func() {
			a.Context.AutoCompact = true
			close(eventCh)
		}()

		totalStart := time.Now()

		for iteration := 1; ; iteration++ {
			// Check iteration limit (for subagents)
			if a.maxIterations > 0 && iteration > a.maxIterations && iteration <= a.maxIterations+1 {
				// Instead of just taking last message text,
				// inject a summary request so the model produces a final answer
				a.Context.Add(provider.UserMsg(
					"You have reached the iteration limit. Please provide a concise summary of what you accomplished and what you found. Do not make any more tool calls.",
				))
				// One more iteration for the summary
				continue
			}
			// Hard limit: if we're way past maxIterations, force stop
			if a.maxIterations > 0 && iteration > a.maxIterations+2 {
				lastText := ""
				if len(a.Context.Messages) > 0 {
					lastText = a.Context.Messages[len(a.Context.Messages)-1].GetText()
				}
				if lastText != "" {
					eventCh <- provider.StreamEvent{Type: provider.EventTextDelta, Text: lastText}
				}
				eventCh <- provider.StreamEvent{Type: provider.EventDone, Duration: time.Since(totalStart)}
				return
			}

			// Check cancellation context before each iteration
			select {
			case <-ctx.Done():
				eventCh <- provider.StreamEvent{Type: provider.EventError, Error: ctx.Err()}
				return
			default:
			}

			// Check user comment injection
			a.drainUserInject(eventCh)

			iterStart := time.Now()

			// Send iteration start event
			eventCh <- provider.StreamEvent{
				Type:      provider.EventIterationStart,
				Iteration: iteration,
			}

			// Debug mode: log request
			if a.debug {
				a.debugLog("→ Stream Request", map[string]any{
					"iteration": iteration,
					"messages":  a.Context.Messages,
					"tools":     a.buildToolDefs(),
				})
			}

			// Get threaded response with retry
			stream, err := a.streamRetryRequest(ctx)
			if err != nil {
				eventCh <- provider.StreamEvent{Type: provider.EventError, Error: err}
				return
			}

			// Collect response from thread
			result := a.collectStreamResponse(ctx, stream, eventCh, iteration)
			if result.err != nil {
				eventCh <- provider.StreamEvent{Type: provider.EventError, Error: result.err}
				return
			}

			// Thinking loop — inject hint and continue
			if result.loopType == "thinking" {
				continue
			}

			// Have tool calls — execute
			if !result.done {
				if a.handleStreamToolCalls(ctx, result.toolCalls, eventCh) {
					continue
				}
				continue // Next iteration
			}

			// Final response (no tool calls)
			continueLoop, _ := a.handleStreamFinalResponse(
				result.text.String(), iteration, totalStart, iterStart, eventCh,
			)
			if continueLoop {
				continue
			}
			return
		}
	}()

	return eventCh, nil
}

// buildToolDefs builds tool list for function calling
func (a *AgentLoop) buildToolDefs() []provider.ToolDef {
	var defs []provider.ToolDef
	for name, tool := range a.Tools {
		defs = append(defs, provider.ToolDef{
			Name:        name,
			Description: tool.Description(),
			Parameters:  tool.Parameters(),
		})
	}
	return defs
}

// convertParsedToContentBlocks converts ToolCall from parser to ContentBlock
func convertParsedToContentBlocks(calls []ToolCall) []provider.ContentBlock {
	var blocks []provider.ContentBlock
	for _, tc := range calls {
		input := make(map[string]any)
		for k, v := range tc.Params {
			input[k] = v
		}
		blocks = append(blocks, provider.ContentBlock{
			Type:      "tool_use",
			ToolUseID: fmt.Sprintf("call_%s_%d", tc.Name, time.Now().UnixNano()),
			ToolName:  tc.Name,
			Input:     input,
		})
	}
	return blocks
}

// convertInputToParams converts map[string]any to map[string]string.
// Simple values (strings, numbers) are converted via fmt.Sprintf("%v", v),
// and complex ones (arrays, nested objects) are marshaled to JSON,
// so tools like todo_write receive valid JSON instead of Go syntax.
func convertInputToParams(input map[string]any) map[string]string {
	params := make(map[string]string)
	for k, v := range input {
		switch val := v.(type) {
		case string:
			params[k] = val
		case nil:
			params[k] = ""
		default:
			// Arrays, nested objects, numbers, bool — marshal to JSON
			if b, err := json.Marshal(v); err == nil {
				params[k] = string(b)
			} else {
				params[k] = fmt.Sprintf("%v", v)
			}
		}
	}
	return params
}

// parseToolInput parses JSON-line input tool
func parseToolInput(s string) map[string]any {
	s = strings.TrimSpace(s)
	if s == "" {
		return make(map[string]any)
	}
	// Try parsing as JSON
	var result map[string]any
	if err := json.Unmarshal([]byte(s), &result); err == nil {
		return result
	}
	// If not JSON — return as is
	return map[string]any{"input": s}
}

// BuildSystemPrompt builds system prompt with tool descriptions
// and agent instructions from AGENT.md, CLAUDE.md, .cursorrules, etc.
func BuildSystemPrompt(projectDir string, toolList map[string]tools.Tool) string {
	var sb strings.Builder

	sb.WriteString(i18n.T("system_prompt.intro"))
	sb.WriteString(i18n.T("system_prompt.capabilities"))
	sb.WriteString(i18n.T("system_prompt.help"))

	if projectDir != "" {
		sb.WriteString(fmt.Sprintf(i18n.T("system_prompt.workdir"), projectDir))
	}

	sb.WriteString(i18n.T("system_prompt.tools_header"))
	for name, tool := range toolList {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", name, tool.Description()))
	}

	sb.WriteString(i18n.T("system_prompt.tool_usage"))
	sb.WriteString(i18n.T("system_prompt.tool_format"))
	sb.WriteString(i18n.T("system_prompt.xml_format"))
	sb.WriteString(i18n.T("system_prompt.xml_example"))
	sb.WriteString(i18n.T("system_prompt.json_format"))
	sb.WriteString(i18n.T("system_prompt.json_example") + "\n")

	sb.WriteString(i18n.T("system_prompt.examples_header"))
	sb.WriteString(i18n.T("system_prompt.example_read"))
	sb.WriteString(i18n.T("system_prompt.example_write"))
	sb.WriteString(i18n.T("system_prompt.example_bash"))
	sb.WriteString("\n")

	sb.WriteString(i18n.T("system_prompt.rules"))
	sb.WriteString(i18n.T("system_prompt.rule1"))
	sb.WriteString(i18n.T("system_prompt.rule2"))
	sb.WriteString(i18n.T("system_prompt.rule3"))
	sb.WriteString(i18n.T("system_prompt.rule4"))
	sb.WriteString(i18n.T("system_prompt.rule5"))
	sb.WriteString(i18n.T("system_prompt.rule6"))
	sb.WriteString(i18n.T("system_prompt.rule7"))
	sb.WriteString(i18n.T("system_prompt.rule8"))
	sb.WriteString(i18n.T("system_prompt.rule9"))
	sb.WriteString(i18n.T("system_prompt.rule10"))

	// Load agent instructions from AGENT.md, CLAUDE.md, .cursorrules, etc.
	if projectDir != "" {
		instructions := LoadAgentInstructions(projectDir)
		if len(instructions) > 0 {
			sb.WriteString("\n\n---\n\n")
			sb.WriteString(i18n.T("system_prompt.instructions_header"))
			sb.WriteString("\n\n")
			sb.WriteString(FormatInstructions(instructions))
		}
	}

	return sb.String()
}

// maybeCompact checks if compaction is needed and sends event to UI
func (a *AgentLoop) maybeCompact(eventCh chan<- provider.StreamEvent, ctx context.Context) {
	if a.Context.SkipCompaction {
		return
	}
	// Check cancellation context before compaction
	select {
	case <-ctx.Done():
		return
	default:
	}
	// Check if compaction is needed
	tokenCount := a.Context.TokenCount()
	if tokenCount <= a.Context.MaxTokens {
		return
	}
	// Context exceeds limit — compaction is required, reset anti-thrashing
	a.Context.lowSaveCount = 0
	// Non-blocking send of compaction start events
	select {
	case eventCh <- provider.StreamEvent{Type: provider.EventCompaction}:
	default:
		// Channel is full — skip UI notification, but still compact
	}
	// Perform compaction with cancellation context
	a.Context.Ctx = ctx
	a.Context.Compact()
	// Check cancellation context after compaction
	select {
	case <-ctx.Done():
		// Context cancelled — send compaction completion event and exit
		select {
		case eventCh <- provider.StreamEvent{Type: provider.EventCompactionDone}:
		default:
		}
		return
	default:
	}
	// Non-blocking send of compaction completion events
	select {
	case eventCh <- provider.StreamEvent{Type: provider.EventCompactionDone}:
	default:
	}
	// AfterCompact callback — inject memory facts after compaction
	if a.Context.AfterCompact != nil {
		a.Context.AfterCompact()
	}
}

// drainUserInject extracts all user comments from the channel
// and adds them to context as user messages.
// Called between agent loop iterations.
func (a *AgentLoop) drainUserInject(eventCh chan<- provider.StreamEvent) {
	for {
		select {
		case text := <-a.userInject:
			if text == "" {
				continue
			}
			// Add to context as user message
			a.Context.Add(provider.UserMsg(text))
			// Notify UI that comment was injected
			eventCh <- provider.StreamEvent{
				Type: provider.EventUserInjected,
				Text: text,
			}
		default:
			return // Channel is empty
		}
	}
}

// truncate truncates a line to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// debugLog writes debug information to .bugbuster/debug.log
func (a *AgentLoop) debugLog(prefix string, data map[string]any) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}

	// If directory is specified — write to file
	if a.debugDir != "" {
		logPath := filepath.Join(a.debugDir, "debug.log")
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			// Fallback to logger if file cannot be opened
			logger.Debug("debug_log", "prefix", prefix, "data", string(jsonData))
			return
		}
		defer f.Close()

		timestamp := time.Now().Format("2006-01-02 15:04:05")
		fmt.Fprintf(f, "[%s] %s:\n%s\n\n", timestamp, prefix, string(jsonData))
		return
	}

	// Fallback — console
	logger.Debug("debug_log", "prefix", prefix, "data", string(jsonData))
}
