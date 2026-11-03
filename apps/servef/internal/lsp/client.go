package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"unicode/utf16"
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
	End Position
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
	end     Position
}

type documentFilter struct {
	Language string `json:"language"`
	Scheme   string `json:"scheme"`
	Pattern  string `json:"pattern"`
}

type fileWatcher struct {
	GlobPattern json.RawMessage `json:"globPattern"`
	Kind        *int            `json:"kind"`
}

func (w fileWatcher) pattern() (string, string, error) {
	var pattern string
	if json.Unmarshal(w.GlobPattern, &pattern) == nil {
		return pattern, "", nil
	}
	var relative struct {
		BaseURI json.RawMessage `json:"baseUri"`
		Pattern string          `json:"pattern"`
	}
	if err := json.Unmarshal(w.GlobPattern, &relative); err != nil {
		return "", "", err
	}
	var baseURI string
	if json.Unmarshal(relative.BaseURI, &baseURI) != nil {
		var folder struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(relative.BaseURI, &folder); err != nil {
			return "", "", err
		}
		baseURI = folder.URI
	}
	base, ok := uriPath(baseURI)
	if !ok {
		return "", "", fmt.Errorf("invalid watcher base URI %q", baseURI)
	}
	return relative.Pattern, base, nil
}

type capabilityRegistration struct {
	ID      string `json:"id"`
	Method  string `json:"method"`
	Options struct {
		DocumentSelector []documentFilter `json:"documentSelector"`
		Watchers         []fileWatcher    `json:"watchers"`
	} `json:"registerOptions"`
}

// client is one running server for one project root.
type client struct {
	server  Server
	keyName string
	root    string

	stderr *tail

	// proc guards the process, which stop may end while it still starts.
	proc     sync.Mutex
	cmd      *exec.Cmd
	conn     *conn
	stopped  bool
	stopOnce sync.Once

	queries chan struct{} // serializes document synchronization with its query
	ready   chan struct{} // closed once initialized or failed
	err     error         // set before ready closes, on failure; read after

	capMu                  sync.RWMutex
	definition, references bool
	syncOpen               bool
	syncChange             int
	registrations          map[string]capabilityRegistration

	mu    sync.Mutex
	docs  map[string]*openDocument // by URI
	files map[string]stamp         // last notified stamps, nil before the first sync
}

func newClient(server Server, root string) *client {
	return &client{server: server, root: root, queries: make(chan struct{}, 1), ready: make(chan struct{}), docs: make(map[string]*openDocument), stderr: &tail{}, registrations: make(map[string]capabilityRegistration)}
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
	args := append([]string(nil), c.server.Args...)
	workspace := ""
	if c.server.Name == "jdtls" {
		var err error
		workspace, err = os.MkdirTemp("", "servef-jdtls-")
		if err != nil {
			return fmt.Errorf("create Java workspace: %w", err)
		}
		args = append(args, "-data", workspace)
	}
	cleanup := func() {
		if workspace != "" {
			_ = os.RemoveAll(workspace)
		}
	}
	cmd := exec.Command(path, args...)
	cmd.Dir = c.root
	configureProcess(cmd)
	// Servers exit when servef dies and their stdin closes.
	cmd.Stderr = c.stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cleanup()
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanup()
		return err
	}
	c.proc.Lock()
	if c.stopped {
		c.proc.Unlock()
		_ = stdin.Close()
		_ = stdout.Close()
		cleanup()
		return errClosed
	}
	if err := cmd.Start(); err != nil {
		c.proc.Unlock()
		cleanup()
		return fmt.Errorf("start %s: %w", c.server.Name, err)
	}
	connection := newConn(stdout, stdin, c.answer)
	c.cmd, c.conn = cmd, connection
	c.proc.Unlock()

	go func() {
		err := cmd.Wait()
		cleanup()
		_ = stdin.Close()
		if err == nil {
			err = errClosed
		}
		connection.close(c.explain(err))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), initializeTimeout)
	defer cancel()
	root := fileURI(c.root)
	var result struct {
		Capabilities struct {
			DefinitionProvider json.RawMessage `json:"definitionProvider"`
			ReferencesProvider json.RawMessage `json:"referencesProvider"`
			PositionEncoding   string          `json:"positionEncoding"`
			TextDocumentSync   json.RawMessage `json:"textDocumentSync"`
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
				"definition":      map[string]any{"linkSupport": true, "dynamicRegistration": true},
				"references":      map[string]any{"dynamicRegistration": true},
			},
			"workspace": map[string]any{
				"workspaceFolders":       true,
				"configuration":          true,
				"didChangeWatchedFiles":  map[string]any{"dynamicRegistration": true, "relativePatternSupport": true},
				"workspaceEdit":          map[string]any{"documentChanges": false},
				"didChangeConfiguration": map[string]any{"dynamicRegistration": false},
			},
		},
	}, &result)
	if err != nil {
		return fmt.Errorf("initialize %s: %w", c.server.Name, c.explain(err))
	}
	if encoding := result.Capabilities.PositionEncoding; encoding != "" && encoding != "utf-16" {
		return fmt.Errorf("%s selected unsupported position encoding %q", c.server.Name, encoding)
	}
	c.capMu.Lock()
	c.definition = provides(result.Capabilities.DefinitionProvider)
	c.references = provides(result.Capabilities.ReferencesProvider)
	c.capMu.Unlock()
	if err := c.negotiateSync(result.Capabilities.TextDocumentSync); err != nil {
		return err
	}

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
	case "client/registerCapability":
		var request struct {
			Registrations []capabilityRegistration `json:"registrations"`
		}
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, &rpcError{Code: -32602, Message: err.Error()}
		}
		for _, registration := range request.Registrations {
			switch registration.Method {
			case "textDocument/definition", "textDocument/references", "workspace/didChangeWatchedFiles":
				for _, watcher := range registration.Options.Watchers {
					pattern, _, err := watcher.pattern()
					if err != nil || !doublestar.ValidatePattern(pattern) {
						return nil, &rpcError{Code: -32602, Message: "invalid watcher glob"}
					}
				}
				for _, selector := range registration.Options.DocumentSelector {
					if !doublestar.ValidatePattern(selector.Pattern) {
						return nil, &rpcError{Code: -32602, Message: "invalid document selector glob"}
					}
				}
			default:
				return nil, &rpcError{Code: -32602, Message: "unsupported registration: " + registration.Method}
			}
		}
		c.capMu.Lock()
		for _, registration := range request.Registrations {
			c.registrations[registration.ID] = registration
		}
		c.capMu.Unlock()
		return nil, nil
	case "client/unregisterCapability":
		var request struct {
			Unregistrations []struct {
				ID string `json:"id"`
			} `json:"unregisterations"`
		}
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, &rpcError{Code: -32602, Message: err.Error()}
		}
		c.capMu.Lock()
		for _, registration := range request.Unregistrations {
			delete(c.registrations, registration.ID)
		}
		c.capMu.Unlock()
		return nil, nil
	case "window/workDoneProgress/create", "window/showMessageRequest":
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
		return errors.Join(ErrNotReady, ctx.Err())
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

func (c *client) capabilities(languageID, path string) Capabilities {
	c.capMu.RLock()
	defer c.capMu.RUnlock()
	result := Capabilities{Definition: c.definition, References: c.references}
	for _, registration := range c.registrations {
		if !registration.matches(languageID, path) {
			continue
		}
		method := registration.Method
		if method == "textDocument/definition" {
			result.Definition = true
		}
		if method == "textDocument/references" {
			result.References = true
		}
	}
	return result
}

func (r capabilityRegistration) matches(languageID, path string) bool {
	if r.Options.DocumentSelector == nil {
		return true
	}
	for _, selector := range r.Options.DocumentSelector {
		if selector.Language != "" && selector.Language != languageID {
			continue
		}
		if selector.Scheme != "" && selector.Scheme != "file" {
			continue
		}
		if selector.Pattern != "" && path != "" && !matchPattern(selector.Pattern, filepath.ToSlash(path)) {
			continue
		}
		return true
	}
	return false
}

func matchPattern(pattern, value string) bool {
	ok, err := doublestar.Match(pattern, value)
	return err == nil && ok
}

func (c *client) watches(path string, kind int) bool {
	c.capMu.RLock()
	defer c.capMu.RUnlock()
	for _, registration := range c.registrations {
		if registration.Method != "workspace/didChangeWatchedFiles" {
			continue
		}
		for _, watcher := range registration.Options.Watchers {
			if watcher.Kind != nil && *watcher.Kind&(1<<(kind-1)) == 0 {
				continue
			}
			pattern, base, err := watcher.pattern()
			if err != nil {
				continue
			}
			value := path
			if base != "" {
				value, err = filepath.Rel(base, path)
				if err != nil || value == ".." || strings.HasPrefix(value, ".."+string(filepath.Separator)) {
					continue
				}
			}
			if matchPattern(pattern, filepath.ToSlash(value)) {
				return true
			}
		}
	}
	return false
}

func (c *client) negotiateSync(raw json.RawMessage) error {
	var kind int
	if err := json.Unmarshal(raw, &kind); err == nil {
		if kind < 0 || kind > 2 {
			return fmt.Errorf("unsupported textDocumentSync kind %d", kind)
		}
		c.syncOpen, c.syncChange = kind != 0, kind
		return nil
	}
	var options struct {
		OpenClose bool `json:"openClose"`
		Change    int  `json:"change"`
	}
	if len(raw) != 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if err := json.Unmarshal(raw, &options); err != nil {
			return fmt.Errorf("invalid textDocumentSync: %w", err)
		}
	}
	c.syncOpen, c.syncChange = options.OpenClose, options.Change
	if c.syncChange < 0 || c.syncChange > 2 {
		return fmt.Errorf("unsupported textDocumentSync kind %d", c.syncChange)
	}
	return nil
}

func textEnd(text []byte) Position {
	line, column := 0, 0
	for _, r := range string(text) {
		if r == '\n' {
			line++
			column = 0
		} else {
			column += utf16.RuneLen(r)
		}
	}
	return Position{Line: line, Column: column}
}

// sync keeps the current document overlay fresh. Other changed overlays are
// closed before disk-change notifications, so they cannot mask current files.
func (c *client) sync(document File, languageID string, text []byte, files []File) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	uri := fileURI(document.Path)
	if err := c.watch(files, uri); err != nil {
		return err
	}
	if !c.syncOpen && c.syncChange == 0 {
		return nil
	}
	current := stamp{size: document.Size, modified: document.Modified}
	end := textEnd(text)
	if open, ok := c.docs[uri]; ok {
		open.used = time.Now()
		if open.stamp == current {
			return nil
		}
		oldEnd := open.end
		open.version++
		open.stamp, open.end = current, end
		if c.syncChange == 0 {
			return nil
		}
		change := map[string]any{"text": string(text)}
		if c.syncChange == 2 {
			change["range"] = map[string]any{"start": map[string]int{"line": 0, "character": 0}, "end": map[string]int{"line": oldEnd.Line, "character": oldEnd.Column}}
		}
		return c.connection().notify("textDocument/didChange", map[string]any{
			"textDocument": map[string]any{"uri": uri, "version": open.version}, "contentChanges": []any{change},
		})
	}
	if len(c.docs) >= maxOpenDocuments {
		c.closeOldest()
	}
	c.docs[uri] = &openDocument{version: 1, stamp: current, used: time.Now(), end: end}
	if !c.syncOpen {
		return nil
	}
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
	if c.syncOpen {
		_ = c.connection().notify("textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": oldest}})
	}
}

// watch notifies the server of files created, changed, or deleted since the
// last sync. The first sync records a baseline.
func (c *client) watch(files []File, currentURI string) error {
	next := make(map[string]stamp, len(files))
	for _, file := range files {
		next[file.Path] = stamp{size: file.Size, modified: file.Modified}
	}
	for uri, open := range c.docs {
		if uri == currentURI {
			continue
		}
		path, ok := uriPath(uri)
		if !ok {
			continue
		}
		if current, present := next[path]; !present || current != open.stamp {
			if c.syncOpen {
				if err := c.connection().notify("textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": uri}}); err != nil {
					return err
				}
			}
			delete(c.docs, uri)
		}
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
			if !c.watches(path, created) {
				continue
			}
			changes = append(changes, map[string]any{"uri": fileURI(path), "type": created})
		} else if old != current {
			if !c.watches(path, changed) {
				continue
			}
			changes = append(changes, map[string]any{"uri": fileURI(path), "type": changed})
		}
	}
	for path := range previous {
		if _, ok := next[path]; !ok {
			if !c.watches(path, deleted) {
				continue
			}
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
		var rpc *rpcError
		if errors.As(err, &rpc) && rpc.Code == -32601 {
			return nil, ErrUnsupported
		}
		return nil, err
	}
	return decodeLocations(raw)
}

type lspRange struct {
	Start struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	} `json:"start"`
	End *struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	} `json:"end"`
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
		start := Position{Line: at.Start.Line + 1, Column: at.Start.Character}
		end := start
		if at.End != nil {
			end = Position{Line: at.End.Line + 1, Column: at.End.Character}
		}
		if start.Line < 1 || start.Column < 0 || end.Line < start.Line || end.Column < 0 || (end.Line == start.Line && end.Column < start.Column) {
			continue
		}
		result = append(result, Location{Path: path, Position: start, End: end})
	}
	return result, nil
}

// stop asks the server to exit, then kills it.
func (c *client) stop() { c.stopOnce.Do(c.stopProcess) }

func (c *client) stopProcess() {
	c.proc.Lock()
	c.stopped = true
	connection, cmd := c.conn, c.cmd
	c.proc.Unlock()
	if connection == nil || cmd == nil {
		return
	}
	defer killProcess(cmd)
	select {
	case <-connection.done:
		killProcess(cmd)
		return
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if c.starting() {
		// A handshake in progress cannot be shut down politely.
		killProcess(cmd)
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
		killProcess(cmd)
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
	if err != nil || parsed.Scheme != "file" || (parsed.Host != "" && parsed.Host != "localhost") {
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
