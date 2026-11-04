package workspace

import (
	"slices"
	"testing"

	"github.com/flexdinesh/servef/apps/native/internal/docview"
	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/apps/native/internal/link"
	"github.com/flexdinesh/servef/apps/native/internal/shell"
	"github.com/flexdinesh/servef/apps/native/internal/tabs"
	"github.com/flexdinesh/servef/pkg/contract"
)

var (
	readme = tabs.DocumentTab("README.md")
	source = tabs.DocumentTab("src/main.go")
	guide  = tabs.DocumentTab("docs/guide.md")
)

// remembered runs the app once to leave memory of README.md and src/main.go
// (active) with src open.
func remembered(t *testing.T, daemon *fakeservef.Server, store *memStore) {
	t.Helper()
	first, queue := run(t, daemon, store)
	first.OpenProject(project)
	queue.Drain()
	first.OpenDocument("README.md")
	first.OpenDocument("src/main.go")
	queue.Drain()
	first.RememberExpanded([]string{"src"})
	first.Close()
}

func rememberedTabs(t *testing.T, store *memStore) []tabs.Tab {
	t.Helper()
	p, _ := store.file.Workspace("local").Project(project)
	return p.Tabs
}

// A failed first page has no tree to check tabs against: nothing restores
// and memory stays, for the next healthy page, as use-project-memory.ts's
// `ready`.
func TestFailedFirstPageKeepsTheMemory(t *testing.T) {
	failures := map[string]func(*fakeservef.Server){
		"project unavailable": func(s *fakeservef.Server) { delete(s.Pages, project) },
		"scan failure": func(s *fakeservef.Server) {
			page := s.Pages[project][""]
			page.Tree, page.Error = []contract.TreeNode{}, "scan failed"
			s.Pages[project] = map[string]contract.PageData{"": page}
		},
	}
	for name, fail := range failures {
		t.Run(name, func(t *testing.T) {
			daemon := fakeservef.NewDaemon(t)
			daemon.Load(project)
			store := &memStore{}
			remembered(t, daemon, store)
			want := []tabs.Tab{readme, source}
			if got := rememberedTabs(t, store); !slices.Equal(got, want) {
				t.Fatalf("first run remembered %v", got)
			}

			daemon.Set(fail)
			second, queue := run(t, daemon, store)
			second.OpenProject(project)
			queue.Drain()
			if len(second.Tabs) != 0 {
				t.Fatalf("restored from a failed page: %v", second.Tabs)
			}
			if got := rememberedTabs(t, store); !slices.Equal(got, want) {
				t.Fatalf("a failed page changed memory: %v", got)
			}

			// The same run restores once the project recovers.
			daemon.Load(project)
			second.apply(contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Tree: true}})
			queue.Drain()
			if !slices.Equal(second.Tabs, want) || second.Active != source || second.Page.Selected != "src/main.go" {
				t.Fatalf("recovered: %v %v %q", second.Tabs, second.Active, second.Page.Selected)
			}
			if !slices.Equal(second.TreeOpen, []string{"src"}) {
				t.Fatalf("recovered folders %v", second.TreeOpen)
			}
			second.Close()

			third, queue := run(t, daemon, store)
			queue.Drain()
			if !slices.Equal(third.Tabs, want) {
				t.Fatalf("third run restored %v", third.Tabs)
			}
		})
	}
}

// A link to a missing file fails its page but keeps the tree; memory waits
// for the next healthy page, which restores it beside the link's tab.
func TestMissingLinkedDocumentDelaysRestore(t *testing.T) {
	daemon := fakeservef.NewDaemon(t)
	daemon.Load(project)
	store := &memStore{}
	remembered(t, daemon, store)
	gone := tabs.DocumentTab("gone.md")

	target, err := link.Parse("servef-native://local/?project=" + project + "&path=gone.md")
	if err != nil {
		t.Fatal(err)
	}
	queue := fakeservef.NewQueue()
	model := New(queue, &connector{daemon: daemon})
	t.Cleanup(model.Close)
	model.UseMemory(store)
	model.Handle(link.Request{Link: &target})
	queue.Drain()
	if !slices.Equal(model.Tabs, []tabs.Tab{gone}) || len(model.Page.Tree) == 0 || model.Page.Error == "" {
		t.Fatalf("tabs %v page %+v", model.Tabs, model.Page)
	}
	if got := rememberedTabs(t, store); !slices.Equal(got, []tabs.Tab{readme, source}) {
		t.Fatalf("remembered %v", got)
	}
	model.OpenDocument("docs/guide.md")
	queue.Drain()
	if want := []tabs.Tab{readme, source, gone, guide}; !slices.Equal(model.Tabs, want) || model.Active != guide {
		t.Fatalf("tabs %v active %v", model.Tabs, model.Active)
	}
}

// Memory is not written while the first page is awaited, so a document
// opened then neither replaces nor loses what is remembered.
func TestNothingIsRememberedWhileRestoring(t *testing.T) {
	daemon := fakeservef.NewDaemon(t)
	daemon.Load(project)
	store := &memStore{}
	remembered(t, daemon, store)
	saves := store.saves

	queue := fakeservef.NewQueue()
	model := New(queue, &connector{daemon: daemon})
	t.Cleanup(model.Close)
	model.UseMemory(store)
	target, err := link.Parse("servef-native://local/?project=" + project)
	if err != nil {
		t.Fatal(err)
	}
	model.Handle(link.Request{Link: &target})
	for model.Project == "" && queue.Step() {
	}
	model.OpenDocument("docs/guide.md")
	if store.saves != saves {
		t.Fatalf("saved %d times before the first page", store.saves-saves)
	}
	queue.Drain()
	want := []tabs.Tab{readme, source, guide}
	if !slices.Equal(model.Tabs, want) || !slices.Equal(rememberedTabs(t, store), want) || model.Active != guide {
		t.Fatalf("tabs %v remembered %v active %v", model.Tabs, rememberedTabs(t, store), model.Active)
	}
}

// A live update while a tab's page loads keeps that load's document.
func TestLiveUpdateDuringATabSwitch(t *testing.T) {
	model, queue, _ := setup(t)
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject(project)
	queue.Drain()
	model.OpenDocument("README.md")
	queue.Drain()
	model.OpenDocument("src/main.go")
	model.apply(contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Tree: true}})
	queue.Drain()
	if model.Active != source || model.Page.Selected != "src/main.go" {
		t.Fatalf("active %v page %q", model.Active, model.Page.Selected)
	}
	if model.View.Kind == docview.None {
		t.Fatalf("view %+v", model.View)
	}
}

// A change to the document being loaded rereads it: the load in flight may
// have read it before the change.
func TestLiveChangeToTheLoadingDocumentRereadsIt(t *testing.T) {
	model, queue, daemon := setup(t)
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject(project)
	queue.Drain()
	model.OpenDocument("README.md")
	queue.Drain()
	model.OpenDocument("src/main.go")
	reads := func() (n int) {
		daemon.Get(func(s *fakeservef.Server) { n = s.PageReads })
		return n
	}
	before := reads()
	model.apply(contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Paths: []string{"src/main.go"}}})
	queue.Drain()
	// The superseded load and its reissue.
	if got := reads() - before; got != 2 {
		t.Fatalf("%d page reads", got)
	}
	if model.Page.Selected != "src/main.go" {
		t.Fatalf("page %q", model.Page.Selected)
	}
}

// A live update while the restored active tab loads keeps that tab showing.
func TestLiveUpdateDuringRestore(t *testing.T) {
	daemon := fakeservef.NewDaemon(t)
	daemon.Load(project)
	store := &memStore{}
	remembered(t, daemon, store)

	queue := fakeservef.NewQueue()
	model := New(queue, &connector{daemon: daemon})
	t.Cleanup(model.Close)
	model.UseMemory(store)
	model.Handle(link.Request{})
	for model.Active != source && queue.Step() {
	}
	if model.Active != source || model.Page.Selected == "src/main.go" {
		t.Fatalf("not mid-restore: %v %q", model.Active, model.Page.Selected)
	}
	model.apply(contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Tree: true}})
	queue.Drain()
	if model.Page.Selected != "src/main.go" || model.View.Kind == docview.None {
		t.Fatalf("page %q view %+v", model.Page.Selected, model.View)
	}
}

// A live update before the first page keeps its reset of the open folders,
// so the remembered ones apply.
func TestLiveUpdateBeforeTheFirstPage(t *testing.T) {
	daemon := fakeservef.NewDaemon(t)
	daemon.Load(project)
	store := &memStore{}
	remembered(t, daemon, store)

	queue := fakeservef.NewQueue()
	model := New(queue, &connector{daemon: daemon})
	t.Cleanup(model.Close)
	model.UseMemory(store)
	model.Handle(link.Request{})
	for model.Project == "" && queue.Step() {
	}
	version := model.TreeVersion
	model.apply(contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Tree: true}})
	queue.Drain()
	if model.TreeVersion == version || !slices.Equal(model.TreeOpen, []string{"src"}) {
		t.Fatalf("tree version %d -> %d, open %v", version, model.TreeVersion, model.TreeOpen)
	}
	if !slices.Equal(model.Tabs, []tabs.Tab{readme, source}) || model.Page.Selected != "src/main.go" {
		t.Fatalf("tabs %v page %q", model.Tabs, model.Page.Selected)
	}
}

// Closing the loaded document's tab under a shown review lets its source
// go: the next change does not reread it.
func TestClosingAHeldDocumentUnderAReview(t *testing.T) {
	model, queue, daemon := openWithChanges(t)
	model.OpenDocument("README.md")
	queue.Drain()
	model.OpenChange(model.Changes.Files[0])
	queue.Drain()
	if model.Page.Selected != "README.md" {
		t.Fatalf("page %q", model.Page.Selected)
	}
	model.CloseTabs([]tabs.Tab{readme})
	queue.Drain()
	if model.Page.Selected != "" || model.Page.Source != "" || model.Page.HasFile {
		t.Fatalf("page holds %q (%d bytes)", model.Page.Selected, len(model.Page.Source))
	}
	if model.Active != tabs.Uncommitted() || model.Review == nil {
		t.Fatalf("active %v review %+v", model.Active, model.Review)
	}
	var reads int
	daemon.Get(func(s *fakeservef.Server) { reads = s.PageReads })
	model.apply(contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Paths: []string{"README.md"}}})
	queue.Drain()
	daemon.Get(func(s *fakeservef.Server) {
		if s.PageReads != reads {
			t.Fatalf("a closed document's change read the page %d times", s.PageReads-reads)
		}
	})
}

// A document still loading when its tab closes under a review does not
// land in the page.
func TestClosingALoadingDocumentUnderAReview(t *testing.T) {
	model, queue, _ := openWithChanges(t)
	model.OpenChange(model.Changes.Files[0])
	queue.Drain()
	model.OpenDocument("README.md")
	model.OpenReview()
	model.CloseTabs([]tabs.Tab{readme})
	queue.Drain()
	if model.Page.Selected != "" || model.Page.Source != "" {
		t.Fatalf("page holds %q", model.Page.Selected)
	}
}

// Dragging the sidebar updates the width at once; the view and the store
// see every step, and the store decides how often to write.
func TestSidebarWidthFollowsTheDrag(t *testing.T) {
	store := &memStore{}
	daemon := fakeservef.NewDaemon(t)
	daemon.Load(project)
	model, _ := run(t, daemon, store)
	model.SetSidebarWidth(300)
	if model.Sidebar.Width != 300 || store.file.Sidebar.Width != 300 {
		t.Fatalf("width %v remembered %v", model.Sidebar.Width, store.file.Sidebar.Width)
	}
	if model.Sidebar.View != shell.Explorer {
		t.Fatalf("view %v", model.Sidebar.View)
	}
}
