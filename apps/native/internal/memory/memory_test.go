package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flexdinesh/servef/apps/native/internal/tabs"
)

func docs(paths ...string) []tabs.Tab {
	out := make([]tabs.Tab, len(paths))
	for i, p := range paths {
		out[i] = tabs.DocumentTab(p)
	}
	return out
}

func in(paths ...string) func(string) bool {
	return func(p string) bool { return slices.Contains(paths, p) }
}

// The tests below mirror workspace-memory.test.ts.

func TestStartupTab(t *testing.T) {
	readme := tabs.DocumentTab("README.md")
	if got := StartupTab(false, Restored{Tabs: []tabs.Tab{readme}, Active: readme}, true); got != readme {
		t.Errorf("remembered active: %v", got)
	}
	if got := StartupTab(false, Restored{}, true); got != tabs.WelcomeTab() {
		t.Errorf("nothing restored: %v", got)
	}
	if got := StartupTab(false, Restored{}, false); !got.None() {
		t.Errorf("Welcome off: %v", got)
	}
	// Restored tabs without an active one keep the pane empty.
	if got := StartupTab(false, Restored{Tabs: []tabs.Tab{readme}}, true); !got.None() {
		t.Errorf("no active: %v", got)
	}
	// A routed tab wins over memory and Welcome.
	if got := StartupTab(true, Restored{Tabs: []tabs.Tab{readme}, Active: readme}, true); !got.None() {
		t.Errorf("routed: %v", got)
	}
}

func TestWelcomeIsRememberedLikeAnyTab(t *testing.T) {
	w := Workspace{}.RememberProject("/r", "/r", []tabs.Tab{tabs.WelcomeTab(), tabs.DocumentTab("a.md")}, tabs.WelcomeTab())
	p, _ := w.Project("/r")
	got := RestoreTabs(p, in("a.md"))
	if !slices.Equal(got.Tabs, []tabs.Tab{tabs.WelcomeTab(), tabs.DocumentTab("a.md")}) || got.Active != tabs.WelcomeTab() {
		t.Fatalf("%+v", got)
	}
}

func TestProjectsSharedAcrossSessionsLastPerSessionKey(t *testing.T) {
	w := Workspace{}.RememberProject("/a", "/a/api", docs("README.md", "go.mod"), tabs.DocumentTab("go.mod"))
	w = w.RememberProject("/a\n/b", "/b/web", docs("other.md"), tabs.DocumentTab("other.md"))
	if p, _ := w.Project("/a/api"); !slices.Equal(p.Tabs, docs("README.md", "go.mod")) || p.Active != tabs.DocumentTab("go.mod") {
		t.Errorf("/a/api %+v", p)
	}
	if w.LastProject("/a") != "/a/api" || w.LastProject("/a\n/b") != "/b/web" || w.LastProject("/b\n/a") != "" {
		t.Errorf("last %+v", w.last)
	}
	if _, ok := w.Project("/a/web"); ok {
		t.Error("unknown project remembered")
	}
}

func TestDaemonSessionKeepsItsOwnLastProject(t *testing.T) {
	w := Workspace{}.RememberProject("/a", "/a/api", nil, tabs.Tab{})
	w = w.RememberProject("daemon", "/b/web", nil, tabs.Tab{})
	if w.LastProject("daemon") != "/b/web" || w.LastProject("/a") != "/a/api" {
		t.Fatal("sessions")
	}
	w = w.RememberProject("daemon", "/a/api", nil, tabs.Tab{})
	if w.LastProject("daemon") != "/a/api" || w.LastProject("/a") != "/a/api" || len(w.last) != 2 {
		t.Fatalf("last %+v", w.last)
	}
}

func TestActiveTabThatIsNotOpenIsNotRemembered(t *testing.T) {
	w := Workspace{}.RememberProject("/a", "/a/api", docs("README.md"), tabs.DocumentTab("closed.md"))
	if p, _ := w.Project("/a/api"); !p.Active.None() {
		t.Fatalf("active %v", p.Active)
	}
}

func TestRememberedProjectsMostRecentFirstAndForgotten(t *testing.T) {
	w := Workspace{}.RememberProject("/a", "/a/web", nil, tabs.Tab{})
	w = w.RememberProject("/a", "/a/notes", docs("a.md"), tabs.DocumentTab("a.md"))
	w = w.RememberProject("/a", "/a/web", nil, tabs.Tab{})
	if got := w.Remembered(); !slices.Equal(got, []string{"/a/web", "/a/notes"}) {
		t.Fatalf("%v", got)
	}
	w = w.Forget("/a/web")
	if w.LastProject("/a") != "" || !slices.Equal(w.Remembered(), []string{"/a/notes"}) {
		t.Fatalf("forgotten %+v", w)
	}
	if got := w.Forget("/a/missing").Remembered(); !slices.Equal(got, []string{"/a/notes"}) {
		t.Fatalf("%v", got)
	}
}

func TestRememberedProjectsAreCapped(t *testing.T) {
	w := Workspace{}
	for i := range 110 {
		w = w.RememberProject("/a", fmt.Sprintf("/a/p-%d", i), nil, tabs.Tab{})
	}
	got := w.Remembered()
	if len(got) != 100 || got[0] != "/a/p-109" || slices.Contains(got, "/a/p-0") {
		t.Fatalf("%d %v", len(got), got[:1])
	}
}

func TestRestoredTabsKeepOnlyFilesInTheFreshTree(t *testing.T) {
	files := in("README.md", "src/main.go")
	got := RestoreTabs(Project{Tabs: docs("README.md", "gone.md", "src/main.go", "README.md"), Active: tabs.DocumentTab("gone.md")}, files)
	if !slices.Equal(got.Tabs, docs("README.md", "src/main.go")) || !got.Active.None() {
		t.Errorf("%+v", got)
	}
	got = RestoreTabs(Project{Tabs: docs("src/main.go"), Active: tabs.DocumentTab("src/main.go")}, files)
	if got.Active != tabs.DocumentTab("src/main.go") {
		t.Errorf("%+v", got)
	}
	if got := RestoreTabs(Project{}, files); len(got.Tabs) != 0 || !got.Active.None() {
		t.Errorf("%+v", got)
	}
}

// The review needs no tree entry; documents still do.
func TestRestoredReviewNeedsNoTreeEntry(t *testing.T) {
	got := RestoreTabs(Project{Tabs: []tabs.Tab{tabs.DocumentTab("README.md"), tabs.Uncommitted(), tabs.DocumentTab("removed.md")}, Active: tabs.Uncommitted()}, in("README.md"))
	if !slices.Equal(got.Tabs, []tabs.Tab{tabs.DocumentTab("README.md"), tabs.Uncommitted()}) || got.Active != tabs.Uncommitted() {
		t.Fatalf("%+v", got)
	}
}

func TestExpansionAndTabsPreserveEachOther(t *testing.T) {
	w := Workspace{}.RememberProject("/a", "/a/api", docs("README.md"), tabs.DocumentTab("README.md"))
	w = w.RememberExpanded("/a/api", []string{"src", "src", "docs"})
	w = w.RememberExpanded("/a/web", []string{})
	w = w.RememberProject("/a", "/a/api", docs("main.go"), tabs.DocumentTab("main.go"))
	p, _ := w.Project("/a/api")
	if !reflect.DeepEqual(p, Project{Tabs: docs("main.go"), Active: tabs.DocumentTab("main.go"), Expanded: []string{"src", "docs"}, HasExpanded: true}) {
		t.Errorf("/a/api %+v", p)
	}
	if p, _ := w.Project("/a/web"); len(p.Tabs) != 0 || !p.HasExpanded || len(p.Expanded) != 0 {
		t.Errorf("/a/web %+v", p)
	}
	if w.LastProject("/a") != "/a/api" {
		t.Error("last")
	}
}

func TestRestoringExpansionValidatesDirectories(t *testing.T) {
	dirs := in("src", "docs")
	got, ok := RestoreExpanded(Project{HasExpanded: true, Expanded: []string{"src", "gone", "../outside", "main.go", "src"}}, dirs)
	if !ok || !slices.Equal(got, []string{"src"}) {
		t.Errorf("%v %v", got, ok)
	}
	if got, ok := RestoreExpanded(Project{HasExpanded: true}, dirs); !ok || got == nil || len(got) != 0 {
		t.Errorf("all closed: %v %v", got, ok)
	}
	if _, ok := RestoreExpanded(Project{}, dirs); ok {
		t.Error("absent preference restored")
	}
}

// The file round-trips, keeps entry order, and keeps connections apart.
func TestFileRoundTrips(t *testing.T) {
	w := Workspace{}.RememberProject("daemon", "/a", []tabs.Tab{tabs.DocumentTab("a.md"), tabs.Uncommitted(), tabs.WelcomeTab()}, tabs.Uncommitted())
	w = w.RememberExpanded("/a", []string{"src"})
	w = w.RememberProject("daemon", "/b", nil, tabs.Tab{}).RememberExpanded("/b", nil)
	f := File{Sidebar: Sidebar{View: "source-control", Hidden: true, Width: 320}}.WithWorkspace("local", w)
	f = f.WithWorkspace("http-localhost-7971", Workspace{}.RememberProject("/x", "/x", docs("x.md"), tabs.Tab{}))
	got := Parse(f.Marshal())
	if !reflect.DeepEqual(got, f) {
		t.Fatalf("round trip\n got %+v\nwant %+v\n%s", got, f, f.Marshal())
	}
	if got.Workspace("local").LastProject("daemon") != "/b" || got.Workspace("http-localhost-7971").LastProject("daemon") != "" {
		t.Fatal("connections share memory")
	}
}

func TestParseIgnoresMalformedData(t *testing.T) {
	for _, data := range []string{"", "null", "[]", `{"version":2,"connections":[]}`, `{"version":1,"connections":3}`} {
		if got := Parse([]byte(data)); !reflect.DeepEqual(got, File{}) {
			t.Errorf("%s: %+v", data, got)
		}
	}
	f := Parse([]byte(`{"version":1,"connections":[{"id":"","last":[]},{"id":"local","last":[{"session":"daemon","project":""},{"session":"/b","project":"/b/x"}],
		"projects":[{"id":"p","tabs":["a.md",3,{"review":"other"},""],"active":{"welcome":true}},{"id":"","tabs":["x"]}]}]}`))
	w := f.Workspace("local")
	if w.LastProject("daemon") != "" || w.LastProject("/b") != "/b/x" {
		t.Errorf("last %+v", w.last)
	}
	if p, _ := w.Project("p"); !slices.Equal(p.Tabs, docs("a.md")) || p.Active != tabs.WelcomeTab() || p.HasExpanded {
		t.Errorf("p %+v", p)
	}
	if len(w.projects) != 1 {
		t.Errorf("projects %+v", w.projects)
	}
}

func TestStoreSavesAndLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspaces.json")
	store := &Store{Path: path}
	if got := store.Load(); !reflect.DeepEqual(got, File{}) {
		t.Fatalf("missing file: %+v", got)
	}
	f := File{Sidebar: Sidebar{View: "explorer", Width: 300}}.WithWorkspace("local", Workspace{}.RememberProject("daemon", "/a", docs("a.md"), tabs.DocumentTab("a.md")))
	store.Save(File{})
	store.Save(f)
	store.Flush()
	if got := (&Store{Path: path}).Load(); !reflect.DeepEqual(got, f) {
		t.Fatalf("loaded %+v", got)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", info, err)
	}
}

// A mistyped field drops only itself: the rest of the file stays, as with
// the web's per-field checks.
func TestParseKeepsWhatIsWellFormed(t *testing.T) {
	data := []byte(`{"version":1,
		"sidebar":{"view":3,"hidden":true,"width":"wide"},
		"connections":[
			7,
			{"id":5,"last":[],"projects":[]},
			{"id":"local",
			 "last":[{"session":"daemon","project":"/a"},{"session":"other","project":4},8],
			 "projects":[
				{"id":"/a","tabs":["a.md",3,{"review":"working"}],"active":"a.md","expanded":["src",9]},
				{"id":6,"tabs":["lost.md"]},
				{"id":"/b","tabs":"b.md","active":2,"expanded":"src"},
				"nonsense"
			 ]},
			{"id":"other","last":"none","projects":{"a":1}}
		]}`)
	f := Parse(data)
	if f.Sidebar != (Sidebar{Hidden: true}) {
		t.Errorf("sidebar %+v", f.Sidebar)
	}
	local := f.Workspace("local")
	if got := local.LastProject("daemon"); got != "/a" {
		t.Errorf("last %q", got)
	}
	a, ok := local.Project("/a")
	if !ok || !slices.Equal(a.Tabs, []tabs.Tab{tabs.DocumentTab("a.md"), tabs.Uncommitted()}) || a.Active != tabs.DocumentTab("a.md") || !slices.Equal(a.Expanded, []string{"src"}) {
		t.Errorf("/a %+v", a)
	}
	if b, ok := local.Project("/b"); !ok || len(b.Tabs) != 0 || b.HasExpanded {
		t.Errorf("/b %+v", b)
	}
	if len(local.projects) != 2 || len(f.connections) != 2 {
		t.Errorf("projects %d connections %d", len(local.projects), len(f.connections))
	}
}

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "workspaces.json")
	return &Store{Path: path}, path
}

func remembering(tab string) File {
	return File{}.WithWorkspace("local", Workspace{}.RememberProject("daemon", "/a", docs(tab), tabs.DocumentTab(tab)))
}

// A file of another version is left alone: this build reads nothing from
// it and writes nothing over it.
func TestStoreNeverOverwritesAnotherVersion(t *testing.T) {
	for _, stored := range []string{`{"version":2,"future":true}`, `{"version":"1"}`, `{"future":true}`} {
		store, path := newStore(t)
		if err := os.WriteFile(path, []byte(stored), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := store.Load(); !reflect.DeepEqual(got, File{}) {
			t.Errorf("%s: loaded %+v", stored, got)
		}
		store.Save(remembering("a.md"))
		store.Flush()
		if got, _ := os.ReadFile(path); string(got) != stored {
			t.Errorf("%s: overwritten with %s", stored, got)
		}
	}
}

// What is not memory at all reads as nothing and is replaced.
func TestStoreReplacesAMalformedFile(t *testing.T) {
	for _, stored := range []string{`not json`, `[]`, ``} {
		store, path := newStore(t)
		if err := os.WriteFile(path, []byte(stored), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := store.Load(); !reflect.DeepEqual(got, File{}) {
			t.Errorf("%q: loaded %+v", stored, got)
		}
		f := remembering("a.md")
		store.Save(f)
		store.Flush()
		if got := (&Store{Path: path}).Load(); !reflect.DeepEqual(got, f) {
			t.Errorf("%q: replaced with %+v", stored, got)
		}
	}
}

// Saves coalesce: a sidebar drag's frames write once, the latest, and
// Flush writes without waiting.
func TestStoreCoalescesSavesUntilFlush(t *testing.T) {
	store, path := newStore(t)
	store.Delay = time.Hour
	var writes atomic.Int32
	store.writeFile = func(path string, data []byte) error {
		writes.Add(1)
		return write(path, data)
	}
	var last File
	for i := range 30 {
		last = remembering(fmt.Sprintf("%d.md", i))
		store.Save(last)
		time.Sleep(time.Millisecond)
	}
	if got := writes.Load(); got != 0 {
		t.Fatalf("%d writes before the delay", got)
	}
	store.Flush()
	if got := writes.Load(); got != 1 {
		t.Fatalf("%d writes", got)
	}
	if got := (&Store{Path: path}).Load(); !reflect.DeepEqual(got, last) {
		t.Fatalf("flushed %+v", got)
	}
	// Flush with nothing pending returns, and the store keeps working.
	store.Flush()
	store.Save(remembering("again.md"))
	store.Flush()
	if got := writes.Load(); got != 2 {
		t.Fatalf("%d writes after a second save", got)
	}
}

// The delay writes by itself, with no Flush.
func TestStoreWritesAfterTheDelay(t *testing.T) {
	store, path := newStore(t)
	store.Delay = time.Millisecond
	f := remembering("a.md")
	store.Save(f)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := (&Store{Path: path}).Load(); reflect.DeepEqual(got, f) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("not written")
		}
		time.Sleep(time.Millisecond)
	}
}

// A failed write loses that save only: no temporary file stays behind, the
// old file survives, and the next save works.
func TestStoreSurvivesWriteFailures(t *testing.T) {
	t.Run("no directory", func(t *testing.T) {
		dir := t.TempDir()
		blocker := filepath.Join(dir, "file")
		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		store := &Store{Path: filepath.Join(blocker, "workspaces.json")}
		store.Save(remembering("a.md"))
		store.Flush()
		if entries, _ := os.ReadDir(dir); len(entries) != 1 {
			t.Fatalf("left %v", entries)
		}
	})
	t.Run("target is a directory", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "workspaces.json")
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		store := &Store{Path: path}
		store.Save(remembering("a.md"))
		store.Flush()
		if entries, _ := os.ReadDir(dir); len(entries) != 1 {
			t.Fatalf("left %v", entries)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		f := remembering("b.md")
		store.Save(f)
		store.Flush()
		if got := (&Store{Path: path}).Load(); !reflect.DeepEqual(got, f) {
			t.Fatalf("after the failure %+v", got)
		}
	})
	t.Run("old file survives", func(t *testing.T) {
		store, path := newStore(t)
		old := remembering("old.md")
		store.Save(old)
		store.Flush()
		store.writeFile = func(string, []byte) error { return errors.New("disk full") }
		store.Save(remembering("new.md"))
		store.Flush()
		if got := (&Store{Path: path}).Load(); !reflect.DeepEqual(got, old) {
			t.Fatalf("old file %+v", got)
		}
	})
}
