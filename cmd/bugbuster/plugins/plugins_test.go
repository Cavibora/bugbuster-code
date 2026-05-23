package plugins

import (
	"testing"
	"time"

	"bugbuster-code/pkg/plugin"
)

func TestFilesystemPluginInit(t *testing.T) {
	p := NewFilesystemPlugin()
	if p.Name() != "filesystem" {
		t.Errorf("expected name filesystem, got %s", p.Name())
	}
	if p.Version() != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", p.Version())
	}

	// Init with config
	err := p.Init(map[string]any{
		"allowed_dirs":     []any{"/tmp", "/home"},
		"max_file_size":    2048,
		"max_grep_results": 10,
		"max_glob_results": 20,
	})
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	if len(p.allowedDirs) != 2 {
		t.Errorf("expected 2 allowed dirs, got %d", len(p.allowedDirs))
	}
	if p.maxFileSize != 2048 {
		t.Errorf("expected maxFileSize 2048, got %d", p.maxFileSize)
	}
	if p.maxGrepRes != 10 {
		t.Errorf("expected maxGrepRes 10, got %d", p.maxGrepRes)
	}
	if p.maxGlobRes != 20 {
		t.Errorf("expected maxGlobRes 20, got %d", p.maxGlobRes)
	}
}

func TestFilesystemPluginTools(t *testing.T) {
	p := NewFilesystemPlugin()
	err := p.Init(nil)
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	tools := p.Tools()
	if len(tools) != 5 {
		t.Errorf("expected 5 tools, got %d", len(tools))
	}

	// Проверяем имена инструментов
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name()] = true
	}

	expectedNames := []string{"read", "write", "edit", "glob", "grep"}
	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("expected tool %s not found", name)
		}
	}
}

func TestFilesystemPluginShutdown(t *testing.T) {
	p := NewFilesystemPlugin()
	err := p.Shutdown()
	if err != nil {
		t.Errorf("Shutdown error: %v", err)
	}
}

func TestBashPluginInit(t *testing.T) {
	p := NewBashPlugin()
	if p.Name() != "bash" {
		t.Errorf("expected name bash, got %s", p.Name())
	}

	err := p.Init(map[string]any{
		"timeout":          60,
		"blocked_commands": []any{"rm -rf /", "mkfs"},
		"allow_network":    true,
		"default_dir":      "/tmp",
	})
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	if p.timeout.Seconds() != 60 {
		t.Errorf("expected timeout 60s, got %v", p.timeout)
	}
	if len(p.blockedCmds) != 2 {
		t.Errorf("expected 2 blocked commands, got %d", len(p.blockedCmds))
	}
	if !p.allowNetwork {
		t.Error("expected allowNetwork=true")
	}
	if p.defaultDir != "/tmp" {
		t.Errorf("expected defaultDir /tmp, got %s", p.defaultDir)
	}
}

func TestBashPluginTools(t *testing.T) {
	p := NewBashPlugin()
	err := p.Init(nil)
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	tools := p.Tools()
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name() != "bash" {
		t.Errorf("expected tool name bash, got %s", tools[0].Name())
	}
}

func TestBashPluginValidateTimeout(t *testing.T) {
	p := NewBashPlugin()

	// Timeout too small
	p.timeout = 0
	if err := p.ValidateTimeout(); err == nil {
		t.Error("expected error for timeout < 1s")
	}

	// Timeout too large
	p.timeout = 3600 * time.Second
	if err := p.ValidateTimeout(); err == nil {
		t.Error("expected error for timeout > 5m")
	}

	// Valid timeout
	p.timeout = 30 * time.Second
	if err := p.ValidateTimeout(); err != nil {
		t.Errorf("expected no error for valid timeout, got %v", err)
	}
}

func TestWebPluginInit(t *testing.T) {
	p := NewWebPlugin()
	if p.Name() != "web" {
		t.Errorf("expected name web, got %s", p.Name())
	}

	err := p.Init(map[string]any{
		"allow_network":  false,
		"timeout":        15,
		"max_body_size":  524288,
	})
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	if p.allowNetwork {
		t.Error("expected allowNetwork=false")
	}
	if p.timeout.Seconds() != 15 {
		t.Errorf("expected timeout 15s, got %v", p.timeout)
	}
	if p.maxBodySize != 524288 {
		t.Errorf("expected maxBodySize 524288, got %d", p.maxBodySize)
	}
}

func TestWebPluginTools(t *testing.T) {
	p := NewWebPlugin()
	err := p.Init(nil)
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	tools := p.Tools()
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name() != "web_fetch" {
		t.Errorf("expected tool name web_fetch, got %s", tools[0].Name())
	}
}

func TestRegisterAll(t *testing.T) {
	registry := plugin.NewRegistry()
	RegisterAll(registry)

	// Проверяем, что фабрики зарегистрированы
	_ = registry.GetAllTools()
	// Без Init() инструменты не создаются, но фабрики должны быть зарегистрированы

	// Загружаем плагин filesystem
	p, err := registry.Load("filesystem", map[string]any{
		"allowed_dirs": []any{"/tmp"},
	})
	if err != nil {
		t.Fatalf("Load filesystem error: %v", err)
	}
	if p.Name() != "filesystem" {
		t.Errorf("expected name filesystem, got %s", p.Name())
	}

	// Загружаем плагин bash
	p, err = registry.Load("bash", map[string]any{
		"timeout": 30,
	})
	if err != nil {
		t.Fatalf("Load bash error: %v", err)
	}
	if p.Name() != "bash" {
		t.Errorf("expected name bash, got %s", p.Name())
	}

	// Загружаем плагин web
	p, err = registry.Load("web", nil)
	if err != nil {
		t.Fatalf("Load web error: %v", err)
	}
	if p.Name() != "web" {
		t.Errorf("expected name web, got %s", p.Name())
	}
}

func TestRegisterAllUnknownPlugin(t *testing.T) {
	registry := plugin.NewRegistry()
	RegisterAll(registry)

	_, err := registry.Load("unknown", nil)
	if err == nil {
		t.Error("expected error for unknown plugin")
	}
}