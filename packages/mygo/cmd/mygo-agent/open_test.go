package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/packages/mygo/agent/demo"
	"github.com/flexdinesh/servef/packages/mygo/agent/session"
)

func newCLI(env map[string]string) (*cli, *bytes.Buffer) {
	var out bytes.Buffer
	return &cli{stdout: &out, stderr: &out, getenv: func(k string) string { return env[k] }}, &out
}

func privateDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mga")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// serveDemo serves a counter at socket in this process, as a session left
// running by an earlier open would.
func serveDemo(t *testing.T, socket string) *session.Session {
	t.Helper()
	s := session.New(session.Options{Width: 480, Height: 360, View: func(s *session.Session) func(*ui.Context) { return demo.New(s).View }})
	t.Cleanup(s.Close)
	go func() { _ = s.ListenAndServe(socket) }()
	deadline := time.Now().Add(5 * time.Second)
	for !serving(socket) {
		if time.Now().After(deadline) {
			t.Fatal("demo did not serve")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return s
}

func TestOpenRefusesALiveSessionUnlessReplace(t *testing.T) {
	dir := privateDir(t)
	env := map[string]string{"MYGO_AGENT_DIR": dir}
	t.Setenv("MYGO_AGENT_TEST_HOST", "1")
	stale := serveDemo(t, filepath.Join(dir, "default.sock"))
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c, _ := newCLI(env)
	err = c.run([]string{"open", "--", self})
	if err == nil || !strings.Contains(err.Error(), "already serves") {
		t.Fatalf("open over a live session: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "default.json")); !os.IsNotExist(err) {
		t.Fatalf("refused open recorded a session: %v", err)
	}
	c, out := newCLI(env)
	if err := c.run([]string{"open", "--replace", "--", self}); err != nil {
		t.Fatalf("replace: %v\n%s", err, out)
	}
	t.Cleanup(func() { c, _ := newCLI(env); _ = c.run([]string{"close"}) })
	select {
	case <-stale.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the stale session kept running")
	}
	s, ok := c.readState()
	if !ok || s.PID == 0 || s.PID == os.Getpid() {
		t.Fatalf("state %+v", s)
	}
}

func TestOpenFailsWhenTheHostExitsBeforeServing(t *testing.T) {
	dir := privateDir(t)
	c, _ := newCLI(map[string]string{"MYGO_AGENT_DIR": dir})
	err := c.run([]string{"open", "--", "/bin/false"})
	if err == nil || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("open: %v", err)
	}
	if _, ok := c.readState(); ok {
		t.Fatal("recorded a host that never served")
	}
}

func TestStateDirIsPrivate(t *testing.T) {
	dir := filepath.Join(privateDir(t), "state")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := prepareDir(dir, os.Getuid()); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o700 {
		t.Fatalf("mode %v, want 0700", info.Mode().Perm())
	}
	if err := prepareDir(dir, os.Getuid()+1); err == nil || !strings.Contains(err.Error(), "another user") {
		t.Fatalf("foreign owner: %v", err)
	}
	link := dir + "-link"
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if err := prepareDir(link, os.Getuid()); err == nil {
		t.Fatal("accepted a symlinked directory")
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareDir(file, os.Getuid()); err == nil {
		t.Fatal("accepted a file")
	}
}

func TestStateFilesNeverFollowSymlinks(t *testing.T) {
	dir := privateDir(t)
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "default.json")
	if err := os.Symlink(victim, path); err != nil {
		t.Fatal(err)
	}
	if _, err := readPrivate(path); err == nil {
		t.Fatal("read through a symlink")
	}
	if f, err := createPrivate(path); err == nil {
		_ = f.Close()
		t.Fatal("created through a symlink")
	}
	if err := writePrivate(path, []byte(`{"socket":"x"}`)); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatalf("victim changed: %q", b)
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
		t.Fatalf("state file %v %v", info.Mode(), err)
	}
}

func TestSessionNamesArePlainNames(t *testing.T) {
	for _, name := range []string{"../x", "a/b", "..", ".hidden", ""} {
		c, _ := newCLI(map[string]string{"MYGO_AGENT_DIR": privateDir(t)})
		if err := c.run([]string{"--session", name, "snapshot"}); err == nil || !strings.Contains(err.Error(), "--session") {
			t.Errorf("%q: %v", name, err)
		}
	}
}

func TestWaitForText(t *testing.T) {
	dir := privateDir(t)
	socket := filepath.Join(dir, "w.sock")
	serveDemo(t, socket)
	c, _ := newCLI(map[string]string{"MYGO_AGENT_DIR": dir})
	if err := c.run([]string{"--socket", socket, "click", "Load"}); err != nil {
		t.Fatal(err)
	}
	if err := c.run([]string{"--socket", socket, "wait", "--text", "Loaded", "--timeout", "3000"}); err != nil {
		t.Fatal(err)
	}
	if err := c.run([]string{"--socket", socket, "wait", "--text", "Never", "--timeout", "200"}); err == nil {
		t.Fatal("waited for a text that never shows")
	}
	if err := c.run([]string{"--socket", socket, "wait"}); err == nil {
		t.Fatal("wait without --text")
	}
}
