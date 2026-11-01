// Package runtime composes the servef web application and its HTTP server for
// every transport: the CLI's TCP listener and the daemon's Unix socket.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/web"
)

// ShutdownTimeout bounds a graceful stop.
const ShutdownTimeout = 5 * time.Second

// Config configures the web application shared by every transport.
type Config struct {
	// Roots is required; it may grow while serving.
	Roots     *projects.RootSet
	Depth     int
	ScanDepth int
	// OpenBrowser opens published walkthroughs that ask; nil never opens.
	OpenBrowser func(url string) error
}

// Runtime is the composed web application.
type Runtime struct{ app *web.App }

// New builds the web app over document.Builtins(); language servers, the
// watch hub, and the walkthrough service use web.New's defaults.
func New(config Config) (*Runtime, error) {
	app, err := web.New(web.Config{Roots: config.Roots, ScanDepth: config.ScanDepth, Depth: config.Depth, Types: document.Builtins(), OpenBrowser: config.OpenBrowser})
	if err != nil {
		return nil, fmt.Errorf("create web interface: %w", err)
	}
	return &Runtime{app: app}, nil
}

// Handler is the web application (Host check, CSP, routes).
func (r *Runtime) Handler() http.Handler { return r.app }

// Serve serves handler on listener until ctx ends or serving fails. On ctx
// end it shuts down: event streams end first, then in-flight requests get
// ShutdownTimeout. It returns nil after a clean stop. listener is closed when
// Serve returns; the app is not.
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
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve files: %w", err)
		}
		return nil
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()
	err := server.Shutdown(shutdownCtx)
	// Serve has closed listener once it returns.
	<-serveErrors
	if err != nil {
		return fmt.Errorf("stop local server: %w", err)
	}
	return nil
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
