package client

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/daemon"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/watch"
	"github.com/flexdinesh/servef/internal/web"
	"github.com/flexdinesh/servef/pkg/contract"
)

// transport is one way to reach the same served roots.
type transport struct {
	name   string
	client Client
	daemon bool
}

// fixture serves one root through every transport: TCP to the web app, a
// Unix socket to the daemon's control API before it, and in process.
type fixture struct {
	root       string
	transports []transport
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	for name, content := range map[string]string{
		"README.md":      "# Hello\n",
		"data.json":      `{"b":1,"a":[1,2]}`,
		"broken.json":    `{"b":`,
		"notes/today.md": "today\n",
		"code/main.go":   "package main\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	roots, err := projects.NewRootSet([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	options := watch.DefaultOptions()
	options.Debounce = 30 * time.Millisecond
	options.MaxWait = 200 * time.Millisecond
	webApp, err := web.New(web.Config{Roots: roots, Depth: 5, LiveUpdates: watch.NewHub(options), WalkthroughDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	control := daemon.NewController(daemon.ControllerConfig{
		Roots:     roots,
		RootsFile: filepath.Join(t.TempDir(), "roots.json"),
		Status:    daemon.Status{Protocol: daemon.ProtocolVersion, Version: "test", PID: 1, Socket: "test.sock", Depth: 5, IdleExit: "0s", StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
		Done:      done,
		Notices:   webApp,
	})

	tcp := httptest.NewServer(webApp)
	dir, err := os.MkdirTemp("", "servef-client-")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "daemon.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	unix := &http.Server{Handler: control.Handler(webApp)}
	go func() { _ = unix.Serve(listener) }()
	t.Cleanup(func() {
		// Streams end first, as servef's shutdown does.
		close(done)
		webApp.StopLiveUpdates()
		tcp.Close()
		_ = unix.Close()
		webApp.Close()
		_ = os.RemoveAll(dir)
	})
	return fixture{root: root, transports: []transport{
		{name: "tcp", client: NewTCP(tcp.URL)},
		{name: "unix", client: NewUnix(socket), daemon: true},
		{name: "in-process", client: NewInProcess(webApp.Application(), control), daemon: true},
	}}
}

// outcome is a call's result and error, comparable across transports.
type outcome struct {
	Value any
	Err   error
}

func result[T any](value T, err error) outcome {
	return outcome{Value: value, Err: err}
}

// same runs call on every transport and requires identical outcomes, then
// returns the first.
func (f fixture) same(t *testing.T, name string, call func(Client) outcome) outcome {
	t.Helper()
	var first outcome
	for i, transport := range f.transports {
		got := call(transport.client)
		if i == 0 {
			first = got
			continue
		}
		if !reflect.DeepEqual(got, first) {
			t.Errorf("%s over %s = %+v, want %+v (over %s)", name, transport.name, got, first, f.transports[0].name)
		}
	}
	return first
}

func kindOf(err error) contract.ErrorKind {
	var failure *Error
	if errors.As(err, &failure) {
		return failure.Kind
	}
	return ""
}

func TestDataAPIParityAcrossTransports(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	projectsOutcome := f.same(t, "Projects", func(c Client) outcome { return result(c.Projects(ctx)) })
	if listed, ok := projectsOutcome.Value.(contract.ProjectsResponse); !ok || projectsOutcome.Err != nil || len(listed.Projects) == 0 || listed.Projects[0].ID != f.root {
		t.Fatalf("Projects = %+v", projectsOutcome)
	}
	f.same(t, "Directories", func(c Client) outcome { return result(c.Directories(ctx, f.root)) })
	outside := f.same(t, "Directories outside", func(c Client) outcome { return result(c.Directories(ctx, filepath.Dir(f.root))) })
	if kindOf(outside.Err) != contract.KindInvalid {
		t.Errorf("Directories outside the roots error = %v, want invalid", outside.Err)
	}

	pages := []struct {
		name    string
		request contract.PageRequest
		kind    contract.ErrorKind
	}{
		{"tree", contract.PageRequest{Project: f.root}, ""},
		{"document", contract.PageRequest{Project: f.root, Path: "README.md", Required: true}, ""},
		{"nested", contract.PageRequest{Project: f.root, Path: "notes/today.md", Required: true}, ""},
		{"missing file", contract.PageRequest{Project: f.root, Path: "gone.md", Required: true}, contract.KindNotFound},
		{"no file", contract.PageRequest{Project: f.root, Required: true}, contract.KindInvalid},
		{"no project", contract.PageRequest{}, contract.KindInvalid},
		{"missing project", contract.PageRequest{Project: filepath.Join(f.root, "gone")}, contract.KindNotFound},
		{"bad commit", contract.PageRequest{Project: f.root, Path: "README.md", At: "HEAD"}, contract.KindInvalid},
	}
	for _, page := range pages {
		got := f.same(t, "Page "+page.name, func(c Client) outcome { return result(c.Page(ctx, page.request)) })
		if kindOf(got.Err) != page.kind {
			t.Errorf("Page %s error = %v, want kind %q", page.name, got.Err, page.kind)
		}
		data, ok := got.Value.(contract.PageData)
		if !ok || (got.Err != nil && data.Error != got.Err.Error()) {
			t.Errorf("Page %s = %+v, want the error in the page", page.name, got)
		}
	}

	formats := []struct {
		path string
		kind contract.ErrorKind
	}{
		{"data.json", ""},
		{"broken.json", contract.KindUnprocessable},
		{"README.md", contract.KindInvalid},
		{"gone.json", contract.KindNotFound},
	}
	for _, format := range formats {
		got := f.same(t, "Format "+format.path, func(c Client) outcome {
			return result(c.Format(ctx, contract.FormatRequest{Project: f.root, Path: format.path}))
		})
		if kindOf(got.Err) != format.kind {
			t.Errorf("Format %s error = %v, want kind %q", format.path, got.Err, format.kind)
		}
	}

	search := f.same(t, "SearchDocuments", func(c Client) outcome { return result(c.SearchDocuments(ctx, f.root)) })
	if documents, ok := search.Value.(contract.SearchDocumentsResponse); !ok || len(documents.Documents) != 5 {
		t.Errorf("SearchDocuments = %+v, want 5 documents", search)
	}
	f.same(t, "SearchDocuments without a project", func(c Client) outcome { return result(c.SearchDocuments(ctx, "")) })
}

func TestEventsParityAcrossTransports(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	invalid := f.same(t, "Events invalid lease", func(c Client) outcome {
		stream, err := c.Events(ctx, contract.EventsRequest{Project: f.root, Lease: "forever"})
		return outcome{Value: stream == nil, Err: err}
	})
	var failure *Error
	if !errors.As(invalid.Err, &failure) || failure.Kind != contract.KindInvalid || failure.Code != contract.CodeInvalidLease {
		t.Errorf("Events invalid lease error = %v, want invalid %s", invalid.Err, contract.CodeInvalidLease)
	}

	for _, transport := range f.transports {
		t.Run(transport.name, func(t *testing.T) {
			stream, err := transport.client.Events(ctx, contract.EventsRequest{Project: f.root})
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			status, err := stream.Next()
			if err != nil || status.Name != contract.EventStatus || status.Status == nil || status.Status.Mode != string(watch.ModeWatching) {
				t.Fatalf("first event = %+v, %v; want status watching", status, err)
			}
			name := transport.name + ".md"
			if err := os.WriteFile(filepath.Join(f.root, name), []byte("new\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			for {
				event, err := stream.Next()
				if err != nil {
					t.Fatal(err)
				}
				if event.Name == contract.EventChange && event.Change != nil && containsString(event.Change.Paths, name) {
					break
				}
			}
			stream.Close()
			if _, err := stream.Next(); err == nil {
				t.Error("Next after Close succeeded")
			}
		})
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestDaemonCallsParityAcrossTransports(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var daemons fixture
	for _, transport := range f.transports {
		if transport.daemon {
			daemons.transports = append(daemons.transports, transport)
			continue
		}
		if _, err := transport.client.DaemonStatus(ctx); !errors.Is(err, ErrNoDaemon) {
			t.Errorf("DaemonStatus over %s error = %v, want ErrNoDaemon", transport.name, err)
		}
		if _, err := transport.client.DaemonEvents(ctx); !errors.Is(err, ErrNoDaemon) {
			t.Errorf("DaemonEvents over %s error = %v, want ErrNoDaemon", transport.name, err)
		}
	}

	status := daemons.same(t, "DaemonStatus", func(c Client) outcome { return result(c.DaemonStatus(ctx)) })
	if got, ok := status.Value.(contract.DaemonStatus); !ok || !reflect.DeepEqual(got.Roots, []string{f.root}) {
		t.Errorf("DaemonStatus = %+v", status)
	}
	daemons.same(t, "DaemonRoots", func(c Client) outcome { return result(c.DaemonRoots(ctx)) })
	opened := daemons.same(t, "DaemonOpen within a root", func(c Client) outcome { return result(c.DaemonOpen(ctx, filepath.Join(f.root, "notes"))) })
	if got, ok := opened.Value.(contract.DaemonOpenResult); !ok || got.Added || got.Root != f.root || got.Project != filepath.Join(f.root, "notes") {
		t.Errorf("DaemonOpen = %+v", opened)
	}
	relative := daemons.same(t, "DaemonOpen relative", func(c Client) outcome { return result(c.DaemonOpen(ctx, "notes")) })
	var invalidRoot *Error
	if !errors.As(relative.Err, &invalidRoot) || invalidRoot.Code != contract.CodeInvalidRoot {
		t.Errorf("DaemonOpen relative error = %v, want %s", relative.Err, contract.CodeInvalidRoot)
	}
	daemons.same(t, "DaemonDirectories", func(c Client) outcome { return result(c.DaemonDirectories(ctx, f.root)) })
	missing := daemons.same(t, "DaemonDirectories missing", func(c Client) outcome { return result(c.DaemonDirectories(ctx, filepath.Join(f.root, "gone"))) })
	if kindOf(missing.Err) != contract.KindNotFound {
		t.Errorf("DaemonDirectories missing error = %v, want not found", missing.Err)
	}
	daemons.same(t, "DaemonEvents first", func(c Client) outcome {
		stream, err := c.DaemonEvents(ctx)
		if err != nil {
			return outcome{Err: err}
		}
		defer stream.Close()
		return result(stream.Next())
	})
}

func TestServerSentEventsParse(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(": servef\n\nevent: walkthrough\ndata: {\"project\":\"/p\",\"id\":\"tour\",\"title\":\"Tour\",\"created\":true}\n\n: heartbeat\n\nevent: other\ndata: {}\n\n"))
	name, data, err := readEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	event, ok, err := daemonEvent(name, data)
	if err != nil || !ok || !reflect.DeepEqual(event.Walkthrough, &contract.WalkthroughEvent{Project: "/p", ID: "tour", Title: "Tour", Created: true}) {
		t.Fatalf("event = %+v, %v, %v", event, ok, err)
	}
	name, data, err = readEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := daemonEvent(name, data); ok || err != nil {
		t.Errorf("unknown event = %v, %v; want skipped", ok, err)
	}
	if _, _, err := readEvent(reader); !errors.Is(err, io.EOF) {
		t.Errorf("end = %v, want io.EOF", err)
	}
}
