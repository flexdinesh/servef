//go:build linux || darwin

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestLockHelper(t *testing.T) {
	path := os.Getenv("SERVEF_TEST_LOCK")
	if path == "" {
		return
	}
	lock, err := acquire(path, os.Stdout)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	// Shell builtins keep the lock in the exec'd process until stdin closes.
	if err := syscall.Exec("/bin/sh", []string{"sh", "-c", "printf 'acquired\\n'; read answer"}, os.Environ()); err != nil {
		t.Fatal(err)
	}
}

func TestLockWaitsAcrossProcessesAndReleasesAfterExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checks.lock")
	start := func(path string) (*exec.Cmd, <-chan string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLockHelper$")
		cmd.Env = append(os.Environ(), "SERVEF_TEST_LOCK="+path)
		input, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { input.Close() })
		output, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
		lines := make(chan string, 2)
		go func() {
			defer close(lines)
			scanner := bufio.NewScanner(output)
			for scanner.Scan() {
				lines <- scanner.Text()
			}
		}()
		return cmd, lines
	}
	read := func(lines <-chan string, want string) {
		t.Helper()
		select {
		case got := <-lines:
			if got != want {
				t.Fatalf("lock output = %q, want %q", got, want)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("lock process did not respond")
		}
	}
	first, acquired := start(path)
	read(acquired, "acquired")
	_, waiting := start(path)
	read(waiting, "Waiting for another worktree's checks...")
	// A different repository proceeds while the first lock remains held.
	_, independent := start(filepath.Join(t.TempDir(), "checks.lock"))
	read(independent, "acquired")
	if err := first.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	read(waiting, "acquired")
}

func TestRunReturnsCommandFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"/does/not/exist/servef-check"}} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			if err := run(args); err == nil {
				t.Fatal("invalid command accepted")
			}
		})
	}
}
