package connect

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLocate(t *testing.T) {
	noPath := func(string) (string, error) { return "", exec.ErrNotFound }
	if got, _ := Locate(true, "/app/resources", env(map[string]string{"SERVEF_DESKTOP_BINARY": "/x"}), noPath); got != "/app/resources/bin/servef" {
		t.Fatalf("packaged: %q", got)
	}
	if got, _ := Locate(false, "", env(map[string]string{"SERVEF_DESKTOP_BINARY": "/dev/servef"}), noPath); got != "/dev/servef" {
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
	got := DevFlags(env(map[string]string{"SERVEF_DESKTOP_SOCKET": "/tmp/s.sock", "SERVEF_DESKTOP_ROOTS_FILE": "/tmp/roots.json"}))
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
