package watch

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func testOptions() Options {
	return Options{
		Debounce:        40 * time.Millisecond,
		MaxWait:         200 * time.Millisecond,
		Grace:           80 * time.Millisecond,
		PollInterval:    30 * time.Millisecond,
		MaxPollInterval: 120 * time.Millisecond,
		MaxDirectories:  64,
		MaxEntries:      128,
		MaxProjects:     2,
		MaxPaths:        8,
	}
}

// dirsTarget watches every directory below root.
func dirsTarget(t *testing.T, root string) Target {
	t.Helper()
	return Target{Root: root, Directories: func() ([]string, int, error) {
		var dirs []string
		entries := 0
		err := filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if name != root {
				entries++
			}
			if entry.IsDir() {
				dirs = append(dirs, name)
			}
			return nil
		})
		return dirs, entries, err
	}}
}

func tempRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func write(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// next waits for the subscription's next update.
func next(t *testing.T, s *Subscription) (*Status, *Change) {
	t.Helper()
	select {
	case <-s.Ready():
		return s.Next()
	case <-time.After(3 * time.Second):
		t.Fatal("no update")
		return nil, nil
	}
}

func nextStatus(t *testing.T, s *Subscription) Status {
	t.Helper()
	status, _ := next(t, s)
	if status == nil {
		t.Fatal("update has no status")
	}
	return *status
}

// nextChange waits for a change, skipping status-only updates.
func nextChange(t *testing.T, s *Subscription) Change {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-s.Ready():
			if _, change := s.Next(); change != nil {
				return *change
			}
		case <-deadline:
			t.Fatal("no change")
		}
	}
}

func quiet(t *testing.T, s *Subscription, wait time.Duration) {
	t.Helper()
	select {
	case <-s.Ready():
		status, change := s.Next()
		t.Fatalf("unexpected update: %v %v", status, change)
	case <-time.After(wait):
	}
}

func subscribe(t *testing.T, hub *Hub, target Target) *Subscription {
	t.Helper()
	s, err := hub.Subscribe(target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func newTestHub(t *testing.T) *Hub {
	t.Helper()
	hub := NewHub(testOptions())
	t.Cleanup(hub.Close)
	return hub
}

func TestSubscribeReportsWatchingThenDebouncedChanges(t *testing.T) {
	root := tempRoot(t)
	write(t, filepath.Join(root, "a.md"), "one")
	hub := newTestHub(t)
	s := subscribe(t, hub, dirsTarget(t, root))
	if status := nextStatus(t, s); status.Mode != ModeWatching {
		t.Fatalf("status = %+v, want watching", status)
	}

	write(t, filepath.Join(root, "a.md"), "two")
	write(t, filepath.Join(root, "a.md"), "three")
	change := nextChange(t, s)
	if !reflect.DeepEqual(change.Paths, []string{"a.md"}) || change.Tree || change.Git || change.Overflow {
		t.Fatalf("change = %+v, want one write to a.md", change)
	}
	quiet(t, s, 150*time.Millisecond)
}

func TestCreatedFilesAndDirectoriesChangeTheTreeAndAreWatched(t *testing.T) {
	root := tempRoot(t)
	hub := newTestHub(t)
	s := subscribe(t, hub, dirsTarget(t, root))
	nextStatus(t, s)

	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	change := nextChange(t, s)
	if !change.Tree || !slices.Contains(change.Paths, "docs") {
		t.Fatalf("change = %+v, want docs created", change)
	}

	// The new directory is watched once its batch is sent.
	write(t, filepath.Join(root, "docs", "guide.md"), "x")
	change = nextChange(t, s)
	if !change.Tree || !slices.Contains(change.Paths, "docs/guide.md") {
		t.Fatalf("change = %+v, want docs/guide.md created", change)
	}

	if err := os.Remove(filepath.Join(root, "docs", "guide.md")); err != nil {
		t.Fatal(err)
	}
	change = nextChange(t, s)
	if !change.Tree || !slices.Contains(change.Paths, "docs/guide.md") {
		t.Fatalf("change = %+v, want docs/guide.md removed", change)
	}
}

func TestContinuousChangesFlushByMaxWait(t *testing.T) {
	root := tempRoot(t)
	hub := newTestHub(t)
	s := subscribe(t, hub, dirsTarget(t, root))
	nextStatus(t, s)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			case <-time.After(10 * time.Millisecond):
				_ = os.WriteFile(filepath.Join(root, "log.txt"), []byte(strings.Repeat("x", i)), 0o644)
			}
		}
	}()
	started := time.Now()
	nextChange(t, s)
	close(stop)
	<-done
	// Writes every 10 ms never leave 40 ms of quiet; MaxWait (200 ms) flushes.
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("first batch after %v, want about MaxWait", elapsed)
	}
}

func TestManyChangesOverflow(t *testing.T) {
	root := tempRoot(t)
	hub := newTestHub(t)
	s := subscribe(t, hub, dirsTarget(t, root))
	nextStatus(t, s)

	for i := range 20 {
		write(t, filepath.Join(root, "f"+strings.Repeat("x", i)), "x")
	}
	change := nextChange(t, s)
	for change.Overflow == false {
		change = nextChange(t, s)
	}
	if len(change.Paths) > testOptions().MaxPaths {
		t.Fatalf("paths = %d, want at most %d", len(change.Paths), testOptions().MaxPaths)
	}
}

func TestOverlappingProjectsShareDirectoriesAndBothReceive(t *testing.T) {
	outer := tempRoot(t)
	inner := filepath.Join(outer, "repo")
	write(t, filepath.Join(inner, "main.go"), "package main")
	hub := newTestHub(t)
	outerSub := subscribe(t, hub, dirsTarget(t, outer))
	innerSub := subscribe(t, hub, dirsTarget(t, inner))
	nextStatus(t, outerSub)
	nextStatus(t, innerSub)

	hub.mu.Lock()
	owners := len(hub.owners[inner])
	hub.mu.Unlock()
	if owners != 2 {
		t.Fatalf("owners of shared directory = %d, want 2", owners)
	}

	write(t, filepath.Join(inner, "main.go"), "package main // edit")
	if change := nextChange(t, outerSub); !reflect.DeepEqual(change.Paths, []string{"repo/main.go"}) {
		t.Fatalf("outer change = %+v", change)
	}
	if change := nextChange(t, innerSub); !reflect.DeepEqual(change.Paths, []string{"main.go"}) {
		t.Fatalf("inner change = %+v", change)
	}
}

func TestSameProjectSubscribersShareOneProject(t *testing.T) {
	root := tempRoot(t)
	hub := newTestHub(t)
	first := subscribe(t, hub, dirsTarget(t, root))
	second := subscribe(t, hub, dirsTarget(t, root))
	nextStatus(t, first)
	nextStatus(t, second)
	hub.mu.Lock()
	projects := len(hub.projects)
	hub.mu.Unlock()
	if projects != 1 {
		t.Fatalf("projects = %d, want 1", projects)
	}
	write(t, filepath.Join(root, "a.md"), "x")
	nextChange(t, first)
	nextChange(t, second)
}

func TestLastSubscriberLeavingStopsTheProjectAfterGrace(t *testing.T) {
	root := tempRoot(t)
	hub := newTestHub(t)
	s, err := hub.Subscribe(dirsTarget(t, root))
	if err != nil {
		t.Fatal(err)
	}
	nextStatus(t, s)
	s.Close()

	// A return within the grace period keeps the project.
	again := subscribe(t, hub, dirsTarget(t, root))
	nextStatus(t, again)
	time.Sleep(2 * testOptions().Grace)
	if count := projectCount(hub); count != 1 {
		t.Fatalf("projects = %d after return, want 1", count)
	}

	again.Close()
	deadline := time.Now().Add(time.Second)
	for projectCount(hub) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("project still watched after grace")
		}
		time.Sleep(10 * time.Millisecond)
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if len(hub.owners) != 0 || hub.backend != nil {
		t.Fatalf("owners = %d, backend open = %v; want nothing watched", len(hub.owners), hub.backend != nil)
	}
}

func projectCount(hub *Hub) int {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	return len(hub.projects)
}

func TestProjectsBeyondTheCapAreOffUnlessOneIsIdle(t *testing.T) {
	hub := newTestHub(t)
	first, second, third := tempRoot(t), tempRoot(t), tempRoot(t)
	a := subscribe(t, hub, dirsTarget(t, first))
	nextStatus(t, a)
	b, err := hub.Subscribe(dirsTarget(t, second))
	if err != nil {
		t.Fatal(err)
	}
	nextStatus(t, b)

	c := subscribe(t, hub, dirsTarget(t, third))
	if status := nextStatus(t, c); status.Mode != ModeOff || status.Reason == "" {
		t.Fatalf("status = %+v, want off with a reason", status)
	}

	// An idle project in its grace period gives up its slot.
	b.Close()
	d := subscribe(t, hub, dirsTarget(t, third))
	if status := nextStatus(t, d); status.Mode != ModeWatching {
		t.Fatalf("status = %+v, want watching", status)
	}
}

func TestLargeProjectsPoll(t *testing.T) {
	root := tempRoot(t)
	for i := range 70 {
		if err := os.MkdirAll(filepath.Join(root, "d", strings.Repeat("x", i+1)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	hub := newTestHub(t)
	s := subscribe(t, hub, dirsTarget(t, root))
	status := nextStatus(t, s)
	if status.Mode != ModePolling || !strings.Contains(status.Reason, "watch limit") {
		t.Fatalf("status = %+v, want polling past the directory limit", status)
	}

	write(t, filepath.Join(root, "d", "x", "new.md"), "x")
	change := nextChange(t, s)
	if !change.Tree || !reflect.DeepEqual(change.Paths, []string{"d/x/new.md"}) {
		t.Fatalf("change = %+v, want d/x/new.md created", change)
	}
	time.Sleep(20 * time.Millisecond)
	write(t, filepath.Join(root, "d", "x", "new.md"), "longer")
	change = nextChange(t, s)
	if change.Tree || !reflect.DeepEqual(change.Paths, []string{"d/x/new.md"}) {
		t.Fatalf("change = %+v, want d/x/new.md written", change)
	}
}

func TestWatchFailureFallsBackToPolling(t *testing.T) {
	root := tempRoot(t)
	hub := newTestHub(t)
	hub.newBackend = func(func(event), func()) (backend, error) {
		return failingBackend{err: syscall.ENOSPC}, nil
	}
	s := subscribe(t, hub, dirsTarget(t, root))
	status := nextStatus(t, s)
	if status.Mode != ModePolling || !strings.Contains(status.Reason, "Watching failed") {
		t.Fatalf("status = %+v, want polling after a failed watch", status)
	}
	write(t, filepath.Join(root, "a.md"), "x")
	if change := nextChange(t, s); !reflect.DeepEqual(change.Paths, []string{"a.md"}) {
		t.Fatalf("change = %+v", change)
	}
}

type failingBackend struct{ err error }

func (b failingBackend) Add(string) error    { return b.err }
func (b failingBackend) Remove(string) error { return nil }
func (b failingBackend) Close() error        { return nil }

func TestListingFailureIsOff(t *testing.T) {
	hub := newTestHub(t)
	s := subscribe(t, hub, Target{Root: "/missing", Directories: func() ([]string, int, error) {
		return nil, 0, errors.New("gone")
	}})
	if status := nextStatus(t, s); status.Mode != ModeOff || !strings.Contains(status.Reason, "gone") {
		t.Fatalf("status = %+v, want off", status)
	}
}

func TestGitStateChangesReportGit(t *testing.T) {
	root := tempRoot(t)
	gitDir := filepath.Join(root, ".git")
	write(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	target := dirsTarget(t, root)
	list := target.Directories
	target.Directories = func() ([]string, int, error) {
		dirs, entries, err := list()
		return slices.DeleteFunc(dirs, func(dir string) bool { return strings.HasPrefix(dir, gitDir) }), entries, err
	}
	target.Git = func() ([]string, func(string) bool) {
		return []string{gitDir}, func(name string) bool { return filepath.Base(name) == "HEAD" }
	}
	hub := newTestHub(t)
	s := subscribe(t, hub, target)
	nextStatus(t, s)

	write(t, filepath.Join(gitDir, "FETCH_HEAD"), "ignored")
	quiet(t, s, 150*time.Millisecond)
	write(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/other\n")
	if change := nextChange(t, s); !change.Git || len(change.Paths) != 0 {
		t.Fatalf("change = %+v, want git only", change)
	}
}

func TestCloseEndsEverySubscription(t *testing.T) {
	hub := NewHub(testOptions())
	watched := subscribe(t, hub, dirsTarget(t, tempRoot(t)))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-watched.Done()
	}()
	hub.Close()
	wg.Wait()
	if _, err := hub.Subscribe(dirsTarget(t, tempRoot(t))); !errors.Is(err, ErrClosed) {
		t.Fatalf("Subscribe after Close error = %v, want ErrClosed", err)
	}
}
