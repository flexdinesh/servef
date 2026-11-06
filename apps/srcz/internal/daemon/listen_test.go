//go:build darwin || linux

package daemon

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// socketDir returns a short private directory: t.TempDir paths can exceed
// sun_path.
func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestListenCreatesPrivateSocket(t *testing.T) {
	dir := filepath.Join(socketDir(t), "srcz")
	socket := filepath.Join(dir, "daemon.sock")
	listener, err := Listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if got := mode(t, dir); got != 0o700 {
		t.Errorf("directory mode = %o, want 700", got)
	}
	if got := mode(t, socket); got != 0o600 {
		t.Errorf("socket mode = %o, want 600", got)
	}
	connection, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()
}

func TestListenTightensOwnedDirectory(t *testing.T) {
	dir := socketDir(t)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	listener, err := Listen(filepath.Join(dir, "daemon.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if got := mode(t, dir); got != 0o700 {
		t.Fatalf("directory mode = %o, want 700", got)
	}
}

func TestListenRefusesSymlinkedDirectory(t *testing.T) {
	parent := socketDir(t)
	target := filepath.Join(parent, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if listener, err := Listen(filepath.Join(link, "daemon.sock")); err == nil {
		_ = listener.Close()
		t.Fatal("Listen(symlinked directory) succeeded")
	}
}

func TestListenIsSingleInstance(t *testing.T) {
	socket := filepath.Join(socketDir(t), "daemon.sock")
	first, err := Listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := Listen(socket); !errors.Is(err, ErrRunning) {
		if second != nil {
			_ = second.Close()
		}
		t.Fatalf("second Listen() = %v, want ErrRunning", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
	if _, err := os.Lstat(socket); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket after Close: %v, want removed", err)
	}
	again, err := Listen(socket)
	if err != nil {
		t.Fatalf("Listen after Close = %v", err)
	}
	_ = again.Close()
}

func TestListenReplacesCrashedDaemonSocket(t *testing.T) {
	socket := filepath.Join(socketDir(t), "daemon.sock")
	// A crashed daemon leaves its socket file behind and the kernel releases
	// its lock.
	crashed, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	crashed.SetUnlinkOnClose(false)
	_ = crashed.Close()
	if _, err := os.Lstat(socket); err != nil {
		t.Fatalf("leftover socket missing: %v", err)
	}
	listener, err := Listen(socket)
	if err != nil {
		t.Fatalf("Listen(stale socket) = %v", err)
	}
	defer listener.Close()
	connection, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()
}

func TestListenRefusesNonSocketFile(t *testing.T) {
	socket := filepath.Join(socketDir(t), "daemon.sock")
	if err := os.WriteFile(socket, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if listener, err := Listen(socket); err == nil {
		_ = listener.Close()
		t.Fatal("Listen replaced a regular file")
	}
	if content, err := os.ReadFile(socket); err != nil || string(content) != "keep" {
		t.Fatalf("regular file = %q, %v; want kept", content, err)
	}
}
