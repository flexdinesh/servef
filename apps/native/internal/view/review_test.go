package view

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/apps/native/internal/highlight"
	"github.com/flexdinesh/servef/apps/native/internal/link"
	"github.com/flexdinesh/servef/apps/native/internal/theme"
	"github.com/flexdinesh/servef/packages/mygo/diffs"
	"github.com/flexdinesh/servef/pkg/contract"
)

func (h *harness) openChanges() {
	h.t.Helper()
	h.daemon.LoadChanges(h.t, project)
	h.click("Open this computer")
	h.app.Model.OpenLink(link.Target{Kind: link.Local, Project: project, Path: "src/main.go"})
	h.settle()
	h.click("Source Control")
}

func TestChangesListAndDiffViewer(t *testing.T) {
	h := newHarness(t)
	h.openChanges()
	h.wantText("8 changes", "code/main.go", "guides/intro.md → guides/getting-started.md", "M", "R", "U", "D")

	h.click("code/main.go")
	h.wantText("+1 −1", "Split", "Unified", "Full file", "Ignore whitespace", "Close")
	if title := h.app.Model.Title(); title != "code/main.go (changes) - demo - servef" {
		t.Fatalf("title %q", title)
	}
	v := &h.app.review.viewer
	if v.Style != diffs.Split {
		t.Fatalf("style %d", v.Style)
	}
	// The viewer shows the server's hunk over the whole files: the gap
	// before it collapses, the pair's changed words are emphasized.
	h.wantText("2 unmodified lines", "L7", "R7", "R3")
	d := v.Diff()
	if len(d.Hunks) != 1 || d.Hunks[0].OldStart != 3 || len(d.New) != 8 || d.Text(diffs.New, 7) != "\tfmt.Println(\"hello from servef\") // go-search-token" {
		t.Fatalf("diff %+v", d)
	}
	// Highlighted from the full file in the server's language.
	h.settle()
	if spans := v.Spans(diffs.New, 6); len(spans) == 0 || spans[0].Color != ui.Hex(theme.Light.Syntax[highlight.Keyword]) {
		t.Fatalf("spans of func main() { %+v", spans)
	}

	h.click("Unified")
	if v.Style != diffs.Unified {
		t.Fatal("unified layout")
	}
	h.wantText("L7", "R7")
	if h.tt.HasText("L3") {
		t.Fatal("unified unchanged rows number the new side")
	}

	h.click("Close")
	h.wantText("src/main.go")
	if h.app.Model.Review != nil {
		t.Fatal("closed")
	}
}

func TestDiffViewerGapsAndFullFile(t *testing.T) {
	h := newHarness(t)
	h.openChanges()
	h.click("notes/hunks.txt")
	h.wantText("+4 −4", "9 unmodified lines")
	if h.tt.HasText("line 10") {
		t.Fatal("the gap shows")
	}
	// A gap expands in place.
	h.click("9 unmodified lines")
	h.wantText("line 10", "line 12")
	if h.tt.HasText("9 unmodified lines") {
		t.Fatal("the gap still collapses")
	}
	h.click("Full file")
	if !h.app.Model.FullFile || !h.app.review.viewer.ExpandUnchanged {
		t.Fatal("full file")
	}
	h.click("Ignore whitespace")
	h.daemon.Get(func(s *fakeservef.Server) {
		if got := s.DiffQueries[len(s.DiffQueries)-1].Get("ignoreWhitespace"); got != "1" {
			t.Fatalf("ignoreWhitespace=%q", got)
		}
	})
}

func TestDiffLineSelectionCopiesAReference(t *testing.T) {
	h := newHarness(t)
	h.openChanges()
	h.click("notes/hunks.txt")
	h.click("R3")
	if err := h.tt.ClickWith(ui.Shift, "R5"); err != nil {
		t.Fatal(err)
	}
	h.settle()
	s, ok := h.app.review.viewer.Selection()
	if !ok || s.Anchor() != "R3-R5" {
		t.Fatalf("selection %v %v", s, ok)
	}
	h.click("Copy notes/hunks.txt:3-5")
	if got := h.tt.Clipboard(); got != "notes/hunks.txt:3-5" {
		t.Fatalf("clipboard %q", got)
	}
	// The old side's lines are the old path's, here the same file.
	h.click("L2")
	h.click("Copy notes/hunks.txt:2")
}

func TestDiffHunkSteps(t *testing.T) {
	h := newHarness(t)
	h.openChanges()
	h.click("notes/hunks.txt")
	h.click("Next change")
	if s, _ := h.app.review.viewer.Selection(); s.Anchor() != "R1-R6" {
		t.Fatalf("first step %q", s.Anchor())
	}
	// F7 steps while the viewer has the focus, as a click on a line gives.
	h.click("R3")
	h.tt.Key(0, ui.KeyF7)
	h.settle()
	if s, _ := h.app.review.viewer.Selection(); s.Anchor() != "R16-R23" {
		t.Fatalf("F7 %q", s.Anchor())
	}
	h.click("Previous change")
	if s, _ := h.app.review.viewer.Selection(); s.Anchor() != "R1-R6" {
		t.Fatalf("previous %q", s.Anchor())
	}
	h.wantText("Copy notes/hunks.txt:1-6")
}

func TestDiffMessages(t *testing.T) {
	h := newHarness(t)
	h.openChanges()
	h.click("assets/sample.bin")
	h.wantText("Binary file changed · 12.0 KB → 14.0 KB")
	h.click("guides/intro.md → guides/getting-started.md")
	h.wantText("File contents are unchanged.")
	h.daemon.Set(func(s *fakeservef.Server) { delete(s.Diffs, "notes.txt") })
	h.click("notes.txt")
	h.wantText("No changes to notes.txt since the last commit.")
}

func TestChangesOutsideARepository(t *testing.T) {
	h := newHarness(t)
	var folder contract.ChangesResponse
	fakeservef.Fixture(t, "changes-not-repo.json", &folder)
	h.daemon.Set(func(s *fakeservef.Server) { s.Changes[project] = folder })
	h.click("Open this computer")
	h.app.Model.OpenProject(project)
	h.settle()
	h.click("Source Control")
	h.wantText("Source Control needs a git repository.")
	h.click("Explorer")
	h.wantText("README.md")
}
