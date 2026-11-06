package connect

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/flexdinesh/srcz/apps/desktop/internal/notifier"
	"github.com/flexdinesh/srcz/pkg/client"
	"github.com/flexdinesh/srcz/pkg/connection"
)

func TestStreamClientKeepsItsClientAndEnsuresWhenTheDaemonHasGone(t *testing.T) {
	socket := "/run/one.sock"
	ensures := 0
	daemon := NewDaemon("srcz", nil, nil, func(context.Context, string, ...string) ([]byte, []byte, error) {
		ensures++
		return []byte(fmt.Sprintf(`{"protocol":1,"version":"x","pid":1,"socket":%q,"roots":[],"depth":0,"scanDepth":0,"idleExit":"30m0s","startedAt":"2026-11-03T10:00:00Z"}`, socket)), nil, nil
	})
	first, err := daemon.StreamClient(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	again, err := daemon.StreamClient(context.Background(), errors.New("the stream was reset"))
	if err != nil {
		t.Fatal(err)
	}
	if first != again || ensures != 1 {
		t.Errorf("another failure: same client %v, %d ensures", first == again, ensures)
	}

	socket = "/run/two.sock"
	gone, err := daemon.StreamClient(context.Background(), goneError(t))
	if err != nil {
		t.Fatal(err)
	}
	if gone == first || ensures != 2 {
		t.Errorf("a gone daemon: same client %v, %d ensures", gone == first, ensures)
	}
}

// goneError is what a request answers when the daemon's socket is dead.
func goneError(t *testing.T) error {
	t.Helper()
	dead := client.NewUnix(t.TempDir() + "/dead.sock")
	_, err := dead.DaemonStatus(context.Background())
	if !IsGone(err) {
		t.Fatalf("a dead socket's error %v is not gone", err)
	}
	return err
}

type savedConnections map[string]connection.Saved

func (s savedConnections) Lookup(id string) (connection.Saved, bool, error) {
	saved, ok := s[id]
	return saved, ok, nil
}

func TestRemoteClientsFollowTheSavedConnection(t *testing.T) {
	const id = "r-0123456789ab"
	credential := connection.Credential{Fingerprint: [connection.FingerprintSize]byte{1}, Token: [connection.TokenSize]byte{2}}
	saved := connection.Saved{ID: id, Hosts: []string{"127.0.0.1"}, Port: 8443, Credential: credential.Encode()}
	store := savedConnections{id: saved}
	connect := RemoteClients(store, id)

	first, err := connect(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := connect(context.Background(), errors.New("the stream was reset")); err != nil || again != first {
		t.Errorf("an unchanged connection: same client %v, %v", again == first, err)
	}

	// `srcz connect` with a new address applies to the next attempt.
	saved.Hosts = []string{"10.0.0.2"}
	store[id] = saved
	moved, err := connect(context.Background(), nil)
	if err != nil || moved == first {
		t.Errorf("a changed connection: same client %v, %v", moved == first, err)
	}

	delete(store, id)
	if _, err := connect(context.Background(), nil); !errors.Is(err, notifier.ErrNoConnection) {
		t.Errorf("a forgotten connection: %v", err)
	}
}
