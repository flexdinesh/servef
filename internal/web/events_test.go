package web

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/watch"
)

func newLiveApp(t *testing.T, root string) *App {
	t.Helper()
	options := watch.DefaultOptions()
	options.Debounce = 30 * time.Millisecond
	options.MaxWait = 200 * time.Millisecond
	app, err := New(Config{Roots: []string{root}, Depth: 5, Exclusions: []string{".git", "node_modules", "vendor"}, LiveUpdates: watch.NewHub(options)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	return app
}

// liveServer serves app, ending its streams before closing, as servef's
// shutdown does: Close waits for every request.
func liveServer(t *testing.T, app *App, writeTimeout time.Duration) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(app)
	server.Config.WriteTimeout = writeTimeout
	server.Start()
	t.Cleanup(func() {
		app.StopLiveUpdates()
		server.Close()
	})
	return server
}

// serverSentEvent is one parsed event of a stream.
type serverSentEvent struct {
	name string
	data string
}

type eventStream struct {
	t      *testing.T
	events chan serverSentEvent
	body   io.ReadCloser
}

func openEvents(t *testing.T, server *httptest.Server, project string) *eventStream {
	t.Helper()
	response, err := http.Get(server.URL + "/api/events?project=" + url.QueryEscape(project))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q", got)
	}
	stream := &eventStream{t: t, events: make(chan serverSentEvent, 16), body: response.Body}
	t.Cleanup(func() { _ = response.Body.Close() })
	go func() {
		defer close(stream.events)
		scanner := bufio.NewScanner(response.Body)
		var current serverSentEvent
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				if current.name != "" {
					stream.events <- current
				}
				current = serverSentEvent{}
			case strings.HasPrefix(line, "event: "):
				current.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				current.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return stream
}

func (s *eventStream) next(name string, into any) {
	s.t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case event, ok := <-s.events:
			if !ok {
				s.t.Fatalf("stream ended before %q", name)
			}
			if event.name != name {
				continue
			}
			if err := json.Unmarshal([]byte(event.data), into); err != nil {
				s.t.Fatal(err)
			}
			return
		case <-deadline:
			s.t.Fatalf("no %q event", name)
		}
	}
}

func TestEventsStreamStatusThenChanges(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# One\n")
	app := newLiveApp(t, root)
	server := liveServer(t, app, 0)

	stream := openEvents(t, server, app.roots[0])
	var status statusEvent
	stream.next("status", &status)
	if status.Mode != watch.ModeWatching || status.Reason != "" {
		t.Fatalf("status = %+v, want watching", status)
	}

	writeTestFile(t, filepath.Join(root, "README.md"), "# Two\n")
	var change changeEvent
	stream.next("change", &change)
	if !reflect.DeepEqual(change, changeEvent{Paths: []string{"README.md"}}) {
		t.Fatalf("change = %+v, want README.md written", change)
	}

	writeTestFile(t, filepath.Join(root, "docs", "new.md"), "x")
	stream.next("change", &change)
	if !change.Tree || !slices.Contains(change.Paths, "docs") {
		t.Fatalf("change = %+v, want docs created", change)
	}
}

func TestEventsStreamGitStateChanges(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeTestFile(t, filepath.Join(root, "main.go"), "package main\n")
	app := newLiveApp(t, root)
	server := liveServer(t, app, 0)

	stream := openEvents(t, server, app.roots[0])
	var status statusEvent
	stream.next("status", &status)
	writeTestFile(t, filepath.Join(root, ".git", "refs", "heads", "main"), "0123\n")
	var change changeEvent
	stream.next("change", &change)
	if !change.Git || len(change.Paths) != 0 {
		t.Fatalf("change = %+v, want git only", change)
	}
}

func TestEventsOutliveTheServerWriteTimeout(t *testing.T) {
	root := t.TempDir()
	app := newLiveApp(t, root)
	server := liveServer(t, app, 50*time.Millisecond)

	stream := openEvents(t, server, app.roots[0])
	var status statusEvent
	stream.next("status", &status)
	time.Sleep(200 * time.Millisecond)
	writeTestFile(t, filepath.Join(root, "late.md"), "x")
	var change changeEvent
	stream.next("change", &change)
	if !reflect.DeepEqual(change.Paths, []string{"late.md"}) {
		t.Fatalf("change = %+v", change)
	}
}

func TestStopLiveUpdatesEndsStreams(t *testing.T) {
	root := t.TempDir()
	app := newLiveApp(t, root)
	server := liveServer(t, app, 0)

	stream := openEvents(t, server, app.roots[0])
	var status statusEvent
	stream.next("status", &status)
	app.StopLiveUpdates()
	select {
	case _, ok := <-stream.events:
		for ok {
			_, ok = <-stream.events
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stream still open after StopLiveUpdates")
	}
}

func TestEventsRejectUnknownProjectsAndOtherHosts(t *testing.T) {
	root := t.TempDir()
	app := newLiveApp(t, root)

	response := request(t, app, "/api/events?project="+url.QueryEscape(filepath.Join(root, "missing")))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
	var body errorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error == "" {
		t.Fatalf("body = %s, want an errorResponse", response.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/events?project="+url.QueryEscape(app.roots[0]), nil)
	req.Host = "evil.example"
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("other host status = %d, want 403", recorder.Code)
	}
}

func writeTestFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
