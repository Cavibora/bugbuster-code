package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LSPClient manages LSP server lifecycle via stdio JSON-RPC.
type LSPClient struct {
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       *bufio.Reader
	mu           sync.Mutex
	nextID       int
	pending      map[int]chan jsonResponse
	rootURI      string
	timeout      time.Duration
	opened       map[string]bool // tracking of open documents
	capabilities InitializeResult
	running      bool
}

// jsonResponse — response from LSP server
type jsonResponse struct {
	Result json.RawMessage
	Error  *jsonRPCError
}

// jsonRPCError — error JSON-RPC
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// InitializeResult — initialize result (we only need capabilities)
type InitializeResult struct {
	Capabilities struct {
		HoverProvider          bool `json:"hoverProvider"`
		DefinitionProvider     bool `json:"definitionProvider"`
		ReferencesProvider     bool `json:"referencesProvider"`
		DocumentSymbolProvider bool `json:"documentSymbolProvider"`
	} `json:"capabilities"`
}

// Location — position in file (LSP type)
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// Range — range in file (LSP type)
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position — position in file (LSP type, zero-based)
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// HoverResult — result hover-request (LSP type)
type HoverResult struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// MarkupContent — content with markup type (LSP type)
type MarkupContent struct {
	Kind  string `json:"kind"` // "plaintext" or "markdown"
	Value string `json:"value"`
}

// DocumentSymbol is a symbol in document (LSP type)
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Kind           SymbolKind       `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// SymbolKind — symbol kind (LSP type)
type SymbolKind int

const (
	SymbolKindFile        SymbolKind = 1
	SymbolKindModule      SymbolKind = 2
	SymbolKindNamespace   SymbolKind = 3
	SymbolKindPackage     SymbolKind = 4
	SymbolKindClass       SymbolKind = 5
	SymbolKindMethod      SymbolKind = 6
	SymbolKindProperty    SymbolKind = 7
	SymbolKindField       SymbolKind = 8
	SymbolKindConstructor SymbolKind = 9
	SymbolKindEnum        SymbolKind = 10
	SymbolKindInterface   SymbolKind = 11
	SymbolKindFunction    SymbolKind = 12
	SymbolKindVariable    SymbolKind = 13
	SymbolKindConstant    SymbolKind = 14
	SymbolKindString      SymbolKind = 15
	SymbolKindNumber      SymbolKind = 16
	SymbolKindBoolean     SymbolKind = 17
	SymbolKindArray       SymbolKind = 18
)

// SymbolKindNames — human-readable names of symbol kinds
var SymbolKindNames = map[SymbolKind]string{
	SymbolKindFile:        "File",
	SymbolKindModule:      "Module",
	SymbolKindNamespace:   "Namespace",
	SymbolKindPackage:     "Package",
	SymbolKindClass:       "Class",
	SymbolKindMethod:      "Method",
	SymbolKindProperty:    "Property",
	SymbolKindField:       "Field",
	SymbolKindConstructor: "Constructor",
	SymbolKindEnum:        "Enum",
	SymbolKindInterface:   "Interface",
	SymbolKindFunction:    "Function",
	SymbolKindVariable:    "Variable",
	SymbolKindConstant:    "Constant",
	SymbolKindString:      "String",
	SymbolKindNumber:      "Number",
	SymbolKindBoolean:     "Boolean",
	SymbolKindArray:       "Array",
}

// NewLSPClient creates LSP client (does not start server).
func NewLSPClient(command string, args []string, rootURI string, timeout time.Duration) (*LSPClient, error) {
	cmd := exec.Command(command, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	return &LSPClient{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		rootURI: rootURI,
		timeout: timeout,
		pending: make(map[int]chan jsonResponse),
		opened:  make(map[string]bool),
	}, nil
}

// Start starts LSP server process and sends initialize.
func (c *LSPClient) Start(ctx context.Context) error {
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start LSP server: %w", err)
	}
	c.running = true

	// Start reading responses in background
	go c.readLoop()

	// Initialization
	initParams := map[string]any{
		"processId":    nil,
		"rootUri":      c.rootURI,
		"capabilities": map[string]any{},
	}

	result, err := c.sendRequest(ctx, "initialize", initParams)
	if err != nil {
		_ = c.Shutdown()
		return fmt.Errorf("initialize failed: %w", err)
	}

	// Parse capabilities
	var initResult InitializeResult
	if err := json.Unmarshal(result, &initResult); err == nil {
		c.capabilities = initResult
	}

	// Send initialized notification
	if err := c.sendNotification("initialized", map[string]any{}); err != nil {
		_ = c.Shutdown()
		return fmt.Errorf("initialized notification failed: %w", err)
	}

	return nil
}

// IsAlive checks if LSP server process is still running.
func (c *LSPClient) IsAlive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// GetCapabilities returns server capabilities.
func (c *LSPClient) GetCapabilities() InitializeResult {
	return c.capabilities
}

// readLoop — background goroutine for reading responses from stdout.
func (c *LSPClient) readLoop() {
	defer func() {
		c.mu.Lock()
		c.running = false
		// Close all pending channels
		for id, ch := range c.pending {
			ch <- jsonResponse{Error: &jsonRPCError{Code: -1, Message: "server exited"}}
			close(ch)
			delete(c.pending, id)
		}
		c.mu.Unlock()
	}()

	for {
		msg, err := c.readMessage()
		if err != nil {
			return // EOF or error — server terminated
		}

		var rpcMsg struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  *jsonRPCError   `json:"error"`
		}
		if err := json.Unmarshal(msg, &rpcMsg); err != nil {
			continue
		}

		// If there is an ID — this is a response to a request
		if rpcMsg.ID > 0 {
			c.mu.Lock()
			if ch, ok := c.pending[rpcMsg.ID]; ok {
				ch <- jsonResponse{Result: rpcMsg.Result, Error: rpcMsg.Error}
				delete(c.pending, rpcMsg.ID)
			}
			c.mu.Unlock()
		}
		// Notifications (method without ID) — ignored
	}
}

// sendRequest sends JSON-RPC request and waits for response.
func (c *LSPClient) sendRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	ch := make(chan jsonResponse, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}

	if err := c.writeMessage(msg); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Wait for response with timeout
	timeout := c.timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("LSP error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-time.After(timeout):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("LSP request timed out after %v", timeout)
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

// sendNotification sends JSON-RPC notification (without response).
func (c *LSPClient) sendNotification(method string, params any) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	return c.writeMessage(msg)
}

// writeMessage formats Content-Length header + JSON body.
func (c *LSPClient) writeMessage(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := c.stdin.Write([]byte(header)); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return nil
}

// readMessage reads Content-Length header + JSON body.
func (c *LSPClient) readMessage() (json.RawMessage, error) {
	// Read header
	var contentLength int
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break // end of headers
		}
		if strings.HasPrefix(line, "Content-Length:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
				if err == nil {
					contentLength = n
				}
			}
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	// Read body
	data := make([]byte, contentLength)
	if _, err := io.ReadFull(c.stdout, data); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return json.RawMessage(data), nil
}

// OpenDocument sends textDocument/didOpen (if not already open).
func (c *LSPClient) OpenDocument(ctx context.Context, filePath string, languageID string, content string) error {
	uri := pathToURI(filePath)

	c.mu.Lock()
	if c.opened[uri] {
		c.mu.Unlock()
		return nil // already open
	}
	c.mu.Unlock()

	params := map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": languageID,
			"version":    1,
			"text":       content,
		},
	}

	if err := c.sendNotification("textDocument/didOpen", params); err != nil {
		return fmt.Errorf("didOpen failed: %w", err)
	}

	c.mu.Lock()
	c.opened[uri] = true
	c.mu.Unlock()
	return nil
}

// GoToDefinition sends textDocument/definition.
func (c *LSPClient) GoToDefinition(ctx context.Context, filePath string, line, character int) ([]Location, error) {
	uri := pathToURI(filePath)
	params := map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line, "character": character},
	}

	result, err := c.sendRequest(ctx, "textDocument/definition", params)
	if err != nil {
		return nil, err
	}

	return parseLocations(result)
}

// FindReferences sends textDocument/references.
func (c *LSPClient) FindReferences(ctx context.Context, filePath string, line, character int) ([]Location, error) {
	uri := pathToURI(filePath)
	params := map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line, "character": character},
		"context":      map[string]any{"includeDeclaration": true},
	}

	result, err := c.sendRequest(ctx, "textDocument/references", params)
	if err != nil {
		return nil, err
	}

	return parseLocations(result)
}

// Hover sends textDocument/hover.
func (c *LSPClient) Hover(ctx context.Context, filePath string, line, character int) (*HoverResult, error) {
	uri := pathToURI(filePath)
	params := map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line, "character": character},
	}

	result, err := c.sendRequest(ctx, "textDocument/hover", params)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	var hover HoverResult
	if err := json.Unmarshal(result, &hover); err != nil {
		return nil, fmt.Errorf("parse hover result: %w", err)
	}
	return &hover, nil
}

// DocumentSymbols sends textDocument/documentSymbol.
func (c *LSPClient) DocumentSymbols(ctx context.Context, filePath string) ([]DocumentSymbol, error) {
	uri := pathToURI(filePath)
	params := map[string]any{
		"textDocument": map[string]any{"uri": uri},
	}

	result, err := c.sendRequest(ctx, "textDocument/documentSymbol", params)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	var symbols []DocumentSymbol
	if err := json.Unmarshal(result, &symbols); err != nil {
		return nil, fmt.Errorf("parse document symbols: %w", err)
	}
	return symbols, nil
}

// Shutdown sends shutdown + exit and closes process.
func (c *LSPClient) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// shutdown request
	_, _ = c.sendRequest(ctx, "shutdown", nil)

	// exit notification
	_ = c.sendNotification("exit", nil)

	// Close stdin so server terminates
	c.stdin.Close()

	// Wait for process to terminate
	done := make(chan error, 1)
	go func() {
		done <- c.cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		c.cmd.Process.Kill()
		return fmt.Errorf("LSP server did not exit in time")
	}
}

// parseLocations parses result definition/references.
// LSP may return Location, Location[], or null.
func parseLocations(data json.RawMessage) ([]Location, error) {
	if data == nil || string(data) == "null" {
		return nil, nil
	}

	// Try as array
	var locations []Location
	if err := json.Unmarshal(data, &locations); err == nil {
		return locations, nil
	}

	// Try as single Location
	var loc Location
	if err := json.Unmarshal(data, &loc); err == nil {
		return []Location{loc}, nil
	}

	return nil, nil
}

// pathToURI converts path to file:// URI.
func pathToURI(path string) string {
	// Remove leading slash if any and add file://
	if !strings.HasPrefix(path, "file://") {
		path = strings.ReplaceAll(path, "\\", "/")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		path = "file://" + path
	}
	return path
}

// uriToPath converts file:// URI to path.
func uriToPath(uri string) string {
	return strings.TrimPrefix(uri, "file://")
}
