//go:build darwin || linux

package daemon

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// Listen prepares the socket's private directory, takes <socket>.lock, removes
// a leftover socket (the lock proves no live daemon owns it), listens, and
// restricts the socket to its owner. It returns ErrRunning when the lock is
// held.
func Listen(socket string) (*Listener, error) {
	if err := checkSocketLength(socket); err != nil {
		return nil, err
	}
	if err := prepareDir(filepath.Dir(socket)); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(socket+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open daemon lock: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrRunning
		}
		return nil, fmt.Errorf("lock daemon socket: %w", err)
	}
	if err := removeLeftoverSocket(socket); err != nil {
		_ = lock.Close()
		return nil, err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("listen on %s: %w", socket, err)
	}
	result := &Listener{Listener: listener, path: socket, lock: lock}
	if err := os.Chmod(socket, 0o600); err != nil {
		_ = result.Close()
		return nil, fmt.Errorf("restrict daemon socket: %w", err)
	}
	return result, nil
}

// OpenLog prepares the socket's private directory and truncates
// <socket>.log for a daemon process's output.
func OpenLog(socket string) (*os.File, error) {
	if err := prepareDir(filepath.Dir(socket)); err != nil {
		return nil, err
	}
	return os.OpenFile(socket+".log", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
}

// removeLeftoverSocket removes a crashed daemon's socket; it refuses to
// remove anything that is not a socket.
func removeLeftoverSocket(socket string) error {
	info, err := os.Lstat(socket)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect daemon socket: %w", err)
	}
	if info.Mode().Type() != fs.ModeSocket {
		return fmt.Errorf("%s exists and is not a socket; remove it or pass another --socket", socket)
	}
	if err := os.Remove(socket); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove stale daemon socket: %w", err)
	}
	return nil
}

// prepareDir creates dir 0700 or validates an existing one: it must be a real
// directory (not a symlink) owned by the current user. An owned directory
// open to group or others is tightened to 0700.
func prepareDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create daemon directory: %w", err)
	}
	var stat syscall.Stat_t
	if err := syscall.Lstat(dir, &stat); err != nil {
		return fmt.Errorf("inspect daemon directory: %w", err)
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFDIR {
		return fmt.Errorf("daemon directory %s is not a directory", dir)
	}
	if int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("daemon directory %s is owned by another user", dir)
	}
	if stat.Mode&0o077 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("restrict daemon directory: %w", err)
		}
	}
	return nil
}
