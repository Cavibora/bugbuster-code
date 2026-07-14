package provider

import (
	"context"
	"time"
)

// UserAgent — client identifier for HTTP requests
const UserAgent = "BugBuster-Code/1.0"

// Event types StreamEvent
const (
	EventTextDelta       = "text_delta"
	EventThinking        = "thinking" // model thinking/reasoning block
	EventToolCallStart   = "tool_call_start"
	EventToolCallDelta   = "tool_call_delta"
	EventToolCallEnd     = "tool_call_end"
	EventToolProgress    = "tool_progress"   // tool execution progress
	EventIterationStart  = "iteration_start" // agent loop iteration start
	EventIterationEnd    = "iteration_end"   // iteration end
	EventUsage           = "usage"           // token usage data
	EventDone            = "done"
	EventError           = "error"
	EventUserInjected    = "user_injected"    // user comment during agent execution
	EventCompaction      = "compaction"       // context compaction started
	EventCompactionDone  = "compaction_done"  // context compaction completed
	EventThinkingTimeout = "thinking_timeout" // model thinking too long without tokens
)

// StreamEvent — streaming event from provider
type StreamEvent struct {
	Type       string         // event type (EventTextDelta, EventToolCallStart, etc.)
	Text       string         // for text_delta
	ToolCallID string         // for tool_call_**
	ToolName   string         // for tool_call_*start / tool_call_end
	ToolInput  map[string]any // for tool_call_*end (full input)
	ToolDelta  string         // for tool_call_delta (incremental JSON)
	Error      error          // for error
	StopReason string         // stop reason (end_turn, max_tokens, tool_use)
	// Extended fields for rich UI
	Iteration      int           // iteration number (1-based)
	InputTokens    int           // request tokens (from Usage)
	OutputTokens   int           // response tokens (from Usage)
	Duration       time.Duration // execution time
	ToolResult     string        // short tool result (for header)
	ToolFullResult string        // full tool result (for content display)
	ToolOK         bool          // tool success/error
	ToolProgress   float64       // tool execution progress (0.0-1.0)
	ToolMessage    string        // message about tool progress
}

// Provider — interface LLM-provider
type Provider interface {
	// Name returns provider name (openai, anthropic, ollama, etc.)
	Name() string

	// Model returns the model name
	Model() string

	// Complete sends request and receives full response
	Complete(messages []Message, tools []ToolDef) (*CompletionResult, error)

	// CompleteWithCtx sends request with context (for cancellation/timeout)
	CompleteWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (*CompletionResult, error)

	// Stream sends request and receives streaming response
	Stream(messages []Message, tools []ToolDef) (<-chan StreamEvent, error)

	// StreamWithCtx sends streaming request with context
	StreamWithCtx(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error)
}

// ToolDef is a tool definition for function calling
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// CompletionResult is request completion result
type CompletionResult struct {
	Message    Message
	StopReason string // "end_turn", "tool_use", "max_tokens"
	Usage      Usage
}

// Usage — token usage statistics
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// ProviderConfig — provider configuration (from YAML)
type ProviderConfig struct {
	Type          string           `yaml:"type"`           // "openai", "anthropic", "ollama", "cavibora", "openai_compat"
	BaseURL       string           `yaml:"base_url"`       // API URL (optional, default by type)
	APIKey        string           `yaml:"api_key"`        // API key (optional)
	Model         string           `yaml:"model"`          // model name
	MaxTokens     int              `yaml:"max_tokens"`     // max_tokens for API request (output tokens), 0 = provider default
	ContextWindow int              `yaml:"context_window"` // model context window size (for compaction), 0 = agent.max_tokens
	BudgetTokens  int              `yaml:"budget_tokens"`  // budget_tokens for thinking (Anthropic), 0 = default 4096
	Temperature   float64          `yaml:"temperature"`    // sampling temperature (0.0-2.0), 0 = provider default
	TopP          float64          `yaml:"top_p"`          // top-p sampling (0.0-1.0), 0 = provider default
	TopK          int              `yaml:"top_k"`          // top-k sampling, 0 = provider default
	Security      ProviderSecurity `yaml:"security"`       // settings security provider
}

// ProviderSecurity — settings security provider
type ProviderSecurity struct {
	AllowNetwork    bool     `yaml:"allow_network"`    // allow network requests
	BlockedCommands []string `yaml:"blocked_commands"` // blocked commands
}

// DefaultBaseURL returns default URL for provider type
func (c ProviderConfig) DefaultBaseURL() string {
	switch c.Type {
	case "openai":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com"
	case "ollama":
		return "http://localhost:11434"
	case "cavibora":
		return "https://api.cavibora.com"
	default:
		return ""
	}
}

// GetBaseURL returns provider URL (from config or default)
func (c ProviderConfig) GetBaseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return c.DefaultBaseURL()
}
