package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/control"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/walkthrough"
	webui "github.com/flexdinesh/servef/internal/web"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// publishRepo is a repository with one commit of doc.md, the project a
// walkthrough targets.
func publishRepo(t *testing.T) (string, plumbing.Hash) {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := gogit.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	writePublishFile(t, filepath.Join(root, "doc.md"), "# Doc\nline two\n")
	writePublishFile(t, filepath.Join(root, "sub", "notes.md"), "# Notes\n")
	if _, err := worktree.Add("doc.md"); err != nil {
		t.Fatal(err)
	}
	signature := &object.Signature{Name: "servef", Email: "servef@example.com", When: time.Unix(1700000000, 0)}
	hash, err := worktree.Commit("doc", &gogit.CommitOptions{Author: signature, Committer: signature})
	if err != nil {
		t.Fatal(err)
	}
	return root, hash
}

func writePublishFile(t *testing.T, name, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func publishDraft(t *testing.T, draft walkthrough.Draft) string {
	t.Helper()
	encoded, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func tourDraft(root string, hash plumbing.Hash) walkthrough.Draft {
	return walkthrough.Draft{
		Project: root,
		ID:      "tour",
		Title:   "Tour",
		Steps: []walkthrough.Step{
			{Title: "Working file", File: &walkthrough.FileTarget{Path: "doc.md", Lines: &walkthrough.Lines{1, 2}}},
			{Title: "At a commit", File: &walkthrough.FileTarget{Path: "doc.md", At: hash.String()[:7]}},
		},
	}
}

// testPublishOptions keeps a test's store, control registry, and daemon
// socket private, reports no desktop app, and never opens a real browser.
func testPublishOptions(t *testing.T, source string) (publishOptions, chan string) {
	t.Helper()
	opened := make(chan string, 4)
	return publishOptions{
		source:         source,
		depth:          5,
		walkthroughDir: filepath.Join(t.TempDir(), "walkthroughs"),
		controlDir:     filepath.Join(t.TempDir(), "servers"),
		socket:         filepath.Join(t.TempDir(), "daemon.sock"),
		openBrowser:    func(target string) error { opened <- target; return nil },
		getenv:         func(string) string { return "" },
		installed:      func(context.Context) (bool, error) { return false, nil },
		openApp: func(_ context.Context, target string) error {
			t.Errorf("opened %s in the desktop app without one", target)
			return nil
		},
	}, opened
}

func runPublish(t *testing.T, opts publishOptions, stdin string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := publishWalkthrough(context.Background(), opts, strings.NewReader(stdin), &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

// serveWalkthroughs runs a registered servef for root that reads walkthroughs
// from directory, and returns its origin.
func serveWalkthroughs(t *testing.T, opts publishOptions, root, directory string) string {
	t.Helper()
	roots, err := projects.NewRootSet([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	app, err := webui.New(webui.Config{Roots: roots, Depth: 5, WalkthroughDirectory: directory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewUnstartedServer(nil)
	server.Start()
	t.Cleanup(server.Close)
	host, rawPort, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := control.NewRegistry(opts.controlDir).Register(host, port)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close() })
	server.Config.Handler = instance.Handler(app)
	return server.URL
}

func TestPublishWalkthroughStoresCheckedWalkthrough(t *testing.T) {
	root, hash := publishRepo(t)
	draft := tourDraft(root, hash)
	opts, _ := testPublishOptions(t, filepath.Join(t.TempDir(), "walkthrough.json"))
	writePublishFile(t, opts.source, publishDraft(t, draft))

	stdout, stderr, err := runPublish(t, opts, "")
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "published tour (created) for "+root) || !strings.Contains(stdout, "no running servef shows this project; run: servef "+root) {
		t.Errorf("stdout = %q", stdout)
	}
	store := walkthrough.NewStore(opts.walkthroughDir)
	stored, err := store.Get(root, "tour")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Steps[0].File.Expect != "# Doc" || stored.Steps[1].File.At != hash.String() {
		t.Errorf("stored steps = %+v %+v; want expect recorded and the hash resolved", stored.Steps[0].File, stored.Steps[1].File)
	}

	// Republishing from standard input replaces it and keeps createdAt.
	opts.source = "-"
	draft.Title = "Tour again"
	stdout, stderr, err = runPublish(t, opts, publishDraft(t, draft))
	if err != nil {
		t.Fatalf("republish: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "published tour (replaced)") {
		t.Errorf("stdout = %q", stdout)
	}
	again, err := store.Get(root, "tour")
	if err != nil {
		t.Fatal(err)
	}
	if again.Title != "Tour again" || !again.CreatedAt.Equal(stored.CreatedAt) {
		t.Errorf("republished = %q created %v; want new title and created %v", again.Title, again.CreatedAt, stored.CreatedAt)
	}
}

func TestPublishWalkthroughRejectsInvalidDrafts(t *testing.T) {
	root, hash := publishRepo(t)
	opts, _ := testPublishOptions(t, "-")
	store := walkthrough.NewStore(opts.walkthroughDir)
	assertNothingStored := func(project string) {
		t.Helper()
		if list, err := store.List(project); err != nil || len(list) != 0 {
			t.Errorf("stored after rejection: %+v %v", list, err)
		}
	}

	broken := walkthrough.Draft{Project: root, Title: "Broken", Steps: []walkthrough.Step{
		{Title: "missing", File: &walkthrough.FileTarget{Path: "nope.md"}},
		{Title: "past the end", File: &walkthrough.FileTarget{Path: "doc.md", Lines: &walkthrough.Lines{1, 9}}},
		{Title: "unknown commit", File: &walkthrough.FileTarget{Path: "doc.md", At: "0000000"}},
	}}
	_, stderr, err := runPublish(t, opts, publishDraft(t, broken))
	if !errors.Is(err, errNotPublished) {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{
		"invalid walkthrough:\n",
		"  steps[0].file.path: is not a file servef shows",
		"  steps[1].file.lines: end 9 is past the file's last line, 2\n",
		"  steps[2].file.at: no commit matches 0000000\n",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	assertNothingStored(root)

	inside := tourDraft(filepath.Join(root, "sub"), hash)
	inside.Steps = inside.Steps[:1]
	_, stderr, err = runPublish(t, opts, publishDraft(t, inside))
	if !errors.Is(err, errNotPublished) || !strings.Contains(stderr, "project: is a folder inside a repository; use the repository root (git rev-parse --show-toplevel)") {
		t.Errorf("inside repository = %v\n%s", err, stderr)
	}
	assertNothingStored(filepath.Join(root, "sub"))

	missing := tourDraft(filepath.Join(root, "missing"), hash)
	if _, stderr, err = runPublish(t, opts, publishDraft(t, missing)); !errors.Is(err, errNotPublished) || !strings.Contains(stderr, "project: must be an existing absolute directory") {
		t.Errorf("missing project = %v\n%s", err, stderr)
	}

	if _, _, err = runPublish(t, opts, `{"project":"`+root+`","title":"t","steps":[{"title":"s"}],"colour":1}`); !errors.Is(err, walkthrough.ErrInvalidJSON) {
		t.Errorf("unknown field = %v, want invalid JSON", err)
	}
	if _, _, err = runPublish(t, opts, strings.Repeat(" ", walkthrough.MaxRequestBytes+1)); err == nil || err.Error() != "a walkthrough is at most 1048576 bytes" {
		t.Errorf("oversized = %v", err)
	}
	assertNothingStored(root)
}

func TestPublishWalkthroughFindsRunningViewer(t *testing.T) {
	root, hash := publishRepo(t)
	opts, opened := testPublishOptions(t, "-")
	origin := serveWalkthroughs(t, opts, root, opts.walkthroughDir)
	draft := tourDraft(root, hash)
	draft.Open = true
	want := origin + "/view?" + url.Values{"project": {root}, "wt": {"tour"}}.Encode()

	stdout, stderr, err := runPublish(t, opts, publishDraft(t, draft))
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "url: "+want+"\n") {
		t.Errorf("stdout = %q, want url %s", stdout, want)
	}
	select {
	case target := <-opened:
		if target != want {
			t.Errorf("opened %s, want %s", target, want)
		}
	default:
		t.Error("open: true did not open the viewer")
	}

	// --no-open and SSH sessions never open it.
	noOpen := opts
	noOpen.noOpen = true
	ssh := opts
	ssh.getenv = func(name string) string {
		if name == "SSH_CONNECTION" {
			return "10.0.0.1 22 10.0.0.2 22"
		}
		return ""
	}
	for _, variant := range []publishOptions{noOpen, ssh} {
		if _, stderr, err := runPublish(t, variant, publishDraft(t, draft)); err != nil {
			t.Fatalf("publish: %v\n%s", err, stderr)
		}
		select {
		case target := <-opened:
			t.Errorf("opened %s", target)
		default:
		}
	}

	// --json prints one object with the URL.
	asJSON := opts
	asJSON.json = true
	asJSON.noOpen = true
	stdout, _, err = runPublish(t, asJSON, publishDraft(t, draft))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil || strings.Count(stdout, "\n") != 1 || !strings.Contains(stdout, want) {
		t.Fatalf("json output = %q: %v", stdout, err)
	}
	if result["id"] != "tour" || result["project"] != root || result["created"] != false || result["url"] != want || len(result) != 4 {
		t.Errorf("json = %v", result)
	}
}

func TestPublishWalkthroughWarnsAboutAnotherStore(t *testing.T) {
	root, hash := publishRepo(t)
	opts, opened := testPublishOptions(t, "-")
	origin := serveWalkthroughs(t, opts, root, filepath.Join(t.TempDir(), "elsewhere"))
	draft := tourDraft(root, hash)
	draft.Open = true

	stdout, stderr, err := runPublish(t, opts, publishDraft(t, draft))
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, stderr)
	}
	if want := "servef at " + origin + " serves this project but keeps walkthroughs elsewhere; POST to " + origin + "/api/walkthroughs\n"; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if !strings.Contains(stdout, "no running servef shows this project, so nothing opened") {
		t.Errorf("stdout = %q", stdout)
	}
	select {
	case target := <-opened:
		t.Errorf("opened %s without a viewer", target)
	default:
	}

	// Without a viewer, --json omits the URL.
	opts.json = true
	stdout, _, err = runPublish(t, opts, publishDraft(t, draft))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout, `"url"`) {
		t.Errorf("json output = %q; want no url", stdout)
	}
}

// fakeDaemon answers POST /_daemon/walkthroughs on a short private socket
// with status and code, and records each request body.
type fakeDaemon struct {
	socket   string
	requests chan string
	mu       sync.Mutex
	status   int
	code     string
}

func (f *fakeDaemon) answer(status int, code string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.code = status, code
}

func serveFakeDaemon(t *testing.T) *fakeDaemon {
	t.Helper()
	dir, err := os.MkdirTemp("", "sf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	fake := &fakeDaemon{socket: filepath.Join(dir, "daemon.sock"), requests: make(chan string, 8), status: http.StatusAccepted}
	listener, err := net.Listen("unix", fake.socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fake.requests <- r.Method + " " + r.URL.Path + " " + string(body)
		fake.mu.Lock()
		status, code := fake.status, fake.code
		fake.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusAccepted {
			_, _ = io.WriteString(w, `{"announced":true}`)
			return
		}
		_, _ = io.WriteString(w, `{"error":"unavailable","code":"`+code+`"}`)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return fake
}

func TestPublishWalkthroughAnnouncesToTheDaemon(t *testing.T) {
	if daemonSupported() != nil {
		t.Skip("no daemon on this platform")
	}
	root, hash := publishRepo(t)
	opts, _ := testPublishOptions(t, "-")
	fake := serveFakeDaemon(t)
	opts.socket = fake.socket
	draft := publishDraft(t, tourDraft(root, hash))

	stdout, stderr, err := runPublish(t, opts, draft)
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, stderr)
	}
	want := `POST /_daemon/walkthroughs {"project":"` + root + `","id":"tour","created":true}`
	select {
	case got := <-fake.requests:
		if got != want {
			t.Errorf("daemon request = %s, want %s", got, want)
		}
	default:
		t.Fatal("publish did not announce to the daemon")
	}
	if stderr != "" || !strings.HasPrefix(stdout, "published tour (created) for "+root+"\n") {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}

	// A daemon serving the project from another store gets a warning; one
	// not serving it is ignored.
	fake.answer(http.StatusNotFound, "walkthroughUnavailable")
	if _, stderr, err = runPublish(t, opts, draft); err != nil {
		t.Fatal(err)
	}
	if want := "servef daemon at " + fake.socket + " serves this project but keeps walkthroughs elsewhere\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	fake.answer(http.StatusNotFound, "projectUnavailable")
	if _, stderr, err = runPublish(t, opts, draft); err != nil || stderr != "" {
		t.Errorf("projectUnavailable: stderr = %q, %v; want silent", stderr, err)
	}
	if len(fake.requests) != 2 {
		t.Errorf("daemon got %d more requests, want 2", len(fake.requests))
	}
}

func TestPublishWalkthroughOpensDaemonServedProjectsInTheApp(t *testing.T) {
	if daemonSupported() != nil {
		t.Skip("no daemon on this platform")
	}
	root, hash := publishRepo(t)
	opts, browserOpened := testPublishOptions(t, "-")
	opts.socket = serveFakeDaemon(t).socket
	opts.installed = func(context.Context) (bool, error) { return true, nil }
	appOpened := make(chan string, 4)
	opts.openApp = func(_ context.Context, target string) error { appOpened <- target; return nil }
	// A registered server also shows it; the app is preferred.
	origin := serveWalkthroughs(t, opts, root, opts.walkthroughDir)
	draft := tourDraft(root, hash)
	draft.Open = true
	want := "servef://local/view?" + url.Values{"project": {root}, "wt": {"tour"}}.Encode()

	stdout, stderr, err := runPublish(t, opts, publishDraft(t, draft))
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "url: "+want+"\n") || stderr != "" {
		t.Errorf("stdout = %q, stderr = %q; want url %s", stdout, stderr, want)
	}
	select {
	case target := <-appOpened:
		if target != want {
			t.Errorf("app opened %s, want %s", target, want)
		}
	default:
		t.Error("open: true did not open the desktop app")
	}
	select {
	case target := <-browserOpened:
		t.Errorf("browser opened %s", target)
	default:
	}

	// --json carries the servef:// URL; --no-open opens nothing.
	asJSON := opts
	asJSON.json = true
	asJSON.noOpen = true
	stdout, _, err = runPublish(t, asJSON, publishDraft(t, draft))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil || result["url"] != want {
		t.Errorf("json output = %q: %v; want url %s", stdout, err, want)
	}
	if len(appOpened) != 0 || len(browserOpened) != 0 {
		t.Errorf("--no-open opened %d app and %d browser targets", len(appOpened), len(browserOpened))
	}

	// A failed app open falls back to a running server in the browser.
	opts.openApp = func(context.Context, string) error { return errors.New("no handler") }
	viewer := origin + "/view?" + url.Values{"project": {root}, "wt": {"tour"}}.Encode()
	stdout, stderr, err = runPublish(t, opts, publishDraft(t, draft))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "url: "+viewer+"\n") || stderr != "could not open the servef desktop app: no handler\n" {
		t.Errorf("stdout = %q, stderr = %q; want url %s", stdout, stderr, viewer)
	}
	select {
	case target := <-browserOpened:
		if target != viewer {
			t.Errorf("browser opened %s, want %s", target, viewer)
		}
	default:
		t.Error("app open failure did not open the browser")
	}
}

func TestPublishWalkthroughWithoutTheAppKeepsBrowserViewers(t *testing.T) {
	if daemonSupported() != nil {
		t.Skip("no daemon on this platform")
	}
	root, hash := publishRepo(t)
	draft := tourDraft(root, hash)
	draft.Open = true
	for _, test := range []struct {
		name      string
		installed bool
		status    int
	}{
		{"announced without the app", false, http.StatusAccepted},
		{"app without an announcing daemon", true, http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			opts, opened := testPublishOptions(t, "-")
			fake := serveFakeDaemon(t)
			fake.answer(test.status, "projectUnavailable")
			opts.socket = fake.socket
			opts.installed = func(context.Context) (bool, error) { return test.installed, nil }
			origin := serveWalkthroughs(t, opts, root, opts.walkthroughDir)
			want := origin + "/view?" + url.Values{"project": {root}, "wt": {"tour"}}.Encode()

			stdout, stderr, err := runPublish(t, opts, publishDraft(t, draft))
			if err != nil {
				t.Fatalf("publish: %v\n%s", err, stderr)
			}
			if !strings.Contains(stdout, "url: "+want+"\n") {
				t.Errorf("stdout = %q, want url %s", stdout, want)
			}
			select {
			case target := <-opened:
				if target != want {
					t.Errorf("browser opened %s, want %s", target, want)
				}
			default:
				t.Error("open: true did not open the browser")
			}
		})
	}
}

func TestPublishWalkthroughInsideSSHPrintsTheRemoteLink(t *testing.T) {
	if daemonSupported() != nil {
		t.Skip("no daemon on this platform")
	}
	root, hash := publishRepo(t)
	opts, browserOpened := testPublishOptions(t, "-")
	opts.socket = serveFakeDaemon(t).socket
	opts.getenv = sshEnv
	opts.remoteHints = func() remoteHints {
		return remoteHints{Hostname: "devbox", User: "dee", Addr: "10.0.0.5", Port: 22}
	}
	// The app on this host and a browser server are ignored inside SSH.
	opts.installed = func(context.Context) (bool, error) { return true, nil }
	serveWalkthroughs(t, opts, root, opts.walkthroughDir)
	draft := tourDraft(root, hash)
	draft.Open = true
	want := "servef://ssh/devbox/view?" + url.Values{"project": {root}, "wt": {"tour"}}.Encode() + "&ssh-hostname=devbox&ssh-user=dee&ssh-addr=10.0.0.5"

	stdout, stderr, err := runPublish(t, opts, publishDraft(t, draft))
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, stderr)
	}
	if wantOut := "url: " + want + "\nnothing opened inside SSH; open the link on your computer; it needs the servef desktop app\n"; !strings.HasSuffix(stdout, wantOut) || stderr != "" {
		t.Errorf("stdout = %q, stderr = %q; want suffix %q", stdout, stderr, wantOut)
	}
	if len(browserOpened) != 0 {
		t.Errorf("opened %s", <-browserOpened)
	}

	asJSON := opts
	asJSON.json = true
	stdout, _, err = runPublish(t, asJSON, publishDraft(t, draft))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil || result["url"] != want {
		t.Errorf("json output = %q: %v; want url %s", stdout, err, want)
	}
}

func TestPublishWalkthroughInsideSSHWithoutTheDaemonProbesServers(t *testing.T) {
	root, hash := publishRepo(t)
	opts, opened := testPublishOptions(t, "-")
	opts.getenv = sshEnv
	opts.remoteHints = func() remoteHints {
		t.Error("read remote hints without an announcing daemon")
		return remoteHints{}
	}
	origin := serveWalkthroughs(t, opts, root, opts.walkthroughDir)
	draft := tourDraft(root, hash)
	draft.Open = true
	want := origin + "/view?" + url.Values{"project": {root}, "wt": {"tour"}}.Encode()

	stdout, stderr, err := runPublish(t, opts, publishDraft(t, draft))
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "url: "+want+"\n") {
		t.Errorf("stdout = %q, want url %s", stdout, want)
	}
	if len(opened) != 0 {
		t.Errorf("opened %s inside SSH", <-opened)
	}
}

func TestWalkthroughCommandParsesArgumentsAndFlags(t *testing.T) {
	parse := func(args ...string) publishOptions {
		t.Helper()
		var got publishOptions
		command := newWalkthroughCommand(func(_ context.Context, opts publishOptions, _ io.Reader, _, _ io.Writer) error {
			got = opts
			return nil
		})
		command.SetOut(io.Discard)
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := parse("publish", "-", "--depth", "3", "--no-open", "--json"); got.source != "-" || got.depth != 3 || !got.noOpen || !got.json {
		t.Errorf("options = %+v", got)
	}
	if got := parse("publish", "walkthrough.json"); got.source != "walkthrough.json" || got.depth != 5 || got.noOpen || got.json {
		t.Errorf("default options = %+v", got)
	}
	for _, args := range [][]string{{"publish"}, {"publish", "a.json", "b.json"}, {"publish", "-", "--depth", "-1"}} {
		command := newWalkthroughCommand(func(context.Context, publishOptions, io.Reader, io.Writer, io.Writer) error {
			t.Errorf("publish called for %v", args)
			return nil
		})
		command.SetOut(io.Discard)
		command.SetArgs(args)
		if err := command.Execute(); err == nil {
			t.Errorf("%v succeeded", args)
		}
	}
}

func TestWalkthroughSchemaPrintsTheSchema(t *testing.T) {
	var output bytes.Buffer
	command := newWalkthroughCommand(publishWalkthrough)
	command.SetOut(&output)
	command.SetArgs([]string{"schema"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), walkthrough.Schema) {
		t.Errorf("schema output differs from walkthrough.Schema")
	}
}
