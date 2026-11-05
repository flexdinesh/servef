// Package runtime composes the servef web application and its HTTP server for
// every transport: the CLI's TCP listener and the daemon's Unix socket.
package runtime

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/flexdinesh/servef/internal/app"
	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/web"
)

// ShutdownTimeout bounds a graceful stop.
const ShutdownTimeout = 5 * time.Second

// How often a Binding's HTTP/2 connections are checked for a live peer, and how
// long it has to answer.
const (
	http2PingEvery   = 30 * time.Second
	http2PingTimeout = 15 * time.Second
)

// Config configures the web application shared by every transport.
type Config struct {
	// Roots is required; it may grow while serving.
	Roots     *projects.RootSet
	Depth     int
	ScanDepth int
	// OpenBrowser opens published walkthroughs that ask; nil never opens.
	OpenBrowser func(url string) error
	// SessionKey keys the browser's session memory; "" keys it by roots.
	SessionKey string
	// Presence makes pages hold /api/presence while open.
	Presence bool
}

// Runtime is the composed web application.
type Runtime struct {
	app *web.App

	mu      sync.Mutex
	bound   map[*Bound]struct{}
	stopped bool
}

// New builds the web app over document.Builtins(); language servers, the
// watch hub, and the walkthrough service use web.New's defaults.
func New(config Config) (*Runtime, error) {
	app, err := web.New(web.Config{Roots: config.Roots, ScanDepth: config.ScanDepth, Depth: config.Depth, Types: document.Builtins(), OpenBrowser: config.OpenBrowser, SessionKey: config.SessionKey, Presence: config.Presence})
	if err != nil {
		return nil, fmt.Errorf("create web interface: %w", err)
	}
	return &Runtime{app: app}, nil
}

// Application is the application layer the web app adapts, for in-process
// clients (pkg/client).
func (r *Runtime) Application() *app.App { return r.app.Application() }

// Handler is the web application (Host check, CSP, routes).
func (r *Runtime) Handler() http.Handler { return r.app }

// Serve serves handler on listener until ctx ends or serving fails. On ctx
// end it shuts down with every Binding: event streams end first, then in-flight
// requests get ShutdownTimeout. It returns nil after a clean stop. listener is
// closed when Serve returns; the app is not.
func (r *Runtime) Serve(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := newServer(handler)
	// Event streams never end on their own; shutdown ends them first.
	server.RegisterOnShutdown(r.app.StopLiveUpdates)
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve(listener)
	}()
	select {
	case err := <-serveErrors:
		r.closeBindings()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve files: %w", err)
		}
		return nil
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()
	var bindings sync.WaitGroup
	for _, extra := range r.stopBinding() {
		bindings.Go(func() {
			_ = extra.server.Shutdown(shutdownCtx)
			<-extra.done
		})
	}
	err := server.Shutdown(shutdownCtx)
	// Serve has closed listener once it returns.
	<-serveErrors
	bindings.Wait()
	if err != nil {
		return fmt.Errorf("stop local server: %w", err)
	}
	return nil
}

// Binding is another way to reach the app while Serve runs: the same app over
// another listener, with its own handler chain.
type Binding struct {
	Listener net.Listener
	Handler  http.Handler
	// TLS serves the listener over TLS, with HTTP/2; nil serves it plain.
	TLS *tls.Config
	// WriteTimeout replaces the default for answers crossing a slow link.
	WriteTimeout time.Duration
}

// Bound is a Binding being served.
type Bound struct {
	runtime *Runtime
	server  *http.Server
	done    chan struct{}
}

// ErrStopped reports a Bind after Serve began stopping.
var ErrStopped = errors.New("servef is stopping")

// Bind starts serving binding, until Close or until Serve stops. It closes the
// listener when it fails.
func (r *Runtime) Bind(binding Binding) (*Bound, error) {
	server := newServer(binding.Handler)
	if binding.WriteTimeout > 0 {
		server.WriteTimeout = binding.WriteTimeout
	}
	server.TLSConfig = binding.TLS
	// A peer that vanished, as a sleeping laptop does, sends no FIN: pings end
	// its streams, so it stops counting as connected.
	server.HTTP2 = &http.HTTP2Config{SendPingTimeout: http2PingEvery, PingTimeout: http2PingTimeout}
	bound := &Bound{runtime: r, server: server, done: make(chan struct{})}
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		_ = binding.Listener.Close()
		return nil, ErrStopped
	}
	if r.bound == nil {
		r.bound = map[*Bound]struct{}{}
	}
	r.bound[bound] = struct{}{}
	r.mu.Unlock()
	go func() {
		defer close(bound.done)
		if binding.TLS != nil {
			_ = server.ServeTLS(binding.Listener, "", "")
			return
		}
		_ = server.Serve(binding.Listener)
	}()
	return bound, nil
}

// Close stops serving at once, ending its connections and event streams, and
// waits for that. The rest of the app keeps running.
func (b *Bound) Close() {
	b.runtime.mu.Lock()
	delete(b.runtime.bound, b)
	b.runtime.mu.Unlock()
	_ = b.server.Close()
	<-b.done
}

// stopBinding refuses new bindings and returns the current ones.
func (r *Runtime) stopBinding() []*Bound {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	bindings := make([]*Bound, 0, len(r.bound))
	for bound := range r.bound {
		bindings = append(bindings, bound)
	}
	r.bound = nil
	return bindings
}

// closeBindings ends every binding when the main listener failed.
func (r *Runtime) closeBindings() {
	for _, extra := range r.stopBinding() {
		_ = extra.server.Close()
		<-extra.done
	}
}

// Close ends event streams and stops language servers. Call after Serve.
func (r *Runtime) Close() { r.app.Close() }

func newServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
