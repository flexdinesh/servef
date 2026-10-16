package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// initializeTimeout bounds a server's startup handshake.
	initializeTimeout = 30 * time.Second
	// maxOpenDocuments bounds documents kept open on a server.
	maxOpenDocuments = 64
	// stderrTail is how much server stderr is kept to explain a failure.
	stderrTail = 2 << 10
)

// Position is a 1-based line and a 0-based UTF-16 column, as servef counts.
type Position struct {
	Line   int
	Column int
}

// Location is a position in a file, by absolute path.
type Location struct {
	Path string
	Position
}

// File is a project file and its change stamp, for change notifications.
type File struct {
	Path     string
	Size     int64
	Modified time.Time
}

type stamp struct {
	size     int64
	modified time.Time
}

type openDocument struct {
	version int
	stamp   stamp
	used    time.Time
}

// client is one running server for one project root.
type client struct {
	server Server
	root   string

	stderr *tail

	// proc guards the process, which stop may end while it still starts.
	proc    sync.Mutex
	cmd     *exec.Cmd
	conn    *conn
	stopped bool

	ready chan struct{} // closed once initialized or failed
	err   error         // set before ready closes, on failure; read after

	definition, references bool

	mu    sync.Mutex
	docs  map[string]*openDocument // by URI
	files map[string]stamp         // last notified stamps, nil before the first sync
}

func newClient(server Server, root string) *client {
	return &client{server: server, root: root, ready: make(chan struct{}), docs: make(map[string]*openDocument), stderr: &tail{}}
}

// start launches the server and runs the handshake in the background.
func (c *client) start(path string) {
	go func() {
		err := c.launch(path)
		if err != nil {
			c.err = err
			c.stop()
		}
		close(c.ready)
	}()
}

func (c *client) launch(path string) error {
	cmd := exec.Command(path, c.server.Args...)
	cmd.Dir = c.root
	// Servers exit when servef dies and their stdin closes.
	cmd.Stderr = c.stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", c.server.Name, err)
	}
	connection := newConn(stdout, stdin, c.answer)
	c.proc.Lock()
	stopped := c.stopped
	c.cmd, c.conn = cmd, connection
	c.proc.Unlock()
	go func() {
		err := cmd.Wait()
		_ = stdin.Close()
		if err == nil {
			err = errClosed
		}
		connection.close(c.explain(err))
	}()
	if stopped {
		_ = cmd.Process.Kill()
		return errClosed
	}

	ctx, cancel := context.WithTimeout(context.Background(), initializeTimeout)
	defer cancel()
	root := fileURI(c.root)
	var result struct {
		Capabilities struct {
			DefinitionProvider json.RawMessage `json:"definitionProvider"`
			ReferencesProvider json.RawMessage `json:"referencesProvider"`
		} `json:"capabilities"`
	}
	err = connection.call(ctx, "initialize", map[string]any{
		"processId":        nil,
		"clientInfo":       map[string]string{"name": "servef"},
		"rootUri":          root,
		"workspaceFolders": []map[string]string{{"uri": root, "name": filepath.Base(c.root)}},
		"capabilities": map[string]any{
			"general": map[string]any{"positionEncodings": []string{"utf-16"}},
			"textDocument": map[string]any{
				"synchronization": map[string]any{"didSave": false},
				"definition":      map[string]any{"linkSupport": true},
				"references":      map[string]any{},
			},
			"workspace": map[string]any{
				"workspaceFolders":       true,
				"configuration":          true,
				"didChangeWatchedFiles":  map[string]any{"dynamicRegistration": false},
				"workspaceEdit":          map[string]any{"documentChanges": false},
				"didChangeConfiguration": map[string]any{"dynamicRegistration": false},
			},
		},
	}, &result)
	if err != nil {
		return fmt.Errorf("initialize %s: %w", c.server.Name, c.explain(err))
	}
	c.definition = provides(result.Capabilities.DefinitionProvider)
	c.references = provides(result.Capabilities.ReferencesProvider)
	return connection.notify("initialized", map[string]any{})
}

// provides reads a capability that is a boolean or an options object.
func provides(capability json.RawMessage) bool {
	trimmed := bytes.TrimSpace(capability)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && !bytes.Equal(trimmed, []byte("false"))
}

// answer replies to requests servers send clients.
func (c *client) answer(method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "workspace/configuration":
		var request struct {
			Items []json.RawMessage `json:"items"`
		}
		_ = json.Unmarshal(params, &request)
		return make([]any, len(request.Items)), nil
	case "workspace/workspaceFolders":
		return []map[string]string{{"uri": fileURI(c.root), "name": filepath.Base(c.root)}}, nil
	case "client/registerCapability", "client/unregisterCapability", "window/workDoneProgress/create", "window/showMessageRequest":
		return nil, nil
	}
	return nil, &rpcError{Code: -32601, Message: "method not supported by servef: " + method}
}

// explain adds the server's last stderr output to an error.
func (c *client) explain(err error) error {
	if text := strings.TrimSpace(c.stderr.String()); text != "" {
		lines := strings.Split(text, "\n")
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(lines[len(lines)-1]))
	}
	return err
}

// wait blocks until the client is ready or ctx ends.
func (c *client) wait(ctx context.Context) error {
	select {
	case <-c.ready:
		return c.err
	case <-ctx.Done():
		return ErrNotReady
	}
}

func (c *client) starting() bool {
	select {
	case <-c.ready:
		return false
	default:
		return true
	}
}

// failed reports why a started client cannot serve: its start failed or its
// server exited. A client still starting has not failed.
func (c *client) failed() error {
	if c.starting() {
		return nil
	}
	if c.err != nil {
		return c.err
	}
	return c.exitError()
}

// alive reports a ready client whose server has not exited.
func (c *client) alive() bool {
	return !c.starting() && c.failed() == nil
}

func (c *client) connection() *conn {
	c.proc.Lock()
	defer c.proc.Unlock()
	return c.conn
}

func (c *client) exitError() error {
	connection := c.connection()
	if connection == nil {
		return nil
	}
	select {
	case <-connection.done:
		return connection.err
	default:
		return nil
	}
}

// sync opens or updates the queried document and reports changed project
// files, since servef never edits them and servers do not watch disks.
func (c *client) sync(document File, languageID string, text []byte, files []File) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.watch(files); err != nil {
		return err
	}
	uri := fileURI(document.Path)
	current := stamp{size: document.Size, modified: document.Modified}
	if open, ok := c.docs[uri]; ok {
		open.used = time.Now()
		if open.stamp == current {
			return nil
		}
		open.version++
		open.stamp = current
		return c.connection().notify("textDocument/didChange", map[string]any{
			"textDocument":   map[string]any{"uri": uri, "version": open.version},
			"contentChanges": []map[string]string{{"text": string(text)}},
		})
	}
	if len(c.docs) >= maxOpenDocuments {
		c.closeOldest()
	}
	c.docs[uri] = &openDocument{version: 1, stamp: current, used: time.Now()}
	return c.connection().notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": languageID, "version": 1, "text": string(text)},
	})
}

func (c *client) closeOldest() {
	var oldest string
	var at time.Time
	for uri, open := range c.docs {
		if oldest == "" || open.used.Before(at) {
			oldest, at = uri, open.used
		}
	}
	delete(c.docs, oldest)
	_ = c.connection().notify("textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": oldest}})
}

// watch notifies the server of files created, changed, or deleted since the
// last sync. The first sync records a baseline.
func (c *client) watch(files []File) error {
	next := make(map[string]stamp, len(files))
	for _, file := range files {
		next[file.Path] = stamp{size: file.Size, modified: file.Modified}
	}
	previous := c.files
	c.files = next
	if previous == nil {
		return nil
	}
	const created, changed, deleted = 1, 2, 3
	var changes []map[string]any
	for path, current := range next {
		if old, ok := previous[path]; !ok {
			changes = append(changes, map[string]any{"uri": fileURI(path), "type": created})
		} else if old != current {
			changes = append(changes, map[string]any{"uri": fileURI(path), "type": changed})
		}
	}
	for path := range previous {
		if _, ok := next[path]; !ok {
			changes = append(changes, map[string]any{"uri": fileURI(path), "type": deleted})
		}
	}
	if len(changes) == 0 {
		return nil
	}
	return c.connection().notify("workspace/didChangeWatchedFiles", map[string]any{"changes": changes})
}

func (c *client) locate(ctx context.Context, method string, path string, at Position, extra map[string]any) ([]Location, error) {
	params := map[string]any{
		"textDocument": map[string]string{"uri": fileURI(path)},
		"position":     map[string]int{"line": at.Line - 1, "character": at.Column},
	}
	for key, value := range extra {
		params[key] = value
	}
	var raw json.RawMessage
	if err := c.connection().call(ctx, method, params, &raw); err != nil {
		return nil, err
	}
	return decodeLocations(raw)
}

type lspRange struct {
	Start struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	} `json:"start"`
}

type lspLocation struct {
	URI                  string    `json:"uri"`
	Range                *lspRange `json:"range"`
	TargetURI            string    `json:"targetUri"`
	TargetSelectionRange *lspRange `json:"targetSelectionRange"`
	TargetRange          *lspRange `json:"targetRange"`
}

// decodeLocations accepts null, a Location, a Location array, or a
// LocationLink array.
func decodeLocations(raw json.RawMessage) ([]Location, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	var many []lspLocation
	if trimmed[0] == '{' {
		var one lspLocation
		if err := json.Unmarshal(trimmed, &one); err != nil {
			return nil, err
		}
		many = []lspLocation{one}
	} else if err := json.Unmarshal(trimmed, &many); err != nil {
		return nil, err
	}
	result := make([]Location, 0, len(many))
	for _, item := range many {
		uri, at := item.URI, item.Range
		if item.TargetURI != "" {
			uri, at = item.TargetURI, item.TargetSelectionRange
			if at == nil {
				at = item.TargetRange
			}
		}
		path, ok := uriPath(uri)
		if !ok || at == nil {
			continue
		}
		result = append(result, Location{Path: path, Position: Position{Line: at.Start.Line + 1, Column: at.Start.Character}})
	}
	return result, nil
}

// stop asks the server to exit, then kills it.
func (c *client) stop() {
	c.proc.Lock()
	c.stopped = true
	connection, cmd := c.conn, c.cmd
	c.proc.Unlock()
	if connection == nil || cmd == nil {
		return
	}
	select {
	case <-connection.done:
		return
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if c.starting() {
		// A handshake in progress cannot be shut down politely.
		_ = cmd.Process.Kill()
		return
	}
	if connection.call(ctx, "shutdown", nil, nil) == nil {
		_ = connection.notify("exit", nil)
	}
	select {
	case <-connection.done:
	case <-ctx.Done():
	}
	select {
	case <-connection.done:
	default:
		_ = cmd.Process.Kill()
	}
}

func (c *client) pid() int {
	c.proc.Lock()
	defer c.proc.Unlock()
	if c.cmd == nil || c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

func fileURI(path string) string {
	slashed := filepath.ToSlash(path)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed // Windows drive paths
	}
	return (&url.URL{Scheme: "file", Path: slashed}).String()
}

func uriPath(uri string) (string, bool) {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" {
		return "", false
	}
	path := parsed.Path
	if len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:] // Windows drive paths
	}
	return filepath.FromSlash(path), true
}

// tail keeps the last stderrTail bytes written to it.
type tail struct {
	mu   sync.Mutex
	data []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data = append(t.data, p...)
	if len(t.data) > stderrTail {
		t.data = t.data[len(t.data)-stderrTail:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.data)
}

var _ io.Writer = (*tail)(nil)

// ErrNotReady reports a server still starting when a request gave up.
var ErrNotReady = errors.New("language server is starting")
