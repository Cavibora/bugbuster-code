<p align="center">
  <img src="https://raw.githubusercontent.com/Cavibora/bugbuster-code/main/docs/logo.svg" alt="BugBuster Code" width="200" onerror="this.style.display='none'"/>
</p>

<h1 align="center">BugBuster Code</h1>

<p align="center">
  <strong>Model-agnostic AI coding agent for your terminal</strong>
</p>

<p align="center">
  <a href="https://github.com/Cavibora/bugbuster-code/releases"><img src="https://img.shields.io/github/v/release/Cavibora/bugbuster-code?style=flat-square" alt="Release"></a>
  <a href="https://goreportcard.com/report/github.com/Cavibora/bugbuster-code"><img src="https://goreportcard.com/badge/github.com/Cavibora/bugbuster-code?style=flat-square" alt="Go Report"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow?style=flat-square" alt="License"></a>
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square" alt="Go Version">
  <img src="https://img.shields.io/badge/platforms-macOS%20%7C%20Linux%20%7C%20Windows-blue?style=flat-square" alt="Platforms">
</p>

<p align="center">
  Connect <strong>any LLM</strong> — OpenAI, Anthropic, Ollama, Cavibora, or OpenAI-compatible — and give it 22 tools to read, write, edit, search, and execute code. Plus <strong>multimodal</strong>: screenshots, voice, vision.
</p>

---

## ⚡ 30-Second Setup

```bash
# macOS / Linux
curl -sL https://github.com/Cavibora/bugbuster-code/releases/latest/download/bugbuster_$(uname -s)_$(uname -m).tar.gz | tar xz
mv bugbuster /usr/local/bin/

# Or build from source
go install github.com/Cavibora/bugbuster-code/cmd/bugbuster@latest

# Configure (one-time)
bugbuster config init

# Run
bugbuster                    # interactive CLI
bugbuster --tui              # terminal UI
bugbuster "Fix the bug"     # one-shot
```

## 🆚 Why BugBuster Code?

| Feature | BugBuster | Claude Code | Aider | Continue |
|---------|:---------:|:-----------:|:-----:|:--------:|
| Model-agnostic (any LLM) | ✅ | ❌ Claude only | ✅ | ✅ |
| Local models (Ollama) | ✅ | ❌ | ✅ | ✅ |
| Fallback providers | ✅ | ❌ | ❌ | ❌ |
| Per-tool permissions | ✅ | ❌ | ❌ | ❌ |
| Screenshots & Vision | ✅ | ✅ | ❌ | ❌ |
| Voice (TTS + STT) | ✅ | ❌ | ❌ | ❌ |
| Self-awareness mirror | ✅ | ❌ | ❌ | ❌ |
| Auto-compact on slowdown | ✅ | ❌ | ❌ | ❌ |
| Sub-agents | ✅ | ❌ | ❌ | ❌ |
| MCP (client + server) | ✅ | ✅ | ❌ | ✅ |
| Skills system | ✅ | ❌ | ❌ | ❌ |
| 8 languages (i18n) | ✅ | ❌ | ❌ | ❌ |
| Undo (change tracking) | ✅ | ✅ | ❌ | ❌ |
| Context archiving | ✅ | ❌ | ❌ | ❌ |

## 🛠️ 22 Built-in Tools

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
| `compact_force` | Aggressively reduce context |
| `self_info` | Query model identity & context usage |
| `screenshot` | 🖥️ Capture desktop, window, or region |
| `send_file` | 📎 Send image/audio/document to model |
| `tts` | 🔊 Text-to-speech |
| `stt` | 🎤 Speech-to-text |

## ✨ Highlights

- 🪞 **Speed Mirror** — model sees its own performance and self-optimizes context usage
- 💥 **Aggressive Compaction** — `/compact!` for emergency context reduction; auto-triggers on 3x slowdown
- 🧠 **Self-Awareness** — `self_info` tool lets the model know its provider, context usage, and environment
- 🔐 **Granular Permissions** — per-tool permission overrides (`bash: ask`, `web_fetch: deny`, etc.)
- 🔄 **Fallback Providers** — automatic switch to backup provider when primary fails
- 🖥️ **Screenshots & Vision** — capture desktop, window, or region; send images to vision models
- 🎤🔊 **Voice** — speech-to-text (Whisper) and text-to-speech (OpenAI TTS or system)
- 🌍 **8 Languages** — English, Russian, Spanish, French, German, Japanese, Chinese, Portuguese
- 🤝 **Sub-agents** — isolated context, parallelism semaphore, timeouts
- 📡 **MCP** — both client and server (stdio, SSE, streamable HTTP)
- 🎯 **Skills** — reusable step-by-step procedures (debug, refactor, review, deploy, analyze + custom)

## 📦 Configuration

Create `bugbuster.yaml` in your project root or `~/.bugbuster/config.yaml`:

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

  ollama:
    type: ollama
    base_url: http://localhost:11434
    model: llama3

agent:
  permission_mode: auto-approve
  language: en

  # Per-tool permission overrides
  permissions:
    bash: ask          # always ask before running commands
    web_fetch: deny     # block HTTP requests
    kill: deny          # block process killing

  # Fallback provider — switch when primary fails
  fallback:
    provider: ollama
    max_retries: 2
    retry_delay_ms: 1000
    auto_switch_back: true

  # Auto-switch provider by task type
  agent_providers:
    thinking: anthropic    # complex reasoning → Claude
    coding: openai        # code generation → GPT-4o
    fast: ollama           # quick tasks → local model

tools:
  screenshot:
    enabled: true
  tts:
    enabled: true
    model: tts-1
    voice: alloy
  stt:
    enabled: true
    model: whisper-1
```

Full config reference: [docs/CONFIGURATION.md](docs/CONFIGURATION.md)

## 🔒 Security

- **Path traversal** protection — blocks `..` outside working directory
- **Secret files** — blocks access to `.env`, `*.pem`, `credentials.*`, `.ssh/*`
- **System paths** — blocks writes to `/etc`, `/usr`, `/System`, `/Library`
- **Command blocking** — configurable blocked commands list
- **Network control** — `allow_network: false` blocks all HTTP
- **Sandbox mode** — restrict all file writes to a directory
- **Per-tool permissions** — granular `auto-approve`/`ask`/`deny` per tool

## 🌍 Languages

BugBuster Code speaks your language:

```bash
bugbuster --lang ru    # Русский
bugbuster --lang es    # Español
bugbuster --lang fr    # Français
bugbuster --lang de    # Deutsch
bugbuster --lang ja    # 日本語
bugbuster --lang zh    # 中文
bugbuster --lang pt    # Português
```

## 📖 Documentation

- [Architecture](docs/ARCHITECTURE.md) — how it works internally
- [Tools Reference](docs/TOOLS.md) — all 22 tools in detail
- [Configuration](docs/CONFIGURATION.md) — full config reference
- [Contributing](docs/CONTRIBUTING.md) — how to contribute
- [Security Policy](docs/SECURITY.md) — reporting vulnerabilities
- [README (Русский)](docs/README.ru.md) — документация на русском

## 🤝 Contributing

We welcome contributions! See [CONTRIBUTING.md](docs/CONTRIBUTING.md) for guidelines.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing`)
5. Open a Pull Request

## 📄 License

MIT License — see [LICENSE](LICENSE) for details.

---

<p align="center">
  Made with ❤️ by <a href="https://github.com/Cavibora">Cavibora</a>
</p>