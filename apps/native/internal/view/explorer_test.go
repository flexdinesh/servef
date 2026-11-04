package view

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/apps/native/internal/tabs"
	"github.com/flexdinesh/servef/pkg/contract"
)

func openDemo(t *testing.T, changes []contract.ChangedFile) *harness {
	t.Helper()
	h := newHarness(t)
	h.daemon.Set(func(s *fakeservef.Server) {
		s.Changes[project] = contract.ChangesResponse{Repository: true, Branch: "main", Files: changes}
	})
	h.click("Open this computer")
	h.click("demo  ·  main")
	return h
}

// The Explorer shows the statuses of the files it shows once the
// project's changes are read (on load, as the web), and Pierre's order:
// folders first.
func TestExplorerDecoratesShownChanges(t *testing.T) {
	h := openDemo(t, []contract.ChangedFile{
		{Path: "docs/guide.md", Status: contract.ChangeModified},
		{Path: "README.md", Status: contract.ChangeAdded},
		{Path: "gone.md", Status: contract.ChangeDeleted},
	})
	h.wantText("guide.md", "M", "A")
	if slices.Contains(h.tt.Texts(), "D") || h.tt.HasText("gone.md") {
		t.Fatalf("a path the tree lacks carries no decoration; %q", h.tt.Texts())
	}
	if got := h.app.explorer.state.Rows(); len(got) != 4 || got[0] != "docs" || got[2] != "src" || got[3] != "README.md" {
		t.Fatalf("rows %v", got)
	}
}

// Clicking a folder toggles it without opening anything; clicking a file
// opens it and selects its row; the keyboard moves the cursor and Enter
// opens.
func TestExplorerClicksAndKeys(t *testing.T) {
	h := openDemo(t, nil)
	reads := 0
	h.daemon.Get(func(s *fakeservef.Server) { reads = s.PageReads })
	h.click("src")
	h.wantText("main.go")
	h.daemon.Get(func(s *fakeservef.Server) {
		if s.PageReads != reads {
			t.Fatal("a folder click reads no page")
		}
	})
	h.click("main.go")
	if h.app.Model.Page.Selected != "src/main.go" || h.app.explorer.state.Selected != "src/main.go" {
		t.Fatalf("selected %q / %q", h.app.Model.Page.Selected, h.app.explorer.state.Selected)
	}
	h.tt.Key(0, ui.KeyHome)
	h.settle()
	h.tt.Key(0, ui.KeyDown)
	h.settle()
	if h.app.explorer.state.Focused() != "docs/guide.md" || h.app.Model.Page.Selected != "src/main.go" {
		t.Fatalf("the cursor moves without opening; focused %q", h.app.explorer.state.Focused())
	}
	h.tt.Key(0, ui.KeyEnter)
	h.settle()
	if h.app.Model.Page.Selected != "docs/guide.md" {
		t.Fatalf("Enter opens; selected %q", h.app.Model.Page.Selected)
	}
}

// A live update rereads the tree; open folders, the selection, and the
// new file show without resetting the tree.
func TestExplorerKeepsStateAcrossLiveUpdates(t *testing.T) {
	h := openDemo(t, nil)
	h.click("src")
	h.click("main.go")
	h.daemon.Set(func(s *fakeservef.Server) {
		for path, page := range s.Pages[project] {
			nodes := append([]contract.TreeNode(nil), page.Tree...)
			src := nodes[1]
			src.Children = append(append([]contract.TreeNode(nil), src.Children...), contract.TreeNode{Name: "util.go", Path: "src/util.go", FileType: "go"})
			nodes[1] = src
			page.Tree = nodes
			s.Pages[project][path] = page
		}
	})
	h.daemon.Send(contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Paths: []string{"src/util.go"}, Tree: true}})
	if !h.queue.Until(func() bool { return h.app.Model.Tree.Len() == 6 }, 5*time.Second) {
		t.Fatal("no reread")
	}
	h.settle()
	h.wantText("util.go", "main.go")
	if !h.app.explorer.state.IsOpen("src") || !h.app.explorer.state.IsOpen("docs") || h.app.explorer.state.Selected != "src/main.go" {
		t.Fatalf("open %v selected %q", h.app.explorer.state.OpenPaths(), h.app.explorer.state.Selected)
	}
}

// Single-child folder chains are one row, as the web's tree flattens them.
func TestExplorerFlattensFolderChains(t *testing.T) {
	tr := treeOf([]contract.TreeNode{
		{Name: "pkg", Path: "pkg", IsDir: true, Children: []contract.TreeNode{
			{Name: "util", Path: "pkg/util", IsDir: true, Children: []contract.TreeNode{{Name: "x.go", Path: "pkg/util/x.go"}}},
		}},
		{Name: "file10.md", Path: "file10.md"},
		{Name: "file2.md", Path: "file2.md"},
	})
	if got := tr.Roots(); !slices.Equal(got, []string{"pkg/util", "file2.md", "file10.md"}) || tr.Name("pkg/util") != "pkg / util" {
		t.Fatalf("roots %v, chain %q", got, tr.Name("pkg/util"))
	}
}

// Another project starts with no row selected, no cursor, at the top,
// and only the server's open folders.
func TestExplorerResetsForAnotherProject(t *testing.T) {
	h := newHarness(t)
	h.daemon.Set(func(s *fakeservef.Server) {
		for path, page := range s.Pages[project] {
			nodes := append([]contract.TreeNode(nil), page.Tree...)
			for i := range 200 {
				name := fmt.Sprintf("note%03d.md", i)
				nodes = append(nodes, contract.TreeNode{Name: name, Path: name, FileType: "markdown"})
			}
			page.Tree = nodes
			s.Pages[project][path] = page
		}
	})
	h.click("Open this computer")
	h.click("demo  ·  main")
	h.wantText("docs")
	h.tt.Scroll(100, 300, 0, 3000)
	h.settle()
	at := slices.IndexFunc(h.tt.Texts(), func(text string) bool { return strings.HasPrefix(text, "note1") })
	if at < 0 {
		t.Fatalf("no row scrolled into view; texts %q", h.tt.Texts())
	}
	h.click(h.tt.Texts()[at])
	s := &h.app.explorer.state
	if first, _ := s.Visible(); first == 0 {
		t.Fatal("the tree did not scroll")
	}
	// Closed tabs leave nothing for the project's memory to restore.
	h.app.Model.TabAction(tabs.CloseAll, 0)
	h.app.Model.OpenProject(project)
	h.settle()
	h.wantText("docs")
	if first, _ := s.Visible(); first != 0 || s.Selected != "" || s.Focused() != "" {
		t.Fatalf("first row %d, selected %q, focused %q", first, s.Selected, s.Focused())
	}
}
