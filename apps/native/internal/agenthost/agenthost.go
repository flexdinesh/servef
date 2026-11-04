// Package agenthost composes the real native app (view.App over
// workspace.Model) in a headless agent session (packages/mygo
// agent/session, decision 0086): against fakeservef serving the shared API
// fixtures (testdata/api/default) by default, or a real daemon socket or
// servef server.
package agenthost

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/apps/native/internal/connect"
	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/apps/native/internal/highlight"
	"github.com/flexdinesh/servef/apps/native/internal/link"
	"github.com/flexdinesh/servef/apps/native/internal/view"
	"github.com/flexdinesh/servef/apps/native/internal/workspace"
	"github.com/flexdinesh/servef/packages/mygo/agent/session"
	"github.com/flexdinesh/servef/packages/mygo/ondine"
)

// Options choose what the app connects to.
type Options struct {
	// Fixtures is the API fixture directory the fake daemon serves; empty
	// for testdata/api/default. Ignored with Daemon or Server.
	Fixtures string
	// Daemon is a running servef daemon's socket.
	Daemon string
	// Server is a servef http server's address, as http://localhost:7979.
	Server string
	// Link is a servef-native:// link to open at start.
	Link string
	// ConnectPage starts on the connect page instead of connecting to this
	// computer, as the app does at launch.
	ConnectPage bool
	Width       int
	Height      int
	Dark        bool
	// Warm compiles Highlights and Ondine before the session starts, so
	// steps time the UI, not a first compile (seconds under -race).
	Warm bool
}

// Host is a running headless app.
type Host struct {
	Session *session.Session
	App     *view.App
	fake    *fakeservef.Server
	dir     string
}

// connector reaches the configured daemon; servers by address as the app
// does.
type connector struct{ socket string }

func (c connector) Local(context.Context) (connect.Connection, error) {
	if c.socket == "" {
		return connect.Connection{}, fmt.Errorf("no daemon: start native-agent with --daemon or without --server")
	}
	return connect.Local(c.socket), nil
}
func (connector) Server(target link.Target) connect.Connection { return connect.Server(target) }
func (connector) Servers(context.Context) ([]connect.ListedServer, error) {
	return nil, nil
}

// Start starts the app in a session.
func Start(opts Options) (*Host, error) {
	if opts.Warm {
		if err := highlight.Warm(); err != nil {
			return nil, err
		}
		if err := ondine.Warm(); err != nil {
			return nil, err
		}
	}
	h := &Host{}
	socket := opts.Daemon
	if opts.Daemon == "" && opts.Server == "" {
		dir, err := os.MkdirTemp("", "native-agent")
		if err != nil {
			return nil, err
		}
		h.dir = dir
		fake, err := fakeservef.StartDaemon(filepath.Join(dir, "d.sock"))
		if err != nil {
			h.Close()
			return nil, err
		}
		h.fake = fake
		fixtures := opts.Fixtures
		if fixtures == "" {
			fixtures = fakeservef.FixtureDir()
		}
		if err := fake.LoadFixtures(fixtures); err != nil {
			h.Close()
			return nil, err
		}
		socket = fake.Socket
	}
	var request link.Request
	if opts.Link != "" {
		target, err := link.Parse(opts.Link)
		if err != nil {
			h.Close()
			return nil, err
		}
		request.Link = &target
	}
	var model *workspace.Model
	h.Session = session.New(session.Options{
		Width: opts.Width, Height: opts.Height, Dark: opts.Dark,
		Title: func() string { return model.Title() },
		View: func(s *session.Session) func(*ui.Context) {
			model = workspace.New(s, connector{socket: socket})
			h.App = view.New(model, s)
			switch {
			case opts.Server != "":
				model.ConnectAddress(opts.Server)
			case !opts.ConnectPage || request.Link != nil:
				model.Handle(request)
			}
			model.RefreshServers()
			return h.App.View
		},
	})
	return h, nil
}

// Close stops the session, the app, and the fake daemon.
func (h *Host) Close() {
	if h.Session != nil {
		// The model is the session goroutine's; once the session stopped,
		// nothing else touches it.
		closed := false
		_ = h.Session.Do(func(*ui.Tester) {
			if h.App != nil {
				h.App.Model.Close()
			}
			closed = true
		})
		h.Session.Close()
		if !closed && h.App != nil {
			<-h.Session.Exited()
			h.App.Model.Close()
		}
	}
	if h.fake != nil {
		h.fake.Close()
	}
	if h.dir != "" {
		_ = os.RemoveAll(h.dir)
	}
}
