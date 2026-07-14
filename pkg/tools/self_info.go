package tools

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"bugbuster-code/pkg/provider"
)

// SelfInfoContext provides access to agent internals for self-identification.
type SelfInfoContext interface {
	GetProvider() provider.Provider
	TokenCount() int
	MaxTokensValue() int
	GetSystemPrompt() string
	MessageCount() int
}

// SelfInfoWrapper wraps AgentLoop fields to satisfy SelfInfoContext
type SelfInfoWrapper struct {
	Provider provider.Provider
	Ctx      *SelfInfoCtxAdapter
}

// SelfInfoCtxAdapter wraps ConversationContext-like interface
type SelfInfoCtxAdapter struct {
	TokenCountFn     func() int
	MaxTokensValueFn func() int
	GetSystemPromptFn func() string
	MessageCountFn   func() int
}

func (a *SelfInfoCtxAdapter) TokenCount() int      { return a.TokenCountFn() }
func (a *SelfInfoCtxAdapter) MaxTokensValue() int   { return a.MaxTokensValueFn() }
func (a *SelfInfoCtxAdapter) GetSystemPrompt() string { return a.GetSystemPromptFn() }
func (a *SelfInfoCtxAdapter) MessageCount() int     { return a.MessageCountFn() }

func (w *SelfInfoWrapper) GetProvider() provider.Provider { return w.Provider }
func (w *SelfInfoWrapper) TokenCount() int                { return w.Ctx.TokenCountFn() }
func (w *SelfInfoWrapper) MaxTokensValue() int            { return w.Ctx.MaxTokensValueFn() }
func (w *SelfInfoWrapper) GetSystemPrompt() string        { return w.Ctx.GetSystemPromptFn() }
func (w *SelfInfoWrapper) MessageCount() int              { return w.Ctx.MessageCountFn() }

// SelfInfoTool allows the model to get information about itself,
// the agent, the system, and the environment.
type SelfInfoTool struct {
	context SelfInfoContext
}

// NewSelfInfoTool creates a new self_info tool
func NewSelfInfoTool(ctx SelfInfoContext) *SelfInfoTool {
	return &SelfInfoTool{context: ctx}
}

func (t *SelfInfoTool) Name() string { return "self_info" }

func (t *SelfInfoTool) Description() string {
	return "Get information about yourself (model, provider), the agent (context, system prompt), and the system (hardware, OS, runtime). Use this to understand your environment and capabilities."
}

func (t *SelfInfoTool) Execute(params map[string]string) ToolResult {
	var sb strings.Builder

	// === Model & Provider ===
	sb.WriteString("=== Model & Provider ===\n")
	if t.context != nil {
		provider := t.context.GetProvider()
		if provider != nil {
			sb.WriteString(fmt.Sprintf("Provider: %s\n", provider.Name()))
			sb.WriteString(fmt.Sprintf("Model: %s\n", provider.Model()))
		}
		tokenCount := t.context.TokenCount()
		maxTokens := t.context.MaxTokensValue()
		pct := 0
		if maxTokens > 0 {
			pct = tokenCount * 100 / maxTokens
		}
		sb.WriteString(fmt.Sprintf("Context: %d/%d tokens (%d%%)\n", tokenCount, maxTokens, pct))
		msgCount := t.context.MessageCount()
		sb.WriteString(fmt.Sprintf("Messages in context: %d\n", msgCount))
	}

	// === System Prompt ===
	sb.WriteString("\n=== System Prompt ===\n")
	if t.context != nil {
		prompt := t.context.GetSystemPrompt()
		if len(prompt) > 2000 {
			prompt = prompt[:2000] + "\n... (truncated, full length: " + fmt.Sprintf("%d", len(prompt)) + " chars)"
		}
		sb.WriteString(prompt)
	}
	sb.WriteString("\n")

	// === System Info ===
	sb.WriteString("\n=== System Info ===\n")
	sb.WriteString(fmt.Sprintf("OS: %s %s\n", runtime.GOOS, runtime.GOARCH))
	hostname, _ := os.Hostname()
	sb.WriteString(fmt.Sprintf("Hostname: %s\n", hostname))
	sb.WriteString(fmt.Sprintf("CPU cores: %d\n", runtime.NumCPU()))
	sb.WriteString(fmt.Sprintf("Go version: %s\n", runtime.Version()))

	// Memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	sb.WriteString(fmt.Sprintf("Memory allocated: %d MB\n", m.Alloc/1024/1024))
	sb.WriteString(fmt.Sprintf("Memory total: %d MB\n", m.Sys/1024/1024))
	sb.WriteString(fmt.Sprintf("Goroutines: %d\n", runtime.NumGoroutine()))

	// Working directory
	cwd, _ := os.Getwd()
	sb.WriteString(fmt.Sprintf("Working directory: %s\n", cwd))

	// Current time
	now := time.Now()
	sb.WriteString(fmt.Sprintf("Current time: %s\n", now.Format("2006-01-02 15:04:05 MST")))
	sb.WriteString(fmt.Sprintf("Current year: %d\n", now.Year()))

	// Environment (safe subset)
	sb.WriteString("\n=== Environment ===\n")
	sb.WriteString(fmt.Sprintf("SHELL: %s\n", getEnv("SHELL")))
	sb.WriteString(fmt.Sprintf("HOME: %s\n", getEnv("HOME")))
	sb.WriteString(fmt.Sprintf("PATH: %s\n", getEnv("PATH")))
	sb.WriteString(fmt.Sprintf("USER: %s\n", getEnv("USER")))
	sb.WriteString(fmt.Sprintf("LANG: %s\n", getEnv("LANG")))
	sb.WriteString(fmt.Sprintf("TERM: %s\n", getEnv("TERM")))

	return Success(sb.String())
}

func getEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		return "(not set)"
	}
	return val
}

func (t *SelfInfoTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}
