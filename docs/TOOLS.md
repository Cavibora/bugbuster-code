# Tools Reference

BugBuster Code provides 21 built-in tools that the AI agent can use to interact with your codebase and environment.

## Tool Overview

| Tool | Type | Description |
|------|------|-------------|
| `read` | Sync | Read file contents or list directory |
| `write` | Sync | Write or create files |
| `edit` | Sync | Find and replace text in files |
| `bash` | Async | Execute shell commands |
| `grep` | Sync | Search files by regular expression |
| `glob` | Sync | Find files by pattern |
| `ask` | Sync | Query an external LLM |
| `ask_user` | Sync | Ask the user for input |
| `learn` | Sync | Train the model on input/output pairs |
| `web_fetch` | Sync | Fetch content from URLs |
| `browse` | Sync | Universal search & content tool with headless browser |
| `memory` | Sync | Persistent session-scoped memory for important facts |
| `delegate_task` | Async | Delegate a subtask to a sub-agent |
| `todo_write` | Sync | Create or update a task checklist |
| `todo_read` | Sync | Read the current task checklist |
| `lsp` | Sync | Language Server Protocol analysis |
| `search_context` | Sync | Search archival context memory |
| `compact_force` | Sync | Aggressively reduce context size |
| `self_info` | Sync | Query model identity, context usage, and environment |
| `screenshot` | Sync | Capture desktop, window, or screen region |
| `send_file` | Sync | Send image/audio/document to model |
| `tts` | Sync | Text-to-speech (OpenAI TTS or system) |
| `stt` | Sync | Speech-to-text (Whisper or local) |

---

## File Operations

### `read`

Read file contents or list directory contents.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | ✅ | File or directory path |

**Behavior:**
- If `path` is a file: returns file contents with line numbers
- If `path` is a directory: returns listing of files and subdirectories
- Respects `.gitignore` patterns
- Blocks access to secret files (`.env`, SSH keys, credentials)
- Enforces path security (no `..` traversal outside working directory)

**Examples:**
```json
{"path": "src/main.go"}
{"path": "src/"}
```

---

### `write`

Write content to a file. Creates parent directories if needed.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | ✅ | File path |
| `content` | string | ✅ | File content |

**Behavior:**
- Creates parent directories automatically
- Returns a diff of changes (unified diff format)
- Tracks changes for `/undo` command
- Blocks writes to system paths (`/etc`, `/usr`, `/System`)
- Respects sandbox directory restrictions

**Examples:**
```json
{"path": "src/utils/helper.go", "content": "package utils\n\nfunc Hello() string {\n    return \"Hello!\"\n}\n"}
```

---

### `edit`

Find and replace text in a file.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | ✅ | File path |
| `old` | string | ✅ | Text to find |
| `new` | string | ✅ | Replacement text |

**Behavior:**
- Finds first occurrence of `old` text and replaces with `new`
- Returns a diff of changes
- Tracks changes for `/undo` command
- Error if `old` text not found
- Error if `old` text found multiple times (ambiguous)

**Examples:**
```json
{"path": "main.go", "old": "fmt.Println(\"hello\")", "new": "fmt.Println(\"Hello, World!\")"}
```

---

## Search

### `grep`

Search files by regular expression.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `pattern` | string | ✅ | Regular expression pattern |
| `path` | string | ❌ | Directory to search (default: `.`) |
| `file_pattern` | string | ❌ | File mask (e.g., `*.go`, `**/*.rs`) |
| `case_insensitive` | string | ❌ | `"true"` to ignore case |

**Behavior:**
- Returns matching lines with file paths and line numbers
- Respects `.gitignore` patterns
- Limits results to `max_grep_results` (default: 100)
- Skips binary files

**Examples:**
```json
{"pattern": "func main\\(\\)", "path": "cmd/", "file_pattern": "*.go"}
{"pattern": "TODO|FIXME", "case_insensitive": "true"}
```

---

### `glob`

Find files matching a pattern.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `pattern` | string | ✅ | File pattern (e.g., `*.go`, `**/*.rs`) |
| `path` | string | ❌ | Directory to search (default: `.`) |

**Behavior:**
- Supports standard glob patterns: `*`, `**`, `?`, `[...]`
- Returns list of matching file paths
- Limits results to `max_glob_results` (default: 1000)
- Respects `.gitignore` patterns

**Examples:**
```json
{"pattern": "**/*.go", "path": "pkg/"}
{"pattern": "*_test.go"}
```

---

## Execution

### `bash`

Execute shell commands. Supports both synchronous and asynchronous execution.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `command` | string | ✅ | Shell command to execute |
| `timeout` | string | ❌ | Timeout in seconds (default: 30) |
| `workdir` | string | ❌ | Working directory |

**Behavior:**
- Executes command via `/bin/bash -c`
- Combines stdout and stderr
- Truncates output at 50,000 characters
- Configurable timeout (default: 30 seconds)
- Blocks dangerous commands when `allow_network: false` (`curl`, `wget`, `nc`, etc.)
- Respects `blocked_commands` list from config
- Supports async execution for long-running commands with progress updates

**Security:**
- Commands are validated against blocked list
- Network commands blocked by default
- Timeout enforced via `exec.CommandContext`

**Examples:**
```json
{"command": "go test ./... -count=1", "timeout": "120"}
{"command": "git status", "workdir": "/path/to/project"}
```

---

## AI Interaction

### `ask`

Query an external LLM with a prompt.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `prompt` | string | ✅ | Question text |

**Behavior:**
- Sends prompt to the configured LLM
- Returns the response text
- Useful for getting second opinions or validation

---

### `ask_user`

Ask the user for input during execution.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `question` | string | ✅ | Question to ask |

**Behavior:**
- Pauses execution and prompts the user
- Returns user's response
- Useful for clarifications and confirmations

---

### `learn`

Train the model on an input/output pair for in-context learning.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `input` | string | ✅ | Input example |
| `output` | string | ✅ | Expected output |
| `type` | string | ❌ | `"text"` or `"code"` |

**Behavior:**
- Stores the example in session context
- Model uses examples for few-shot learning
- Examples persist within the session

---

## Task Management

### `delegate_task`

Delegate a subtask to an isolated sub-agent.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `task` | string | ✅ | Task description |
| `context` | string | ❌ | Additional context or constraints |

**Behavior:**
- Creates an isolated `AgentLoop` with separate context
- Sub-agent has access to all tools except `delegate_task` (prevents recursion)
- Default timeout: 10 minutes
- Default max iterations: 30
- Concurrency limited by semaphore (default: 3 concurrent sub-agents)
- Returns summary of sub-agent's work
- Progress events forwarded to parent stream

**Sub-agent system prompt:**
- Instructs to complete the task and return a clear result
- Requires a final text summary (not just tool calls)
- On iteration limit: injects summary request

---

### `todo_write`

Create or update a task checklist.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `todos` | string | ✅ | JSON array of `{id, subject, status}` |

**Status values:** `"pending"`, `"in_progress"`, `"completed"`

**Example:**
```json
{
  "todos": "[{\"id\":\"1\",\"subject\":\"Fix bug in parser\",\"status\":\"in_progress\"},{\"id\":\"2\",\"subject\":\"Add tests\",\"status\":\"pending\"}]"
}
```

---

### `todo_read`

Read the current task checklist.

**Parameters:** None

**Returns:** Current checklist with statuses.

---

## Code Analysis

### `lsp`

Language Server Protocol analysis.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `operation` | string | ✅ | `go_to_definition`, `find_references`, `hover`, `document_symbols` |
| `file_path` | string | ✅ | Absolute path to source file |
| `line` | string | ❌ | Line number (1-based) |
| `character` | string | ❌ | Character offset (1-based) |

**Behavior:**
- Connects to configured LSP server for the file's language
- Supports: `go_to_definition`, `find_references`, `hover`, `document_symbols`
- LSP servers configured in `config.yaml` under `lsp.servers`

**Configuration example:**
```yaml
lsp:
  timeout: 10
  servers:
    go:
      command: gopls
      args: ["serve"]
    rust:
      command: rust-analyzer
```

---

### `search_context`

Search archival context — past decisions, code changes, and discussions.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query` | string | ✅ | Search query — keywords or phrases |
| `max_results` | int | ❌ | Max results (default: 5, max: 20) |

**Behavior:**
- Searches through compacted/archived conversation context
- Useful for recalling information from earlier in the session
- Returns relevant past decisions, code changes, and discussions

---

## Web

### `web_fetch`

Fetch content from a URL.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `url` | string | ✅ | URL to fetch |
| `method` | string | ❌ | HTTP method: `GET`, `HEAD`, `POST` |
| `headers` | string | ❌ | Headers in `key:value,key2:value2` format |

**Behavior:**
- Fetches URL content via HTTP
- Returns response body as text
- Respects `allow_network` security setting
- Default timeout: 30 seconds

---

### `browse`

Universal search & content tool with configurable headless browser. Replaces `web_fetch` for advanced use cases.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `action` | string | ✅ | `search`, `fetch`, `extract` (aliases: `find`, `render`, `open`, `readability`) |
| `query` | string | ❌ | Search query (for `search` action) |
| `url` | string | ❌ | URL to fetch/extract |
| `selector` | string | ❌ | CSS selector to extract specific elements |
| `max_results` | string | ❌ | Max search results (default: 10, max: 20) |
| `engine` | string | ❌ | Override search engine for this query: `duckduckgo`, `google`, `yandex`, `bing` |

**Actions:**

| Action | Description |
|--------|-------------|
| `search` | Search the web using configured search engine |
| `fetch` | Render a page with headless Chrome (full JS/AJAX support) |
| `extract` | Extract clean text from a page (removes nav, scripts, styles) |

**Search Engines:**

| Engine | Method | JS Rendering |
|--------|--------|-------------|
| `duckduckgo` (default) | HTTP HTML | Not needed |
| `google` | HTTP HTML | Not needed |
| `yandex` | Headless Chrome | Required (JS-only) |
| `bing` | HTTP HTML | Not needed |

**Browser Engines:**

| Engine | Description |
|--------|-------------|
| `chromedp` (default) | Headless Chrome via chromedp (Go native) |
| `rod` | Alternative Chrome driver |
| `playwright` | Playwright browser automation |
| `http` | Simple HTTP fetch (no JS rendering) |

**Behavior:**
- Uses headless Chrome for JS-heavy pages (AJAX, SPAs)
- Falls back to simple HTTP if Chrome unavailable
- Extracts clean text: removes `<script>`, `<style>`, `<nav>`, `<header>`, `<footer>`
- Truncates output at 50,000 characters
- Per-query engine override: `engine=yandex`

**Examples:**
```json
{"action": "search", "query": "Rust async patterns", "engine": "google"}
{"action": "fetch", "url": "https://example.com"}
{"action": "extract", "url": "https://example.com/article"}
{"action": "search", "query": "конкуренты cavibora", "engine": "yandex"}
```

**Configuration:**
```yaml
tools:
  browse:
    engine: chromedp          # chromedp, rod, playwright, http
    search_engine: duckduckgo # duckduckgo, google, yandex, bing
    timeout: 30
    max_results: 10
    user_agent: "Mozilla/5.0..."
    headless: true
    chrome_path: ""           # auto-detect if empty
```

---

### `memory`

Persistent session-scoped memory for storing important project facts. Each session has its own memory file. The model is **required** to save and check memory.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `action` | string | ✅ | `save`, `load`, `list`, `delete` |
| `key` | string | ❌ | Fact identifier |
| `value` | string | ❌ | Fact value |
| `category` | string | ❌ | Group name (default: `general`) |

**Actions:**

| Action | Description |
|--------|-------------|
| `save` | Save or update a fact |
| `load` | Load facts by key or category |
| `list` | List all stored facts |
| `delete` | Delete a fact by key |

**Mandatory Rules (enforced via system prompt):**
- The model MUST save facts when it discovers them
- The model MUST check memory at the START of every task
- **Save triggers** — save immediately when:
  - User says "remember", "don't forget", "important", "note", "keep in mind"
  - Project paths, database hosts, API endpoints, credentials
  - Dataset sizes, test results, build times
  - User corrections or warnings ("don't do X", "always do Y")
  - Architecture decisions, design patterns
- If memory has facts for the current project, the model MUST follow them strictly

**Storage Priority:**
| Priority | Path | When |
|----------|------|------|
| 1 | `<project>/.bugbuster/memory/<session-id>.md` | `.bugbuster/` exists in project directory |
| 2 | `~/.bugbuster/memory/<session-id>.md` | Fallback (no `.bugbuster/` in project) |

**Behavior:**
- Each session has its own memory file (session-scoped)
- Facts are automatically injected into the system prompt at session start
- Facts are re-injected after context compaction (never lost)
- Markdown format (human-readable and editable)
- Case-insensitive key matching
- Thread-safe (concurrent access protected by mutex)
- Categories are sorted alphabetically

**Examples:**
```json
{"action": "save", "key": "project_path", "value": "/Users/ss/ai/grfn", "category": "project"}
{"action": "save", "key": "avoid_mysql_memory_load", "value": "User warned: never load full MySQL datasets into memory, use pagination", "category": "warnings"}
{"action": "load", "key": "project_path"}
{"action": "load", "category": "warnings"}
{"action": "list"}
{"action": "delete", "key": "temp_data"}
```

**Memory file format:**
```markdown
# BugBuster Agent Memory

## database
- **mysql_host**: localhost:3306
- **mysql_user**: root

## project
- **language**: Rust
- **project_path**: /Users/ss/ai/grfn

## warnings
- **avoid_mysql_memory_load**: User warned: never load full MySQL datasets into memory, use pagination
```

**Auto-injection:** At session start and after context compaction, all stored facts are injected into the system prompt:
```
Important facts about this project (from agent memory):

[database]
- mysql_host: localhost:3306

[project]
- language: Rust
- project_path: /Users/ss/ai/grfn

[warnings]
- avoid_mysql_memory_load: User warned: never load full MySQL datasets into memory, use pagination
```

---

### `compact_force`

Aggressively reduce context size by stripping tool calls, errors, thinking blocks, and low-value data. Auto-triggers when context >50% and iteration speed drops 3x below initial.

**Parameters:** None

**Behavior:**
- Strips all tool calls, tool results, errors, and thinking blocks from context
- Keeps only system prompt, recent messages, and memory facts
- Reduces context to ~15% of its original size
- Also triggered automatically by the speed mirror when slowdown >3x
- Can be triggered manually via `/compact!` command

**When it auto-triggers:**
- Context usage >50% AND iteration speed is 3x slower than initial
- Model receives a system message about slowdown before auto-trigger

**Examples:**
```json
{}
```

---

### `self_info`

Query information about the model, provider, agent context, and system environment.

**Parameters:** None

**Behavior:**
- Returns model name and provider
- Returns context window size and current usage percentage
- Returns message count in conversation
- Returns system information (OS, architecture, runtime)
- Returns agent configuration details

**Example output:**
```
=== Model & Provider ===
Provider: anthropic
Model: claude-sonnet-4-20250514
Context Window: 200000
Current Usage: 45%

=== Agent ===
Session: abc123
Messages: 42
Tools: 21

=== System ===
OS: darwin/arm64
Go: 1.22.0
Working Dir: /Users/ss/project
```

**Examples:**
```json
{}
```

---

## Multimodal

### `screenshot`

Capture a screenshot of the desktop, a specific window, or a screen region.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `mode` | string | ❌ | `fullscreen` (default), `window`, `region` |
| `display` | string | ❌ | Display number (default: main display) |
| `region` | string | ❌ | Region in `x,y,w,h` format (for `region` mode) |
| `format` | string | ❌ | `png` (default) or `jpeg` |
| `quality` | int | ❌ | JPEG quality 1-100 (default: 85) |

**Modes:**

| Mode | Description |
|------|-------------|
| `fullscreen` | Capture entire screen |
| `window` | Interactive window selection (click to select) |
| `region` | Capture specific region by coordinates |

**Behavior:**
- Uses `screencapture` on macOS, `scrot`/`xdg-screenshot` on Linux
- Automatically sends captured image to vision-capable model
- Falls back to base64 encoding if no native tool available
- Images are included as `image` content blocks in the message

**Configuration:**
```yaml
tools:
  screenshot:
    enabled: true
    format: png
    quality: 85
```

**Examples:**
```json
{"mode": "fullscreen"}
{"mode": "window"}
{"mode": "region", "region": "100,100,800,600"}
```

---

### `send_file`

Send a file (image, audio, document) to the model for analysis.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | ✅ | Path to file |
| `media_type` | string | ❌ | MIME type (auto-detected if not specified) |

**Supported file types:**
| Type | Extensions | Description |
|------|-----------|-------------|
| Image | `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp` | Sent as vision content |
| Audio | `.mp3`, `.wav`, `.ogg`, `.flac`, `.m4a` | Sent as audio content |
| Document | `.pdf`, `.txt`, `.csv` | Sent as text content |

**Behavior:**
- Auto-detects MIME type from file extension
- Encodes file as base64 for transmission
- Images are sent as `image` content blocks (vision)
- Audio files are sent as `audio` content blocks
- Documents are extracted and sent as text
- Respects `max_file_size` limit (default: 10MB)

**Examples:**
```json
{"path": "/tmp/screenshot.png"}
{"path": "/tmp/recording.mp3", "media_type": "audio/mpeg"}
{"path": "report.pdf"}
```

---

### `tts`

Text-to-speech synthesis. Generates audio from text using OpenAI TTS or system speech.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `text` | string | ✅ | Text to synthesize |
| `voice` | string | ❌ | Voice name (default: `alloy`) |
| `model` | string | ❌ | TTS model: `tts-1` (default) or `tts-1-hd` |
| `speed` | float | ❌ | Speed multiplier 0.25-4.0 (default: 1.0) |
| `output` | string | ❌ | Output file path (default: temp file) |
| `provider` | string | ❌ | `openai` (default) or `system` |

**OpenAI Voices:**

| Voice | Description |
|-------|-------------|
| `alloy` | Balanced, neutral |
| `echo` | Clear, authoritative |
| `fable` | Expressive, storytelling |
| `onyx` | Deep, confident |
| `nova` | Warm, friendly |
| `shimmer` | Bright, cheerful |

**System Voices (macOS/Linux):**
- Uses `say` on macOS, `espeak` on Linux
- Voice names depend on system-installed voices

**Behavior:**
- Generates audio file and plays it automatically
- Uses `afplay` on macOS, `aplay` on Linux for playback
- Falls back to system TTS if OpenAI unavailable
- Returns file path of generated audio

**Configuration:**
```yaml
tools:
  tts:
    enabled: true
    model: tts-1
    voice: alloy
    speed: 1.0
```

**Examples:**
```json
{"text": "Hello! BugBuster Code is running.", "voice": "alloy"}
{"text": "Build complete.", "provider": "system"}
{"text": "Critical error found!", "voice": "onyx", "model": "tts-1-hd"}
```

---

### `stt`

Speech-to-text transcription. Transcribes audio files or records from microphone.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file` | string | ❌ | Path to audio file (for file transcription) |
| `duration` | string | ❌ | Recording duration (for mic input, e.g. `10s`, `30s`) |
| `language` | string | ❌ | Language code (`en`, `ru`, etc.) or auto-detect |
| `model` | string | ❌ | STT model: `whisper-1` (default) |
| `provider` | string | ❌ | `openai` (default) or `local` |

**Modes:**

| Mode | Parameters | Description |
|------|-----------|-------------|
| File | `file` | Transcribe existing audio file |
| Mic | `duration` | Record from microphone then transcribe |

**Behavior:**
- Uses OpenAI Whisper API by default
- Falls back to `ffmpeg` + local whisper if OpenAI unavailable
- Mic recording uses `ffmpeg` or `sox` on macOS/Linux
- Returns transcribed text
- Supports MP3, WAV, OGG, FLAC, M4A formats

**Configuration:**
```yaml
tools:
  stt:
    enabled: true
    model: whisper-1
    language: ""  # auto-detect
```

**Examples:**
```json
{"file": "/tmp/meeting.mp3"}
{"duration": "10s", "language": "en"}
{"file": "/tmp/voice_memo.wav", "language": "ru"}
```

---

## Skills

Skills are reusable step-by-step procedures that guide the agent through common tasks. Unlike tools (which perform single operations), skills combine **instructions + context + tools** into a workflow.

### Built-in Skills

| Skill | Description | Steps |
|-------|-------------|-------|
| `debug` | Systematic debugging | Read error → Find file → Read context → Propose fix → Run tests |
| `refactor` | Safe refactoring | Find usages → Dependency graph → Plan → Make changes → Verify tests |
| `review` | Code review | Read diff → Check style → Check security → Check tests → Write report |
| `deploy` | Deployment | Run tests → Build → Deploy → Health check → Rollback if needed |
| `analyze` | Codebase analysis | Structure overview → Find patterns → Metrics → Recommendations |

### Commands

| Command | Description |
|---------|-------------|
| `/skills` | List available skills |
| `/skill <name>` | Activate a skill |
| `/skill off` | Deactivate current skill |

### Custom Skills

Create `.bugbuster/skills/my-skill.md` in your project:

```markdown
# My Custom Skill

## Description
Does something custom and useful

## Steps
1. First, read the relevant files
2. Analyze the code structure
3. Identify the problem
4. Propose a solution
5. Implement the fix
6. Run tests to verify

## Tools
- read
- grep
- edit
- bash

## Context
- README.md
- go.mod
- pkg/config/config.go
```

### How Skills Work

1. **Activation** — `/skill debug` injects the skill's instructions into the system prompt
2. **Execution** — the model follows the skill's steps using the listed tools
3. **Context** — files listed in `Context` are automatically read when the skill activates
4. **Compaction-safe** — skills are re-injected after context compaction (never lost)
5. **Deactivation** — `/skill off` removes the skill from the system prompt

### Skill File Format

| Section | Required | Description |
|---------|----------|-------------|
| `# Name` | ✅ | Skill name (H1 heading) |
| `## Description` | ✅ | What the skill does |
| `## Steps` | ✅ | Ordered list of steps to follow |
| `## Tools` | ❌ | Tools the skill should use |
| `## Context` | ❌ | Files to read when skill activates |

### Storage Priority

| Priority | Path | When |
|----------|------|------|
| 1 | `<project>/.bugbuster/skills/*.md` | `.bugbuster/` exists in project directory |
| 2 | `~/.bugbuster/skills/*.md` | Fallback (global skills) |

---

## Tool Security

All tools are subject to security checks:

1. **Path Security** (`path_security.go`)
   - Blocks `..` traversal outside working directory
   - Blocks access to secret files (`.env`, `*.pem`, `credentials.*`, `.ssh/*`)
   - Blocks writes to system paths (`/etc`, `/usr`, `/System`, `/Library`)

2. **Command Security** (`bash.go`)
   - Blocks network commands when `allow_network: false`
   - Blocks commands in `blocked_commands` list
   - Enforces timeout

3. **Sandbox Mode**
   - When `sandbox_dir` is set, all file writes restricted to that directory

4. **File Size Limits**
   - `max_file_size` limits read/write operations (default: 10MB)

5. **Granular Permissions** (`permissions` in config)
   - Per-tool permission overrides: `auto-approve`, `ask`, `deny`
   - Overrides global `permission_mode` for specific tools
   - Example: `bash: ask` requires confirmation for every command, while `memory: auto-approve` always allows

6. **Fallback Providers** (`fallback` in config)
   - When primary provider fails (network error, rate limit), automatically switches to fallback
   - Configurable retries, delay, and auto-switch-back behavior
   - Example: use OpenAI as primary, Ollama as fallback for offline resilience
