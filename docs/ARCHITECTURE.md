# Architecture

BugBuster Code is a model-agnostic CLI agent for software development, written in Go (~21K lines of application code, ~19K lines of tests across 66 test files).

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         User Interface                          │
│  ┌──────────┐  ┌──────────────┐  ┌──────────────────────────┐  │
│  │   CLI     │  │  Split Term  │  │         TUI              │  │
│  │ readline  │  │  (default)   │  │  Bubble Tea v2 +         │  │
│  │           │  │  input+output│  │  lipgloss + glamour      │  │
│  └─────┬─────┘  └──────┬───────┘  └───────────┬──────────────┘  │
│        │               │                       │                 │
│        └───────────────┼───────────────────────┘                 │
│                        │                                         │
│                   AgentLoop                                      │
│              (core agent logic)                                  │
│                        │                                         │
│         ┌──────────────┼──────────────┐                         │
│         │              │              │                          │
│    StreamEvents    Tool Calls    Compaction                     │
│         │              │              │                          │
│         ▼              ▼              ▼                          │
│    Provider        Tool Registry   Compactor                    │
│    Interface       (16 tools)      (LLM/simple)                │
│         │              │                                         │
│    ┌────┼────┐    ┌────┼────┐                                   │
│    │    │    │    │    │    │                                    │
│  OpenAI Anthr. ... bash read write edit grep glob ...           │
│  Ollama Cavibora   MCP plugins  Sub-agents                     │
└─────────────────────────────────────────────────────────────────┘
```

## Package Structure

```
bugbuster-code/
├── cmd/bugbuster/          # Application entry point and UI layer
│   ├── main.go             # CLI setup, signal handling, crash handler
│   ├── interactive.go      # Interactive mode: readline input, command handling
│   ├── split_terminal.go   # Split terminal mode (default CLI)
│   ├── streaming.go        # Streaming response display, spinners
│   ├── tui.go              # TUI mode (Bubble Tea v2)
│   ├── tui_stream.go       # TUI streaming event handler
│   ├── tui_styles.go       # TUI key bindings and styles
│   ├── ui.go               # Shared UI: spinner, formatting, markdown rendering
│   ├── autopilot.go        # Autopilot mode (automatic tool execution)
│   ├── commands.go         # Slash command implementations
│   ├── session_history.go  # Session save/restore
│   ├── crash_handler.go    # Crash logging and recovery
│   ├── exec.go             # External command execution
│   ├── escape.go           # ANSI escape sequence utilities
│   ├── mcp_serve.go        # MCP server mode
│   └── agent_setup.go      # Agent initialization from config
│
├── pkg/
│   ├── agent/              # Core agent logic
│   │   ├── agent.go        # AgentLoop: streaming, tool calls, iteration loop
│   │   ├── agent_stream.go # Stream retry, event processing
│   │   ├── subagent.go     # Sub-agent delegation (delegate_task tool)
│   │   ├── context.go      # ConversationContext: messages, tokens, compaction
│   │   ├── loop_detector.go# Loop detection (text similarity, tool repetition)
│   │   ├── compactor.go    # Context compaction (LLM summarization)
│   │   └── system_prompt.go# System prompt construction
│   │
│   ├── provider/           # LLM provider abstraction
│   │   ├── provider.go     # Provider interface, Message types, StreamEvent
│   │   ├── openai.go       # OpenAI provider (GPT-4o, o1, o3)
│   │   ├── anthropic.go    # Anthropic provider (Claude, extended thinking)
│   │   ├── ollama.go       # Ollama provider (local models)
│   │   ├── cavibora.go     # Cavibora provider (wave-based AI)
│   │   ├── openai_compat.go# OpenAI-compatible APIs (DeepSeek, etc.)
│   │   ├── factory.go      # Provider factory from config
│   │   ├── retry.go        # Retry logic with exponential backoff
│   │   ├── sse.go          # Server-Sent Events parser
│   │   └── message.go      # Message types (User, Assistant, Tool, System)
│   │
│   ├── tools/              # Built-in tools
│   │   ├── read.go         # File/directory reading
│   │   ├── write.go        # File writing with diff
│   │   ├── edit.go         # Find & replace in files
│   │   ├── bash.go         # Shell command execution (sync + async)
│   │   ├── grep.go         # Regex search in files
│   │   ├── glob.go         # File pattern matching
│   │   ├── ask.go          # Ask external LLM
│   │   ├── ask_user.go     # Ask user for input
│   │   ├── learn.go        # Train model on input/output
│   │   ├── web_fetch.go    # HTTP URL fetching
│   │   ├── browse.go       # Headless browser + search (configurable engine)
│   │   ├── browse_chrome.go# chromedp headless Chrome implementation
│   │   ├── browse_nochrome.go# HTTP fallback (no Chrome dependency)
│   │   ├── memory.go       # Session-scoped persistent memory
│   │   ├── todo.go         # Task checklist management
│   │   ├── lsp.go          # Language Server Protocol client
│   │   ├── lsp_client.go   # LSP JSON-RPC client implementation
│   │   ├── diff.go         # Diff generation and statistics
│   │   ├── path_security.go# Path traversal and security checks
│   │   └── hooks.go        # Tool lifecycle hooks
│   │
│   ├── config/             # Configuration management
│   │   └── config.go       # YAML config loading, defaults, validation
│   │
│   ├── i18n/               # Internationalization
│   │   ├── i18n.go         # Translation engine
│   │   ├── embed.go        # Embedded locale files
│   │   └── locales/        # 8 language files (en, ru, es, fr, de, ja, zh, pt)
│   │
│   ├── theme/              # Terminal theming
│   │   └── theme.go        # Color themes, ANSI codes, dark/light modes
│   │
│   ├── mcp/                # Model Context Protocol
│   │   ├── client.go       # MCP client (connect to external servers)
│   │   └── server.go       # MCP server (expose tools to external clients)
│   │
│   ├── plugin/             # Plugin system
│   │   └── plugin.go       # Go plugin loading (.so), builtin plugins
│   │
│   └── logger/             # Logging
│       └── logger.go       # Structured logging with levels
```

## Core Data Flow

### 1. User Input → Agent Response

```
User types message
       │
       ▼
readline / TUI textarea
       │
       ▼
AgentLoop.StreamWithCancel(ctx, prompt)
       │
       ▼
┌──────────────────────────┐
│ Iteration Loop           │
│                          │
│  1. Build messages from  │
│     ConversationContext  │
│  2. Call Provider.Stream │
│  3. Process StreamEvents │
│     - TextDelta → output │
│     - ToolCall → execute │
│     - Thinking → display │
│  4. If tool was called:  │
│     - Execute tool       │
│     - Add result to ctx  │
│     - Go to step 1       │
│  5. If text only: done   │
└──────────────────────────┘
       │
       ▼
StreamEvent channel → UI renders output
```

### 2. Tool Execution

```
AgentLoop receives ToolCall event
       │
       ▼
Look up tool in ToolRegistry
       │
       ▼
Check path security (path_security.go)
       │
       ▼
Execute tool (sync or async)
       │
       ├── read → os.ReadFile / os.ReadDir
       ├── write → os.WriteFile + diff
       ├── edit → strings.Replace + diff
       ├── bash → exec.CommandContext
       ├── grep → regexp + filepath.Walk
       ├── glob → filepath.Glob
       ├── delegate_task → new AgentLoop (isolated)
       └── ...
       │
       ▼
Return ToolResult to AgentLoop
       │
       ▼
Add to ConversationContext
       │
       ▼
Continue iteration loop
```

### 3. Context Compaction

When token count approaches the limit:

```
ConversationContext.TokenCount() > 80% of maxTokens
       │
       ▼
Compactor.Compact(messages)
       │
       ├── LLM Compaction (preferred):
       │   - Send messages to LLM with "summarize" prompt
       │   - Replace old messages with summary
       │   - Keep recent N messages
       │
       └── Simple Fallback:
           - Keep system prompt
           - Keep recent N messages
           - Drop oldest messages
       │
       ▼
Continue with compacted context
```

## Key Design Decisions

### Provider Interface

All LLM providers implement the same `Provider` interface:

```go
type Provider interface {
    Name() string
    Complete(ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error)
    Stream(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error)
}
```

This allows seamless switching between providers without changing agent logic.

### Stream Events

All communication uses a unified event stream:

```go
type StreamEvent struct {
    Type           string  // "text_delta", "tool_call_start", "tool_call_end",
                           // "tool_progress", "thinking", "done", "error",
                           // "iteration_end", "compaction"
    Text           string
    ToolName       string
    ToolInput      map[string]any
    ToolResult     string
    ToolFullResult string
    ToolOK         bool
    Duration       time.Duration
    Error          error
}
```

### Tool Interface

Tools implement either `Tool` (synchronous) or `AsyncTool` (asynchronous):

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]any
    Execute(params map[string]string) ToolResult
}

type AsyncTool interface {
    Tool
    ExecuteAsync(params map[string]string) <-chan AsyncEvent
}
```

### Sub-Agent Isolation

`delegate_task` creates an isolated `AgentLoop` with:
- Separate `ConversationContext` (16K token limit)
- Filtered tool set (no `delegate_task` to prevent recursion)
- Timeout (10 minutes default)
- Max iterations (30 default)
- Summary request on iteration limit

### Session Persistence

Sessions are saved as JSON files containing:
- All messages (user, assistant, tool calls, tool results)
- Token counts
- Provider/model info
- Timestamps

Sessions are saved:
- On normal exit (`/exit`, Ctrl+D)
- On signals (SIGINT, SIGTERM)
- On panic (crash handler)
- Incrementally during long operations

### Session-Scoped Memory

The `memory` tool provides persistent, session-scoped storage for important facts:

**Storage Priority:**
| Priority | Path | When |
|----------|------|------|
| 1 | `<project>/.bugbuster/memory/<session-id>.md` | `.bugbuster/` exists in project directory |
| 2 | `~/.bugbuster/memory/<session-id>.md` | Fallback (no `.bugbuster/` in project) |

- **Session-scoped**: each session has its own memory file, preventing cross-session contamination
- **Project-local**: memory is stored in the project's `.bugbuster/` directory when available
- **Auto-injected**: all facts are loaded into the system prompt at session start
- **Compaction-safe**: facts are re-injected after context compaction (never lost)
- **Mandatory**: the model is required to save and check memory (enforced via system prompt rule8)
- **Human-readable**: Markdown format, editable by user
- **Categories**: facts are organized by category (project, database, warnings, metrics, etc.)

**Save triggers** (model must save immediately):
- User says "remember", "don't forget", "important", "note"
- Project paths, DB hosts, API endpoints, credentials
- Dataset sizes, test results, build times
- User corrections/warnings ("don't do X", "always do Y")
- Architecture decisions, design patterns

Data flow:
```
Agent discovers important fact (e.g., user warning)
       │
       ▼
memory(action=save, key="avoid_mysql_memory_load",
       value="User warned: never load full MySQL datasets into memory",
       category="warnings")
       │
       ▼
Write to <project>/.bugbuster/memory/<session-id>.md
       │
       ▼
On next session start → LoadAllFacts() → inject into system prompt
After context compaction → AfterCompact callback → re-inject facts
```

### Headless Browser (Browse Tool)

The `browse` tool provides web search and page rendering without external dependencies:

```
┌─────────────────────────────────────────────┐
│ Browse Tool                                  │
│                                              │
│  Actions:                                    │
│  ├── search → web search (configurable)      │
│  ├── fetch  → render page (headless)         │
│  └── extract → clean text extraction         │
│                                              │
│  Search Engines:                             │
│  ├── DuckDuckGo (default, HTTP HTML)         │
│  ├── Google (HTTP HTML)                      │
│  ├── Yandex (headless Chrome, JS required)   │
│  └── Bing (HTTP HTML)                        │
│                                              │
│  Rendering Engines:                          │
│  ├── chromedp (default, headless Chrome)     │
│  ├── rod (alternative)                       │
│  ├── playwright (alternative)                │
│  └── http (fallback, no JS)                  │
└─────────────────────────────────────────────┘
```

Configuration in `bugbuster.yaml`:
```yaml
tools:
  browse:
    engine: chromedp          # chromedp, rod, playwright, http
    search_engine: duckduckgo # duckduckgo, google, yandex, bing
    timeout: 30
    max_results: 10
    headless: true
```

Per-query override:
```
browse(action=search, query="competitors", engine=yandex)
browse(action=fetch, url="https://example.com")
```

## UI Modes

### CLI (Split Terminal) — Default

- Input: readline with multi-line support (Shift+Enter)
- Output: streaming with spinners, syntax highlighting
- Tools: formatted with `⏺` / `⎿` markers

### TUI (Terminal UI)

- Input: Bubble Tea textarea
- Output: viewport with glamour markdown rendering
- Features: scrollable output, progress bars, context bar

### Query Mode

- Non-interactive single-shot
- Output to stdout
- Exit code 0/1

### MCP Server Mode

- Expose all tools via MCP protocol
- Supports stdio, SSE, streamable HTTP transports
