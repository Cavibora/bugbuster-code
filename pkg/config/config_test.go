package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bugbuster-code/pkg/provider"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DefaultProvider != "ollama" {
		t.Errorf("Expected DefaultProvider='ollama', got '%s'", cfg.DefaultProvider)
	}
	if cfg.Agent.MaxTokens != 32768 {
		t.Errorf("Expected MaxTokens=32768, got %d", cfg.Agent.MaxTokens)
	}
	if cfg.Agent.KeepRecent != 20 {
		t.Errorf("Expected KeepRecent=20, got %d", cfg.Agent.KeepRecent)
	}
	if cfg.Tools.MaxFileSize != 1024*1024 {
		t.Errorf("Expected MaxFileSize=1048576, got %d", cfg.Tools.MaxFileSize)
	}
	if cfg.Tools.BashTimeout != 30 {
		t.Errorf("Expected BashTimeout=30, got %d", cfg.Tools.BashTimeout)
	}
	if _, ok := cfg.Providers["ollama"]; !ok {
		t.Error("Expected 'ollama' provider in default config")
	}
	if cfg.Agent.PermissionMode != "auto-approve" {
		t.Errorf("Expected PermissionMode='auto-approve', got '%s'", cfg.Agent.PermissionMode)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bugbuster.yaml")

	cfg := DefaultConfig()
	cfg.Agent.MaxTokens = 16000
	cfg.Providers["openai"] = provider.ProviderConfig{
		Type:   "openai",
		APIKey: "sk-test",
		Model:  "gpt-4o",
	}

	// Сохраняем
	if err := cfg.SaveConfig(configPath); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}

	// Загружаем
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	if loaded.Agent.MaxTokens != 16000 {
		t.Errorf("Expected MaxTokens=16000, got %d", loaded.Agent.MaxTokens)
	}
	if loaded.Providers["openai"].Model != "gpt-4o" {
		t.Errorf("Expected openai model='gpt-4o', got '%s'", loaded.Providers["openai"].Model)
	}
}

func TestLoadConfig_NotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent config file")
	}
}

func TestResolveEnvVars(t *testing.T) {
	os.Setenv("TEST_BUGBUSTER_KEY", "my-secret-key")
	defer os.Unsetenv("TEST_BUGBUSTER_KEY")

	result := resolveEnvVars("${TEST_BUGBUSTER_KEY}")
	if result != "my-secret-key" {
		t.Errorf("Expected 'my-secret-key', got '%s'", result)
	}

	// Без переменных
	result = resolveEnvVars("plain-text")
	if result != "plain-text" {
		t.Errorf("Expected 'plain-text', got '%s'", result)
	}

	// Несуществующая переменная
	result = resolveEnvVars("${NONEXISTENT_VAR}")
	if result != "" {
		t.Errorf("Expected empty string for nonexistent var, got '%s'", result)
	}
}

func TestMergeConfigs(t *testing.T) {
	base := DefaultConfig()

	override := &BugBusterConfig{
		DefaultProvider: "openai",
		Agent: AgentConfig{
			MaxTokens: 16000,
		},
		Providers: map[string]provider.ProviderConfig{
			"openai": {
				Type:   "openai",
				APIKey: "sk-test",
				Model:  "gpt-4o",
			},
		},
	}

	merged := MergeConfigs(base, override)

	if merged.DefaultProvider != "openai" {
		t.Errorf("Expected DefaultProvider='openai', got '%s'", merged.DefaultProvider)
	}
	if merged.Agent.MaxTokens != 16000 {
		t.Errorf("Expected MaxTokens=16000, got %d", merged.Agent.MaxTokens)
	}
	if merged.Providers["ollama"].Model != "qwen-fast-27b" {
		t.Error("Expected ollama provider to be preserved from base")
	}
	if merged.Providers["openai"].Model != "gpt-4o" {
		t.Error("Expected openai provider from override")
	}
}

func TestMergeConfigs_PermissionMode(t *testing.T) {
	base := DefaultConfig()

	override := &BugBusterConfig{
		Agent: AgentConfig{
			PermissionMode: "deny",
		},
	}

	merged := MergeConfigs(base, override)

	if merged.Agent.PermissionMode != "deny" {
		t.Errorf("Expected PermissionMode='deny', got '%s'", merged.Agent.PermissionMode)
	}

	// Empty override should keep default
	emptyOverride := &BugBusterConfig{
		Agent: AgentConfig{
			PermissionMode: "",
		},
	}
	merged2 := MergeConfigs(base, emptyOverride)
	if merged2.Agent.PermissionMode != "auto-approve" {
		t.Errorf("Expected PermissionMode='auto-approve' when empty override, got '%s'", merged2.Agent.PermissionMode)
	}
}

func TestProviderConfigDefaultBaseURL(t *testing.T) {
	tests := []struct {
		providerType string
		expected     string
	}{
		{"openai", "https://api.openai.com/v1"},
		{"anthropic", "https://api.anthropic.com"},
		{"ollama", "http://localhost:11434"},
		{"cavibora", "https://api.cavibora.com"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		cfg := provider.ProviderConfig{Type: tt.providerType}
		result := cfg.DefaultBaseURL()
		if result != tt.expected {
			t.Errorf("provider.ProviderConfig{Type:'%s'}.DefaultBaseURL() = '%s', want '%s'",
				tt.providerType, result, tt.expected)
		}
	}
}

func TestFindConfigFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a deep subdirectory to avoid finding config files in parent directories
	deepDir := filepath.Join(tmpDir, "a", "b", "c", "d", "e", "f", "g", "h", "i", "j")
	if err := os.MkdirAll(deepDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Override HOME to prevent finding ~/.bugbuster/config.yaml
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	origDir, _ := os.Getwd()
	if err := os.Chdir(deepDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	// No config file — should return error
	_, err := FindConfigFile()
	if err == nil {
		t.Error("Expected error when no config file found")
	}
}

func TestFindConfigFile_HiddenFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bugbuster.yaml")
	if err := os.WriteFile(configPath, []byte("default_provider: ollama"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	found, err := FindConfigFile()
	if err != nil {
		t.Errorf("Expected to find config, got error: %v", err)
	}
	// Normalize paths to handle macOS /var → /private/var symlink
	foundNorm, _ := filepath.EvalSymlinks(found)
	expectedNorm, _ := filepath.EvalSymlinks(configPath)
	if foundNorm != expectedNorm {
		t.Errorf("Expected path %s, got %s", expectedNorm, foundNorm)
	}
}

func TestFindConfigFile_VisibleFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "bugbuster.yaml")
	if err := os.WriteFile(configPath, []byte("default_provider: ollama"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	found, err := FindConfigFile()
	if err != nil {
		t.Errorf("Expected to find config, got error: %v", err)
	}
	foundNorm, _ := filepath.EvalSymlinks(found)
	expectedNorm, _ := filepath.EvalSymlinks(configPath)
	if foundNorm != expectedNorm {
		t.Errorf("Expected path %s, got %s", expectedNorm, foundNorm)
	}
}

func TestFindConfigFile_HiddenTakesPriority(t *testing.T) {
	tmpDir := t.TempDir()

	// Create both files
	hiddenPath := filepath.Join(tmpDir, ".bugbuster.yaml")
	visiblePath := filepath.Join(tmpDir, "bugbuster.yaml")
	if err := os.WriteFile(hiddenPath, []byte("default_provider: hidden"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(visiblePath, []byte("default_provider: visible"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	found, err := FindConfigFile()
	if err != nil {
		t.Errorf("Expected to find config, got error: %v", err)
	}
	// Hidden file (.bugbuster.yaml) takes priority
	foundNorm, _ := filepath.EvalSymlinks(found)
	expectedNorm, _ := filepath.EvalSymlinks(hiddenPath)
	if foundNorm != expectedNorm {
		t.Errorf("Expected hidden file %s to take priority, got %s", expectedNorm, foundNorm)
	}
}

func TestFindConfigFile_WalkUp(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config in parent directory
	configPath := filepath.Join(tmpDir, ".bugbuster.yaml")
	if err := os.WriteFile(configPath, []byte("default_provider: ollama"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create subdirectory
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(subDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	found, err := FindConfigFile()
	if err != nil {
		t.Errorf("Expected to find config in parent directory, got error: %v", err)
	}
	foundNorm, _ := filepath.EvalSymlinks(found)
	expectedNorm, _ := filepath.EvalSymlinks(configPath)
	if foundNorm != expectedNorm {
		t.Errorf("Expected path %s, got %s", expectedNorm, foundNorm)
	}
}

func TestEffectiveSecurity(t *testing.T) {
	cfg := DefaultConfig()
	// По умолчанию AllowNetwork=false, BlockedCommands=["rm -rf /", "mkfs", "dd if=", "format c:"]

	// Провайдер без security — используем глобальный
	provCfg := provider.ProviderConfig{Type: "anthropic"}
	sec := cfg.EffectiveSecurity(&provCfg)
	if sec.AllowNetwork != false {
		t.Errorf("Expected AllowNetwork=false without provider security, got %v", sec.AllowNetwork)
	}
	if len(sec.BlockedCommands) != 4 {
		t.Errorf("Expected 4 blocked commands from global, got %d", len(sec.BlockedCommands))
	}

	// Провайдер с allow_network: true — побеждает
	provCfg = provider.ProviderConfig{
		Type: "anthropic",
		Security: provider.ProviderSecurity{
			AllowNetwork: true,
		},
	}
	sec = cfg.EffectiveSecurity(&provCfg)
	if sec.AllowNetwork != true {
		t.Errorf("Expected AllowNetwork=true with provider override, got %v", sec.AllowNetwork)
	}

	// Провайдер с blocked_commands — заменяет глобальный
	provCfg = provider.ProviderConfig{
		Type: "anthropic",
		Security: provider.ProviderSecurity{
			BlockedCommands: []string{"rm -rf /"},
		},
	}
	sec = cfg.EffectiveSecurity(&provCfg)
	if len(sec.BlockedCommands) != 1 {
		t.Errorf("Expected 1 blocked command from provider, got %d", len(sec.BlockedCommands))
	}
	if sec.BlockedCommands[0] != "rm -rf /" {
		t.Errorf("Expected 'rm -rf /' from provider, got '%s'", sec.BlockedCommands[0])
	}
}

func TestEffectiveContextWindow(t *testing.T) {
	cfg := DefaultConfig()
	// По умолчанию Agent.MaxTokens = 32768

	// Провайдер без context_window — используем agent.max_tokens
	provCfg := provider.ProviderConfig{Type: "anthropic"}
	ctx := cfg.EffectiveContextWindow(&provCfg)
	if ctx != 32768 {
		t.Errorf("Expected context_window=32768 from agent fallback, got %d", ctx)
	}

	// Провайдер с context_window — побеждает
	provCfg = provider.ProviderConfig{
		Type:          "anthropic",
		ContextWindow: 200000,
	}
	ctx = cfg.EffectiveContextWindow(&provCfg)
	if ctx != 200000 {
		t.Errorf("Expected context_window=200000 from provider, got %d", ctx)
	}

	// Провайдер с context_window=0, agent.max_tokens=180000
	cfg.Agent.MaxTokens = 180000
	provCfg = provider.ProviderConfig{Type: "anthropic"}
	ctx = cfg.EffectiveContextWindow(&provCfg)
	if ctx != 180000 {
		t.Errorf("Expected context_window=180000 from agent fallback, got %d", ctx)
	}
}

func TestMergeConfigs_AllowNetwork(t *testing.T) {
	base := DefaultConfig()
	// По умолчанию AllowNetwork=false

	override := &BugBusterConfig{
		Security: SecurityConfig{
			AllowNetwork: true,
		},
	}

	merged := MergeConfigs(base, override)
	if !merged.Security.AllowNetwork {
		t.Errorf("Expected AllowNetwork=true after merge, got %v", merged.Security.AllowNetwork)
	}

	// Два override: один true, другой false — true побеждает
	override2 := &BugBusterConfig{
		Security: SecurityConfig{
			AllowNetwork: false,
		},
	}
	merged2 := MergeConfigs(base, override, override2)
	if !merged2.Security.AllowNetwork {
		t.Errorf("Expected AllowNetwork=true (true wins over false), got %v", merged2.Security.AllowNetwork)
	}
}

func TestProviderConfig_SystemPromptAndSkills(t *testing.T) {
	// Test that SystemPrompt and Skills fields are properly loaded from YAML
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bugbuster.yaml")

	yamlContent := `
default_provider: openai
providers:
  openai:
    type: openai
    api_key: sk-test
    model: gpt-4o
    system_prompt: |
      You are an expert in Rust and systems programming.
      Always use idiomatic Rust patterns.
    skills:
      - debug
      - review
  anthropic:
    type: anthropic
    api_key: sk-test2
    model: claude-sonnet-4-20250514
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	// Check openai provider has system_prompt and skills
	openai, ok := cfg.Providers["openai"]
	if !ok {
		t.Fatal("Expected 'openai' provider")
	}
	if openai.SystemPrompt == "" {
		t.Error("Expected SystemPrompt to be set for openai provider")
	}
	if !strings.Contains(openai.SystemPrompt, "Rust") {
		t.Errorf("Expected SystemPrompt to contain 'Rust', got '%s'", openai.SystemPrompt)
	}
	if len(openai.Skills) != 2 {
		t.Errorf("Expected 2 skills for openai provider, got %d", len(openai.Skills))
	}
	if openai.Skills[0] != "debug" || openai.Skills[1] != "review" {
		t.Errorf("Expected skills ['debug', 'review'], got %v", openai.Skills)
	}

	// Check anthropic provider has no system_prompt or skills
	anthropic, ok := cfg.Providers["anthropic"]
	if !ok {
		t.Fatal("Expected 'anthropic' provider")
	}
	if anthropic.SystemPrompt != "" {
		t.Errorf("Expected empty SystemPrompt for anthropic provider, got '%s'", anthropic.SystemPrompt)
	}
	if len(anthropic.Skills) != 0 {
		t.Errorf("Expected 0 skills for anthropic provider, got %d", len(anthropic.Skills))
	}
}

func TestProviderConfig_SystemPromptAndSkills_SaveLoad(t *testing.T) {
	// Test that SystemPrompt and Skills survive save/load cycle
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bugbuster.yaml")

	cfg := DefaultConfig()
	cfg.Providers["openai"] = provider.ProviderConfig{
		Type:         "openai",
		APIKey:       "sk-test",
		Model:        "gpt-4o",
		SystemPrompt: "You are a Python expert.\nAlways follow PEP 8.",
		Skills:       []string{"refactor", "test"},
	}

	if err := cfg.SaveConfig(configPath); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	openai := loaded.Providers["openai"]
	if openai.SystemPrompt != cfg.Providers["openai"].SystemPrompt {
		t.Errorf("SystemPrompt mismatch: got '%s', want '%s'", openai.SystemPrompt, cfg.Providers["openai"].SystemPrompt)
	}
	if len(openai.Skills) != 2 {
		t.Errorf("Expected 2 skills, got %d", len(openai.Skills))
	}
	if openai.Skills[0] != "refactor" || openai.Skills[1] != "test" {
		t.Errorf("Skills mismatch: got %v", openai.Skills)
	}
}

func TestMergeConfigs_SystemPromptAndSkills(t *testing.T) {
	// Test that SystemPrompt and Skills are properly merged
	base := DefaultConfig()

	override := &BugBusterConfig{
		Providers: map[string]provider.ProviderConfig{
			"openai": {
				Type:         "openai",
				APIKey:       "sk-test",
				Model:        "gpt-4o",
				SystemPrompt: "Custom prompt for OpenAI",
				Skills:       []string{"debug"},
			},
		},
	}

	merged := MergeConfigs(base, override)

	openai := merged.Providers["openai"]
	if openai.SystemPrompt != "Custom prompt for OpenAI" {
		t.Errorf("Expected SystemPrompt='Custom prompt for OpenAI', got '%s'", openai.SystemPrompt)
	}
	if len(openai.Skills) != 1 || openai.Skills[0] != "debug" {
		t.Errorf("Expected skills=['debug'], got %v", openai.Skills)
	}
}

func TestProviderConfig_SystemPromptFileAndSkillsDir(t *testing.T) {
	// Test that SystemPromptFile and SkillsDir fields are properly loaded from YAML
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bugbuster.yaml")

	yamlContent := `
default_provider: openai
providers:
  openai:
    type: openai
    api_key: sk-test
    model: gpt-4o
    system_prompt: |
      You are an expert in Rust.
    system_prompt_file: /path/to/custom_prompt.md
    skills:
      - debug
      - review
    skills_dir: /path/to/custom/skills
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	openai, ok := cfg.Providers["openai"]
	if !ok {
		t.Fatal("Expected 'openai' provider")
	}
	if openai.SystemPromptFile != "/path/to/custom_prompt.md" {
		t.Errorf("Expected SystemPromptFile='/path/to/custom_prompt.md', got '%s'", openai.SystemPromptFile)
	}
	if openai.SkillsDir != "/path/to/custom/skills" {
		t.Errorf("Expected SkillsDir='/path/to/custom/skills', got '%s'", openai.SkillsDir)
	}
	if !strings.Contains(openai.SystemPrompt, "Rust") {
		t.Errorf("Expected SystemPrompt to contain 'Rust', got '%s'", openai.SystemPrompt)
	}
	if len(openai.Skills) != 2 {
		t.Errorf("Expected 2 skills, got %d", len(openai.Skills))
	}
}

func TestProviderConfig_SystemPromptFile_SaveLoad(t *testing.T) {
	// Test that SystemPromptFile and SkillsDir survive save/load cycle
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bugbuster.yaml")

	cfg := DefaultConfig()
	cfg.Providers["openai"] = provider.ProviderConfig{
		Type:             "openai",
		APIKey:           "sk-test",
		Model:            "gpt-4o",
		SystemPrompt:     "You are a Python expert.",
		SystemPromptFile: "/path/to/prompt.md",
		Skills:           []string{"refactor", "test"},
		SkillsDir:        "/path/to/skills",
	}

	if err := cfg.SaveConfig(configPath); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	openai := loaded.Providers["openai"]
	if openai.SystemPromptFile != "/path/to/prompt.md" {
		t.Errorf("SystemPromptFile mismatch: got '%s', want '/path/to/prompt.md'", openai.SystemPromptFile)
	}
	if openai.SkillsDir != "/path/to/skills" {
		t.Errorf("SkillsDir mismatch: got '%s', want '/path/to/skills'", openai.SkillsDir)
	}
	if openai.SystemPrompt != "You are a Python expert." {
		t.Errorf("SystemPrompt mismatch: got '%s'", openai.SystemPrompt)
	}
}
