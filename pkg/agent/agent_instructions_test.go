package agent

import (
	"os"
	"path/filepath"
	"testing"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/tools"
)

func TestLoadAgentInstructions_NoFiles(t *testing.T) {
	dir := t.TempDir()
	instructions := LoadAgentInstructions(dir)
	if len(instructions) != 0 {
		t.Errorf("Expected no instructions for empty dir, got %d", len(instructions))
	}
}

func TestLoadAgentInstructions_AgentMd(t *testing.T) {
	dir := t.TempDir()
	content := "# My Project\n\nAlways use tabs for indentation.\nNever use fmt.Println."
	err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	instructions := LoadAgentInstructions(dir)
	if len(instructions) != 1 {
		t.Fatalf("Expected 1 instruction, got %d", len(instructions))
	}

	if instructions[0].File.Path != "AGENT.md" {
		t.Errorf("Expected AGENT.md, got %s", instructions[0].File.Path)
	}
	if instructions[0].Content != content {
		t.Errorf("Content mismatch: got %q, want %q", instructions[0].Content, content)
	}
	if instructions[0].File.Format != "markdown" {
		t.Errorf("Expected markdown format, got %s", instructions[0].File.Format)
	}
}

func TestLoadAgentInstructions_ClaudeMd(t *testing.T) {
	dir := t.TempDir()
	content := "Use TypeScript strict mode.\nPrefer functional components."
	err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	instructions := LoadAgentInstructions(dir)
	if len(instructions) != 1 {
		t.Fatalf("Expected 1 instruction, got %d", len(instructions))
	}
	if instructions[0].File.Path != "CLAUDE.md" {
		t.Errorf("Expected CLAUDE.md, got %s", instructions[0].File.Path)
	}
}

func TestLoadAgentInstructions_Cursorrules(t *testing.T) {
	dir := t.TempDir()
	content := "Always use 2 spaces for indentation."
	err := os.WriteFile(filepath.Join(dir, ".cursorrules"), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	instructions := LoadAgentInstructions(dir)
	if len(instructions) != 1 {
		t.Fatalf("Expected 1 instruction, got %d", len(instructions))
	}
	if instructions[0].File.Path != ".cursorrules" {
		t.Errorf("Expected .cursorrules, got %s", instructions[0].File.Path)
	}
}

func TestLoadAgentInstructions_GitHubCopilot(t *testing.T) {
	dir := t.TempDir()
	githubDir := filepath.Join(dir, ".github")
	err := os.MkdirAll(githubDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	content := "# Copilot Instructions\n\nUse Go 1.22+ features."
	err = os.WriteFile(filepath.Join(githubDir, "copilot-instructions.md"), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	instructions := LoadAgentInstructions(dir)
	if len(instructions) != 1 {
		t.Fatalf("Expected 1 instruction, got %d", len(instructions))
	}
	if instructions[0].File.Path != ".github/copilot-instructions.md" {
		t.Errorf("Expected .github/copilot-instructions.md, got %s", instructions[0].File.Path)
	}
}

func TestLoadAgentInstructions_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	// Create AGENT.md
	agentContent := "# Agent Instructions\n\nUse tabs."
	err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte(agentContent), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Create .cursorrules
	cursorContent := "Use 2 spaces."
	err = os.WriteFile(filepath.Join(dir, ".cursorrules"), []byte(cursorContent), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	instructions := LoadAgentInstructions(dir)
	if len(instructions) != 2 {
		t.Fatalf("Expected 2 instructions, got %d", len(instructions))
	}

	// AGENT.md should be first (higher priority)
	if instructions[0].File.Path != "AGENT.md" {
		t.Errorf("Expected AGENT.md first, got %s", instructions[0].File.Path)
	}
	if instructions[1].File.Path != ".cursorrules" {
		t.Errorf("Expected .cursorrules second, got %s", instructions[1].File.Path)
	}
}

func TestLoadAgentInstructions_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte("  \n  \n  "), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	instructions := LoadAgentInstructions(dir)
	if len(instructions) != 0 {
		t.Errorf("Expected no instructions for empty file, got %d", len(instructions))
	}
}

func TestLoadAgentInstructions_AgentMdPriority(t *testing.T) {
	dir := t.TempDir()

	// Create both AGENT.md and CLAUDE.md
	agentContent := "BugBuster instructions"
	claudeContent := "Claude instructions"
	err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte(agentContent), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(claudeContent), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	instructions := LoadAgentInstructions(dir)
	if len(instructions) != 2 {
		t.Fatalf("Expected 2 instructions, got %d", len(instructions))
	}

	// Both files should be loaded — AGENT.md first
	if instructions[0].File.Path != "AGENT.md" {
		t.Errorf("Expected AGENT.md first, got %s", instructions[0].File.Path)
	}
	if instructions[1].File.Path != "CLAUDE.md" {
		t.Errorf("Expected CLAUDE.md second, got %s", instructions[1].File.Path)
	}
}

func TestFormatInstructions_Empty(t *testing.T) {
	result := FormatInstructions(nil)
	if result != "" {
		t.Errorf("Expected empty string for nil instructions, got %q", result)
	}

	result = FormatInstructions([]LoadedInstruction{})
	if result != "" {
		t.Errorf("Expected empty string for empty instructions, got %q", result)
	}
}

func TestFormatInstructions_SingleFile(t *testing.T) {
	instructions := []LoadedInstruction{
		{
			File:    AgentInstructionFiles[0], // AGENT.md
			Content: "Always use tabs.",
		},
	}

	result := FormatInstructions(instructions)
	if result != "Always use tabs." {
		t.Errorf("Expected 'Always use tabs.', got %q", result)
	}
}

func TestFormatInstructions_MultipleFiles(t *testing.T) {
	instructions := []LoadedInstruction{
		{
			File:    AgentInstructionFiles[0], // AGENT.md
			Content: "Always use tabs.",
		},
		{
			File:    AgentInstructionFiles[2], // .cursorrules
			Content: "Use 2 spaces.",
		},
	}

	result := FormatInstructions(instructions)
	expected := "Always use tabs.\n\nUse 2 spaces."
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestGetInstructionFileNames(t *testing.T) {
	names := GetInstructionFileNames()
	if len(names) != len(AgentInstructionFiles) {
		t.Errorf("Expected %d names, got %d", len(AgentInstructionFiles), len(names))
	}
	if names[0] != "AGENT.md" {
		t.Errorf("Expected first name to be AGENT.md, got %s", names[0])
	}
}

func TestBuildSystemPrompt_WithAgentInstructions(t *testing.T) {
	dir := t.TempDir()
	content := "# My Project\n\nAlways use tabs for indentation."
	err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Initialize i18n
	i18n.Init("en")

	toolList := map[string]tools.Tool{}
	prompt := BuildSystemPrompt(dir, toolList)

	if prompt == "" {
		t.Error("Expected non-empty system prompt")
	}

	// Check that instructions are included
	if !containsStr(prompt, "Always use tabs for indentation.") {
		t.Error("Expected system prompt to contain agent instructions")
	}

	// Check that instructions header is included
	if !containsStr(prompt, "Project Instructions") {
		t.Error("Expected system prompt to contain instructions header")
	}
}

func TestBuildSystemPrompt_WithoutAgentInstructions(t *testing.T) {
	dir := t.TempDir()

	// Initialize i18n
	i18n.Init("en")

	toolList := map[string]tools.Tool{}
	prompt := BuildSystemPrompt(dir, toolList)

	if prompt == "" {
		t.Error("Expected non-empty system prompt")
	}

	// Check that instructions header is NOT included (no instruction files)
	if containsStr(prompt, "Project Instructions") {
		t.Error("Expected system prompt NOT to contain instructions header when no files exist")
	}
}

