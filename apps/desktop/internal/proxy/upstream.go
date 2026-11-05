package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/flexdinesh/servef/apps/desktop/internal/connect"
	"github.com/flexdinesh/servef/pkg/connection"
)

// Failure is a request that got no answer: JSON for the page's requests, a
// page of the app's own for a page load.
type Failure struct {
	Status      int
	Code        string
	Title       string
	Detail      string
	jsonMessage string
}

func (f *Failure) Error() string { return f.Detail }

// message is what a page's request reads.
func (f *Failure) message() string {
	if f.jsonMessage != "" {
		return f.jsonMessage
	}
	return f.Detail
}

// Upstream is the server of one connection.
type Upstream interface {
	// Send makes the request. document says a person asked for a page, so a
	// recent failure is worth retrying now. Failures are *Failure.
	Send(r *http.Request, body []byte, document bool) (*http.Response, error)
	Close()
}

// Resolver finds the server behind a scheme host: one connection per origin.
type Resolver interface {
	// Resolve returns host's server; a failure means it has none.
	Resolve(host string, document bool) (Upstream, *Failure)
	Close()
}

// Store is the saved connections.
type Store interface {
	Lookup(id string) (connection.Saved, bool, error)
}

// Connections resolves "local" to the daemon and connection ids to saved
// remote servers. A page load reads the store again, so running `servef
// connect` with new addresses applies to a window already open.
type Connections struct {
	local *Local
	store Store

	mu      sync.Mutex
	remotes map[string]*Remote
}

// NewConnections returns the app's connections.
func NewConnections(daemon Daemon, store Store) *Connections {
	return &Connections{local: NewLocal(daemon), store: store, remotes: map[string]*Remote{}}
}

// Resolve implements Resolver.
func (c *Connections) Resolve(host string, document bool) (Upstream, *Failure) {
	if host == Host {
		return c.local, nil
	}
	if !connection.IsID(host) {
		return nil, notPaired()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	existing := c.remotes[host]
	if existing != nil && !document {
		return existing, nil
	}
	saved, found, err := c.store.Lookup(host)
	if err != nil {
		if existing != nil {
			return existing, nil
		}
		return nil, &Failure{Status: http.StatusServiceUnavailable, Code: "connectionsUnavailable", Title: "servef can't read its saved connections", Detail: err.Error()}
	}
	if !found {
		if existing != nil {
			existing.Close()
			delete(c.remotes, host)
		}
		return nil, notPaired()
	}
	if existing != nil && existing.matches(saved) {
		return existing, nil
	}
	created, err := newRemote(saved)
	if err != nil {
		return nil, &Failure{Status: http.StatusServiceUnavailable, Code: "connectionsUnavailable", Title: "servef can't read this saved connection", Detail: err.Error()}
	}
	if existing != nil {
		existing.Close()
	}
	c.remotes[host] = created
	return created, nil
}

// Label names a saved connection for people; "" when there is none.
func (c *Connections) Label(host string) string {
	saved, found, err := c.store.Lookup(host)
	if err != nil || !found {
		return ""
	}
	return saved.Label
}

// Known reports a host pages can load from: the daemon, or a saved connection.
func (c *Connections) Known(host string) bool {
	if host == Host {
		return true
	}
	if !connection.IsID(host) {
		return false
	}
	_, found, err := c.store.Lookup(host)
	return err == nil && found
}

// Close drops idle connections to every server.
func (c *Connections) Close() {
	c.local.Close()
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, remote := range c.remotes {
		remote.Close()
	}
}

func notPaired() *Failure {
	return &Failure{
		Status: http.StatusNotFound,
		Code:   "unknownConnection",
		Title:  "This computer has no such servef connection",
		Detail: "Run `servef connect` with the string `servef <path> --remote` prints on the server.",
	}
}

// Daemon is where the local connection's requests go: the ensured daemon's
// socket.
type Daemon interface {
	// Socket returns the socket, ensuring when needed; force retries now
	// after a recent failure.
	Socket(ctx context.Context, force bool) (string, error)
	// Reensure ensures again after the daemon stopped answering.
	Reensure(ctx context.Context) (string, error)
}

// Local is the per-user daemon, started again when it has gone.
type Local struct {
	daemon    Daemon
	transport *http.Transport

	mu     sync.Mutex
	socket string
}

// NewLocal returns the local connection.
func NewLocal(daemon Daemon) *Local {
	l := &Local{daemon: daemon}
	l.transport = &http.Transport{
		// The socket can change when the daemon restarts.
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", l.currentSocket())
		},
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
		// Bytes pass through as the daemon sent them.
		DisableCompression: true,
	}
	return l
}

// Close implements Upstream.
func (l *Local) Close() { l.transport.CloseIdleConnections() }

func (l *Local) currentSocket() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.socket
}

func (l *Local) setSocket(socket string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.socket = socket
}

// Send implements Upstream. A page load retries at once after a failure.
func (l *Local) Send(r *http.Request, body []byte, document bool) (*http.Response, error) {
	socket, err := l.daemon.Socket(r.Context(), document)
	if err != nil {
		return nil, daemonFailure(err)
	}
	l.setSocket(socket)
	response, err := l.send(r, body)
	if err != nil && connect.IsGone(err) && r.Context().Err() == nil {
		// One re-ensure, and a retry for reads only (decision 0067).
		socket, ensureErr := l.daemon.Reensure(r.Context())
		if ensureErr != nil {
			return nil, daemonFailure(ensureErr)
		}
		l.setSocket(socket)
		if !readOnly(r) {
			return nil, &Failure{Status: http.StatusServiceUnavailable, Code: "daemonRestarted", Title: "servef restarted its daemon", Detail: "The servef daemon restarted; the request was not sent. Try again."}
		}
		response, err = l.send(r, nil)
	}
	if err != nil {
		return nil, daemonFailure(err)
	}
	return response, nil
}

func (l *Local) send(r *http.Request, body []byte) (*http.Response, error) {
	upstream, err := upstreamRequest(r, connection.Unix{}.BaseURL(), body)
	if err != nil {
		return nil, err
	}
	// Keeps the daemon's control API refusing whatever arrives through a proxy.
	upstream.Header.Set("Sec-Fetch-Site", "same-origin")
	return l.transport.RoundTrip(upstream)
}

func daemonFailure(err error) *Failure {
	return &Failure{Status: http.StatusServiceUnavailable, Code: "daemonUnavailable", Title: "servef can't reach its daemon", Detail: err.Error()}
}

// Remote is a saved connection to a server over pinned TLS. The page never
// sees the credential: only this transport adds it.
type Remote struct {
	saved     connection.Saved
	base      string
	transport http.RoundTripper
}

func newRemote(saved connection.Saved) (*Remote, error) {
	endpoint, err := saved.Endpoint()
	if err != nil {
		return nil, err
	}
	return &Remote{saved: saved, base: endpoint.BaseURL(), transport: endpoint.RoundTripper()}, nil
}

func (r *Remote) matches(saved connection.Saved) bool {
	return r.saved.Credential == saved.Credential && r.saved.Port == saved.Port && r.saved.Label == saved.Label && slices.Equal(r.saved.Hosts, saved.Hosts)
}

// Close implements Upstream.
func (r *Remote) Close() {
	if closer, ok := r.transport.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// Send implements Upstream.
func (r *Remote) Send(request *http.Request, body []byte, document bool) (*http.Response, error) {
	upstream, err := upstreamRequest(request, r.base, body)
	if err != nil {
		return nil, r.unreachable(err)
	}
	response, err := r.transport.RoundTrip(upstream)
	if errors.Is(err, connection.ErrFingerprint) {
		return nil, &Failure{
			Status: http.StatusBadGateway,
			Code:   "remoteIdentityChanged",
			Title:  r.saved.Label + " has a different identity",
			Detail: "Its credential was rotated, or something else answers at this address. Run `servef <path> --remote` there, then `servef connect` with the new string.",
		}
	}
	if err != nil {
		return nil, r.unreachable(err)
	}
	if response.StatusCode == http.StatusUnauthorized && document {
		_ = response.Body.Close()
		return nil, &Failure{
			Status: http.StatusUnauthorized,
			Code:   "remoteRefused",
			Title:  r.saved.Label + " refused this connection",
			Detail: "Run `servef <path> --remote` there, then `servef connect` with the new string.",
		}
	}
	return response, nil
}

func (r *Remote) unreachable(err error) *Failure {
	return &Failure{
		Status:      http.StatusServiceUnavailable,
		Code:        "remoteUnavailable",
		Title:       "servef can't reach " + r.saved.Label,
		Detail:      "servef stops on a server when no UI needs it. Run `servef <path> --remote` there, then try again.",
		jsonMessage: fmt.Sprintf("servef can't reach %s: %v", r.saved.Label, err),
	}
}

// upstreamRequest is the request a server sees: the allowlisted headers, and
// the Host its check expects. A page's own Cookie or Authorization never pass.
func upstreamRequest(r *http.Request, base string, body []byte) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	upstream, err := http.NewRequestWithContext(r.Context(), r.Method, base+r.URL.RequestURI(), reader)
	if err != nil {
		return nil, err
	}
	for _, name := range forwardedRequestHeaders {
		if values, ok := r.Header[name]; ok {
			upstream.Header[name] = values
		}
	}
	upstream.Host = "localhost"
	return upstream, nil
}

func readOnly(r *http.Request) bool {
	return r.Method == http.MethodGet || r.Method == http.MethodHead
}
