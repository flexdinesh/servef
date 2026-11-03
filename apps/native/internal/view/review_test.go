package view

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/apps/native/internal/link"
	"github.com/flexdinesh/servef/pkg/contract"
)

func (h *harness) openChanges() {
	h.t.Helper()
	h.daemon.LoadChanges(h.t, project)
	h.daemon.Set(func(s *fakeservef.Server) {
		s.Diffs["notes/hunks.txt"] = fakeservef.Diff(h.t, "diff-hunks.json")
		changes := s.Changes[project]
		changes.Files = append(changes.Files, contract.ChangedFile{Path: "notes/hunks.txt", Status: contract.ChangeModified})
		s.Changes[project] = changes
	})
	h.click("Open this computer")
	h.app.Model.OpenLink(link.Target{Kind: link.Local, Project: project, Path: "src/main.go"})
	h.settle()
	h.click("Changes")
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
	state := h.app.review.state
	if state == nil || state.Layout != editor.DiffSplit {
		t.Fatalf("diff state %+v", state)
	}
	// The viewer's hunks are the server's change run, not its own diff.
	if got := state.Hunks(); !slices.Equal(got, []editor.DiffHunk{{OldStart: 4, OldEnd: 5, NewStart: 4, NewEnd: 5}}) {
		t.Fatalf("hunks %v", got)
	}
	if got := state.New.Line(4); got != "\tfmt.Println(\"hello from servef\") // go-search-token" {
		t.Fatalf("new line 4 %q", got)
	}
	// Highlighted from the full file in the server's language.
	language := state.New.Language()
	if language == nil || language.Name != "go" {
		t.Fatalf("language %+v", language)
	}
	tokens, _ := language.Highlighter(state.New.Snapshot(), func() {}).Tokens(3, "func main() {", nil)
	if len(tokens) == 0 || tokens[0].Kind != editor.TokenKeyword {
		t.Fatalf("tokens %v", tokens)
	}

	h.click("Unified")
	if h.app.review.state == state || h.app.review.state.Layout != editor.DiffUnified {
		t.Fatal("unified layout")
	}

	h.click("Close")
	h.wantText("src/main.go")
	if h.app.Model.Review != nil {
		t.Fatal("closed")
	}
}

func TestDiffViewerHunksAndFullFile(t *testing.T) {
	h := newHarness(t)
	h.openChanges()
	h.click("notes/hunks.txt")
	h.wantText("+4 −4", "10 unmodified lines hidden")
	state := h.app.review.state
	if got := len(state.Hunks()); got != 5 {
		t.Fatalf("%d change runs", got)
	}
	if !strings.HasPrefix(state.Old.String(), "line 1\nline 2\nline 3") || strings.Contains(state.Old.String(), "line 10\n") {
		t.Fatalf("collapsed old text %q", state.Old.String())
	}
	h.click("Full file")
	if h.app.review.state == state || !h.app.Model.FullFile {
		t.Fatal("full file rebuilt the viewer")
	}
	if got := h.app.review.state.Old.LineCount(); got != 40 {
		t.Fatalf("full old side %d lines", got)
	}
	if h.tt.HasText("10 unmodified lines hidden") {
		t.Fatal("full file hides nothing")
	}
	h.click("Ignore whitespace")
	h.daemon.Get(func(s *fakeservef.Server) {
		if got := s.DiffQueries[len(s.DiffQueries)-1].Get("ignoreWhitespace"); got != "1" {
			t.Fatalf("ignoreWhitespace=%q", got)
		}
	})
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
	h.click("Changes")
	h.wantText("Source Control needs a git repository.")
	h.click("Files")
	h.wantText("README.md")
}
