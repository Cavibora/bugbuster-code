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
	fallbackProvider  provider.Provider // fallback provider when primary fails
	fallbackMaxRetries int              // max retries on primary before fallback (default: 2)
	fallbackRetryDelay time.Duration    // delay between retries (default: 1s)
	fallbackAutoSwitchBack bool         // switch back to primary when it recovers (default: true)
	verbose           bool
	debug             bool   // debug mode: log request/response JSON
	debugDir          string // directory for debug logs (.bugbuster)
	nonInteractive    bool   // exec mode: auto-skip ask_user
	PermissionChecker PermissionChecker
	LoopDetector      *LoopDetector // loop detector
	SkillManager      *skills.Manager // skill system

	// LLM request timeouts
	RequestTimeout  time.Duration // max time for a single LLM request (0 = 40 min)
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

	// autoContinueCount limits auto-continues (max 3)
	autoContinueCount int

	// providerMaxTokens is the provider's output token limit (max_tokens sent to API)
	// Used for display in warning messages. 0 = unknown.
	providerMaxTokens int

	// autoContinue enables auto-continuation when model responds with text only
	// Only enabled in TUI mode, not in sync mode (Run)
	autoContinue bool

	// Speed mirror — tracks iteration durations for self-awareness
	iterDurations       []time.Duration
	lastMirrorAt        int // iteration when mirror was last injected
	lastAutoCompactAt   int // iteration when last auto-compact happened
	autoCompactCooldown int // iterations to skip after auto-compact (prevents loop)
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

	// compact_force — registered later when context is fully initialized
	// (needs CompactForceContext interface, which is the ConversationContext)
	a.RegisterTool(tools.NewCompactForceTool(a.Context))

	// self_info — model can query its own identity and environment
	a.RegisterTool(tools.NewSelfInfoTool(&tools.SelfInfoWrapper{
		Provider: p,
		Ctx: &tools.SelfInfoCtxAdapter{
			TokenCountFn:      a.Context.TokenCount,
			MaxTokensValueFn:  a.Context.MaxTokensValue,
			GetSystemPromptFn: a.Context.GetSystemPrompt,
			MessageCountFn:    a.Context.MessageCount,
		},
	}))

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

// SetProviderMaxTokens sets the provider's output token limit
func (a *AgentLoop) SetProviderMaxTokens(maxTokens int) {
	a.providerMaxTokens = maxTokens
}

// SetKeepRecent sets the count of recent messages to keep during compaction
func (a *AgentLoop) SetKeepRecent(n int) {
	a.Context.KeepRecent = n
}

// SetProvider sets the provider
func (a *AgentLoop) SetProvider(p provider.Provider) {
	a.provider = p
}

// SetFallbackProvider sets the fallback provider (used when primary fails)
func (a *AgentLoop) SetFallbackProvider(p provider.Provider) {
	a.fallbackProvider = p
}

// SetFallbackConfig configures fallback behavior
func (a *AgentLoop) SetFallbackConfig(maxRetries int, retryDelay time.Duration, autoSwitchBack bool) {
	if maxRetries > 0 {
		a.fallbackMaxRetries = maxRetries
	}
	if retryDelay > 0 {
		a.fallbackRetryDelay = retryDelay
	}
	a.fallbackAutoSwitchBack = autoSwitchBack
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
	return 40 * time.Minute
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

// effectiveMaxTokens returns the effective max_tokens limit for display in messages.
// Returns provider's output token limit if set, otherwise context window size.
func (a *AgentLoop) effectiveMaxTokens() int {
	if a.providerMaxTokens > 0 {
		return a.providerMaxTokens
	}
	if a.Context.MaxTokens > 0 {
		return a.Context.MaxTokens
	}
	return 16384
}

// SetAutoContinue enables auto-continuation when model responds with text only.
// Only enabled in TUI mode, not in sync mode (Run).
func (a *AgentLoop) SetAutoContinue(v bool) {
	a.autoContinue = v
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
	if a.Context.OriginalTask == "" {
		a.Context.OriginalTask = input
	}
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

		// Get response from provider with retry and fallback
		var result *provider.CompletionResult
		var err error
		maxRetries := a.fallbackMaxRetries
		if maxRetries <= 0 {
			maxRetries = 2
		}
		retryDelay := a.fallbackRetryDelay
		if retryDelay <= 0 {
			retryDelay = time.Second
		}

		// Try primary provider
		for retry := 0; retry < maxRetries; retry++ {
			result, err = a.provider.Complete(a.Context.Messages, toolDefs)
			if err == nil {
				break
			}
			if a.verbose {
				logger.Debug("retry", "attempt", retry+1, "provider", a.provider.Name(), "error", err)
			}
			time.Sleep(retryDelay)
		}

		// Primary failed — try fallback provider
		if err != nil && a.fallbackProvider != nil {
			if a.verbose {
				logger.Debug("fallback_switch", "from", a.provider.Name(), "to", a.fallbackProvider.Name())
			}
			for retry := 0; retry < maxRetries; retry++ {
				result, err = a.fallbackProvider.Complete(a.Context.Messages, toolDefs)
				if err == nil {
					break
				}
				if a.verbose {
					logger.Debug("fallback_retry", "attempt", retry+1, "provider", a.fallbackProvider.Name(), "error", err)
				}
				time.Sleep(retryDelay)
			}
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

		// max_tokens: continue truncated text response
		// No limit — compaction handles context overflow automatically
		if result.StopReason == "max_tokens" {
			toolCalls := result.Message.GetToolCalls()
			if len(toolCalls) == 0 {
				// Check for XML/JSON tool calls in text
				text := result.Message.GetText()
				parsedCalls := ParseToolCalls(text)
				if len(parsedCalls) == 0 {
					// Pure text response was truncated — send "Continue."
					if a.verbose {
						logger.Debug("max_tokens_continue", "msg", "sending Continue.")
					}
					a.Context.Add(provider.UserMsg("Continue."))
					continue
				}
			}
			// Has tool calls — fall through to normal processing
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
				// Auto-continue: model responded without tool calls (max 3 times)
				// Only when auto-continue is enabled (TUI mode)
				// Skip if the response looks like a genuine completion (recap, "done", short answer)
				completionDetected := LooksLikeCompletion(text)
				if a.verbose {
					logger.Debug("auto_continue_check",
						"autoContinue", a.autoContinue,
						"count", a.autoContinueCount,
						"textLen", len(text),
						"completionDetected", completionDetected,
						"textPreview", truncate(text, 100),
					)
				}
				if a.autoContinue && a.autoContinueCount < 3 && a.Context != nil && !completionDetected {
						a.autoContinueCount++
						continueHint := "Continue working. Use tools to read files, run commands, or search code. Do not just describe what to do — actually do it using tools."
						if a.Context.OriginalTask != "" {
							continueHint = fmt.Sprintf(
								"You responded with text only, but your task is not done yet.\n"+
									"Original task: %s\n\n"+
									"Continue working — use tools (read, bash, grep, edit, etc.) to make progress. "+
									"Do NOT just describe what needs to be done. Actually DO it.",
								a.Context.OriginalTask,
							)
						}
						a.Context.Add(provider.UserMsg(continueHint))
						continue
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
			// Normalize tool name (handles PascalCase, aliases, typos)
			normalizedToolName := normalizeToolName(tc.ToolName)
			tool, ok := a.Tools[normalizedToolName]
			if !ok {
				// Try fuzzy match using Levenshtein distance
				availableTools := make(map[string]bool)
				for name := range a.Tools {
					availableTools[name] = true
				}
				if closest, found := findClosestToolName(tc.ToolName, availableTools); found {
					normalizedToolName = closest
					tool = a.Tools[closest]
					ok = true
				}
			}
			if !ok {
				errMsg := i18n.T("errors.unknown_tool", tc.ToolName)
				logger.Error("unknown tool requested", "tool", tc.ToolName)
				// List available tools for helpful error message
				availableTools := make([]string, 0, len(a.Tools))
				for name := range a.Tools {
					availableTools = append(availableTools, name)
				}
				hint := fmt.Sprintf("\n\nAvailable tools: %s\nCorrect format: %s",
					strings.Join(availableTools, ", "),
					getToolCallHint("read"))
				a.Context.Messages = append(a.Context.Messages, provider.ToolResultMsg(
					tc.ToolUseID, tc.ToolName, "ERROR: "+errMsg+hint, true,
				))
				continue
			}

			// Use normalized name for tool execution
			tc.ToolName = normalizedToolName

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
				// Check if tool result contains image data (e.g., screenshot)
				if base64Data, format, ok := tools.ExtractImageFromResult(toolResult); ok {
					// Create a tool_result with image content block
					imageBlock := provider.ContentBlock{
						Type:        "image",
						ImageSource: base64Data,
						ImageFormat: format,
					}
					textBlock := provider.ContentBlock{
						Type: "text",
						Text: toolResult.Output,
					}
					// For Anthropic: tool_result can contain content blocks
					a.Context.Messages = append(a.Context.Messages, provider.Message{
						Role: "user",
						Content: []provider.ContentBlock{
							{
								Type:      "tool_result",
								ToolUseID: tc.ToolUseID,
								Output:    toolResult.Output,
							},
							textBlock,
							imageBlock,
						},
					})
					// Also add image as separate user message for models that need it
					a.Context.Messages = append(a.Context.Messages, provider.UserImageTextMsg(
						fmt.Sprintf("[Screenshot from tool %s]", tc.ToolName),
						base64Data, format,
					))
				} else {
					if a.verbose {
						logger.Debug("tool_result", "tool", tc.ToolName, "duration", elapsed, "output", truncate(toolResult.Output, 500))
					}
					a.Context.Messages = append(a.Context.Messages, provider.ToolResultMsg(
						tc.ToolUseID, tc.ToolName, toolResult.Output, false,
					))
				}
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
	if a.Context.OriginalTask == "" {
		a.Context.OriginalTask = input
	}
	a.Context.Add(provider.UserMsg(input))
	return a.streamLoopWithCtx(context.Background())
}

// StreamWithCancel is the streaming loop with context cancellation support
func (a *AgentLoop) StreamWithCancel(ctx context.Context, input string) (<-chan provider.StreamEvent, error) {
	if a.Context.OriginalTask == "" {
		a.Context.OriginalTask = input
	}
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

			// Compact context if needed before sending request
			// This prevents context overflow from max_tokens continuations
			a.maybeCompact(eventCh, ctx)

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
					// Inject speed mirror after tool execution
					a.injectSpeedMirror(iteration, time.Since(iterStart))
					continue
				}
				// Inject speed mirror after tool execution
				a.injectSpeedMirror(iteration, time.Since(iterStart))
				continue // Next iteration
			}

			// Final response (no tool calls)
			// max_tokens: continue truncated text — no limit, compaction handles overflow
			if result.maxTokens {
				if a.verbose {
					logger.Debug("max_tokens_continue_stream", "msg", "sending Continue.")
				}
				// Send iteration end so TUI can reset state (textReceived, spinner)
				eventCh <- provider.StreamEvent{
					Type:      provider.EventIterationEnd,
					Iteration: iteration,
					Duration:  time.Since(iterStart),
				}
				// Assistant message already added to context by collectStreamResponse
				// Just send "Continue." — model will pick up where it left off
				// Text from next response will be streamed seamlessly to user
				a.Context.Add(provider.UserMsg("Continue."))
				continue
			}

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

	// Current system time — model needs this for reports and timestamps
	now := time.Now()
	sb.WriteString(fmt.Sprintf("Current date and time: %s\n", now.Format("2006-01-02 15:04:05 MST")))
	sb.WriteString(fmt.Sprintf("Current year: %d\n\n", now.Year()))

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
	sb.WriteString(i18n.T("system_prompt.rule0"))
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
	sb.WriteString(i18n.T("system_prompt.rule11"))
	sb.WriteString(i18n.T("system_prompt.rule12"))

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
	// Cooldown after CompactForce — skip compaction for 3 iterations
	// to prevent double compaction (tool call + auto-compact)
	if a.autoCompactCooldown > 0 {
		a.autoCompactCooldown--
		return
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

	// Inject original task reminder after compaction so the model
	// doesn't lose track of what it was doing
	if a.Context.OriginalTask != "" {
		taskReminder := fmt.Sprintf(
			"[Context was compacted to save space. Your original task: %s\n"+
				"Continue working on this task. Do NOT stop or say you are done unless the task is truly complete.]",
			a.Context.OriginalTask,
		)
		a.Context.Add(provider.UserMsg(taskReminder))
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

// injectSpeedMirror adds a system message with performance statistics
// so the model can self-assess its speed and decide to compact.
// Injected every 5 iterations or when speed degrades significantly.
// When slowdown is critical (>3x), auto-compacts to prevent degradation.
func (a *AgentLoop) injectSpeedMirror(iteration int, iterDuration time.Duration) {
	a.iterDurations = append(a.iterDurations, iterDuration)

	// Inject mirror every 5 iterations
	if iteration-a.lastMirrorAt < 5 {
		return
	}
	a.lastMirrorAt = iteration

	if len(a.iterDurations) < 2 {
		return
	}

	// Calculate average duration of last iterations
	recent := a.iterDurations
	if len(recent) > 10 {
		recent = recent[len(recent)-10:]
	}
	var sum time.Duration
	for _, d := range recent {
		sum += d
	}
	avg := sum / time.Duration(len(recent))

	// Compare first half vs second half to detect slowdown
	mid := len(recent) / 2
	if mid == 0 {
		return
	}
	var firstHalf, secondHalf time.Duration
	for i, d := range recent {
		if i < mid {
			firstHalf += d
		} else {
			secondHalf += d
		}
	}
	firstAvg := firstHalf / time.Duration(mid)
	secondAvg := secondHalf / time.Duration(len(recent)-mid)

	slowdownRatio := 0.0
	if firstAvg > 0 {
		slowdownRatio = float64(secondAvg) / float64(firstAvg)
	}

	tokenCount := a.Context.TokenCount()
	maxTokens := a.Context.MaxTokens
	contextUsage := 0
	if maxTokens > 0 {
		contextUsage = tokenCount * 100 / maxTokens
	}

	// Auto-compact when slowdown is critical (>3x) and context is large (>50%)
	// This handles the case when model is "in the flow" and ignores mirror warnings
	// Cooldown: skip auto-compact for 10 iterations after last one to prevent loops
	if slowdownRatio > 3.0 && contextUsage > 50 && maxTokens > 0 {
		iterationsSinceLastCompact := iteration - a.lastAutoCompactAt
		if a.lastAutoCompactAt == 0 || iterationsSinceLastCompact >= 10 {
			a.lastAutoCompactAt = iteration
			a.Context.CompactForce()
			compactedTokens := a.Context.TokenCount()
			compactedUsage := 0
			if maxTokens > 0 {
				compactedUsage = compactedTokens * 100 / maxTokens
			}
			a.Context.Add(provider.SystemMsg(fmt.Sprintf(
				"[Auto-compacted] Context was critically slow (%.1fx degradation, %d%% usage). "+
					"Compacted from %d%% to %d%%. Continue working on your task.",
				slowdownRatio, contextUsage, contextUsage, compactedUsage,
			)))
			// Reset speed tracking after compaction, keep last 3 for baseline
			if len(a.iterDurations) > 3 {
				a.iterDurations = a.iterDurations[len(a.iterDurations)-3:]
			}
			a.lastMirrorAt = iteration
			a.autoCompactCooldown = 3 // skip maybeCompact for 3 iterations
			return
		}
		// Cooldown active — don't auto-compact again yet
	}

	var sb strings.Builder
	sb.WriteString("[Performance mirror] ")
	sb.WriteString(fmt.Sprintf("Iteration %d. ", iteration))
	sb.WriteString(fmt.Sprintf("Avg iteration time: %s. ", avg.Truncate(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("Context: %d/%d tokens (%d%%). ", tokenCount, maxTokens, contextUsage))

	if slowdownRatio > 1.5 {
		sb.WriteString(fmt.Sprintf("⚠ SLOWDOWN detected: %.1fx slower than initial. ", slowdownRatio))
		sb.WriteString("Consider using compact_force tool to reduce context and speed up responses.")
	} else if contextUsage > 70 {
		sb.WriteString("Context is getting large. Consider using compact_force tool to reduce context.")
	} else {
		sb.WriteString("Performance is stable.")
	}

	// Report stale background processes
	if staleMsg := a.checkStaleProcesses(); staleMsg != "" {
		sb.WriteString("\n")
		sb.WriteString(staleMsg)
	}

	a.Context.Add(provider.SystemMsg(sb.String()))
}

// ResetSpeedTracking resets speed tracking after CompactForce.
// This prevents double compaction (tool call + auto-compact).
func (a *AgentLoop) ResetSpeedTracking() {
	a.iterDurations = nil
	a.lastMirrorAt = 0
	a.lastAutoCompactAt = 0
	a.autoCompactCooldown = 3 // skip maybeCompact for 3 iterations after CompactForce
}

// checkStaleProcesses checks for long-running background processes
// and returns a warning message if any are found.
// Also auto-kills processes that have been running for more than 30 minutes.
func (a *AgentLoop) checkStaleProcesses() string {
	psTool, ok := a.Tools["ps"]
	if !ok {
		return ""
	}
	ps, ok := psTool.(*tools.PSTool)
	if !ok {
		return ""
	}

	processes := ps.ListProcesses()
	if len(processes) == 0 {
		return ""
	}

	var stale []string
	var longRunning []string
	for _, p := range processes {
		if !p.Running.Load() {
			continue
		}
		uptime := time.Since(p.StartTime)
		if uptime > 7*24*time.Hour {
			// Auto-kill processes running more than 7 days (truly stuck)
			killTool, ok := a.Tools["kill"]
			if ok {
				kill, ok := killTool.(*tools.KillProcessTool)
				if ok {
					_ = kill.Execute(map[string]string{"id": fmt.Sprintf("%d", p.ID)})
					longRunning = append(longRunning, fmt.Sprintf("  • Auto-killed process #%d (PID %d, uptime %s): %s",
						p.ID, p.PID, uptime.Truncate(time.Hour), truncate(p.Command, 50)))
				}
			}
		} else if uptime > 1*time.Hour {
			stale = append(stale, fmt.Sprintf("  • Process #%d (PID %d, uptime %s): %s — use kill tool to terminate if no longer needed",
				p.ID, p.PID, uptime.Truncate(time.Minute), truncate(p.Command, 50)))
		}
	}

	var sb strings.Builder
	if len(longRunning) > 0 {
		sb.WriteString("⚠ Auto-killed stuck background processes (>7 days):\n")
		sb.WriteString(strings.Join(longRunning, "\n"))
	}
	if len(stale) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("ℹ Long-running background processes (>1 hour) — consider killing if not needed:\n")
		sb.WriteString(strings.Join(stale, "\n"))
	}
	return sb.String()
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

// LooksLikeCompletion checks if the model's text response indicates
// that the task is genuinely complete and auto-continue would be wasteful.
// This prevents spending tokens on unnecessary continuation after:
// - Recap summaries (※ Recap:, Recap:, Итог:, Summary:)
// - Explicit completion signals ("Готово", "Done", etc.)
// - Short answers to informational questions
func LooksLikeCompletion(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}

	// Check for recap/summary markers — model has finished and summarized
	// Match various formats: "※ Recap:", "Recap:", "Итог:", "Summary:"
	// Also match without colon: "※ Recap", "Recap", etc.
	recapMarkers := []string{
		"※ recap:",
		"※ recap",
		"※ итог:",
		"※ итог",
		"recap:",
		"итог:",
		"summary:",
	}
	lower := strings.ToLower(text)
	for _, marker := range recapMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}

	// Check for explicit completion signals in various languages
	completionPhrases := []string{
		"всё готово", "всё сделано", "задача выполнена",
		"всё работает", "всё работает корректно",
		"готово!", "готово.", "сделано!",
		"all done", "task complete", "everything works",
		"task is complete", "task is done", "work is done",
		"nothing more to do", "no more changes needed",
		"no further action", "all changes have been",
	}
	for _, phrase := range completionPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}

	// Short answers (< 200 chars) to informational questions — likely just an answer
	// Check if it looks like a direct answer (starts with common answer patterns)
	if len(text) < 200 {
		answerPrefixes := []string{"да", "нет", "yes", "no", "ok", "ок"}
		firstLine := text
		if idx := strings.Index(text, "\n"); idx >= 0 {
			firstLine = text[:idx]
		}
		firstLine = strings.TrimSpace(strings.ToLower(firstLine))
		for _, prefix := range answerPrefixes {
			if strings.HasPrefix(firstLine, prefix) {
				return true
			}
		}
	}

	return false
}
