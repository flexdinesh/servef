package lsp

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	// DefaultIdle is how long a server may sit unused before it is stopped.
	DefaultIdle = 10 * time.Minute
	// retryAfter keeps a failed server from restarting on every request.
	retryAfter = 30 * time.Second
)

// State is a server's lifecycle state for a project.
type State string

const (
	// StateMissing means the server binary was not found.
	StateMissing State = "missing"
	// StateStopped means the server is installed but not running.
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateReady    State = "ready"
	// StateFailed means the last start failed or the server exited.
	StateFailed State = "failed"
)

// ErrUnsupported reports a request the server does not answer.
var ErrUnsupported = errors.New("language server does not support this request")

// Status describes a family's language server for a project.
type Status struct {
	Server Server
	State  State
	// Message explains a failure.
	Message string
	// Memory is the running server's resident memory in bytes, when known.
	Memory uint64
}

// Options configure a Manager.
type Options struct {
	// Idle stops servers unused this long; zero means DefaultIdle.
	Idle time.Duration
	// LookPath finds a server binary; nil searches PATH and install dirs.
	LookPath func(name string) (string, bool)
}

type clientKey struct {
	server string
	root   string
}

type failure struct {
	at      time.Time
	message string
}

// Manager runs language servers on demand: one per server and project
// root, started by the first request and stopped after Idle.
type Manager struct {
	servers  []Server
	idle     time.Duration
	lookPath func(string) (string, bool)

	mu       sync.Mutex
	found    map[string]string // server name to binary, "" when missing
	clients  map[clientKey]*client
	timers   map[clientKey]*time.Timer
	failures map[clientKey]failure
	closed   bool
}

// NewManager creates a Manager for servers.
func NewManager(servers []Server, options Options) *Manager {
	if options.Idle <= 0 {
		options.Idle = DefaultIdle
	}
	if options.LookPath == nil {
		options.LookPath = lookPath
	}
	return &Manager{
		servers:  servers,
		idle:     options.Idle,
		lookPath: options.LookPath,
		found:    make(map[string]string),
		clients:  make(map[clientKey]*client),
		timers:   make(map[clientKey]*time.Timer),
		failures: make(map[clientKey]failure),
	}
}

// Servers lists the registered servers.
func (m *Manager) Servers() []Server { return m.servers }

// ServerFor finds the server for a document family.
func (m *Manager) ServerFor(family string) (Server, bool) {
	for _, server := range m.servers {
		if _, ok := server.Families[family]; ok {
			return server, true
		}
	}
	return Server{}, false
}

// Refresh forgets discovered binaries and failures, so newly installed
// servers are found and failed ones retried.
func (m *Manager) Refresh() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.found = make(map[string]string)
	m.failures = make(map[clientKey]failure)
}

func (m *Manager) binary(server Server) (string, bool) {
	if path, ok := m.found[server.Name]; ok {
		return path, path != ""
	}
	path, ok := m.lookPath(server.Name)
	if !ok {
		path = ""
	}
	m.found[server.Name] = path
	return path, ok
}

// Status reports a family's server for a project root without starting it.
func (m *Manager) Status(root, family string) (Status, bool) {
	server, ok := m.ServerFor(family)
	if !ok {
		return Status{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	status := Status{Server: server, State: StateStopped}
	key := clientKey{server: server.Name, root: root}
	if _, installed := m.binary(server); !installed {
		status.State = StateMissing
		return status, true
	}
	if failed, ok := m.failures[key]; ok {
		status.State, status.Message = StateFailed, failed.message
	}
	if c, ok := m.clients[key]; ok {
		switch {
		case c.starting():
			status.State, status.Message = StateStarting, ""
		case c.alive():
			status.State, status.Message = StateReady, ""
			status.Memory, _ = treeMemory(c.pid())
		}
	}
	return status, true
}

// Available reports whether a family's server is installed and not in its
// failure backoff, so requests may wait for it.
func (m *Manager) Available(root, family string) bool {
	status, ok := m.Status(root, family)
	return ok && status.State != StateMissing && status.State != StateFailed
}

// acquire returns the running client for a family and root, starting one.
func (m *Manager) acquire(root, family string) (*client, string, error) {
	server, ok := m.ServerFor(family)
	if !ok {
		return nil, "", ErrUnsupported
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, "", errClosed
	}
	key := clientKey{server: server.Name, root: root}
	if c, ok := m.clients[key]; ok {
		if c.failed() == nil {
			m.touch(key)
			return c, server.Families[family], nil
		}
		m.dropLocked(key, c)
	}
	if failed, ok := m.failures[key]; ok && time.Since(failed.at) < retryAfter {
		return nil, "", errors.New(failed.message)
	}
	path, installed := m.binary(server)
	if !installed {
		return nil, "", ErrMissing
	}
	c := newClient(server, root)
	m.clients[key] = c
	delete(m.failures, key)
	c.start(path)
	m.touch(key)
	return c, server.Families[family], nil
}

// ErrMissing reports a server that is not installed.
var ErrMissing = errors.New("language server is not installed")

// touch restarts a client's idle timer; m.mu is held.
func (m *Manager) touch(key clientKey) {
	if timer, ok := m.timers[key]; ok {
		timer.Reset(m.idle)
		return
	}
	m.timers[key] = time.AfterFunc(m.idle, func() { m.expire(key) })
}

func (m *Manager) expire(key clientKey) {
	m.mu.Lock()
	c, ok := m.clients[key]
	if ok {
		delete(m.clients, key)
	}
	delete(m.timers, key)
	m.mu.Unlock()
	if ok {
		c.stop()
	}
}

// dropLocked forgets a client that failed or exited; m.mu is held.
func (m *Manager) dropLocked(key clientKey, c *client) {
	delete(m.clients, key)
	if timer, ok := m.timers[key]; ok {
		timer.Stop()
		delete(m.timers, key)
	}
	if err := c.failed(); err != nil {
		m.failures[key] = failure{at: time.Now(), message: err.Error()}
	}
	go c.stop()
}

// Start starts a family's server for a root if needed and waits until it is
// ready or ctx ends, returning ErrNotReady then.
func (m *Manager) Start(ctx context.Context, root, family string) error {
	c, _, err := m.acquire(root, family)
	if err != nil {
		return err
	}
	if err := c.wait(ctx); err != nil {
		if !errors.Is(err, ErrNotReady) {
			m.fail(root, c)
		}
		return err
	}
	return nil
}

// Idle is how long servers run unused before stopping.
func (m *Manager) Idle() time.Duration { return m.idle }

// Request is one navigation query against a language server.
type Request struct {
	// Root is the project root; one server runs per root.
	Root   string
	Family string
	// Document is the queried file and Text its contents.
	Document File
	Text     []byte
	At       Position
	// Files are the project's files of the server's families, so the server
	// learns about changes on disk.
	Files []File
}

// Definition asks the family's server where the symbol at a position is
// defined. It waits for a starting server until ctx ends, then returns
// ErrNotReady.
func (m *Manager) Definition(ctx context.Context, request Request) ([]Location, error) {
	return m.query(ctx, request, "textDocument/definition", nil, func(c *client) bool { return c.definition })
}

// References asks the family's server for every use of the symbol at a
// position, including its declaration.
func (m *Manager) References(ctx context.Context, request Request) ([]Location, error) {
	return m.query(ctx, request, "textDocument/references",
		map[string]any{"context": map[string]bool{"includeDeclaration": true}},
		func(c *client) bool { return c.references })
}

func (m *Manager) query(ctx context.Context, request Request, method string, extra map[string]any, supported func(*client) bool) ([]Location, error) {
	c, languageID, err := m.acquire(request.Root, request.Family)
	if err != nil {
		return nil, err
	}
	if err := c.wait(ctx); err != nil {
		if !errors.Is(err, ErrNotReady) {
			m.fail(request.Root, c)
		}
		return nil, err
	}
	if !supported(c) {
		return nil, ErrUnsupported
	}
	if err := c.sync(request.Document, languageID, request.Text, request.Files); err != nil {
		m.fail(request.Root, c)
		return nil, err
	}
	locations, err := c.locate(ctx, method, request.Document.Path, request.At, extra)
	if err != nil && c.failed() != nil {
		m.fail(request.Root, c)
	}
	return locations, err
}

func (m *Manager) fail(root string, c *client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := clientKey{server: c.server.Name, root: root}
	if m.clients[key] == c {
		m.dropLocked(key, c)
	}
}

// Close stops every server.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	clients := make([]*client, 0, len(m.clients))
	for key, c := range m.clients {
		clients = append(clients, c)
		if timer, ok := m.timers[key]; ok {
			timer.Stop()
		}
	}
	m.clients = make(map[clientKey]*client)
	m.timers = make(map[clientKey]*time.Timer)
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.stop()
		}()
	}
	wg.Wait()
}
