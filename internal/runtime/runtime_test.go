package runtime

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/projects"
)

func newRuntime(t *testing.T) (*Runtime, string) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	roots, err := projects.NewRootSet([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := New(Config{Roots: roots, Depth: 5, ScanDepth: 3})
	if err != nil {
		t.Fatal(err)
	}
	return rt, root
}

func serve(t *testing.T, rt *Runtime, ctx context.Context) (string, <-chan error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- rt.Serve(ctx, listener, rt.Handler()) }()
	return "http://" + listener.Addr().String(), done
}

func wait(t *testing.T, done <-chan error, within time.Duration) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(within):
		t.Fatalf("Serve did not return within %s", within)
		return nil
	}
}

func TestServeStopsCleanlyOnContextAndClosesListener(t *testing.T) {
	rt, _ := newRuntime(t)
	ctx, cancel := context.WithCancel(context.Background())
	address, done := serve(t, rt, ctx)
	response, err := http.Get(address + "/api/projects")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/projects status = %d", response.StatusCode)
	}
	cancel()
	if err := wait(t, done, 2*time.Second); err != nil {
		t.Fatalf("Serve() = %v, want nil", err)
	}
	if _, err := net.DialTimeout("tcp", strings.TrimPrefix(address, "http://"), time.Second); err == nil {
		t.Fatal("listener still accepts after Serve returned")
	}
	// Close after Serve is safe.
	rt.Close()
}

func TestServeEndsEventStreamsBeforeShutdownTimeout(t *testing.T) {
	rt, root := newRuntime(t)
	defer rt.Close()
	ctx, cancel := context.WithCancel(context.Background())
	address, done := serve(t, rt, ctx)
	response, err := http.Get(address + "/api/events?project=" + url.QueryEscape(root))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/events status = %d", response.StatusCode)
	}
	ended := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
		}
		close(ended)
	}()
	started := time.Now()
	cancel()
	if err := wait(t, done, ShutdownTimeout+time.Second); err != nil {
		t.Fatalf("Serve() = %v, want nil", err)
	}
	if elapsed := time.Since(started); elapsed >= ShutdownTimeout {
		t.Fatalf("Serve took %s to stop with an open event stream", elapsed)
	}
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("event stream still open after Serve returned")
	}
}

func TestServeReportsServeErrors(t *testing.T) {
	rt, _ := newRuntime(t)
	defer rt.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = listener.Close()
	err = rt.Serve(context.Background(), listener, rt.Handler())
	if err == nil || !strings.HasPrefix(err.Error(), "serve files: ") {
		t.Fatalf("Serve(closed listener) = %v, want serve files error", err)
	}
}

func TestNewRequiresRoots(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if _, err := New(Config{}); err == nil || !strings.HasPrefix(err.Error(), "create web interface: ") {
		t.Fatalf("New(no roots) = %v, want create web interface error", err)
	}
}
