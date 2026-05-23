# BugBuster Code

[![Status: Alpha](https://img.shields.io/badge/Status-Alpha-orange.svg)](https://github.com/Cavibora/bugbuster-code)
[![Go Report Card](https://goreportcard.com/badge/github.com/Cavibora/bugbuster-code)](https://goreportcard.com/report/github.com/Cavibora/bugbuster-code)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> ⚠️ **Alpha Software** — BugBuster Code is in early development. CLI mode is stable. TUI mode is experimental. Expect bugs and breaking changes.

## Features

- 🔧 **11+ built-in tools**: read, write, edit, bash, grep, glob, ask, learn, web_fetch, ask_user, delegate_task, todo, lsp, search_context
- 🤖 **5 LLM providers**: OpenAI, Anthropic, Ollama, [Cavibora](https://github.com/Cavibora), OpenAI-compatible
- 🌍 **8 languages**: English, Russian, Spanish, French, German, Japanese, Chinese, Portuguese
- 🧠 **Thinking/reasoning blocks**: Claude extended thinking, OpenAI o1/o3
- 🔄 **Context compaction**: LLM summarization + simple fallback + archiving
- 🔁 **Loop detection**: Text similarity thresholds, sliding window, anti-thrashing
- 🤝 **Sub-agents**: Isolated context, parallelism semaphore, timeouts
- 📡 **MCP**: Both client and server (stdio, SSE, streamable HTTP)
- 🎨 **TUI**: Rich terminal UI with markdown rendering, spinners, progress bars
- 💾 **Sessions**: Save/restore conversation context
- 🔒 **Security**: Path traversal protection, secret file blocking, sandbox mode, command blocking

## Quick Start

```bash
# Build
go build -o bugbuster ./cmd/bugbuster/

# Run (interactive CLI mode)
./bugbuster

# Run with TUI
./bugbuster --tui

# One-shot query
./bugbuster "Fix the bug in main.go"

# Scan project for bugs
./bugbuster scan ./...
```

## Configuration

Create `bugbuster.yaml` (or `.bugbuster.yaml`) in your project root or `~/.bugbuster/config.yaml`:

```yaml
default_provider: openai

providers:
  openai:
    type: openai
    api_key: ${OPENAI_API_KEY}
    model: gpt-4o
    max_tokens: 8192
    context_window: 128000

  anthropic:
    type: anthropic
    api_key: ${ANTHROPIC_API_KEY}
    model: claude-sonnet-4-20250514
    max_tokens: 8192
    context_window: 200000
    budget_tokens: 4096

  ollama:
    type: ollama
    base_url: http://localhost:11434
    model: llama3
    max_tokens: 4096
    context_window: 32000

  cavibora:
    type: cavibora
    api_key: ${CAVIBORA_API_KEY}
    model: cavibora-v1
    max_tokens: 8192
    context_window: 128000

agent:
  permission_mode: auto-approve
  language: en
  request_timeout: 1200
  thinking_timeout: 600
  idle_timeout: 120
```

### CLI Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | `-c` | Path to config file |
| `--verbose` | `-v` | Verbose output |
| `--model` | `-m` | Model to use |
| `--dir` | `-d` | Working directory |
| `--permission-mode` | `-p` | Permission mode: `auto-approve`, `ask`, `deny` |
| `--session` | `-s` | Session ID to restore |
| `--lang` | `-l` | Interface language: `en`, `ru`, `es`, `fr`, `de`, `ja`, `zh`, `pt` |
| `--tui` | `-t` | TUI mode: `auto` (default) or `inline` |

### Commands

| Command | Description |
|---------|-------------|
| `/help` | Show help |
| `/exit`, `/quit` | Exit (saves session) |
| `/reset` | Reset conversation context |
| `/context` | Show context info |
| `/tools` | Show available tools |
| `/model <name>` | Switch model |
| `/provider <name>` | Switch provider |
| `/sessions` | Show saved sessions |
| `/undo` | Undo last file change |
| `/undoall` | Undo all file changes |
| `/diff` | Show file changes list |
| `/lang <code>` | Switch interface language |
| `/auto` | Toggle autopilot mode |

## Agent Instructions

BugBuster reads project-specific instructions from agent instruction files. These files define coding standards, project rules, and preferences that the agent follows.

### Supported Files (in priority order)

| File | Format | Description |
|------|--------|-------------|
| `AGENT.md` | Markdown | BugBuster native format (highest priority) |
| `CLAUDE.md` | Markdown | Claude Code format |
| `.cursorrules` | Text | Cursor format |
| `.github/copilot-instructions.md` | Markdown | GitHub Copilot format |
| `.windsurfrules` | Text | Windsurf format |
| `.aider.conf.yml` | YAML | Aider format |
| `.clinerules` | Text | Cline format |

All found files are loaded and appended to the system prompt. Create `AGENT.md` in your project root:

```markdown
# Project Instructions

## Code Style
- Use TypeScript strict mode
- Prefer functional components over class components
- All functions must have JSDoc comments

## Testing
- Write tests before fixing bugs
- Use vitest for unit tests
- Minimum 80% code coverage

## Git
- Conventional commits: feat, fix, refactor, test, docs
- Always run lint before commit
```

## Architecture

```
User → readline/TUI → AgentLoop.Stream() → Provider.Stream()
                                              ↓
                                        StreamEvent channel
                                              ↓
                                      runQueryWithLoop()
                                              ↓
                                    MarkdownRenderer → terminal
```

### Provider Interface

```go
type Provider interface {
    Name() string
    Complete(messages []Message, tools []ToolDef) (*CompletionResult, error)
    CompleteWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error)
    Stream(messages []Message, tools []ToolDef) (<-chan StreamEvent, error)
    StreamWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error)
}
```

## Tools

| Tool | Description |
|------|-------------|
| `read` | Read file or list directory |
| `write` | Write file (creates parent dirs) |
| `edit` | Find & replace text in file |
| `bash` | Execute shell command |
| `grep` | Search by regex in files |
| `glob` | Find files by pattern |
| `ask` | Ask external LLM |
| `ask_user` | Ask user for input |
| `learn` | Train model on input/output pair |
| `web_fetch` | Fetch URL content |
| `delegate_task` | Delegate to sub-agent |
| `todo` | Manage task checklist |
| `lsp` | Language Server Protocol analysis |
| `search_context` | Search archival context |

## MCP (Model Context Protocol)

BugBuster supports MCP servers as both client and server:

```yaml
mcp:
  servers:
    filesystem:
      type: stdio
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
      enabled: true
```

Supported transports: `stdio`, `sse`, `streamable-http`

## Security

- **Path traversal** — blocks paths with `..`
- **Secret files** — blocks access to `.env`, `credentials.json`, SSH keys, etc.
- **System paths** — blocks writes to `/etc`, `/usr`, `/System`
- **Sandbox** — restrict file writes to `sandbox_dir`
- **Network commands** — blocks `curl`, `wget` when `allow_network: false`
- **Blocked commands** — configurable list of forbidden bash commands

## Development

```bash
# Run tests
go test ./...

# Run tests with race detector
go test -race ./...

# Run specific package tests
go test ./pkg/agent/...

# Build
go build -o bugbuster ./cmd/bugbuster/
```

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture](docs/ARCHITECTURE.md) | Package structure, data flow, design decisions |
| [Tools Reference](docs/TOOLS.md) | All 14 built-in tools with parameters and examples |
| [Configuration](docs/CONFIGURATION.md) | Full YAML config reference, CLI flags, provider setup |
| [Security](docs/SECURITY.md) | Security model, threat analysis, best practices |
| [Contributing](docs/CONTRIBUTING.md) | Development setup, coding conventions, PR process |

## License

MIT License — see [LICENSE](LICENSE) for details.