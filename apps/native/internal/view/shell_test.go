package view

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"weak"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/apps/native/internal/memory"
	"github.com/flexdinesh/servef/apps/native/internal/shell"
	"github.com/flexdinesh/servef/apps/native/internal/tabs"
	"github.com/flexdinesh/servef/pkg/contract"
)

func (h *harness) openProject() {
	h.t.Helper()
	h.click("Open this computer")
	h.click("demo  ·  main")
}

func (h *harness) tabs() []tabs.Tab { return h.app.Model.Tabs }

// clickTab clicks a tab's label, left of its close action: the tree shows
// the same names first.
func (h *harness) clickTab(title string) {
	h.t.Helper()
	r, ok := h.tt.Find("Close " + title)
	if !ok {
		h.t.Fatalf("no tab %q; texts %q", title, h.tt.Texts())
	}
	h.tt.ClickAt(r.X-30, r.Y+r.H/2)
	h.settle()
}

// Tabs open from the tree, switch by click and arrow, and close from their
// close action, the keyboard, and their menu.
func TestTabStrip(t *testing.T) {
	h := newHarness(t)
	h.openProject()
	h.click("README.md")
	h.click("src")
	h.click("main.go")
	h.wantText("Open documents", "Close README.md", "Close src/main.go")
	if want := []tabs.Tab{tabs.DocumentTab("README.md"), tabs.DocumentTab("src/main.go")}; !slices.Equal(h.tabs(), want) {
		t.Fatalf("tabs %v", h.tabs())
	}
	// The mode switch follows the active document: Markdown has two views.
	if h.tt.HasText("Preview") {
		t.Fatal("main.go shows a mode switch")
	}
	h.clickTab("README.md")
	if h.app.Model.Active != tabs.DocumentTab("README.md") {
		t.Fatalf("active %v", h.app.Model.Active)
	}
	h.wantText("Preview", "Source")

	// The arrows choose the next tab.
	h.tt.Key(0, ui.KeyRight)
	h.settle()
	if h.app.Model.Active != tabs.DocumentTab("src/main.go") || h.app.Model.Page.Selected != "src/main.go" {
		t.Fatalf("arrow: %v %q", h.app.Model.Active, h.app.Model.Page.Selected)
	}
	// Delete closes the focused tab; its left neighbor shows.
	h.tt.Key(0, ui.KeyDelete)
	h.settle()
	if !slices.Equal(h.tabs(), []tabs.Tab{tabs.DocumentTab("README.md")}) || h.app.Model.Page.Selected != "README.md" {
		t.Fatalf("delete: %v %q", h.tabs(), h.app.Model.Page.Selected)
	}
	h.click("Close README.md")
	if len(h.tabs()) != 0 {
		t.Fatalf("close: %v", h.tabs())
	}
	h.wantText("Choose a file", "Open documents")
}

func TestTabMenu(t *testing.T) {
	h := newHarness(t)
	h.openProject()
	h.click("README.md")
	h.click("guide.md")
	h.click("src")
	h.click("main.go")
	if err := h.tt.RightClick("Close docs/guide.md"); err != nil {
		t.Fatal(err)
	}
	h.settle()
	if got, want := h.tt.Menu(), []string{"Close", "Close Others", "Close to the Right", "-", "Close All"}; !slices.Equal(got, want) {
		t.Fatalf("menu %q", got)
	}
	if err := h.tt.ChooseMenuItem("Close to the Right"); err != nil {
		t.Fatal(err)
	}
	h.settle()
	if want := []tabs.Tab{tabs.DocumentTab("README.md"), tabs.DocumentTab("docs/guide.md")}; !slices.Equal(h.tabs(), want) || h.app.Model.Page.Selected != "docs/guide.md" {
		t.Fatalf("close right: %v %q", h.tabs(), h.app.Model.Page.Selected)
	}
	if err := h.tt.RightClick("Close README.md"); err != nil {
		t.Fatal(err)
	}
	h.settle()
	if err := h.tt.ChooseMenuItem("Close Others"); err != nil {
		t.Fatal(err)
	}
	h.settle()
	if !slices.Equal(h.tabs(), []tabs.Tab{tabs.DocumentTab("README.md")}) {
		t.Fatalf("close others: %v", h.tabs())
	}
	if err := h.tt.RightClick("Close README.md"); err != nil {
		t.Fatal(err)
	}
	h.settle()
	if got := h.tt.Menu(); len(got) != 5 {
		t.Fatalf("menu %q", got)
	}
	if err := h.tt.ChooseMenuItem("Close All"); err != nil {
		t.Fatal(err)
	}
	h.settle()
	if len(h.tabs()) != 0 {
		t.Fatalf("close all: %v", h.tabs())
	}
}

// The activity bar shows a view, hides the sidebar on the shown view, and
// counts the pending changes; the empty pane follows the view.
func TestActivityBarAndEmptyPane(t *testing.T) {
	h := newHarness(t)
	h.daemon.LoadChanges(t, project)
	h.openProject()
	h.wantText("Choose a file", "Select a file from the folder tree.", "8")
	h.click("Explorer")
	if !h.app.Model.Sidebar.Hidden || h.tt.HasText("README.md") {
		t.Fatal("clicking the shown view hides the sidebar")
	}
	h.wantText("Nothing open", "Show sidebar  Ctrl+B")
	h.click("Show sidebar  Ctrl+B")
	h.wantText("README.md")

	h.click("Source Control")
	h.wantText("8 changes", "Review your changes", "8 uncommitted changes. Open a file, a commit, or a branch.")
	h.click("Review uncommitted changes")
	if h.app.Model.Active != tabs.Uncommitted() || h.app.Model.Review == nil {
		t.Fatalf("review %v %+v", h.app.Model.Active, h.app.Model.Review)
	}
	h.wantText("Uncommitted", "Close Uncommitted changes")

	h.click("Search")
	h.wantText("Search is not in the native app yet.")
	h.click("Walkthroughs")
	h.wantText("Walkthroughs is not in the native app yet.")

	// Cmd/Ctrl+B toggles the sidebar; Cmd/Ctrl+Shift+E shows the Explorer.
	h.tt.Key(ui.Cmd, ui.KeyB)
	h.settle()
	if !h.app.Model.Sidebar.Hidden {
		t.Fatal("Cmd+B")
	}
	h.tt.Key(ui.Cmd|ui.Shift, ui.KeyE)
	h.settle()
	if h.app.Model.Sidebar.Shown() != shell.Explorer {
		t.Fatalf("Cmd+Shift+E: %+v", h.app.Model.Sidebar)
	}
	h.tt.Key(ui.Cmd|ui.Shift, ui.KeyG)
	h.settle()
	if h.app.Model.Sidebar.Shown() != shell.SourceControl {
		t.Fatalf("Cmd+Shift+G: %+v", h.app.Model.Sidebar)
	}
}

// The sidebar's divider resizes it within the web's bounds.
func TestSidebarResize(t *testing.T) {
	h := newHarness(t)
	h.openProject()
	if h.app.sidebar != shell.DefaultWidth {
		t.Fatalf("width %v", h.app.sidebar)
	}
	x := activityWidth + shell.DefaultWidth
	h.tt.Press(x, 300)
	h.tt.Move(x+400, 300)
	h.tt.Release(x+400, 300)
	h.settle()
	if h.app.Model.Sidebar.Width != shell.MaxWidth || h.app.sidebar != shell.MaxWidth {
		t.Fatalf("dragged to %v (%v)", h.app.Model.Sidebar.Width, h.app.sidebar)
	}
	x = activityWidth + shell.MaxWidth
	h.tt.Press(x, 300)
	h.tt.Move(x-400, 300)
	h.tt.Release(x-400, 300)
	h.settle()
	if h.app.Model.Sidebar.Width != shell.MinWidth {
		t.Fatalf("dragged to %v", h.app.Model.Sidebar.Width)
	}
}

// memStore keeps memory in RAM.
type memStore struct{ file memory.File }

func (s *memStore) Load() memory.File  { return s.file }
func (s *memStore) Save(f memory.File) { s.file = f }

func (h *harness) remembering() *memStore {
	store := &memStore{}
	h.app.Model.UseMemory(store)
	return store
}

func (h *harness) expanded(store *memStore) []string {
	p, _ := store.file.Workspace(h.app.Model.Connection.ID).Project(project)
	return p.Expanded
}

// A click on a folder is remembered.
func TestToggledFoldersAreRemembered(t *testing.T) {
	h := newHarness(t)
	store := h.remembering()
	h.openProject()
	h.click("src")
	if got := h.expanded(store); !slices.Contains(got, "src") {
		t.Fatalf("expanded %q", got)
	}
	h.tt.Advance(time.Second)
	h.click("src")
	if got := h.expanded(store); slices.Contains(got, "src") {
		t.Fatalf("closed, expanded %q", got)
	}
}

// Folders a reveal opens are remembered too, as FileTree.tsx saves every
// expansion change.
func TestRevealedFoldersAreRemembered(t *testing.T) {
	h := newHarness(t)
	store := h.remembering()
	h.openProject()
	h.app.Model.OpenDocument("src/main.go")
	h.settle()
	if !h.tt.HasText("main.go") {
		t.Fatalf("not revealed; texts %q", h.tt.Texts())
	}
	if got := h.expanded(store); !slices.Contains(got, "src") {
		t.Fatalf("expanded %q", got)
	}
}

// A review tab says why it is empty when the changes cannot be read.
func TestReviewTabShowsAChangesFailure(t *testing.T) {
	h := newHarness(t)
	h.openProject()
	h.app.Model.OpenReview()
	h.settle()
	if h.tt.HasText("Reading changes…") {
		t.Fatal("still reading after the read failed")
	}
	h.wantText("Could not read changes: Project unavailable")
}

// The tab strip's band stays with no tabs, as the web's, so content does
// not shift when the first tab opens.
func TestTabBandStaysWithNoTabs(t *testing.T) {
	h := newHarness(t)
	h.openProject()
	h.wantText("Open documents")
	_, palette := windowTheme(ui.LightTheme())
	band := func(when string) {
		t.Helper()
		h.tt.Frame()
		x, y := 900, chromeHeight/2
		r, g, b, _ := h.tt.Image().At(x, y).RGBA()
		if got := fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8); got != palette.Chrome() {
			t.Fatalf("%s: the band at (%d, %d) is %s, not %s", when, x, y, got, palette.Chrome())
		}
		below := h.tt.Image().At(x, chromeHeight+2)
		if r2, g2, b2, _ := below.RGBA(); r2>>8 == r>>8 && g2>>8 == g>>8 && b2>>8 == b>>8 {
			t.Fatalf("%s: the band reaches below %d", when, chromeHeight)
		}
	}
	band("no tabs")
	h.click("README.md")
	band("a tab open")
}

// imageDocument adds an image file to the fake server.
func (h *harness) imageDocument(path string) {
	h.daemon.Set(func(s *fakeservef.Server) {
		s.Images[path] = pngBytes()
		page := s.Pages[project][""]
		page.Selected, page.HasFile, page.Binary = path, true, true
		page.DocumentType = &contract.FileType{ID: "png", Label: "PNG", PreviewKind: contract.PreviewImage, PreviewLabel: "Preview", DefaultMode: contract.ModePreview}
		s.Pages[project][path] = page
	})
}

// What a document holds in the view goes when no tab shows it.
func TestClosedDocumentsAreReleased(t *testing.T) {
	h := newHarness(t)
	h.imageDocument("docs/logo.png")
	h.openProject()
	h.app.Model.OpenDocument("docs/logo.png")
	h.settle()
	if h.app.doc.image == nil {
		t.Fatalf("image not shown; texts %q", h.tt.Texts())
	}
	h.app.Model.TabAction(tabs.CloseAll, 0)
	h.settle()
	if h.app.doc.image != nil {
		t.Fatal("Close All kept the decoded image")
	}

	h.app.Model.OpenDocument("src/main.go")
	h.settle()
	if h.app.code == nil {
		t.Fatal("source not shown")
	}
	h.app.Model.TabAction(tabs.CloseAll, 0)
	h.settle()
	if h.app.code != nil {
		t.Fatal("Close All kept the source editor")
	}

	h.app.Model.OpenDocument("README.md")
	h.settle()
	if h.app.blocks.document == "" {
		t.Fatal("preview not shown")
	}
	h.app.Model.TabAction(tabs.CloseAll, 0)
	h.settle()
	if h.app.blocks.document != "" || h.app.blocks.fences != nil || h.app.blocks.diagrams != nil {
		t.Fatalf("Close All kept the preview's blocks: %+v", h.app.blocks)
	}
}

// A closed review's diff and highlighted lines can be collected after Close
// All: the view holds nothing of them.
func TestClosedReviewIsReleased(t *testing.T) {
	h := newHarness(t)
	h.openChanges()
	h.click("code/main.go")
	h.settle()
	open := h.app.Model.Review
	if open == nil || open.Old == nil || h.app.review.lines != open.Version {
		t.Fatalf("diff not highlighted: %+v", open)
	}
	review := weak.Make(open)
	open = nil
	h.app.Model.TabAction(tabs.CloseAll, 0)
	// mygo reuses the last tree's nodes over the next frames, and a sync.Pool
	// keeps its items through one collection.
	h.settle()
	h.settle()
	h.settle()
	runtime.GC()
	runtime.GC()
	if review.Value() != nil {
		t.Fatal("the closed review is still reachable")
	}
	if h.app.review.spans[0] != nil || h.app.review.spans[1] != nil {
		t.Fatal("the view kept the closed review's spans")
	}
}

// Leaving the project drops the view's review too.
func TestLeavingTheProjectReleasesTheReview(t *testing.T) {
	h := newHarness(t)
	h.openChanges()
	h.click("code/main.go")
	h.settle()
	if h.app.review.of == nil {
		t.Fatal("review not shown")
	}
	h.app.Model.CloseProject()
	h.settle()
	if h.app.review.of != nil || h.app.review.spans[0] != nil || h.app.review.spans[1] != nil {
		t.Fatal("the view kept the closed review")
	}
}

// Another document's view drops the last one's, whichever kind it was.
func TestAnotherDocumentReleasesTheLast(t *testing.T) {
	h := newHarness(t)
	h.imageDocument("docs/logo.png")
	h.openProject()
	h.app.Model.OpenDocument("docs/logo.png")
	h.settle()
	h.app.Model.OpenDocument("src/main.go")
	h.settle()
	if h.app.doc.image != nil {
		t.Fatal("the source view kept the image")
	}
	h.app.Model.OpenDocument("README.md")
	h.settle()
	if h.app.code != nil {
		t.Fatal("the preview kept the source editor")
	}
	// Back to an image reads it again.
	h.app.Model.OpenDocument("docs/logo.png")
	h.settle()
	if h.app.doc.image == nil {
		t.Fatal("the image did not come back")
	}
}

// Source and Preview of one document keep the source editor between them.
func TestModeSwitchKeepsTheEditor(t *testing.T) {
	h := newHarness(t)
	h.openProject()
	h.click("README.md")
	h.click("Source")
	state := h.app.code
	if state == nil {
		t.Fatal("source not shown")
	}
	h.click("Preview")
	h.click("Source")
	if h.app.code != state {
		t.Fatal("a mode switch rebuilt the editor")
	}
}

// Hiding the sidebar keeps the document where it is scrolled.
func TestSidebarToggleKeepsThePreviewScroll(t *testing.T) {
	h := newHarness(t)
	h.daemon.Set(func(s *fakeservef.Server) {
		page := s.Pages[project]["README.md"]
		page.Source = "# Top\n\n" + strings.Repeat("A paragraph of text.\n\n", 5) + "## Marker\n\n" + strings.Repeat("A paragraph of text.\n\n", 300)
		s.Pages[project]["README.md"] = page
	})
	h.openProject()
	h.click("README.md")
	marker, ok := h.tt.Find("Marker")
	if !ok {
		t.Fatalf("texts %q", h.tt.Texts())
	}
	h.tt.Scroll(800, 400, 0, 100)
	h.settle()
	scrolled, ok := h.tt.Find("Marker")
	if !ok || scrolled.Y >= marker.Y {
		t.Fatalf("not scrolled: %+v, then %+v (%v)", marker, scrolled, ok)
	}
	h.tt.Key(ui.Cmd, ui.KeyB)
	h.settle()
	if !h.app.Model.Sidebar.Hidden {
		t.Fatal("not hidden")
	}
	hidden, ok := h.tt.Find("Marker")
	if !ok || hidden.Y != scrolled.Y {
		t.Fatalf("hiding moved the document from %+v to %+v (%v)", scrolled, hidden, ok)
	}
	h.tt.Key(ui.Cmd, ui.KeyB)
	h.settle()
	shown, ok := h.tt.Find("Marker")
	if !ok || shown.Y != scrolled.Y {
		t.Fatalf("showing moved the document from %+v to %+v (%v)", scrolled, shown, ok)
	}
}

// A review over an open document keeps its editor, so the document comes
// back as it was, and lets its decoded image go.
func TestReviewKeepsAnOpenDocumentsEditor(t *testing.T) {
	h := newHarness(t)
	h.imageDocument("docs/logo.png")
	h.daemon.LoadChanges(t, project)
	h.openProject()
	h.app.Model.OpenDocument("src/main.go")
	h.settle()
	state := h.app.code
	h.app.Model.OpenReview()
	h.settle()
	if h.app.code != state {
		t.Fatal("a review dropped the open document's editor")
	}
	h.app.Model.SelectTab(tabs.DocumentTab("src/main.go"))
	h.settle()
	if h.app.code != state {
		t.Fatal("returning rebuilt the editor")
	}
	h.app.Model.OpenDocument("docs/logo.png")
	h.settle()
	if h.app.doc.image == nil {
		t.Fatal("image not shown")
	}
	h.app.Model.OpenReview()
	h.settle()
	if h.app.doc.image != nil {
		t.Fatal("a review kept the decoded image")
	}
	h.app.Model.SelectTab(tabs.DocumentTab("docs/logo.png"))
	h.settle()
	if h.app.doc.image == nil {
		t.Fatal("the image did not come back")
	}
}
