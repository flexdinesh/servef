//go:build darwin || linux

package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/flexdinesh/servef/pkg/contract"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/app"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/walkthrough"
)

// fakeNotices records announcements and hands each stream subscription a
// channel the test feeds.
type fakeNotices struct {
	mu         sync.Mutex
	announced  []announceRequest
	err        error
	stream     chan contract.WalkthroughEvent
	subscribed chan struct{}
	cancelled  chan struct{}
}

func newFakeNotices() *fakeNotices {
	return &fakeNotices{stream: make(chan contract.WalkthroughEvent, 4), subscribed: make(chan struct{}, 4), cancelled: make(chan struct{}, 4)}
}

func (f *fakeNotices) Announce(project, id string, created bool) (contract.WalkthroughEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return contract.WalkthroughEvent{}, f.err
	}
	f.announced = append(f.announced, announceRequest{Project: project, ID: id, Created: created})
	return contract.WalkthroughEvent{Project: project, ID: id, Title: "Title", Created: created}, nil
}

func (f *fakeNotices) Notices() (<-chan contract.WalkthroughEvent, func()) {
	f.subscribed <- struct{}{}
	return f.stream, func() { f.cancelled <- struct{}{} }
}

func (f *fakeNotices) requests() []announceRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]announceRequest(nil), f.announced...)
}

func (f *fakeNotices) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// serveControl serves handler on a private socket and returns a client.
func serveControl(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	socket := filepath.Join(socketDir(t), "daemon.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return NewClient(socket)
}

func testControl(t *testing.T, notices WalkthroughNotices) (*Controller, chan struct{}) {
	t.Helper()
	done := make(chan struct{})
	return &Controller{
		roots:     &projects.RootSet{},
		rootsFile: filepath.Join(t.TempDir(), "servef", "roots.json"),
		status:    Status{Protocol: ProtocolVersion, Version: "1.2.3"},
		stop:      func() {},
		done:      done,
		notices:   notices,
	}, done
}

func controlRequest(t *testing.T, client *Client, method, name string, header http.Header, body string) (int, http.Header, []byte) {
	t.Helper()
	request, err := http.NewRequest(method, "http://localhost"+controlPrefix+name, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for key, values := range header {
		request.Header[key] = values
	}
	response, err := client.http.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, response.Header, content
}

// eventStream is an open GET /_daemon/events.
type eventStream struct {
	response *http.Response
	reader   *bufio.Reader
	cancel   context.CancelFunc
}

func openEvents(t *testing.T, client *Client) *eventStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost"+controlPrefix+"events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.http.Do(request)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	stream := &eventStream{response: response, reader: bufio.NewReader(response.Body), cancel: cancel}
	t.Cleanup(stream.close)
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("GET /_daemon/events = %d %v", response.StatusCode, response.Header)
	}
	return stream
}

func (s *eventStream) close() {
	s.cancel()
	_ = s.response.Body.Close()
}

// next reads one event block (lines up to a blank line); io.EOF when the
// stream ends.
func (s *eventStream) next(t *testing.T) (string, error) {
	t.Helper()
	type result struct {
		block string
		err   error
	}
	read := make(chan result, 1)
	go func() {
		var block strings.Builder
		for {
			line, err := s.reader.ReadString('\n')
			if err != nil {
				read <- result{block.String(), err}
				return
			}
			if line == "\n" {
				read <- result{block.String(), nil}
				return
			}
			block.WriteString(line)
		}
	}()
	select {
	case r := <-read:
		return r.block, r.err
	case <-time.After(10 * time.Second):
		t.Fatal("no event within 10s")
		return "", nil
	}
}

func (s *eventStream) expect(t *testing.T, want string) {
	t.Helper()
	if got, err := s.next(t); err != nil || got != want {
		t.Fatalf("event = %q, %v; want %q", got, err, want)
	}
}

func (s *eventStream) expectEnd(t *testing.T) {
	t.Helper()
	if got, err := s.next(t); !errors.Is(err, io.EOF) {
		t.Fatalf("event = %q, %v; want the stream to end", got, err)
	}
}

func receive(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not happen", what)
	}
}

func TestControlEventsSendStatusThenWalkthroughs(t *testing.T) {
	notices := newFakeNotices()
	c, _ := testControl(t, notices)
	client := serveControl(t, c.Handler(http.NotFoundHandler()))

	stream := openEvents(t, client)
	stream.expect(t, ": servef\n")
	stream.expect(t, "event: status\ndata: {\"protocol\":1,\"version\":\"1.2.3\"}\n")
	receive(t, notices.subscribed, "subscription")
	notices.stream <- contract.WalkthroughEvent{Project: "/work/repo", ID: "tour", Title: "Tour", Created: true}
	stream.expect(t, "event: walkthrough\ndata: {\"project\":\"/work/repo\",\"id\":\"tour\",\"title\":\"Tour\",\"created\":true}\n")

	// Closed notices (live updates stopped) end the stream.
	close(notices.stream)
	stream.expectEnd(t)
	receive(t, notices.cancelled, "subscription cancel")
}

func TestControlEventsEndWithTheDaemonOrTheRequest(t *testing.T) {
	notices := newFakeNotices()
	c, done := testControl(t, notices)
	client := serveControl(t, c.Handler(http.NotFoundHandler()))

	first := openEvents(t, client)
	first.expect(t, ": servef\n")
	receive(t, notices.subscribed, "subscription")
	first.close()
	receive(t, notices.cancelled, "cancel after the client left")

	second := openEvents(t, client)
	second.expect(t, ": servef\n")
	second.expect(t, "event: status\ndata: {\"protocol\":1,\"version\":\"1.2.3\"}\n")
	close(done)
	second.expectEnd(t)
	receive(t, notices.cancelled, "cancel after the daemon stopped")
}

func TestControlEventsRefuseBrowserRequests(t *testing.T) {
	notices := newFakeNotices()
	c, _ := testControl(t, notices)
	client := serveControl(t, c.Handler(http.NotFoundHandler()))
	for _, header := range []http.Header{{"Origin": {"http://localhost"}}, {"Sec-Fetch-Site": {"same-origin"}}} {
		if code, _, body := controlRequest(t, client, http.MethodGet, "events", header, ""); code != http.StatusForbidden || !strings.Contains(string(body), "browserRequest") {
			t.Errorf("GET /_daemon/events with %v = %d %s, want 403 browserRequest", header, code, body)
		}
	}
	select {
	case <-notices.subscribed:
		t.Error("a refused request subscribed to notices")
	default:
	}
}

func TestControlEventsKeepTheDaemonFromIdling(t *testing.T) {
	const idleExit = 100 * time.Millisecond
	notices := newFakeNotices()
	c, _ := testControl(t, notices)
	fired := make(chan struct{}, 1)
	idle := newIdleTracker(idleExit, func() { fired <- struct{}{} })
	t.Cleanup(idle.stop)
	client := serveControl(t, idle.wrap(c.Handler(http.NotFoundHandler())))

	stream := openEvents(t, client)
	stream.expect(t, ": servef\n")
	select {
	case <-fired:
		t.Fatal("idle exit fired while a control stream was open")
	case <-time.After(5 * idleExit):
	}
	stream.close()
	receive(t, notices.cancelled, "stream end")
	receive(t, fired, "idle exit after the stream ended")
}

func TestControlRejectsWrongMethods(t *testing.T) {
	c, _ := testControl(t, newFakeNotices())
	client := serveControl(t, c.Handler(http.NotFoundHandler()))
	for name, test := range map[string]struct{ method, allow string }{
		"open":         {http.MethodGet, http.MethodPost},
		"walkthroughs": {http.MethodGet, http.MethodPost},
		"events":       {http.MethodPost, http.MethodGet},
	} {
		if code, header, _ := controlRequest(t, client, test.method, name, nil, ""); code != http.StatusMethodNotAllowed || header.Get("Allow") != test.allow {
			t.Errorf("%s /_daemon/%s = %d Allow %q, want 405 Allow %q", test.method, name, code, header.Get("Allow"), test.allow)
		}
	}
}

func TestControlAnnouncesWalkthroughs(t *testing.T) {
	notices := newFakeNotices()
	c, _ := testControl(t, notices)
	client := serveControl(t, c.Handler(http.NotFoundHandler()))

	code, _, body := controlRequest(t, client, http.MethodPost, "walkthroughs", jsonHeader, `{"project":"/work/repo","id":"tour","created":true}`)
	if code != http.StatusAccepted || strings.TrimSpace(string(body)) != `{"announced":true}` {
		t.Fatalf("POST /_daemon/walkthroughs = %d %s, want 202 announced", code, body)
	}
	if err := client.AnnounceWalkthrough(context.Background(), "/work/repo", "again", false); err != nil {
		t.Fatalf("AnnounceWalkthrough() = %v", err)
	}
	if want := []announceRequest{{"/work/repo", "tour", true}, {"/work/repo", "again", false}}; !reflect.DeepEqual(notices.requests(), want) {
		t.Fatalf("announced = %+v, want %+v", notices.requests(), want)
	}

	for name, body := range map[string]string{
		"unknown field": `{"project":"/work/repo","id":"tour","colour":1}`,
		"no id":         `{"project":"/work/repo"}`,
		"no project":    `{"id":"tour"}`,
		"not json":      `project=/work/repo`,
		"too large":     `{"project":"` + strings.Repeat("a", maxControlRequest) + `","id":"tour"}`,
	} {
		if code, _, response := controlRequest(t, client, http.MethodPost, "walkthroughs", jsonHeader, body); code != http.StatusBadRequest || !strings.Contains(string(response), "invalidRequest") {
			t.Errorf("POST %s = %d %s, want 400 invalidRequest", name, code, response)
		}
	}
	if code, _, response := controlRequest(t, client, http.MethodPost, "walkthroughs", nil, `{"project":"/work/repo","id":"tour"}`); code != http.StatusUnsupportedMediaType {
		t.Errorf("POST without JSON content type = %d %s, want 415", code, response)
	}

	for code, err := range map[string]error{
		"walkthroughUnavailable": &app.Error{Kind: contract.KindNotFound, Code: contract.CodeWalkthroughUnavailable, Message: "read"},
		"projectUnavailable":     &app.Error{Kind: contract.KindNotFound, Code: contract.CodeProjectUnavailable, Message: "resolve"},
	} {
		notices.setErr(err)
		err := client.AnnounceWalkthrough(context.Background(), "/work/repo", "tour", false)
		var failure *ControlError
		if !errors.As(err, &failure) || failure.Status != http.StatusNotFound || failure.Code != code || !strings.HasPrefix(err.Error(), "servef daemon: ") {
			t.Errorf("AnnounceWalkthrough() = %#v, want 404 %s", err, code)
		}
	}
}

func TestControlOpenReportsUnsavedRoots(t *testing.T) {
	c, _ := testControl(t, newFakeNotices())
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	c.rootsFile = filepath.Join(blocker, "roots.json")
	client := serveControl(t, c.Handler(http.NotFoundHandler()))
	dir := canonicalDir(t, t.TempDir(), "work")

	_, err := client.Open(context.Background(), dir)
	var failure *ControlError
	if !errors.As(err, &failure) || failure.Status != http.StatusInternalServerError || failure.Code != "rootsNotSaved" {
		t.Fatalf("Open() = %v, want 500 rootsNotSaved", err)
	}
	if got := c.roots.List(); !reflect.DeepEqual(got, []string{dir}) {
		t.Fatalf("roots = %v, want the unsaved root served", got)
	}
}

func TestRunOpensPathsCoveringServedRoots(t *testing.T) {
	config := daemonConfig(t)
	parent := canonicalDir(t, t.TempDir(), "code")
	inner := canonicalDir(t, parent, "inner")
	sibling := canonicalDir(t, t.TempDir(), "sibling")
	repo := canonicalDir(t, parent, "repo")
	canonicalDir(t, repo, ".git")
	nested := canonicalDir(t, repo, filepath.Join("docs", "guides"))
	excluded := canonicalDir(t, parent, filepath.Join("node_modules", "pkg"))
	goneInside := filepath.Join(parent, "unmounted")
	goneElsewhere := filepath.Join(sibling, "..", "elsewhere-unmounted")
	if err := SaveRoots(config.RootsFile, []string{inner, sibling, goneInside, goneElsewhere}); err != nil {
		t.Fatal(err)
	}
	d := startDaemon(t, config)
	ctx := context.Background()

	// Within a served root: nothing is added; the project is the folder.
	result, err := d.client.Open(ctx, inner+"/")
	if want := (OpenResult{Root: inner, Replaced: []string{}, Project: inner}); err != nil || !reflect.DeepEqual(result, want) {
		t.Fatalf("Open(inner) = %+v, %v; want %+v", result, err, want)
	}

	// Outside every root: added, replacing the roots inside it and dropping
	// skipped roots inside it from the saved file.
	result, err = d.client.Open(ctx, parent)
	if want := (OpenResult{Root: parent, Added: true, Replaced: []string{inner}, Project: parent}); err != nil || !reflect.DeepEqual(result, want) {
		t.Fatalf("Open(parent) = %+v, %v; want %+v", result, err, want)
	}
	saved, err := LoadRoots(config.RootsFile)
	if want := []string{parent, sibling, goneElsewhere}; err != nil || !reflect.DeepEqual(saved, want) {
		t.Fatalf("saved roots = %v, %v; want %v", saved, err, want)
	}
	if roots, err := d.client.Roots(ctx); err != nil || !reflect.DeepEqual(roots, []string{parent, sibling}) {
		t.Fatalf("Roots() = %v, %v", roots, err)
	}

	// A folder inside a repository opens the repository.
	code, body := d.request(t, http.MethodPost, "/_daemon/open", jsonHeader, `{"path":"`+nested+`"}`)
	var opened OpenResult
	if err := json.Unmarshal(body, &opened); err != nil || code != http.StatusOK || opened.Project != repo || opened.Root != parent || opened.Added || opened.Replaced == nil {
		t.Fatalf("POST /_daemon/open nested = %d %s; want 200 project %s", code, body, repo)
	}

	// A new sibling root: 201 with an empty replaced array.
	other := canonicalDir(t, t.TempDir(), "other")
	code, body = d.request(t, http.MethodPost, "/_daemon/open", jsonHeader, `{"path":"`+other+`"}`)
	if code != http.StatusCreated || !strings.Contains(string(body), `"replaced":[]`) || !strings.Contains(string(body), `"project":"`+other+`"`) {
		t.Fatalf("POST /_daemon/open other = %d %s; want 201 replaced []", code, body)
	}

	file := filepath.Join(other, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		body string
		code int
		want string
	}{
		"relative":        {`{"path":"code"}`, http.StatusBadRequest, "invalidRoot"},
		"missing":         {`{"path":"` + filepath.Join(parent, "nope") + `"}`, http.StatusBadRequest, "invalidRoot"},
		"not a directory": {`{"path":"` + file + `"}`, http.StatusBadRequest, "invalidRoot"},
		"unknown field":   {`{"root":"` + parent + `"}`, http.StatusBadRequest, "invalidRequest"},
		"excluded":        {`{"path":"` + excluded + `"}`, http.StatusNotFound, "projectUnavailable"},
	} {
		if code, response := d.request(t, http.MethodPost, "/_daemon/open", jsonHeader, test.body); code != test.code || !strings.Contains(string(response), test.want) {
			t.Errorf("POST %s = %d %s, want %d %s", name, code, response, test.code, test.want)
		}
	}
	if code, response := d.request(t, http.MethodPost, "/_daemon/open", http.Header{"Content-Type": {"text/plain"}}, `{"path":"`+parent+`"}`); code != http.StatusUnsupportedMediaType {
		t.Errorf("POST text/plain = %d %s, want 415", code, response)
	}
	if roots, err := d.client.Roots(ctx); err != nil || !reflect.DeepEqual(roots, []string{parent, sibling, other}) {
		t.Fatalf("Roots() after rejections = %v, %v", roots, err)
	}
}

// TestRunAnnouncesWalkthroughsOnTheControlStream goes through the real web
// app: the daemon reads the walkthrough from its own store.
func TestRunAnnouncesWalkthroughsOnTheControlStream(t *testing.T) {
	config := daemonConfig(t)
	root := canonicalDir(t, t.TempDir(), "work")
	if err := SaveRoots(config.RootsFile, []string{root}); err != nil {
		t.Fatal(err)
	}
	directory, err := walkthrough.DefaultDirectory()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := walkthrough.NewStore(directory).Put(walkthrough.Walkthrough{ID: "tour", Project: root, Title: "Tour", Steps: []walkthrough.Step{{Title: "Start"}}, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	d := startDaemon(t, config)
	ctx := context.Background()

	stream := openEvents(t, d.client)
	stream.expect(t, ": servef\n")
	stream.expect(t, fmt.Sprintf("event: status\ndata: {\"protocol\":%d,\"version\":%q}\n", ProtocolVersion, d.status(t).Version))

	if err := d.client.AnnounceWalkthrough(ctx, root, "tour", false); err != nil {
		t.Fatalf("AnnounceWalkthrough() = %v", err)
	}
	stream.expect(t, "event: walkthrough\ndata: {\"project\":\""+root+"\",\"id\":\"tour\",\"title\":\"Tour\",\"created\":false}\n")

	for code, target := range map[string][2]string{
		"walkthroughUnavailable": {root, "missing"},
		"projectUnavailable":     {filepath.Dir(root), "tour"},
	} {
		err := d.client.AnnounceWalkthrough(ctx, target[0], target[1], true)
		var failure *ControlError
		if !errors.As(err, &failure) || failure.Status != http.StatusNotFound || failure.Code != code {
			t.Errorf("AnnounceWalkthrough(%v) = %v, want 404 %s", target, err, code)
		}
	}

	if err := d.client.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	stream.expectEnd(t)
	if err := d.wait(t); err != nil {
		t.Fatal(err)
	}
}

func (d *testDaemon) status(t *testing.T) Status {
	t.Helper()
	status, err := d.client.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return status
}
