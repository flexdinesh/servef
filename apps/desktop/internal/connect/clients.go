package connect

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/flexdinesh/servef/apps/desktop/internal/notifier"
	"github.com/flexdinesh/servef/pkg/client"
	"github.com/flexdinesh/servef/pkg/connection"
)

// StreamClient returns a client for the long-lived streams of a notifier: the
// same client while the socket stands, since each opens idle connections of its
// own. failed is why the last stream ended; when the daemon has gone, as after
// its idle exit, it ensures again, as the page's requests do.
func (d *Daemon) StreamClient(ctx context.Context, failed error) (client.Client, error) {
	var (
		socket string
		err    error
	)
	if IsGone(failed) {
		socket, err = d.Reensure(ctx)
	} else {
		socket, err = d.Socket(ctx, false)
	}
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.streamClient == nil || d.streamSocket != socket {
		d.streamClient, d.streamSocket = client.NewUnix(socket), socket
	}
	return d.streamClient, nil
}

// Saved looks up a saved connection.
type Saved interface {
	Lookup(id string) (connection.Saved, bool, error)
}

// RemoteClients returns what a notifier connects to a saved server with. It
// rereads the store each time, so `servef connect` with new addresses applies to
// the next attempt, and keeps a client while the connection is unchanged.
func RemoteClients(store Saved, id string) func(context.Context, error) (client.Client, error) {
	var (
		mu      sync.Mutex
		current client.Client
		last    connection.Saved
	)
	return func(context.Context, error) (client.Client, error) {
		saved, found, err := store.Lookup(id)
		if err != nil {
			return nil, fmt.Errorf("read saved connections: %w", err)
		}
		if !found {
			return nil, notifier.ErrNoConnection
		}
		mu.Lock()
		defer mu.Unlock()
		if current != nil && saved.Credential == last.Credential && saved.Port == last.Port && slices.Equal(saved.Hosts, last.Hosts) {
			return current, nil
		}
		endpoint, err := saved.Endpoint()
		if err != nil {
			return nil, err
		}
		current, last = client.NewRemote(endpoint), saved
		return current, nil
	}
}
