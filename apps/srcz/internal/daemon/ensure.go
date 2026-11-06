package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ensurePollInterval paces Ensure's status checks.
const ensurePollInterval = 50 * time.Millisecond

// StartFunc launches a detached daemon and returns a channel that reports the
// process's exit.
type StartFunc func(ctx context.Context) (exited <-chan error, err error)

// Ensure returns a running daemon's status, starting one if none answers;
// started reports whether this call launched it. It retries until ctx ends:
// a child that exits before answering is started again, since a dying daemon
// may still have held the lock. A daemon speaking another protocol is a
// ProtocolMismatchError.
func Ensure(ctx context.Context, socket string, start StartFunc) (Status, bool, error) {
	client := NewClient(socket)
	ticker := time.NewTicker(ensurePollInterval)
	defer ticker.Stop()
	var exited <-chan error
	var lastExit error
	started := false
	for {
		status, err := client.Status(ctx)
		if err == nil {
			if status.Protocol != ProtocolVersion {
				return status, started, &ProtocolMismatchError{Daemon: status.Protocol, Client: ProtocolVersion, Socket: socket, DaemonVersion: status.Version}
			}
			return status, started, nil
		}
		if !errors.Is(err, ErrNotRunning) && ctx.Err() == nil {
			return Status{}, started, err
		}
		if exited == nil && ctx.Err() == nil {
			exited, err = start(ctx)
			if err != nil {
				return Status{}, started, fmt.Errorf("start srcz daemon: %w", err)
			}
			started = true
		}
		select {
		case <-ctx.Done():
			if lastExit != nil {
				return Status{}, started, fmt.Errorf("srcz daemon did not start at %s (last exit: %v): %w", socket, lastExit, ctx.Err())
			}
			return Status{}, started, fmt.Errorf("srcz daemon did not start at %s: %w", socket, ctx.Err())
		case lastExit = <-exited:
			if lastExit == nil {
				lastExit = errors.New("exited")
			}
			exited = nil
		case <-ticker.C:
		}
	}
}
