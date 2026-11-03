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

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/flexdinesh/servef/internal/watch"
)

func newLiveApp(t *testing.T, root string) *App {
	t.Helper()
	options := watch.DefaultOptions()
	options.Debounce = 30 * time.Millisecond
	options.MaxWait = 200 * time.Millisecond
	app, err := New(Config{Roots: testRoots(t, root), Depth: 5, LiveUpdates: watch.NewHub(options)})
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
	return openEventsQuery(t, server, url.Values{"project": {project}})
}

func openEventsQuery(t *testing.T, server *httptest.Server, query url.Values) *eventStream {
	t.Helper()
	response, err := http.Get(server.URL + "/api/events?" + query.Encode())
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

// take returns the stream's next event of any name.
func (s *eventStream) take() serverSentEvent {
	s.t.Helper()
	select {
	case event, ok := <-s.events:
		if !ok {
			s.t.Fatal("stream ended")
		}
		return event
	case <-time.After(3 * time.Second):
		s.t.Fatal("no event")
	}
	return serverSentEvent{}
}

// nextHead skips to the next head event, checking that it follows a git
// change or, for the first, the status.
func (s *eventStream) nextHead() headEvent {
	s.t.Helper()
	var previous serverSentEvent
	for {
		event := s.take()
		if event.name != "head" {
			previous = event
			continue
		}
		var head headEvent
		if err := json.Unmarshal([]byte(event.data), &head); err != nil {
			s.t.Fatal(err)
		}
		var change changeEvent
		switch {
		case head.Previous == nil && previous.name == "status":
		case previous.name == "change" && json.Unmarshal([]byte(previous.data), &change) == nil && change.Git:
		default:
			s.t.Fatalf("head %s follows %+v, want the status or a git change", event.data, previous)
		}
		return head
	}
}

// liveRepo is a repository with one commit on master, its git config
// isolated from the host's.
func liveRepo(t *testing.T) (string, *gogit.Repository, plumbing.Hash) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := gogit.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "README.md"), "# One\n")
	return root, repo, commitAll(t, repo, "Initial commit\n\nBody.")
}

func commitAll(t *testing.T, repo *gogit.Repository, message string) plumbing.Hash {
	t.Helper()
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := worktree.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	hash, err := worktree.Commit(message, &gogit.CommitOptions{Author: &object.Signature{Name: "servef", Email: "servef@example.com", When: time.Unix(1700000000, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestEventsStreamStatusThenChanges(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# One\n")
	app := newLiveApp(t, root)
	server := liveServer(t, app, 0)

	stream := openEvents(t, server, app.roots.List()[0])
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

	stream := openEvents(t, server, app.roots.List()[0])
	var status statusEvent
	stream.next("status", &status)
	writeTestFile(t, filepath.Join(root, ".git", "refs", "heads", "main"), "0123\n")
	var change changeEvent
	stream.next("change", &change)
	if !change.Git || len(change.Paths) != 0 {
		t.Fatalf("change = %+v, want git only", change)
	}
}

func TestEventsSendHeadFirstThenWhenItMoves(t *testing.T) {
	root, repo, initial := liveRepo(t)
	app := newLiveApp(t, root)
	server := liveServer(t, app, 0)

	stream := openEvents(t, server, app.roots.List()[0])
	if event := stream.take(); event.name != "status" {
		t.Fatalf("first event = %+v, want status", event)
	}
	event := stream.take()
	if event.name != "head" {
		t.Fatalf("second event = %+v, want head", event)
	}
	want := `{"branch":"master","commit":"` + initial.String() + `","subject":"Initial commit","previous":null}`
	if event.data != want {
		t.Fatalf("baseline head = %s, want %s", event.data, want)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := worktree.Checkout(&gogit.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("feature"), Create: true}); err != nil {
		t.Fatal(err)
	}
	switched := headEvent{Branch: "feature", Commit: initial.String(), Subject: "Initial commit", Previous: &headRef{Branch: "master", Commit: initial.String()}}
	if got := stream.nextHead(); !reflect.DeepEqual(got, switched) {
		t.Fatalf("head after switch = %+v (previous %+v), want %+v", got, got.Previous, switched)
	}

	writeTestFile(t, filepath.Join(root, "README.md"), "# Two\n")
	next := commitAll(t, repo, "Second commit")
	committed := headEvent{Branch: "feature", Commit: next.String(), Subject: "Second commit", Previous: &headRef{Branch: "feature", Commit: initial.String()}}
	if got := stream.nextHead(); !reflect.DeepEqual(got, committed) {
		t.Fatalf("head after commit = %+v (previous %+v), want %+v", got, got.Previous, committed)
	}
}

func TestEventsSendNoHeadForFolders(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# One\n")
	app := newLiveApp(t, root)
	server := liveServer(t, app, 0)

	stream := openEvents(t, server, app.roots.List()[0])
	if event := stream.take(); event.name != "status" {
		t.Fatalf("first event = %+v, want status", event)
	}
	writeTestFile(t, filepath.Join(root, "README.md"), "# Two\n")
	if event := stream.take(); event.name != "change" {
		t.Fatalf("event after status = %+v, want the change and no head", event)
	}
}

func TestEventsWatchedLeaseFollowsGitStateOnly(t *testing.T) {
	root, repo, initial := liveRepo(t)
	app := newLiveApp(t, root)
	server := liveServer(t, app, 0)

	stream := openEventsQuery(t, server, url.Values{"project": {app.roots.List()[0]}, "lease": {"watched"}})
	event := stream.take()
	var status statusEvent
	if event.name != "status" || json.Unmarshal([]byte(event.data), &status) != nil || status.Mode != watch.ModeWatching {
		t.Fatalf("first event = %+v, want watching status", event)
	}
	if event := stream.take(); event.name != "head" || !strings.Contains(event.data, initial.String()) || !strings.HasSuffix(event.data, `"previous":null}`) {
		t.Fatalf("second event = %+v, want the baseline head", event)
	}

	// A working-tree edit alone sends nothing to a watched stream.
	writeTestFile(t, filepath.Join(root, "notes.md"), "x")
	time.Sleep(200 * time.Millisecond)
	next := commitAll(t, repo, "Add notes")
	for {
		event := stream.take()
		if event.name == "head" {
			var head headEvent
			if err := json.Unmarshal([]byte(event.data), &head); err != nil {
				t.Fatal(err)
			}
			if head.Commit != next.String() {
				t.Fatalf("head = %+v, want %s", head, next)
			}
			return
		}
		var change changeEvent
		if event.name != "change" || json.Unmarshal([]byte(event.data), &change) != nil || !change.Git || change.Tree || change.Overflow || len(change.Paths) != 0 {
			t.Fatalf("event = %+v, want git-only changes", event)
		}
	}
}

func TestEventsRejectInvalidLeases(t *testing.T) {
	root := t.TempDir()
	app := newLiveApp(t, root)

	// The lease is checked before the project.
	for _, project := range []string{app.roots.List()[0], filepath.Join(root, "missing")} {
		response := request(t, app, "/api/events?lease=forever&project="+url.QueryEscape(project))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", project, response.Code)
		}
		if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
			t.Fatalf("Content-Type = %q", got)
		}
		if body := decodeError(t, response); body.Code != "invalidLease" || body.Error == "" {
			t.Fatalf("body = %+v, want invalidLease", body)
		}
	}

	server := liveServer(t, app, 0)
	for _, lease := range []string{"open", "watched"} {
		stream := openEventsQuery(t, server, url.Values{"project": {app.roots.List()[0]}, "lease": {lease}})
		var status statusEvent
		stream.next("status", &status)
	}
}

func TestEventsOutliveTheServerWriteTimeout(t *testing.T) {
	root := t.TempDir()
	app := newLiveApp(t, root)
	server := liveServer(t, app, 50*time.Millisecond)

	stream := openEvents(t, server, app.roots.List()[0])
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

	stream := openEvents(t, server, app.roots.List()[0])
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

	req := httptest.NewRequest(http.MethodGet, "/api/events?project="+url.QueryEscape(app.roots.List()[0]), nil)
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
