package config

import (
	"testing"
)

func TestGetProviderForTask(t *testing.T) {
	cfg := DefaultConfig()

	// No AgentProviders configured — should return default
	result := cfg.GetProviderForTask("thinking")
	if result != "ollama" {
		t.Errorf("Expected default provider 'ollama', got '%s'", result)
	}

	// Configure AgentProviders
	cfg.AgentProviders = map[string]string{
		"thinking": "openai",
		"fast":     "ollama",
	}

	// Known task type
	result = cfg.GetProviderForTask("thinking")
	if result != "openai" {
		t.Errorf("Expected 'openai' for thinking, got '%s'", result)
	}

	// Another known task type
	result = cfg.GetProviderForTask("fast")
	if result != "ollama" {
		t.Errorf("Expected 'ollama' for fast, got '%s'", result)
	}

	// Unknown task type — should return default
	result = cfg.GetProviderForTask("unknown")
	if result != "ollama" {
		t.Errorf("Expected default 'ollama' for unknown task, got '%s'", result)
	}
}

func TestGetTaskTypeForProvider(t *testing.T) {
	cfg := DefaultConfig()

	// No AgentProviders configured
	result := cfg.GetTaskTypeForProvider("openai")
	if result != "" {
		t.Errorf("Expected empty string for nil AgentProviders, got '%s'", result)
	}

	// Configure AgentProviders
	cfg.AgentProviders = map[string]string{
		"thinking": "openai",
		"fast":     "ollama",
	}

	// Known provider
	result = cfg.GetTaskTypeForProvider("openai")
	if result != "thinking" {
		t.Errorf("Expected 'thinking' for openai, got '%s'", result)
	}

	// Another known provider
	result = cfg.GetTaskTypeForProvider("ollama")
	if result != "fast" {
		t.Errorf("Expected 'fast' for ollama, got '%s'", result)
	}

	// Unknown provider
	result = cfg.GetTaskTypeForProvider("unknown")
	if result != "" {
		t.Errorf("Expected empty string for unknown provider, got '%s'", result)
	}
}

func TestTaskTypes(t *testing.T) {
	cfg := DefaultConfig()

	// No AgentProviders configured
	types := cfg.TaskTypes()
	if types != nil {
		t.Errorf("Expected nil for nil AgentProviders, got %v", types)
	}

	// Configure AgentProviders
	cfg.AgentProviders = map[string]string{
		"thinking": "openai",
		"fast":     "ollama",
	}

	types = cfg.TaskTypes()
	if len(types) != 2 {
		t.Errorf("Expected 2 task types, got %d", len(types))
	}

	// Check both types are present
	found := map[string]bool{}
	for _, tt := range types {
		found[tt] = true
	}
	if !found["thinking"] || !found["fast"] {
		t.Errorf("Expected 'thinking' and 'fast' in task types, got %v", types)
	}
}

func TestAutoContinueDefault(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Agent.AutoContinue {
		t.Error("Expected AutoContinue=true by default")
	}
}

func TestSubagentYAMLConfigDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Agent.Subagent.MaxConcurrent != 3 {
		t.Errorf("Expected Subagent.MaxConcurrent=3, got %d", cfg.Agent.Subagent.MaxConcurrent)
	}
	if cfg.Agent.Subagent.MaxIterations != 15 {
		t.Errorf("Expected Subagent.MaxIterations=15, got %d", cfg.Agent.Subagent.MaxIterations)
	}
	if cfg.Agent.Subagent.Timeout != 600 {
		t.Errorf("Expected Subagent.Timeout=600, got %d", cfg.Agent.Subagent.Timeout)
	}
}

func TestMergeConfigs_AgentProviders(t *testing.T) {
	base := DefaultConfig()
	overlay := &BugBusterConfig{
		AgentProviders: map[string]string{
			"thinking": "openai",
		},
	}

	result := MergeConfigs(base, overlay)
	if result.AgentProviders == nil {
		t.Error("Expected AgentProviders to be set after merge")
	}
	if result.AgentProviders["thinking"] != "openai" {
		t.Errorf("Expected AgentProviders['thinking']='openai', got '%s'", result.AgentProviders["thinking"])
	}
}

func TestMergeConfigs_SubagentConfig(t *testing.T) {
	base := DefaultConfig()
	overlay := &BugBusterConfig{
		Agent: AgentConfig{
			Subagent: SubagentYAMLConfig{
				Provider:      "openai",
				Model:         "gpt-4o",
				MaxConcurrent: 5,
				MaxIterations: 20,
				Timeout:       300,
			},
		},
	}

	result := MergeConfigs(base, overlay)
	if result.Agent.Subagent.Provider != "openai" {
		t.Errorf("Expected Subagent.Provider='openai', got '%s'", result.Agent.Subagent.Provider)
	}
	if result.Agent.Subagent.Model != "gpt-4o" {
		t.Errorf("Expected Subagent.Model='gpt-4o', got '%s'", result.Agent.Subagent.Model)
	}
	if result.Agent.Subagent.MaxConcurrent != 5 {
		t.Errorf("Expected Subagent.MaxConcurrent=5, got %d", result.Agent.Subagent.MaxConcurrent)
	}
	if result.Agent.Subagent.MaxIterations != 20 {
		t.Errorf("Expected Subagent.MaxIterations=20, got %d", result.Agent.Subagent.MaxIterations)
	}
	if result.Agent.Subagent.Timeout != 300 {
		t.Errorf("Expected Subagent.Timeout=300, got %d", result.Agent.Subagent.Timeout)
	}
}