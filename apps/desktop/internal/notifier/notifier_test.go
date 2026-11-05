package notifier

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/servef/pkg/client"
	"github.com/flexdinesh/servef/pkg/connection"
	"github.com/flexdinesh/servef/pkg/contract"
)

const projectA, projectB = "/code/a", "/code/b"

// fakeServer answers the routes a notifier uses, as servef does. The real server
// is the e2e's: this module cannot import servef's internals.
type fakeServer struct {
	mu        sync.Mutex
	streams   map[*fakeStream]struct{}
	opened    []string
	changes   contract.ChangesResponse
	summaries map[string][]contract.WalkthroughSummary
	// refuse answers /api/events with this status and error body.
	refuseStatus int
	refuseCode   string
}

type fakeStream struct {
	project, lease string
	events         chan string
}

func newFakeServer() *fakeServer {
	return &fakeServer{streams: map[*fakeStream]struct{}{}, changes: contract.ChangesResponse{Repository: true, Files: []contract.ChangedFile{}}, summaries: map[string][]contract.WalkthroughSummary{}}
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	switch r.URL.Path {
	case "/api/events":
		f.serveEvents(w, r, project)
	case "/api/changes":
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(f.changes)
	case "/api/walkthroughs":
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(append([]contract.WalkthroughSummary{}, f.summaries[project]...))
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeServer) serveEvents(w http.ResponseWriter, r *http.Request, project string) {
	f.mu.Lock()
	if f.refuseStatus != 0 {
		status, code := f.refuseStatus, f.refuseCode
		f.opened = append(f.opened, project)
		f.mu.Unlock()
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(contract.ErrorResponse{Error: "refused", Code: code})
		return
	}
	stream := &fakeStream{project: project, lease: r.URL.Query().Get("lease"), events: make(chan string, 16)}
	f.streams[stream] = struct{}{}
	f.opened = append(f.opened, project+" "+stream.lease)
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		delete(f.streams, stream)
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	_, _ = fmt.Fprint(w, sse(contract.EventStatus, contract.StatusEvent{Mode: "watching"}))
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-stream.events:
			if event == "" {
				return
			}
			_, _ = fmt.Fprint(w, event)
			flusher.Flush()
		}
	}
}

func sse(name string, value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return "event: " + name + "\ndata: " + string(data) + "\n\n"
}

// send delivers an event to project's open streams and reports how many took it.
func (f *fakeServer) send(project, event string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	sent := 0
	for stream := range f.streams {
		if stream.project == project {
			stream.events <- event
			sent++
		}
	}
	return sent
}

func (f *fakeServer) streamsOf(project string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var leases []string
	for stream := range f.streams {
		if stream.project == project {
			leases = append(leases, stream.lease)
		}
	}
	return leases
}

func (f *fakeServer) streamCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.streams)
}

func eventually(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type transport struct {
	name string
	// connect returns a client of server over this transport.
	connect func(t *testing.T, server http.Handler) client.Client
}

// transports is the two ways the app reaches a server. A notifier must behave
// the same over each (decision 0099).
func transports() []transport {
	return []transport{
		{"unix", func(t *testing.T, server http.Handler) client.Client {
			t.Helper()
			directory, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("unix", filepath.Join(directory, "s"))
			if err != nil {
				t.Fatal(err)
			}
			running := &http.Server{Handler: server}
			go func() { _ = running.Serve(listener) }()
			t.Cleanup(func() { _ = running.Close() })
			return client.NewUnix(listener.Addr().String())
		}},
		{"remote", func(t *testing.T, server http.Handler) client.Client {
			t.Helper()
			running := httptest.NewUnstartedServer(server)
			running.EnableHTTP2 = true
			running.StartTLS()
			t.Cleanup(func() {
				running.CloseClientConnections()
				running.Close()
			})
			certificate, err := x509.ParseCertificate(running.TLS.Certificates[0].Certificate[0])
			if err != nil {
				t.Fatal(err)
			}
			address, ok := running.Listener.Addr().(*net.TCPAddr)
			if !ok {
				t.Fatalf("listener = %v", running.Listener.Addr())
			}
			return client.NewRemote(connection.TLS{Hosts: []string{"127.0.0.1"}, Port: address.Port, Credential: connection.Credential{Fingerprint: connection.FingerprintOf(certificate.RawSubjectPublicKeyInfo)}})
		}},
	}
}

// harness is a Notifier over a fake server, with what it showed.
type harness struct {
	server   *fakeServer
	notifier *Notifier

	mu      sync.Mutex
	notices []Notice
	focused bool
	now     time.Time
}

func newHarness(t *testing.T, over transport) *harness {
	t.Helper()
	h := &harness{server: newFakeServer(), now: time.Unix(1_000_000, 0)}
	server := over.connect(t, h.server)
	h.start(t, Options{Connect: func(context.Context, error) (client.Client, error) { return server, nil }})
	return h
}

func (h *harness) start(t *testing.T, options Options) {
	t.Helper()
	options.Connection = "local"
	options.Settle = 30 * time.Millisecond
	options.MinBackoff = 10 * time.Millisecond
	options.Now = func() time.Time {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.now
	}
	options.Focused = func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.focused
	}
	options.Show = func(notice Notice) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.notices = append(h.notices, notice)
	}
	h.notifier = New(options)
	t.Cleanup(h.notifier.Stop)
}

func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.now = h.now.Add(d)
}

func (h *harness) setFocused(focused bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.focused = focused
}

func (h *harness) shown() []Notice {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.notices)
}

func (h *harness) waitTitle(t *testing.T, title string) Notice {
	t.Helper()
	var found Notice
	eventually(t, "the notice "+title, func() bool {
		for _, notice := range h.shown() {
			if notice.Title == title {
				found = notice
				return true
			}
		}
		return false
	})
	return found
}

func (h *harness) titles() []string {
	var titles []string
	for _, notice := range h.shown() {
		titles = append(titles, notice.Title)
	}
	return titles
}

func head(branch, commit, parent, subject string) string {
	return sse(contract.EventHead, contract.HeadEvent{Branch: branch, Commit: commit, Parent: parent, Subject: subject})
}

func (h *harness) waitStream(t *testing.T, project, lease string) {
	t.Helper()
	eventually(t, project+" followed at "+lease, func() bool { return slices.Contains(h.server.streamsOf(project), lease) })
}

// baseline sends project's first head and waits for it to be taken: the next
// head then moves it.
func (h *harness) baseline(t *testing.T, project, commit string) {
	t.Helper()
	if h.server.send(project, head("main", commit, "", "Base")) == 0 {
		t.Fatalf("no stream of %s", project)
	}
	time.Sleep(20 * time.Millisecond)
}

var everyKind = Settings{Commits: true, Branches: true, Edits: true, Walkthroughs: true}
var gitOnly = Settings{Commits: true, Branches: true, Walkthroughs: true}

func TestNotifierOverEachTransport(t *testing.T) {
	for _, over := range transports() {
		t.Run(over.name, func(t *testing.T) {
			h := newHarness(t, over)
			h.notifier.Configure(projectA, everyKind)
			h.waitStream(t, projectA, leaseOpen)

			// The first head is the baseline: nothing to report.
			h.baseline(t, projectA, hashA)
			if got := h.titles(); len(got) != 0 {
				t.Fatalf("a baseline showed %v", got)
			}

			h.server.send(projectA, head("main", hashB, hashA, "Fix the parser"))
			commit := h.waitTitle(t, "New commit on main")
			if commit.Body != "a · bbbbbbb Fix the parser" || !strings.Contains(commit.Route, "commit="+hashB) || !strings.Contains(commit.Route, "project=%2Fcode%2Fa") {
				t.Errorf("commit notice = %+v", commit)
			}

			h.server.send(projectA, head("feature", hashB, hashA, "Fix the parser"))
			if notice := h.waitTitle(t, "Switched to feature"); notice.Kind != KindBranch {
				t.Errorf("switch notice = %+v", notice)
			}

			// Edits report once settled, through the working tree's changes.
			h.server.mu.Lock()
			h.server.changes.Files = []contract.ChangedFile{{Path: "a.md", Status: "modified"}}
			h.server.mu.Unlock()
			h.server.send(projectA, sse(contract.EventChange, contract.ChangeEvent{Paths: []string{"a.md"}}))
			edits := h.waitTitle(t, "1 file changed")
			if !strings.Contains(edits.Route, "review=working") {
				t.Errorf("edits notice = %+v", edits)
			}

			// A walkthrough is reported from the list the event asks for.
			h.server.mu.Lock()
			h.server.summaries[projectA] = []contract.WalkthroughSummary{{ID: "tour", Title: "Auth flow", UpdatedAt: "2026-01-01T00:00:00Z"}}
			h.server.mu.Unlock()
			h.server.send(projectA, sse(contract.EventWalkthrough, contract.WalkthroughEvent{Project: projectA, ID: "tour", Title: "Auth flow", Created: true}))
			walkthrough := h.waitTitle(t, "New walkthrough")
			if walkthrough.Body != "a · Auth flow" || !strings.Contains(walkthrough.Route, "wt=tour") {
				t.Errorf("walkthrough notice = %+v", walkthrough)
			}

			h.notifier.Stop()
			eventually(t, "every stream to end after Stop", func() bool { return h.server.streamCount() == 0 })
		})
	}
}

func TestIgnoredFilesAndUnchangedChangesAreSilent(t *testing.T) {
	h := newHarness(t, transports()[0])
	h.server.mu.Lock()
	h.server.changes.Files = []contract.ChangedFile{{Path: "a.md", Status: "modified"}}
	h.server.mu.Unlock()
	h.notifier.Configure(projectA, everyKind)
	h.waitStream(t, projectA, leaseOpen)
	h.baseline(t, projectA, hashA)

	// A build wrote dist/: it is watched but ignored, so the set of changes is
	// what it was.
	h.server.send(projectA, sse(contract.EventChange, contract.ChangeEvent{Paths: []string{"dist/x.js"}}))
	time.Sleep(100 * time.Millisecond)
	if got := h.titles(); len(got) != 0 {
		t.Fatalf("an ignored file showed %v", got)
	}

	h.server.mu.Lock()
	h.server.changes.Files = append(h.server.changes.Files, contract.ChangedFile{Path: "b.md", Status: "added"})
	h.server.mu.Unlock()
	h.server.send(projectA, sse(contract.EventChange, contract.ChangeEvent{Paths: []string{"b.md"}}))
	if notice := h.waitTitle(t, "1 file changed"); notice.Body != "a · b.md" {
		t.Errorf("notice = %+v", notice)
	}
}

func TestFoldersReportEveryPathOnceSettled(t *testing.T) {
	h := newHarness(t, transports()[0])
	h.server.changes = contract.ChangesResponse{Files: []contract.ChangedFile{}}
	h.notifier.Configure(projectA, everyKind)
	h.waitStream(t, projectA, leaseOpen)
	h.server.send(projectA, sse(contract.EventChange, contract.ChangeEvent{Paths: []string{"a.md"}}))
	h.server.send(projectA, sse(contract.EventChange, contract.ChangeEvent{Paths: []string{"a.md", "b.md"}}))
	notice := h.waitTitle(t, "2 files changed")
	if notice.Body != "a · a.md, b.md" || strings.Contains(notice.Route, "review=") {
		t.Errorf("notice = %+v", notice)
	}
}

func TestFocusSuppressesOnlyTheProjectInView(t *testing.T) {
	h := newHarness(t, transports()[0])
	h.notifier.Configure(projectB, gitOnly)
	h.waitStream(t, projectB, leaseWatched)
	h.notifier.Configure(projectA, gitOnly)
	h.waitStream(t, projectA, leaseWatched)
	h.baseline(t, projectA, hashA)
	h.baseline(t, projectB, hashA)

	h.setFocused(true)
	h.server.send(projectA, head("main", hashB, hashA, "Seen"))
	h.server.send(projectB, head("main", hashB, hashA, "Elsewhere"))
	if notice := h.waitTitle(t, "New commit on main"); notice.Project != projectB {
		t.Fatalf("notice = %+v, want one from the other project", notice)
	}
	time.Sleep(50 * time.Millisecond)
	if got := h.titles(); len(got) != 1 {
		t.Fatalf("shown %v, want only the other project's", got)
	}

	// Suppression spends none of the rate limit.
	h.setFocused(false)
	h.server.send(projectA, head("main", hashC, hashB, "Unseen"))
	eventually(t, "the project's notice once focus left", func() bool { return len(h.shown()) == 2 })
	if got := h.shown()[1]; got.Project != projectA || got.Body != "a · ccccccc Unseen" {
		t.Errorf("notice = %+v", got)
	}
}

func TestSettingsChooseStreamsLeasesAndKinds(t *testing.T) {
	h := newHarness(t, transports()[0])
	// Off: nothing is followed.
	h.notifier.Configure(projectA, Settings{})
	time.Sleep(30 * time.Millisecond)
	if got := h.server.streamCount(); got != 0 {
		t.Fatalf("%d streams with every kind off", got)
	}

	// Git kinds hold a watched lease, on every project opened this run.
	h.notifier.Configure(projectA, gitOnly)
	h.waitStream(t, projectA, leaseWatched)
	h.notifier.Configure(projectB, gitOnly)
	h.waitStream(t, projectB, leaseWatched)
	if got := h.server.streamsOf(projectA); !slices.Equal(got, []string{leaseWatched}) {
		t.Errorf("the earlier project = %v", got)
	}

	// File Edits holds the current project open; the head survives the change.
	h.baseline(t, projectB, hashA)
	h.notifier.Configure(projectB, everyKind)
	h.waitStream(t, projectB, leaseOpen)
	eventually(t, "the watched stream to end", func() bool { return slices.Equal(h.server.streamsOf(projectB), []string{leaseOpen}) })
	h.server.send(projectB, head("main", hashB, hashA, "After the lease"))
	h.waitTitle(t, "New commit on main")

	// File Edits alone follows only the project in view.
	h.notifier.Configure(projectB, Settings{Edits: true})
	eventually(t, "the other project to stop", func() bool { return len(h.server.streamsOf(projectA)) == 0 })

	// A kind turned off is not shown.
	h.advance(time.Minute)
	h.notifier.Configure(projectB, Settings{Branches: true})
	h.waitStream(t, projectB, leaseWatched)
	h.server.send(projectB, head("main", hashC, hashB, "Quiet"))
	h.server.send(projectB, head("other", hashC, hashB, "Quiet"))
	h.waitTitle(t, "Switched to other")
	commits := 0
	for _, title := range h.titles() {
		if title == "New commit on main" {
			commits++
		}
	}
	if commits != 1 {
		t.Errorf("a commit with its setting off was shown: %v", h.titles())
	}

	// All off stops following.
	h.notifier.Configure(projectB, Settings{})
	eventually(t, "every stream to end", func() bool { return h.server.streamCount() == 0 })
}

func TestReconnectCatchesUpOnceAndKeepsItsBaseline(t *testing.T) {
	h := newHarness(t, transports()[0])
	h.server.summaries[projectA] = []contract.WalkthroughSummary{{ID: "old", Title: "Old", UpdatedAt: "2026-01-01T00:00:00Z"}}
	h.notifier.Configure(projectA, everyKind)
	h.waitStream(t, projectA, leaseOpen)
	h.baseline(t, projectA, hashA)

	// While the stream is gone, HEAD moves, a file changes, and a walkthrough
	// is published.
	h.server.mu.Lock()
	h.server.changes.Files = []contract.ChangedFile{{Path: "a.md", Status: "modified"}}
	h.server.summaries[projectA] = append(h.server.summaries[projectA], contract.WalkthroughSummary{ID: "new", Title: "New", UpdatedAt: "2026-02-01T00:00:00Z"})
	h.server.mu.Unlock()
	h.server.send(projectA, "")
	h.waitTitle(t, "New walkthrough")
	h.waitTitle(t, "1 file changed")
	h.waitStream(t, projectA, leaseOpen)
	h.server.send(projectA, head("main", hashC, hashB, "While away"))
	notice := h.waitTitle(t, "main moved")
	if notice.Body != "a · ccccccc While away" {
		t.Errorf("catch-up = %+v", notice)
	}
}

func TestIdentityFailureStopsUntilThePageAsksAgain(t *testing.T) {
	for name, refuse := range map[string]func(*harness) func(){
		"a refused token": func(h *harness) func() {
			h.server.mu.Lock()
			defer h.server.mu.Unlock()
			h.server.refuseStatus, h.server.refuseCode = http.StatusUnauthorized, contract.CodeUnauthorized
			return func() {
				h.server.mu.Lock()
				defer h.server.mu.Unlock()
				h.server.refuseStatus = 0
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, transports()[0])
			lift := refuse(h)
			h.notifier.Configure(projectA, gitOnly)
			opened := func() int {
				h.server.mu.Lock()
				defer h.server.mu.Unlock()
				return len(h.server.opened)
			}
			eventually(t, "the refused attempt", func() bool { return opened() == 1 })
			time.Sleep(100 * time.Millisecond)
			if got := opened(); got != 1 {
				t.Fatalf("%d attempts after an identity failure, want 1", got)
			}
			lift()
			h.notifier.Configure(projectA, gitOnly)
			h.waitStream(t, projectA, leaseWatched)
		})
	}
}

func TestConnectionNoLongerSavedStops(t *testing.T) {
	h := &harness{server: newFakeServer(), now: time.Unix(1_000_000, 0)}
	var mu sync.Mutex
	attempts := 0
	h.start(t, Options{Connect: func(context.Context, error) (client.Client, error) {
		mu.Lock()
		defer mu.Unlock()
		attempts++
		return nil, ErrNoConnection
	}})
	h.notifier.Configure(projectA, gitOnly)
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if attempts != 1 {
		t.Errorf("%d attempts, want 1", attempts)
	}
}

func TestProjectTheServerLacksIsForgotten(t *testing.T) {
	h := newHarness(t, transports()[0])
	h.server.mu.Lock()
	h.server.refuseStatus, h.server.refuseCode = http.StatusNotFound, "projectUnavailable"
	h.server.mu.Unlock()
	h.notifier.Configure(projectA, gitOnly)
	eventually(t, "the project to be dropped", func() bool {
		h.notifier.mu.Lock()
		defer h.notifier.mu.Unlock()
		return len(h.notifier.recent) == 0 && len(h.notifier.followers) == 0
	})
}

func TestFailuresReachConnectAndRetry(t *testing.T) {
	h := &harness{server: newFakeServer(), now: time.Unix(1_000_000, 0)}
	gone := errors.New("the daemon has gone")
	var mu sync.Mutex
	var failures []error
	server := transports()[0].connect(t, h.server)
	h.start(t, Options{Connect: func(_ context.Context, failed error) (client.Client, error) {
		mu.Lock()
		defer mu.Unlock()
		failures = append(failures, failed)
		if len(failures) == 1 {
			return nil, gone
		}
		return server, nil
	}})
	h.notifier.Configure(projectA, gitOnly)
	h.waitStream(t, projectA, leaseWatched)
	mu.Lock()
	defer mu.Unlock()
	if failures[0] != nil || !errors.Is(failures[1], gone) {
		t.Errorf("Connect saw %v", failures)
	}
}

func TestFollowsAtMostSixteenProjects(t *testing.T) {
	h := newHarness(t, transports()[0])
	for i := range maxFollowed + 3 {
		h.notifier.Configure(fmt.Sprintf("/code/p%d", i), gitOnly)
	}
	eventually(t, "the oldest projects to stop", func() bool { return h.server.streamCount() == maxFollowed })
	if got := h.server.streamsOf("/code/p0"); len(got) != 0 {
		t.Errorf("the oldest project is still followed: %v", got)
	}
}
