package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// AgentInstructionFile defines a file that contains agent instructions.
// These files are read from the project directory and appended to the system prompt.
// Priority is determined by order: first match wins.
var AgentInstructionFiles = []AgentInstructionFile{
	// BugBuster native format — highest priority
	{Path: "AGENT.md", Format: "markdown", Description: "BugBuster agent instructions"},
	// Claude Code format
	{Path: "CLAUDE.md", Format: "markdown", Description: "Claude Code instructions"},
	// Cursor format
	{Path: ".cursorrules", Format: "text", Description: "Cursor rules"},
	// GitHub Copilot format
	{Path: ".github/copilot-instructions.md", Format: "markdown", Description: "GitHub Copilot instructions"},
	// Windsurf format
	{Path: ".windsurfrules", Format: "text", Description: "Windsurf rules"},
	// Aider format
	{Path: ".aider.conf.yml", Format: "yaml", Description: "Aider configuration"},
	// Cline format
	{Path: ".clinerules", Format: "text", Description: "Cline rules"},
}

// AgentInstructionFile describes a file with agent instructions.
type AgentInstructionFile struct {
	Path        string // Relative path from project directory
	Format      string // File format: "markdown", "text", "yaml"
	Description string // Human-readable description
}

// LoadAgentInstructions reads agent instruction files from the project directory.
// It returns the content of the first found file, plus any additional files found.
// Priority is determined by the order in AgentInstructionFiles.
//
// The function looks for files in the following order:
// 1. AGENT.md — BugBuster native format (highest priority)
// 2. CLAUDE.md — Claude Code format
// 3. .cursorrules — Cursor format
// 4. .github/copilot-instructions.md — GitHub Copilot format
// 5. .windsurfrules — Windsurf format
// 6. .aider.conf.yml — Aider format
// 7. .clinerules — Cline format
//
// If AGENT.md exists, it is always used. Other files are used as fallback.
// Multiple files can be loaded if they exist (e.g., both AGENT.md and .cursorrules).
func LoadAgentInstructions(projectDir string) []LoadedInstruction {
	var instructions []LoadedInstruction

	for _, file := range AgentInstructionFiles {
		fullPath := filepath.Join(projectDir, file.Path)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue // File doesn't exist or can't be read — skip
		}

		content := strings.TrimSpace(string(data))
		if content == "" {
			continue // Empty file — skip
		}

		instructions = append(instructions, LoadedInstruction{
			File:        file,
			Content:     content,
			Source:      fullPath,
			ContentSize: len(content),
		})
	}

	return instructions
}

// LoadedInstruction represents a loaded agent instruction file.
type LoadedInstruction struct {
	File        AgentInstructionFile
	Content     string // Raw content of the file
	Source      string // Full path to the file
	ContentSize int    // Size in bytes
}

// FormatInstructions formats loaded instructions for inclusion in the system prompt.
// It returns a string that can be appended to the system prompt.
// If no instructions are loaded, it returns an empty string.
func FormatInstructions(instructions []LoadedInstruction) string {
	if len(instructions) == 0 {
		return ""
	}

	var sb strings.Builder

	for i, instr := range instructions {
		if i > 0 {
			sb.WriteString("\n\n")
		}

		// Add header based on file format
		switch instr.File.Format {
		case "markdown":
			// For markdown files, include content as-is
			// The file may already have headers
			sb.WriteString(instr.Content)
		case "yaml":
			// For YAML files, extract relevant sections
			sb.WriteString(instr.Content)
		default:
			// For text files, wrap in a section
			sb.WriteString(instr.Content)
		}
	}

	return sb.String()
}

// GetInstructionFileNames returns human-readable names of all supported instruction files.
func GetInstructionFileNames() []string {
	names := make([]string, len(AgentInstructionFiles))
	for i, f := range AgentInstructionFiles {
		names[i] = f.Path
	}
	return names
}