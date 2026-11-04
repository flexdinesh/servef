package workspace

import (
	"slices"
	"testing"

	"github.com/flexdinesh/servef/apps/native/internal/docview"
	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/apps/native/internal/link"
	"github.com/flexdinesh/servef/apps/native/internal/memory"
	"github.com/flexdinesh/servef/apps/native/internal/shell"
	"github.com/flexdinesh/servef/apps/native/internal/tabs"
	"github.com/flexdinesh/servef/pkg/contract"
)

// memStore keeps memory in RAM, as one file between two runs.
type memStore struct {
	file  memory.File
	saves int
}

func (s *memStore) Load() memory.File  { return s.file }
func (s *memStore) Save(f memory.File) { s.file, s.saves = f, s.saves+1 }

func run(t *testing.T, daemon *fakeservef.Server, store *memStore) (*Model, *fakeservef.Queue) {
	t.Helper()
	queue := fakeservef.NewQueue()
	model := New(queue, &connector{daemon: daemon})
	t.Cleanup(model.Close)
	if store != nil {
		model.UseMemory(store)
	}
	model.Handle(link.Request{})
	queue.Drain()
	return model, queue
}

func TestTabsOpenSwitchAndClose(t *testing.T) {
	model, queue, _ := setup(t)
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject(project)
	queue.Drain()
	if len(model.Tabs) != 0 || !model.Active.None() {
		t.Fatalf("a project opens with no tabs: %v %v", model.Tabs, model.Active)
	}
	readme, main, guide := tabs.DocumentTab("README.md"), tabs.DocumentTab("src/main.go"), tabs.DocumentTab("docs/guide.md")
	model.OpenDocument("README.md")
	model.OpenDocument("src/main.go")
	model.OpenDocument("docs/guide.md")
	model.OpenDocument("README.md")
	queue.Drain()
	if !slices.Equal(model.Tabs, []tabs.Tab{readme, main, guide}) || model.Active != readme || model.Page.Selected != "README.md" {
		t.Fatalf("tabs %v active %v page %q", model.Tabs, model.Active, model.Page.Selected)
	}
	model.SelectTab(main)
	queue.Drain()
	if model.Active != main || model.Page.Selected != "src/main.go" {
		t.Fatalf("select: %v %q", model.Active, model.Page.Selected)
	}
	// Closing the active tab shows its right neighbor, else its left.
	model.CloseTabs([]tabs.Tab{main})
	queue.Drain()
	if model.Active != guide || model.Page.Selected != "docs/guide.md" || !slices.Equal(model.Tabs, []tabs.Tab{readme, guide}) {
		t.Fatalf("close: %v %v %q", model.Tabs, model.Active, model.Page.Selected)
	}
	model.CloseTabs([]tabs.Tab{guide})
	queue.Drain()
	if model.Active != readme || model.Page.Selected != "README.md" {
		t.Fatalf("close last: %v %q", model.Active, model.Page.Selected)
	}
	// Closing another tab keeps the active one.
	model.OpenDocument("src/main.go")
	queue.Drain()
	model.CloseTabs([]tabs.Tab{readme})
	if model.Active != main || !slices.Equal(model.Tabs, []tabs.Tab{main}) {
		t.Fatalf("close other: %v %v", model.Tabs, model.Active)
	}
	// Closing every tab shows the project with no document.
	model.TabAction(tabs.CloseAll, 0)
	queue.Drain()
	if len(model.Tabs) != 0 || !model.Active.None() || model.Page.Selected != "" || model.Page.HasFile || model.Title() != "demo - servef" {
		t.Fatalf("close all: %v %v %q %q", model.Tabs, model.Active, model.Page.Selected, model.Title())
	}
}

func TestTabMenuActions(t *testing.T) {
	model, queue, _ := setup(t)
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject(project)
	queue.Drain()
	for _, p := range []string{"README.md", "src/main.go", "docs/guide.md"} {
		model.OpenDocument(p)
	}
	queue.Drain()
	model.SelectTab(tabs.DocumentTab("README.md"))
	queue.Drain()
	model.TabAction(tabs.CloseRight, 1)
	if !slices.Equal(model.Tabs, []tabs.Tab{tabs.DocumentTab("README.md"), tabs.DocumentTab("src/main.go")}) || model.Active != tabs.DocumentTab("README.md") {
		t.Fatalf("close right: %v %v", model.Tabs, model.Active)
	}
	model.TabAction(tabs.CloseOthers, 1)
	queue.Drain()
	if !slices.Equal(model.Tabs, []tabs.Tab{tabs.DocumentTab("src/main.go")}) || model.Active != tabs.DocumentTab("src/main.go") || model.Page.Selected != "src/main.go" {
		t.Fatalf("close others: %v %v %q", model.Tabs, model.Active, model.Page.Selected)
	}
	model.TabAction(tabs.Reveal, 0)
	model.TabAction(tabs.CloseTab, 5)
	if len(model.Tabs) != 1 {
		t.Fatalf("no-op actions closed tabs: %v", model.Tabs)
	}
}

// The review is one tab; it opens on the first change when none is open,
// and stays open beside documents.
func TestReviewTab(t *testing.T) {
	model, queue, _ := openWithChanges(t)
	model.OpenDocument("src/main.go")
	queue.Drain()
	model.OpenReview()
	queue.Drain()
	if model.Active != tabs.Uncommitted() || model.Review == nil || model.Review.File != model.Changes.Files[0] || model.Review.Diff == nil && model.Review.Error == "" {
		t.Fatalf("review: %v %+v", model.Active, model.Review)
	}
	model.OpenChange(model.Changes.Files[2])
	model.OpenChange(model.Changes.Files[2])
	if !slices.Equal(model.Tabs, []tabs.Tab{tabs.DocumentTab("src/main.go"), tabs.Uncommitted()}) {
		t.Fatalf("one review tab: %v", model.Tabs)
	}
	model.SelectTab(tabs.DocumentTab("src/main.go"))
	queue.Drain()
	model.SelectTab(tabs.Uncommitted())
	if model.Review == nil || model.Review.File != model.Changes.Files[2] {
		t.Fatalf("the review keeps its file: %+v", model.Review)
	}
	model.CloseReview()
	if model.Review != nil || model.Active != tabs.DocumentTab("src/main.go") || len(model.Tabs) != 1 {
		t.Fatalf("close review: %v %v", model.Active, model.Tabs)
	}
}

// Modes are kept per document while its tab is open.
func TestModesPerDocument(t *testing.T) {
	model, queue, _ := setup(t)
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject(project)
	model.OpenDocument("README.md")
	queue.Drain()
	model.SetMode(docview.Source)
	model.OpenDocument("docs/guide.md")
	queue.Drain()
	model.SetMode(docview.Preview)
	model.SelectTab(tabs.DocumentTab("README.md"))
	queue.Drain()
	if model.View.Mode != docview.Source {
		t.Fatalf("README mode %v", model.View.Mode)
	}
	model.CloseTabs([]tabs.Tab{tabs.DocumentTab("README.md")})
	model.OpenDocument("README.md")
	queue.Drain()
	if model.View.Mode != docview.Preview {
		t.Fatalf("a reopened document takes the last choice: %v", model.View.Mode)
	}
}

// A second run lands on the session's last project and restores its tabs,
// active tab, and open folders, checked against the fresh tree.
func TestMemoryRestoresTheWorkspace(t *testing.T) {
	daemon := fakeservef.NewDaemon(t)
	daemon.Load(project)
	store := &memStore{}
	first, queue := run(t, daemon, store)
	if first.Project != "" {
		t.Fatal("nothing remembered lands on the project list")
	}
	first.OpenProject(project)
	queue.Drain()
	first.OpenDocument("README.md")
	first.OpenDocument("src/main.go")
	first.OpenDocument("docs/guide.md")
	queue.Drain()
	first.SelectTab(tabs.DocumentTab("src/main.go"))
	first.RememberExpanded([]string{"src"})
	first.SelectView(shell.SourceControl)
	first.SetSidebarWidth(1000)
	queue.Drain()
	first.Close()

	// A file the tree no longer has drops from the restored tabs.
	daemon.Set(func(s *fakeservef.Server) {
		pages := s.Pages[project]
		delete(pages, "docs/guide.md")
		for path, page := range pages {
			page.Tree = withoutPath(page.Tree, "docs/guide.md")
			pages[path] = page
		}
	})
	second, queue := run(t, daemon, store)
	queue.Drain()
	if second.Project != project {
		t.Fatalf("landed on %q", second.Project)
	}
	want := []tabs.Tab{tabs.DocumentTab("README.md"), tabs.DocumentTab("src/main.go")}
	if !slices.Equal(second.Tabs, want) || second.Active != tabs.DocumentTab("src/main.go") || second.Page.Selected != "src/main.go" {
		t.Fatalf("restored %v %v %q", second.Tabs, second.Active, second.Page.Selected)
	}
	if !slices.Equal(second.TreeOpen, []string{"src"}) {
		t.Fatalf("open folders %v", second.TreeOpen)
	}
	if second.Sidebar != (shell.Sidebar{View: shell.SourceControl, Width: shell.MaxWidth}) {
		t.Fatalf("sidebar %+v", second.Sidebar)
	}
}

// withoutPath is nodes without the file at path.
func withoutPath(nodes []contract.TreeNode, path string) []contract.TreeNode {
	out := make([]contract.TreeNode, 0, len(nodes))
	for _, node := range nodes {
		if node.Path == path {
			continue
		}
		node.Children = withoutPath(node.Children, path)
		out = append(out, node)
	}
	return out
}

func TestRestoredTabsJoinALinkedDocument(t *testing.T) {
	daemon := fakeservef.NewDaemon(t)
	daemon.Load(project)
	store := &memStore{}
	first, queue := run(t, daemon, store)
	first.OpenProject(project)
	queue.Drain()
	first.OpenDocument("README.md")
	first.OpenDocument("gone.md")
	queue.Drain()
	first.Close()

	target, err := link.Parse("servef-native://local/?project=" + project + "&path=src/main.go")
	if err != nil {
		t.Fatal(err)
	}
	queue = fakeservef.NewQueue()
	second := New(queue, &connector{daemon: daemon})
	t.Cleanup(second.Close)
	second.UseMemory(store)
	second.Handle(link.Request{Link: &target})
	queue.Drain()
	queue.Drain()
	// gone.md is not in the tree; the link's document stays active, after
	// the remembered tabs.
	if !slices.Equal(second.Tabs, []tabs.Tab{tabs.DocumentTab("README.md"), tabs.DocumentTab("src/main.go")}) ||
		second.Active != tabs.DocumentTab("src/main.go") || second.Page.Selected != "src/main.go" {
		t.Fatalf("tabs %v active %v page %q", second.Tabs, second.Active, second.Page.Selected)
	}
}

// Memory is per connection: another server's projects do not land here.
func TestMemoryIsPerConnection(t *testing.T) {
	daemon := fakeservef.NewDaemon(t)
	daemon.Load(project)
	store := &memStore{file: memory.File{}.WithWorkspace("http-localhost-7971", memory.Workspace{}.RememberProject("daemon", project, nil, tabs.Tab{}))}
	model, _ := run(t, daemon, store)
	if model.Project != "" {
		t.Fatalf("landed on %q from another connection's memory", model.Project)
	}
}

func TestSidebarViews(t *testing.T) {
	model, queue, daemon := openWithChanges(t)
	model.SelectView(shell.Explorer)
	if model.Sidebar.Shown() != "" {
		t.Fatal("clicking the shown view hides the sidebar")
	}
	model.ToggleSidebar()
	model.SelectView(shell.SourceControl)
	queue.Drain()
	model.SelectView(shell.SourceControl)
	model.SelectView(shell.SourceControl)
	queue.Drain()
	daemon.Get(func(s *fakeservef.Server) {
		if s.ChangesReads != 3 {
			t.Fatalf("changes read %d times; the load and each showing of Source Control read them", s.ChangesReads)
		}
	})
	model.SetSidebarWidth(10)
	if model.Sidebar.Width != shell.MinWidth {
		t.Fatalf("width %v", model.Sidebar.Width)
	}
}

// As closeTabs in App.tsx: a closed tab's view and scroll go with it, and its
// Format with them; the project's preference stays.
func TestClosingATabForgetsItsPlace(t *testing.T) {
	model, queue, _ := setup(t)
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject(project)
	queue.Drain()
	model.OpenDocument("README.md")
	queue.Drain()
	model.SetMode(docview.Source)
	model.SetMode(docview.Preview)
	key, source := model.OpenPlaceKey(), model.Page.Source
	model.RememberScroll(key, 40)
	model.documents.formats.Set("README.md", source, docview.FormatState{Kind: docview.FormatReady, Text: "formatted"})
	model.CloseTabs([]tabs.Tab{tabs.DocumentTab("README.md")})
	queue.Drain()
	if place := model.Place(key); place != (docview.Place{}) {
		t.Fatalf("place %+v", place)
	}
	if model.documents.formats.Get("README.md", source) != nil {
		t.Fatal("Format stayed")
	}
	if !model.documents.preview[project] {
		t.Fatal("the project's preference went with the tab")
	}
}

// Back from a review shows the document last shown, without moving through
// the history; the review's tab stays.
func TestBackFromAReviewShowsTheDocument(t *testing.T) {
	model, queue, daemon := setup(t)
	daemon.LoadChanges(t, project)
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject(project)
	queue.Drain()
	model.OpenDocument("README.md")
	queue.Drain()
	model.OpenDocument("src/main.go")
	queue.Drain()
	model.OpenReview()
	queue.Drain()
	model.GoBack()
	queue.Drain()
	if model.Active != tabs.DocumentTab("src/main.go") || model.Page.Selected != "src/main.go" {
		t.Fatalf("active %v page %q", model.Active, model.Page.Selected)
	}
	if !tabs.Includes(model.Tabs, tabs.Uncommitted()) || model.Review == nil {
		t.Fatalf("the review closed: %v", model.Tabs)
	}
	model.GoBack()
	queue.Drain()
	if model.Active != tabs.DocumentTab("README.md") {
		t.Fatalf("back moved to %v", model.Active)
	}
}

// Back to a document whose tab closed opens it again, as a route does.
func TestBackReopensAClosedTab(t *testing.T) {
	model, queue, _ := setup(t)
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject(project)
	queue.Drain()
	model.OpenDocument("README.md")
	queue.Drain()
	model.OpenDocument("src/main.go")
	queue.Drain()
	model.CloseTabs([]tabs.Tab{tabs.DocumentTab("README.md")})
	queue.Drain()
	model.GoBack()
	queue.Drain()
	if model.Active != tabs.DocumentTab("README.md") || model.Page.Selected != "README.md" {
		t.Fatalf("active %v page %q", model.Active, model.Page.Selected)
	}
	if !tabs.Includes(model.Tabs, tabs.DocumentTab("README.md")) {
		t.Fatalf("tabs %v", model.Tabs)
	}
}

// Back while another document loads ends on the document asked for: the load
// in flight does not replace the page kept.
func TestBackDuringALoad(t *testing.T) {
	model, queue, _ := setup(t)
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject(project)
	queue.Drain()
	model.OpenDocument("README.md")
	queue.Drain()
	model.OpenDocument("src/main.go")
	model.GoBack()
	queue.Drain()
	if model.Active != tabs.DocumentTab("README.md") || model.Page.Selected != "README.md" {
		t.Fatalf("active %v page %q", model.Active, model.Page.Selected)
	}
}
