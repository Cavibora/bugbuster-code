# Configuration Reference

BugBuster Code is configured via YAML files. Configuration is loaded from (in priority order):

1. `bugbuster.yaml` or `.bugbuster.yaml` in the current directory
2. `~/.bugbuster/config.yaml` (global config)

## Minimal Configuration

```yaml
default_provider: openai

providers:
  openai:
    type: openai
    api_key: ${OPENAI_API_KEY}
    model: gpt-4o
```

## Full Configuration

```yaml
# ─── Provider ────────────────────────────────────────────────────────────────

default_provider: openai

providers:
  openai:
    type: openai
    api_key: ${OPENAI_API_KEY}
    model: gpt-4o
    max_tokens: 8192
    context_window: 128000
    base_url: https://api.openai.com/v1  # optional, for compatible APIs

  anthropic:
    type: anthropic
    api_key: ${ANTHROPIC_API_KEY}
    model: claude-sonnet-4-20250514
    max_tokens: 8192
    context_window: 200000

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

  deepseek:
    type: openai  # uses OpenAI-compatible API
    api_key: ${DEEPSEEK_API_KEY}
    base_url: https://api.deepseek.com/v1
    model: deepseek-chat
    max_tokens: 8192
    context_window: 128000

# ─── Agent ────────────────────────────────────────────────────────────────────

agent:
  max_tokens: 8000              # max tokens in context window
  keep_recent: 20               # messages to keep during compaction
  verbose: false                # verbose logging
  permission_mode: auto-approve # auto-approve | ask | deny
  language: en                  # en | ru | es | fr | de | ja | zh | pt
  request_timeout: 1200         # max time for single LLM request (seconds)
  thinking_timeout: 600         # max time without tokens (seconds)
  idle_timeout: 300             # streaming timeout without events (seconds)

  loop_detection:
    repeat_threshold: 6           # identical consecutive calls = loop
    tool_repeat_threshold: 8      # same tool + same params = loop
    window_size: 30               # sliding window size
    text_similarity_threshold: 0.65  # 0.0-1.0, text similarity = loop
    text_similarity_window: 4     # how many text responses to check

# ─── Tools ────────────────────────────────────────────────────────────────────

tools:
  allowed_dirs: []              # restrict file access to these dirs
  max_file_size: 10485760       # max file size for read/write (10MB)
  bash_timeout: 30              # default bash timeout (seconds)
  max_grep_results: 100         # max grep results
  max_glob_results: 1000        # max glob results

  browse:
    engine: chromedp            # chromedp | rod | playwright | http
    search_engine: duckduckgo   # duckduckgo | google | yandex | bing
    timeout: 30                 # page load timeout (seconds)
    max_results: 10             # max search results (1-20)
    user_agent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"
    headless: true              # run browser in headless mode
    chrome_path: ""             # custom Chrome path (auto-detect if empty)

# ─── Security ─────────────────────────────────────────────────────────────────

security:
  allow_network: false          # allow network commands (curl, wget)
  blocked_commands:             # blocked bash commands
    - rm -rf /
    - mkfs
    - dd if=
  sandbox_dir: ""               # restrict writes to this directory

# ─── Theme ────────────────────────────────────────────────────────────────────

theme:
  mode: dark                    # dark | light
  word_wrap: 80                 # markdown word wrap (0 = no wrap)
  colors:
    primary: "#7C3AED"          # spinner, tool call headers
    success: "#10B981"          # ✓, create, diff additions
    error: "#EF4444"            # ✗, errors, diff deletions
    warning: "#F59E0B"          # warnings
    info: "#3B82F6"             # input tokens
    dim: "#6B7280"              # dimmed text
    thinking: "#8B5CF6"         # thinking block
    tool_params: "#9CA3AF"      # tool call parameters
    tool_summary: "#6B7280"     # tool call result summary
    status_time: "#9CA3AF"      # ⏱ time in status
    status_separator: "#4B5563" # │ separator in status
    context_bar_good: "#10B981" # context < 50%
    context_bar_warn: "#F59E0B" # context 50-80%
    context_bar_bad: "#EF4444"  # context > 80%
    user_message: "#3B82F6"     # ❯ user input (TUI)
    assistant: "#7C3AED"        # assistant spinner/status (TUI)
    separator: "#374151"        # ─── separator

# ─── Key Bindings (TUI) ──────────────────────────────────────────────────────

keys:
  submit: "enter"               # send message
  newline: "shift+enter"        # new line in input
  newline_alt: "ctrl+j"         # alternative new line
  cancel: "esc"                 # cancel current operation

# ─── MCP Client ──────────────────────────────────────────────────────────────

mcp:
  servers:
    filesystem:
      type: stdio
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
      enabled: true

    github:
      type: stdio
      command: npx
      args: ["-y", "@modelcontextprotocol/server-github"]
      env:
        GITHUB_TOKEN: ${GITHUB_TOKEN}
      enabled: true

    remote-api:
      type: sse
      url: http://localhost:3001/sse
      headers:
        Authorization: "Bearer ${API_TOKEN}"
      enabled: true

# ─── MCP Server (BugBuster as server) ────────────────────────────────────────

mcp_serve:
  transport: stdio              # stdio | sse | streamable-http
  host: "localhost"             # host for SSE/HTTP
  port: 3001                    # port for SSE/HTTP
  prefix: "bugbuster_"          # tools prefix
  enabled: false                # enable on startup

# ─── LSP ─────────────────────────────────────────────────────────────────────

lsp:
  timeout: 10                   # request timeout (seconds)
  servers:
    go:
      command: gopls
      args: ["serve"]
    rust:
      command: rust-analyzer
    python:
      command: pyright-langserver
      args: ["--stdio"]
    typescript:
      command: typescript-language-server
      args: ["--stdio"]

# ─── Context Archive ─────────────────────────────────────────────────────────

context_archive:
  enabled: true                 # enable archiving
  max_blocks: 50                # max blocks in archive
  auto_optimize: true           # background optimization during compaction

# ─── UI ──────────────────────────────────────────────────────────────────────

ui: auto                        # auto | tui | cli
```

## Environment Variables

Environment variables can be used in config values with `${VAR_NAME}` syntax:

```yaml
providers:
  openai:
    api_key: ${OPENAI_API_KEY}
```

BugBuster resolves these at load time. If a variable is not set, the value remains as-is.

## CLI Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | `-c` | Path to config file |
| `--verbose` | `-v` | Verbose output |
| `--model` | `-m` | Override model for this session |
| `--dir` | `-d` | Working directory |
| `--permission-mode` | `-p` | `auto-approve`, `ask`, `deny` |
| `--session` | `-s` | Session ID to restore |
| `--session-name` | `-n` | Set session name |
| `--lang` | `-l` | Interface language |
| `--tui` | `-t` | Start in TUI mode |
| `--query` | `-q` | Non-interactive query mode |
| `--mcp-serve` | | Start as MCP server |
| `--clear-crash` | | Clear crash logs |
| `--version` | | Show version |

## Interactive Commands

| Command | Description |
|---------|-------------|
| `/help` | Show help |
| `/exit`, `/quit` | Exit (saves session) |
| `/reset` | Reset conversation context |
| `/context` | Show context info (messages, tokens, context bar) |
| `/tools` | Show available tools |
| `/model <name>` | Switch model |
| `/provider <name>` | Switch provider |
| `/sessions` | Show saved sessions |
| `/session <id>` | Restore session |
| `/rename <name>` | Rename current session |
| `/compact` | Force context compaction |
| `/clear` | Clear screen |
| `/undo` | Undo last file change |
| `/undoall` | Undo all file changes in session |
| `/diff` | Show all file changes |
| `/lang <code>` | Switch interface language |
| `/auto` | Toggle autopilot mode |
| `/tui` | Switch to TUI mode |
| `/cli` | Switch to CLI mode |
| `/config` | Show current configuration |

## Agent Instructions

BugBuster reads project-specific instructions from agent instruction files. These define coding standards, project rules, and preferences.

### Supported Files (priority order)

| File | Format | Tool |
|------|--------|------|
| `AGENT.md` | Markdown | BugBuster native |
| `CLAUDE.md` | Markdown | Claude Code |
| `.cursorrules` | Text | Cursor |
| `.github/copilot-instructions.md` | Markdown | GitHub Copilot |
| `.windsurfrules` | Text | Windsurf |
| `.aider.conf.yml` | YAML | Aider |
| `.clinerules` | Text | Cline |

All found files are loaded and appended to the system prompt.

### Example `AGENT.md`

```markdown
# Project Instructions

## Code Style
- Use Go 1.21+ features
- Prefer table-driven tests
- All exported functions must have doc comments

## Testing
- Write tests before fixing bugs
- Use `t.Parallel()` where possible
- Minimum 80% code coverage

## Git
- Conventional commits: feat, fix, refactor, test, docs
- Always run `go test ./...` before commit
```

## Provider Types

### OpenAI

```yaml
type: openai
api_key: ${OPENAI_API_KEY}
model: gpt-4o           # or gpt-4o-mini, o1-preview, o3-mini
max_tokens: 8192
context_window: 128000
```

Supports extended thinking for `o1` and `o3` models.

### Anthropic

```yaml
type: anthropic
api_key: ${ANTHROPIC_API_KEY}
model: claude-sonnet-4-20250514  # or claude-haiku-4-20250514
max_tokens: 8192
context_window: 200000
```

Supports extended thinking (Claude's "thinking" blocks).

### Ollama

```yaml
type: ollama
base_url: http://localhost:11434
model: llama3           # or codellama, mistral, deepseek-coder, etc.
max_tokens: 4096
context_window: 32000
```

No API key required. Works with any Ollama-compatible model.

### Cavibora

```yaml
type: cavibora
api_key: ${CAVIBORA_API_KEY}
model: cavibora-v1
max_tokens: 8192
context_window: 128000
```

Wave-based associative AI engine.

### OpenAI-Compatible

```yaml
type: openai
api_key: ${API_KEY}
base_url: https://api.deepseek.com/v1  # or any OpenAI-compatible endpoint
model: deepseek-chat
max_tokens: 8192
context_window: 128000
```

Works with DeepSeek, Together AI, Groq, Fireworks AI, and any OpenAI-compatible API.
