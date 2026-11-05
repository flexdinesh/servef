package connect

import (
	"context"
	"errors"
	"io"
	"sync"
	"syscall"
	"time"

	"github.com/flexdinesh/servef/pkg/client"
)

// failureCooldown spaces automatic ensures after a failure: a live page's
// event stream retries every few seconds. A document load forces one.
const failureCooldown = 5 * time.Second

// Daemon is the local daemon, ensured on demand with the bundled binary. It
// is safe for concurrent use; ensures run one at a time.
type Daemon struct {
	binary string
	flags  []string
	run    Run
	// binaryErr explains a binary that could not be located.
	binaryErr error

	mu       sync.Mutex
	socket   string
	failure  error
	failedAt time.Time
}

// NewDaemon returns a Daemon that ensures with binary; binaryErr is the
// failure every ensure reports when the binary could not be located.
func NewDaemon(binary string, binaryErr error, flags []string, run Run) *Daemon {
	return &Daemon{binary: binary, binaryErr: binaryErr, flags: flags, run: run}
}

// Socket returns the daemon's socket, ensuring it when none is known. After
// a failure it reports that failure for failureCooldown unless forced.
func (d *Daemon) Socket(ctx context.Context, force bool) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.socket != "" {
		return d.socket, nil
	}
	if d.failure != nil && !force && time.Since(d.failedAt) < failureCooldown {
		return "", d.failure
	}
	return d.ensure(ctx)
}

// Reensure runs ensure again after the daemon stopped answering.
func (d *Daemon) Reensure(ctx context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ensure(ctx)
}

// Client returns a client of the ensured daemon.
func (d *Daemon) Client(ctx context.Context) (client.Client, error) {
	socket, err := d.Socket(ctx, true)
	if err != nil {
		return nil, err
	}
	return client.NewUnix(socket), nil
}

// Do runs op with a client of the daemon. When the daemon has gone, as after
// its idle exit, it ensures again and runs op once more; op must be safe to
// repeat.
func (d *Daemon) Do(ctx context.Context, op func(client.Client) error) error {
	daemon, err := d.Client(ctx)
	if err != nil {
		return err
	}
	if err = op(daemon); err == nil || !IsGone(err) || ctx.Err() != nil {
		return err
	}
	socket, err := d.Reensure(ctx)
	if err != nil {
		return err
	}
	return op(client.NewUnix(socket))
}

// IsGone reports a socket failure before a response head: nothing answers.
func IsGone(err error) bool {
	for _, target := range []error{syscall.ECONNREFUSED, syscall.ENOENT, syscall.ECONNRESET, syscall.EPIPE, io.EOF, io.ErrUnexpectedEOF} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func (d *Daemon) ensure(ctx context.Context) (string, error) {
	d.socket = ""
	if d.binaryErr != nil {
		d.failure, d.failedAt = d.binaryErr, time.Now()
		return "", d.binaryErr
	}
	status, err := Ensure(ctx, d.run, d.binary, d.flags)
	if err != nil {
		d.failure, d.failedAt = err, time.Now()
		return "", err
	}
	d.socket, d.failure = status.Socket, nil
	return d.socket, nil
}
