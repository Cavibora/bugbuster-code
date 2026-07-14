# BugBuster Code

[![Status: Alpha](https://img.shields.io/badge/Status-Alpha-orange.svg)](https://github.com/Cavibora/bugbuster-code)
[![Go Report Card](https://goreportcard.com/badge/github.com/Cavibora/bugbuster-code)](https://goreportcard.com/report/github.com/Cavibora/bugbuster-code)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> ⚠️ **Alpha Software** — BugBuster Code is in early development. CLI mode is stable. TUI mode is experimental. Expect bugs and breaking changes.

**BugBuster Code** is a model-agnostic CLI agent for software development. It connects to any LLM (OpenAI, Anthropic, Ollama, Cavibora, or OpenAI-compatible) and gives it 21 tools to read, write, edit, search, and execute code — plus **multimodal capabilities** like screenshots, voice, and vision.

## ✨ Highlights

- 🖥️ **Screenshots & Vision** — capture desktop, window, or region; send images to vision models
- 🎤 **Speech-to-Text** — record from microphone or transcribe audio files via Whisper
- 🔊 **Text-to-Speech** — generate voice with OpenAI TTS or system `say`/`espeak`
- 🪞 **Speed Mirror** — model sees its own performance and self-optimizes context
- 💥 **Aggressive Compaction** — `/compact!` for emergency context reduction; auto-triggers on 3x slowdown
- 🧠 **Self-Awareness** — `self_info` tool lets the model know its provider, context usage, and environment
- 🔒 **Security** — path traversal protection, secret file blocking, sandbox mode, command blocking
- 🔐 **Granular Permissions** — per-tool permission overrides (auto-approve/ask/deny per tool)
- 🔄 **Fallback Providers** — automatic switch to backup provider when primary fails
- 🌍 **8 Languages** — English, Russian, Spanish, French, German, Japanese, Chinese, Portuguese
- 🤖 **5 LLM Providers** — OpenAI, Anthropic, Ollama, Cavibora, OpenAI-compatible
- 🔄 **Smart Context** — LLM summarization + simple fallback + archiving + auto-compact on slowdown
- 🤝 **Sub-agents** — isolated context, parallelism semaphore, timeouts
- 📡 **MCP** — both client and server (stdio, SSE, streamable HTTP)
- 🎯 **Skills** — reusable step-by-step procedures (debug, refactor, review, deploy, analyze + custom)

## 🛠️ 21 Built-in Tools

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
| `browse` | Headless browser + web search |
| `memory` | Session-scoped persistent memory |
| `delegate_task` | Delegate to sub-agent |
| `todo` | Manage task checklist |
| `lsp` | Language Server Protocol analysis |
| `search_context` | Search archival context |
| `compact_force` | Aggressively reduce context (auto-triggers on 3x slowdown) |
| `self_info` | Query model identity, context usage, environment |
| `screenshot` | 🖥️ Capture desktop, window, or region |
| `send_file` | 📎 Send image/audio/document to model |
| `tts` | 🔊 Text-to-speech (OpenAI TTS or system) |
| `stt` | 🎤 Speech-to-text (Whisper or local) |

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

# Multimodal settings
tools:
  screenshot:
    enabled: true
    format: png          # png or jpeg
  tts:
    enabled: true
    model: tts-1          # tts-1 or tts-1-hd
    voice: alloy          # alloy, echo, fable, onyx, nova, shimmer
  stt:
    enabled: true
    model: whisper-1      # whisper-1
    language: ""           # auto-detect, or "en", "ru", etc.

agent:
  permission_mode: auto-approve
  language: en
  request_timeout: 2400
  thinking_timeout: 600
  idle_timeout: 120

  # Per-tool permission overrides (override global permission_mode)
  # Each tool can have: "auto-approve", "ask", or "deny"
  permissions:
    bash: ask          # always ask before running commands
    web_fetch: deny     # block all HTTP requests
    browse: deny        # block web browsing
    kill: deny          # block process killing
    memory: auto-approve  # memory always allowed

  # Fallback provider — switch when primary fails
  fallback:
    provider: ollama    # use ollama when openai/anthropic fails
    max_retries: 2       # retry primary N times before fallback
    retry_delay_ms: 1000 # delay between retries
    auto_switch_back: true  # switch back to primary when it recovers
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
| `--session-name` | `-n` | Set session name |
| `--lang` | `-l` | Interface language: `en`, `ru`, `es`, `fr`, `de`, `ja`, `zh`, `pt` |
| `--tui` | `-t` | TUI mode: `auto` (default) or `inline` |

### Interactive Commands

| Command | Description |
|---------|-------------|
| `/help` | Show help |
| `/exit`, `/quit` | Exit (saves session) |
| `/reset` | Reset conversation context |
| `/compact` | Soft compact — summarize context |
| `/compact!` | 💥 Aggressive compact — strip to 15% of context |
| `/context` | Show context info |
| `/tools` | Show available tools |
| `/model <name>` | Switch model |
| `/provider <name>` | Switch provider |
| `/sessions` | Show saved sessions |
| `/rename <name>` | Rename current session |
| `/undo` | Undo last file change |
| `/undoall` | Undo all file changes |
| `/diff` | Show file changes list |
| `/lang <code>` | Switch interface language |
| `/auto` | Toggle autopilot mode |
| `/skills` | List available skills |
| `/skill <name>` | Activate a skill |
| `/skill off` | Deactivate current skill |

## 🖥️ Multimodal Examples

### Screenshot + Vision
```
> Take a screenshot and tell me what you see
[screenshot mode=fullscreen]
→ Captures entire screen, sends to vision model

> Take a screenshot of the browser window
[screenshot mode=window]
→ Click to select window, sends to model

> What's in this image?
[send_file path=/tmp/error.png]
→ Sends image to model for analysis
```

### Voice I/O
```
> Read this aloud
[tts text="Hello! BugBuster Code is running." voice=alloy]
→ Generates speech via OpenAI TTS, plays with afplay

> Transcribe my recording
[stt file=/tmp/meeting.mp3]
→ Transcribes audio via Whisper

> Record 10 seconds from microphone and transcribe
[stt duration=10s language=en]
→ Records from mic, transcribes
```

## 🪞 Speed Mirror & Self-Awareness

BugBuster Code makes the LLM **self-aware** of its performance:

- **Speed Mirror** — every 5 iterations, the model receives a system message with its iteration speed, context usage, and slowdown ratio
- **Auto-compact on 3x slowdown** — when context >50% and speed drops 3x, aggressive compaction triggers automatically
- **`self_info` tool** — model can query its provider, model name, context window, and message count
- **`/compact!` command** — user can force aggressive compaction anytime

This means the model **knows when it's slowing down** and takes action — no external monitoring needed.

## Agent Instructions

BugBuster reads project-specific instructions from agent instruction files:

| File | Format | Description |
|------|--------|-------------|
| `AGENT.md` | Markdown | BugBuster native format (highest priority) |
| `CLAUDE.md` | Markdown | Claude Code format |
| `.cursorrules` | Text | Cursor format |
| `.github/copilot-instructions.md` | Markdown | GitHub Copilot format |
| `.windsurfrules` | Text | Windsurf format |
| `.aider.conf.yml` | YAML | Aider format |
| `.clinerules` | Text | Cline format |

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
    Model() string
    Complete(messages []Message, tools []ToolDef) (*CompletionResult, error)
    CompleteWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error)
    Stream(messages []Message, tools []ToolDef) (<-chan StreamEvent, error)
    StreamWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error)
}
```

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

## Skills

Skills are reusable step-by-step procedures that guide the agent through common tasks:

| Skill | Description |
|-------|-------------|
| `debug` | Systematic debugging: read error → find file → propose fix → run tests |
| `refactor` | Safe refactoring: find usages → plan → make changes → verify tests |
| `review` | Code review: diff → style → security → tests → report |
| `deploy` | Deployment: tests → build → deploy → health check → rollback |
| `analyze` | Codebase analysis: structure → patterns → metrics → recommendations |

Create custom skills in `.bugbuster/skills/my-skill.md`.

## Security

- **Path traversal** — blocks paths with `..`
- **Secret files** — blocks access to `.env`, `credentials.json`, SSH keys, etc.
- **System paths** — blocks writes to `/etc`, `/usr`, `/System`
- **Sandbox** — restrict file writes to `sandbox_dir`
- **Network commands** — blocks `curl`, `wget` when `allow_network: false`
- **Blocked commands** — configurable list of forbidden bash commands
- **Auto-kill stale processes** — processes running >7 days are auto-killed

## Development

```bash
# Run tests
go test ./...

# Run tests with race detector
go test -race ./...

# Build
go build -o bugbuster ./cmd/bugbuster/

# Build with version info
go build -ldflags "-X main.Version=1.1.0 -X main.GitCommit=$(git rev-parse --short HEAD)" -o bugbuster ./cmd/bugbuster/
```

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture](docs/ARCHITECTURE.md) | Package structure, data flow, design decisions |
| [Tools Reference](docs/TOOLS.md) | All 21 built-in tools with parameters and examples |
| [Configuration](docs/CONFIGURATION.md) | Full YAML config reference, CLI flags, provider setup |
| [Security](docs/SECURITY.md) | Security model, threat analysis, best practices |
| [Contributing](docs/CONTRIBUTING.md) | Development setup, coding conventions, PR process |

## License

MIT License — see [LICENSE](LICENSE) for details.