package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/lsp/lsptest"
)

// The test binary doubles as a fake language server, so tests run real
// processes without installing servers.
func TestMain(m *testing.M) {
	if lsptest.Serving() {
		lsptest.Serve()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type fixture struct {
	root string
	log  string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	t.Setenv(lsptest.Env, "1")
	root := t.TempDir()
	log := filepath.Join(t.TempDir(), "log")
	t.Setenv("SERVEF_FAKE_LSP_LOG", log)
	return fixture{root: root, log: log}
}

func (f fixture) lines(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(f.log)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

var fake = Server{Name: "fake", Language: "Fake", Families: map[string]string{"go": "go"}}

func manager(t *testing.T, options Options) *Manager {
	t.Helper()
	if options.LookPath == nil {
		options.LookPath = func(name string) (string, bool) { return os.Args[0], name == "fake" }
	}
	m := NewManager([]Server{fake}, options)
	t.Cleanup(m.Close)
	return m
}

func (f fixture) request(name string, modified time.Time, files ...string) Request {
	document := File{Path: filepath.Join(f.root, name), Size: 10, Modified: modified}
	listed := []File{document}
	for _, file := range files {
		listed = append(listed, File{Path: filepath.Join(f.root, file), Size: 1, Modified: modified})
	}
	return Request{Root: f.root, Family: "go", Document: document, Text: []byte("package a\n"), At: Position{Line: 3, Column: 7}, Files: listed}
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDefinitionAndReferencesRoundTrip(t *testing.T) {
	f := newFixture(t)
	m := manager(t, Options{})
	status, ok := m.Status(f.root, "go")
	if !ok || status.State != StateStopped {
		t.Fatalf("status = %+v, %v; want stopped", status, ok)
	}
	if _, ok := m.Status(f.root, "markdown"); ok {
		t.Fatal("markdown has no server")
	}
	now := time.Now()
	definitions, err := m.Definition(context.Background(), f.request("main.go", now))
	if err != nil {
		t.Fatal(err)
	}
	want := Location{Path: filepath.Join(f.root, "target.go"), Position: Position{Line: 5, Column: 2}}
	if len(definitions) != 1 || definitions[0] != want {
		t.Fatalf("definitions = %+v, want %+v", definitions, want)
	}
	references, err := m.References(context.Background(), f.request("main.go", now))
	if err != nil {
		t.Fatal(err)
	}
	// The fake answers with the queried position (1-based lines, UTF-16
	// columns) and a file outside the project, which callers filter.
	if len(references) != 2 || references[0].Position != (Position{Line: 3, Column: 7}) || filepath.Base(references[0].Path) != "main.go" {
		t.Fatalf("references = %+v", references)
	}
	status, _ = m.Status(f.root, "go")
	if status.State != StateReady {
		t.Fatalf("state = %s, want ready", status.State)
	}
	if runtime.GOOS == "linux" && status.Memory == 0 {
		t.Fatal("ready server reports no memory")
	}
}

func TestStartingServerAnswersOnceReady(t *testing.T) {
	f := newFixture(t)
	t.Setenv("SERVEF_FAKE_LSP_SLOW", "300ms")
	m := manager(t, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := m.Definition(ctx, f.request("main.go", time.Now())); !errors.Is(err, ErrNotReady) {
		t.Fatalf("error = %v, want ErrNotReady", err)
	}
	if status, _ := m.Status(f.root, "go"); status.State != StateStarting {
		t.Fatalf("state = %s, want starting", status.State)
	}
	if _, err := m.Definition(context.Background(), f.request("main.go", time.Now())); err != nil {
		t.Fatal(err)
	}
}

func TestServersLearnOfChangedFiles(t *testing.T) {
	f := newFixture(t)
	m := manager(t, Options{})
	first := time.Now()
	if _, err := m.Definition(context.Background(), f.request("main.go", first, "old.go", "kept.go")); err != nil {
		t.Fatal(err)
	}
	later := first.Add(time.Second)
	if _, err := m.Definition(context.Background(), f.request("main.go", later, "kept.go", "new.go")); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return len(f.lines(t)) >= 6 })
	got := strings.Join(f.lines(t), "\n")
	for _, want := range []string{
		"textDocument/didOpen main.go 1",
		"watched main.go 2", "watched kept.go 2", "watched new.go 1", "watched old.go 3",
		"textDocument/didChange main.go 2",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("log missing %q:\n%s", want, got)
		}
	}
}

func TestIdleServersStop(t *testing.T) {
	f := newFixture(t)
	m := manager(t, Options{Idle: 50 * time.Millisecond})
	if _, err := m.Definition(context.Background(), f.request("main.go", time.Now())); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		status, _ := m.Status(f.root, "go")
		return status.State == StateStopped
	})
	// The next request starts it again.
	if _, err := m.Definition(context.Background(), f.request("main.go", time.Now())); err != nil {
		t.Fatal(err)
	}
}

func TestCrashedServersReportFailureUntilRefresh(t *testing.T) {
	f := newFixture(t)
	t.Setenv("SERVEF_FAKE_LSP_CRASH", "1")
	m := manager(t, Options{})
	if _, err := m.Definition(context.Background(), f.request("main.go", time.Now())); err == nil {
		t.Fatal("crashed server answered")
	}
	status, _ := m.Status(f.root, "go")
	if status.State != StateFailed || !strings.Contains(status.Message, "fake server crashed") {
		t.Fatalf("status = %+v, want failed with stderr", status)
	}
	if m.Available(f.root, "go") {
		t.Fatal("failed server reported available")
	}
	m.Refresh()
	if status, _ := m.Status(f.root, "go"); status.State != StateStopped {
		t.Fatalf("after refresh state = %s, want stopped", status.State)
	}
}

func TestMissingServers(t *testing.T) {
	f := newFixture(t)
	installed := false
	m := manager(t, Options{LookPath: func(string) (string, bool) { return os.Args[0], installed }})
	if status, _ := m.Status(f.root, "go"); status.State != StateMissing {
		t.Fatalf("state = %s, want missing", status.State)
	}
	if _, err := m.Definition(context.Background(), f.request("main.go", time.Now())); !errors.Is(err, ErrMissing) {
		t.Fatalf("error = %v, want ErrMissing", err)
	}
	// Discovery is cached until Refresh, e.g. after installing.
	installed = true
	if status, _ := m.Status(f.root, "go"); status.State != StateMissing {
		t.Fatal("discovery not cached")
	}
	m.Refresh()
	if status, _ := m.Status(f.root, "go"); status.State != StateStopped {
		t.Fatalf("after refresh state = %s, want stopped", status.State)
	}
}

func TestCloseStopsServers(t *testing.T) {
	f := newFixture(t)
	m := manager(t, Options{})
	if _, err := m.Definition(context.Background(), f.request("main.go", time.Now())); err != nil {
		t.Fatal(err)
	}
	m.Close()
	if _, err := m.Definition(context.Background(), f.request("main.go", time.Now())); err == nil {
		t.Fatal("closed manager answered")
	}
}

func TestDecodeLocationsAcceptsEveryShape(t *testing.T) {
	for _, raw := range []string{`null`, `[]`} {
		if locations, err := decodeLocations(json.RawMessage(raw)); err != nil || len(locations) != 0 {
			t.Fatalf("%s: %v, %v", raw, locations, err)
		}
	}
	one, err := decodeLocations(json.RawMessage(`{"uri":"file:///a/b%20c.go","range":{"start":{"line":1,"character":3}}}`))
	if err != nil || len(one) != 1 || one[0].Path != filepath.FromSlash("/a/b c.go") || one[0].Position != (Position{Line: 2, Column: 3}) {
		t.Fatalf("one = %+v, %v", one, err)
	}
	if skipped, _ := decodeLocations(json.RawMessage(`[{"uri":"jdt://x","range":{"start":{"line":0,"character":0}}}]`)); len(skipped) != 0 {
		t.Fatalf("non-file URIs kept: %+v", skipped)
	}
}

func TestBuiltinServersAreData(t *testing.T) {
	seen := map[string]string{}
	for _, server := range Builtins() {
		if server.Name == "" || server.Language == "" || len(server.Install) == 0 || server.Docs == "" {
			t.Fatalf("incomplete server %+v", server)
		}
		for family, language := range server.Families {
			if language == "" {
				t.Fatalf("%s: family %s has no language ID", server.Name, family)
			}
			if other, ok := seen[family]; ok {
				t.Fatalf("family %s served by %s and %s", family, other, server.Name)
			}
			seen[family] = server.Name
		}
	}
}
