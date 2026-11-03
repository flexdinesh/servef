package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStatusObservesCrashedReadyServerAndBackoffExpires(t *testing.T) {
	f := newFixture(t)
	m := manager(t, Options{})
	if err := m.Start(context.Background(), f.root, "go"); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	key := clientKey{server: "fake", root: f.root}
	c := m.clients[key]
	m.mu.Unlock()
	c.proc.Lock()
	killProcess(c.cmd)
	c.proc.Unlock()
	eventually(t, func() bool { status, _ := m.Status(f.root, "go"); return status.State == StateFailed })
	m.mu.Lock()
	failed := m.failures[key]
	failed.at = time.Now().Add(-retryAfter - time.Second)
	m.failures[key] = failed
	m.mu.Unlock()
	if status, _ := m.Status(f.root, "go"); status.State != StateStopped {
		t.Fatalf("expired backoff = %+v", status)
	}
	if _, err := m.Definition(context.Background(), f.request("main.go", time.Now())); err != nil {
		t.Fatal(err)
	}
}

func TestIdleDoesNotStopActiveRequest(t *testing.T) {
	f := newFixture(t)
	t.Setenv("SERVEF_FAKE_LSP_QUERY_SLOW", "150ms")
	m := manager(t, Options{Idle: 20 * time.Millisecond})
	if _, err := m.Definition(context.Background(), f.request("main.go", time.Now())); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { status, _ := m.Status(f.root, "go"); return status.State == StateStopped })
}

func TestCancellationKeepsStartingServerAndPreservesCause(t *testing.T) {
	f := newFixture(t)
	t.Setenv("SERVEF_FAKE_LSP_SLOW", "100ms")
	m := manager(t, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := m.Definition(ctx, f.request("main.go", time.Now()))
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrNotReady) {
		t.Fatalf("startup error loses cause: %v", err)
	}
	if err := m.Start(context.Background(), f.root, "go"); err != nil {
		t.Fatal(err)
	}
	if status, _ := m.Status(f.root, "go"); status.State != StateReady {
		t.Fatalf("cancelled request damaged server: %+v", status)
	}
}

func TestUnsupportedMethodsAreNegotiated(t *testing.T) {
	f := newFixture(t)
	t.Setenv("SERVEF_FAKE_LSP_UNSUPPORTED", "1")
	m := manager(t, Options{})
	if _, err := m.Definition(context.Background(), f.request("main.go", time.Now())); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported definition = %v", err)
	}
	status, _ := m.Status(f.root, "go")
	if status.Capabilities == nil || status.Capabilities.Definition || status.Capabilities.References {
		t.Fatalf("capabilities = %+v", status.Capabilities)
	}
}

func TestUnsupportedPositionEncodingFailsInitialization(t *testing.T) {
	f := newFixture(t)
	t.Setenv("SERVEF_FAKE_LSP_ENCODING", "utf-8")
	m := manager(t, Options{})
	if err := m.Start(context.Background(), f.root, "go"); err == nil || !strings.Contains(err.Error(), "unsupported position encoding") {
		t.Fatalf("initialize = %v", err)
	}
}

func TestIncrementalSyncReplacesCompletePreviousUTF16Range(t *testing.T) {
	f := newFixture(t)
	t.Setenv("SERVEF_FAKE_LSP_SYNC", `{"openClose":true,"change":2}`)
	m := manager(t, Options{})
	request := f.request("main.go", time.Now())
	request.Text = []byte("a\nx😀")
	if _, err := m.Definition(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Document.Modified = request.Document.Modified.Add(time.Second)
	request.Files[0] = request.Document
	request.Text = []byte("updated")
	if _, err := m.Definition(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		return strings.Contains(strings.Join(f.lines(t), "\n"), `"range":{"end":{"character":3,"line":1},"start":{"character":0,"line":0}}`)
	})
}

func TestChangedPreviouslyOpenedDocumentClosesOverlayBeforeWatch(t *testing.T) {
	f := newFixture(t)
	m := manager(t, Options{})
	now := time.Now()
	first := f.request("first.go", now, "second.go", "tsconfig.json")
	if _, err := m.Definition(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := f.request("second.go", now.Add(time.Second), "first.go", "tsconfig.json")
	if _, err := m.Definition(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return strings.Contains(strings.Join(f.lines(t), "\n"), "watched tsconfig.json 2") })
	logs := strings.Join(f.lines(t), "\n")
	closeAt, watchAt := strings.Index(logs, "textDocument/didClose first.go"), strings.Index(logs, "watched first.go 2")
	if closeAt < 0 || watchAt < closeAt {
		t.Fatalf("stale overlay not closed before disk notification:\n%s", logs)
	}
}

func TestDynamicCapabilitiesRespectLanguageAndUnregister(t *testing.T) {
	c := newClient(fake, t.TempDir())
	params := json.RawMessage(`{"registrations":[{"id":"typescript-def","method":"textDocument/definition","registerOptions":{"documentSelector":[{"language":"typescript","scheme":"file","pattern":"**/*.ts"}]}}]}`)
	if _, err := c.answer("client/registerCapability", params); err != nil {
		t.Fatal(err)
	}
	if !c.capabilities("typescript", filepath.FromSlash("/project/file.ts")).Definition || c.capabilities("javascript", "/project/file.js").Definition {
		t.Fatal("dynamic selector ignored")
	}
	if _, err := c.answer("client/unregisterCapability", json.RawMessage(`{"unregisterations":[{"id":"typescript-def"}]}`)); err != nil {
		t.Fatal(err)
	}
	if c.capabilities("typescript", "/project/file.ts").Definition {
		t.Fatal("unregister did not remove capability")
	}
	if _, err := c.answer("client/registerCapability", json.RawMessage(`{"registrations":[{"id":"edit","method":"workspace/applyEdit"}]}`)); err == nil {
		t.Fatal("accepted unsupported client operation")
	}
}

func TestLocationRangeAndURIValidation(t *testing.T) {
	locations, err := decodeLocations(json.RawMessage(`[
 {"uri":"file:///project/a.ts","range":{"start":{"line":1,"character":2},"end":{"line":1,"character":8}}},
 {"uri":"file://remote/project/a.ts","range":{"start":{"line":1,"character":2},"end":{"line":1,"character":8}}},
 {"uri":"file:///project/a.ts","range":{"start":{"line":1,"character":2},"end":{"line":0,"character":8}}}
 ]`))
	if err != nil || len(locations) != 1 || locations[0].End != (Position{Line: 2, Column: 8}) {
		t.Fatalf("ranges = %+v, %v", locations, err)
	}
}

func TestWatcherGlobsRelativeBaseKindsAndUnregister(t *testing.T) {
	c := newClient(fake, "/project")
	params := json.RawMessage(`{"registrations":[{"id":"go","method":"workspace/didChangeWatchedFiles","registerOptions":{"watchers":[{"globPattern":{"baseUri":{"uri":"file:///project"},"pattern":"**/*.{go,mod,sum,work}"},"kind":2}]}}]}`)
	if _, err := c.answer("client/registerCapability", params); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path string
		kind int
		want bool
	}{
		{"/project/main.go", 2, true}, {"/project/nested/go.mod", 2, true}, {"/project/go.sum", 2, true},
		{"/project/go.work", 2, true}, {"/project/main.go", 1, false}, {"/project/main.go", 3, false},
		{"/outside/main.go", 2, false}, {"/project/main.js", 2, false},
	} {
		if got := c.watches(filepath.FromSlash(test.path), test.kind); got != test.want {
			t.Errorf("watches(%s,%d)=%v", test.path, test.kind, got)
		}
	}
	if _, err := c.answer("client/unregisterCapability", json.RawMessage(`{"unregisterations":[{"id":"go"}]}`)); err != nil {
		t.Fatal(err)
	}
	if c.watches("/project/main.go", 2) {
		t.Fatal("unregistered watcher still active")
	}
}

func TestConcurrentQueriesRegistrationsStatusAndClose(t *testing.T) {
	f := newFixture(t)
	m := manager(t, Options{})
	if err := m.Start(context.Background(), f.root, "go"); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	c := m.clients[clientKey{server: "fake", root: f.root}]
	m.mu.Unlock()
	var group sync.WaitGroup
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < 8; j++ {
				_, _ = m.Definition(context.Background(), f.request("main.go", time.Now()))
			}
		}()
	}
	group.Add(1)
	go func() {
		defer group.Done()
		for i := 0; i < 16; i++ {
			_, _ = c.answer("client/registerCapability", json.RawMessage(`{"registrations":[{"id":"dynamic","method":"textDocument/references"}]}`))
			_, _ = m.Status(f.root, "go")
			_, _ = c.answer("client/unregisterCapability", json.RawMessage(`{"unregisterations":[{"id":"dynamic"}]}`))
		}
	}()
	m.Close()
	group.Wait()
}
