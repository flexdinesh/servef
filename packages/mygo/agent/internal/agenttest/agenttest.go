// Package agenttest hosts the demo counter in a session on a temporary
// socket for tests.
package agenttest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/packages/mygo/agent/client"
	"github.com/flexdinesh/servef/packages/mygo/agent/demo"
	"github.com/flexdinesh/servef/packages/mygo/agent/session"
)

// Start serves a counter of width×height and returns its socket and a
// connected client.
func Start(t testing.TB, width, height int) (string, *client.Client) {
	t.Helper()
	s := session.New(session.Options{Width: width, Height: height, View: func(s *session.Session) func(*ui.Context) {
		return demo.New(s).View
	}})
	t.Cleanup(s.Close)
	// Unix socket paths are short; t.TempDir can be too long.
	dir, err := os.MkdirTemp("", "mygo-agent")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s.sock")
	go func() { _ = s.ListenAndServe(socket) }()
	c, err := client.DialWait(socket, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return socket, c
}
