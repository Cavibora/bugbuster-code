# Security Model

BugBuster Code operates with direct access to your filesystem and shell. This document explains the security measures in place and how to configure them.

## Threat Model

BugBuster executes AI-generated code and file operations. The primary risks are:

1. **File system damage** — AI writes to unintended files
2. **Data exfiltration** — AI reads sensitive files and sends content to LLM API
3. **Command injection** — AI executes dangerous shell commands
4. **Supply chain** — Malicious MCP servers or plugins

## Security Layers

### Layer 1: Path Security

All file operations (`read`, `write`, `edit`) go through path security checks:

```go
// Blocked patterns:
// - Path traversal: ../../../etc/passwd
// - Secret files: .env, *.pem, id_rsa, credentials.json
// - System paths: /etc/*, /usr/*, /System/*, /Library/*
// - Hidden files in home: ~/.ssh/*, ~/.gnupg/*
```

**Configuration:**
```yaml
security:
  sandbox_dir: "/path/to/project"  # restrict all writes to this directory
```

**Blocked file patterns:**
- `.env`, `.env.*`
- `*.pem`, `*.key`, `*.p12`, `*.pfx`
- `id_rsa`, `id_ed25519`, `id_ecdsa`
- `credentials.json`, `service-account*.json`
- `.ssh/*`, `.gnupg/*`, `.aws/*`
- `.htpasswd`, `.netrc`

### Layer 2: Command Security

Shell commands (`bash` tool) are validated before execution:

```yaml
security:
  allow_network: false        # blocks: curl, wget, nc, ncat, socat, ssh, scp, rsync, ftp, telnet
  blocked_commands:           # additional blocked commands
    - "rm -rf /"
    - "mkfs"
    - "dd if="
    - ":(){ :|:& };:"
```

**Default blocked commands** (when `allow_network: false`):
- Network: `curl`, `wget`, `nc`, `ncat`, `socat`, `ssh`, `scp`, `rsync`
- Download: `apt-get`, `yum`, `brew install`, `pip install`

### Layer 3: Permission Modes

```yaml
agent:
  permission_mode: auto-approve  # auto-approve | ask | deny
```

| Mode | Behavior |
|------|----------|
| `auto-approve` | All tool calls execute automatically (default) |
| `ask` | Prompt user before each tool call |
| `deny` | Block all tool calls (read-only mode) |

### Layer 4: File Size Limits

```yaml
tools:
  max_file_size: 10485760  # 10MB — blocks reading/writing files larger than this
```

### Layer 5: Timeout Protection

```yaml
agent:
  request_timeout: 1200    # 20 min — max time for single LLM request
  thinking_timeout: 600    # 10 min — max time without tokens from model
  idle_timeout: 300        # 5 min — streaming timeout without events

tools:
  bash_timeout: 30         # 30 sec — default bash command timeout
```

### Layer 6: Loop Detection

Prevents the AI from repeating the same action indefinitely:

```yaml
agent:
  loop_detection:
    repeat_threshold: 6           # identical consecutive calls = loop
    tool_repeat_threshold: 8      # same tool + same params = loop
    text_similarity_threshold: 0.65  # text similarity = loop
    text_similarity_window: 4     # how many responses to check
```

When a loop is detected, BugBuster stops and asks the user for guidance.

## MCP Security

MCP (Model Context Protocol) servers run as external processes. Security considerations:

### Client Mode (connecting to MCP servers)

```yaml
mcp:
  servers:
    suspicious-server:
      type: stdio
      command: /path/to/untrusted/binary
      enabled: false  # disabled by default
```

**Risks:**
- MCP servers can execute arbitrary code
- Servers receive tool call parameters (may contain file contents)
- Servers can return arbitrary data that gets injected into context

**Mitigations:**
- Review MCP server code before enabling
- Use `enabled: false` by default
- Restrict network access for MCP server processes

### Server Mode (BugBuster as MCP server)

```yaml
mcp_serve:
  transport: stdio
  enabled: false
```

**Risks:**
- External clients can invoke all tools
- No authentication on stdio transport

**Mitigations:**
- Use SSE/HTTP with authentication headers
- Set `prefix` to namespace tools
- Keep `enabled: false` when not needed

## Data Privacy

### What is sent to LLM APIs

BugBuster sends the following to your configured LLM provider:

1. **System prompt** — agent instructions, tool definitions
2. **Conversation history** — your messages and AI responses
3. **Tool results** — file contents, command outputs, search results
4. **Agent instructions** — contents of `AGENT.md`, `CLAUDE.md`, etc.

### What is NOT sent

- Files not explicitly read by the agent
- Environment variables (only `${VAR}` references in config are resolved locally)
- Other sessions' data
- Crash logs

### Local-only data

All of the following stay on your machine:

- Session files (`~/.bugbuster/sessions/`)
- Crash logs (`~/.bugbuster/crashes/`)
- Configuration files
- Agent instruction files

### Provider-specific notes

| Provider | Data destination | Notes |
|----------|-----------------|-------|
| OpenAI | OpenAI servers | Subject to OpenAI's data policy |
| Anthropic | Anthropic servers | Subject to Anthropic's data policy |
| Ollama | Local (`localhost:11434`) | Data stays on your machine |
| Cavibora | Cavibora servers | Subject to Cavibora's data policy |
| OpenAI-compatible | Configured `base_url` | Depends on provider |

## Best Practices

### 1. Use Ollama for sensitive code

```yaml
default_provider: ollama
providers:
  ollama:
    type: ollama
    base_url: http://localhost:11434
    model: codellama
```

No data leaves your machine.

### 2. Restrict file access

```yaml
security:
  sandbox_dir: "/path/to/project"
  allow_network: false

tools:
  allowed_dirs:
    - "/path/to/project/src"
    - "/path/to/project/tests"
```

### 3. Use `ask` permission mode for untrusted models

```yaml
agent:
  permission_mode: ask
```

You'll be prompted before every tool call.

### 4. Review agent instructions

Check `AGENT.md` and other instruction files for sensitive information. Their contents are sent to the LLM with every request.

### 5. Keep API keys secure

```yaml
# Good: use environment variables
providers:
  openai:
    api_key: ${OPENAI_API_KEY}

# Bad: hardcode keys
providers:
  openai:
    api_key: sk-abc123...  # DON'T DO THIS
```

### 6. Use `.gitignore` for session files

```gitignore
.bugbuster/
bugbuster.yaml
```

## Crash Handler

BugBuster includes a crash handler that:

1. **Redirects stderr** to a crash log file (`~/.bugbuster/crashes/`)
2. **Shows friendly message** instead of raw stack trace
3. **Preserves session** — sessions are saved on crash via signal handlers
4. **Notifies on restart** — shows previous crash info on next launch

```bash
# View crash logs
ls ~/.bugbuster/crashes/

# Clear crash logs
bugbuster --clear-crash
```

## Reporting Security Issues

If you find a security vulnerability in BugBuster Code, please report it privately via GitHub Security Advisories rather than public issues.
