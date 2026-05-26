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
	requestTimeout := a.effectiveRequestTimeout()
	idleTimer := time.NewTimer(idleTimeout)
	thinkingTimer := time.NewTimer(thinkingTimeout)
	defer idleTimer.Stop()
	defer thinkingTimer.Stop()
	lastTokenTime := time.Now()
	iterStartTime := time.Now()

streamLoop:
	for {
		// Check request timeout — if entire iteration takes too long
		if requestTimeout > 0 && time.Since(iterStartTime) > requestTimeout {
			mins := int(time.Since(iterStartTime).Minutes())
			if mins < 1 {
				mins = 1
			}
			eventCh <- provider.StreamEvent{
				Type:     provider.EventRequestTimeout,
				Duration: time.Since(iterStartTime),
			}
			result.err = fmt.Errorf("%s", i18n.T("cli.request_timeout_warn", fmt.Sprintf("%d", mins)))
			return result
		}

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

			case "stop_reason":
				if event.StopReason == "max_tokens" {
					eventCh <- provider.StreamEvent{
						Type:  provider.EventError,
						Error: fmt.Errorf("%s (max_tokens=%d)", i18n.T("errors.max_tokens_reached"), a.effectiveMaxTokens()),
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
		tool, ok := a.Tools[tc.ToolName]
		if !ok {
			errMsg := i18n.T("errors.unknown_tool", tc.ToolName)
			eventCh <- provider.StreamEvent{Type: provider.EventTextDelta, Text: "\n[ERROR: " + errMsg + "]\n"}
			a.Context.Messages = append(a.Context.Messages, provider.ToolResultMsg(
				tc.ToolUseID, tc.ToolName, "ERROR: "+errMsg, true,
			))
			continue
		}

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
			a.Context.Messages = append(a.Context.Messages, provider.ToolResultMsg(
				tc.ToolUseID, tc.ToolName, "ERROR: "+toolResult.Error, true,
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

	// Final event
	eventCh <- provider.StreamEvent{
		Type:     provider.EventDone,
		Duration: time.Since(totalStart),
	}
	return false, nil
}
