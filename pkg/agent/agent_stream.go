package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/logger"
	"bugbuster-code/pkg/provider"
	"bugbuster-code/pkg/tools"
)

// streamResult — stream response assembly result.
// Contains text, thinking, tool calls and loop control flags.
type streamResult struct {
	text      strings.Builder
	thinking  strings.Builder
	toolCalls []provider.ContentBlock
	done      bool // true = final response (no tool calls)
	err       error
	loopMsg   string // loop message (empty = no loop)
	loopType  string // "thinking" | "text" | ""
	maxTokens bool  // true = response was truncated by max_tokens limit
}

// streamRetryRequest receives streaming response from provider with retry (3 attempts).
func (a *AgentLoop) streamRetryRequest(ctx context.Context) (<-chan provider.StreamEvent, error) {
	var stream <-chan provider.StreamEvent
	var err error
	for retry := 0; retry < 3; retry++ {
		stream, err = a.provider.StreamWithCtx(ctx, a.Context.Messages, a.buildToolDefs())
		if err == nil {
			return stream, nil
		}
		if a.verbose {
			logger.Debug("stream_retry", "attempt", retry+1, "max", 3, "provider", a.provider.Name(), "error", err)
		}
		time.Sleep(time.Duration(retry+1) * time.Second)
	}
	return nil, err
}

// collectStreamResponse assembles response from stream events.
// Handles: event parsing, timeouts (idle, thinking, request),
// fallback XML/JSON, assistant message formation, thinking loop check.
// Does NOT execute tool calls — returns them in result.toolCalls.
func (a *AgentLoop) collectStreamResponse(
	ctx context.Context,
	stream <-chan provider.StreamEvent,
	eventCh chan<- provider.StreamEvent,
	iteration int,
) streamResult {
	var result streamResult
	var currentToolID, currentToolName string
	var toolInputBuf strings.Builder

	idleTimeout := a.effectiveIdleTimeout()
	thinkingTimeout := a.effectiveThinkingTimeout()
	idleTimer := time.NewTimer(idleTimeout)
	thinkingTimer := time.NewTimer(thinkingTimeout)
	defer idleTimer.Stop()
	defer thinkingTimer.Stop()
	lastTokenTime := time.Now()

streamLoop:
	for {
		select {
		case event, ok := <-stream:
			if !ok {
				idleTimer.Stop()
				thinkingTimer.Stop()
				break streamLoop
			}
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			if !thinkingTimer.Stop() {
				select {
				case <-thinkingTimer.C:
				default:
				}
			}
			idleTimer.Reset(idleTimeout)
			thinkingTimer.Reset(thinkingTimeout)
			lastTokenTime = time.Now()
			a.idleTimeoutCount = 0 // reset on data received

			switch event.Type {
			case provider.EventTextDelta:
				result.text.WriteString(event.Text)
				eventCh <- event

			case provider.EventThinking:
				result.thinking.WriteString(event.Text)
				eventCh <- event

			case provider.EventToolCallStart:
				currentToolID = event.ToolCallID
				currentToolName = event.ToolName
				toolInputBuf.Reset()
				eventCh <- provider.StreamEvent{
					Type:       provider.EventToolCallStart,
					ToolName:   event.ToolName,
					ToolCallID: event.ToolCallID,
					Iteration:  iteration,
				}

			case provider.EventToolCallDelta:
				toolInputBuf.WriteString(event.ToolDelta)
				eventCh <- provider.StreamEvent{
					Type:       provider.EventToolCallDelta,
					ToolDelta:  event.ToolDelta,
					ToolName:   currentToolName,
					ToolCallID: currentToolID,
				}

			case provider.EventToolCallEnd:
				inputRaw := toolInputBuf.String()
				input := parseToolInput(inputRaw)
				if a.verbose {
					logger.Debug("tool_call_end", "tool", currentToolName, "id", currentToolID, "input_raw", inputRaw, "parsed", input)
				}
				result.toolCalls = append(result.toolCalls, provider.ContentBlock{
					Type:      "tool_use",
					ToolUseID: currentToolID,
					ToolName:  currentToolName,
					Input:     input,
				})
				currentToolID = ""
				currentToolName = ""

			case provider.EventUsage:
				// Forward usage data from provider to UI
				// Without this, totalInTokens stays 0 for OpenAI providers
				eventCh <- event

			case "stop_reason":
				if event.StopReason == "max_tokens" {
					// max_tokens is NOT a fatal error — model just didn't finish.
					// Accept the truncated response and continue normally.
					// No warning, no continuation hint — just log it.
					result.maxTokens = true
					if a.verbose {
						logger.Debug("max_tokens_truncated", "msg", "accepting truncated response")
					}
				}

			case provider.EventDone:
				// Debug mode: log response
				if a.debug {
					a.debugLog("← Stream Response", map[string]any{
						"iteration":  iteration,
						"thinking":   result.thinking.String(),
						"text":       result.text.String(),
						"tool_calls": len(result.toolCalls),
					})
				}

				// Fallback: check XML/JSON tool calls in text
				a.applyFallbackIfNeeded(&result)

				// Build assistant message
				var content []provider.ContentBlock
				if result.thinking.Len() > 0 {
					content = append(content, provider.ContentBlock{
						Type: "thinking",
						Text: result.thinking.String(),
					})
				}
				if result.text.Len() > 0 {
					content = append(content, provider.ContentBlock{
						Type: "text",
						Text: result.text.String(),
					})
				}
				content = append(content, result.toolCalls...)

				a.Context.Messages = append(a.Context.Messages, provider.Message{
					Role:    "assistant",
					Content: content,
				})

				// Check thinking loop
				if a.LoopDetector != nil && result.thinking.Len() > 0 {
					if isLoop, msg := a.LoopDetector.RecordThinking(result.thinking.String()); isLoop {
						eventCh <- provider.StreamEvent{Type: provider.EventTextDelta, Text: "\n" + msg + "\n"}
						a.Context.Messages = append(a.Context.Messages, provider.UserMsg(
							i18n.T("loop_detector.hint_thinking_loop"),
						))
						a.LoopDetector.Reset()
						result.loopMsg = msg
						result.loopType = "thinking"
						return result
					}
				}

				// If there are tool calls — not final response
				if len(result.toolCalls) > 0 {
					result.done = false
					return result
				}

				// Final response (no tool calls)
				result.done = true
				return result

			case provider.EventError:
				result.err = event.Error
				return result
			}

		case <-idleTimer.C:
			// Idle timeout — no events from provider for too long
			a.idleTimeoutCount++
			idleSec := int(idleTimeout.Seconds())
			if a.idleTimeoutCount >= 2 {
				// Second consecutive idle timeout — fatal error
				result.err = fmt.Errorf("%s (%d %s)", i18n.T("errors.stream_idle_timeout"), idleSec, i18n.T("time.seconds"))
				return result
			}
			// First idle timeout — warning, continue
			eventCh <- provider.StreamEvent{Type: provider.EventTextDelta, Text: fmt.Sprintf("\n⚠️ %s (%d %s). %s\n", i18n.T("errors.stream_idle_timeout"), idleSec, i18n.T("time.seconds"), i18n.T("errors.stream_idle_hint"))}
			a.Context.Messages = append(a.Context.Messages, provider.UserMsg(
				i18n.T("errors.stream_idle_hint"),
			))
			// Reset timers and continue
			idleTimer.Reset(idleTimeout)
			thinkingTimer.Reset(thinkingTimeout)
			lastTokenTime = time.Now()
			result.text.Reset()
			result.toolCalls = nil
			continue

		case <-thinkingTimer.C:
			// Thinking timeout — model thinking too long without tokens
			eventCh <- provider.StreamEvent{
				Type:     provider.EventThinkingTimeout,
				Duration: time.Since(lastTokenTime),
			}
			thinkingTimer.Reset(30 * time.Second)
		}
	}

	// Stream closed (stream channel closed) — handle as end of response
	// Fallback: check XML/JSON tool calls in text
	a.applyFallbackIfNeeded(&result)

	// Build assistant message
	var content []provider.ContentBlock
	if result.thinking.Len() > 0 {
		content = append(content, provider.ContentBlock{
			Type: "thinking",
			Text: result.thinking.String(),
		})
	}
	if result.text.Len() > 0 {
		content = append(content, provider.ContentBlock{
			Type: "text",
			Text: result.text.String(),
		})
	}
	content = append(content, result.toolCalls...)

	a.Context.Messages = append(a.Context.Messages, provider.Message{
		Role:    "assistant",
		Content: content,
	})

	// Check thinking loop
	if a.LoopDetector != nil && result.thinking.Len() > 0 {
		if isLoop, msg := a.LoopDetector.RecordThinking(result.thinking.String()); isLoop {
			eventCh <- provider.StreamEvent{Type: provider.EventTextDelta, Text: "\n" + msg + "\n"}
			a.Context.Messages = append(a.Context.Messages, provider.UserMsg(
				i18n.T("loop_detector.hint_thinking_loop"),
			))
			a.LoopDetector.Reset()
			result.loopMsg = msg
			result.loopType = "thinking"
			return result
		}
	}

	if len(result.toolCalls) > 0 {
		result.done = false
	} else {
		result.done = true
	}
	return result
}

// applyFallbackIfNeeded checks if XML/JSON fallback parsing is needed.
// If no toolCalls or toolCalls with empty parameters — parse text.
func (a *AgentLoop) applyFallbackIfNeeded(result *streamResult) {
	needFallback := len(result.toolCalls) == 0
	if !needFallback {
		for _, tc := range result.toolCalls {
			if len(tc.Input) == 0 {
				needFallback = true
				break
			}
		}
	}
	if !needFallback || result.text.Len() == 0 {
		return
	}

	parsedCalls := ParseToolCalls(result.text.String())
	if len(parsedCalls) == 0 {
		return
	}

	if a.verbose {
		logger.Debug("xml_json_fallback", "tool_calls", len(parsedCalls))
		for i, pc := range parsedCalls {
			logger.Debug("fallback_tool_call", "index", i, "name", pc.Name, "params", pc.Params)
		}
	}

	result.toolCalls = convertParsedToContentBlocks(parsedCalls)
	cleanText := StripToolCalls(result.text.String())
	result.text.Reset()
	result.text.WriteString(cleanText)
}

// handleStreamToolCalls executes all tool calls in streaming mode.
// Returns true if loop should continue (loop detection), false for normal execution.
func (a *AgentLoop) handleStreamToolCalls(
	ctx context.Context,
	toolCalls []provider.ContentBlock,
	eventCh chan<- provider.StreamEvent,
) (continueLoop bool) {
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
			// List available tools for helpful error message
			availableTools := make([]string, 0, len(a.Tools))
			for name := range a.Tools {
				availableTools = append(availableTools, name)
			}
			errMsg := i18n.T("errors.unknown_tool", tc.ToolName)
			hint := fmt.Sprintf("\n\nAvailable tools: %s\nCorrect format: %s",
				strings.Join(availableTools, ", "),
				getToolCallHint("read"))
			eventCh <- provider.StreamEvent{Type: provider.EventTextDelta, Text: "\n[ERROR: " + errMsg + hint + "]\n"}
			a.Context.Messages = append(a.Context.Messages, provider.ToolResultMsg(
				tc.ToolUseID, tc.ToolName, "ERROR: "+errMsg+hint, true,
			))
			continue
		}

		// Use normalized name for tool execution
		tc.ToolName = normalizedToolName

		// Check error parsing parameters
		if parseErr, hasErr := tc.Input["_parse_error"]; hasErr {
			errMsg := fmt.Sprintf("%v", parseErr)
			if rawInput, hasRaw := tc.Input["_raw_input"]; hasRaw {
				errMsg += fmt.Sprintf("\nRaw input: %v", rawInput)
			}
			eventCh <- provider.StreamEvent{Type: provider.EventTextDelta, Text: "\n[ERROR: " + errMsg + "]\n"}
			a.Context.Messages = append(a.Context.Messages, provider.ToolResultMsg(
				tc.ToolUseID, tc.ToolName, "ERROR: "+errMsg, true,
			))
			continue
		}

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
			permResult := a.PermissionChecker.CheckPermission(req)
			if permResult == PermDenied {
				eventCh <- provider.StreamEvent{Type: provider.EventTextDelta, Text: fmt.Sprintf("[FORBIDDEN] %s: %s\n", tc.ToolName, i18n.T("errors.forbidden", req.Level))}
				a.Context.Messages = append(a.Context.Messages, provider.ToolResultMsg(
					tc.ToolUseID, tc.ToolName,
					i18n.T("errors.forbidden", req.Level), true,
				))
				continue
			}
		}

		// Execute tool
		toolStart := time.Now()
		var toolResult tools.ToolResult
		if asyncTool, ok := tool.(tools.AsyncTool); ok {
			if subagentTool, ok2 := asyncTool.(*SubagentTool); ok2 {
				subagentTool.SetCancelCh(ctx.Done())
			}
			asyncCh := asyncTool.ExecuteAsync(params)
			var outputBuf strings.Builder
			for evt := range asyncCh {
				if evt.Type == "progress" && evt.Output != "" {
					eventCh <- provider.StreamEvent{
						Type:        provider.EventToolProgress,
						ToolName:    tc.ToolName,
						ToolMessage: evt.Output,
					}
					outputBuf.WriteString(evt.Output)
					outputBuf.WriteString("\n")
				}
				if evt.Done {
					toolResult = tools.ToolResult{
						Output: outputBuf.String(),
						Error:  evt.Error,
					}
					if evt.Output != "" {
						toolResult.Output = evt.Output
					}
					break
				}
			}
		} else {
			toolResult = tool.Execute(params)
		}
		toolDur := time.Since(toolStart)

		// Send tool completion event
		resultSummary := toolResult.Output
		if idx := strings.Index(resultSummary, "\n"); idx >= 0 {
			resultSummary = resultSummary[:idx]
		}
		if toolResult.Error != "" {
			resultSummary = toolResult.Error
		} else if len(resultSummary) > 80 {
			if tc.ToolName == "bash" {
				resultSummary = resultSummary[:min(len(resultSummary), 500)]
			} else {
				resultSummary = resultSummary[:80] + "..."
			}
		}

		eventCh <- provider.StreamEvent{
			Type:           provider.EventToolCallEnd,
			ToolName:       tc.ToolName,
			ToolInput:      tc.Input,
			ToolResult:     resultSummary,
			ToolFullResult: toolResult.Output,
			ToolOK:         toolResult.Error == "",
			Duration:       toolDur,
		}

		if toolResult.Error != "" {
			// Auto-repair: if missing parameters, add hint with correct format
			errorMsg := toolResult.Error
			if strings.Contains(errorMsg, "param_required") || strings.Contains(errorMsg, "param_path") || strings.Contains(errorMsg, "param_content") || strings.Contains(errorMsg, "param_query") || strings.Contains(errorMsg, "param_pattern") || strings.Contains(errorMsg, "param_url") {
				hint := getToolCallHint(tc.ToolName)
				errorMsg = fmt.Sprintf("%s\n\nCorrect format: %s", errorMsg, hint)
			}
			a.Context.Messages = append(a.Context.Messages, provider.ToolResultMsg(
				tc.ToolUseID, tc.ToolName, "ERROR: "+errorMsg, true,
			))
		} else {
			a.Context.Messages = append(a.Context.Messages, provider.ToolResultMsg(
				tc.ToolUseID, tc.ToolName, toolResult.Output, false,
			))
		}

		// Check for loop
		if a.LoopDetector != nil {
			if isLoop, msg := a.LoopDetector.RecordToolCall(tc.ToolName, params, toolResult.Error == ""); isLoop {
				eventCh <- provider.StreamEvent{Type: provider.EventTextDelta, Text: "\n" + msg + "\n"}
				a.Context.Messages = append(a.Context.Messages, provider.UserMsg(
					i18n.T("loop_detector.strategy_hint"),
				))
				a.LoopDetector.Reset()
				return true // loop detected — continue loop
			}
		}
	}

	// Check context cancellation after tool execution
	select {
	case <-ctx.Done():
		eventCh <- provider.StreamEvent{Type: provider.EventError, Error: ctx.Err()}
		return false
	default:
	}

	// Context compaction (if necessary)
	a.maybeCompact(eventCh, ctx)

	return false
}

// handleStreamFinalResponse handles final response (without tool calls).
// Checks text response loop and sends final events.
func (a *AgentLoop) handleStreamFinalResponse(
	text string,
	iteration int,
	totalStart, iterStart time.Time,
	eventCh chan<- provider.StreamEvent,
) (continueLoop bool, err error) {
	// Check text response loop
	if a.LoopDetector != nil {
		if isLoop, msg := a.LoopDetector.RecordTextResponse(text); isLoop {
			eventCh <- provider.StreamEvent{Type: provider.EventTextDelta, Text: "\n" + msg + "\n"}
			a.Context.Messages = append(a.Context.Messages, provider.UserMsg(
				i18n.T("loop_detector.strategy_hint"),
			))
			a.LoopDetector.Reset()
			return true, nil // loop detected — continue loop
		}
	}

	// Send iteration completion event
	iterDur := time.Since(iterStart)
	eventCh <- provider.StreamEvent{
		Type:      provider.EventIterationEnd,
		Iteration: iteration,
		Duration:  iterDur,
	}

	// Send data by tokens (approximate)
	tokensUsed := a.Context.TokenCount()
	eventCh <- provider.StreamEvent{
		Type:         provider.EventUsage,
		InputTokens:  tokensUsed,
		OutputTokens: EstimateTokens(text),
	}

	// Auto-continue: if model responds with text only (no tool calls),
	// prompt it to continue working with tools (max 3 times)
	// Only when auto-continue is enabled (TUI mode)
	// NOTE: Check auto-continue BEFORE sending EventDone,
	// because TUI will close the session on EventDone.
	if a.autoContinue && a.autoContinueCount < 3 && a.Context != nil && len(a.Context.Messages) > 0 {
		lastMsg := a.Context.Messages[len(a.Context.Messages)-1]
		if lastMsg.Role == "assistant" && len(text) > 0 {
			// Model responded with text only, no tool calls
			// Add a hint to continue with tools
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
			eventCh <- provider.StreamEvent{Type: provider.EventTextDelta, Text: "\n[Auto-continue: prompting model to use tools]\n"}
			return true, nil // continue loop — don't send EventDone
		}
	}

	// Final event — only sent when session is truly done
	eventCh <- provider.StreamEvent{
		Type:     provider.EventDone,
		Duration: time.Since(totalStart),
	}

	// Auto-compress memory if it exceeds 10% of context window
	if memTool, ok := a.Tools["memory"].(*tools.MemoryTool); ok {
		memTokens := memTool.TokenCount()
		maxTokens := a.effectiveMaxTokens()
		if maxTokens > 0 && memTokens > 0 {
			threshold := maxTokens / 10 // 10% of context window
			if memTokens > threshold {
				result := memTool.Compress(threshold)
				if a.verbose {
					logger.Debug("memory_auto_compress", "tokens", memTokens, "threshold", threshold, "result", result.Output)
				}
			}
		}
	}

	return false, nil
}

// getToolCallHint returns a hint with the correct format for a tool call.
func getToolCallHint(toolName string) string {
	hints := map[string]string{
		"read":    `<tool name="read"><path>/path/to/file</path></tool>`,
		"write":   `<tool name="write"><path>/path/to/file</path><content>file content here</content></tool>`,
		"edit":    `<tool name="edit"><path>/path/to/file</path><old>old text</old><new>new text</new></tool>`,
		"bash":    `<tool name="bash"><command>ls -la</command></tool>`,
		"grep":    `<tool name="grep"><pattern>search_pattern</pattern><path>/path/to/dir</path></tool>`,
		"glob":    `<tool name="glob"><pattern>**/*.go</pattern><path>/path/to/dir</path></tool>`,
		"memory":  `<tool name="memory"><action>save</action><key>my_key</key><value>my value</value></tool>`,
		"lsp":     `<tool name="lsp"><operation>document_symbols</operation><file_path>/path/to/file.go</file_path></tool>`,
		"browse":  `<tool name="browse"><action>search</action><query>search query</query></tool>`,
		"web_fetch": `<tool name="web_fetch"><url>https://example.com</url></tool>`,
		"ask_user": `<tool name="ask_user"><question>Your question here</question></tool>`,
		"todo_write": `<tool name="todo_write"><todos>[{"id":"1","subject":"Task","status":"pending"}]</todos></tool>`,
		"todo_read": `<tool name="todo_read"></tool>`,
		"learn":   `<tool name="learn"><input>input text</input><output>expected output</output></tool>`,
		"delegate_task": `<tool name="delegate_task"><task>Task description</task></tool>`,
	}
	if hint, ok := hints[toolName]; ok {
		return hint
	}
	return fmt.Sprintf(`<tool name="%s"><param1>value1</param1></tool>`, toolName)
}
