package connect

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/srcz/pkg/client"
)

func TestDaemonEnsuresOnceAndAgainWhenAsked(t *testing.T) {
	ensures := 0
	daemon := NewDaemon("srcz", nil, nil, func(context.Context, string, ...string) ([]byte, []byte, error) {
		ensures++
		return []byte(status), nil, nil
	})
	for range 2 {
		if socket, err := daemon.Socket(context.Background(), false); err != nil || socket != "/run/user/1000/srcz/daemon.sock" {
			t.Fatalf("socket %q, %v", socket, err)
		}
	}
	if ensures != 1 {
		t.Fatalf("%d ensures for a known socket", ensures)
	}
	if _, err := daemon.Reensure(context.Background()); err != nil || ensures != 2 {
		t.Fatalf("reensure: %v, %d ensures", err, ensures)
	}
}

func TestDaemonFailureCooldown(t *testing.T) {
	ensures := 0
	daemon := NewDaemon("srcz", nil, nil, func(context.Context, string, ...string) ([]byte, []byte, error) {
		ensures++
		return nil, []byte("no daemon"), errors.New("exit status 1")
	})
	for range 2 {
		if _, err := daemon.Socket(context.Background(), false); err == nil || err.Error() != "no daemon" {
			t.Fatalf("failure: %v", err)
		}
	}
	if ensures != 1 {
		t.Fatalf("%d ensures inside the cooldown", ensures)
	}
	if _, err := daemon.Socket(context.Background(), true); err == nil || ensures != 2 {
		t.Fatalf("forced: %v, %d ensures", err, ensures)
	}
	daemon.failedAt = time.Now().Add(-failureCooldown)
	if _, err := daemon.Socket(context.Background(), false); err == nil || ensures != 3 {
		t.Fatalf("after the cooldown: %v, %d ensures", err, ensures)
	}
}

func TestDaemonReportsAMissingBinary(t *testing.T) {
	missing := errors.New("srcz is not on PATH")
	daemon := NewDaemon("", missing, nil, func(context.Context, string, ...string) ([]byte, []byte, error) {
		t.Error("ran without a binary")
		return nil, nil, nil
	})
	if _, err := daemon.Socket(context.Background(), true); !errors.Is(err, missing) {
		t.Fatalf("got %v", err)
	}
}

// After the daemon's idle exit the known socket is dead: the next operation
// ensures again and reaches the new daemon.
func TestDoEnsuresAgainWhenTheDaemonHasGone(t *testing.T) {
	dir := t.TempDir()
	dead := filepath.Join(dir, "dead.sock")
	live := filepath.Join(dir, "live.sock")
	listener, err := net.Listen("unix", live)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"protocol":1,"version":"x","pid":1,"socket":%q,"roots":["/r"],"depth":0,"scanDepth":0,"idleExit":"30m0s","startedAt":"2026-11-03T10:00:00Z"}`, live)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	socket := dead
	ensures := 0
	daemon := NewDaemon("srcz", nil, nil, func(context.Context, string, ...string) ([]byte, []byte, error) {
		ensures++
		return []byte(fmt.Sprintf(`{"protocol":1,"version":"x","pid":1,"socket":%q,"roots":[],"depth":0,"scanDepth":0,"idleExit":"30m0s","startedAt":"2026-11-03T10:00:00Z"}`, socket)), nil, nil
	})
	if _, err := daemon.Socket(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	socket = live
	calls := 0
	err = daemon.Do(context.Background(), func(c client.Client) error {
		calls++
		status, err := c.DaemonStatus(context.Background())
		if err != nil {
			return err
		}
		if len(status.Roots) != 1 {
			t.Errorf("roots %v", status.Roots)
		}
		return nil
	})
	if err != nil || calls != 2 || ensures != 2 {
		t.Fatalf("Do: %v, %d calls, %d ensures", err, calls, ensures)
	}
}

func TestDoDoesNotRepeatOtherFailures(t *testing.T) {
	daemon := NewDaemon("srcz", nil, nil, func(context.Context, string, ...string) ([]byte, []byte, error) {
		return []byte(status), nil, nil
	})
	refused := errors.New("the path is not a directory")
	calls := 0
	err := daemon.Do(context.Background(), func(client.Client) error {
		calls++
		return refused
	})
	if !errors.Is(err, refused) || calls != 1 {
		t.Fatalf("Do: %v, %d calls", err, calls)
	}
}
