//go:build linux || darwin

// check-lock serializes checks across linked worktrees on supported dev hosts.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: check-lock command [args...]")
	}
	command, err := exec.LookPath(args[0])
	if err != nil {
		return err
	}
	output, err := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return fmt.Errorf("find shared Git directory: %w", err)
	}
	lock, err := acquire(filepath.Join(strings.TrimSpace(string(output)), "servef-check.lock"), os.Stderr)
	if err != nil {
		return err
	}
	defer lock.Close()
	// Replace this process; the lock follows mise and its children. A killed
	// wrapper cannot release it while its check processes are still running.
	return syscall.Exec(command, args, os.Environ())
}

func acquire(path string, output io.Writer) (*os.File, error) {
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*os.File, error) {
		lock.Close()
		return nil, err
	}
	fd := int(lock.Fd())
	err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		fmt.Fprintln(output, "Waiting for another worktree's checks...")
		for {
			err = syscall.Flock(fd, syscall.LOCK_EX)
			if !errors.Is(err, syscall.EINTR) {
				break
			}
		}
	}
	if err != nil {
		return fail(fmt.Errorf("lock checks: %w", err))
	}
	// os.OpenFile sets close-on-exec. Keep the descriptor across exec so the
	// OS releases the lock only after the last check process closes it.
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_SETFD, 0); errno != 0 {
		return fail(errno)
	}
	return lock, nil
}
