package connect

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/flexdinesh/servef/apps/native/internal/link"
	"github.com/flexdinesh/servef/pkg/client"
	"github.com/flexdinesh/servef/pkg/contract"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLocate(t *testing.T) {
	noPath := func(string) (string, error) { return "", exec.ErrNotFound }
	if got, _ := Locate(true, "/app/resources", env(map[string]string{"SERVEF_NATIVE_BINARY": "/x"}), noPath); got != "/app/resources/bin/servef" {
		t.Fatalf("packaged: %q", got)
	}
	if got, _ := Locate(false, "", env(map[string]string{"SERVEF_NATIVE_BINARY": "/dev/servef"}), noPath); got != "/dev/servef" {
		t.Fatalf("override: %q", got)
	}
	onPath := func(string) (string, error) { return "/usr/bin/servef", nil }
	if got, _ := Locate(false, "", env(nil), onPath); got != "/usr/bin/servef" {
		t.Fatalf("PATH: %q", got)
	}
	if _, err := Locate(false, "", env(nil), noPath); err == nil {
		t.Fatal("missing binary accepted")
	}
}

func TestDevFlags(t *testing.T) {
	got := DevFlags(env(map[string]string{"SERVEF_NATIVE_SOCKET": "/tmp/s.sock", "SERVEF_NATIVE_ROOTS_FILE": "/tmp/roots.json"}))
	if !slices.Equal(got, []string{"--socket", "/tmp/s.sock", "--roots-file", "/tmp/roots.json"}) {
		t.Fatalf("flags %v", got)
	}
	if DevFlags(env(nil)) != nil {
		t.Fatal("no flags without variables")
	}
}

const status = `{"protocol":1,"version":"0.1.7","pid":42,"socket":"/run/user/1000/servef/daemon.sock","roots":["/home/dee/code"],"depth":0,"scanDepth":0,"idleExit":"30m0s","startedAt":"2026-11-03T10:00:00Z"}`

func TestEnsure(t *testing.T) {
	var args []string
	run := func(_ context.Context, name string, a ...string) ([]byte, []byte, error) {
		args = append([]string{name}, a...)
		return []byte("starting\n" + status + "\n"), nil, nil
	}
	got, err := Ensure(context.Background(), run, "/bin/servef", []string{"--socket", "/s"})
	if err != nil || got.Socket != "/run/user/1000/servef/daemon.sock" || got.Version != "0.1.7" {
		t.Fatalf("ensure: %+v, %v", got, err)
	}
	if !slices.Equal(args, []string{"/bin/servef", "daemon", "ensure", "--json", "--socket", "/s"}) {
		t.Fatalf("args %v", args)
	}
}

func TestEnsureFailures(t *testing.T) {
	mismatch := func(context.Context, string, ...string) ([]byte, []byte, error) {
		return []byte(strings.Replace(status, `"protocol":1`, `"protocol":2`, 1)), []byte("protocol mismatch"), errors.New("exit status 1")
	}
	_, err := Ensure(context.Background(), mismatch, "servef", nil)
	var ensureErr *EnsureError
	if !errors.As(err, &ensureErr) || !ensureErr.Mismatch || ensureErr.Status == nil || ensureErr.Status.Protocol != 2 {
		t.Fatalf("mismatch: %v", err)
	}

	failed := func(context.Context, string, ...string) ([]byte, []byte, error) {
		return nil, []byte("daemon did not answer; see /tmp/x.log\n"), errors.New("exit status 1")
	}
	if _, err := Ensure(context.Background(), failed, "servef", nil); err == nil || err.Error() != "daemon did not answer; see /tmp/x.log" {
		t.Fatalf("failed: %v", err)
	}

	missing := func(context.Context, string, ...string) ([]byte, []byte, error) { return nil, nil, exec.ErrNotFound }
	if _, err := Ensure(context.Background(), missing, "nope", nil); err == nil || !strings.Contains(err.Error(), "cannot run nope") {
		t.Fatalf("missing: %v", err)
	}
}

func TestListServers(t *testing.T) {
	run := func(_ context.Context, _ string, a ...string) ([]byte, []byte, error) {
		if !slices.Equal(a, []string{"control", "list", "--json"}) {
			t.Fatalf("args %v", a)
		}
		return []byte(`{"servers":[{"host":"localhost","port":7971,"origin":"http://localhost:7971"},{"host":"10.0.0.2","port":7972,"origin":"http://10.0.0.2:7972"},{"host":"127.0.0.1","port":7971,"origin":"http://127.0.0.1:7971"}]}`), nil, nil
	}
	servers, err := ListServers(context.Background(), run, "servef")
	if err != nil || len(servers) != 1 || servers[0].Port != 7971 {
		t.Fatalf("servers %+v, %v", servers, err)
	}
}

func TestServerImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/image" || r.URL.Query().Get("project") != "/p" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		if r.URL.Query().Get("path") == "big.png" {
			_, _ = w.Write(make([]byte, contract.MaxImageBytes+1))
			return
		}
		_, _ = w.Write([]byte("PNG"))
	}))
	defer server.Close()
	target, err := link.ParseServer(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	connection := Server(target)
	if connection.Daemon || !strings.HasPrefix(connection.ID, "http-127.0.0.1-") {
		t.Fatalf("connection %+v", connection)
	}
	image, err := connection.Client.Image(context.Background(), "/p", "img/a.png")
	if err != nil || string(image.Content) != "PNG" || image.MediaType != "image/png" {
		t.Fatalf("image %+v, %v", image, err)
	}
	var failure *client.Error
	if _, err := connection.Client.Image(context.Background(), "/other", "a.png"); !errors.As(err, &failure) || failure.Kind != contract.KindNotFound {
		t.Fatalf("not found error = %v", err)
	}
	if _, err := connection.Client.Image(context.Background(), "/p", "big.png"); !errors.As(err, &failure) || failure.Kind != contract.KindTooLarge {
		t.Fatalf("oversized image error = %v", err)
	}
}
