package daemon

import (
	"errors"
	"net"
	"os"
	"sync"
)

// ErrRunning reports that another daemon holds the socket's lock.
var ErrRunning = errors.New("servef daemon is already running")

// Listener is the daemon's socket, held under an exclusive lock.
type Listener struct {
	net.Listener
	path string
	lock *os.File
	once sync.Once
	err  error
}

// Close closes the socket (unlinking it) and then releases the lock. It is
// idempotent.
func (l *Listener) Close() error {
	l.once.Do(func() {
		l.err = errors.Join(l.Listener.Close(), l.lock.Close())
	})
	return l.err
}
