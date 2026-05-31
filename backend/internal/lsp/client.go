package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// Client communicates with a language server via JSON-RPC over stdio.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex // protects stdin writes
	nextID atomic.Int64
	respCh map[int]chan *Response
	respMu sync.Mutex
	done   chan struct{}

	// Diagnostics from server
	diags   map[string][]Diagnostic // uri → diagnostics
	diagsMu sync.RWMutex

	rootURI  string
	language string
}

// Start launches a language server process and initializes it.
func Start(rootPath, language string) (*Client, error) {
	cmd, err := serverCommand(language)
	if err != nil {
		return nil, err
	}

	absRoot, _ := filepath.Abs(rootPath)

	c := &Client{
		cmd:      cmd,
		respCh:   make(map[int]chan *Response),
		done:     make(chan struct{}),
		diags:    make(map[string][]Diagnostic),
		rootURI:  "file://" + absRoot,
		language: language,
	}

	// Pipe stdio
	c.stdin, err = cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c.stdout = bufio.NewReader(stdoutPipe)

	// Read responses and notifications in background
	go c.readLoop()

	// Initialize
	if _, err := c.initialize(); err != nil {
		c.Close()
		return nil, fmt.Errorf("initialize %s: %w", language, err)
	}

	// Initialized notification
	c.notify("initialized", nil)

	slog.Info("LSP server started", "language", language, "root", absRoot)
	return c, nil
}

func serverCommand(language string) (*exec.Cmd, error) {
	switch language {
	// Systems languages
	case "go":
		return exec.Command("gopls", "serve"), nil
	case "rust":
		return exec.Command("rust-analyzer"), nil
	case "c", "cpp":
		return exec.Command("clangd"), nil
	case "zig":
		return exec.Command("zls"), nil

	// Scripting languages
	case "python":
		return exec.Command("pylsp"), nil
	case "ruby":
		return exec.Command("solargraph", "stdio"), nil
	case "lua":
		return exec.Command("lua-lsp"), nil
	case "php":
		return exec.Command("intelephense", "--stdio"), nil

	// JVM languages
	case "java":
		return exec.Command("jdtls"), nil
	case "kotlin":
		return exec.Command("kotlin-language-server"), nil
	case "scala":
		return exec.Command("metals-v2", "--stdio"), nil

	// Web languages
	case "typescript", "javascript":
		return exec.Command("typescript-language-server", "--stdio"), nil
	case "html", "css":
		return exec.Command("vscode-html-languageserver", "--stdio"), nil

	// .NET
	case "csharp":
		return exec.Command("omnisharp", "-lsp"), nil

	// Apple
	case "swift":
		return exec.Command("sourcekit-lsp"), nil

	default:
		return nil, fmt.Errorf("unsupported language: %s. Install the LSP server and add it here", language)
	}
}

// DetectLanguage guesses the language from file extension.
func DetectLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp", ".hxx":
		return "cpp"
	case ".zig":
		return "zig"

	case ".py", ".pyw":
		return "python"
	case ".rb":
		return "ruby"
	case ".lua":
		return "lua"
	case ".php":
		return "php"

	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".scala":
		return "scala"

	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs":
		return "javascript"
	case ".html", ".htm":
		return "html"
	case ".css", ".scss", ".less":
		return "css"

	case ".cs":
		return "csharp"
	case ".swift":
		return "swift"

	default:
		return ""
	}
}

func (c *Client) initialize() (*InitializeResult, error) {
	params := InitializeParams{
		ProcessID: os.Getpid(),
		RootURI:   c.rootURI,
		Capabilities: ClientCapabilities{
			TextDocument: TextDocumentClientCapabilities{
				Hover:      &struct{}{},
				Definition: &struct{}{},
				References: &struct{}{},
			},
		},
	}
	var result InitializeResult
	if err := c.call("initialize", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ── Public API ──────────────────────────────────────────────

// Hover returns type information at a position.
func (c *Client) Hover(filePath string, line, character int) (*Hover, error) {
	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: toURI(filePath)},
		Position:     Position{Line: line, Character: character},
	}
	var hover Hover
	if err := c.call("textDocument/hover", params, &hover); err != nil {
		return nil, err
	}
	return &hover, nil
}

// Definition returns the definition location of a symbol.
func (c *Client) Definition(filePath string, line, character int) ([]Location, error) {
	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: toURI(filePath)},
		Position:     Position{Line: line, Character: character},
	}
	var locs []Location
	if err := c.call("textDocument/definition", params, &locs); err != nil {
		return nil, err
	}
	return locs, nil
}

// References returns all references to a symbol.
func (c *Client) References(filePath string, line, character int) ([]Location, error) {
	params := ReferenceParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: toURI(filePath)},
			Position:     Position{Line: line, Character: character},
		},
		Context: ReferenceContext{IncludeDeclaration: true},
	}
	var locs []Location
	if err := c.call("textDocument/references", params, &locs); err != nil {
		return nil, err
	}
	return locs, nil
}

// Symbols returns document symbols.
func (c *Client) Symbols(filePath string) ([]DocumentSymbol, error) {
	params := TextDocumentIdentifier{URI: toURI(filePath)}
	var syms []DocumentSymbol
	if err := c.call("textDocument/documentSymbol", params, &syms); err != nil {
		return nil, err
	}
	return syms, nil
}

// OpenDocument notifies the server that a file is open (needed for diagnostics).
func (c *Client) OpenDocument(filePath, text string) {
	params := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        toURI(filePath),
			LanguageID: c.language,
			Version:    1,
			Text:       text,
		},
	}
	c.notify("textDocument/didOpen", params)
}

// Diagnostics returns cached diagnostics for a file.
func (c *Client) Diagnostics(filePath string) []Diagnostic {
	c.diagsMu.RLock()
	defer c.diagsMu.RUnlock()
	return c.diags[toURI(filePath)]
}

// Close shuts down the language server.
func (c *Client) Close() {
	c.notify("shutdown", nil)
	c.notify("exit", nil)
	close(c.done)
	c.cmd.Wait()
}

// ── JSON-RPC Core ──────────────────────────────────────────

func (c *Client) call(method string, params, result interface{}) error {
	id := int(c.nextID.Add(1))

	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	ch := make(chan *Response, 1)
	c.respMu.Lock()
	c.respCh[id] = ch
	c.respMu.Unlock()

	body, _ := json.Marshal(req)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))

	c.mu.Lock()
	c.stdin.Write([]byte(header))
	c.stdin.Write(body)
	c.mu.Unlock()

	resp := <-ch
	if resp.Error != nil {
		return fmt.Errorf("LSP error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	if result != nil && resp.Result != nil {
		return json.Unmarshal(resp.Result, result)
	}
	return nil
}

func (c *Client) notify(method string, params interface{}) {
	n := Notification{JSONRPC: "2.0", Method: method, Params: params}
	body, _ := json.Marshal(n)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	c.mu.Lock()
	c.stdin.Write([]byte(header))
	c.stdin.Write(body)
	c.mu.Unlock()
}

func (c *Client) readLoop() {
	reader := c.stdout
	for {
		select {
		case <-c.done:
			return
		default:
		}
		header, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		header = strings.TrimSpace(header)
		if !strings.HasPrefix(header, "Content-Length:") {
			continue
		}
		var length int
		fmt.Sscanf(header, "Content-Length: %d", &length)
		// Skip empty line
		reader.ReadString('\n')

		body := make([]byte, length)
		_, err = io.ReadFull(reader, body)
		if err != nil {
			return
		}

		// Try response first
		var resp Response
		if json.Unmarshal(body, &resp) == nil && resp.ID > 0 {
			c.respMu.Lock()
			if ch, ok := c.respCh[resp.ID]; ok {
				ch <- &resp
				delete(c.respCh, resp.ID)
			}
			c.respMu.Unlock()
			continue
		}

		// Try notification
		var notif Notification
		if json.Unmarshal(body, &notif) == nil {
			c.handleNotification(&notif)
		}
	}
}

func (c *Client) handleNotification(notif *Notification) {
	switch notif.Method {
	case "textDocument/publishDiagnostics":
		var params PublishDiagnosticsParams
		if data, _ := json.Marshal(notif.Params); len(data) > 0 {
			json.Unmarshal(data, &params)
			c.diagsMu.Lock()
			c.diags[params.URI] = params.Diagnostics
			c.diagsMu.Unlock()
		}
	}
}

func toURI(path string) string {
	abs, _ := filepath.Abs(path)
	return "file://" + abs
}
