# Changelog

All notable changes to BugBuster Code will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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