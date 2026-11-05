# Changelog

All notable changes to BugBuster Code will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.4.0] - 2026-11-05

### Added
- **HTTP control-plane API — `bugbuster serve`** — run the full agent behind a bearer-token HTTP server so other programs can drive it: create sessions, post tasks, stream events. Endpoints: `GET /health`, CRUD `/v1/sessions[/{id}]`, `POST /v1/sessions/{id}/task|inject|answer|cancel|compact`, `GET /v1/sessions/{id}/events` (SSE with `?since=` replay), `GET /v1/sessions/{id}/transcript`, `POST /v1/run` (one-shot, waits for the result). `ask_user` round-trips through `POST .../answer`; the autopilot (auto-continue) is **off** by default for headless clients (re-enable with `serve.auto_continue` or `--auto-continue`); sessions persist as JSONL transcripts and idle sessions are evicted.
- **Thread-safe `ConversationContext`** — the agent loop runs in its own goroutine and mutates the context while UI/HTTP readers touch the same data. All access now goes through `AppendMessages`/`GetMessages`/`SetMessages` behind a mutex, iteration counters are atomics, the streaming goroutine has a panic recover so one bad run can't kill the process, and compaction is deferred so `CompactForce` from an HTTP handler doesn't race the loop. Fixes the data races surfaced by `go test -race`.

### Fixed
- **Disguised upstream failures no longer stop or hang the agent** — OpenCode Zen wraps transient gateway errors in a client-looking HTTP 400 (`error.type: "server_error"` / "Upstream request failed: ... not valid JSON"), which was treated as a fatal client error. New `UpstreamError` type + detector (never misclassifies genuine `invalid_request`/`context_length`) is retried at the provider, agent-loop and autopilot levels. The retry budget is deliberately small (3 per session, then the `x-opencode-session` ID is **rotated** — the same recovery a client restart gave, since a persistent failure means the cached upstream route is poisoned) with a cap of 4 consecutive iteration failures, so the worst case is ~2 minutes with visible progress instead of a silent ~2-hour spin.
- **Silent/stalled upstream could hang forever** — the OpenAI/Anthropic `http.Client` had no transport timeouts and the streaming context carried no deadline, so `client.Do` blocked indefinitely when the gateway accepted the TCP connection but never sent response headers (the idle timer only started after the channel was returned, so it never fired). `newStreamingHTTPClient()` now sets `ResponseHeaderTimeout` (120s) plus dial/TLS timeouts (no overall `Client.Timeout`, so long generations are unaffected), and `runLoop` bounds each non-stream call by `effectiveRequestTimeout`.
- **Sessions permanently blocked by a stale image** — one image block in history made every later request fail with a deterministic HTTP 400 "model does not support image input" on text-only endpoints, and since genuine 400s are not retried the session died. New `ImageUnsupportedError` type plus `StripImages()`/`HasImages()` let the stream retry **self-heal**: images are stripped from history (keeping text, thinking and tool_result blocks so tool-call chains stay valid), the user is notified, and the request is resent image-free.
- **HTTP 400 "Messages with role 'tool' must be a response to a preceding message with 'tool_calls'"** — OpenAI-compatible providers (OpenAI, Ollama, OpenCode, Cavibora) now sanitize the message list before sending: orphan `tool` messages (e.g. left by compaction that kept `tool_result` but stripped `tool_use`) are converted to user text, and unanswered assistant `tool_calls` are stripped. Also fixed at the source: compaction now calls `EnsureToolPairIntegrity` after `stripToolCalls` in `CompactForce`, `CompactContext`, and `CompactContextWithCompactor`.
- **Exactly one system message** — all system messages are merged into a single one at position 0. Qwen3.5's chat template allows exactly one and returns HTTP 500 (`System message must be at the beginning`) otherwise.
- **Compaction reminders and performance mirrors accumulate** — `[Context was compacted...]` and `[Performance mirror]` messages accumulated over a long session (12+ in one session) and polluted the context. `CleanupTransients` now keeps only the latest of each; `maybeCompact` and `injectSpeedMirror` remove old ones before adding new. Sync mode (`runLoop`) now also runs `CleanupTransients` every iteration.
- **Compact system notifications** — shorter i18n headers (`[Summary]`/`[Recaps]`, `U:`/`A:` labels across 8 locales), shorter task reminder, mirror and auto-continue hints. `isTransientNoise` and `stripToolResultNoise` filter reminders, mirrors and `tool_result` blocks out of summaries (no more `User: [Tool output] ...` pollution); `extractRecaps` dedups and caps at 5.
- **Spinner spam** — rate-limit (429) retries update the spinner message in place instead of printing a new line per retry (was 50 identical lines), and `bash`/`write`/`edit` show a compact single-line summary instead of redrawing the full multi-line command every tick.

### Documentation
- HTTP API documented in README/ARCHITECTURE/CONFIGURATION, plus a dedicated SECURITY section (the API runs shell commands with your credentials — loopback default, refuses unauthenticated non-loopback, hardening checklist).

## [1.3.0] - 2026-08-24

### Added
- **Rate limit (HTTP 429) resilience** — automatic retry with short delay instead of stopping. Configurable via `agent.rate_limit` (`max_retries`, `delay_ms`) and the `/rate` interactive command. Rate limit messages are shown to the user but not saved to context.
- **Configurable autopilot** — `agent.autopilot.max_iterations` and `agent.autopilot.delay_ms` replace the hardcoded 5000-iteration limit and 2s delay. Configurable via config and `/auto N`.
- **Reasoning/thinking parsing for OpenRouter** — now parses `reasoning` and `reasoning_details` fields (OpenRouter) in addition to `reasoning_content` (DeepSeek). New `reasoning` provider config block (`enabled`, `effort`, `max_tokens`, `exclude`) enables reasoning tokens for models like GLM-5.2.
- **Context cleanup of transient messages** — auto-continue prompts and error hints (idle timeout, loop detection) are cleaned from context, keeping only the last auto-continue. Prevents context pollution.
- **`/rate` command** — show or configure rate limit retries in both CLI and TUI.

### Changed
- **Autopilot survives transient errors** — 429, 404, 500, 502, 503, 504 no longer stop autopilot. It continues after a short delay.
- **Autopilot doesn't falsely report completion after error** — if a stream ends with an error, plan-completion detection is skipped and autopilot retries.
- **HTTP 429 removed from provider retry policy** — rate limit retries are now handled by the agent loop (with `RateLimitError`), preventing double retries that caused long stalls.
- **Full error message shown on 429** — the complete provider error body is displayed (no truncation).
- **Comments translated to English** — all Russian comments in production and test code translated to English (except Russian string literals used as test data).

### Fixed
- **Spinner duplication in CLI** — bash and rate-limit spinners no longer stretch/duplicate; the spinner is stopped, message printed on its own line, and restarted.

## [1.2.7] - 2026-08-05

### Added
- **Config validation on load** — BugBuster now validates the config file at startup and prints errors/warnings in prominent red/yellow text. YAML parse errors show the exact line number (e.g. `yaml: line 2: did not find expected key`), so you can locate the problem immediately.
- **Hub message status visibility** — `hub_check` now shows a "Recent Message Statuses" section so the model can see whether its outgoing messages were read / replied to / ignored.
- **Hub message size limit** — new `hub.max_context_tokens` config option caps how many tokens of hub messages are injected per drain. Default (0) = 10% of context window, capped at 4096. Prevents small local models from having their context flooded by hub traffic.

### Changed
- **Agent Hub disabled by default** — `hub.enabled` now defaults to `false` (was `true`). Enable explicitly with `hub.enabled: true`.
- **Long bash/write/edit commands wrap to multiple lines** — in both TUI progress bar and CLI tool-call output, very long commands are now wrapped instead of being truncated or overflowing the terminal.

### Fixed
- **Session selection: typing a task instead of a number** — when choosing a session, if you type a task (not a session number), a new session is created and your text becomes the first request (previously the text was lost).
- **Command history after reopening a session** — readline is now recreated with the correct session history file after session selection, so `↑` history works when reopening a session.

## [1.2.6] - 2026-07-23

### Added
- **Per-provider hub configuration** — `hub.role`, `hub.intelligence`, `hub.name` can now be set per-provider, overriding global hub settings. Priority: `providers.<name>.hub.*` > `hub.*` > auto-detect.
- **Terminal window title** — shows role and project: `BugBuster coder · myproject` (uses Bubble Tea v2 `WindowTitle`). Without role: `BugBuster · myproject`.

### Changed
- **Agent Hub enabled by default** — `hub.enabled` defaults to `true` instead of `false`.

### Fixed
- **LooksLikeCompletion expanded** — added markdown heading markers (`## Recap`, `## Итог`, `# Summary`), standalone completion words (`Done`, `Done.`, `Done!`, `Готово`, `Готово.`, `Готово!`), and `committed and pushed` phrase. Removed `all tests pass` (model often continues after tests pass).

## [1.2.5] - 2026-07-15

### Added
- **Agent Hub (multi-agent coordination)** — shared workspace for multiple BugBuster Code instances to coordinate work:
  - `hub_list` — list all registered agents with their model, intelligence level, and status
  - `hub_message` — send direct messages between agents
  - `hub_broadcast` — broadcast announcements to all agents
  - `hub_alert` — send urgent alerts (e.g., "Tests are broken!")
  - `hub_info` — inspect another agent's profile, role, and system prompt
  - `hub_history` — view message history between agents
  - `hub_request` — send task requests (do, redo, stop, wait, review, test, fix) with priority
  - `hub_respond` — accept or decline task requests from other agents
  - `hub_check` — check for unread messages and pending requests
  - `hub_tasks` — view another agent's task list (or all agents' tasks)
  - `hub_status` — update own status, current task, and task list in the hub
- **Intelligence hierarchy** — auto-detected from model name or configured per model, with 5 levels (low → superior)
- **File-based persistence** — hub data stored in `.bugbuster/hub/` for cross-process agent discovery
- **Per-provider `system_prompt_file`** — load system prompt from a `.md` file (appended after `system_prompt`)
- **Per-provider `skills_dir`** — load custom skills from a directory (in addition to builtins/project/global)

## [1.2.4] - 2026-07-15

### Added
- **Per-provider system prompt** — each provider/model can have its own `system_prompt` appended to the default, allowing provider-specific instructions and coding style preferences
- **Per-provider skills** — each provider/model can activate specific skills (`debug`, `refactor`, `review`, `migrate`, `test`) via `skills` config field
- **Automatic system prompt rebuild on provider switch** — switching providers with `/provider` or `/model` now rebuilds the system prompt with the new provider's `system_prompt` and `skills`
- **SkillManager.Active() and Deactivate()** — new methods for listing active skills and deactivating them
- **Homebrew, Scoop, and nfpm packages** — restored brew tap, scoop bucket, and deb/rpm/apk packaging in GoReleaser
- **Windows builds** — added Windows to GoReleaser build targets
- **Changelog groups** — release notes now grouped by type (Features, Bug Fixes, Documentation, Other)

### Fixed
- **[Auto-continue] after Recap** — expanded `LooksLikeCompletion` with standalone recap words (Recap, Итог, Summary without colon/dash), more completion phrases (that's all, nothing else to do, task finished, mission accomplished, всё, конец, завершено, выполнено), and context compaction acknowledgment detection
- **Missing `lastUserMsgIsCompact` check in `Run()`** — non-streaming mode was missing the compaction check that streaming mode had, causing auto-continue after context compaction

## [1.2.3] - 2026-07-15

### Added
- **`/system` command** — show/set system prompt in TUI, CLI, and interactive modes. `/system` shows current prompt, `/system <text>` sets inline, `/system <file>` loads from file
- **`/system` in help** — added to `/help`, TUI help, and all 8 locale files (en/ru/de/es/fr/pt/ja/zh)

### Fixed
- **[Auto-continue] after Recap** — `EventTextDelta` replaced with `EventAutoContinue` so auto-continue messages no longer appear as model text in chat
- **More completion markers** — `LooksLikeCompletion` now recognizes `※ Итоги`, `итоги`, `резюме`, `результаты`, `※` symbol alone, `Готово`/`Сделано`/`Исправлено` without punctuation, `Changes applied`, `Fixed the issue`
- **Auto-continue in TUI** — shown as dim `↻ auto-continue` instead of full `[Auto-continue: prompting model to use tools]` text
- **Nil pointer crash** — `Spinner.UpdateMessage` crash on `EventAutoContinue` when spinner was nil after `stopActiveSpinner`

## [1.2.2] - 2025-07-15

### Fixed
- **Auto-continue skip on completion** — model no longer wastes tokens continuing after Recap/Done/Готово
- **i18n completeness** — added 324 missing translation keys to de/es/fr/ja/pt/zh locales
- **Todo write error handling** — `os.WriteFile` error in todo tool now logged instead of silently ignored

## [1.2.1] - 2025-07-15

### Fixed
- **CompactForce infinite loop** — increased keepCount 4→8, added 10-iteration cooldown, preserved baseline context
- **Cross-platform Dup2/Dup** — replaced `syscall.Dup2` with `unix.Dup2` for Linux ARM64 support

## [1.2.0] - 2025-07-14

### Added
- **Granular permissions** — per-tool permission overrides (`bash: ask`, `web_fetch: deny`, etc.)
- **Fallback providers** — automatic switch to backup provider when primary fails
- **Speed mirror** — model sees its own performance and self-optimizes context
- **Self-awareness** — `self_info` tool for model identity, context usage, environment
- **Aggressive compaction** — `/compact!` command + auto-trigger on 3x slowdown
- **Multimodal support** — screenshot, send_file, TTS, STT
- **Stale process control** — auto-kill background processes older than 7 days
- **System time injection** — current date/time in system prompt
- **Cross-platform build** — GoReleaser config for linux/darwin/windows amd64/arm64
- **Release workflow** — automatic GitHub releases on v* tags

### Fixed
- Cross-platform `syscall.Select` (macOS returns `error`, Linux returns `(int, error)`)
- OpenAI streaming usage + edit diff format for model clarity
- Context loss after compaction and premature session end
- Memory `.bak` file generation removed
- Provider `Model()` method for all providers
- `max_tokens` continuation for local models

## [1.1.0] - 2025-06-27

### Added
- Multimodal support — screenshot, send_file, TTS, STT
- Self-awareness tool (`self_info`)
- Aggressive compaction (`/compact!`)
- Speed mirror (performance monitoring)

### Fixed
- Context loss after compaction
- Memory backup file generation
- Provider interface compliance

## [1.0.0] - 2025-05-24

### Added
- Initial release
- 22 built-in tools (read, write, edit, bash, grep, glob, ask, ask_user, learn, web_fetch, browse, memory, delegate_task, todo, lsp, search_context, compact_force, self_info, screenshot, send_file, tts, stt)
- 5 LLM providers (OpenAI, Anthropic, Ollama, Cavibora, OpenAI-compatible)
- TUI and CLI modes
- Session management
- MCP client and server
- Skills system
- Sub-agents
- 8 languages (en, ru, es, fr, de, ja, zh, pt)
- Undo/change tracking
- Context archiving
- Security (path traversal, secret files, sandbox, command blocking)