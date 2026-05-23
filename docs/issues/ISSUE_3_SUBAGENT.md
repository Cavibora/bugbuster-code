## Issue 3: Sub-agent (delegate_task) returns empty result

**Labels:** `bug`, `priority: medium`, `area: agent`

**Title:** delegate_task sub-agent sometimes returns empty result after tool calls

**Body:**

### Description

When `delegate_task` delegates work to a sub-agent, the sub-agent may complete all tool calls (read, bash, edit, etc.) but fail to produce a final text summary. The result is an empty string, making it appear as if the sub-agent did nothing.

### Steps to Reproduce

1. Ask BugBuster to delegate a complex task: "delegate fixing the bug in parser.rs to a sub-agent"
2. Sub-agent makes multiple tool calls (read, bash, edit)
3. Sub-agent reaches `maxIterations` (30) without writing a final summary
4. Result is empty or "subagent completed"

### Root Cause

1. **No summary on iteration limit** — when sub-agent hits `maxIterations`, it takes `lastText` from the last message, which may be a `ToolResultMsg` (empty text)
2. **Weak system prompt** — sub-agent prompt doesn't strongly enforce writing a final summary
3. **Result collection** — `runSubagent` only collects `EventTextDelta`, ignoring tool results

### Fix Status

Partially fixed in recent commits:
- Added summary request injection on iteration limit
- Added `toolResults` collection for fallback summary
- Improved sub-agent system prompt
- Increased `maxIterations` from 10 to 30

Needs more testing with real-world tasks.
