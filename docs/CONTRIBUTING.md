# Contributing to BugBuster Code

Thank you for your interest in contributing! This guide covers everything you need to get started.

## Prerequisites

- **Go 1.25+** (check with `go version`)
- **Git**
- A terminal (macOS, Linux, or WSL on Windows)

## Quick Start

```bash
# Clone the repository
git clone https://github.com/Cavibora/bugbuster-code.git
cd bugbuster-code

# Build
go build -o bugbuster ./cmd/bugbuster/

# Run tests
go test ./...

# Run tests with race detector
go test -race ./...

# Run
./bugbuster
```

## Project Structure

```
bugbuster-code/
├── cmd/bugbuster/       # Application entry point and UI
├── pkg/
│   ├── agent/           # Core agent logic (AgentLoop, context, compaction)
│   ├── config/          # Configuration loading and validation
│   ├── i18n/            # Internationalization (8 languages)
│   ├── logger/          # Structured logging
│   ├── mcp/             # Model Context Protocol (client + server)
│   ├── plugin/          # Plugin system
│   ├── provider/        # LLM provider abstraction
│   ├── theme/           # Terminal theming
│   └── tools/           # Built-in tools (14 tools)
├── docs/                # Documentation
└── ROADMAP.md           # Development roadmap (internal)
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed architecture overview.

## Development Workflow

### 1. Create a Branch

```bash
git checkout -b feature/my-feature
# or
git checkout -b fix/my-fix
```

### 2. Make Changes

Follow the coding conventions:
- **Go standard formatting** — `gofmt` / `go fmt`
- **Error handling** — always check errors, wrap with context
- **Comments** — doc comments on all exported types and functions
- **Tests** — write tests for new functionality

### 3. Run Tests

```bash
# All tests
go test ./...

# With race detector
go test -race ./...

# Specific package
go test ./pkg/agent/...

# Specific test
go test -run TestAgentLoop ./pkg/agent/

# With verbose output
go test -v -run TestAgentLoop ./pkg/agent/
```

### 4. Build and Test Manually

```bash
go build -o bugbuster ./cmd/bugbuster/

# Test with Ollama (no API key needed)
./bugbuster -c test-config.yaml

# Test specific mode
./bugbuster --tui
./bugbuster "explain main.go"
```

### 5. Commit and Push

```bash
git add .
git commit -m "feat: add new tool for X"
git push origin feature/my-feature
```

## Coding Conventions

### Go Style

- Follow [Effective Go](https://go.dev/doc/effective_go)
- Use `gofmt` for formatting
- Run `go vet` before committing

### Error Handling

```go
// Good: wrap errors with context
result, err := doSomething()
if err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}

// Bad: ignore errors
result, _ := doSomething()
```

### Tool Implementation

Tools implement the `Tool` interface:

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]any  // JSON Schema
    Execute(params map[string]string) ToolResult
}
```

For async tools (long-running operations):

```go
type AsyncTool interface {
    Tool
    ExecuteAsync(params map[string]string) <-chan AsyncEvent
}
```

Example tool:

```go
package tools

type MyTool struct{}

func (t *MyTool) Name() string { return "my_tool" }

func (t *MyTool) Description() string {
    return "Does something useful"
}

func (t *MyTool) Parameters() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "input": map[string]any{
                "type":        "string",
                "description": "Input parameter",
            },
        },
        "required": []string{"input"},
    }
}

func (t *MyTool) Execute(params map[string]string) ToolResult {
    input := params["input"]
    if input == "" {
        return ToolResult{Error: "input is required"}
    }
    // Do work...
    return ToolResult{Output: "result"}
}
```

### Provider Implementation

Providers implement the `Provider` interface:

```go
type Provider interface {
    Name() string
    Complete(ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error)
    Stream(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error)
}
```

### Internationalization

All user-facing strings must use i18n:

```go
// Good
fmt.Println(i18n.T("cli.session_saved", sessionID))

// Bad
fmt.Println("Session saved: " + sessionID)
```

Add translations to all 8 locale files in `pkg/i18n/locales/`:
- `en.json`, `ru.json`, `es.json`, `fr.json`, `de.json`, `ja.json`, `zh.json`, `pt.json`

### Testing

```go
func TestMyFeature(t *testing.T) {
    // Use table-driven tests
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"basic", "hello", "HELLO", false},
        {"empty", "", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFunction(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("MyFunction() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("MyFunction() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add new tool for database queries
fix: resolve crash in TUI mode on resize
refactor: simplify provider interface
test: add tests for context compaction
docs: update configuration reference
i18n: add Portuguese translations
```

## Pull Request Process

1. **Update tests** — all new code should have tests
2. **Update docs** — if you change behavior, update relevant `.md` files
3. **Run full test suite** — `go test -race ./...`
4. **Keep PRs focused** — one feature/fix per PR
5. **Add description** — explain what and why

## Adding a New Tool

1. Create `pkg/tools/my_tool.go`
2. Implement `Tool` interface (or `AsyncTool`)
3. Register in `cmd/bugbuster/agent_setup.go`
4. Add i18n strings to all 8 locale files
5. Write tests in `pkg/tools/my_tool_test.go`
6. Update [TOOLS.md](TOOLS.md)

## Adding a New Provider

1. Create `pkg/provider/my_provider.go`
2. Implement `Provider` interface
3. Add to factory in `pkg/provider/factory.go`
4. Add config type in `pkg/config/config.go`
5. Write tests in `pkg/provider/my_provider_test.go`
6. Update [CONFIGURATION.md](CONFIGURATION.md)

## Adding a New Language

1. Copy `pkg/i18n/locales/en.json` to `pkg/i18n/locales/xx.json`
2. Translate all strings
3. Add language code to `pkg/i18n/i18n.go` supported languages
4. Update `pkg/config/config.go` language validation
5. Update [CONFIGURATION.md](CONFIGURATION.md)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
