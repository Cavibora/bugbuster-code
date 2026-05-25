# Tools Reference

BugBuster Code provides 14 built-in tools that the AI agent can use to interact with your codebase and environment.

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

Persistent session-scoped memory for storing important project facts. Each session has its own memory file.

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

**Behavior:**
- Each session stores facts in `.bugbuster/memory/<session-id>.md`
- Facts are automatically loaded into the system prompt at session start
- Markdown format (human-readable)
- Case-insensitive key matching
- Thread-safe (concurrent access protected by mutex)
- Categories are sorted alphabetically

**Examples:**
```json
{"action": "save", "key": "project_path", "value": "/Users/ss/ai/grfn", "category": "project"}
{"action": "save", "key": "mysql_host", "value": "localhost:3306", "category": "database"}
{"action": "load", "key": "project_path"}
{"action": "load", "category": "database"}
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
```

**Auto-injection:** At session start, all stored facts are injected into the system prompt:
```
Important facts about this project (from agent memory):

[database]
- mysql_host: localhost:3306

[project]
- language: Rust
- project_path: /Users/ss/ai/grfn
```

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
