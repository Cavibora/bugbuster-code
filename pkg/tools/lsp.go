package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bugbuster-code/pkg/i18n"
)

// LSPServerConfig — LSP server configuration (compatible with config.LSPServerConfig)
type LSPServerConfig struct {
	Command string
	Args    []string
}

// LSPTool — LSP code analysis tool via Language Server Protocol.
// Connects to LSP servers (gopls, clangd, typescript-language-server etc.)
// and provides operations: go_to_definition, find_references, hover, document_symbols.
type LSPTool struct {
	AllowedDirs []string
	Timeout     time.Duration
	Servers     map[string]LSPServerConfig // language → server configuration
	rootDir     string

	// Cache of running clients: language → *LSPClient
	clients   map[string]*LSPClient
	clientsMu sync.Mutex
}

// NewLSPTool creates LSP analysis tool.
func NewLSPTool() *LSPTool {
	return &LSPTool{
		Timeout: 10 * time.Second,
		Servers: make(map[string]LSPServerConfig),
		clients: make(map[string]*LSPClient),
	}
}

// SetRootDir sets project root directory.
func (t *LSPTool) SetRootDir(dir string) {
	t.rootDir = dir
}

func (t *LSPTool) Name() string { return "lsp" }

func (t *LSPTool) Description() string {
	return i18n.T("tools.lsp.description")
}

func (t *LSPTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"go_to_definition", "find_references", "hover", "document_symbols"},
				"description": i18n.T("tools.lsp.param_operation_desc"),
			},
			"file_path": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.lsp.param_file_path_desc"),
			},
			"line": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.lsp.param_line_desc"),
			},
			"character": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.lsp.param_character_desc"),
			},
		},
		"required": []string{"operation", "file_path"},
	}
}

// Execute executes an LSP operation.
func (t *LSPTool) Execute(params map[string]string) ToolResult {
	operation := params["operation"]
	filePath := params["file_path"]

	if operation == "" {
		return Error("tools.lsp.param_required")
	}
	if filePath == "" {
		return Error("tools.lsp.param_required")
	}

	// Security: resolve and check path
	resolvedPath, err := ResolvePath(filePath)
	if err != nil {
		return Error("security.error", err)
	}
	if !IsPathAllowed(resolvedPath, t.AllowedDirs) {
		return Error("security.path_not_allowed", resolvedPath)
	}

	// Determine language by extension
	language := detectLanguage(resolvedPath)
	if language == "" {
		return Error("tools.lsp.unsupported_language", filepath.Ext(resolvedPath))
	}

	// Check if there is a server for this language
	serverCfg, ok := t.Servers[language]
	if !ok {
		return Error("tools.lsp.server_not_found", language, language)
	}

	// Get or create LSP client
	client, err := t.getClient(language, serverCfg)
	if err != nil {
		return Error("tools.lsp.server_start_error", language, err)
	}

	// Read file and open document
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return Error("tools.lsp.file_read_error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), t.Timeout)
	defer cancel()

	if err := client.OpenDocument(ctx, resolvedPath, language, string(content)); err != nil {
		return Error("tools.lsp.server_error", language, err)
	}

	// Execute operation
	switch operation {
	case "go_to_definition":
		line, char := parseLineChar(params)
		locations, err := client.GoToDefinition(ctx, resolvedPath, line, char)
		if err != nil {
			return Error("tools.lsp.server_error", language, err)
		}
		if len(locations) == 0 {
			return Success("tools.lsp.no_definition")
		}
		return Success("%s", formatLocations(locations))

	case "find_references":
		line, char := parseLineChar(params)
		locations, err := client.FindReferences(ctx, resolvedPath, line, char)
		if err != nil {
			return Error("tools.lsp.server_error", language, err)
		}
		if len(locations) == 0 {
			return Success("tools.lsp.no_references")
		}
		return Success("%s", formatLocations(locations))

	case "hover":
		line, char := parseLineChar(params)
		hover, err := client.Hover(ctx, resolvedPath, line, char)
		if err != nil {
			return Error("tools.lsp.server_error", language, err)
		}
		if hover == nil {
			return Success("tools.lsp.no_hover")
		}
		return Success("%s", formatHover(hover))

	case "document_symbols":
		symbols, err := client.DocumentSymbols(ctx, resolvedPath)
		if err != nil {
			return Error("tools.lsp.server_error", language, err)
		}
		if len(symbols) == 0 {
			return Success("tools.lsp.no_symbols")
		}
		return Success("%s", formatSymbols(symbols, ""))

	default:
		return Error("tools.lsp.invalid_operation", operation)
	}
}

// getClient returns or creates LSP client for language.
func (t *LSPTool) getClient(language string, serverCfg LSPServerConfig) (*LSPClient, error) {
	t.clientsMu.Lock()
	defer t.clientsMu.Unlock()

	// Check cache
	if client, ok := t.clients[language]; ok && client.IsAlive() {
		return client, nil
	}

	// Kill dead client
	if client, ok := t.clients[language]; ok {
		_ = client.Shutdown()
		delete(t.clients, language)
	}

	// Create new
	rootDir := t.rootDir
	if rootDir == "" {
		rootDir, _ = os.Getwd()
	}
	rootURI := pathToURI(rootDir)

	client, err := NewLSPClient(serverCfg.Command, serverCfg.Args, rootURI, t.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create LSP client for %s: %w", language, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start LSP server for %s: %w", language, err)
	}

	t.clients[language] = client
	return client, nil
}

// Shutdown closes all LSP clients.
func (t *LSPTool) Shutdown() {
	t.clientsMu.Lock()
	defer t.clientsMu.Unlock()

	for lang, client := range t.clients {
		_ = client.Shutdown()
		delete(t.clients, lang)
	}
}

// detectLanguage determines programming language by file extension.
func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "typescript" // typescript-language-server supports JS
	case ".py":
		return "python"
	case ".c", ".h":
		return "clangd"
	case ".cpp", ".cc", ".cxx", ".hpp", ".hxx":
		return "clangd"
	case ".rs":
		return "rust-analyzer"
	case ".java":
		return "jdtls"
	default:
		return ""
	}
}

// parseLineChar parses line and character parameters (1-based → 0-based).
func parseLineChar(params map[string]string) (line, character int) {
	line = 0
	character = 0
	if l, ok := params["line"]; ok && l != "" {
		if n, err := fmt.Sscanf(l, "%d", &line); err == nil && n == 1 {
			// LSP uses 0-based, user enters 1-based
			if line > 0 {
				line--
			}
		}
	}
	if c, ok := params["character"]; ok && c != "" {
		if n, err := fmt.Sscanf(c, "%d", &character); err == nil && n == 1 {
			if character > 0 {
				character--
			}
		}
	}
	return line, character
}

// formatLocations formats Location list in readable form.
func formatLocations(locations []Location) string {
	var lines []string
	for _, loc := range locations {
		path := uriToPath(loc.URI)
		line := fmt.Sprintf("%s:%d:%d", path, loc.Range.Start.Line+1, loc.Range.Start.Character+1)
		if loc.Range.End.Line != loc.Range.Start.Line || loc.Range.End.Character != loc.Range.Start.Character {
			line += fmt.Sprintf(" — %d:%d", loc.Range.End.Line+1, loc.Range.End.Character+1)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// formatHover formats result hover.
func formatHover(hover *HoverResult) string {
	if hover.Contents.Value != "" {
		return hover.Contents.Value
	}
	return "(no hover content)"
}

// formatSymbols formats tree DocumentSymbol.
func formatSymbols(symbols []DocumentSymbol, indent string) string {
	var lines []string
	for _, sym := range symbols {
		kindName := SymbolKindNames[sym.Kind]
		if kindName == "" {
			kindName = fmt.Sprintf("Kind%d", sym.Kind)
		}
		line := fmt.Sprintf("%s%s [%s] line %d", indent, sym.Name, kindName, sym.Range.Start.Line+1)
		lines = append(lines, line)
		if len(sym.Children) > 0 {
			lines = append(lines, formatSymbols(sym.Children, indent+"  "))
		}
	}
	return strings.Join(lines, "\n")
}
