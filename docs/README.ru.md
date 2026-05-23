# BugBuster Code — Документация

## Обзор

BugBuster Code — модель-агностический CLI-агент для разработки. Работает с OpenAI, Anthropic, Ollama, [Cavibora](https://github.com/Cavibora) и любым OpenAI-совместимым API.

Поддерживает:
- Чтение, запись, редактирование файлов
- Выполнение bash-команд
- Поиск по файлам (grep, glob)
- Запросы к внешним LLM (ask)
- Обучение модели (learn)
- Загрузку URL (web_fetch)
- Интерактивный режим с потоковым выводом
- Thinking/reasoning-блоки (Claude extended thinking, OpenAI o1/o3)
- Markdown-рендеринг в терминале
- История команд (↑/↓)
- Сессии с сохранением контекста

---

## Установка и запуск

```bash
go build -o bugbuster ./cmd/bugbuster/
./bugbuster
```

### Флаги командной строки

| Флаг | Сокращение | Описание |
|------|-----------|----------|
| `--config` | `-c` | Путь к конфигурационному файлу |
| `--verbose` | `-v` | Подробный вывод (логирование итераций) |
| `--model` | `-m` | Модель для использования |
| `--dir` | `-d` | Рабочая директория проекта |
| `--permission-mode` | `-p` | Режим разрешений: `auto-approve`, `ask`, `deny` |
| `--session` | `-s` | ID сессии для восстановления |
| `--lang` | `-l` | Язык интерфейса: `en`, `ru`, `es`, `fr`, `de`, `ja`, `zh`, `pt` |
| `--tui` | `-t` | Режим TUI: `auto` (по умолчанию) или `inline` |

### Подкоманды

```bash
bugbuster scan [path]           # Сканирование проекта на баги
bugbuster fix [description]    # Исправление бага по описанию
bugbuster test [path]          # Запуск тестов и анализ результатов
bugbuster config show           # Показать текущую конфигурацию
bugbuster config init           # Создать конфигурационный файл по умолчанию
bugbuster config providers      # Показать доступных провайдеров
bugbuster version               # Показать версию
```

---

## Конфигурация

Конфигурация хранится в файле `bugbuster.yaml` (или `.bugbuster.yaml`). Поиск файла:
1. Флаг `--config` (явный путь)
2. `.bugbuster.yaml` или `bugbuster.yaml` в текущей директории и выше (walk up, скрытый файл приоритетнее)
3. `~/.bugbuster/config.yaml` (домашняя директория)

### Пример конфигурации

```yaml
default_provider: zai

providers:
  zai:
    type: anthropic
    base_url: https://api.z.ai/api/anthropic
    api_key: your-api-key
    model: glm-5.1
    max_tokens: 8192          # max output tokens (API-запрос)
    context_window: 200000    # размер контекстного окна модели (для компакции)
    budget_tokens: 4096       # budget_tokens для thinking (Anthropic)
    security:
      allow_network: true
      blocked_commands:
        - rm -rf /

  local:
    type: anthropic
    base_url: http://localhost:8180/api
    api_key: ${LOCAL_API_KEY}  # Поддержка переменных окружения
    model: glm-5.1:clode
    max_tokens: 4096
    context_window: 180000
    budget_tokens: 4096
    security:
      allow_network: true

  ollama:
    type: ollama
    base_url: http://192.168.0.106:11434
    model: deepseek-v4-pro:cloud
    max_tokens: 4096
    context_window: 32000
    security:
      allow_network: false

  cavibora:
    type: cavibora
    api_key: ${CAVIBORA_API_KEY}
    model: cavibora-v1
    max_tokens: 8192
    context_window: 128000

agent:
  max_tokens: 180000          # fallback для context_window если не указан в провайдере
  keep_recent: 6               # Сколько последних сообщений сохранять при компакции
  verbose: false               # Подробный вывод
  permission_mode: auto-approve  # auto-approve | ask | deny
  language: ru                 # Язык интерфейса
  request_timeout: 1200        # Макс. время одного LLM-запроса (секунды, default: 1200 = 20 мин)
  thinking_timeout: 600       # Макс. время без токенов от модели (секунды, default: 600 = 10 мин)
  idle_timeout: 120            # Таймаут стриминга без событий (секунды, default: 120 = 2 мин)

tools:
  allowed_dirs:              # Разрешённые директории (пусто = все)
    - /home/user/projects
  max_file_size: 1048576     # Макс. размер файла для чтения (1 МБ)
  bash_timeout: 30           # Таймаут bash-команд (секунды)
  max_grep_results: 50       # Макс. результатов grep
  max_glob_results: 100      # Макс. результатов glob

security:
  allow_network: false       # Разрешить сетевые команды в bash (глобальный fallback)
  blocked_commands:          # Заблокированные команды (глобальный fallback)
    - "rm -rf /"
    - "mkfs"
    - "dd if="
  sandbox_dir: ""            # Песочница для записи файлов

# Клавиатурные привязки TUI (неуказанные — по умолчанию)
# keys:
#   send: ["enter"]
#   newline: ["shift+enter", "alt+enter", "ctrl+j"]
#   cancel: ["ctrl+c"]
#   interrupt: ["esc"]
#   history_up: ["up"]
#   history_down: ["down"]
#   scroll_up: ["pgup", "ctrl+u"]
#   scroll_down: ["pgdown", "ctrl+d"]

# MCP-серверы (Model Context Protocol)
mcp:
  servers:
    filesystem:
      type: stdio
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
      enabled: true

    # SSE-транспорт
    # remote-api:
    #   type: sse
    #   url: https://api.example.com/mcp/sse
    #   headers:
    #     Authorization: "Bearer ${MCP_API_TOKEN}"
    #   enabled: false

    # Streamable HTTP-транспорт
    # remote-http:
    #   type: streamable-http
    #   url: https://api.example.com/mcp
    #   headers:
    #     Authorization: "Bearer ${MCP_API_TOKEN}"
    #   enabled: false

# Плагины (встроенные инструменты с конфигурацией)
plugins:
  enabled:
    - filesystem
    - bash
    - web
  config:
    filesystem:
      allowed_dirs:
        - /tmp
        - /home
      max_file_size: 1048576
    bash:
      timeout: 30
      blocked_commands:
        - rm -rf /
      allow_network: false
    web:
      allow_network: true
      timeout: 15
      max_body_size: 524288
```

### Типы провайдеров

| Тип | Описание | Обязательные поля |
|-----|----------|-------------------|
| `openai` | OpenAI API | `api_key`, `model` |
| `anthropic` | Anthropic API (Claude) | `api_key`, `model` |
| `ollama` | Ollama (локальные модели) | `base_url`, `model` |
| `cavibora` | [Cavibora](https://github.com/Cavibora) API | `base_url`, `api_key`, `model` |
| `openai_compat` | Любой OpenAI-совместимый API | `base_url`, `model` |

### Поля провайдера

| Поле | Тип | Описание |
|------|-----|----------|
| `type` | string | Тип провайдера (обязательное) |
| `base_url` | string | URL API (опционально, дефолт по типу) |
| `api_key` | string | API ключ (опционально, поддерживает `${ENV_VAR}`) |
| `model` | string | Имя модели (обязательное) |
| `max_tokens` | int | Макс. output tokens для API-запроса (0 = дефолт провайдера: 4096) |
| `context_window` | int | Размер контекстного окна модели для компакции (0 = `agent.max_tokens`) |
| `budget_tokens` | int | Budget tokens для thinking (только Anthropic, 0 = дефолт 4096) |
| `temperature` | float | Температура сэмплирования (0.0-2.0, 0 = дефолт провайдера) |
| `top_p` | float | Top-p сэмплирование (0.0-1.0, 0 = дефолт провайдера) |
| `top_k` | int | Top-k сэмплирование (0 = дефолт провайдера, только Anthropic/Ollama) |
| `security.allow_network` | bool | Разрешить сетевые команды (true побеждает глобальный false) |
| `security.blocked_commands` | []string | Заблокированные bash-команды (заменяет глобальный список) |

**Приоритет настроек:**
- `context_window` провайдера > `agent.max_tokens` > дефолт 8000
- `security` провайдера > глобальный `security` (для `allow_network`: true побеждает false)
- `blocked_commands` провайдера полностью заменяет глобальный список

### Настройки агента

| Параметр | Тип | Описание | Дефолт |
|----------|-----|----------|--------|
| `agent.max_tokens` | int | Макс. токенов в контексте (fallback для context_window) | 8000 |
| `agent.keep_recent` | int | Сколько последних сообщений сохранять при компакции | 20 |
| `agent.verbose` | bool | Подробный вывод (логирование итераций) | false |
| `agent.permission_mode` | string | Режим разрешений: `auto-approve`, `ask`, `deny` | auto-approve |
| `agent.language` | string | Язык интерфейса: `en`, `ru`, `es`, `fr`, `de`, `ja`, `zh`, `pt` | en |
| `agent.request_timeout` | int | Макс. время одного LLM-запроса (секунды). При превышении — ошибка и прерывание | 1200 (20 мин) |
| `agent.thinking_timeout` | int | Макс. время без токенов от модели (секунды). При превышении — предупреждение, продолжает ждать | 600 (10 мин) |
| `agent.idle_timeout` | int | Таймаут стриминга без событий (секунды). При превышении — ошибка и прерывание | 120 (2 мин) |

**Таймауты LLM-запросов:**

- `request_timeout` — жёсткий лимит на всю итерацию. Если модель обрабатывает запрос дольше этого времени — прерываем с ошибкой. Полезно для защиты от зависаний модели.
- `thinking_timeout` — мягкий лимит на время без токенов. Если модель молчит дольше этого времени — показываем предупреждение "⚠️ Модель думает уже X мин" и продолжаем ждать. Предупреждение повторяется каждые 30 сек.
- `idle_timeout` — таймаут стриминга. Если от провайдера не приходит ни одного события (даже thinking) — прерываем с ошибкой. Обычно означает проблему с подключением.

**Секретные файлы и permission_mode:**

Доступ к секретным файлам (`.env`, `credentials.json`, `id_rsa`, `.pem`, `.key` и т.д.) зависит от `permission_mode`:
- `auto-approve` — секретные файлы доступны для чтения/записи (с предупреждением)
- `ask` — секретные файлы доступны (PermissionChecker может запросить подтверждение)
- `deny` — секретные файлы заблокированы

### Переменные окружения

В `api_key` можно использовать `${ENV_VAR}` для подстановки переменных окружения:

```yaml
api_key: ${ANTHROPIC_API_KEY}
```

---

## Интерактивный режим

### Режимы интерфейса

BugBuster Code поддерживает два режима интерфейса: **CLI** (по умолчанию) и **TUI** (богатый интерфейс).

#### CLI-режим (по умолчанию)

Запускается без флагов. Использует readline для ввода, ANSI-форматирование для вывода.

```bash
bugbuster                  # Интерактивный CLI
bugbuster "Исправь баг"    # Одноразовый запрос
```

#### TUI-режим

Полноценный терминальный UI на bubbletea с textarea, viewport, спиннерами и прогресс-барами.

```bash
bugbuster --tui            # TUI с альтернативным экраном (auto)
bugbuster -t               # То же
bugbuster --tui=auto       # То же (явно)
bugbuster --tui=inline     # TUI без альт. экрана — история остаётся в терминале
```

| Режим | AltScreen | История в терминале | Описание |
|-------|-----------|---------------------|----------|
| `auto` | Да | Печатается при выходе | Полноценный TUI, при выходе — чат в scrollback |
| `inline` | Нет | Да (реальное время) | TUI рисуется в основном буфере, diff-рендеринг |

**Рекомендация:** `auto` — для комфортной работы, `inline` — если нужна история в терминале.

#### Горячие клавиши TUI (настраиваемые в `bugbuster.yaml`)

| Клавиша | Действие |
|---------|----------|
| `Enter` | Отправить запрос |
| `Shift+Enter` / `Ctrl+J` | Перенос строки |
| `Ctrl+C` | Отмена стриминга / выход |
| `Esc` | Прервать стриминг / выход |
| `↑` / `↓` | Навигация по истории |
| `PgUp` / `PgDown` | Скролл viewport |

### Команды (CLI и TUI)

| Команда | Описание |
|---------|----------|
| `/help` | Показать справку |
| `/exit`, `/quit` | Выйти (сохраняет сессию) |
| `/reset` | Сбросить контекст разговора |
| `/context` | Показать информацию о контексте |
| `/tools` | Показать список инструментов |
| `/model <name>` | Переключить модель |
| `/provider <name>` | Переключить провайдера |
| `/sessions` | Показать сохранённые сессии |
| `/undo` | Отменить последнее изменение файла |
| `/undoall` | Отменить все изменения файлов |
| `/diff` | Показать список изменений файлов |
| `/lang <code>` | Сменить язык интерфейса (en, ru, es, ...) |

### Навигация по истории (CLI)

- **↑/↓** — навигация по предыдущим командам
- **Ctrl+C** — прервать текущий запрос (второе нажатие — выход)
- **Ctrl+D** — выход из программы

### UI-элементы

#### Спиннер

При ожидании ответа отображается анимированный спиннер:
```
  ⠹ Думаю...
  ⠹ Шаг 2/10...
```

#### Thinking-блоки

Для моделей с поддержкой extended thinking (Claude, o1/o3):
```
  ∴ Thinking…
  <текст размышлений в dim-стиле>
```

#### Tool calls

Вызовы инструментов отображаются с параметрами и статусом:
```
  ⏺ read(/path/to/file.go)
  ⎿ ✓ read 5 lines 120ms

  ⏺ bash(command="go test")
  ⎿ ✗ bash file not found 2.1s
```

#### Markdown-рендеринг

Ответы модели рендерятся в терминале:
- **Кодовые блоки** — подсвечиваются cyan с рамкой `┌`/`│`
- **Заголовки** (#, ##, ###) — bold + cyan
- **Списки** (- и *) — заменяются на `•`
- **Жирный текст** (**bold**) — ANSI bold

#### Статус-строка

После каждого ответа отображается строка статуса:
```
  ⬆ 1132 ⬇ 216 Σ 1348 │ ⏱ 9.3s │ ████████░░░░ 1200/8000 │ zai · glm-5.1
```

- **⬆/⬇/Σ** — входящие/исходящие/общие токены
- **⏱** — время генерации
- **Полоска контекста** — использование контекста (зелёный < 50%, жёлтый < 80%, красный ≥ 80%)
- **Провайдер · модель** — текущий провайдер и модель

#### Тема оформления

Все цвета и параметры markdown-рендеринга настраиваются через секцию `theme` в конфигурации:

```yaml
theme:
  mode: dark              # dark | light
  word_wrap: 80           # перенос слов в markdown (0 = без переноса)
  colors:
    primary: cyan          # спиннер, заголовки tool calls
    success: green         # ✓, create, добавления в diff
    error: red             # ✗, ошибки, удаления в diff
    warning: yellow        # предупреждения, modify в diff
    info: blue             # токены входящие
    dim: "244"             # приглушённый текст
    thinking: "244"        # thinking-блок
    tool_params: cyan      # параметры tool call
    tool_summary: "244"    # сводка результата tool call
    status_time: "244"     # ⏱ время в статусе
    status_separator: "244"  # │ разделитель в статусе
    context_bar_good: green   # контекст < 50%
    context_bar_warn: yellow  # контекст 50-80%
    context_bar_bad: red      # контекст > 80%
    user_message: "#04B575"  # ❯ пользовательский ввод (TUI)
    assistant: "#7D56F4"     # спиннер/статус ассистента (TUI)
    separator: "#3C3C3C"     # ─── разделитель
```

**Формат цветов** — три варианта:
- **ANSI имя**: `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white`, `black`
- **256-color номер**: `"244"` (строка с числом 0-255)
- **Hex**: `"#04B575"` (6-значный hex-код)

Непустые поля в `colors` перекрывают дефолтные значения. Можно указать только те цвета, которые хотите изменить.

---

## Инструменты

### read — Чтение файлов и директорий

```
Параметры: path (обязательный) — путь к файлу или директории
```

Читает содержимое файла или выводит список директории. Поддерживает ограничения по размеру (`max_file_size`) и безопасности (проверка path traversal, секретных файлов).

### write — Запись файлов

```
Параметры: path (обязательный), content (обязательный)
```

Создаёт или перезаписывает файл. Автоматически создаёт родительские директории.

### edit — Поиск и замена в файле

```
Параметры: path (обязательный), old (обязательный), new (обязательный)
```

Находит текст `old` в файле и заменяет на `new`. Точное совпадение — не регулярное выражение.

### bash — Выполнение shell-команд

```
Параметры: command (обязательный), timeout (опциональный), workdir (опциональный)
```

Выполняет команду в shell. Поддерживает таймаут и рабочую директорию. Сетевые команды блокируются если `allow_network: false`.

### grep — Поиск по регулярному выражению

```
Параметры: pattern (обязательный), path, glob, ignore_case
```

Рекурсивный поиск по файлам. Поддерживает регулярные выражения и фильтрацию по glob.

### glob — Поиск файлов по маске

```
Параметры: pattern (обязательный), path
```

Поиск файлов по шаблону (например `*.go`, `**/*.go`).

### ask — Запрос к внешнему LLM

```
Параметры: prompt (обязательный)
```

Отправляет запрос к другому LLM через текущего провайдера. Используется для получения второго мнения.

### ask_user — Запрос информации у пользователя

```
Параметры: question (обязательный)
```

Запрашивает ввод у пользователя в интерактивном режиме.

### learn — Обучение модели

```
Параметры: input (обязательный), output (обязательный), type (опциональный: text/code)
```

Отправляет обучающие данные в [Cavibora](https://github.com/Cavibora) для дообучения.

### web_fetch — Загрузка URL

```
Параметры: url (обязательный), method (опциональный: GET/HEAD/POST), headers
```

Загружает содержимое URL. Поддерживает ограничение по размеру ответа.

---

## Архитектура

### Поток данных

```
Пользователь → readline → AgentLoop.Stream() → Provider.Stream()
                                                    ↓
                                              StreamEvent канал
                                                    ↓
                                            runQueryWithLoop()
                                                    ↓
                                          MarkdownRenderer → терминал
```

### Провайдеры

Каждый провайдер реализует интерфейс `Provider`:

```go
type Provider interface {
    Name() string
    Complete(messages []Message, tools []ToolDef) (*CompletionResult, error)
    CompleteWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error)
    Stream(messages []Message, tools []ToolDef) (<-chan StreamEvent, error)
    StreamWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error)
}
```

### StreamEvent — события потокового вывода

| Тип | Описание |
|-----|----------|
| `text_delta` | Фрагмент текста ответа |
| `thinking` | Фрагмент размышлений модели (thinking/reasoning) |
| `tool_call_start` | Начало вызова инструмента |
| `tool_call_delta` | Фрагмент JSON-параметров инструмента |
| `tool_progress` | Прогресс выполнения инструмента |
| `tool_call_end` | Завершение вызова инструмента (с результатом) |
| `user_injected` | Пользовательский комментарий добавлен в контекст |
| `compaction` | Компакция контекста (суммаризация) |
| `iteration_start` | Начало итерации агентного цикла |
| `iteration_end` | Завершение итерации |
| `usage` | Данные по токенам |
| `done` | Завершение ответа |
| `error` | Ошибка |

### Контекст и компакция

- `ConversationContext` хранит историю сообщений
- При превышении `max_tokens` или `max_context` происходит компакция:
  - Сохраняются последние `keep_recent` сообщений
  - Остальные суммаризуются через LLM
- Thinking-блоки **не удаляются** при компакции (сохраняются в контексте)
- Во время компакции в UI отображается `🔄 Компакция контекста…` со спиннером

### Безопасность

- **Path traversal** — блокировка путей с `..`
- **Секретные файлы** — блокировка доступа к `.env`, `credentials.json` и т.д.
- **Системные пути** — блокировка записи в `/etc`, `/usr`, `/System`
- **Песочница** — ограничение записи файлов в `sandbox_dir`
- **Сетевые команды** — блокировка `curl`, `wget` при `allow_network: false`
- **Заблокированные команды** — настраиваемый список запрещённых bash-команд

### i18n — Интернационализация

Поддерживаемые языки: `en`, `ru`, `es`, `fr`, `de`, `ja`, `zh`, `pt`

Файлы локалей: `pkg/i18n/locales/{lang}.json`

Ключи: `cli.*`, `cli_error.*`, `cli_success.*`, `cli_help.*`, `cli_flag.*`, `cli_banner.*`, `tools.*`, `agent.*`, `system_prompt.*`, `security.*`, `errors.*`

---

## MCP (Model Context Protocol)

BugBuster Code поддерживает MCP-серверы для расширения возможностей агента. Поддерживаются три типа транспорта:

| Тип | Описание |
|-----|----------|
| `stdio` | Локальный процесс (stdin/stdout) |
| `sse` | Server-Sent Events (HTTP) |
| `streamable-http` | Streamable HTTP |

### Настройка

MCP-серверы настраиваются в секции `mcp` конфигурации:

```yaml
mcp:
  servers:
    my-server:
      type: sse
      url: https://api.example.com/mcp/sse
      headers:                    # HTTP-заголовки (для авторизации)
        Authorization: "Bearer ${MCP_TOKEN}"
      enabled: true
```

### Автоимпорт

BugBuster автоматически сканирует проект на наличие MCP-конфигураций (`.mcp.json`, `claude_desktop_config.json`). Результаты доступны через команду `/mcp`.

### Импорт из Claude Code

Поддерживается импорт конфигурации MCP из `.claude/settings.json` (формата Claude Code).

---

## Сессии

Сессии сохраняются в `.bugbuster/sessions/`. Каждая сессия — JSONL-файл с историей сообщений.

- **Создание** — автоматически при запуске
- **Восстановление** — `bugbuster --session <ID>` или `bugbuster -s <ID>`
- **Сохранение** — автоматически при `/exit`, `/quit`, Ctrl+D
- **Просмотр** — команда `/sessions`
- **История чата** — при восстановлении сессии вся история сообщений отображается на экране

### Восстановление сессии

```bash
# По ID (из /sessions или сообщения при выходе)
bugbuster --session sess_20250101_120000_abcd1234

# CLI-режим: при запуске предлагается восстановить последнюю сессию
bugbuster

# TUI-режим: при --session история загружается в viewport
bugbuster --tui --session sess_20250101_120000_abcd1234
```

При выходе из программы выводится подсказка для восстановления:
```
  Session saved: sess_20250101_120000_abcd1234
  Restore: bugbuster --session sess_20250101_120000_abcd1234
```

---

## Файлы и директории

```
.bugbuster/
├── config.yaml          # Конфигурация (альтернативное расположение)
├── history              # История команд readline (CLI)
├── changes.json         # Трекер изменений (для /undo)
└── sessions/
    └── <session-id>.jsonl  # Файлы сессий (JSONL формат)
```

---

## Плагины

BugBuster поддерживает три типа плагинов:

### 1. Встроенные плагины (builtins)

Go-плагины, скомпилированные вместе с BugBuster. Подключаются через секцию `plugins.builtins`:

```yaml
plugins:
  builtins:
    - filesystem
    - bash
    - web
  config:
    filesystem:
      allowed_dirs: ["/tmp", "/home"]
    bash:
      timeout: 30
      blocked_commands: ["rm -rf /"]
    web:
      allow_network: true
```

Доступные встроенные плагины:
- **filesystem** — read, write, edit, grep, glob
- **bash** — выполнение shell-команд
- **web** — web_fetch

### 2. Внешние Go-плагины (.so)

Скомпилированные Go-плагины, загружаемые как shared libraries. Подключаются через секцию `plugins.go`:

```yaml
plugins:
  go:
    - name: my-plugin
      path: /path/to/myplugin.so
      config:
        key: value
```

Для создания Go-плагина нужно реализовать интерфейс `plugin.Plugin` и экспортировать символ `Plugin`:

```go
package main

import "bugbuster-code/pkg/plugin"

type MyPlugin struct {
    plugin.BasePlugin
}

func (p *MyPlugin) Init(config map[string]any) error {
    // Инициализация
    return nil
}

func (p *MyPlugin) Tools() []tools.Tool {
    return []tools.Tool{/* ваши инструменты */}
}

var Plugin plugin.Plugin = &MyPlugin{
    BasePlugin: plugin.BasePlugin{
        PluginName:        "my-plugin",
        PluginDescription: "My custom plugin",
        PluginVersion:     "1.0.0",
    },
}
```

Компиляция: `go build -buildmode=plugin -o myplugin.so`

### 3. MCP-плагины (всеядные)

MCP (Model Context Protocol) серверы — внешние процессы, общающиеся через stdio/SSE/HTTP. Подключаются через секцию `plugins.mcp` или `mcp.servers`:

```yaml
plugins:
  mcp:
    github:
      type: stdio
      command: npx
      args: ["-y", "@modelcontextprotocol/server-github"]
      env:
        GITHUB_TOKEN: ${GITHUB_TOKEN}
      enabled: true

    postgres:
      type: stdio
      command: uvx
      args: ["mcp-server-postgres", "postgresql://localhost/mydb"]
      enabled: true
```

### Команды управления плагинами

| Команда | Описание |
|---------|----------|
| `/plugin` | Показать список плагинов (встроенные, Go, MCP) |
| `/plugin install <name>` | Установить MCP-плагин из реестра |
| `/plugin remove <name>` | Удалить плагин из конфигурации |

Доступные плагины в реестре: `github`, `filesystem`, `postgres`, `brave-search`, `sqlite`, `memory`, `puppeteer`, `sequential-thinking`, `fetch`, `everything`

### Автоимпорт

BugBuster автоматически сканирует проект на наличие MCP-конфигураций (`.mcp.json`, `.cline/mcp.json`, VS Code settings). Результаты доступны через команду `/mcp`.

### Приоритет инструментов

Встроенные инструменты (созданные вручную в `agent_setup.go`) имеют приоритет над плагинами. Если инструмент с таким именем уже зарегистрирован, плагин пропускается с логированием.