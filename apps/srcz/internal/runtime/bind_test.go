package runtime

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/srcz/internal/remote"
)

func tlsBinding(t *testing.T, rt *Runtime) (Binding, *http.Client, string) {
	t.Helper()
	identity, err := remote.LoadOrCreateIdentity(filepath.Join(t.TempDir(), "identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		ForceAttemptHTTP2: true,
	}}
	t.Cleanup(client.CloseIdleConnections)
	return Binding{Listener: listener, Handler: rt.Handler(), TLS: identity.TLSConfig()}, client, "https://" + listener.Addr().String()
}

func status(t *testing.T, client *http.Client, target string) (int, string) {
	t.Helper()
	response, err := client.Get(target)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer response.Body.Close()
	return response.StatusCode, response.Proto
}

func TestBindingServesBesideTheMainListener(t *testing.T) {
	rt, _ := newRuntime(t)
	defer rt.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	address, done := serve(t, rt, ctx)
	binding, client, secure := tlsBinding(t, rt)
	bound, err := rt.Bind(binding)
	if err != nil {
		t.Fatal(err)
	}
	if got, proto := status(t, client, secure+"/api/projects"); got != http.StatusOK || proto != "HTTP/2.0" {
		t.Errorf("binding answered %d over %s", got, proto)
	}

	// Closing it leaves the app running.
	bound.Close()
	if _, err := net.DialTimeout("tcp", strings.TrimPrefix(secure, "https://"), time.Second); err == nil {
		t.Error("the binding still accepts after Close")
	}
	if got, _ := status(t, http.DefaultClient, address+"/api/projects"); got != http.StatusOK {
		t.Errorf("main listener answered %d after the binding closed", got)
	}
	cancel()
	if err := wait(t, done, 2*time.Second); err != nil {
		t.Fatalf("Serve() = %v", err)
	}
}

func TestServeStopsBindingsAndRefusesNewOnes(t *testing.T) {
	rt, root := newRuntime(t)
	defer rt.Close()
	ctx, cancel := context.WithCancel(context.Background())
	_, done := serve(t, rt, ctx)
	binding, client, secure := tlsBinding(t, rt)
	if _, err := rt.Bind(binding); err != nil {
		t.Fatal(err)
	}
	// A stream on the binding must not hold the stop open.
	response, err := client.Get(secure + "/api/events?project=" + url.QueryEscape(root))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
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
		t.Fatalf("Serve() = %v", err)
	}
	if elapsed := time.Since(started); elapsed >= ShutdownTimeout {
		t.Errorf("Serve took %s to stop with a stream on a binding", elapsed)
	}
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Error("the binding's stream outlived Serve")
	}
	if _, err := net.DialTimeout("tcp", strings.TrimPrefix(secure, "https://"), time.Second); err == nil {
		t.Error("the binding still accepts after Serve returned")
	}

	late, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Bind(Binding{Listener: late, Handler: rt.Handler()}); !errors.Is(err, ErrStopped) {
		t.Errorf("Bind after stop = %v, want ErrStopped", err)
	}
	if _, err := net.DialTimeout("tcp", late.Addr().String(), time.Second); err == nil {
		t.Error("a refused Bind left its listener open")
	}
}
