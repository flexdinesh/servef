package control

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestRegisteredServerCanBeVerifiedAndStopped(t *testing.T) {
	registry := NewRegistry(t.TempDir())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	instance, err := registry.Register("127.0.0.1", port)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close() })

	server := &http.Server{Handler: instance.Handler(http.NotFoundHandler())}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	remote, err := registry.Lookup("127.0.0.1", port)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !remote.Running(ctx) {
		t.Fatal("registered server is not running")
	}
	if err := remote.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-instance.StopRequested():
	case <-ctx.Done():
		t.Fatal("shutdown was not requested")
	}
}

func TestControlEndpointRejectsMissingToken(t *testing.T) {
	registry := NewRegistry(t.TempDir())
	instance, err := registry.Register("localhost", 7971)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close() })

	request, err := http.NewRequest(http.MethodPost, endpointPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	response := &responseRecorder{header: make(http.Header)}
	instance.Handler(http.NotFoundHandler()).ServeHTTP(response, request)
	if response.status != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.status, http.StatusNotFound)
	}
	select {
	case <-instance.StopRequested():
		t.Fatal("unauthenticated shutdown was requested")
	default:
	}
}

type responseRecorder struct {
	header http.Header
	status int
}

func (r *responseRecorder) Header() http.Header { return r.header }

func (r *responseRecorder) Write(contents []byte) (int, error) { return len(contents), nil }

func (r *responseRecorder) WriteHeader(status int) { r.status = status }

func TestRegistryList(t *testing.T) {
	directory := t.TempDir()
	registry := NewRegistry(directory)
	if remotes, err := NewRegistry(filepath.Join(directory, "missing")).List(); err != nil || len(remotes) != 0 {
		t.Fatalf("missing directory = %v, %v; want none", remotes, err)
	}
	for _, address := range []struct {
		host string
		port int
	}{{"localhost", 9000}, {"0.0.0.0", 7971}, {"::", 7971}} {
		instance, err := registry.Register(address.host, address.port)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = instance.Close() })
	}
	for name, contents := range map[string]string{
		"broken.json":    "{",
		"empty.json":     `{"host":"localhost","port":8000}`,
		"noport.json":    `{"host":"localhost","token":"x"}`,
		"foreign.txt":    `{"host":"localhost","port":8001,"token":"x"}`,
		"emptyhost.json": `{"port":8002,"token":"x"}`,
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	remotes, err := registry.List()
	if err != nil {
		t.Fatal(err)
	}
	var origins []string
	for _, remote := range remotes {
		origins = append(origins, remote.Origin())
	}
	want := []string{"http://127.0.0.1:7971", "http://[::1]:7971", "http://localhost:9000"}
	if !slices.Equal(origins, want) {
		t.Errorf("origins = %v, want %v", origins, want)
	}
}
